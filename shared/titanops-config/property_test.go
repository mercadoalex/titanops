package config

import (
	"os"
	"path/filepath"
	"testing"

	"pgregory.net/rapid"
)

// Feature: titanops-platform-integration, Property 1: Port configuration accepts valid range and rejects invalid
// **Validates: Requirements 1.2**
func TestProperty1_PortConfigurationAcceptsValidRangeAndRejectsInvalid(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		port := rapid.IntRange(-100000, 200000).Draw(t, "port")

		result := ValidatePort(port)

		isValid := port >= 1024 && port <= 65535

		if isValid && result != nil {
			t.Fatalf("port %d should be valid but got error: %v", port, result)
		}
		if !isValid && result == nil {
			t.Fatalf("port %d should be invalid but was accepted", port)
		}
		if result != nil {
			if result.Field != "port" {
				t.Fatalf("expected field 'port', got %q", result.Field)
			}
			if result.Value != port {
				t.Fatalf("expected value %d, got %v", port, result.Value)
			}
		}
	})
}

// Feature: titanops-platform-integration, Property 14: Configuration loading produces valid config or field-level errors
// **Validates: Requirements 9.4**
func TestProperty14_ConfigurationLoadingProducesValidConfigOrFieldLevelErrors(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		// Generate random config values
		port := rapid.IntRange(-1000, 100000).Draw(t, "port")
		name := rapid.SampledFrom([]string{"", "valid-name", "test-service", "my-app"}).Draw(t, "name")
		logLevel := rapid.SampledFrom([]string{"", "debug", "info", "warn", "error", "invalid_level"}).Draw(t, "logLevel")

		type TestConfig struct {
			Port     int    `validate:"min=1024,max=65535"`
			Name     string `validate:"required"`
			LogLevel string `validate:"oneof=debug info warn error"`
		}

		cfg := &TestConfig{
			Port:     port,
			Name:     name,
			LogLevel: logLevel,
		}

		validationErrors, err := Load(cfg)

		// Load should never return a fatal error for a valid struct pointer
		if err != nil {
			t.Fatalf("Load returned unexpected fatal error: %v", err)
		}

		// Check that validation errors are field-level and properly structured
		for _, ve := range validationErrors {
			if ve.Field == "" {
				t.Fatal("validation error has empty Field")
			}
			if ve.Message == "" {
				t.Fatal("validation error has empty Message")
			}
		}

		// If port is valid AND name is non-empty AND logLevel is valid/empty,
		// there should be no errors
		portValid := port >= 1024 && port <= 65535
		nameValid := name != ""
		logLevelValid := logLevel == "" || logLevel == "debug" || logLevel == "info" || logLevel == "warn" || logLevel == "error"

		if portValid && nameValid && logLevelValid {
			if len(validationErrors) != 0 {
				t.Fatalf("expected no validation errors for valid config (port=%d, name=%q, logLevel=%q), got %v",
					port, name, logLevel, validationErrors)
			}
		}

		// If port is outside range and non-zero, there should be a validation error for it
		if port != 0 && !portValid {
			found := false
			for _, ve := range validationErrors {
				if ve.Field == "Port" {
					found = true
					break
				}
			}
			if !found {
				t.Fatalf("expected validation error for invalid port %d, got none", port)
			}
		}
	})
}

// Feature: titanops-platform-integration, Property 15: Shared library operations return typed errors without panicking
// **Validates: Requirements 9.7**
func TestProperty15_SharedLibraryOperationsReturnTypedErrorsWithoutPanicking(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		scenario := rapid.IntRange(0, 4).Draw(t, "scenario")

		// All scenarios must not panic and must return typed errors
		switch scenario {
		case 0:
			// Missing file scenario
			fakePath := filepath.Join(os.TempDir(), "nonexistent_"+rapid.StringMatching(`[a-z]{8}`).Draw(t, "filename")+".yaml")
			type DummyConfig struct {
				Port int `validate:"min=1024,max=65535"`
			}
			cfg := &DummyConfig{}
			_, err := Load(cfg, WithFile(fakePath))
			if err == nil {
				t.Fatal("expected error for missing file, got nil")
			}
			// Verify it's a typed ConfigError
			configErr, ok := err.(*ConfigError)
			if !ok {
				t.Fatalf("expected *ConfigError, got %T: %v", err, err)
			}
			if configErr.Op == "" {
				t.Fatal("ConfigError.Op should not be empty")
			}

		case 1:
			// Nil target scenario
			_, err := Load(nil)
			if err == nil {
				t.Fatal("expected error for nil target, got nil")
			}
			_, ok := err.(*ConfigError)
			if !ok {
				t.Fatalf("expected *ConfigError for nil target, got %T", err)
			}

		case 2:
			// Non-pointer target scenario
			type DummyConfig struct {
				Port int
			}
			cfg := DummyConfig{}
			_, err := Load(cfg)
			if err == nil {
				t.Fatal("expected error for non-pointer target, got nil")
			}
			_, ok := err.(*ConfigError)
			if !ok {
				t.Fatalf("expected *ConfigError for non-pointer, got %T", err)
			}

		case 3:
			// Invalid input: pointer to non-struct
			val := 42
			_, err := Load(&val)
			if err == nil {
				t.Fatal("expected error for pointer to non-struct, got nil")
			}
			_, ok := err.(*ConfigError)
			if !ok {
				t.Fatalf("expected *ConfigError for pointer to non-struct, got %T", err)
			}

		case 4:
			// Invalid file content (create temp file with garbage)
			tmpDir := t.Name()
			dir, dirErr := os.MkdirTemp("", "config-test-*")
			if dirErr != nil {
				// Skip if we can't create temp dir, don't fail the property
				_ = tmpDir
				return
			}
			defer os.RemoveAll(dir)

			badFile := filepath.Join(dir, "bad.yaml")
			os.WriteFile(badFile, []byte("{{{{invalid yaml content!!!!"), 0644)

			type DummyConfig struct {
				Port int `validate:"min=1024,max=65535"`
			}
			cfg := &DummyConfig{}
			_, err := Load(cfg, WithFile(badFile))
			// For invalid YAML, it may or may not error depending on how the parser handles it,
			// but it must NOT panic. If it errors, it should be a typed error.
			if err != nil {
				_, ok := err.(*ConfigError)
				if !ok {
					t.Fatalf("expected *ConfigError for bad file, got %T: %v", err, err)
				}
			}
		}
	})
}
