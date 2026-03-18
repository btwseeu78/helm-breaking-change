package diff

import (
	"testing"

	"check-breaking-change/internal/models"
)

func TestValidateCurrentValues_Compatible(t *testing.T) {
	parent := map[string]interface{}{
		"image":   "registry.k8s.io:v1",
		"replica": 1,
	}
	source := map[string]interface{}{
		"image":   "registry.k8s.io:v5",
		"replica": 5,
	}

	results := ValidateCurrentValues(parent, source)

	for _, r := range results {
		if r.Breaking {
			t.Errorf("expected no breaking issues, but found: %+v", r)
		}
	}
}

func TestValidateCurrentValues_TypeMismatch_ScalarVsMap(t *testing.T) {
	parent := map[string]interface{}{
		"image": "registry.k8s.io:v1",
	}
	source := map[string]interface{}{
		"image": map[string]interface{}{
			"registry": "registry.k8s.io",
			"tag":      "v5",
		},
	}

	results := ValidateCurrentValues(parent, source)

	found := false
	for _, r := range results {
		if r.KeyPath == "image" && r.Breaking && r.Type == models.ChangeParentTypeMismatch {
			found = true
		}
	}
	if !found {
		t.Errorf("expected breaking parent-type-mismatch at 'image', got: %+v", results)
	}
}

func TestValidateCurrentValues_TypeMismatch_MapVsScalar(t *testing.T) {
	parent := map[string]interface{}{
		"image": map[string]interface{}{
			"registry": "registry.k8s.io",
			"tag":      "v1",
		},
	}
	source := map[string]interface{}{
		"image": "registry.k8s.io:v5",
	}

	results := ValidateCurrentValues(parent, source)

	found := false
	for _, r := range results {
		if r.KeyPath == "image" && r.Breaking && r.Type == models.ChangeParentTypeMismatch {
			found = true
		}
	}
	if !found {
		t.Errorf("expected breaking parent-type-mismatch at 'image', got: %+v", results)
	}
}

func TestValidateCurrentValues_OrphanOverride(t *testing.T) {
	parent := map[string]interface{}{
		"image":      "registry.k8s.io:v1",
		"missingKey": "some-value",
	}
	source := map[string]interface{}{
		"image": "registry.k8s.io:v5",
	}

	results := ValidateCurrentValues(parent, source)

	found := false
	for _, r := range results {
		if r.KeyPath == "missingKey" && r.Type == models.ChangeKeyOrphanOverride {
			found = true
			if r.Breaking {
				t.Error("orphan override should be non-breaking (informational)")
			}
		}
	}
	if !found {
		t.Errorf("expected orphan override at 'missingKey', got: %+v", results)
	}
}

func TestValidateCurrentValues_NestedCompatible(t *testing.T) {
	parent := map[string]interface{}{
		"config": map[string]interface{}{
			"host": "localhost",
			"port": 5432,
		},
	}
	source := map[string]interface{}{
		"config": map[string]interface{}{
			"host": "0.0.0.0",
			"port": 3306,
		},
	}

	results := ValidateCurrentValues(parent, source)

	for _, r := range results {
		if r.Breaking {
			t.Errorf("expected no breaking issues in nested compatible maps, but found: %+v", r)
		}
	}
}

func TestValidateCurrentValues_NestedTypeMismatch(t *testing.T) {
	parent := map[string]interface{}{
		"config": map[string]interface{}{
			"database": "postgres://localhost:5432",
		},
	}
	source := map[string]interface{}{
		"config": map[string]interface{}{
			"database": map[string]interface{}{
				"host": "localhost",
				"port": 5432,
			},
		},
	}

	results := ValidateCurrentValues(parent, source)

	found := false
	for _, r := range results {
		if r.KeyPath == "config.database" && r.Breaking && r.Type == models.ChangeParentTypeMismatch {
			found = true
		}
	}
	if !found {
		t.Errorf("expected breaking type mismatch at 'config.database', got: %+v", results)
	}
}

func TestValidateCurrentValues_NestedOrphanOverride(t *testing.T) {
	parent := map[string]interface{}{
		"config": map[string]interface{}{
			"host":      "localhost",
			"extraFlag": true,
		},
	}
	source := map[string]interface{}{
		"config": map[string]interface{}{
			"host": "0.0.0.0",
		},
	}

	results := ValidateCurrentValues(parent, source)

	found := false
	for _, r := range results {
		if r.KeyPath == "config.extraFlag" && r.Type == models.ChangeKeyOrphanOverride {
			found = true
		}
	}
	if !found {
		t.Errorf("expected orphan override at 'config.extraFlag', got: %+v", results)
	}
}

func TestValidateCurrentValues_EmptyParent(t *testing.T) {
	parent := map[string]interface{}{}
	source := map[string]interface{}{
		"image":   "registry.k8s.io:v5",
		"replica": 5,
	}

	results := ValidateCurrentValues(parent, source)

	if len(results) != 0 {
		t.Errorf("expected no results for empty parent overrides, got: %+v", results)
	}
}

func TestValidateCurrentValues_EmptySource(t *testing.T) {
	parent := map[string]interface{}{
		"image": "registry.k8s.io:v1",
	}
	source := map[string]interface{}{}

	results := ValidateCurrentValues(parent, source)

	found := false
	for _, r := range results {
		if r.KeyPath == "image" && r.Type == models.ChangeKeyOrphanOverride {
			found = true
		}
	}
	if !found {
		t.Errorf("expected orphan override at 'image', got: %+v", results)
	}
}

func TestValidateCurrentValues_MixedIssues(t *testing.T) {
	parent := map[string]interface{}{
		"image":      "registry.k8s.io:v1", // will be mismatch (scalar vs map)
		"replica":    3,                     // compatible (scalar vs scalar)
		"deprecated": "old-value",           // orphan (not in source)
	}
	source := map[string]interface{}{
		"image": map[string]interface{}{
			"registry": "registry.k8s.io",
			"tag":      "v5",
		},
		"replica": 5,
		"newKey":  "new-default",
	}

	results := ValidateCurrentValues(parent, source)

	hasTypeMismatch := false
	hasOrphan := false
	for _, r := range results {
		if r.KeyPath == "image" && r.Type == models.ChangeParentTypeMismatch && r.Breaking {
			hasTypeMismatch = true
		}
		if r.KeyPath == "deprecated" && r.Type == models.ChangeKeyOrphanOverride && !r.Breaking {
			hasOrphan = true
		}
	}

	if !hasTypeMismatch {
		t.Error("expected type mismatch for 'image'")
	}
	if !hasOrphan {
		t.Error("expected orphan override for 'deprecated'")
	}
}

