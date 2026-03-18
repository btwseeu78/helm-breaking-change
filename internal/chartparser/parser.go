package chartparser

import (
	"fmt"
	"strings"

	"check-breaking-change/internal/helmadapter"
	"check-breaking-change/internal/models"

	"gopkg.in/yaml.v3"
)

// ChartMeta represents the parsed Chart.yaml, focused on dependencies.
type ChartMeta struct {
	APIVersion   string
	Name         string
	Version      string
	Dependencies []models.Dependency
}

// ParseChartFile parses raw Chart.yaml bytes into ChartMeta using the Helm SDK.
func ParseChartFile(data []byte) (*ChartMeta, error) {
	meta, err := helmadapter.ParseChartMeta(data)
	if err != nil {
		return nil, err
	}
	return &ChartMeta{
		APIVersion:   meta.APIVersion,
		Name:         meta.Name,
		Version:      meta.Version,
		Dependencies: helmadapter.ConvertDependencies(meta.Dependencies),
	}, nil
}

// ParseValues parses raw values.yaml bytes into a generic map.
func ParseValues(data []byte) (map[string]interface{}, error) {
	var values map[string]interface{}
	if err := yaml.Unmarshal(data, &values); err != nil {
		return nil, fmt.Errorf("failed to parse values.yaml: %w", err)
	}
	if values == nil {
		values = make(map[string]interface{})
	}
	return values, nil
}

// FindVersionChanges compares old and new dependency lists and returns
// only the dependencies whose versions have changed.
func FindVersionChanges(oldDeps, newDeps []models.Dependency) []models.VersionChange {
	oldMap := make(map[string]models.Dependency)
	for _, d := range oldDeps {
		oldMap[d.ResolveKey()] = d
	}

	var changes []models.VersionChange
	for _, newDep := range newDeps {
		key := newDep.ResolveKey()
		oldDep, exists := oldMap[key]
		if !exists {
			// New subchart added — not a version change, skip
			continue
		}
		if oldDep.Version != newDep.Version {
			changes = append(changes, models.VersionChange{
				Dependency: newDep,
				OldVersion: oldDep.Version,
				NewVersion: newDep.Version,
			})
		}
	}

	return changes
}

// IsActive checks whether a dependency is active based on its condition
// and tags against the current values.
func IsActive(dep models.Dependency, values map[string]interface{}) bool {
	// If no condition and no tags, the dependency is always active
	if dep.Condition == "" && len(dep.Tags) == 0 {
		return true
	}

	// Check condition (e.g., "sub-1.enabled")
	if dep.Condition != "" {
		val := resolveKeyPath(values, dep.Condition)
		if val != nil {
			if boolVal, ok := val.(bool); ok {
				return boolVal
			}
			// If condition key exists but is not bool, treat as active
			return true
		}
		// Condition key not found in values; Helm defaults to true when
		// the condition key is absent, so treat as active
	}

	// Check tags — if any tag is enabled, the dependency is active
	if len(dep.Tags) > 0 {
		tagsVal := resolveKeyPath(values, "tags")
		if tagsMap, ok := tagsVal.(map[string]interface{}); ok {
			for _, tag := range dep.Tags {
				if v, exists := tagsMap[tag]; exists {
					if boolVal, ok := v.(bool); ok && boolVal {
						return true
					}
				}
			}
			// All tags are false or not found → inactive
			return false
		}
		// No tags section in values → Helm defaults tags to enabled
		return true
	}

	return true
}

// ExtractSubchartValues extracts the subtree for a given subchart key
// from the parent values.yaml.
// In the parent chart, overrides are nested under [alias/name].params.
// In the subchart source repo, the same params appear at the root level.
func ExtractSubchartValues(values map[string]interface{}, key string) map[string]interface{} {
	if val, ok := values[key]; ok {
		if subMap, ok := val.(map[string]interface{}); ok {
			return subMap
		}
	}
	return nil
}

// resolveKeyPath resolves a dot-separated key path in a nested map.
// e.g., "sub-1.enabled" in {"sub-1": {"enabled": true}} → true
func resolveKeyPath(values map[string]interface{}, keyPath string) interface{} {
	parts := strings.Split(keyPath, ".")
	var current interface{} = values
	for _, part := range parts {
		m, ok := current.(map[string]interface{})
		if !ok {
			return nil
		}
		current, ok = m[part]
		if !ok {
			return nil
		}
	}
	return current
}
