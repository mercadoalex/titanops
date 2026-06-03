package ai

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sync"
)

// Compile-time assertion that LocalProvider implements Provider.
var _ Provider = (*LocalProvider)(nil)

// ONNXSession represents a loaded ONNX model session.
// This is an abstraction layer that can be swapped for a real ONNX Runtime
// implementation when the C library is available. The stub implementation
// validates model files and performs simulated inference based on input features.
type ONNXSession struct {
	// ModelPath is the path to the loaded ONNX model file.
	ModelPath string
	// ModuleID is the module this session belongs to.
	ModuleID string
}

// Run executes inference on the session with the given input features.
// The stub implementation returns a simulated score based on the average
// of input features, normalized to [0, 1].
func (s *ONNXSession) Run(features []float32) (score float64, confidence float64, err error) {
	if len(features) == 0 {
		return 0, 0, fmt.Errorf("empty feature vector")
	}

	// Simulated inference: compute mean of features as a base score.
	var sum float64
	for _, f := range features {
		sum += float64(f)
	}
	mean := sum / float64(len(features))

	// Normalize score to [0, 1] range using sigmoid-like clamping.
	score = clamp(mean, 0, 1)

	// Confidence based on feature vector length (more features = higher confidence).
	// Caps at 0.99 for the stub.
	confidence = clamp(float64(len(features))/10.0, 0.1, 0.99)

	return score, confidence, nil
}

// Close releases resources associated with the session.
func (s *ONNXSession) Close() error {
	return nil
}

// LocalProvider implements the Provider interface using local ONNX models.
// It loads models from disk and performs inference without any network calls.
type LocalProvider struct {
	modelDir string
	mu       sync.RWMutex
	models   map[string]*ONNXSession
}

// NewLocalProvider creates a new LocalProvider that loads ONNX models from modelDir.
// Each module's model is expected at: {modelDir}/{moduleID}-anomaly.onnx
// Returns an error if the model directory does not exist.
func NewLocalProvider(modelDir string) (*LocalProvider, error) {
	info, err := os.Stat(modelDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("model directory does not exist: %s", modelDir)
		}
		return nil, fmt.Errorf("failed to access model directory: %w", err)
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("model path is not a directory: %s", modelDir)
	}

	return &LocalProvider{
		modelDir: modelDir,
		models:   make(map[string]*ONNXSession),
	}, nil
}

// Predict runs local ONNX inference for the given module.
// It never makes outbound network calls regardless of provider configuration.
// Returns typed errors for missing models, corrupt files, or invalid input.
func (lp *LocalProvider) Predict(ctx context.Context, req PredictRequest) (PredictResponse, error) {
	if req.ModuleID == "" {
		return PredictResponse{}, &AIError{
			Category: ErrInvalidInput,
			Module:   req.ModuleID,
			Message:  "module ID is required",
		}
	}

	if len(req.Features) == 0 {
		return PredictResponse{}, &AIError{
			Category: ErrInvalidInput,
			Module:   req.ModuleID,
			Message:  "feature vector must not be empty",
		}
	}

	// Check context cancellation before proceeding.
	select {
	case <-ctx.Done():
		return PredictResponse{}, ctx.Err()
	default:
	}

	session, err := lp.getOrLoadSession(req.ModuleID)
	if err != nil {
		return PredictResponse{}, err
	}

	score, confidence, err := session.Run(req.Features)
	if err != nil {
		return PredictResponse{}, &AIError{
			Category: ErrInferenceTimeout,
			Module:   req.ModuleID,
			Message:  "inference failed",
			Path:     session.ModelPath,
			Cause:    err,
		}
	}

	return PredictResponse{
		Score:      score,
		Confidence: confidence,
		Labels: map[string]float64{
			"anomaly": score,
			"normal":  1.0 - score,
		},
	}, nil
}

// Train returns a no-op response. The local provider does not support cloud training.
func (lp *LocalProvider) Train(ctx context.Context, req TrainRequest) (TrainResponse, error) {
	return TrainResponse{
		ModelPath: filepath.Join(lp.modelDir, req.ModuleID+"-anomaly.onnx"),
		Metrics:   map[string]float64{"status": 0},
	}, nil
}

// Explain generates a basic explanation from model factors without any cloud call.
// It returns a simplified explanation based on the decision's score and labels.
func (lp *LocalProvider) Explain(ctx context.Context, req ExplainRequest) (ExplainResponse, error) {
	factors := make([]ExplainFactor, 0, len(req.Decision.Labels))
	for name, weight := range req.Decision.Labels {
		direction := "positive"
		if weight < 0.5 {
			direction = "negative"
		}
		factors = append(factors, ExplainFactor{
			Name:      name,
			Weight:    weight,
			Direction: direction,
		})
	}

	reasoning := fmt.Sprintf(
		"Local model for module %q produced score %.4f with confidence %.4f. "+
			"Decision based on %d input factors.",
		req.ModuleID, req.Decision.Score, req.Decision.Confidence, len(factors),
	)

	return ExplainResponse{
		Reasoning: reasoning,
		Factors:   factors,
	}, nil
}

// getOrLoadSession retrieves a cached session or loads the model from disk.
func (lp *LocalProvider) getOrLoadSession(moduleID string) (*ONNXSession, error) {
	// Check cache first with read lock.
	lp.mu.RLock()
	session, ok := lp.models[moduleID]
	lp.mu.RUnlock()
	if ok {
		return session, nil
	}

	// Load model with write lock.
	lp.mu.Lock()
	defer lp.mu.Unlock()

	// Double-check after acquiring write lock.
	if session, ok := lp.models[moduleID]; ok {
		return session, nil
	}

	modelPath := filepath.Join(lp.modelDir, moduleID+"-anomaly.onnx")

	// Check if the model file exists.
	info, err := os.Stat(modelPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, &AIError{
				Category: ErrModelUnavailable,
				Module:   moduleID,
				Message:  "model file not found",
				Path:     modelPath,
				Cause:    err,
			}
		}
		return nil, &AIError{
			Category: ErrModelLoadFailed,
			Module:   moduleID,
			Message:  "failed to access model file",
			Path:     modelPath,
			Cause:    err,
		}
	}

	// Validate the model file is not empty (corrupt check).
	if info.Size() == 0 {
		return nil, &AIError{
			Category: ErrModelLoadFailed,
			Module:   moduleID,
			Message:  "model file is empty (corrupt)",
			Path:     modelPath,
		}
	}

	// Read a portion of the file to verify it's readable (simulates model loading).
	f, err := os.Open(modelPath)
	if err != nil {
		return nil, &AIError{
			Category: ErrModelLoadFailed,
			Module:   moduleID,
			Message:  "failed to open model file",
			Path:     modelPath,
			Cause:    err,
		}
	}
	defer f.Close()

	// Read first bytes to verify the file is readable.
	buf := make([]byte, 64)
	n, err := f.Read(buf)
	if err != nil || n == 0 {
		return nil, &AIError{
			Category: ErrModelLoadFailed,
			Module:   moduleID,
			Message:  "failed to read model file (possibly corrupt)",
			Path:     modelPath,
			Cause:    err,
		}
	}

	session = &ONNXSession{
		ModelPath: modelPath,
		ModuleID:  moduleID,
	}
	lp.models[moduleID] = session
	return session, nil
}

// clamp restricts a value to the range [min, max].
func clamp(val, min, max float64) float64 {
	if val < min {
		return min
	}
	if val > max {
		return max
	}
	return val
}
