package export

import "fmt"

// MaxPayloadSize is the maximum allowed payload size in bytes (64 KB).
const MaxPayloadSize = 65536

// ValidSeverities contains the accepted severity level values.
var ValidSeverities = map[string]bool{
	"critical":      true,
	"high":          true,
	"medium":        true,
	"low":           true,
	"informational": true,
}

// ValidModules contains the accepted module identifier values.
var ValidModules = map[string]bool{
	"tlapix":      true,
	"earthworm":   true,
	"ebeecontrol": true,
	"quack":       true,
	"correlation": true,
}

// ValidationError represents a field-level validation failure.
type ValidationError struct {
	// Field is the name of the field that failed validation.
	Field string
	// Value is the value that was rejected (as a string representation).
	Value string
	// Message describes what constraint was violated.
	Message string
}

// Error implements the error interface for ValidationError.
func (ve ValidationError) Error() string {
	return fmt.Sprintf("field %q: %s (got %q)", ve.Field, ve.Message, ve.Value)
}

// ValidateEvent validates an Event against the shared event schema constraints.
// It checks all required fields, payload size limits, timestamp validity,
// and severity/module enum membership.
// Returns a slice of ValidationError for each violation found.
// An empty slice indicates the event is valid.
func ValidateEvent(e Event) []ValidationError {
	var errs []ValidationError

	// Required field: namespace
	if e.Namespace == "" {
		errs = append(errs, ValidationError{
			Field:   "namespace",
			Value:   "",
			Message: "required field is empty",
		})
	}

	// Required field: timestamp (zero time means not set)
	if e.Timestamp.IsZero() {
		errs = append(errs, ValidationError{
			Field:   "timestamp",
			Value:   "",
			Message: "required field is empty; must be a valid UTC timestamp in RFC 3339 format with millisecond precision",
		})
	} else {
		// Enforce UTC timezone
		if e.Timestamp.Location().String() != "UTC" {
			errs = append(errs, ValidationError{
				Field:   "timestamp",
				Value:   e.Timestamp.String(),
				Message: "timestamp must be in UTC timezone",
			})
		}
	}

	// Required field: severity (must be a valid enum value)
	if e.Severity == "" {
		errs = append(errs, ValidationError{
			Field:   "severity",
			Value:   "",
			Message: "required field is empty",
		})
	} else if !ValidSeverities[e.Severity] {
		errs = append(errs, ValidationError{
			Field:   "severity",
			Value:   e.Severity,
			Message: "invalid severity level; must be one of: critical, high, medium, low, informational",
		})
	}

	// Required field: module (must be a valid enum value)
	if e.Module == "" {
		errs = append(errs, ValidationError{
			Field:   "module",
			Value:   "",
			Message: "required field is empty",
		})
	} else if !ValidModules[e.Module] {
		errs = append(errs, ValidationError{
			Field:   "module",
			Value:   e.Module,
			Message: "invalid module; must be one of: tlapix, earthworm, ebeecontrol, quack, correlation",
		})
	}

	// Required field: event_type
	if e.EventType == "" {
		errs = append(errs, ValidationError{
			Field:   "event_type",
			Value:   "",
			Message: "required field is empty",
		})
	}

	// Required field: payload (must not be nil/empty)
	if len(e.Payload) == 0 {
		errs = append(errs, ValidationError{
			Field:   "payload",
			Value:   "",
			Message: "required field is empty",
		})
	}

	// Payload size constraint: max 64 KB
	if len(e.Payload) > MaxPayloadSize {
		errs = append(errs, ValidationError{
			Field:   "payload",
			Value:   fmt.Sprintf("%d bytes", len(e.Payload)),
			Message: fmt.Sprintf("payload exceeds maximum size of %d bytes (64 KB)", MaxPayloadSize),
		})
	}

	return errs
}
