package ai

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestNewLocalProvider_ValidDirectory(t *testing.T) {
	dir := t.TempDir()
	p, err := NewLocalProvider(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if p == nil {
		t.Fatal("expected non-nil provider")
	}
	if p.modelDir != dir {
		t.Errorf("expected modelDir=%s, got %s", dir, p.modelDir)
	}
}

func TestNewLocalProvider_NonExistentDirectory(t *testing.T) {
	_, err := NewLocalProvider("/nonexistent/path/that/does/not/exist")
	if err == nil {
		t.Fatal("expected error for non-existent directory")
	}
}

func TestNewLocalProvider_FileNotDirectory(t *testing.T) {
	f, err := os.CreateTemp("", "notadir")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(f.Name())
	f.Close()

	_, err = NewLocalProvider(f.Name())
	if err == nil {
		t.Fatal("expected error when path is a file, not a directory")
	}
}

func TestPredict_ModelUnavailable(t *testing.T) {
	dir := t.TempDir()
	p, err := NewLocalProvider(dir)
	if err != nil {
		t.Fatal(err)
	}

	resp, err := p.Predict(context.Background(), PredictRequest{
		ModuleID: "nonexistent",
		Features: []float32{0.5, 0.6},
	})
	if err == nil {
		t.Fatal("expected error for missing model")
	}

	var aiErr *AIError
	if !errors.As(err, &aiErr) {
		t.Fatalf("expected *AIError, got %T: %v", err, err)
	}
	if aiErr.Category != ErrModelUnavailable {
		t.Errorf("expected category %s, got %s", ErrModelUnavailable, aiErr.Category)
	}
	if aiErr.Module != "nonexistent" {
		t.Errorf("expected module=nonexistent, got %s", aiErr.Module)
	}
	if resp.Score != 0 {
		t.Errorf("expected zero score on error, got %f", resp.Score)
	}
}

func TestPredict_ModelLoadFailed_EmptyFile(t *testing.T) {
	dir := t.TempDir()
	// Create an empty file to simulate a corrupt model.
	modelPath := filepath.Join(dir, "corrupt-anomaly.onnx")
	if err := os.WriteFile(modelPath, []byte{}, 0644); err != nil {
		t.Fatal(err)
	}

	p, err := NewLocalProvider(dir)
	if err != nil {
		t.Fatal(err)
	}

	_, err = p.Predict(context.Background(), PredictRequest{
		ModuleID: "corrupt",
		Features: []float32{0.5},
	})
	if err == nil {
		t.Fatal("expected error for empty model file")
	}

	var aiErr *AIError
	if !errors.As(err, &aiErr) {
		t.Fatalf("expected *AIError, got %T: %v", err, err)
	}
	if aiErr.Category != ErrModelLoadFailed {
		t.Errorf("expected category %s, got %s", ErrModelLoadFailed, aiErr.Category)
	}
}

func TestPredict_Success(t *testing.T) {
	dir := t.TempDir()
	// Create a valid model file with some content.
	modelPath := filepath.Join(dir, "earthworm-anomaly.onnx")
	content := []byte("ONNX_STUB_MODEL_DATA_FOR_TESTING_PURPOSES_ONLY")
	if err := os.WriteFile(modelPath, content, 0644); err != nil {
		t.Fatal(err)
	}

	p, err := NewLocalProvider(dir)
	if err != nil {
		t.Fatal(err)
	}

	resp, err := p.Predict(context.Background(), PredictRequest{
		ModuleID: "earthworm",
		Features: []float32{0.3, 0.5, 0.7},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if resp.Score < 0 || resp.Score > 1 {
		t.Errorf("score out of range [0,1]: %f", resp.Score)
	}
	if resp.Confidence < 0 || resp.Confidence > 1 {
		t.Errorf("confidence out of range [0,1]: %f", resp.Confidence)
	}
	if resp.Labels == nil {
		t.Error("expected non-nil labels map")
	}
	if _, ok := resp.Labels["anomaly"]; !ok {
		t.Error("expected 'anomaly' label in response")
	}
	if _, ok := resp.Labels["normal"]; !ok {
		t.Error("expected 'normal' label in response")
	}
}

func TestPredict_InvalidInput_EmptyModuleID(t *testing.T) {
	dir := t.TempDir()
	p, err := NewLocalProvider(dir)
	if err != nil {
		t.Fatal(err)
	}

	_, err = p.Predict(context.Background(), PredictRequest{
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

func TestPredict_InvalidInput_EmptyFeatures(t *testing.T) {
	dir := t.TempDir()
	p, err := NewLocalProvider(dir)
	if err != nil {
		t.Fatal(err)
	}

	_, err = p.Predict(context.Background(), PredictRequest{
		ModuleID: "earthworm",
		Features: []float32{},
	})
	if err == nil {
		t.Fatal("expected error for empty features")
	}

	var aiErr *AIError
	if !errors.As(err, &aiErr) {
		t.Fatalf("expected *AIError, got %T: %v", err, err)
	}
	if aiErr.Category != ErrInvalidInput {
		t.Errorf("expected category %s, got %s", ErrInvalidInput, aiErr.Category)
	}
}

func TestPredict_CancelledContext(t *testing.T) {
	dir := t.TempDir()
	modelPath := filepath.Join(dir, "test-anomaly.onnx")
	if err := os.WriteFile(modelPath, []byte("model_data"), 0644); err != nil {
		t.Fatal(err)
	}

	p, err := NewLocalProvider(dir)
	if err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately.

	_, err = p.Predict(ctx, PredictRequest{
		ModuleID: "test",
		Features: []float32{0.5},
	})
	if err == nil {
		t.Fatal("expected error for cancelled context")
	}
}

func TestPredict_SessionCaching(t *testing.T) {
	dir := t.TempDir()
	modelPath := filepath.Join(dir, "cached-anomaly.onnx")
	if err := os.WriteFile(modelPath, []byte("model_data_for_cache_test"), 0644); err != nil {
		t.Fatal(err)
	}

	p, err := NewLocalProvider(dir)
	if err != nil {
		t.Fatal(err)
	}

	// First call loads the session.
	_, err = p.Predict(context.Background(), PredictRequest{
		ModuleID: "cached",
		Features: []float32{0.5, 0.6},
	})
	if err != nil {
		t.Fatalf("first predict failed: %v", err)
	}

	// Second call should use the cached session.
	_, err = p.Predict(context.Background(), PredictRequest{
		ModuleID: "cached",
		Features: []float32{0.7, 0.8},
	})
	if err != nil {
		t.Fatalf("second predict failed: %v", err)
	}

	// Verify the session is cached.
	p.mu.RLock()
	_, ok := p.models["cached"]
	p.mu.RUnlock()
	if !ok {
		t.Error("expected session to be cached")
	}
}

func TestTrain_NoOp(t *testing.T) {
	dir := t.TempDir()
	p, err := NewLocalProvider(dir)
	if err != nil {
		t.Fatal(err)
	}

	resp, err := p.Train(context.Background(), TrainRequest{
		ModuleID: "earthworm",
		Data:     [][]float32{{0.1, 0.2}, {0.3, 0.4}},
		Labels:   []string{"normal", "anomaly"},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.ModelPath == "" {
		t.Error("expected non-empty model path")
	}
}

func TestExplain_LocalBasic(t *testing.T) {
	dir := t.TempDir()
	p, err := NewLocalProvider(dir)
	if err != nil {
		t.Fatal(err)
	}

	resp, err := p.Explain(context.Background(), ExplainRequest{
		ModuleID: "earthworm",
		Decision: PredictResponse{
			Score:      0.85,
			Confidence: 0.9,
			Labels: map[string]float64{
				"anomaly": 0.85,
				"normal":  0.15,
			},
		},
		Context: map[string]string{"node": "worker-1"},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Reasoning == "" {
		t.Error("expected non-empty reasoning")
	}
	if len(resp.Factors) == 0 {
		t.Error("expected at least one factor")
	}
}

func TestAIError_Error(t *testing.T) {
	err := &AIError{
		Category: ErrModelUnavailable,
		Module:   "earthworm",
		Message:  "model file not found",
		Path:     "/models/earthworm-anomaly.onnx",
	}
	s := err.Error()
	if s == "" {
		t.Error("expected non-empty error string")
	}
}

func TestAIError_Unwrap(t *testing.T) {
	cause := errors.New("underlying cause")
	err := &AIError{
		Category: ErrModelLoadFailed,
		Module:   "test",
		Message:  "failed",
		Path:     "/path",
		Cause:    cause,
	}
	if !errors.Is(err, cause) {
		t.Error("expected Unwrap to return the cause")
	}
}

func TestAIError_ErrorWithCause(t *testing.T) {
	cause := errors.New("file permission denied")
	err := &AIError{
		Category: ErrModelLoadFailed,
		Module:   "earthworm",
		Message:  "failed to open model file",
		Path:     "/models/earthworm-anomaly.onnx",
		Cause:    cause,
	}
	s := err.Error()
	if s == "" {
		t.Error("expected non-empty error string")
	}
}
