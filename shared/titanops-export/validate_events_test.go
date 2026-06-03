package export

import (
	"testing"
	"time"
)

// validEvent returns a fully valid Event for use in tests.
func validEvent() Event {
	return Event{
		Namespace: "titanops",
		Timestamp: time.Now().UTC(),
		Severity:  "critical",
		Module:    "earthworm",
		EventType: "node_anomaly_detected",
		Payload:   []byte(`{"node":"worker-1","score":0.92}`),
		Node:      "worker-1",
		Pod:       "earthworm-agent-abc123",
		EventID:   "550e8400-e29b-41d4-a716-446655440000",
		Labels:    map[string]string{"env": "production"},
	}
}

func TestValidateEvent_ValidEvent(t *testing.T) {
	e := validEvent()
	errs := ValidateEvent(e)
	if len(errs) != 0 {
		t.Errorf("expected no validation errors for a valid event, got %d: %v", len(errs), errs)
	}
}

func TestValidateEvent_MissingNamespace(t *testing.T) {
	e := validEvent()
	e.Namespace = ""
	errs := ValidateEvent(e)
	assertFieldError(t, errs, "namespace")
}

func TestValidateEvent_MissingTimestamp(t *testing.T) {
	e := validEvent()
	e.Timestamp = time.Time{}
	errs := ValidateEvent(e)
	assertFieldError(t, errs, "timestamp")
}

func TestValidateEvent_MissingSeverity(t *testing.T) {
	e := validEvent()
	e.Severity = ""
	errs := ValidateEvent(e)
	assertFieldError(t, errs, "severity")
}

func TestValidateEvent_MissingModule(t *testing.T) {
	e := validEvent()
	e.Module = ""
	errs := ValidateEvent(e)
	assertFieldError(t, errs, "module")
}

func TestValidateEvent_MissingEventType(t *testing.T) {
	e := validEvent()
	e.EventType = ""
	errs := ValidateEvent(e)
	assertFieldError(t, errs, "event_type")
}

func TestValidateEvent_MissingPayload(t *testing.T) {
	e := validEvent()
	e.Payload = nil
	errs := ValidateEvent(e)
	assertFieldError(t, errs, "payload")
}

func TestValidateEvent_EmptyPayload(t *testing.T) {
	e := validEvent()
	e.Payload = []byte{}
	errs := ValidateEvent(e)
	assertFieldError(t, errs, "payload")
}

func TestValidateEvent_PayloadExactlyAtLimit(t *testing.T) {
	e := validEvent()
	e.Payload = make([]byte, MaxPayloadSize) // exactly 65536 bytes
	errs := ValidateEvent(e)
	if len(errs) != 0 {
		t.Errorf("expected no errors for payload at exactly 64KB limit, got %d: %v", len(errs), errs)
	}
}

func TestValidateEvent_PayloadOverLimit(t *testing.T) {
	e := validEvent()
	e.Payload = make([]byte, MaxPayloadSize+1) // 65537 bytes
	errs := ValidateEvent(e)
	assertFieldError(t, errs, "payload")
	// Check it contains the size-limit message
	found := false
	for _, err := range errs {
		if err.Field == "payload" && err.Message != "required field is empty" {
			found = true
		}
	}
	if !found {
		t.Error("expected a payload size-limit validation error")
	}
}

func TestValidateEvent_InvalidSeverity(t *testing.T) {
	e := validEvent()
	e.Severity = "extreme"
	errs := ValidateEvent(e)
	assertFieldError(t, errs, "severity")
}

func TestValidateEvent_InvalidModule(t *testing.T) {
	e := validEvent()
	e.Module = "unknown_module"
	errs := ValidateEvent(e)
	assertFieldError(t, errs, "module")
}

func TestValidateEvent_NonUTCTimestamp(t *testing.T) {
	e := validEvent()
	loc, _ := time.LoadLocation("America/New_York")
	e.Timestamp = time.Now().In(loc)
	errs := ValidateEvent(e)
	assertFieldError(t, errs, "timestamp")
}

func TestValidateEvent_MultipleErrors(t *testing.T) {
	e := Event{} // all fields empty/zero
	errs := ValidateEvent(e)
	if len(errs) < 5 {
		t.Errorf("expected at least 5 validation errors for a completely empty event, got %d: %v", len(errs), errs)
	}
	// Should contain errors for namespace, timestamp, severity, module, event_type, payload
	fields := map[string]bool{}
	for _, err := range errs {
		fields[err.Field] = true
	}
	required := []string{"namespace", "timestamp", "severity", "module", "event_type", "payload"}
	for _, f := range required {
		if !fields[f] {
			t.Errorf("expected validation error for field %q, but none found", f)
		}
	}
}

func TestValidateEvent_AllValidSeverities(t *testing.T) {
	severities := []string{"critical", "high", "medium", "low", "informational"}
	for _, sev := range severities {
		e := validEvent()
		e.Severity = sev
		errs := ValidateEvent(e)
		if len(errs) != 0 {
			t.Errorf("severity %q should be valid, got errors: %v", sev, errs)
		}
	}
}

func TestValidateEvent_AllValidModules(t *testing.T) {
	modules := []string{"tlapix", "earthworm", "ebeecontrol", "quack", "correlation"}
	for _, mod := range modules {
		e := validEvent()
		e.Module = mod
		errs := ValidateEvent(e)
		if len(errs) != 0 {
			t.Errorf("module %q should be valid, got errors: %v", mod, errs)
		}
	}
}

// assertFieldError checks that at least one validation error exists for the given field.
func assertFieldError(t *testing.T, errs []ValidationError, field string) {
	t.Helper()
	for _, err := range errs {
		if err.Field == field {
			return
		}
	}
	t.Errorf("expected validation error for field %q, got: %v", field, errs)
}
