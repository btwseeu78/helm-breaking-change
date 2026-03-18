package helmrepo

import (
	"fmt"

	"check-breaking-change/internal/helmadapter"
	"check-breaking-change/internal/models"

	"helm.sh/helm/v3/pkg/registry"
)

// OCIFetcher fetches chart values from OCI-compatible registries
// using the Helm SDK registry client.
type OCIFetcher struct {
	client *registry.Client
}

// NewOCIFetcher creates a new OCIFetcher with the given auth configuration.
func NewOCIFetcher(auth *models.RepoAuth) (*OCIFetcher, error) {
	opts := helmadapter.OCIClientOptions{}
	if auth != nil {
		opts.DockerConfigPath = auth.DockerConfigPath
		opts.CAFile = auth.RegistryCAFile
	}

	client, err := helmadapter.NewOCIClient(opts)
	if err != nil {
		return nil, fmt.Errorf("failed to create OCI registry client: %w", err)
	}

	return &OCIFetcher{client: client}, nil
}

// loginIfNeeded performs OCI login when username/password credentials are available.
func (f *OCIFetcher) loginIfNeeded(repoURL string, auth *models.RepoAuth) error {
	if auth != nil && auth.Username != "" && auth.Password != "" {
		host := helmadapter.ExtractOCIHost(repoURL)
		if err := helmadapter.LoginOCI(f.client, host, auth); err != nil {
			return fmt.Errorf("OCI login failed for %s: %w", host, err)
		}
	}
	return nil
}

// FetchValues pulls the chart from the OCI registry and returns its values.yaml.
func (f *OCIFetcher) FetchValues(repoURL, chartName, version string, auth *models.RepoAuth) (map[string]interface{}, error) {
	if err := f.loginIfNeeded(repoURL, auth); err != nil {
		return nil, err
	}
	return helmadapter.FetchOCIChartValues(f.client, repoURL, chartName, version)
}

// FetchChartDeps pulls the chart from the OCI registry and returns its Chart.yaml dependencies.
func (f *OCIFetcher) FetchChartDeps(repoURL, chartName, version string, auth *models.RepoAuth) ([]models.Dependency, error) {
	if err := f.loginIfNeeded(repoURL, auth); err != nil {
		return nil, err
	}
	return helmadapter.FetchOCIChartDependencies(f.client, repoURL, chartName, version)
}
