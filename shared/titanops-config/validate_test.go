package config

import (
	"testing"
)

// --- ValidatePort tests ---

func TestValidatePort_ValidMinBound(t *testing.T) {
	err := ValidatePort(1024)
	if err != nil {
		t.Errorf("expected port 1024 to be valid, got error: %s", err.Message)
	}
}

func TestValidatePort_ValidMaxBound(t *testing.T) {
	err := ValidatePort(65535)
	if err != nil {
		t.Errorf("expected port 65535 to be valid, got error: %s", err.Message)
	}
}

func TestValidatePort_ValidMiddle(t *testing.T) {
	err := ValidatePort(9090)
	if err != nil {
		t.Errorf("expected port 9090 to be valid, got error: %s", err.Message)
	}
}

func TestValidatePort_InvalidBelowRange(t *testing.T) {
	err := ValidatePort(1023)
	if err == nil {
		t.Fatal("expected port 1023 to be invalid, got nil")
	}
	if err.Field != "port" {
		t.Errorf("expected field 'port', got %q", err.Field)
	}
	if err.Value != 1023 {
		t.Errorf("expected value 1023, got %v", err.Value)
	}
}

func TestValidatePort_InvalidZero(t *testing.T) {
	err := ValidatePort(0)
	if err == nil {
		t.Fatal("expected port 0 to be invalid, got nil")
	}
}

func TestValidatePort_InvalidAboveRange(t *testing.T) {
	err := ValidatePort(65536)
	if err == nil {
		t.Fatal("expected port 65536 to be invalid, got nil")
	}
}

func TestValidatePort_InvalidNegative(t *testing.T) {
	err := ValidatePort(-1)
	if err == nil {
		t.Fatal("expected negative port to be invalid, got nil")
	}
}

// --- ValidateRequired tests ---

func TestValidateRequired_NonEmptyString(t *testing.T) {
	err := ValidateRequired("name", "hello")
	if err != nil {
		t.Errorf("expected non-empty string to be valid, got error: %s", err.Message)
	}
}

func TestValidateRequired_EmptyString(t *testing.T) {
	err := ValidateRequired("name", "")
	if err == nil {
		t.Fatal("expected empty string to be invalid, got nil")
	}
	if err.Field != "name" {
		t.Errorf("expected field 'name', got %q", err.Field)
	}
}

func TestValidateRequired_NonZeroInt(t *testing.T) {
	err := ValidateRequired("port", 8080)
	if err != nil {
		t.Errorf("expected non-zero int to be valid, got error: %s", err.Message)
	}
}

func TestValidateRequired_ZeroInt(t *testing.T) {
	err := ValidateRequired("port", 0)
	if err == nil {
		t.Fatal("expected zero int to be invalid, got nil")
	}
}

func TestValidateRequired_NilValue(t *testing.T) {
	err := ValidateRequired("config", nil)
	if err == nil {
		t.Fatal("expected nil to be invalid, got nil")
	}
}

func TestValidateRequired_NonEmptySlice(t *testing.T) {
	err := ValidateRequired("data", []byte{1, 2, 3})
	if err != nil {
		t.Errorf("expected non-empty slice to be valid, got error: %s", err.Message)
	}
}

func TestValidateRequired_EmptySlice(t *testing.T) {
	err := ValidateRequired("data", []byte{})
	if err == nil {
		t.Fatal("expected empty slice to be invalid, got nil")
	}
}

// --- ValidateEnum tests ---

func TestValidateEnum_ValidValue(t *testing.T) {
	allowed := []string{"critical", "high", "medium", "low"}
	err := ValidateEnum("severity", "high", allowed)
	if err != nil {
		t.Errorf("expected 'high' to be valid, got error: %s", err.Message)
	}
}

func TestValidateEnum_InvalidValue(t *testing.T) {
	allowed := []string{"critical", "high", "medium", "low"}
	err := ValidateEnum("severity", "unknown", allowed)
	if err == nil {
		t.Fatal("expected 'unknown' to be invalid, got nil")
	}
	if err.Field != "severity" {
		t.Errorf("expected field 'severity', got %q", err.Field)
	}
	if err.Value != "unknown" {
		t.Errorf("expected value 'unknown', got %v", err.Value)
	}
}

func TestValidateEnum_EmptyValue(t *testing.T) {
	allowed := []string{"local", "gemini", "bedrock"}
	err := ValidateEnum("provider", "", allowed)
	if err == nil {
		t.Fatal("expected empty string to be invalid enum, got nil")
	}
}

func TestValidateEnum_EmptyAllowed(t *testing.T) {
	err := ValidateEnum("mode", "anything", []string{})
	if err == nil {
		t.Fatal("expected any value to be invalid when allowed set is empty, got nil")
	}
}

// --- ValidateRange tests ---

func TestValidateRange_ValidInRange(t *testing.T) {
	err := ValidateRange("timeout", 30, 1, 60)
	if err != nil {
		t.Errorf("expected 30 in [1,60] to be valid, got error: %s", err.Message)
	}
}

func TestValidateRange_ValidMinBound(t *testing.T) {
	err := ValidateRange("threshold", 1, 1, 100)
	if err != nil {
		t.Errorf("expected 1 in [1,100] to be valid, got error: %s", err.Message)
	}
}

func TestValidateRange_ValidMaxBound(t *testing.T) {
	err := ValidateRange("threshold", 100, 1, 100)
	if err != nil {
		t.Errorf("expected 100 in [1,100] to be valid, got error: %s", err.Message)
	}
}

func TestValidateRange_InvalidBelowMin(t *testing.T) {
	err := ValidateRange("timeout", 0, 1, 60)
	if err == nil {
		t.Fatal("expected 0 below [1,60] to be invalid, got nil")
	}
	if err.Field != "timeout" {
		t.Errorf("expected field 'timeout', got %q", err.Field)
	}
}

func TestValidateRange_InvalidAboveMax(t *testing.T) {
	err := ValidateRange("timeout", 61, 1, 60)
	if err == nil {
		t.Fatal("expected 61 above [1,60] to be invalid, got nil")
	}
}

func TestValidateRange_NegativeValue(t *testing.T) {
	err := ValidateRange("count", -5, 0, 100)
	if err == nil {
		t.Fatal("expected -5 below [0,100] to be invalid, got nil")
	}
}

// --- No-panic guarantee tests ---

func TestValidatePort_NeverPanics(t *testing.T) {
	// Test with extreme values to ensure no panic
	extremes := []int{-2147483648, -1, 0, 1, 1023, 1024, 65535, 65536, 2147483647}
	for _, port := range extremes {
		func() {
			defer func() {
				if r := recover(); r != nil {
					t.Errorf("ValidatePort(%d) panicked: %v", port, r)
				}
			}()
			_ = ValidatePort(port)
		}()
	}
}

func TestValidateRequired_NeverPanics(t *testing.T) {
	values := []interface{}{nil, "", 0, false, int64(0), float64(0), []byte(nil), []byte{}}
	for _, v := range values {
		func() {
			defer func() {
				if r := recover(); r != nil {
					t.Errorf("ValidateRequired with value %v panicked: %v", v, r)
				}
			}()
			_ = ValidateRequired("field", v)
		}()
	}
}

func TestValidateEnum_NeverPanics(t *testing.T) {
	// Test with nil-like scenarios
	func() {
		defer func() {
			if r := recover(); r != nil {
				t.Errorf("ValidateEnum panicked: %v", r)
			}
		}()
		_ = ValidateEnum("field", "", nil)
	}()
}

func TestValidateRange_NeverPanics(t *testing.T) {
	// Test with extreme values
	extremes := []struct{ val, min, max int }{
		{-2147483648, -2147483648, 2147483647},
		{2147483647, 0, 2147483647},
		{0, 0, 0},
	}
	for _, e := range extremes {
		func() {
			defer func() {
				if r := recover(); r != nil {
					t.Errorf("ValidateRange(%d, %d, %d) panicked: %v", e.val, e.min, e.max, r)
				}
			}()
			_ = ValidateRange("field", e.val, e.min, e.max)
		}()
	}
}
