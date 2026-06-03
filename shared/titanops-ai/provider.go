// Package ai provides unified AI inference and training abstraction
// supporting local ONNX models and optional cloud AI providers.
package ai

import "context"

// Provider defines the unified AI interface all backends implement.
// It supports local-first inference with optional cloud delegation
// for training and explanation operations.
type Provider interface {
	// Predict runs local ONNX inference for the given module.
	// It never makes outbound network calls regardless of provider configuration.
	Predict(ctx context.Context, req PredictRequest) (PredictResponse, error)

	// Train delegates model training to a cloud backend.
	// Returns a no-op response for local-only providers.
	Train(ctx context.Context, req TrainRequest) (TrainResponse, error)

	// Explain generates human-readable reasoning for a prediction.
	// May use cloud backends when configured; falls back to local on failure.
	Explain(ctx context.Context, req ExplainRequest) (ExplainResponse, error)
}

// PredictRequest contains the input for a local ONNX prediction.
type PredictRequest struct {
	// ModuleID identifies which module's model to use (e.g., "earthworm", "tlapix").
	ModuleID string
	// Features is the input feature vector for the model.
	Features []float32
}

// PredictResponse contains the result of a local ONNX prediction.
type PredictResponse struct {
	// Score is the primary anomaly/prediction score.
	Score float64
	// Confidence indicates the model's certainty in the prediction.
	Confidence float64
	// Labels maps label names to their predicted probabilities.
	Labels map[string]float64
}

// TrainRequest contains the input for a model training operation.
type TrainRequest struct {
	// ModuleID identifies which module's model to train.
	ModuleID string
	// Data is the training feature matrix (rows of feature vectors).
	Data [][]float32
	// Labels are the ground truth labels for each data row.
	Labels []string
}

// TrainResponse contains the result of a model training operation.
type TrainResponse struct {
	// ModelPath is the file path where the trained model was saved.
	ModelPath string
	// Metrics contains training metrics (e.g., accuracy, loss).
	Metrics map[string]float64
}

// ExplainRequest contains the input for generating an explanation.
type ExplainRequest struct {
	// ModuleID identifies which module generated the decision.
	ModuleID string
	// Decision is the prediction response to explain.
	Decision PredictResponse
	// Context provides additional key-value context for explanation generation.
	Context map[string]string
}

// ExplainResponse contains the human-readable explanation of a decision.
type ExplainResponse struct {
	// Reasoning is a human-readable summary of why the decision was made.
	Reasoning string
	// Factors lists the individual factors contributing to the decision.
	Factors []ExplainFactor
}

// ExplainFactor represents a single contributing factor to an AI decision.
type ExplainFactor struct {
	// Name identifies the factor (e.g., "cpu_usage", "memory_pressure").
	Name string
	// Weight indicates how strongly this factor influenced the decision.
	Weight float64
	// Direction indicates whether the factor contributed positively or negatively.
	// Valid values are "positive" or "negative".
	Direction string
}

// CloudBackend is implemented by each cloud provider adapter (Gemini, Bedrock,
// Vertex AI, SageMaker). It handles training and explanation operations that
// may be delegated to cloud services.
type CloudBackend interface {
	// Train sends training data to the cloud provider and returns the result.
	Train(ctx context.Context, req TrainRequest) (TrainResponse, error)

	// Explain requests an explanation from the cloud provider for a given decision.
	Explain(ctx context.Context, req ExplainRequest) (ExplainResponse, error)
}
