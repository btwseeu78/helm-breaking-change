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

var validateCmd = &cobra.Command{
	Use:   "validate [chart-dir]",
	Short: "Validate the current values.yaml against the current Chart.yaml subchart dependencies",
	Long: `Reads Chart.yaml and values.yaml from the given directory, fetches each subchart's
upstream values.yaml at the currently pinned version, and checks whether the parent
chart's overrides are structurally compatible with the upstream defaults.

This is a self-validation mode: no override file is needed. It answers the question
"are my current values.yaml overrides valid for the subchart versions I depend on?"

Example:
  check-breaking-change validate .
  check-breaking-change validate /path/to/chart --dry-run`,
	Args: cobra.ExactArgs(1),
	RunE: runValidate,
}

func init() {
	rootCmd.AddCommand(validateCmd)
}

func runValidate(cmd *cobra.Command, args []string) error {
	chartDir := args[0]

	// 1. Read Chart.yaml
	chartPath := filepath.Join(chartDir, "Chart.yaml")
	chartData, err := os.ReadFile(chartPath)
	if err != nil {
		return fmt.Errorf("failed to read Chart.yaml at %s: %w", chartPath, err)
	}

	chartMeta, err := chartparser.ParseChartFile(chartData)
	if err != nil {
		return fmt.Errorf("failed to parse Chart.yaml: %w", err)
	}

	if len(chartMeta.Dependencies) == 0 {
		fmt.Println("No subchart dependencies found in Chart.yaml. Nothing to validate.")
		return nil
	}

	// 2. Read values.yaml
	valuesPath := filepath.Join(chartDir, "values.yaml")
	valuesData, err := os.ReadFile(valuesPath)
	if err != nil {
		return fmt.Errorf("failed to read values.yaml at %s: %w", valuesPath, err)
	}

	parentValues, err := chartparser.ParseValues(valuesData)
	if err != nil {
		return fmt.Errorf("failed to parse values.yaml: %w", err)
	}

	// 3. Build auth
	auth := buildAuth()

	// 4. Validate each active dependency
	report := models.Report{
		ChartName: chartMeta.Name,
	}

	fmt.Printf("Validating %d subchart dependency(ies) for chart %q...\n", len(chartMeta.Dependencies), chartMeta.Name)

	for _, dep := range chartMeta.Dependencies {
		key := dep.ResolveKey()

		// Check if the subchart is active (condition/tags)
		if !chartparser.IsActive(dep, parentValues) {
			fmt.Printf("  Skipping %s (inactive via condition/tags)\n", key)
			continue
		}

		// Extract parent overrides for this subchart
		parentOverrides := chartparser.ExtractSubchartValues(parentValues, key)
		if parentOverrides == nil {
			fmt.Printf("  Skipping %s (no parent overrides in values.yaml)\n", key)
			continue
		}

		fmt.Printf("  Validating %s @ %s...\n", key, dep.Version)

		// Create fetcher
		fetcher, err := helmrepo.NewFetcher(dep.Repository, auth)
		if err != nil {
			return fmt.Errorf("failed to create fetcher for %s: %w", dep.Repository, err)
		}

		// Fetch upstream source values at the current version
		sourceValues, err := fetcher.FetchValues(dep.Repository, dep.Name, dep.Version, auth)
		if err != nil {
			return fmt.Errorf("failed to fetch upstream values for %s@%s: %w", dep.Name, dep.Version, err)
		}

		// Validate parent overrides against upstream source
		results := diff.ValidateCurrentValues(parentOverrides, sourceValues)

		scReport := models.SubchartReport{
			SubchartName: key,
			OldVersion:   dep.Version,
			NewVersion:   dep.Version,
			Results:      results,
		}

		for _, r := range results {
			if r.Breaking {
				report.HasBreaking = true
				break
			}
		}

		report.SubchartReports = append(report.SubchartReports, scReport)

		// Also check transitive deps that the parent overrides
		transitiveReports, err := validateTransitiveDeps(
			fetcher, dep, key, parentOverrides, auth,
		)
		if err != nil {
			fmt.Printf("  Warning: Could not validate transitive deps for %s: %v\n", key, err)
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

	// 5. Output results
	if len(report.SubchartReports) == 0 {
		fmt.Println("No subchart overrides to validate. All clear.")
		return nil
	}

	fmt.Println("\n" + reporter.FormatStdout(report))

	if !report.HasBreaking {
		fmt.Println("✅ Current values.yaml is compatible with all subchart dependencies.")
		return nil
	}

	fmt.Println("⛔ Validation failed: values.yaml has incompatibilities with upstream subchart defaults.")
	if dryRun {
		fmt.Println("\n--- Markdown Report (dry-run) ---")
		fmt.Println(reporter.FormatMarkdown(report))
	}
	os.Exit(1)
	return nil
}

// validateTransitiveDeps fetches a subchart's own Chart.yaml dependencies and
// validates parent overrides against each transitive dep's upstream source values.
func validateTransitiveDeps(
	fetcher helmrepo.Fetcher,
	dep models.Dependency,
	parentKey string,
	parentOverrides map[string]interface{},
	auth *models.RepoAuth,
) ([]models.SubchartReport, error) {
	var reports []models.SubchartReport

	deps, err := fetcher.FetchChartDeps(dep.Repository, dep.Name, dep.Version, auth)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch chart deps for %s@%s: %w", dep.Name, dep.Version, err)
	}

	for _, td := range deps {
		tdKey := td.ResolveKey()

		// Check if parent values.yaml has overrides for this transitive dep
		transitiveOverrides := chartparser.ExtractSubchartValues(parentOverrides, tdKey)
		if transitiveOverrides == nil {
			continue
		}

		fmt.Printf("    Validating transitive dep %s.%s @ %s...\n", parentKey, tdKey, td.Version)

		tdFetcher, err := helmrepo.NewFetcher(td.Repository, auth)
		if err != nil {
			fmt.Printf("    Warning: Could not create fetcher for transitive dep %s: %v\n", tdKey, err)
			continue
		}

		sourceValues, err := tdFetcher.FetchValues(td.Repository, td.Name, td.Version, auth)
		if err != nil {
			fmt.Printf("    Warning: Could not fetch upstream values for %s@%s: %v\n", td.Name, td.Version, err)
			continue
		}

		results := diff.ValidateCurrentValues(transitiveOverrides, sourceValues)
		if len(results) > 0 {
			reports = append(reports, models.SubchartReport{
				SubchartName: fmt.Sprintf("%s → %s", parentKey, tdKey),
				OldVersion:   td.Version,
				NewVersion:   td.Version,
				Results:      results,
			})
		}
	}

	return reports, nil
}

