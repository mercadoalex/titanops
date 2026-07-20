package config

import (
	"fmt"
	"os"
	"strconv"
	"sync"
)

// RunMode represents the platform execution mode that controls how infrastructure
// adapters behave. Every TitanOps module reads this to decide whether to use real
// infrastructure, synthetic mocks, or dry-run stubs.
type RunMode string

const (
	// ModeLive is full production behavior: real infrastructure, real side-effects.
	ModeLive RunMode = "live"
	// ModeMock replaces all infrastructure adapters with deterministic in-memory
	// implementations. No network calls. CI default.
	ModeMock RunMode = "mock"
	// ModeDryRun reads from real data sources but executes no actuator side-effects.
	// Actions are logged with full parameters instead of executed.
	ModeDryRun RunMode = "dry-run"
)

// Environment variable names for run mode configuration.
const (
	// EnvMode is the environment variable that controls the run mode.
	EnvMode = "TITANOPS_MODE"
	// EnvMockSeed is the environment variable that controls the deterministic
	// seed for mock mode. Default: 42.
	EnvMockSeed = "TITANOPS_MOCK_SEED"
)

// DefaultMockSeed is the seed used when TITANOPS_MOCK_SEED is not set.
const DefaultMockSeed int64 = 42

// validModes is the set of accepted RunMode values.
var validModes = map[RunMode]bool{
	ModeLive:   true,
	ModeMock:   true,
	ModeDryRun: true,
}

// currentMode caches the resolved mode to avoid repeated env lookups.
var (
	currentMode     RunMode
	currentModeOnce sync.Once
	currentModeErr  error
)

// CurrentMode reads and validates the TITANOPS_MODE environment variable.
// Returns ModeLive if the variable is unset or empty.
// Returns an error if the value is not one of: live, mock, dry-run.
//
// The result is cached after the first call. Use ResetMode() in tests
// to clear the cache between test cases.
func CurrentMode() (RunMode, error) {
	currentModeOnce.Do(func() {
		currentMode, currentModeErr = resolveMode()
	})
	return currentMode, currentModeErr
}

// MustCurrentMode is like CurrentMode but panics on invalid configuration.
// Use only in cmd/ entry points where fail-fast is appropriate.
func MustCurrentMode() RunMode {
	mode, err := CurrentMode()
	if err != nil {
		panic(fmt.Sprintf("titanops-config: %v", err))
	}
	return mode
}

// MockSeed returns the deterministic seed for mock mode.
// Reads from TITANOPS_MOCK_SEED environment variable.
// Returns DefaultMockSeed (42) if unset, empty, or unparseable.
func MockSeed() int64 {
	raw := os.Getenv(EnvMockSeed)
	if raw == "" {
		return DefaultMockSeed
	}
	seed, err := strconv.ParseInt(raw, 10, 64)
	if err != nil {
		return DefaultMockSeed
	}
	return seed
}

// IsMock returns true if the current mode is mock.
// Returns false if the mode cannot be determined (treats errors as non-mock).
func IsMock() bool {
	mode, err := CurrentMode()
	if err != nil {
		return false
	}
	return mode == ModeMock
}

// IsDryRun returns true if the current mode is dry-run.
// Returns false if the mode cannot be determined.
func IsDryRun() bool {
	mode, err := CurrentMode()
	if err != nil {
		return false
	}
	return mode == ModeDryRun
}

// IsLive returns true if the current mode is live (or defaulted to live).
// Returns false if the mode cannot be determined.
func IsLive() bool {
	mode, err := CurrentMode()
	if err != nil {
		return false
	}
	return mode == ModeLive
}

// ValidateMode checks whether a string is a valid RunMode value.
// Returns nil if valid, or a *ValidationError if not.
func ValidateMode(value string) *ValidationError {
	if value == "" {
		// Empty is valid — defaults to live.
		return nil
	}
	mode := RunMode(value)
	if !validModes[mode] {
		return &ValidationError{
			Field:   EnvMode,
			Value:   value,
			Message: fmt.Sprintf("must be one of: live, mock, dry-run (got %q)", value),
		}
	}
	return nil
}

// ResetMode clears the cached mode so that CurrentMode() re-reads the
// environment on next call. Use only in tests.
func ResetMode() {
	currentModeOnce = sync.Once{}
	currentMode = ""
	currentModeErr = nil
}

// resolveMode reads TITANOPS_MODE from the environment and validates it.
func resolveMode() (RunMode, error) {
	raw := os.Getenv(EnvMode)
	if raw == "" {
		return ModeLive, nil
	}

	mode := RunMode(raw)
	if !validModes[mode] {
		return "", &ConfigError{
			Op:  "resolve_mode",
			Err: fmt.Errorf("invalid %s value %q: must be one of live, mock, dry-run", EnvMode, raw),
		}
	}
	return mode, nil
}
