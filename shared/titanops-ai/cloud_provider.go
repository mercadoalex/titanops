package ai

import (
	"context"
	"log"
	"time"
)

// DefaultCloudTimeout is the default timeout for cloud AI operations.
const DefaultCloudTimeout = 5 * time.Second

// Compile-time assertion that CloudProvider implements Provider.
var _ Provider = (*CloudProvider)(nil)

// CloudProvider wraps a LocalProvider and a CloudBackend to provide
// cloud-augmented AI operations with automatic fallback to local ONNX.
//
// Behavior:
//   - Predict() always delegates to LocalProvider (never uses cloud)
//   - Train() delegates to CloudBackend with timeout, falls back to local on failure
//   - Explain() delegates to CloudBackend with timeout, falls back to local on failure
type CloudProvider struct {
	local   *LocalProvider
	backend CloudBackend
	timeout time.Duration
}

// NewCloudProvider creates a CloudProvider that wraps the given local provider
// and cloud backend. The timeout controls how long cloud operations are allowed
// to run before falling back to local inference.
//
// If timeout is zero or negative, DefaultCloudTimeout (5s) is used.
func NewCloudProvider(local *LocalProvider, backend CloudBackend, timeout time.Duration) *CloudProvider {
	if timeout <= 0 {
		timeout = DefaultCloudTimeout
	}
	return &CloudProvider{
		local:   local,
		backend: backend,
		timeout: timeout,
	}
}

// Predict always delegates to the local ONNX provider regardless of cloud
// configuration. This ensures predictions never depend on network round-trips.
func (cp *CloudProvider) Predict(ctx context.Context, req PredictRequest) (PredictResponse, error) {
	return cp.local.Predict(ctx, req)
}

// Train attempts to delegate training to the cloud backend with the configured
// timeout. If the cloud backend fails (timeout, connection error, or any other
// error), it falls back to the local provider and logs a warning.
func (cp *CloudProvider) Train(ctx context.Context, req TrainRequest) (TrainResponse, error) {
	cloudCtx, cancel := context.WithTimeout(ctx, cp.timeout)
	defer cancel()

	resp, err := cp.backend.Train(cloudCtx, req)
	if err == nil {
		return resp, nil
	}

	// Cloud failed — log warning and fall back to local.
	log.Printf("[WARN] cloud train failed for module %q (provider=%T): %v — falling back to local",
		req.ModuleID, cp.backend, err)

	return cp.local.Train(ctx, req)
}

// Explain attempts to delegate explanation to the cloud backend with the
// configured timeout. If the cloud backend fails (timeout, connection error,
// or any other error), it falls back to the local provider and logs a warning.
func (cp *CloudProvider) Explain(ctx context.Context, req ExplainRequest) (ExplainResponse, error) {
	cloudCtx, cancel := context.WithTimeout(ctx, cp.timeout)
	defer cancel()

	resp, err := cp.backend.Explain(cloudCtx, req)
	if err == nil {
		return resp, nil
	}

	// Cloud failed — log warning and fall back to local.
	log.Printf("[WARN] cloud explain failed for module %q (provider=%T): %v — falling back to local",
		req.ModuleID, cp.backend, err)

	return cp.local.Explain(ctx, req)
}
