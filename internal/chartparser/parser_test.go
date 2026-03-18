package chartparser

import (
	"testing"

	"check-breaking-change/internal/models"
)

func TestParseChartFile(t *testing.T) {
	data := []byte(`
apiVersion: v2
name: prometheus
version: 28.13.0
type: application
dependencies:
  - name: sub-1
    version: "1.33.4"
    repository: https://sub1.github.io/helm-charts
  - name: sub-2
    alias: monitoring
    version: "7.3.1"
    repository: https://sub2.github.io/helm-charts
`)
	chart, err := ParseChartFile(data)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if chart.Name != "prometheus" {
		t.Errorf("expected name 'prometheus', got %q", chart.Name)
	}

	if len(chart.Dependencies) != 2 {
		t.Fatalf("expected 2 dependencies, got %d", len(chart.Dependencies))
	}

	if chart.Dependencies[0].Name != "sub-1" {
		t.Errorf("expected first dep name 'sub-1', got %q", chart.Dependencies[0].Name)
	}

	if chart.Dependencies[1].Alias != "monitoring" {
		t.Errorf("expected second dep alias 'monitoring', got %q", chart.Dependencies[1].Alias)
	}
}

func TestResolveKey_WithAlias(t *testing.T) {
	dep := models.Dependency{Name: "sub-1", Alias: "broken-wing"}
	if dep.ResolveKey() != "broken-wing" {
		t.Errorf("expected 'broken-wing', got %q", dep.ResolveKey())
	}
}

func TestResolveKey_WithoutAlias(t *testing.T) {
	dep := models.Dependency{Name: "sub-1"}
	if dep.ResolveKey() != "sub-1" {
		t.Errorf("expected 'sub-1', got %q", dep.ResolveKey())
	}
}

func TestFindVersionChanges(t *testing.T) {
	oldDeps := []models.Dependency{
		{Name: "sub-1", Version: "1.33.4"},
		{Name: "sub-2", Version: "7.3.1"},
		{Name: "sub-3", Version: "2.0.0"},
	}
	newDeps := []models.Dependency{
		{Name: "sub-1", Version: "1.33.5"},
		{Name: "sub-2", Version: "7.3.1"}, // unchanged
		{Name: "sub-3", Version: "2.1.0"},
	}

	changes := FindVersionChanges(oldDeps, newDeps)

	if len(changes) != 2 {
		t.Fatalf("expected 2 changes, got %d", len(changes))
	}

	if changes[0].OldVersion != "1.33.4" || changes[0].NewVersion != "1.33.5" {
		t.Errorf("unexpected first change: %+v", changes[0])
	}
}

func TestFindVersionChanges_WithAlias(t *testing.T) {
	oldDeps := []models.Dependency{
		{Name: "sub-1", Alias: "my-sub", Version: "1.0.0"},
	}
	newDeps := []models.Dependency{
		{Name: "sub-1", Alias: "my-sub", Version: "2.0.0"},
	}

	changes := FindVersionChanges(oldDeps, newDeps)

	if len(changes) != 1 {
		t.Fatalf("expected 1 change, got %d", len(changes))
	}
}

func TestFindVersionChanges_NewSubchart(t *testing.T) {
	oldDeps := []models.Dependency{
		{Name: "sub-1", Version: "1.0.0"},
	}
	newDeps := []models.Dependency{
		{Name: "sub-1", Version: "1.0.0"},
		{Name: "sub-new", Version: "1.0.0"},
	}

	changes := FindVersionChanges(oldDeps, newDeps)

	if len(changes) != 0 {
		t.Errorf("expected no changes for newly added subchart, got %d", len(changes))
	}
}

func TestIsActive_NoConditionNoTags(t *testing.T) {
	dep := models.Dependency{Name: "sub-1"}
	values := map[string]interface{}{}

	if !IsActive(dep, values) {
		t.Error("expected dependency with no condition/tags to be active")
	}
}

func TestIsActive_ConditionTrue(t *testing.T) {
	dep := models.Dependency{Name: "sub-1", Condition: "sub-1.enabled"}
	values := map[string]interface{}{
		"sub-1": map[string]interface{}{
			"enabled": true,
		},
	}

	if !IsActive(dep, values) {
		t.Error("expected dependency with condition=true to be active")
	}
}

func TestIsActive_ConditionFalse(t *testing.T) {
	dep := models.Dependency{Name: "sub-1", Condition: "sub-1.enabled"}
	values := map[string]interface{}{
		"sub-1": map[string]interface{}{
			"enabled": false,
		},
	}

	if IsActive(dep, values) {
		t.Error("expected dependency with condition=false to be inactive")
	}
}

func TestIsActive_TagEnabled(t *testing.T) {
	dep := models.Dependency{Name: "sub-1", Tags: []string{"monitoring"}}
	values := map[string]interface{}{
		"tags": map[string]interface{}{
			"monitoring": true,
		},
	}

	if !IsActive(dep, values) {
		t.Error("expected dependency with enabled tag to be active")
	}
}

func TestIsActive_TagDisabled(t *testing.T) {
	dep := models.Dependency{Name: "sub-1", Tags: []string{"monitoring"}}
	values := map[string]interface{}{
		"tags": map[string]interface{}{
			"monitoring": false,
		},
	}

	if IsActive(dep, values) {
		t.Error("expected dependency with disabled tag to be inactive")
	}
}

func TestExtractSubchartValues(t *testing.T) {
	values := map[string]interface{}{
		"global": map[string]interface{}{"key": "test"},
		"sub-1": map[string]interface{}{
			"image":   "registry.k8s.io:v1",
			"replica": 1,
		},
	}

	sub := ExtractSubchartValues(values, "sub-1")
	if sub == nil {
		t.Fatal("expected non-nil subchart values")
	}

	if sub["image"] != "registry.k8s.io:v1" {
		t.Errorf("expected image 'registry.k8s.io:v1', got %v", sub["image"])
	}
}

func TestExtractSubchartValues_NotFound(t *testing.T) {
	values := map[string]interface{}{
		"global": map[string]interface{}{"key": "test"},
	}

	sub := ExtractSubchartValues(values, "sub-1")
	if sub != nil {
		t.Errorf("expected nil for missing subchart, got %v", sub)
	}
}

func TestParseValues(t *testing.T) {
	data := []byte(`
global:
    key: test
sub-1:
    image: registry.k8s.io:v1
    replica: 1
`)
	values, err := ParseValues(data)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if _, ok := values["global"]; !ok {
		t.Error("expected 'global' key in values")
	}
	if _, ok := values["sub-1"]; !ok {
		t.Error("expected 'sub-1' key in values")
	}
}
