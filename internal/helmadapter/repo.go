package helmadapter

import (
	"fmt"
	"os"
	"strings"

	"check-breaking-change/internal/models"

	"helm.sh/helm/v3/pkg/chart"
	"helm.sh/helm/v3/pkg/chart/loader"
	"helm.sh/helm/v3/pkg/getter"
	"helm.sh/helm/v3/pkg/repo"
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

	// Write to temp file and use Helm's LoadIndexFile for proper YAML parsing.
	// This avoids nil pointer issues that occur with gopkg.in/yaml.v3.
	tmpFile, err := os.CreateTemp("", "helm-index-*.yaml")
	if err != nil {
		return "", fmt.Errorf("failed to create temp file for index: %w", err)
	}
	defer os.Remove(tmpFile.Name())
	defer tmpFile.Close()

	if _, err := tmpFile.Write(indexData.Bytes()); err != nil {
		return "", fmt.Errorf("failed to write index to temp file: %w", err)
	}
	tmpFile.Close() // Close before reading

	idx, err := repo.LoadIndexFile(tmpFile.Name())
	if err != nil {
		return "", fmt.Errorf("failed to parse index.yaml: %w", err)
	}

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
