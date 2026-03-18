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
	rootCmd.PersistentFlags().StringVar(&gitlabURL, "gitlab-url", envOrDefault("GITLAB_URL", ""), "GitLab instance URL")
	rootCmd.PersistentFlags().StringVar(&gitlabToken, "gitlab-token", envOrDefault("GITLAB_TOKEN", ""), "GitLab API token")
	rootCmd.PersistentFlags().StringVar(&projectID, "project-id", envOrDefault("GITLAB_PROJECT_ID", ""), "GitLab project ID")
	rootCmd.PersistentFlags().StringVar(&mrID, "mr-id", envOrDefault("CI_MERGE_REQUEST_IID", ""), "GitLab MR ID (optional, for issue title)")
	rootCmd.PersistentFlags().BoolVar(&dryRun, "dry-run", false, "Print report to stdout without creating a GitLab issue")
	rootCmd.PersistentFlags().StringVar(&helmUser, "helm-repo-user", envOrDefault("HELM_REPO_USERNAME", ""), "Username for protected Helm repos")
	rootCmd.PersistentFlags().StringVar(&helmPass, "helm-repo-pass", envOrDefault("HELM_REPO_PASSWORD", ""), "Password for protected Helm repos")
	rootCmd.PersistentFlags().StringVar(&helmToken, "helm-repo-token", envOrDefault("HELM_REPO_TOKEN", ""), "Token for protected Helm repos")
	rootCmd.PersistentFlags().StringVar(&ociRegistryConfig, "oci-registry-config", envOrDefault("OCI_REGISTRY_CONFIG", ""), "Path to Docker config.json for OCI registry auth")
	rootCmd.PersistentFlags().StringVar(&ociRegistryCA, "oci-registry-ca", envOrDefault("OCI_REGISTRY_CA_FILE", ""), "CA certificate for private OCI registries")
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

		// Fetch old subchart source values
		oldSource, err := fetcher.FetchValues(
			change.Dependency.Repository, change.Dependency.Name,
			change.OldVersion, auth,
		)
		if err != nil {
			return fmt.Errorf("failed to fetch old subchart source values for %s@%s: %w", change.Dependency.Name, change.OldVersion, err)
		}

		// Fetch new subchart source values
		newSource, err := fetcher.FetchValues(
			change.Dependency.Repository, change.Dependency.Name,
			change.NewVersion, auth,
		)
		if err != nil {
			return fmt.Errorf("failed to fetch new subchart source values for %s@%s: %w", change.Dependency.Name, change.NewVersion, err)
		}

		// Compare direct subchart
		results := diff.Compare(parentOverrides, oldSource, newSource)

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

		// Fetch old and new subchart source values for the transitive dep
		oldSource, err := tcFetcher.FetchValues(
			tc.Dependency.Repository, tc.Dependency.Name,
			tc.OldVersion, auth,
		)
		if err != nil {
			fmt.Printf("    Warning: Could not fetch old values for %s@%s: %v\n", tc.Dependency.Name, tc.OldVersion, err)
			continue
		}

		newSource, err := tcFetcher.FetchValues(
			tc.Dependency.Repository, tc.Dependency.Name,
			tc.NewVersion, auth,
		)
		if err != nil {
			fmt.Printf("    Warning: Could not fetch new values for %s@%s: %v\n", tc.Dependency.Name, tc.NewVersion, err)
			continue
		}

		results := diff.Compare(transitiveOverrides, oldSource, newSource)

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
