package helmrepo

import (
	"check-breaking-change/internal/models"
)

// Fetcher defines the interface for fetching subchart source values.
// This allows swapping implementations (HTTP, OCI) transparently.
type Fetcher interface {
	// FetchValues downloads the subchart from its source repo at the given version
	// and returns its default values.yaml as a parsed map.
	FetchValues(repoURL, chartName, version string, auth *models.RepoAuth) (map[string]interface{}, error)

	// FetchChartDeps downloads the subchart from its source repo at the given version
	// and returns its Chart.yaml dependencies for transitive dependency checking.
	FetchChartDeps(repoURL, chartName, version string, auth *models.RepoAuth) ([]models.Dependency, error)
}
