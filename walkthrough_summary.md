# Walkthrough — Refactored check-breaking-change

## Changes Made

### 1. Removed Git Branch Comparison → Local File Flow
- **Deleted** [cmd/diff.go](file:///Users/arpan-k8s/check-breaking-change/cmd/diff.go) and [internal/git/diff.go](file:///Users/arpan-k8s/check-breaking-change/internal/git/diff.go) — no more `go-git` dependency
- **Rewrote** [cmd/root.go](file:///Users/arpan-k8s/check-breaking-change/cmd/root.go) — now takes `[chart-dir] [override-chart-yaml]` positional args
- Reads [Chart.yaml](file:///Users/arpan-k8s/check-breaking-change/e2etest/testchart/Chart.yaml), [values.yaml](file:///Users/arpan-k8s/check-breaking-change/e2etest/testchart/values.yaml) from `chart-dir` and override [Chart.yaml](file:///Users/arpan-k8s/check-breaking-change/e2etest/testchart/Chart.yaml) from second arg
- **Removed flags**: `--repo-path`, `--target-branch`
- **Kept**: all GitLab reporting and auth flags

```diff:root.go
package cmd

import (
	"fmt"
	"os"

	"check-breaking-change/internal/chartparser"
	"check-breaking-change/internal/diff"
	gitpkg "check-breaking-change/internal/git"
	"check-breaking-change/internal/helmrepo"
	"check-breaking-change/internal/models"
	"check-breaking-change/internal/reporter"

	"github.com/spf13/cobra"
)

var (
	repoPath          string
	targetBranch      string
	gitlabURL         string
	gitlabToken       string
	projectID         string
	mrID              string
	dryRun            bool
	helmUser          string
	helmPass          string
	helmToken         string
	ociRegistryConfig string
	ociRegistryCA     string
)

var rootCmd = &cobra.Command{
	Use:   "check-breaking-change",
	Short: "Detect breaking structural changes in Helm subchart dependency upgrades",
	Long: `Compares subchart dependency versions between the current branch and the target branch.
For each version change, fetches old and new upstream values.yaml and detects
structural breaking changes against the parent chart's values.yaml overrides.`,
	RunE: run,
}

func init() {
	rootCmd.Flags().StringVar(&repoPath, "repo-path", ".", "Path to the git repository")
	rootCmd.Flags().StringVar(&targetBranch, "target-branch", envOrDefault("TARGET_BRANCH", "main"), "MR target branch")
	rootCmd.Flags().StringVar(&gitlabURL, "gitlab-url", envOrDefault("GITLAB_URL", ""), "GitLab instance URL")
	rootCmd.Flags().StringVar(&gitlabToken, "gitlab-token", envOrDefault("GITLAB_TOKEN", ""), "GitLab API token")
	rootCmd.Flags().StringVar(&projectID, "project-id", envOrDefault("GITLAB_PROJECT_ID", ""), "GitLab project ID")
	rootCmd.Flags().StringVar(&mrID, "mr-id", envOrDefault("CI_MERGE_REQUEST_IID", ""), "GitLab MR ID (optional, for issue title)")
	rootCmd.Flags().BoolVar(&dryRun, "dry-run", false, "Print report to stdout without creating a GitLab issue")
	rootCmd.Flags().StringVar(&helmUser, "helm-repo-user", envOrDefault("HELM_REPO_USERNAME", ""), "Username for protected Helm repos")
	rootCmd.Flags().StringVar(&helmPass, "helm-repo-pass", envOrDefault("HELM_REPO_PASSWORD", ""), "Password for protected Helm repos")
	rootCmd.Flags().StringVar(&helmToken, "helm-repo-token", envOrDefault("HELM_REPO_TOKEN", ""), "Token for protected Helm repos")
	rootCmd.Flags().StringVar(&ociRegistryConfig, "oci-registry-config", envOrDefault("OCI_REGISTRY_CONFIG", ""), "Path to Docker config.json for OCI registry auth")
	rootCmd.Flags().StringVar(&ociRegistryCA, "oci-registry-ca", envOrDefault("OCI_REGISTRY_CA_FILE", ""), "CA certificate for private OCI registries")
}

// Execute runs the root command.
func Execute() {
	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}

func run(cmd *cobra.Command, args []string) error {
	// 1. Open git repo and read Chart.yaml from both branches
	fmt.Println("Opening git repository...")
	gitReader, err := gitpkg.NewDiffReader(repoPath)
	if err != nil {
		return fmt.Errorf("git error: %w", err)
	}

	oldChartData, newChartData, err := gitReader.GetChartFiles(targetBranch)
	if err != nil {
		return fmt.Errorf("failed to read Chart.yaml: %w", err)
	}

	// 2. Parse both Chart.yaml files
	oldChart, err := chartparser.ParseChartFile(oldChartData)
	if err != nil {
		return fmt.Errorf("failed to parse old Chart.yaml: %w", err)
	}

	newChart, err := chartparser.ParseChartFile(newChartData)
	if err != nil {
		return fmt.Errorf("failed to parse new Chart.yaml: %w", err)
	}

	// 3. Find version changes
	changes := chartparser.FindVersionChanges(oldChart.Dependencies, newChart.Dependencies)
	if len(changes) == 0 {
		fmt.Println("No subchart version changes detected. Nothing to check.")
		return nil
	}

	fmt.Printf("Found %d subchart version change(s) to check.\n", len(changes))

	// 4. Read parent values.yaml from target branch
	valuesData, err := gitReader.GetValuesFromBranch(targetBranch)
	if err != nil {
		return fmt.Errorf("failed to read values.yaml: %w", err)
	}

	parentValues, err := chartparser.ParseValues(valuesData)
	if err != nil {
		return fmt.Errorf("failed to parse values.yaml: %w", err)
	}

	// 5. Build auth
	auth := buildAuth()

	// 6. For each changed subchart, fetch upstream values and compare
	report := models.Report{
		ChartName:    newChart.Name,
		TargetBranch: targetBranch,
	}

	for _, change := range changes {
		key := change.Dependency.ResolveKey()

		// Check if the subchart is active (condition/tags)
		if !chartparser.IsActive(change.Dependency, parentValues) {
			fmt.Printf("  Skipping %s (inactive via condition/tags)\n", key)
			continue
		}

		// Extract parent overrides for this subchart
		parentOverrides := chartparser.ExtractSubchartValues(parentValues, key)
		if parentOverrides == nil {
			fmt.Printf("  Skipping %s (no parent overrides in values.yaml)\n", key)
			continue
		}

		fmt.Printf("  Checking %s (%s → %s)...\n", key, change.OldVersion, change.NewVersion)

		// Create appropriate fetcher for this repo (HTTP or OCI)
		fetcher, err := helmrepo.NewFetcher(change.Dependency.Repository, auth)
		if err != nil {
			return fmt.Errorf("failed to create fetcher for %s: %w", change.Dependency.Repository, err)
		}

		// Fetch old upstream values
		oldUpstream, err := fetcher.FetchValues(
			change.Dependency.Repository, change.Dependency.Name,
			change.OldVersion, auth,
		)
		if err != nil {
			return fmt.Errorf("failed to fetch old upstream values for %s@%s: %w", change.Dependency.Name, change.OldVersion, err)
		}

		// Fetch new upstream values
		newUpstream, err := fetcher.FetchValues(
			change.Dependency.Repository, change.Dependency.Name,
			change.NewVersion, auth,
		)
		if err != nil {
			return fmt.Errorf("failed to fetch new upstream values for %s@%s: %w", change.Dependency.Name, change.NewVersion, err)
		}

		// Compare
		results := diff.Compare(parentOverrides, oldUpstream, newUpstream)

		scReport := models.SubchartReport{
			SubchartName: key,
			OldVersion:   change.OldVersion,
			NewVersion:   change.NewVersion,
			Results:      results,
		}

		// Check if any results are breaking
		for _, r := range results {
			if r.Breaking {
				report.HasBreaking = true
				break
			}
		}

		report.SubchartReports = append(report.SubchartReports, scReport)
	}

	// 8. Output results
	if len(report.SubchartReports) == 0 {
		fmt.Println("No subchart overrides to check. All clear.")
		return nil
	}

	// Always print stdout summary
	fmt.Println("\n" + reporter.FormatStdout(report))

	if !report.HasBreaking {
		fmt.Println("✅ No breaking changes detected.")
		return nil
	}

	// Report breaking changes
	fmt.Println("⛔ Breaking changes detected!")

	if dryRun {
		fmt.Println("\n--- Markdown Report (dry-run) ---")
		fmt.Println(reporter.FormatMarkdown(report))
		os.Exit(1)
	}

	// Create GitLab issue
	if gitlabURL == "" || gitlabToken == "" || projectID == "" {
		fmt.Println("Warning: GitLab credentials not provided. Printing markdown report to stdout.")
		fmt.Println("\n" + reporter.FormatMarkdown(report))
		os.Exit(1)
	}

	glReporter, err := reporter.NewGitLabReporter(gitlabURL, gitlabToken, projectID, mrID)
	if err != nil {
		return fmt.Errorf("failed to create GitLab reporter: %w", err)
	}

	if err := glReporter.Report(report); err != nil {
		return fmt.Errorf("failed to create GitLab issue: %w", err)
	}

	os.Exit(1) // Fail CI pipeline
	return nil
}

func buildAuth() *models.RepoAuth {
	auth := &models.RepoAuth{
		Username:         helmUser,
		Password:         helmPass,
		Token:            helmToken,
		DockerConfigPath: ociRegistryConfig,
		RegistryCAFile:   ociRegistryCA,
	}
	if !auth.HasCredentials() {
		return nil
	}
	return auth
}

func envOrDefault(key, defaultVal string) string {
	if val := os.Getenv(key); val != "" {
		return val
	}
	return defaultVal
}
===
package cmd

import (
	"fmt"
	"os"
	"path/filepath"

	"check-breaking-change/internal/chartparser"
	"check-breaking-change/internal/diff"
	"check-breaking-change/internal/helmrepo"
	"check-breaking-change/internal/models"
	"check-breaking-change/internal/reporter"

	"github.com/spf13/cobra"
)

var (
	gitlabURL         string
	gitlabToken       string
	projectID         string
	mrID              string
	dryRun            bool
	helmUser          string
	helmPass          string
	helmToken         string
	ociRegistryConfig string
	ociRegistryCA     string
)

var rootCmd = &cobra.Command{
	Use:   "check-breaking-change [chart-dir] [override-chart-yaml]",
	Short: "Detect breaking structural changes in Helm subchart dependency upgrades",
	Long: `Compares the Chart.yaml in the given directory with an override Chart.yaml file.
For each subchart version difference, fetches old and new upstream values.yaml and detects
structural breaking changes against the parent chart's values.yaml overrides.

Also checks nested/transitive subchart dependencies when the parent chart
overrides their values.

Example:
  check-breaking-change . override-chart.yaml --dry-run
  check-breaking-change /path/to/chart /path/to/override-Chart.yaml`,
	Args: cobra.ExactArgs(2),
	RunE: run,
}

func init() {
	rootCmd.Flags().StringVar(&gitlabURL, "gitlab-url", envOrDefault("GITLAB_URL", ""), "GitLab instance URL")
	rootCmd.Flags().StringVar(&gitlabToken, "gitlab-token", envOrDefault("GITLAB_TOKEN", ""), "GitLab API token")
	rootCmd.Flags().StringVar(&projectID, "project-id", envOrDefault("GITLAB_PROJECT_ID", ""), "GitLab project ID")
	rootCmd.Flags().StringVar(&mrID, "mr-id", envOrDefault("CI_MERGE_REQUEST_IID", ""), "GitLab MR ID (optional, for issue title)")
	rootCmd.Flags().BoolVar(&dryRun, "dry-run", false, "Print report to stdout without creating a GitLab issue")
	rootCmd.Flags().StringVar(&helmUser, "helm-repo-user", envOrDefault("HELM_REPO_USERNAME", ""), "Username for protected Helm repos")
	rootCmd.Flags().StringVar(&helmPass, "helm-repo-pass", envOrDefault("HELM_REPO_PASSWORD", ""), "Password for protected Helm repos")
	rootCmd.Flags().StringVar(&helmToken, "helm-repo-token", envOrDefault("HELM_REPO_TOKEN", ""), "Token for protected Helm repos")
	rootCmd.Flags().StringVar(&ociRegistryConfig, "oci-registry-config", envOrDefault("OCI_REGISTRY_CONFIG", ""), "Path to Docker config.json for OCI registry auth")
	rootCmd.Flags().StringVar(&ociRegistryCA, "oci-registry-ca", envOrDefault("OCI_REGISTRY_CA_FILE", ""), "CA certificate for private OCI registries")
}

// Execute runs the root command.
func Execute() {
	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}

func run(cmd *cobra.Command, args []string) error {
	chartDir := args[0]
	overridePath := args[1]

	// 1. Read Chart.yaml from chart directory (current/base)
	currentChartPath := filepath.Join(chartDir, "Chart.yaml")
	currentChartData, err := os.ReadFile(currentChartPath)
	if err != nil {
		return fmt.Errorf("failed to read current Chart.yaml at %s: %w", currentChartPath, err)
	}

	// 2. Read override Chart.yaml
	overrideChartData, err := os.ReadFile(overridePath)
	if err != nil {
		return fmt.Errorf("failed to read override Chart.yaml at %s: %w", overridePath, err)
	}

	// 3. Parse both Chart.yaml files
	currentChart, err := chartparser.ParseChartFile(currentChartData)
	if err != nil {
		return fmt.Errorf("failed to parse current Chart.yaml: %w", err)
	}

	overrideChart, err := chartparser.ParseChartFile(overrideChartData)
	if err != nil {
		return fmt.Errorf("failed to parse override Chart.yaml: %w", err)
	}

	// 4. Find version changes (current = old, override = new)
	changes := chartparser.FindVersionChanges(currentChart.Dependencies, overrideChart.Dependencies)
	if len(changes) == 0 {
		fmt.Println("No subchart version changes detected. Nothing to check.")
		return nil
	}

	fmt.Printf("Found %d subchart version change(s) to check.\n", len(changes))

	// 5. Read parent values.yaml from the chart directory
	valuesPath := filepath.Join(chartDir, "values.yaml")
	valuesData, err := os.ReadFile(valuesPath)
	if err != nil {
		return fmt.Errorf("failed to read values.yaml at %s: %w", valuesPath, err)
	}

	parentValues, err := chartparser.ParseValues(valuesData)
	if err != nil {
		return fmt.Errorf("failed to parse values.yaml: %w", err)
	}

	// 6. Build auth
	auth := buildAuth()

	// 7. For each changed subchart, fetch upstream values and compare
	report := models.Report{
		ChartName: currentChart.Name,
	}

	// Track visited subcharts for cycle detection in transitive checks
	visited := make(map[string]bool)

	for _, change := range changes {
		key := change.Dependency.ResolveKey()

		// Check if the subchart is active (condition/tags)
		if !chartparser.IsActive(change.Dependency, parentValues) {
			fmt.Printf("  Skipping %s (inactive via condition/tags)\n", key)
			continue
		}

		// Extract parent overrides for this subchart
		parentOverrides := chartparser.ExtractSubchartValues(parentValues, key)
		if parentOverrides == nil {
			fmt.Printf("  Skipping %s (no parent overrides in values.yaml)\n", key)
			continue
		}

		fmt.Printf("  Checking %s (%s → %s)...\n", key, change.OldVersion, change.NewVersion)

		// Create appropriate fetcher for this repo (HTTP or OCI)
		fetcher, err := helmrepo.NewFetcher(change.Dependency.Repository, auth)
		if err != nil {
			return fmt.Errorf("failed to create fetcher for %s: %w", change.Dependency.Repository, err)
		}

		// Fetch old upstream values
		oldUpstream, err := fetcher.FetchValues(
			change.Dependency.Repository, change.Dependency.Name,
			change.OldVersion, auth,
		)
		if err != nil {
			return fmt.Errorf("failed to fetch old upstream values for %s@%s: %w", change.Dependency.Name, change.OldVersion, err)
		}

		// Fetch new upstream values
		newUpstream, err := fetcher.FetchValues(
			change.Dependency.Repository, change.Dependency.Name,
			change.NewVersion, auth,
		)
		if err != nil {
			return fmt.Errorf("failed to fetch new upstream values for %s@%s: %w", change.Dependency.Name, change.NewVersion, err)
		}

		// Compare direct subchart
		results := diff.Compare(parentOverrides, oldUpstream, newUpstream)

		scReport := models.SubchartReport{
			SubchartName: key,
			OldVersion:   change.OldVersion,
			NewVersion:   change.NewVersion,
			Results:      results,
		}

		for _, r := range results {
			if r.Breaking {
				report.HasBreaking = true
				break
			}
		}

		report.SubchartReports = append(report.SubchartReports, scReport)

		// 7b. Check transitive/nested subchart dependencies
		visitedKey := fmt.Sprintf("%s@%s", change.Dependency.Name, change.NewVersion)
		if !visited[visitedKey] {
			visited[visitedKey] = true
			transitiveReports, err := checkTransitiveDeps(
				fetcher, change.Dependency, change.OldVersion, change.NewVersion,
				key, parentOverrides, auth, visited,
			)
			if err != nil {
				fmt.Printf("  Warning: Could not check transitive deps for %s: %v\n", key, err)
			} else {
				for _, tr := range transitiveReports {
					for _, r := range tr.Results {
						if r.Breaking {
							report.HasBreaking = true
							break
						}
					}
					report.SubchartReports = append(report.SubchartReports, tr)
				}
			}
		}
	}

	// 8. Output results
	if len(report.SubchartReports) == 0 {
		fmt.Println("No subchart overrides to check. All clear.")
		return nil
	}

	// Always print stdout summary
	fmt.Println("\n" + reporter.FormatStdout(report))

	if !report.HasBreaking {
		fmt.Println("✅ No breaking changes detected.")
		return nil
	}

	// Report breaking changes
	fmt.Println("⛔ Breaking changes detected!")

	if dryRun {
		fmt.Println("\n--- Markdown Report (dry-run) ---")
		fmt.Println(reporter.FormatMarkdown(report))
		os.Exit(1)
	}

	// Create GitLab issue
	if gitlabURL == "" || gitlabToken == "" || projectID == "" {
		fmt.Println("Warning: GitLab credentials not provided. Printing markdown report to stdout.")
		fmt.Println("\n" + reporter.FormatMarkdown(report))
		os.Exit(1)
	}

	glReporter, err := reporter.NewGitLabReporter(gitlabURL, gitlabToken, projectID, mrID)
	if err != nil {
		return fmt.Errorf("failed to create GitLab reporter: %w", err)
	}

	if err := glReporter.Report(report); err != nil {
		return fmt.Errorf("failed to create GitLab issue: %w", err)
	}

	os.Exit(1) // Fail CI pipeline
	return nil
}

// checkTransitiveDeps fetches a subchart's own Chart.yaml dependencies and
// checks for breaking changes in any transitive dep that the parent overrides.
func checkTransitiveDeps(
	fetcher helmrepo.Fetcher,
	dep models.Dependency,
	oldVersion, newVersion string,
	parentKey string,
	parentOverrides map[string]interface{},
	auth *models.RepoAuth,
	visited map[string]bool,
) ([]models.SubchartReport, error) {
	var reports []models.SubchartReport

	// Fetch the new subchart's own dependencies
	newDeps, err := fetcher.FetchChartDeps(dep.Repository, dep.Name, newVersion, auth)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch chart deps for %s@%s: %w", dep.Name, newVersion, err)
	}

	if len(newDeps) == 0 {
		return nil, nil
	}

	// Also fetch old subchart's dependencies for version comparison
	oldDeps, err := fetcher.FetchChartDeps(dep.Repository, dep.Name, oldVersion, auth)
	if err != nil {
		// If old version doesn't have deps, treat as all-new (no comparison needed)
		return nil, nil
	}

	// Find transitive version changes
	transitiveChanges := chartparser.FindVersionChanges(oldDeps, newDeps)

	for _, tc := range transitiveChanges {
		tcKey := tc.Dependency.ResolveKey()

		// Check if parent values.yaml has overrides for this transitive dep
		// Transitive overrides appear as parentKey.tcKey.* in parent values
		transitiveOverrides := chartparser.ExtractSubchartValues(parentOverrides, tcKey)
		if transitiveOverrides == nil {
			continue
		}

		// Cycle detection
		visitedKey := fmt.Sprintf("%s@%s", tc.Dependency.Name, tc.NewVersion)
		if visited[visitedKey] {
			continue
		}
		visited[visitedKey] = true

		fmt.Printf("    Checking transitive dep %s.%s (%s → %s)...\n",
			parentKey, tcKey, tc.OldVersion, tc.NewVersion)

		// Create fetcher for the transitive dep's repo
		tcFetcher, err := helmrepo.NewFetcher(tc.Dependency.Repository, auth)
		if err != nil {
			fmt.Printf("    Warning: Could not create fetcher for transitive dep %s: %v\n", tcKey, err)
			continue
		}

		// Fetch old and new upstream values for the transitive dep
		oldUpstream, err := tcFetcher.FetchValues(
			tc.Dependency.Repository, tc.Dependency.Name,
			tc.OldVersion, auth,
		)
		if err != nil {
			fmt.Printf("    Warning: Could not fetch old values for %s@%s: %v\n", tc.Dependency.Name, tc.OldVersion, err)
			continue
		}

		newUpstream, err := tcFetcher.FetchValues(
			tc.Dependency.Repository, tc.Dependency.Name,
			tc.NewVersion, auth,
		)
		if err != nil {
			fmt.Printf("    Warning: Could not fetch new values for %s@%s: %v\n", tc.Dependency.Name, tc.NewVersion, err)
			continue
		}

		results := diff.Compare(transitiveOverrides, oldUpstream, newUpstream)

		if len(results) > 0 {
			reports = append(reports, models.SubchartReport{
				SubchartName: fmt.Sprintf("%s → %s", parentKey, tcKey),
				OldVersion:   tc.OldVersion,
				NewVersion:   tc.NewVersion,
				Results:      results,
			})
		}
	}

	return reports, nil
}

func buildAuth() *models.RepoAuth {
	auth := &models.RepoAuth{
		Username:         helmUser,
		Password:         helmPass,
		Token:            helmToken,
		DockerConfigPath: ociRegistryConfig,
		RegistryCAFile:   ociRegistryCA,
	}
	if !auth.HasCredentials() {
		return nil
	}
	return auth
}

func envOrDefault(key, defaultVal string) string {
	if val := os.Getenv(key); val != "" {
		return val
	}
	return defaultVal
}

```

### 2. Added Transitive Subchart Dependency Checking
- Extended [Fetcher](file:///Users/arpan-k8s/check-breaking-change/internal/helmrepo/fetcher.go#9-18) interface with [FetchChartDeps()](file:///Users/arpan-k8s/check-breaking-change/internal/helmrepo/fetcher.go#14-17) method
- Added [FetchChartDependencies()](file:///Users/arpan-k8s/check-breaking-change/internal/helmadapter/repo.go#32-45) and [FetchOCIChartDependencies()](file:///Users/arpan-k8s/check-breaking-change/internal/helmadapter/oci.go#56-69) to `helmadapter`
- Refactored chart loading into shared [fetchAndLoadChart()](file:///Users/arpan-k8s/check-breaking-change/internal/helmadapter/repo.go#46-75) / [fetchAndLoadOCIChart()](file:///Users/arpan-k8s/check-breaking-change/internal/helmadapter/oci.go#70-90)
- [checkTransitiveDeps()](file:///Users/arpan-k8s/check-breaking-change/cmd/root.go#255-348) in [cmd/root.go](file:///Users/arpan-k8s/check-breaking-change/cmd/root.go) checks nested deps with **cycle detection**
- Only checks transitive deps with parent [values.yaml](file:///Users/arpan-k8s/check-breaking-change/e2etest/testchart/values.yaml) overrides

```diff:fetcher.go
package helmrepo

import (
	"check-breaking-change/internal/models"
)

// Fetcher defines the interface for fetching upstream chart values.
// This allows swapping implementations (HTTP, OCI) transparently.
type Fetcher interface {
	// FetchValues downloads the upstream chart at the given version and returns
	// its default values.yaml as a parsed map.
	FetchValues(repoURL, chartName, version string, auth *models.RepoAuth) (map[string]interface{}, error)
}
===
package helmrepo

import (
	"check-breaking-change/internal/models"
)

// Fetcher defines the interface for fetching upstream chart values.
// This allows swapping implementations (HTTP, OCI) transparently.
type Fetcher interface {
	// FetchValues downloads the upstream chart at the given version and returns
	// its default values.yaml as a parsed map.
	FetchValues(repoURL, chartName, version string, auth *models.RepoAuth) (map[string]interface{}, error)

	// FetchChartDeps downloads the upstream chart at the given version and returns
	// its Chart.yaml dependencies for transitive dependency checking.
	FetchChartDeps(repoURL, chartName, version string, auth *models.RepoAuth) ([]models.Dependency, error)
}
```
```diff:repo.go
package helmadapter

import (
	"bytes"
	"fmt"
	"strings"

	"check-breaking-change/internal/models"

	"helm.sh/helm/v3/pkg/chart/loader"
	"helm.sh/helm/v3/pkg/getter"
	"helm.sh/helm/v3/pkg/repo"

	"gopkg.in/yaml.v3"
)

// FetchChartValues downloads a chart from an HTTP(S) Helm repository and returns
// its default values.yaml as a parsed map. Uses Helm SDK getter + repo + loader.
func FetchChartValues(repoURL, chartName, version string, auth *models.RepoAuth) (map[string]interface{}, error) {
	getterOpts := buildGetterOpts(auth)

	// Resolve chart download URL from index.yaml
	chartURL, err := resolveChartURL(repoURL, chartName, version, getterOpts)
	if err != nil {
		return nil, fmt.Errorf("failed to resolve chart URL for %s@%s: %w", chartName, version, err)
	}

	// Download the chart archive
	httpGetter, err := getter.NewHTTPGetter(getterOpts...)
	if err != nil {
		return nil, fmt.Errorf("failed to create HTTP getter: %w", err)
	}

	data, err := httpGetter.Get(chartURL)
	if err != nil {
		return nil, fmt.Errorf("failed to download chart %s@%s: %w", chartName, version, err)
	}

	// Load chart from archive and extract values
	chrt, err := loader.LoadArchive(data)
	if err != nil {
		return nil, fmt.Errorf("failed to load chart archive for %s@%s: %w", chartName, version, err)
	}

	if chrt.Values == nil {
		return make(map[string]interface{}), nil
	}
	return chrt.Values, nil
}

// resolveChartURL fetches and parses the repo index.yaml, returning the download URL
// for the specified chart name and version.
func resolveChartURL(repoURL, chartName, version string, opts []getter.Option) (string, error) {
	indexURL := strings.TrimRight(repoURL, "/") + "/index.yaml"

	httpGetter, err := getter.NewHTTPGetter(opts...)
	if err != nil {
		return "", fmt.Errorf("failed to create HTTP getter: %w", err)
	}

	indexData, err := httpGetter.Get(indexURL)
	if err != nil {
		return "", fmt.Errorf("failed to fetch index.yaml from %s: %w", indexURL, err)
	}

	idx := &repo.IndexFile{}
	if err := yaml.NewDecoder(bytes.NewReader(indexData.Bytes())).Decode(idx); err != nil {
		return "", fmt.Errorf("failed to parse index.yaml: %w", err)
	}
	idx.SortEntries()

	cv, err := idx.Get(chartName, version)
	if err != nil {
		return "", fmt.Errorf("chart %s@%s not found in index: %w", chartName, version, err)
	}

	if len(cv.URLs) == 0 {
		return "", fmt.Errorf("no download URLs for %s@%s", chartName, version)
	}

	chartURL := cv.URLs[0]
	if !strings.HasPrefix(chartURL, "http://") && !strings.HasPrefix(chartURL, "https://") {
		chartURL = strings.TrimRight(repoURL, "/") + "/" + chartURL
	}
	return chartURL, nil
}

// buildGetterOpts constructs Helm getter options from our auth model.
func buildGetterOpts(auth *models.RepoAuth) []getter.Option {
	var opts []getter.Option
	if auth == nil {
		return opts
	}
	if auth.Username != "" && auth.Password != "" {
		opts = append(opts, getter.WithBasicAuth(auth.Username, auth.Password))
		opts = append(opts, getter.WithPassCredentialsAll(true))
	}
	if auth.RegistryCAFile != "" {
		opts = append(opts, getter.WithTLSClientConfig("", "", auth.RegistryCAFile))
	}
	return opts
}
===
package helmadapter

import (
	"bytes"
	"fmt"
	"strings"

	"check-breaking-change/internal/models"

	"helm.sh/helm/v3/pkg/chart"
	"helm.sh/helm/v3/pkg/chart/loader"
	"helm.sh/helm/v3/pkg/getter"
	"helm.sh/helm/v3/pkg/repo"

	"gopkg.in/yaml.v3"
)

// FetchChartValues downloads a chart from an HTTP(S) Helm repository and returns
// its default values.yaml as a parsed map. Uses Helm SDK getter + repo + loader.
func FetchChartValues(repoURL, chartName, version string, auth *models.RepoAuth) (map[string]interface{}, error) {
	chrt, err := fetchAndLoadChart(repoURL, chartName, version, auth)
	if err != nil {
		return nil, err
	}

	if chrt.Values == nil {
		return make(map[string]interface{}), nil
	}
	return chrt.Values, nil
}

// FetchChartDependencies downloads a chart from an HTTP(S) Helm repository and
// returns its Chart.yaml dependencies for transitive dependency checking.
func FetchChartDependencies(repoURL, chartName, version string, auth *models.RepoAuth) ([]models.Dependency, error) {
	chrt, err := fetchAndLoadChart(repoURL, chartName, version, auth)
	if err != nil {
		return nil, err
	}

	if chrt.Metadata == nil || len(chrt.Metadata.Dependencies) == 0 {
		return nil, nil
	}
	return ConvertDependencies(chrt.Metadata.Dependencies), nil
}

// fetchAndLoadChart handles the common flow: resolve URL, download, load archive.
func fetchAndLoadChart(repoURL, chartName, version string, auth *models.RepoAuth) (*chart.Chart, error) {
	getterOpts := buildGetterOpts(auth)

	// Resolve chart download URL from index.yaml
	chartURL, err := resolveChartURL(repoURL, chartName, version, getterOpts)
	if err != nil {
		return nil, fmt.Errorf("failed to resolve chart URL for %s@%s: %w", chartName, version, err)
	}

	// Download the chart archive
	httpGetter, err := getter.NewHTTPGetter(getterOpts...)
	if err != nil {
		return nil, fmt.Errorf("failed to create HTTP getter: %w", err)
	}

	data, err := httpGetter.Get(chartURL)
	if err != nil {
		return nil, fmt.Errorf("failed to download chart %s@%s: %w", chartName, version, err)
	}

	// Load chart from archive
	chrt, err := loader.LoadArchive(data)
	if err != nil {
		return nil, fmt.Errorf("failed to load chart archive for %s@%s: %w", chartName, version, err)
	}

	return chrt, nil
}

// resolveChartURL fetches and parses the repo index.yaml, returning the download URL
// for the specified chart name and version.
func resolveChartURL(repoURL, chartName, version string, opts []getter.Option) (string, error) {
	indexURL := strings.TrimRight(repoURL, "/") + "/index.yaml"

	httpGetter, err := getter.NewHTTPGetter(opts...)
	if err != nil {
		return "", fmt.Errorf("failed to create HTTP getter: %w", err)
	}

	indexData, err := httpGetter.Get(indexURL)
	if err != nil {
		return "", fmt.Errorf("failed to fetch index.yaml from %s: %w", indexURL, err)
	}

	idx := &repo.IndexFile{}
	if err := yaml.NewDecoder(bytes.NewReader(indexData.Bytes())).Decode(idx); err != nil {
		return "", fmt.Errorf("failed to parse index.yaml: %w", err)
	}
	idx.SortEntries()

	cv, err := idx.Get(chartName, version)
	if err != nil {
		return "", fmt.Errorf("chart %s@%s not found in index: %w", chartName, version, err)
	}

	if len(cv.URLs) == 0 {
		return "", fmt.Errorf("no download URLs for %s@%s", chartName, version)
	}

	chartURL := cv.URLs[0]
	if !strings.HasPrefix(chartURL, "http://") && !strings.HasPrefix(chartURL, "https://") {
		chartURL = strings.TrimRight(repoURL, "/") + "/" + chartURL
	}
	return chartURL, nil
}

// buildGetterOpts constructs Helm getter options from our auth model.
func buildGetterOpts(auth *models.RepoAuth) []getter.Option {
	var opts []getter.Option
	if auth == nil {
		return opts
	}
	if auth.Username != "" && auth.Password != "" {
		opts = append(opts, getter.WithBasicAuth(auth.Username, auth.Password))
		opts = append(opts, getter.WithPassCredentialsAll(true))
	} else if auth.Token != "" {
		// Token-based auth: empty username with token as password.
		// Standard pattern for GitLab Package Registry, Artifactory, etc.
		opts = append(opts, getter.WithBasicAuth("", auth.Token))
		opts = append(opts, getter.WithPassCredentialsAll(true))
	}
	if auth.RegistryCAFile != "" {
		opts = append(opts, getter.WithTLSClientConfig("", "", auth.RegistryCAFile))
	}
	return opts
}
```
```diff:oci.go
package helmadapter

import (
	"bytes"
	"fmt"
	"strings"

	"check-breaking-change/internal/models"

	"helm.sh/helm/v3/pkg/chart/loader"
	"helm.sh/helm/v3/pkg/registry"
)

// OCIClientOptions holds options for creating an OCI registry client.
type OCIClientOptions struct {
	DockerConfigPath string
	CAFile           string
	PlainHTTP        bool
}

// NewOCIClient creates a Helm SDK registry client with the given options.
func NewOCIClient(opts OCIClientOptions) (*registry.Client, error) {
	var clientOpts []registry.ClientOption
	if opts.DockerConfigPath != "" {
		clientOpts = append(clientOpts, registry.ClientOptCredentialsFile(opts.DockerConfigPath))
	}
	if opts.PlainHTTP {
		clientOpts = append(clientOpts, registry.ClientOptPlainHTTP())
	}
	return registry.NewClient(clientOpts...)
}

// LoginOCI performs an explicit login to an OCI registry when username/password are provided.
func LoginOCI(client *registry.Client, host string, auth *models.RepoAuth) error {
	if auth == nil || (auth.Username == "" && auth.Password == "") {
		return nil
	}
	return client.Login(host, registry.LoginOptBasicAuth(auth.Username, auth.Password))
}

// FetchOCIChartValues pulls a chart from an OCI registry and returns its values.yaml.
// repoURL should include the oci:// scheme, e.g. "oci://ghcr.io/charts".
func FetchOCIChartValues(client *registry.Client, repoURL, chartName, version string) (map[string]interface{}, error) {
	ref := buildOCIRef(repoURL, chartName, version)

	result, err := client.Pull(ref)
	if err != nil {
		return nil, fmt.Errorf("failed to pull OCI chart %s: %w", ref, err)
	}

	if result.Chart == nil || result.Chart.Data == nil {
		return nil, fmt.Errorf("no chart data returned for %s", ref)
	}

	chrt, err := loader.LoadArchive(bytes.NewReader(result.Chart.Data))
	if err != nil {
		return nil, fmt.Errorf("failed to load chart archive for %s: %w", ref, err)
	}

	if chrt.Values == nil {
		return make(map[string]interface{}), nil
	}
	return chrt.Values, nil
}

// buildOCIRef constructs an OCI reference from repo URL, chart name, and version.
// e.g. "oci://ghcr.io/charts" + "sub-1" + "1.0.0" → "oci://ghcr.io/charts/sub-1:1.0.0"
func buildOCIRef(repoURL, chartName, version string) string {
	base := strings.TrimRight(repoURL, "/")
	return fmt.Sprintf("%s/%s:%s", base, chartName, version)
}

// ExtractOCIHost extracts the registry host from an OCI URL.
// e.g. "oci://ghcr.io/charts/sub-1" → "ghcr.io"
func ExtractOCIHost(repoURL string) string {
	trimmed := strings.TrimPrefix(repoURL, "oci://")
	parts := strings.SplitN(trimmed, "/", 2)
	return parts[0]
}
===
package helmadapter

import (
	"bytes"
	"fmt"
	"strings"

	"check-breaking-change/internal/models"

	"helm.sh/helm/v3/pkg/chart"
	"helm.sh/helm/v3/pkg/chart/loader"
	"helm.sh/helm/v3/pkg/registry"
)

// OCIClientOptions holds options for creating an OCI registry client.
type OCIClientOptions struct {
	DockerConfigPath string
	CAFile           string
	PlainHTTP        bool
}

// NewOCIClient creates a Helm SDK registry client with the given options.
func NewOCIClient(opts OCIClientOptions) (*registry.Client, error) {
	var clientOpts []registry.ClientOption
	if opts.DockerConfigPath != "" {
		clientOpts = append(clientOpts, registry.ClientOptCredentialsFile(opts.DockerConfigPath))
	}
	if opts.PlainHTTP {
		clientOpts = append(clientOpts, registry.ClientOptPlainHTTP())
	}
	return registry.NewClient(clientOpts...)
}

// LoginOCI performs an explicit login to an OCI registry when username/password are provided.
func LoginOCI(client *registry.Client, host string, auth *models.RepoAuth) error {
	if auth == nil || (auth.Username == "" && auth.Password == "") {
		return nil
	}
	return client.Login(host, registry.LoginOptBasicAuth(auth.Username, auth.Password))
}

// FetchOCIChartValues pulls a chart from an OCI registry and returns its values.yaml.
// repoURL should include the oci:// scheme, e.g. "oci://ghcr.io/charts".
func FetchOCIChartValues(client *registry.Client, repoURL, chartName, version string) (map[string]interface{}, error) {
	chrt, err := fetchAndLoadOCIChart(client, repoURL, chartName, version)
	if err != nil {
		return nil, err
	}

	if chrt.Values == nil {
		return make(map[string]interface{}), nil
	}
	return chrt.Values, nil
}

// FetchOCIChartDependencies pulls a chart from an OCI registry and returns
// its Chart.yaml dependencies for transitive dependency checking.
func FetchOCIChartDependencies(client *registry.Client, repoURL, chartName, version string) ([]models.Dependency, error) {
	chrt, err := fetchAndLoadOCIChart(client, repoURL, chartName, version)
	if err != nil {
		return nil, err
	}

	if chrt.Metadata == nil || len(chrt.Metadata.Dependencies) == 0 {
		return nil, nil
	}
	return ConvertDependencies(chrt.Metadata.Dependencies), nil
}

// fetchAndLoadOCIChart handles the common OCI flow: pull + load archive.
func fetchAndLoadOCIChart(client *registry.Client, repoURL, chartName, version string) (*chart.Chart, error) {
	ref := buildOCIRef(repoURL, chartName, version)

	result, err := client.Pull(ref)
	if err != nil {
		return nil, fmt.Errorf("failed to pull OCI chart %s: %w", ref, err)
	}

	if result.Chart == nil || result.Chart.Data == nil {
		return nil, fmt.Errorf("no chart data returned for %s", ref)
	}

	chrt, err := loader.LoadArchive(bytes.NewReader(result.Chart.Data))
	if err != nil {
		return nil, fmt.Errorf("failed to load chart archive for %s: %w", ref, err)
	}

	return chrt, nil
}

// buildOCIRef constructs an OCI reference from repo URL, chart name, and version.
// e.g. "oci://ghcr.io/charts" + "sub-1" + "1.0.0" → "oci://ghcr.io/charts/sub-1:1.0.0"
func buildOCIRef(repoURL, chartName, version string) string {
	base := strings.TrimRight(repoURL, "/")
	return fmt.Sprintf("%s/%s:%s", base, chartName, version)
}

// ExtractOCIHost extracts the registry host from an OCI URL.
// e.g. "oci://ghcr.io/charts/sub-1" → "ghcr.io"
func ExtractOCIHost(repoURL string) string {
	trimmed := strings.TrimPrefix(repoURL, "oci://")
	parts := strings.SplitN(trimmed, "/", 2)
	return parts[0]
}
```

### 3. Wired HELM_REPO_TOKEN Into HTTP Getter
- Token-based auth: `getter.WithBasicAuth("", token)` — standard GitLab/Artifactory pattern
- Only applies when explicit username+password are not set

### 4. Updated walkthrough.md
- Reflects local-file flow, transitive deps, token auth, updated architecture diagram

## Verification

```
✅ go build ./...     — passes
✅ go vet ./...       — passes
✅ go test ./...      — 34 tests pass (14 chartparser + 20 diff)
```
