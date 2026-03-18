package diff

import (
	"testing"

	"check-breaking-change/internal/models"
)

func TestCompare_ScalarToMap_Breaking(t *testing.T) {
	parent := map[string]interface{}{
		"image": "registry.k8s.io:v1",
	}
	oldSource := map[string]interface{}{
		"image": "registry.k8s.io:v1",
	}
	newSource := map[string]interface{}{
		"image": map[string]interface{}{
			"registry": "registry.k8s.io",
			"tag":      "v5",
		},
	}

	results := Compare(parent, oldSource, newSource)

	if len(results) == 0 {
		t.Fatal("expected at least one result")
	}

	found := false
	for _, r := range results {
		if r.KeyPath == "image" && r.Breaking && r.Type == models.ChangeStructural {
			found = true
		}
	}
	if !found {
		t.Errorf("expected breaking structural change at 'image', got: %+v", results)
	}
}

func TestCompare_MapToScalar_Breaking(t *testing.T) {
	parent := map[string]interface{}{
		"image": map[string]interface{}{
			"registry": "registry.k8s.io",
			"tag":      "v1",
		},
	}
	oldSource := map[string]interface{}{
		"image": map[string]interface{}{
			"registry": "registry.k8s.io",
			"tag":      "v1",
		},
	}
	newSource := map[string]interface{}{
		"image": "registry.k8s.io:v5",
	}

	results := Compare(parent, oldSource, newSource)

	found := false
	for _, r := range results {
		if r.KeyPath == "image" && r.Breaking && r.Type == models.ChangeStructural {
			found = true
		}
	}
	if !found {
		t.Errorf("expected breaking structural change at 'image', got: %+v", results)
	}
}

func TestCompare_KeyRemoved_Breaking(t *testing.T) {
	parent := map[string]interface{}{
		"image":   "registry.k8s.io:v1",
		"replica": 1,
	}
	oldSource := map[string]interface{}{
		"image":   "registry.k8s.io:v1",
		"replica": 1,
	}
	newSource := map[string]interface{}{
		"image": "registry.k8s.io:v5",
		// "replica" removed
	}

	results := Compare(parent, oldSource, newSource)

	found := false
	for _, r := range results {
		if r.KeyPath == "replica" && r.Breaking && r.Type == models.ChangeKeyRemoved {
			found = true
		}
	}
	if !found {
		t.Errorf("expected breaking key-removed at 'replica', got: %+v", results)
	}
}

func TestCompare_KeyAdded_NotBreaking(t *testing.T) {
	parent := map[string]interface{}{
		"image":  "registry.k8s.io:v1",
		"newKey": "some-value",
	}
	oldSource := map[string]interface{}{
		"image": "registry.k8s.io:v1",
	}
	newSource := map[string]interface{}{
		"image":  "registry.k8s.io:v5",
		"newKey": "default-value",
	}

	results := Compare(parent, oldSource, newSource)

	for _, r := range results {
		if r.KeyPath == "newKey" && r.Breaking {
			t.Errorf("expected new key to be non-breaking, got: %+v", r)
		}
	}
}

func TestCompare_ValueOnlyChange_NotBreaking(t *testing.T) {
	parent := map[string]interface{}{
		"image":   "registry.k8s.io:v1",
		"replica": 1,
	}
	oldSource := map[string]interface{}{
		"image":   "registry.k8s.io:v1",
		"replica": 1,
	}
	newSource := map[string]interface{}{
		"image":   "registry.k8s.io:v5",
		"replica": 5,
	}

	results := Compare(parent, oldSource, newSource)

	for _, r := range results {
		if r.Breaking {
			t.Errorf("expected no breaking changes, but found: %+v", r)
		}
	}
}

func TestCompare_SafeTypeConversion_NotBreaking(t *testing.T) {
	parent := map[string]interface{}{
		"port": "8080",
	}
	oldSource := map[string]interface{}{
		"port": "8080",
	}
	newSource := map[string]interface{}{
		"port": 8080,
	}

	results := Compare(parent, oldSource, newSource)

	for _, r := range results {
		if r.Breaking {
			t.Errorf("expected safe type conversion to be non-breaking, got: %+v", r)
		}
	}
}

func TestCompare_NestedStructuralChange_Breaking(t *testing.T) {
	parent := map[string]interface{}{
		"config": map[string]interface{}{
			"database": "postgres://localhost:5432",
		},
	}
	oldSource := map[string]interface{}{
		"config": map[string]interface{}{
			"database": "postgres://localhost:5432",
		},
	}
	newSource := map[string]interface{}{
		"config": map[string]interface{}{
			"database": map[string]interface{}{
				"host": "localhost",
				"port": 5432,
			},
		},
	}

	results := Compare(parent, oldSource, newSource)

	found := false
	for _, r := range results {
		if r.KeyPath == "config.database" && r.Breaking && r.Type == models.ChangeStructural {
			found = true
		}
	}
	if !found {
		t.Errorf("expected breaking structural change at 'config.database', got: %+v", results)
	}
}

func TestCompare_NestedKeyRemoved_Breaking(t *testing.T) {
	parent := map[string]interface{}{
		"config": map[string]interface{}{
			"host": "localhost",
			"port": 5432,
		},
	}
	oldSource := map[string]interface{}{
		"config": map[string]interface{}{
			"host": "localhost",
			"port": 5432,
		},
	}
	newSource := map[string]interface{}{
		"config": map[string]interface{}{
			"host": "localhost",
			// "port" removed
		},
	}

	results := Compare(parent, oldSource, newSource)

	found := false
	for _, r := range results {
		if r.KeyPath == "config.port" && r.Breaking && r.Type == models.ChangeKeyRemoved {
			found = true
		}
	}
	if !found {
		t.Errorf("expected breaking key-removed at 'config.port', got: %+v", results)
	}
}

func TestCompare_NoChanges_Empty(t *testing.T) {
	parent := map[string]interface{}{
		"image":   "registry.k8s.io:v1",
		"replica": 1,
	}
	oldSource := map[string]interface{}{
		"image":   "registry.k8s.io:v1",
		"replica": 1,
	}
	newSource := map[string]interface{}{
		"image":   "registry.k8s.io:v1",
		"replica": 1,
	}

	results := Compare(parent, oldSource, newSource)

	if len(results) != 0 {
		t.Errorf("expected no results for identical values, got: %+v", results)
	}
}

func TestCompare_FullScenario_Example1_NotBreaking(t *testing.T) {
	// Example 1 from AGENTS.md: value-only changes, no structural changes
	parent := map[string]interface{}{
		"image":   "registry.k8s.io:v1",
		"replica": 1,
	}
	oldSource := map[string]interface{}{
		"image":   "registry.k8s.io:v1",
		"replica": 1,
	}
	newSource := map[string]interface{}{
		"image":   "registry.k8s.io:v5",
		"replica": 5,
	}

	results := Compare(parent, oldSource, newSource)

	for _, r := range results {
		if r.Breaking {
			t.Errorf("example 1 should have no breaking changes, got: %+v", r)
		}
	}
}

func TestCompare_FullScenario_Example2_Breaking(t *testing.T) {
	// Example 2 from AGENTS.md: sub-1 has structural change on image
	parent := map[string]interface{}{
		"image":   "registry.k8s.io:v1",
		"replica": 1,
	}
	oldSource := map[string]interface{}{
		"image":   "registry.k8s.io:v1",
		"replica": 1,
	}
	newSource := map[string]interface{}{
		"image": map[string]interface{}{
			"registry": "registry.k8s.io",
			"tag":      "v5",
		},
		"replica": 5,
	}

	results := Compare(parent, oldSource, newSource)

	hasBreaking := false
	for _, r := range results {
		if r.Breaking {
			hasBreaking = true
		}
	}
	if !hasBreaking {
		t.Error("example 2 should have breaking changes")
	}
}

func TestCompare_ScalarToSlice_Breaking(t *testing.T) {
	parent := map[string]interface{}{
		"pullSecrets": "my-secret",
	}
	oldSource := map[string]interface{}{
		"pullSecrets": "default-secret",
	}
	newSource := map[string]interface{}{
		"pullSecrets": []interface{}{},
	}

	results := Compare(parent, oldSource, newSource)

	found := false
	for _, r := range results {
		if r.KeyPath == "pullSecrets" && r.Breaking && r.Type == models.ChangeStructural {
			found = true
		}
	}
	if !found {
		t.Errorf("expected breaking structural change (scalar→slice) at 'pullSecrets', got: %+v", results)
	}
}

func TestCompare_SliceToScalar_Breaking(t *testing.T) {
	parent := map[string]interface{}{
		"pullSecrets": []interface{}{"secret-a"},
	}
	oldSource := map[string]interface{}{
		"pullSecrets": []interface{}{"default"},
	}
	newSource := map[string]interface{}{
		"pullSecrets": "single-secret",
	}

	results := Compare(parent, oldSource, newSource)

	found := false
	for _, r := range results {
		if r.KeyPath == "pullSecrets" && r.Breaking && r.Type == models.ChangeStructural {
			found = true
		}
	}
	if !found {
		t.Errorf("expected breaking structural change (slice→scalar) at 'pullSecrets', got: %+v", results)
	}
}

func TestCompare_SliceToMap_Breaking(t *testing.T) {
	parent := map[string]interface{}{
		"pullSecrets": []interface{}{"secret-a"},
	}
	oldSource := map[string]interface{}{
		"pullSecrets": []interface{}{},
	}
	newSource := map[string]interface{}{
		"pullSecrets": map[string]interface{}{
			"name": "my-secret",
		},
	}

	results := Compare(parent, oldSource, newSource)

	found := false
	for _, r := range results {
		if r.KeyPath == "pullSecrets" && r.Breaking && r.Type == models.ChangeStructural {
			found = true
		}
	}
	if !found {
		t.Errorf("expected breaking structural change (slice→map) at 'pullSecrets', got: %+v", results)
	}
}

func TestCompare_MapToSlice_Breaking(t *testing.T) {
	parent := map[string]interface{}{
		"config": map[string]interface{}{"key": "val"},
	}
	oldSource := map[string]interface{}{
		"config": map[string]interface{}{"key": "val"},
	}
	newSource := map[string]interface{}{
		"config": []interface{}{"item1", "item2"},
	}

	results := Compare(parent, oldSource, newSource)

	found := false
	for _, r := range results {
		if r.KeyPath == "config" && r.Breaking && r.Type == models.ChangeStructural {
			found = true
		}
	}
	if !found {
		t.Errorf("expected breaking structural change (map→slice) at 'config', got: %+v", results)
	}
}

func TestCompare_ParentScalarSourceSlice_TypeMismatch(t *testing.T) {
	// Both subchart source versions have pullSecrets as a slice, but parent overrides as scalar.
	parent := map[string]interface{}{
		"pullSecrets": "my-gateway-helm-secret",
	}
	oldSource := map[string]interface{}{
		"pullSecrets": []interface{}{},
	}
	newSource := map[string]interface{}{
		"pullSecrets": []interface{}{},
	}

	results := Compare(parent, oldSource, newSource)

	found := false
	for _, r := range results {
		if r.KeyPath == "pullSecrets" && r.Breaking && r.Type == models.ChangeParentTypeMismatch {
			found = true
		}
	}
	if !found {
		t.Errorf("expected breaking parent type mismatch at 'pullSecrets', got: %+v", results)
	}
}

func TestCompare_ParentSliceSourceSlice_NoMismatch(t *testing.T) {
	// Parent and subchart source both use slices — no type mismatch
	parent := map[string]interface{}{
		"pullSecrets": []interface{}{
			map[string]interface{}{"name": "my-secret"},
		},
	}
	oldSource := map[string]interface{}{
		"pullSecrets": []interface{}{},
	}
	newSource := map[string]interface{}{
		"pullSecrets": []interface{}{},
	}

	results := Compare(parent, oldSource, newSource)

	for _, r := range results {
		if r.Breaking {
			t.Errorf("expected no breaking changes when parent and source are both slices, got: %+v", r)
		}
	}
}

func TestCompare_ParentScalarSourceMap_TypeMismatch(t *testing.T) {
	// Subchart source has map but parent overrides as scalar
	parent := map[string]interface{}{
		"database": "postgres://localhost:5432",
	}
	oldSource := map[string]interface{}{
		"database": map[string]interface{}{
			"host": "localhost",
			"port": 5432,
		},
	}
	newSource := map[string]interface{}{
		"database": map[string]interface{}{
			"host": "localhost",
			"port": 5432,
		},
	}

	results := Compare(parent, oldSource, newSource)

	found := false
	for _, r := range results {
		if r.KeyPath == "database" && r.Breaking && r.Type == models.ChangeParentTypeMismatch {
			found = true
		}
	}
	if !found {
		t.Errorf("expected breaking parent type mismatch at 'database', got: %+v", results)
	}
}

func TestCompare_NestedParentTypeMismatch(t *testing.T) {
	// Deep nested: parent has scalar at images.envoyGateway.pullSecrets,
	// subchart source has slice — mimics the gateway-helm test case
	parent := map[string]interface{}{
		"global": map[string]interface{}{
			"images": map[string]interface{}{
				"envoyGateway": map[string]interface{}{
					"image":       "v1.6.0",
					"pullSecrets": "my-gateway-helm-secret",
				},
			},
		},
	}
	oldSource := map[string]interface{}{
		"global": map[string]interface{}{
			"images": map[string]interface{}{
				"envoyGateway": map[string]interface{}{
					"image":       "docker.io/envoyproxy/gateway:v1.7.0",
					"pullSecrets": []interface{}{},
				},
			},
		},
	}
	newSource := map[string]interface{}{
		"global": map[string]interface{}{
			"images": map[string]interface{}{
				"envoyGateway": map[string]interface{}{
					"image":       "docker.io/envoyproxy/gateway:v1.7.1",
					"pullSecrets": []interface{}{},
				},
			},
		},
	}

	results := Compare(parent, oldSource, newSource)

	foundMismatch := false
	for _, r := range results {
		if r.KeyPath == "global.images.envoyGateway.pullSecrets" && r.Breaking && r.Type == models.ChangeParentTypeMismatch {
			foundMismatch = true
		}
	}
	if !foundMismatch {
		t.Errorf("expected breaking parent type mismatch at 'global.images.envoyGateway.pullSecrets', got: %+v", results)
	}
}

// =====================================================================
// Missing Override Detection Tests
// =====================================================================

func TestCompare_MissingOverride_NewlyAddedKey_Breaking(t *testing.T) {
	// Parent overrides a block but subchart source adds a new key in the new version.
	// The parent doesn't have the new key → should be flagged as breaking.
	parent := map[string]interface{}{
		"global": map[string]interface{}{
			"images": map[string]interface{}{
				"envoyGateway": map[string]interface{}{
					"image": "v1.6.0",
				},
			},
		},
	}
	oldSource := map[string]interface{}{
		"global": map[string]interface{}{
			"images": map[string]interface{}{
				"envoyGateway": map[string]interface{}{
					"image": "docker.io/envoyproxy/gateway:v1.7.0",
				},
			},
		},
	}
	newSource := map[string]interface{}{
		"global": map[string]interface{}{
			"images": map[string]interface{}{
				"envoyGateway": map[string]interface{}{
					"image":      "docker.io/envoyproxy/gateway:v1.7.1",
					"pullPolicy": "IfNotPresent", // newly added in new version
				},
			},
		},
	}

	results := Compare(parent, oldSource, newSource)

	found := false
	for _, r := range results {
		if r.KeyPath == "global.images.envoyGateway.pullPolicy" &&
			r.Type == models.ChangeMissingOverride &&
			r.Breaking {
			found = true
		}
	}
	if !found {
		t.Errorf("expected breaking missing override at 'global.images.envoyGateway.pullPolicy', got: %+v", results)
	}
}

func TestCompare_MissingOverride_PreExistingKey_NonBreaking(t *testing.T) {
	// Parent overrides a block but subchart source has a key that existed in both
	// old and new versions. The parent never set it → informational, non-breaking.
	parent := map[string]interface{}{
		"global": map[string]interface{}{
			"images": map[string]interface{}{
				"envoyGateway": map[string]interface{}{
					"image": "v1.6.0",
				},
			},
		},
	}
	oldSource := map[string]interface{}{
		"global": map[string]interface{}{
			"images": map[string]interface{}{
				"envoyGateway": map[string]interface{}{
					"image":      "docker.io/envoyproxy/gateway:v1.7.0",
					"pullPolicy": "IfNotPresent", // existed in old version already
				},
			},
		},
	}
	newSource := map[string]interface{}{
		"global": map[string]interface{}{
			"images": map[string]interface{}{
				"envoyGateway": map[string]interface{}{
					"image":      "docker.io/envoyproxy/gateway:v1.7.1",
					"pullPolicy": "IfNotPresent", // still exists, not newly added
				},
			},
		},
	}

	results := Compare(parent, oldSource, newSource)

	for _, r := range results {
		if r.KeyPath == "global.images.envoyGateway.pullPolicy" &&
			r.Type == models.ChangeMissingOverride {
			if r.Breaking {
				t.Errorf("expected pre-existing missing override to be non-breaking, got: %+v", r)
			}
			return // found the informational result, pass
		}
	}
	// It's acceptable to emit a non-breaking informational result
}

func TestCompare_MissingOverride_GatewayHelmScenario(t *testing.T) {
	// Real-world scenario: parent overrides gateway-helm.global.images block
	// but doesn't have imagePullSecrets at the global level.
	// The subchart source repo has it in both versions → non-breaking informational.
	parent := map[string]interface{}{
		"global": map[string]interface{}{
			"images": map[string]interface{}{
				"envoyGateway": map[string]interface{}{
					"image":   "v1.6.0",
					"testing": true,
				},
				"ratelimit": map[string]interface{}{
					"image":      "docker.io/envoyproxy/ratelimit:master",
					"pullPolicy": "IfNotPresent",
					"pullSecrets": []interface{}{
						map[string]interface{}{"name": "my-ratelimit-secret"},
					},
				},
			},
		},
	}
	oldSource := map[string]interface{}{
		"global": map[string]interface{}{
			"imageRegistry":  "",
			"imagePullSecrets": []interface{}{},
			"images": map[string]interface{}{
				"envoyGateway": map[string]interface{}{
					"image":       "docker.io/envoyproxy/gateway:v1.7.0",
					"pullPolicy":  "IfNotPresent",
					"pullSecrets": []interface{}{},
				},
				"ratelimit": map[string]interface{}{
					"image":       "docker.io/envoyproxy/ratelimit:3fb70258",
					"pullPolicy":  "IfNotPresent",
					"pullSecrets": []interface{}{},
				},
			},
		},
	}
	newSource := map[string]interface{}{
		"global": map[string]interface{}{
			"imageRegistry":  "",
			"imagePullSecrets": []interface{}{},
			"images": map[string]interface{}{
				"envoyGateway": map[string]interface{}{
					"image":       "docker.io/envoyproxy/gateway:v1.7.1",
					"pullPolicy":  "IfNotPresent",
					"pullSecrets": []interface{}{},
				},
				"ratelimit": map[string]interface{}{
					"image":       "docker.io/envoyproxy/ratelimit:4ab12345",
					"pullPolicy":  "IfNotPresent",
					"pullSecrets": []interface{}{},
				},
			},
		},
	}

	results := Compare(parent, oldSource, newSource)

	// global.imagePullSecrets exists in subchart source (both versions) but
	// not in parent → should be flagged as non-breaking missing override
	foundImagePullSecrets := false
	for _, r := range results {
		if r.KeyPath == "global.imagePullSecrets" && r.Type == models.ChangeMissingOverride {
			foundImagePullSecrets = true
			if r.Breaking {
				t.Errorf("expected pre-existing missing key 'global.imagePullSecrets' to be non-breaking, got: %+v", r)
			}
		}
	}
	if !foundImagePullSecrets {
		t.Errorf("expected missing override result for 'global.imagePullSecrets', got: %+v", results)
	}

	// global.images.envoyGateway.pullPolicy exists in subchart source but
	// not in parent → should be flagged as non-breaking missing override
	foundPullPolicy := false
	for _, r := range results {
		if r.KeyPath == "global.images.envoyGateway.pullPolicy" && r.Type == models.ChangeMissingOverride {
			foundPullPolicy = true
			if r.Breaking {
				t.Errorf("expected pre-existing missing key 'global.images.envoyGateway.pullPolicy' to be non-breaking, got: %+v", r)
			}
		}
	}
	if !foundPullPolicy {
		t.Errorf("expected missing override result for 'global.images.envoyGateway.pullPolicy', got: %+v", results)
	}
}

func TestCompare_MissingOverride_NewVersionAddsKey_Breaking(t *testing.T) {
	// New subchart source version adds imagePullSecrets under global that
	// didn't exist in old version. Parent overrides global block → breaking.
	parent := map[string]interface{}{
		"global": map[string]interface{}{
			"images": map[string]interface{}{
				"envoyGateway": map[string]interface{}{
					"image": "v1.6.0",
				},
			},
		},
	}
	oldSource := map[string]interface{}{
		"global": map[string]interface{}{
			"images": map[string]interface{}{
				"envoyGateway": map[string]interface{}{
					"image": "docker.io/envoyproxy/gateway:v1.7.0",
				},
			},
		},
	}
	newSource := map[string]interface{}{
		"global": map[string]interface{}{
			"imagePullSecrets": []interface{}{}, // newly added in new version
			"images": map[string]interface{}{
				"envoyGateway": map[string]interface{}{
					"image": "docker.io/envoyproxy/gateway:v1.7.1",
				},
			},
		},
	}

	results := Compare(parent, oldSource, newSource)

	found := false
	for _, r := range results {
		if r.KeyPath == "global.imagePullSecrets" &&
			r.Type == models.ChangeMissingOverride &&
			r.Breaking {
			found = true
		}
	}
	if !found {
		t.Errorf("expected breaking missing override for newly added 'global.imagePullSecrets', got: %+v", results)
	}
}

func TestCompare_MissingOverride_NestedDeep(t *testing.T) {
	// New subchart source version adds pullSecrets deep in the structure
	// that the parent doesn't have → breaking missing override.
	parent := map[string]interface{}{
		"config": map[string]interface{}{
			"auth": map[string]interface{}{
				"enabled": true,
			},
		},
	}
	oldSource := map[string]interface{}{
		"config": map[string]interface{}{
			"auth": map[string]interface{}{
				"enabled": false,
			},
		},
	}
	newSource := map[string]interface{}{
		"config": map[string]interface{}{
			"auth": map[string]interface{}{
				"enabled":  false,
				"provider": "oidc", // newly added
			},
		},
	}

	results := Compare(parent, oldSource, newSource)

	found := false
	for _, r := range results {
		if r.KeyPath == "config.auth.provider" &&
			r.Type == models.ChangeMissingOverride &&
			r.Breaking {
			found = true
		}
	}
	if !found {
		t.Errorf("expected breaking missing override at 'config.auth.provider', got: %+v", results)
	}
}

func TestCompare_MissingOverride_NoFalsePositiveWhenParentHasKey(t *testing.T) {
	// Parent has all the keys the subchart source has → no missing override.
	parent := map[string]interface{}{
		"global": map[string]interface{}{
			"imageRegistry":    "",
			"imagePullSecrets": []interface{}{},
			"images": map[string]interface{}{
				"envoyGateway": map[string]interface{}{
					"image": "v1.6.0",
				},
			},
		},
	}
	oldSource := map[string]interface{}{
		"global": map[string]interface{}{
			"imageRegistry":    "",
			"imagePullSecrets": []interface{}{},
			"images": map[string]interface{}{
				"envoyGateway": map[string]interface{}{
					"image": "docker.io/envoyproxy/gateway:v1.7.0",
				},
			},
		},
	}
	newSource := map[string]interface{}{
		"global": map[string]interface{}{
			"imageRegistry":    "",
			"imagePullSecrets": []interface{}{},
			"images": map[string]interface{}{
				"envoyGateway": map[string]interface{}{
					"image": "docker.io/envoyproxy/gateway:v1.7.1",
				},
			},
		},
	}

	results := Compare(parent, oldSource, newSource)

	for _, r := range results {
		if r.Type == models.ChangeMissingOverride {
			t.Errorf("expected no missing override when parent has all keys, got: %+v", r)
		}
	}
}
