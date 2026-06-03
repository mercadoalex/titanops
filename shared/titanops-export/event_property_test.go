package export

import (
	"strings"
	"testing"
	"time"

	"pgregory.net/rapid"
)

// **Validates: Requirements 7.3, 7.5, 7.6, 7.7, 7.8**
// Property 12: Event schema validation round-trip and constraints

// validSeverityGen generates a random valid severity level.
func validSeverityGen() *rapid.Generator[string] {
	return rapid.SampledFrom([]string{"critical", "high", "medium", "low", "informational"})
}

// validModuleGen generates a random valid module identifier.
func validModuleGen() *rapid.Generator[string] {
	return rapid.SampledFrom([]string{"tlapix", "earthworm", "ebeecontrol", "quack", "correlation"})
}

// validEventGen generates a random valid Event.
func validEventGen() *rapid.Generator[Event] {
	return rapid.Custom(func(t *rapid.T) Event {
		namespace := rapid.StringMatching(`[a-z][a-z0-9\-]{2,20}`).Draw(t, "namespace")
		severity := validSeverityGen().Draw(t, "severity")
		module := validModuleGen().Draw(t, "module")
		eventType := rapid.StringMatching(`[a-z_]{3,30}`).Draw(t, "eventType")
		// Payload between 1 byte and 64KB.
		payloadSize := rapid.IntRange(1, MaxPayloadSize).Draw(t, "payloadSize")
		payload := rapid.SliceOfN(rapid.Byte(), payloadSize, payloadSize).Draw(t, "payload")
		eventID := rapid.StringMatching(`[a-f0-9]{8}-[a-f0-9]{4}-4[a-f0-9]{3}-[89ab][a-f0-9]{3}-[a-f0-9]{12}`).Draw(t, "eventID")

		return Event{
			Namespace: namespace,
			Timestamp: time.Now().UTC(),
			Severity:  severity,
			Module:    module,
			EventType: eventType,
			Payload:   payload,
			EventID:   eventID,
			Node:      rapid.StringMatching(`node-[a-z0-9]{1,10}`).Draw(t, "node"),
			Pod:       rapid.StringMatching(`pod-[a-z0-9]{1,10}`).Draw(t, "pod"),
		}
	})
}

func TestProperty12_ValidEvent_NoValidationErrors(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		event := validEventGen().Draw(t, "event")

		errs := ValidateEvent(event)
		if len(errs) != 0 {
			var msgs []string
			for _, e := range errs {
				msgs = append(msgs, e.Error())
			}
			t.Fatalf("expected no validation errors for valid event, got: %s", strings.Join(msgs, "; "))
		}
	})
}

func TestProperty12_MissingRequiredFields_ReturnsErrors(t *testing.T) {
	// Generate events with randomly removed required fields.
	requiredFields := []string{"namespace", "timestamp", "severity", "module", "event_type", "payload"}

	rapid.Check(t, func(t *rapid.T) {
		// Start with a valid event.
		event := validEventGen().Draw(t, "event")

		// Choose which required fields to blank out (at least 1).
		numFieldsToRemove := rapid.IntRange(1, len(requiredFields)).Draw(t, "numFields")
		fieldsToRemove := rapid.Permutation(requiredFields).Draw(t, "fieldsToRemove")[:numFieldsToRemove]

		for _, field := range fieldsToRemove {
			switch field {
			case "namespace":
				event.Namespace = ""
			case "timestamp":
				event.Timestamp = time.Time{}
			case "severity":
				event.Severity = ""
			case "module":
				event.Module = ""
			case "event_type":
				event.EventType = ""
			case "payload":
				event.Payload = nil
			}
		}

		errs := ValidateEvent(event)
		if len(errs) == 0 {
			t.Fatalf("expected validation errors when fields %v are missing, got none", fieldsToRemove)
		}

		// Verify each missing field is identified.
		errFields := make(map[string]bool)
		for _, e := range errs {
			errFields[e.Field] = true
		}

		for _, field := range fieldsToRemove {
			if !errFields[field] {
				t.Errorf("expected validation error for missing field %q, but not found in errors", field)
			}
		}
	})
}

func TestProperty12_PayloadSizeConstraint(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		event := validEventGen().Draw(t, "event")

		// Generate payload that's between 64KB and 128KB (oversized).
		oversizeAmount := rapid.IntRange(1, MaxPayloadSize).Draw(t, "oversizeAmount")
		event.Payload = make([]byte, MaxPayloadSize+oversizeAmount)

		errs := ValidateEvent(event)

		// Should have a payload size error.
		foundPayloadErr := false
		for _, e := range errs {
			if e.Field == "payload" && strings.Contains(e.Message, "exceeds maximum") {
				foundPayloadErr = true
			}
		}
		if !foundPayloadErr {
			t.Fatalf("expected payload size validation error for %d bytes, got errors: %v", len(event.Payload), errs)
		}
	})
}

func TestProperty12_PayloadWithinLimit_Accepted(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		event := validEventGen().Draw(t, "event")

		// Ensure payload is within limit (already guaranteed by validEventGen, but explicit).
		payloadSize := rapid.IntRange(1, MaxPayloadSize).Draw(t, "payloadSize")
		event.Payload = make([]byte, payloadSize)
		// Fill with non-zero bytes so it's not "empty".
		for i := range event.Payload {
			event.Payload[i] = 0x42
		}

		errs := ValidateEvent(event)

		// Should have no payload size error.
		for _, e := range errs {
			if e.Field == "payload" && strings.Contains(e.Message, "exceeds maximum") {
				t.Fatalf("payload of %d bytes should be accepted, but got size error", payloadSize)
			}
		}
	})
}

func TestProperty12_InvalidSeverity_Rejected(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		event := validEventGen().Draw(t, "event")

		// Set invalid severity.
		invalidSeverity := rapid.StringMatching(`[A-Z][a-z]{3,10}`).Draw(t, "invalidSeverity")
		// Ensure it's actually invalid.
		if ValidSeverities[invalidSeverity] {
			t.Skip("generated a valid severity by chance")
		}
		event.Severity = invalidSeverity

		errs := ValidateEvent(event)
		foundSeverityErr := false
		for _, e := range errs {
			if e.Field == "severity" {
				foundSeverityErr = true
			}
		}
		if !foundSeverityErr {
			t.Fatalf("expected severity validation error for %q, got none", invalidSeverity)
		}
	})
}

func TestProperty12_InvalidModule_Rejected(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		event := validEventGen().Draw(t, "event")

		// Set invalid module.
		invalidModule := rapid.StringMatching(`invalid_[a-z]{3,10}`).Draw(t, "invalidModule")
		event.Module = invalidModule

		errs := ValidateEvent(event)
		foundModuleErr := false
		for _, e := range errs {
			if e.Field == "module" {
				foundModuleErr = true
			}
		}
		if !foundModuleErr {
			t.Fatalf("expected module validation error for %q, got none", invalidModule)
		}
	})
}
