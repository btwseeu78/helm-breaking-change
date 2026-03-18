package diff

import "testing"

func TestIsSafeConversion_StringToInt(t *testing.T) {
	if !IsSafeConversion("8080", 8080) {
		t.Error("expected string '8080' → int 8080 to be safe")
	}
}

func TestIsSafeConversion_IntToString(t *testing.T) {
	if !IsSafeConversion(8080, "8080") {
		t.Error("expected int 8080 → string '8080' to be safe")
	}
}

func TestIsSafeConversion_StringToFloat(t *testing.T) {
	if !IsSafeConversion("3.14", 3.14) {
		t.Error("expected string '3.14' → float 3.14 to be safe")
	}
}

func TestIsSafeConversion_StringToBool(t *testing.T) {
	if !IsSafeConversion("true", true) {
		t.Error("expected string 'true' → bool true to be safe")
	}
}

func TestIsSafeConversion_BoolToString(t *testing.T) {
	if !IsSafeConversion(true, "true") {
		t.Error("expected bool true → string 'true' to be safe")
	}
}

func TestIsSafeConversion_IntToFloat(t *testing.T) {
	if !IsSafeConversion(42, float64(42)) {
		t.Error("expected int 42 → float 42.0 to be safe")
	}
}

func TestIsSafeConversion_DifferentStringValues(t *testing.T) {
	// Different string values are same type — not a conversion
	if !IsSafeConversion("hello", "world") {
		t.Error("expected same-type comparison to be safe")
	}
}

func TestIsSafeConversion_StringToIncompatibleInt(t *testing.T) {
	if IsSafeConversion("not-a-number", 42) {
		t.Error("expected non-numeric string to int to be unsafe")
	}
}

func TestIsSafeConversion_NilValues(t *testing.T) {
	if IsSafeConversion(nil, 42) {
		t.Error("expected nil to int to be unsafe")
	}
	if IsSafeConversion(42, nil) {
		t.Error("expected int to nil to be unsafe")
	}
}
