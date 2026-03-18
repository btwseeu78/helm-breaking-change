package diff

import (
	"fmt"
	"strconv"
)

// IsSafeConversion checks whether converting from oldVal's type to newVal's type
// is lossless and safe. Returns true for conversions like string↔int, string↔float,
// string↔bool where the value is preservable.
func IsSafeConversion(oldVal, newVal interface{}) bool {
	if oldVal == nil || newVal == nil {
		return false
	}

	// Same type is always safe (not really a conversion)
	oldStr := fmt.Sprintf("%T", oldVal)
	newStr := fmt.Sprintf("%T", newVal)
	if oldStr == newStr {
		return true
	}

	// Try all known safe conversions
	return canConvertStringToNumber(oldVal, newVal) ||
		canConvertStringToNumber(newVal, oldVal) ||
		canConvertStringToBool(oldVal, newVal) ||
		canConvertStringToBool(newVal, oldVal) ||
		canConvertIntToFloat(oldVal, newVal) ||
		canConvertIntToFloat(newVal, oldVal)
}

// canConvertStringToNumber checks if one side is a string representation of a number
// and the other side is the numeric value.
func canConvertStringToNumber(a, b interface{}) bool {
	str, ok := a.(string)
	if !ok {
		return false
	}

	switch bv := b.(type) {
	case int:
		parsed, err := strconv.Atoi(str)
		return err == nil && parsed == bv
	case int64:
		parsed, err := strconv.ParseInt(str, 10, 64)
		return err == nil && parsed == bv
	case float64:
		parsed, err := strconv.ParseFloat(str, 64)
		return err == nil && parsed == bv
	}

	return false
}

// canConvertStringToBool checks if one side is a string representation of a bool
// and the other side is the bool value.
func canConvertStringToBool(a, b interface{}) bool {
	str, ok := a.(string)
	if !ok {
		return false
	}

	bv, ok := b.(bool)
	if !ok {
		return false
	}

	parsed, err := strconv.ParseBool(str)
	return err == nil && parsed == bv
}

// canConvertIntToFloat checks if one side is an int and the other is a float
// with the same numeric value (no precision loss).
func canConvertIntToFloat(a, b interface{}) bool {
	switch av := a.(type) {
	case int:
		if bv, ok := b.(float64); ok {
			return float64(av) == bv
		}
	case int64:
		if bv, ok := b.(float64); ok {
			return float64(av) == bv
		}
	}
	return false
}
