package helmrepo

import (
	"check-breaking-change/internal/helmadapter"
	"check-breaking-change/internal/models"
)

// HTTPFetcher fetches chart values from standard HTTPS Helm repositories
// using the Helm SDK for index resolution, download, and archive loading.
type HTTPFetcher struct{}

// NewHTTPFetcher creates a new HTTPFetcher.
func NewHTTPFetcher() *HTTPFetcher {
	return &HTTPFetcher{}
}

// FetchValues downloads the chart .tgz via the Helm SDK, extracts values.yaml,
// and returns parsed values.
func (f *HTTPFetcher) FetchValues(repoURL, chartName, version string, auth *models.RepoAuth) (map[string]interface{}, error) {
	return helmadapter.FetchChartValues(repoURL, chartName, version, auth)
}

// FetchChartDeps downloads the chart .tgz and returns its Chart.yaml dependencies.
func (f *HTTPFetcher) FetchChartDeps(repoURL, chartName, version string, auth *models.RepoAuth) ([]models.Dependency, error) {
	return helmadapter.FetchChartDependencies(repoURL, chartName, version, auth)
}
