package diff

import (
	"fmt"
	"reflect"

	"check-breaking-change/internal/models"
)

// Compare walks the parent overrides and checks each key path against the old
// and new subchart source values. It returns a list of DiffResults.
//
// parentOverrides: the subtree from parent values.yaml for this subchart
// oldSource:       the subchart source repo's values.yaml at the old version
// newSource:       the subchart source repo's values.yaml at the new version
func Compare(parentOverrides, oldSource, newSource map[string]interface{}) []models.DiffResult {
	var results []models.DiffResult
	compareMaps(parentOverrides, oldSource, newSource, "", &results)
	return results
}

// compareMaps recursively compares maps, only examining keys that exist in parentOverrides.
func compareMaps(parentOverrides, oldSource, newSource map[string]interface{}, prefix string, results *[]models.DiffResult) {
	for key, parentVal := range parentOverrides {
		fullKey := joinKeyPath(prefix, key)

		oldVal, oldExists := oldSource[key]
		newVal, newExists := newSource[key]

		// Key removed in new subchart source but parent overrides it → BREAKING
		if oldExists && !newExists {
			*results = append(*results, models.DiffResult{
				KeyPath:  fullKey,
				Type:     models.ChangeKeyRemoved,
				Breaking: true,
				OldValue: oldVal,
				NewValue: nil,
				Detail:   fmt.Sprintf("Key %q removed in subchart source but overridden in parent values", fullKey),
			})
			continue
		}

		// Key doesn't exist in old subchart source either (new override with no subchart backing) → skip
		if !oldExists && !newExists {
			continue
		}

		// Key added in new subchart source (didn't exist in old) → informational
		if !oldExists && newExists {
			// Still check parent-vs-subchart-source type compatibility for newly added keys
			checkParentTypeCompat(fullKey, parentVal, newVal, results)
			*results = append(*results, models.DiffResult{
				KeyPath:  fullKey,
				Type:     models.ChangeKeyAdded,
				Breaking: false,
				OldValue: nil,
				NewValue: newVal,
				Detail:   fmt.Sprintf("Key %q added in subchart source", fullKey),
			})
			continue
		}

		// Both exist — check for structural changes using three-type classification
		oldType := structuralType(oldVal)
		newType := structuralType(newVal)

		if oldType != newType {
			// Structural change between subchart source versions: scalar↔map, scalar↔slice, map↔slice
			*results = append(*results, models.DiffResult{
				KeyPath:  fullKey,
				Type:     models.ChangeStructural,
				Breaking: true,
				OldValue: oldVal,
				NewValue: newVal,
				Detail:   fmt.Sprintf("Structural change at %q: %s → %s", fullKey, typeLabel(oldVal), typeLabel(newVal)),
			})
			continue
		}

		// Same structural type in old and new subchart source — also check parent compatibility
		checkParentTypeCompat(fullKey, parentVal, newVal, results)

		if oldType == "map" {
			// Both are maps — recurse
			oldMap := toStringMap(oldVal)
			newMap := toStringMap(newVal)
			parentMap, parentIsMap := parentVal.(map[string]interface{})
			if parentIsMap {
				compareMaps(parentMap, oldMap, newMap, fullKey, results)
			}
			// Note: if parent is not a map but subchart source is, checkParentTypeCompat above already flags it.

			// Check for keys removed in new subchart source map that parent overrides
			if parentIsMap {
				for oldKey := range oldMap {
					if _, inNew := newMap[oldKey]; !inNew {
						if _, parentHas := parentMap[oldKey]; parentHas {
							childKey := joinKeyPath(fullKey, oldKey)
							*results = append(*results, models.DiffResult{
								KeyPath:  childKey,
								Type:     models.ChangeKeyRemoved,
								Breaking: true,
								OldValue: oldMap[oldKey],
								NewValue: nil,
								Detail:   fmt.Sprintf("Key %q removed in subchart source but overridden in parent values", childKey),
							})
						}
					}
				}
			}

			// Check for subchart source keys the parent doesn't override (missing overrides).
			// When the parent overrides a block, any new subchart source keys in that
			// block that the parent doesn't set should be flagged — the user
			// may need to explicitly configure these values.
			if parentIsMap {
				checkMissingOverrides(parentMap, oldMap, newMap, fullKey, results)
			}
			continue
		}

		// Both are slices or both are scalars — check for value changes
		if !reflect.DeepEqual(oldVal, newVal) {
			if IsSafeConversion(oldVal, newVal) {
				*results = append(*results, models.DiffResult{
					KeyPath:  fullKey,
					Type:     models.ChangeSafeTypeConv,
					Breaking: false,
					OldValue: oldVal,
					NewValue: newVal,
					Detail:   fmt.Sprintf("Safe type conversion at %q: %v (%T) → %v (%T)", fullKey, oldVal, oldVal, newVal, newVal),
				})
			} else {
				*results = append(*results, models.DiffResult{
					KeyPath:  fullKey,
					Type:     models.ChangeValueOnly,
					Breaking: false,
					OldValue: oldVal,
					NewValue: newVal,
					Detail:   fmt.Sprintf("Value change at %q: %v → %v", fullKey, oldVal, newVal),
				})
			}
		}
	}
}

// checkParentTypeCompat checks if the parent override's structural type matches
// the subchart source value's structural type. If the parent overrides a key as a scalar
// but the subchart source expects a slice (or vice versa), this is a type mismatch that will
// cause broken behavior in the deployed chart.
func checkParentTypeCompat(fullKey string, parentVal, sourceVal interface{}, results *[]models.DiffResult) {
	parentType := structuralType(parentVal)
	sourceType := structuralType(sourceVal)

	// nil subchart source means nothing to compare against
	if sourceType == "nil" || parentType == "nil" {
		return
	}

	if parentType != sourceType {
		*results = append(*results, models.DiffResult{
			KeyPath:  fullKey,
			Type:     models.ChangeParentTypeMismatch,
			Breaking: true,
			OldValue: parentVal,
			NewValue: sourceVal,
			Detail:   fmt.Sprintf("Parent overrides %q as %s but subchart source expects %s", fullKey, parentType, sourceType),
		})
	}
}

// isMap checks if a value is a map type.
func isMap(v interface{}) bool {
	if v == nil {
		return false
	}
	rt := reflect.TypeOf(v)
	return rt.Kind() == reflect.Map
}

// isSlice checks if a value is a slice/array type.
func isSlice(v interface{}) bool {
	if v == nil {
		return false
	}
	rt := reflect.TypeOf(v)
	return rt.Kind() == reflect.Slice || rt.Kind() == reflect.Array
}

// structuralType returns a classification label for the structural type of a value.
// Returns "map", "slice", "scalar", or "nil".
func structuralType(v interface{}) string {
	if v == nil {
		return "nil"
	}
	if isMap(v) {
		return "map"
	}
	if isSlice(v) {
		return "slice"
	}
	return "scalar"
}

// toStringMap converts an interface{} to map[string]interface{}, returning empty map if not possible.
func toStringMap(v interface{}) map[string]interface{} {
	if m, ok := v.(map[string]interface{}); ok {
		return m
	}
	return make(map[string]interface{})
}

// typeLabel returns a human-readable label for a value's type.
func typeLabel(v interface{}) string {
	if v == nil {
		return "nil"
	}
	if isMap(v) {
		return "map"
	}
	if isSlice(v) {
		return fmt.Sprintf("slice(%T)", v)
	}
	return fmt.Sprintf("scalar(%T)", v)
}

// checkMissingOverrides scans the new subchart source map for keys that the parent
// does not override. When the parent overrides a map block, any keys in the subchart
// source that the parent doesn't set are flagged — the user may need to explicitly
// configure these values for their deployment.
//
// Keys that are newly added in the new subchart source (not present in old version)
// AND missing from parent are flagged as Breaking (task required).
// Keys that existed in the old subchart source too but are missing from parent are
// flagged as informational (non-breaking).
//
// NOTE: This function only checks the current map level. It does NOT recurse into
// sub-maps where both parent and source have the key — that recursion is already
// handled by compareMaps calling checkMissingOverrides at each deeper level.
func checkMissingOverrides(parentMap, oldSourceMap, newSourceMap map[string]interface{}, prefix string, results *[]models.DiffResult) {
	for srcKey, srcVal := range newSourceMap {
		if _, parentHas := parentMap[srcKey]; !parentHas {
			childKey := joinKeyPath(prefix, srcKey)

			// Determine if this key is newly added in the new subchart source version
			_, existedInOld := oldSourceMap[srcKey]
			isNewlyAdded := !existedInOld

			if isNewlyAdded {
				*results = append(*results, models.DiffResult{
					KeyPath:  childKey,
					Type:     models.ChangeMissingOverride,
					Breaking: true,
					OldValue: nil,
					NewValue: srcVal,
					Detail:   fmt.Sprintf("Key %q newly added in subchart source but not overridden in parent values (parent overrides this block)", childKey),
				})
			} else {
				*results = append(*results, models.DiffResult{
					KeyPath:  childKey,
					Type:     models.ChangeMissingOverride,
					Breaking: false,
					OldValue: oldSourceMap[srcKey],
					NewValue: srcVal,
					Detail:   fmt.Sprintf("Key %q exists in subchart source but not overridden in parent values (parent overrides this block)", childKey),
				})
			}
		}
		// When both parent and source have this key, do NOT recurse here.
		// compareMaps already handles recursion and calls checkMissingOverrides
		// at each deeper level, avoiding duplicate results.
	}
}

// joinKeyPath joins key path segments with dots.
func joinKeyPath(prefix, key string) string {
	if prefix == "" {
		return key
	}
	return prefix + "." + key
}
