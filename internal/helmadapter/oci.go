package helmadapter

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
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
// If no DockerConfigPath is provided, it uses the default ~/.docker/config.json
// to leverage system-wide Docker credential helpers (gcloud, ecr, etc.).
func NewOCIClient(opts OCIClientOptions) (*registry.Client, error) {
	var clientOpts []registry.ClientOption

	// Use provided path or fall back to default Docker config
	configPath := opts.DockerConfigPath
	if configPath == "" {
		configPath = defaultDockerConfigPath()
	}
	if configPath != "" {
		clientOpts = append(clientOpts, registry.ClientOptCredentialsFile(configPath))
	}

	if opts.PlainHTTP {
		clientOpts = append(clientOpts, registry.ClientOptPlainHTTP())
	}
	return registry.NewClient(clientOpts...)
}

// defaultDockerConfigPath returns the default Docker config path if it exists.
func defaultDockerConfigPath() string {
	// Check DOCKER_CONFIG env var first
	if dockerConfig := os.Getenv("DOCKER_CONFIG"); dockerConfig != "" {
		configFile := filepath.Join(dockerConfig, "config.json")
		if _, err := os.Stat(configFile); err == nil {
			return configFile
		}
	}

	// Fall back to ~/.docker/config.json
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	configFile := filepath.Join(home, ".docker", "config.json")
	if _, err := os.Stat(configFile); err == nil {
		return configFile
	}
	return ""
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
