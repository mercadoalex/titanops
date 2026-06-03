package config

import "fmt"

// ValidatePort checks if a port is in the valid range [1024, 65535].
// Returns nil if valid, or a *ValidationError with details if invalid.
func ValidatePort(port int) *ValidationError {
	if port < 1024 || port > 65535 {
		return &ValidationError{
			Field:   "port",
			Value:   port,
			Message: fmt.Sprintf("port must be in range [1024, 65535], got %d", port),
		}
	}
	return nil
}

// ValidateRequired checks if a value is non-zero for its type.
// Returns nil if the value is non-zero, or a *ValidationError if empty/zero.
func ValidateRequired(field string, value interface{}) *ValidationError {
	if isZero(value) {
		return &ValidationError{
			Field:   field,
			Value:   value,
			Message: fmt.Sprintf("field %q is required but was empty or zero", field),
		}
	}
	return nil
}

// ValidateEnum checks if a string value is in the allowed set.
// Returns nil if the value is in the allowed set, or a *ValidationError if not.
func ValidateEnum(field string, value string, allowed []string) *ValidationError {
	for _, a := range allowed {
		if value == a {
			return nil
		}
	}
	return &ValidationError{
		Field:   field,
		Value:   value,
		Message: fmt.Sprintf("field %q must be one of %v, got %q", field, allowed, value),
	}
}

// ValidateRange checks if a numeric value is within [min, max].
// Returns nil if in range, or a *ValidationError if outside the range.
func ValidateRange(field string, value int, min, max int) *ValidationError {
	if value < min || value > max {
		return &ValidationError{
			Field:   field,
			Value:   value,
			Message: fmt.Sprintf("field %q must be in range [%d, %d], got %d", field, min, max, value),
		}
	}
	return nil
}

// isZero reports whether a value is the zero value for its type.
func isZero(value interface{}) bool {
	if value == nil {
		return true
	}
	switch v := value.(type) {
	case string:
		return v == ""
	case int:
		return v == 0
	case int8:
		return v == 0
	case int16:
		return v == 0
	case int32:
		return v == 0
	case int64:
		return v == 0
	case uint:
		return v == 0
	case uint8:
		return v == 0
	case uint16:
		return v == 0
	case uint32:
		return v == 0
	case uint64:
		return v == 0
	case float32:
		return v == 0
	case float64:
		return v == 0
	case bool:
		return !v
	case []byte:
		return len(v) == 0
	default:
		return false
	}
}
