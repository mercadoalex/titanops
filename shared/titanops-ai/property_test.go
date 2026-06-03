package ai

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"pgregory.net/rapid"
)

// --- Test helpers ---

// failingCloudBackend always returns an error for Train and Explain.
type failingCloudBackend struct {
	err error
}

func (f *failingCloudBackend) Train(_ context.Context, _ TrainRequest) (TrainResponse, error) {
	return TrainResponse{}, f.err
}

func (f *failingCloudBackend) Explain(_ context.Context, _ ExplainRequest) (ExplainResponse, error) {
	return ExplainResponse{}, f.err
}

// successCloudBackend always succeeds (used to verify Predict still uses local).
type successCloudBackend struct{}

func (s *successCloudBackend) Train(_ context.Context, req TrainRequest) (TrainResponse, error) {
	return TrainResponse{
		ModelPath: "/cloud/" + req.ModuleID + "-model.onnx",
		Metrics:   map[string]float64{"accuracy": 0.95},
	}, nil
}

func (s *successCloudBackend) Explain(_ context.Context, req ExplainRequest) (ExplainResponse, error) {
	return ExplainResponse{
		Reasoning: "cloud explanation for " + req.ModuleID,
		Factors:   []ExplainFactor{{Name: "cloud_factor", Weight: 0.9, Direction: "positive"}},
	}, nil
}

// setupTestModelDir creates a temp directory with ONNX model files for test modules.
func setupTestModelDir(t testing.TB, modules []string) (string, func()) {
	t.Helper()
	dir, err := os.MkdirTemp("", "ai-property-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}

	for _, mod := range modules {
		modelPath := filepath.Join(dir, mod+"-anomaly.onnx")
		// Write some non-empty content to simulate a valid model file
		err := os.WriteFile(modelPath, []byte("ONNX_MODEL_STUB_DATA_FOR_TESTING"), 0644)
		if err != nil {
			os.RemoveAll(dir)
			t.Fatalf("failed to create model file: %v", err)
		}
	}

	return dir, func() { os.RemoveAll(dir) }
}

// --- Generators ---

func genFeatureVector() *rapid.Generator[[]float32] {
	return rapid.Custom[[]float32](func(t *rapid.T) []float32 {
		n := rapid.IntRange(1, 20).Draw(t, "featureLen")
		vec := make([]float32, n)
		for i := range vec {
			vec[i] = rapid.Float32Range(0, 1).Draw(t, fmt.Sprintf("f[%d]", i))
		}
		return vec
	})
}

func genModuleID() *rapid.Generator[string] {
	return rapid.SampledFrom([]string{"tlapix", "earthworm", "ebeecontrol", "quack", "correlation"})
}

// Feature: titanops-platform-integration, Property 9: Predict operations always use local ONNX without network calls
// **Validates: Requirements 6.1, 6.5, 6.6**
func TestProperty9_PredictOperationsAlwaysUseLocalONNX(t *testing.T) {
	modules := []string{"tlapix", "earthworm", "ebeecontrol", "quack", "correlation"}
	modelDir, cleanup := setupTestModelDir(t, modules)
	defer cleanup()

	localProvider, err := NewLocalProvider(modelDir)
	if err != nil {
		t.Fatalf("failed to create local provider: %v", err)
	}

	// Create a CloudProvider wrapping local + a success cloud backend
	// Even with a working cloud backend, Predict must still use local.
	cloudProvider := NewCloudProvider(localProvider, &successCloudBackend{}, 5*time.Second)

	rapid.Check(t, func(t *rapid.T) {
		moduleID := genModuleID().Draw(t, "moduleID")
		features := genFeatureVector().Draw(t, "features")

		ctx := context.Background()
		req := PredictRequest{
			ModuleID: moduleID,
			Features: features,
		}

		// Call Predict on CloudProvider — it must delegate to local
		resp, err := cloudProvider.Predict(ctx, req)
		if err != nil {
			t.Fatalf("Predict failed unexpectedly: %v", err)
		}

		// Verify response is valid (comes from local ONNX inference)
		if resp.Score < 0 || resp.Score > 1 {
			t.Fatalf("score out of [0,1]: %f", resp.Score)
		}
		if resp.Confidence < 0 || resp.Confidence > 1 {
			t.Fatalf("confidence out of [0,1]: %f", resp.Confidence)
		}
		if resp.Labels == nil {
			t.Fatal("labels should not be nil")
		}

		// Also verify the local provider directly gives same behavior
		respLocal, errLocal := localProvider.Predict(ctx, req)
		if errLocal != nil {
			t.Fatalf("local Predict failed: %v", errLocal)
		}
		if resp.Score != respLocal.Score || resp.Confidence != respLocal.Confidence {
			t.Fatalf("CloudProvider.Predict gave different result than LocalProvider.Predict: cloud=%+v, local=%+v", resp, respLocal)
		}
	})
}

// Feature: titanops-platform-integration, Property 10: Cloud AI fallback to local on failure
// **Validates: Requirements 6.3**
func TestProperty10_CloudAIFallbackToLocalOnFailure(t *testing.T) {
	modules := []string{"tlapix", "earthworm", "ebeecontrol", "quack", "correlation"}
	modelDir, cleanup := setupTestModelDir(t, modules)
	defer cleanup()

	localProvider, err := NewLocalProvider(modelDir)
	if err != nil {
		t.Fatalf("failed to create local provider: %v", err)
	}

	rapid.Check(t, func(t *rapid.T) {
		moduleID := genModuleID().Draw(t, "moduleID")
		failureType := rapid.SampledFrom([]string{"timeout", "connection_refused", "internal_error"}).Draw(t, "failureType")

		var cloudErr error
		switch failureType {
		case "timeout":
			cloudErr = context.DeadlineExceeded
		case "connection_refused":
			cloudErr = errors.New("connection refused")
		case "internal_error":
			cloudErr = errors.New("internal server error")
		}

		failingBackend := &failingCloudBackend{err: cloudErr}
		cloudProvider := NewCloudProvider(localProvider, failingBackend, 1*time.Millisecond)

		ctx := context.Background()

		// Test Train fallback
		trainReq := TrainRequest{
			ModuleID: moduleID,
			Data:     [][]float32{{0.1, 0.2, 0.3}},
			Labels:   []string{"normal"},
		}
		trainResp, trainErr := cloudProvider.Train(ctx, trainReq)
		if trainErr != nil {
			t.Fatalf("Train should fallback to local without error, got: %v", trainErr)
		}
		// Local provider returns a valid model path
		if trainResp.ModelPath == "" {
			t.Fatal("Train fallback should return a valid model path")
		}

		// Test Explain fallback
		explainReq := ExplainRequest{
			ModuleID: moduleID,
			Decision: PredictResponse{Score: 0.8, Confidence: 0.9, Labels: map[string]float64{"anomaly": 0.8}},
			Context:  map[string]string{"test": "value"},
		}
		explainResp, explainErr := cloudProvider.Explain(ctx, explainReq)
		if explainErr != nil {
			t.Fatalf("Explain should fallback to local without error, got: %v", explainErr)
		}
		// Local provider returns a valid explanation
		if explainResp.Reasoning == "" {
			t.Fatal("Explain fallback should return a non-empty reasoning")
		}
	})
}

// Feature: titanops-platform-integration, Property 11: Missing ONNX model returns typed error
// **Validates: Requirements 6.7**
func TestProperty11_MissingONNXModelReturnsTypedError(t *testing.T) {
	// Create a model directory with NO model files
	emptyDir, err := os.MkdirTemp("", "ai-empty-models-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(emptyDir)

	localProvider, err := NewLocalProvider(emptyDir)
	if err != nil {
		t.Fatalf("failed to create local provider: %v", err)
	}

	rapid.Check(t, func(t *rapid.T) {
		// Generate a random module ID (all will be missing since dir is empty)
		moduleID := rapid.SampledFrom([]string{
			"tlapix", "earthworm", "ebeecontrol", "quack", "correlation",
			"nonexistent-module", "random_mod_123",
		}).Draw(t, "moduleID")

		features := genFeatureVector().Draw(t, "features")

		ctx := context.Background()
		req := PredictRequest{
			ModuleID: moduleID,
			Features: features,
		}

		// Predict should return a typed error, not panic
		_, err := localProvider.Predict(ctx, req)
		if err == nil {
			t.Fatalf("expected error for missing model (module=%q), got nil", moduleID)
		}

		// Verify it's an *AIError with ErrModelUnavailable category
		var aiErr *AIError
		if !errors.As(err, &aiErr) {
			t.Fatalf("expected *AIError, got %T: %v", err, err)
		}
		if aiErr.Category != ErrModelUnavailable {
			t.Fatalf("expected category ErrModelUnavailable, got %q", aiErr.Category)
		}
		if aiErr.Module != moduleID {
			t.Fatalf("expected module %q, got %q", moduleID, aiErr.Module)
		}
		if aiErr.Path == "" {
			t.Fatal("expected non-empty Path in AIError")
		}
	})
}
