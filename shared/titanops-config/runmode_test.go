package config

import (
	"os"
	"strconv"
	"testing"

	"pgregory.net/rapid"
)

// Feature: titanops-platform-integration, Property 16: RunMode validation accepts only valid modes
// **Validates: Requirements 13.1, 13.7**
func TestProperty16_RunModeValidationAcceptsOnlyValidModes(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		validValues := []string{"live", "mock", "dry-run"}
		invalidValues := []string{"Live", "MOCK", "DRY-RUN", "dryrun", "dry_run", "test", "staging", "prod", "debug", "off", "on", "true", "false"}
		allValues := append(validValues, invalidValues...)

		value := rapid.SampledFrom(allValues).Draw(t, "modeValue")

		result := ValidateMode(value)

		isValid := value == "live" || value == "mock" || value == "dry-run"

		if isValid && result != nil {
			t.Fatalf("mode %q should be valid but got error: %v", value, result)
		}
		if !isValid && result == nil {
			t.Fatalf("mode %q should be invalid but was accepted", value)
		}
		if result != nil {
			if result.Field != EnvMode {
				t.Fatalf("expected field %q, got %q", EnvMode, result.Field)
			}
			if result.Value != value {
				t.Fatalf("expected value %q, got %v", value, result.Value)
			}
		}
	})
}

// Feature: titanops-platform-integration, Property 17: RunMode empty string validation always passes
// **Validates: Requirements 13.1**
func TestProperty17_RunModeEmptyStringValidationAlwaysPasses(t *testing.T) {
	result := ValidateMode("")
	if result != nil {
		t.Fatalf("empty string should be valid (defaults to live), got error: %v", result)
	}
}

// Feature: titanops-platform-integration, Property 18: CurrentMode returns valid mode or typed error
// **Validates: Requirements 13.1, 13.7**
func TestProperty18_CurrentModeReturnsValidModeOrTypedError(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		// Generate either a valid mode, empty string, or garbage
		values := []string{"", "live", "mock", "dry-run", "invalid", "MOCK", "Live"}
		value := rapid.SampledFrom(values).Draw(t, "envValue")

		// Reset cached mode and set environment
		ResetMode()
		if value == "" {
			os.Unsetenv(EnvMode)
		} else {
			os.Setenv(EnvMode, value)
		}
		defer func() {
			os.Unsetenv(EnvMode)
			ResetMode()
		}()

		mode, err := CurrentMode()

		isValid := value == "" || value == "live" || value == "mock" || value == "dry-run"

		if isValid {
			if err != nil {
				t.Fatalf("expected no error for %q, got: %v", value, err)
			}
			// Empty defaults to live
			expected := RunMode(value)
			if value == "" {
				expected = ModeLive
			}
			if mode != expected {
				t.Fatalf("expected mode %q for env %q, got %q", expected, value, mode)
			}
		} else {
			if err == nil {
				t.Fatalf("expected error for invalid mode %q, got nil", value)
			}
			// Verify it's a typed ConfigError
			configErr, ok := err.(*ConfigError)
			if !ok {
				t.Fatalf("expected *ConfigError, got %T: %v", err, err)
			}
			if configErr.Op == "" {
				t.Fatal("ConfigError.Op should not be empty")
			}
		}
	})
}

// Feature: titanops-platform-integration, Property 19: IsMock/IsDryRun/IsLive are mutually exclusive for valid modes
// **Validates: Requirements 13.1**
func TestProperty19_ModeHelpersMutuallyExclusive(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		mode := rapid.SampledFrom([]string{"live", "mock", "dry-run"}).Draw(t, "mode")

		ResetMode()
		os.Setenv(EnvMode, mode)
		defer func() {
			os.Unsetenv(EnvMode)
			ResetMode()
		}()

		isMock := IsMock()
		isDryRun := IsDryRun()
		isLive := IsLive()

		// Exactly one must be true
		trueCount := 0
		if isMock {
			trueCount++
		}
		if isDryRun {
			trueCount++
		}
		if isLive {
			trueCount++
		}

		if trueCount != 1 {
			t.Fatalf("mode=%q: expected exactly one helper true, got isMock=%v isDryRun=%v isLive=%v",
				mode, isMock, isDryRun, isLive)
		}

		// Verify correct one is true
		switch mode {
		case "mock":
			if !isMock {
				t.Fatalf("mode=mock but IsMock()=false")
			}
		case "dry-run":
			if !isDryRun {
				t.Fatalf("mode=dry-run but IsDryRun()=false")
			}
		case "live":
			if !isLive {
				t.Fatalf("mode=live but IsLive()=false")
			}
		}
	})
}

// Feature: titanops-platform-integration, Property 20: MockSeed is deterministic and respects environment
// **Validates: Requirements 13.4**
func TestProperty20_MockSeedDeterministicAndRespectsEnvironment(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		scenario := rapid.IntRange(0, 2).Draw(t, "scenario")

		switch scenario {
		case 0:
			// Unset env: returns default seed
			os.Unsetenv(EnvMockSeed)
			seed := MockSeed()
			if seed != DefaultMockSeed {
				t.Fatalf("expected default seed %d when env unset, got %d", DefaultMockSeed, seed)
			}

		case 1:
			// Valid integer env: returns that value
			value := rapid.Int64Range(-1000000, 1000000).Draw(t, "seedValue")
			os.Setenv(EnvMockSeed, strconv.FormatInt(value, 10))
			defer os.Unsetenv(EnvMockSeed)

			seed := MockSeed()
			if seed != value {
				t.Fatalf("expected seed %d from env, got %d", value, seed)
			}

		case 2:
			// Non-numeric env: falls back to default
			garbage := rapid.SampledFrom([]string{"abc", "not-a-number", "12.5", "", "true"}).Draw(t, "garbage")
			os.Setenv(EnvMockSeed, garbage)
			defer os.Unsetenv(EnvMockSeed)

			seed := MockSeed()
			if garbage == "" {
				// Empty string returns default
				if seed != DefaultMockSeed {
					t.Fatalf("expected default seed for empty string, got %d", seed)
				}
			} else {
				// Non-numeric returns default
				if seed != DefaultMockSeed {
					t.Fatalf("expected default seed for garbage %q, got %d", garbage, seed)
				}
			}
		}
	})
}

// Feature: titanops-platform-integration, Property 21: MustCurrentMode panics on invalid mode
// **Validates: Requirements 13.7**
func TestProperty21_MustCurrentModePanicsOnInvalidMode(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		invalid := rapid.SampledFrom([]string{"invalid", "MOCK", "Live", "test"}).Draw(t, "invalidMode")

		ResetMode()
		os.Setenv(EnvMode, invalid)
		defer func() {
			os.Unsetenv(EnvMode)
			ResetMode()
		}()

		defer func() {
			r := recover()
			if r == nil {
				t.Fatalf("expected panic for invalid mode %q, got none", invalid)
			}
		}()

		MustCurrentMode()
	})
}

// Feature: titanops-platform-integration, Property 22: CurrentMode caching is consistent
// **Validates: Requirements 13.7**
func TestProperty22_CurrentModeCachingIsConsistent(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		mode := rapid.SampledFrom([]string{"live", "mock", "dry-run"}).Draw(t, "mode")

		ResetMode()
		os.Setenv(EnvMode, mode)
		defer func() {
			os.Unsetenv(EnvMode)
			ResetMode()
		}()

		// First call resolves and caches
		mode1, err1 := CurrentMode()
		if err1 != nil {
			t.Fatalf("unexpected error: %v", err1)
		}

		// Change env — cached result should not change
		os.Setenv(EnvMode, "dry-run")
		mode2, err2 := CurrentMode()
		if err2 != nil {
			t.Fatalf("unexpected error on second call: %v", err2)
		}

		if mode1 != mode2 {
			t.Fatalf("cached mode changed: first=%q second=%q", mode1, mode2)
		}

		// After reset, should read new value
		ResetMode()
		mode3, err3 := CurrentMode()
		if err3 != nil {
			t.Fatalf("unexpected error after reset: %v", err3)
		}
		if mode3 != ModeDryRun {
			t.Fatalf("expected dry-run after reset with new env, got %q", mode3)
		}
	})
}
