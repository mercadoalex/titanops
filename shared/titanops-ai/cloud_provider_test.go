package ai

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// mockCloudBackend is a test helper that simulates cloud backend behavior.
type mockCloudBackend struct {
	trainFn   func(ctx context.Context, req TrainRequest) (TrainResponse, error)
	explainFn func(ctx context.Context, req ExplainRequest) (ExplainResponse, error)
}

func (m *mockCloudBackend) Train(ctx context.Context, req TrainRequest) (TrainResponse, error) {
	if m.trainFn != nil {
		return m.trainFn(ctx, req)
	}
	return TrainResponse{}, errors.New("mock: train not implemented")
}

func (m *mockCloudBackend) Explain(ctx context.Context, req ExplainRequest) (ExplainResponse, error) {
	if m.explainFn != nil {
		return m.explainFn(ctx, req)
	}
	return ExplainResponse{}, errors.New("mock: explain not implemented")
}

// newTestLocalProvider creates a LocalProvider with a valid model file for testing.
func newTestLocalProvider(t *testing.T, moduleID string) *LocalProvider {
	t.Helper()
	dir := t.TempDir()
	modelPath := filepath.Join(dir, moduleID+"-anomaly.onnx")
	if err := os.WriteFile(modelPath, []byte("ONNX_STUB_MODEL_DATA"), 0644); err != nil {
		t.Fatal(err)
	}
	p, err := NewLocalProvider(dir)
	if err != nil {
		t.Fatal(err)
	}
	return p
}

// --- CloudProvider Tests ---

func TestNewCloudProvider_DefaultTimeout(t *testing.T) {
	local := &LocalProvider{modelDir: "/tmp", models: map[string]*ONNXSession{}}
	backend := &mockCloudBackend{}

	cp := NewCloudProvider(local, backend, 0)
	if cp.timeout != DefaultCloudTimeout {
		t.Errorf("expected default timeout %v, got %v", DefaultCloudTimeout, cp.timeout)
	}
}

func TestNewCloudProvider_CustomTimeout(t *testing.T) {
	local := &LocalProvider{modelDir: "/tmp", models: map[string]*ONNXSession{}}
	backend := &mockCloudBackend{}

	cp := NewCloudProvider(local, backend, 10*time.Second)
	if cp.timeout != 10*time.Second {
		t.Errorf("expected timeout 10s, got %v", cp.timeout)
	}
}

func TestNewCloudProvider_NegativeTimeoutUsesDefault(t *testing.T) {
	local := &LocalProvider{modelDir: "/tmp", models: map[string]*ONNXSession{}}
	backend := &mockCloudBackend{}

	cp := NewCloudProvider(local, backend, -1*time.Second)
	if cp.timeout != DefaultCloudTimeout {
		t.Errorf("expected default timeout %v, got %v", DefaultCloudTimeout, cp.timeout)
	}
}

func TestCloudProvider_Predict_AlwaysLocal(t *testing.T) {
	local := newTestLocalProvider(t, "earthworm")
	cloudCalled := false
	backend := &mockCloudBackend{
		trainFn: func(ctx context.Context, req TrainRequest) (TrainResponse, error) {
			cloudCalled = true
			return TrainResponse{}, nil
		},
	}

	cp := NewCloudProvider(local, backend, DefaultCloudTimeout)

	resp, err := cp.Predict(context.Background(), PredictRequest{
		ModuleID: "earthworm",
		Features: []float32{0.5, 0.6, 0.7},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cloudCalled {
		t.Error("cloud backend should never be called for Predict")
	}
	if resp.Score < 0 || resp.Score > 1 {
		t.Errorf("score out of range [0,1]: %f", resp.Score)
	}
}

func TestCloudProvider_Train_CloudSuccess(t *testing.T) {
	local := newTestLocalProvider(t, "earthworm")
	expectedPath := "/cloud/models/earthworm-v2.onnx"
	backend := &mockCloudBackend{
		trainFn: func(ctx context.Context, req TrainRequest) (TrainResponse, error) {
			return TrainResponse{
				ModelPath: expectedPath,
				Metrics:   map[string]float64{"accuracy": 0.95},
			}, nil
		},
	}

	cp := NewCloudProvider(local, backend, DefaultCloudTimeout)

	resp, err := cp.Train(context.Background(), TrainRequest{
		ModuleID: "earthworm",
		Data:     [][]float32{{0.1, 0.2}, {0.3, 0.4}},
		Labels:   []string{"normal", "anomaly"},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.ModelPath != expectedPath {
		t.Errorf("expected model path %s, got %s", expectedPath, resp.ModelPath)
	}
	if resp.Metrics["accuracy"] != 0.95 {
		t.Errorf("expected accuracy 0.95, got %f", resp.Metrics["accuracy"])
	}
}

func TestCloudProvider_Train_CloudFailure_FallsBackToLocal(t *testing.T) {
	local := newTestLocalProvider(t, "earthworm")
	backend := &mockCloudBackend{
		trainFn: func(ctx context.Context, req TrainRequest) (TrainResponse, error) {
			return TrainResponse{}, errors.New("connection refused")
		},
	}

	cp := NewCloudProvider(local, backend, DefaultCloudTimeout)

	resp, err := cp.Train(context.Background(), TrainRequest{
		ModuleID: "earthworm",
		Data:     [][]float32{{0.1, 0.2}},
		Labels:   []string{"normal"},
	})
	if err != nil {
		t.Fatalf("expected fallback to succeed, got error: %v", err)
	}
	// Local Train returns a no-op response with a model path.
	if resp.ModelPath == "" {
		t.Error("expected non-empty model path from local fallback")
	}
}

func TestCloudProvider_Train_CloudTimeout_FallsBackToLocal(t *testing.T) {
	local := newTestLocalProvider(t, "earthworm")
	backend := &mockCloudBackend{
		trainFn: func(ctx context.Context, req TrainRequest) (TrainResponse, error) {
			// Simulate a slow cloud operation that exceeds the timeout.
			select {
			case <-ctx.Done():
				return TrainResponse{}, ctx.Err()
			case <-time.After(10 * time.Second):
				return TrainResponse{ModelPath: "/should/not/reach"}, nil
			}
		},
	}

	// Use a very short timeout to make the test fast.
	cp := NewCloudProvider(local, backend, 50*time.Millisecond)

	resp, err := cp.Train(context.Background(), TrainRequest{
		ModuleID: "earthworm",
		Data:     [][]float32{{0.1, 0.2}},
		Labels:   []string{"normal"},
	})
	if err != nil {
		t.Fatalf("expected fallback to succeed after timeout, got error: %v", err)
	}
	if resp.ModelPath == "" {
		t.Error("expected non-empty model path from local fallback")
	}
}

func TestCloudProvider_Explain_CloudSuccess(t *testing.T) {
	local := newTestLocalProvider(t, "earthworm")
	backend := &mockCloudBackend{
		explainFn: func(ctx context.Context, req ExplainRequest) (ExplainResponse, error) {
			return ExplainResponse{
				Reasoning: "Cloud-generated explanation for module " + req.ModuleID,
				Factors: []ExplainFactor{
					{Name: "cpu_spike", Weight: 0.8, Direction: "positive"},
				},
			}, nil
		},
	}

	cp := NewCloudProvider(local, backend, DefaultCloudTimeout)

	resp, err := cp.Explain(context.Background(), ExplainRequest{
		ModuleID: "earthworm",
		Decision: PredictResponse{Score: 0.9, Confidence: 0.85, Labels: map[string]float64{"anomaly": 0.9}},
		Context:  map[string]string{"node": "worker-1"},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Reasoning != "Cloud-generated explanation for module earthworm" {
		t.Errorf("unexpected reasoning: %s", resp.Reasoning)
	}
	if len(resp.Factors) != 1 || resp.Factors[0].Name != "cpu_spike" {
		t.Errorf("unexpected factors: %+v", resp.Factors)
	}
}

func TestCloudProvider_Explain_CloudFailure_FallsBackToLocal(t *testing.T) {
	local := newTestLocalProvider(t, "earthworm")
	backend := &mockCloudBackend{
		explainFn: func(ctx context.Context, req ExplainRequest) (ExplainResponse, error) {
			return ExplainResponse{}, errors.New("service unavailable")
		},
	}

	cp := NewCloudProvider(local, backend, DefaultCloudTimeout)

	resp, err := cp.Explain(context.Background(), ExplainRequest{
		ModuleID: "earthworm",
		Decision: PredictResponse{
			Score:      0.85,
			Confidence: 0.9,
			Labels:     map[string]float64{"anomaly": 0.85, "normal": 0.15},
		},
		Context: map[string]string{"node": "worker-1"},
	})
	if err != nil {
		t.Fatalf("expected fallback to succeed, got error: %v", err)
	}
	if resp.Reasoning == "" {
		t.Error("expected non-empty reasoning from local fallback")
	}
	if len(resp.Factors) == 0 {
		t.Error("expected factors from local fallback")
	}
}

func TestCloudProvider_Explain_CloudTimeout_FallsBackToLocal(t *testing.T) {
	local := newTestLocalProvider(t, "earthworm")
	backend := &mockCloudBackend{
		explainFn: func(ctx context.Context, req ExplainRequest) (ExplainResponse, error) {
			select {
			case <-ctx.Done():
				return ExplainResponse{}, ctx.Err()
			case <-time.After(10 * time.Second):
				return ExplainResponse{Reasoning: "should not reach"}, nil
			}
		},
	}

	cp := NewCloudProvider(local, backend, 50*time.Millisecond)

	resp, err := cp.Explain(context.Background(), ExplainRequest{
		ModuleID: "earthworm",
		Decision: PredictResponse{
			Score:      0.7,
			Confidence: 0.8,
			Labels:     map[string]float64{"anomaly": 0.7, "normal": 0.3},
		},
	})
	if err != nil {
		t.Fatalf("expected fallback to succeed after timeout, got error: %v", err)
	}
	if resp.Reasoning == "" {
		t.Error("expected non-empty reasoning from local fallback")
	}
}

func TestCloudProvider_Predict_InvalidInput_ReturnsError(t *testing.T) {
	local := newTestLocalProvider(t, "earthworm")
	backend := &mockCloudBackend{}

	cp := NewCloudProvider(local, backend, DefaultCloudTimeout)

	_, err := cp.Predict(context.Background(), PredictRequest{
		ModuleID: "",
		Features: []float32{0.5},
	})
	if err == nil {
		t.Fatal("expected error for empty module ID")
	}
	var aiErr *AIError
	if !errors.As(err, &aiErr) {
		t.Fatalf("expected *AIError, got %T: %v", err, err)
	}
	if aiErr.Category != ErrInvalidInput {
		t.Errorf("expected category %s, got %s", ErrInvalidInput, aiErr.Category)
	}
}

func TestCloudProvider_Predict_CancelledContext(t *testing.T) {
	local := newTestLocalProvider(t, "earthworm")
	backend := &mockCloudBackend{}

	cp := NewCloudProvider(local, backend, DefaultCloudTimeout)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := cp.Predict(ctx, PredictRequest{
		ModuleID: "earthworm",
		Features: []float32{0.5},
	})
	if err == nil {
		t.Fatal("expected error for cancelled context")
	}
}

// --- Backend Stub Tests ---

func TestGeminiBackend_Train_NotConfigured(t *testing.T) {
	b := &GeminiBackend{ProjectID: "test-project", Region: "us-central1"}
	_, err := b.Train(context.Background(), TrainRequest{ModuleID: "earthworm"})
	if err == nil {
		t.Fatal("expected not-configured error")
	}
	var aiErr *AIError
	if !errors.As(err, &aiErr) {
		t.Fatalf("expected *AIError, got %T: %v", err, err)
	}
	if aiErr.Category != ErrCloudNotConfigured {
		t.Errorf("expected category %s, got %s", ErrCloudNotConfigured, aiErr.Category)
	}
}

func TestGeminiBackend_Explain_NotConfigured(t *testing.T) {
	b := &GeminiBackend{ProjectID: "test-project", Region: "us-central1"}
	_, err := b.Explain(context.Background(), ExplainRequest{ModuleID: "earthworm"})
	if err == nil {
		t.Fatal("expected not-configured error")
	}
	var aiErr *AIError
	if !errors.As(err, &aiErr) {
		t.Fatalf("expected *AIError, got %T: %v", err, err)
	}
	if aiErr.Category != ErrCloudNotConfigured {
		t.Errorf("expected category %s, got %s", ErrCloudNotConfigured, aiErr.Category)
	}
}

func TestBedrockBackend_Train_NotConfigured(t *testing.T) {
	b := &BedrockBackend{Region: "us-east-1", ModelID: "anthropic.claude-v2"}
	_, err := b.Train(context.Background(), TrainRequest{ModuleID: "tlapix"})
	if err == nil {
		t.Fatal("expected not-configured error")
	}
	var aiErr *AIError
	if !errors.As(err, &aiErr) {
		t.Fatalf("expected *AIError, got %T: %v", err, err)
	}
	if aiErr.Category != ErrCloudNotConfigured {
		t.Errorf("expected category %s, got %s", ErrCloudNotConfigured, aiErr.Category)
	}
}

func TestBedrockBackend_Explain_NotConfigured(t *testing.T) {
	b := &BedrockBackend{Region: "us-east-1", ModelID: "anthropic.claude-v2"}
	_, err := b.Explain(context.Background(), ExplainRequest{ModuleID: "tlapix"})
	if err == nil {
		t.Fatal("expected not-configured error")
	}
	var aiErr *AIError
	if !errors.As(err, &aiErr) {
		t.Fatalf("expected *AIError, got %T: %v", err, err)
	}
	if aiErr.Category != ErrCloudNotConfigured {
		t.Errorf("expected category %s, got %s", ErrCloudNotConfigured, aiErr.Category)
	}
}

func TestVertexBackend_Train_NotConfigured(t *testing.T) {
	b := &VertexBackend{ProjectID: "my-project", Location: "us-central1", EndpointID: "ep-123"}
	_, err := b.Train(context.Background(), TrainRequest{ModuleID: "quack"})
	if err == nil {
		t.Fatal("expected not-configured error")
	}
	var aiErr *AIError
	if !errors.As(err, &aiErr) {
		t.Fatalf("expected *AIError, got %T: %v", err, err)
	}
	if aiErr.Category != ErrCloudNotConfigured {
		t.Errorf("expected category %s, got %s", ErrCloudNotConfigured, aiErr.Category)
	}
}

func TestVertexBackend_Explain_NotConfigured(t *testing.T) {
	b := &VertexBackend{ProjectID: "my-project", Location: "us-central1", EndpointID: "ep-123"}
	_, err := b.Explain(context.Background(), ExplainRequest{ModuleID: "quack"})
	if err == nil {
		t.Fatal("expected not-configured error")
	}
	var aiErr *AIError
	if !errors.As(err, &aiErr) {
		t.Fatalf("expected *AIError, got %T: %v", err, err)
	}
	if aiErr.Category != ErrCloudNotConfigured {
		t.Errorf("expected category %s, got %s", ErrCloudNotConfigured, aiErr.Category)
	}
}

func TestSageMakerBackend_Train_NotConfigured(t *testing.T) {
	b := &SageMakerBackend{Region: "us-west-2", EndpointName: "my-endpoint"}
	_, err := b.Train(context.Background(), TrainRequest{ModuleID: "ebeecontrol"})
	if err == nil {
		t.Fatal("expected not-configured error")
	}
	var aiErr *AIError
	if !errors.As(err, &aiErr) {
		t.Fatalf("expected *AIError, got %T: %v", err, err)
	}
	if aiErr.Category != ErrCloudNotConfigured {
		t.Errorf("expected category %s, got %s", ErrCloudNotConfigured, aiErr.Category)
	}
}

func TestSageMakerBackend_Explain_NotConfigured(t *testing.T) {
	b := &SageMakerBackend{Region: "us-west-2", EndpointName: "my-endpoint"}
	_, err := b.Explain(context.Background(), ExplainRequest{ModuleID: "ebeecontrol"})
	if err == nil {
		t.Fatal("expected not-configured error")
	}
	var aiErr *AIError
	if !errors.As(err, &aiErr) {
		t.Fatalf("expected *AIError, got %T: %v", err, err)
	}
	if aiErr.Category != ErrCloudNotConfigured {
		t.Errorf("expected category %s, got %s", ErrCloudNotConfigured, aiErr.Category)
	}
}

func TestBackend_CancelledContext_Train(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	backends := []CloudBackend{
		&GeminiBackend{},
		&BedrockBackend{},
		&VertexBackend{},
		&SageMakerBackend{},
	}

	for _, b := range backends {
		_, err := b.Train(ctx, TrainRequest{ModuleID: "test"})
		if err == nil {
			t.Errorf("expected error for cancelled context with backend %T", b)
		}
	}
}

func TestBackend_CancelledContext_Explain(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	backends := []CloudBackend{
		&GeminiBackend{},
		&BedrockBackend{},
		&VertexBackend{},
		&SageMakerBackend{},
	}

	for _, b := range backends {
		_, err := b.Explain(ctx, ExplainRequest{ModuleID: "test"})
		if err == nil {
			t.Errorf("expected error for cancelled context with backend %T", b)
		}
	}
}

func TestCloudProvider_Train_WithStubBackend_FallsBack(t *testing.T) {
	local := newTestLocalProvider(t, "earthworm")
	// Use a real stub backend (not configured) to verify fallback.
	backend := &GeminiBackend{ProjectID: "test", Region: "us-central1"}

	cp := NewCloudProvider(local, backend, DefaultCloudTimeout)

	resp, err := cp.Train(context.Background(), TrainRequest{
		ModuleID: "earthworm",
		Data:     [][]float32{{0.1, 0.2}},
		Labels:   []string{"normal"},
	})
	if err != nil {
		t.Fatalf("expected fallback to succeed, got error: %v", err)
	}
	if resp.ModelPath == "" {
		t.Error("expected non-empty model path from local fallback")
	}
}

func TestCloudProvider_Explain_WithStubBackend_FallsBack(t *testing.T) {
	local := newTestLocalProvider(t, "earthworm")
	backend := &BedrockBackend{Region: "us-east-1", ModelID: "claude-v2"}

	cp := NewCloudProvider(local, backend, DefaultCloudTimeout)

	resp, err := cp.Explain(context.Background(), ExplainRequest{
		ModuleID: "earthworm",
		Decision: PredictResponse{
			Score:      0.8,
			Confidence: 0.9,
			Labels:     map[string]float64{"anomaly": 0.8, "normal": 0.2},
		},
	})
	if err != nil {
		t.Fatalf("expected fallback to succeed, got error: %v", err)
	}
	if resp.Reasoning == "" {
		t.Error("expected non-empty reasoning from local fallback")
	}
}
