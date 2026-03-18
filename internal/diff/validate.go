package diff

import (
	"fmt"

	"check-breaking-change/internal/models"
)

// ValidateCurrentValues walks the parent overrides and checks each key path
// against the current subchart source values for structural compatibility.
// Unlike Compare (which diffs old-vs-new upstream), this validates the parent
// chart's values.yaml against the currently pinned subchart version.
//
// parentOverrides: the subtree from parent values.yaml for this subchart
// sourceValues:    the upstream subchart's values.yaml at the current version
func ValidateCurrentValues(parentOverrides, sourceValues map[string]interface{}) []models.DiffResult {
	var results []models.DiffResult
	validateMaps(parentOverrides, sourceValues, "", &results)
	return results
}

// validateMaps recursively walks parentOverrides and checks each key against
// the upstream source values.
func validateMaps(parentOverrides, sourceValues map[string]interface{}, prefix string, results *[]models.DiffResult) {
	for key, parentVal := range parentOverrides {
		fullKey := joinKeyPath(prefix, key)
		sourceVal, sourceExists := sourceValues[key]

		// Parent overrides a key that doesn't exist in upstream source → BREAKING
		// The parent chart is setting a value for a key the subchart doesn't
		// recognise, which means the override is dead/ineffective and the
		// deployment will not behave as intended.
		if !sourceExists {
			*results = append(*results, models.DiffResult{
				KeyPath:  fullKey,
				Type:     models.ChangeKeyOrphanOverride,
				Breaking: true,
				OldValue: parentVal,
				NewValue: nil,
				Detail:   fmt.Sprintf("Parent overrides %q but key does not exist in upstream subchart source — override is ineffective (breaking)", fullKey),
			})
			continue
		}

		// Both exist — check structural type compatibility
		parentType := structuralType(parentVal)
		sourceType := structuralType(sourceVal)

		if parentType == "nil" || sourceType == "nil" {
			continue
		}

		if parentType != sourceType {
			*results = append(*results, models.DiffResult{
				KeyPath:  fullKey,
				Type:     models.ChangeParentTypeMismatch,
				Breaking: true,
				OldValue: parentVal,
				NewValue: sourceVal,
				Detail:   fmt.Sprintf("Parent overrides %q as %s but upstream subchart expects %s", fullKey, parentType, sourceType),
			})
			continue
		}

		// If both are maps, recurse
		if parentType == "map" {
			parentMap := toStringMap(parentVal)
			sourceMap := toStringMap(sourceVal)
			validateMaps(parentMap, sourceMap, fullKey, results)
		}
		// Both scalar or both slice — structurally compatible, no issue
	}
}

