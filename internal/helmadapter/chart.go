package helmadapter

import (
	"fmt"

	"check-breaking-change/internal/models"

	"helm.sh/helm/v3/pkg/chart"
	sigsyaml "sigs.k8s.io/yaml"
)

// ParseChartMeta parses raw Chart.yaml bytes using the Helm SDK's chart.Metadata type.
func ParseChartMeta(data []byte) (*chart.Metadata, error) {
	var meta chart.Metadata
	if err := sigsyaml.Unmarshal(data, &meta); err != nil {
		return nil, fmt.Errorf("failed to parse Chart.yaml: %w", err)
	}
	if err := meta.Validate(); err != nil {
		return nil, fmt.Errorf("invalid Chart.yaml: %w", err)
	}
	return &meta, nil
}

// ConvertDependencies converts Helm SDK chart.Dependency slice to our internal models.
func ConvertDependencies(deps []*chart.Dependency) []models.Dependency {
	result := make([]models.Dependency, 0, len(deps))
	for _, d := range deps {
		result = append(result, FromHelmDependency(d))
	}
	return result
}

// FromHelmDependency converts a single Helm SDK chart.Dependency to models.Dependency.
func FromHelmDependency(d *chart.Dependency) models.Dependency {
	return models.Dependency{
		Name:       d.Name,
		Alias:      d.Alias,
		Version:    d.Version,
		Repository: d.Repository,
		Condition:  d.Condition,
		Tags:       d.Tags,
	}
}
