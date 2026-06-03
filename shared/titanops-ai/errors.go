package ai

import "fmt"

// ErrorCategory classifies AI layer errors for typed error handling.
type ErrorCategory string

const (
	// ErrModelUnavailable indicates the ONNX model file is missing for the requested module.
	ErrModelUnavailable ErrorCategory = "model_unavailable"
	// ErrModelLoadFailed indicates the ONNX model file exists but is corrupt or unreadable.
	ErrModelLoadFailed ErrorCategory = "model_load_failed"
	// ErrInferenceTimeout indicates the inference operation exceeded the allowed time limit.
	ErrInferenceTimeout ErrorCategory = "inference_timeout"
	// ErrInvalidInput indicates the input features are invalid (wrong dimensions, empty, etc.).
	ErrInvalidInput ErrorCategory = "invalid_input"
)

// AIError is a typed error returned by AI layer operations.
// It includes the error category, module ID, human-readable message,
// file path (if applicable), and an optional wrapped cause.
type AIError struct {
	Category ErrorCategory
	Module   string
	Message  string
	Path     string
	Cause    error
}

// Error implements the error interface.
func (e *AIError) Error() string {
	if e.Cause != nil {
		return fmt.Sprintf("ai [%s] module=%s: %s (path=%s): %v", e.Category, e.Module, e.Message, e.Path, e.Cause)
	}
	return fmt.Sprintf("ai [%s] module=%s: %s (path=%s)", e.Category, e.Module, e.Message, e.Path)
}

// Unwrap returns the underlying cause for errors.Is/errors.As support.
func (e *AIError) Unwrap() error {
	return e.Cause
}
