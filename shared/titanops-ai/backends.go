package ai

import (
	"context"
	"fmt"
)

// ErrCloudTimeout is the error category for cloud provider timeout failures.
const ErrCloudTimeout ErrorCategory = "cloud_timeout"

// ErrCloudConnection is the error category for cloud provider connection failures.
const ErrCloudConnection ErrorCategory = "cloud_connection"

// ErrCloudNotConfigured is the error category for unconfigured cloud backends.
const ErrCloudNotConfigured ErrorCategory = "cloud_not_configured"

// Compile-time assertions that all backends implement CloudBackend.
var (
	_ CloudBackend = (*GeminiBackend)(nil)
	_ CloudBackend = (*BedrockBackend)(nil)
	_ CloudBackend = (*VertexBackend)(nil)
	_ CloudBackend = (*SageMakerBackend)(nil)
)

// GeminiBackend is a stub adapter for Google Gemini AI.
// It implements the CloudBackend interface but returns a "not configured" error
// until a real implementation is provided.
type GeminiBackend struct {
	// ProjectID is the Google Cloud project identifier.
	ProjectID string
	// Region is the Google Cloud region for API calls.
	Region string
}

// Train sends training data to Gemini. Returns a not-configured error in this stub.
func (g *GeminiBackend) Train(ctx context.Context, req TrainRequest) (TrainResponse, error) {
	select {
	case <-ctx.Done():
		return TrainResponse{}, ctx.Err()
	default:
	}
	return TrainResponse{}, &AIError{
		Category: ErrCloudNotConfigured,
		Module:   req.ModuleID,
		Message:  fmt.Sprintf("gemini backend not configured (project=%s, region=%s)", g.ProjectID, g.Region),
	}
}

// Explain requests an explanation from Gemini. Returns a not-configured error in this stub.
func (g *GeminiBackend) Explain(ctx context.Context, req ExplainRequest) (ExplainResponse, error) {
	select {
	case <-ctx.Done():
		return ExplainResponse{}, ctx.Err()
	default:
	}
	return ExplainResponse{}, &AIError{
		Category: ErrCloudNotConfigured,
		Module:   req.ModuleID,
		Message:  fmt.Sprintf("gemini backend not configured (project=%s, region=%s)", g.ProjectID, g.Region),
	}
}

// BedrockBackend is a stub adapter for AWS Bedrock.
// It implements the CloudBackend interface but returns a "not configured" error
// until a real implementation is provided.
type BedrockBackend struct {
	// Region is the AWS region for Bedrock API calls.
	Region string
	// ModelID is the Bedrock foundation model identifier.
	ModelID string
}

// Train sends training data to Bedrock. Returns a not-configured error in this stub.
func (b *BedrockBackend) Train(ctx context.Context, req TrainRequest) (TrainResponse, error) {
	select {
	case <-ctx.Done():
		return TrainResponse{}, ctx.Err()
	default:
	}
	return TrainResponse{}, &AIError{
		Category: ErrCloudNotConfigured,
		Module:   req.ModuleID,
		Message:  fmt.Sprintf("bedrock backend not configured (region=%s, model=%s)", b.Region, b.ModelID),
	}
}

// Explain requests an explanation from Bedrock. Returns a not-configured error in this stub.
func (b *BedrockBackend) Explain(ctx context.Context, req ExplainRequest) (ExplainResponse, error) {
	select {
	case <-ctx.Done():
		return ExplainResponse{}, ctx.Err()
	default:
	}
	return ExplainResponse{}, &AIError{
		Category: ErrCloudNotConfigured,
		Module:   req.ModuleID,
		Message:  fmt.Sprintf("bedrock backend not configured (region=%s, model=%s)", b.Region, b.ModelID),
	}
}

// VertexBackend is a stub adapter for Google Vertex AI.
// It implements the CloudBackend interface but returns a "not configured" error
// until a real implementation is provided.
type VertexBackend struct {
	// ProjectID is the Google Cloud project identifier.
	ProjectID string
	// Location is the Vertex AI location (e.g., "us-central1").
	Location string
	// EndpointID is the Vertex AI endpoint identifier.
	EndpointID string
}

// Train sends training data to Vertex AI. Returns a not-configured error in this stub.
func (v *VertexBackend) Train(ctx context.Context, req TrainRequest) (TrainResponse, error) {
	select {
	case <-ctx.Done():
		return TrainResponse{}, ctx.Err()
	default:
	}
	return TrainResponse{}, &AIError{
		Category: ErrCloudNotConfigured,
		Module:   req.ModuleID,
		Message:  fmt.Sprintf("vertex backend not configured (project=%s, location=%s)", v.ProjectID, v.Location),
	}
}

// Explain requests an explanation from Vertex AI. Returns a not-configured error in this stub.
func (v *VertexBackend) Explain(ctx context.Context, req ExplainRequest) (ExplainResponse, error) {
	select {
	case <-ctx.Done():
		return ExplainResponse{}, ctx.Err()
	default:
	}
	return ExplainResponse{}, &AIError{
		Category: ErrCloudNotConfigured,
		Module:   req.ModuleID,
		Message:  fmt.Sprintf("vertex backend not configured (project=%s, location=%s)", v.ProjectID, v.Location),
	}
}

// SageMakerBackend is a stub adapter for AWS SageMaker.
// It implements the CloudBackend interface but returns a "not configured" error
// until a real implementation is provided.
type SageMakerBackend struct {
	// Region is the AWS region for SageMaker API calls.
	Region string
	// EndpointName is the SageMaker endpoint name.
	EndpointName string
}

// Train sends training data to SageMaker. Returns a not-configured error in this stub.
func (s *SageMakerBackend) Train(ctx context.Context, req TrainRequest) (TrainResponse, error) {
	select {
	case <-ctx.Done():
		return TrainResponse{}, ctx.Err()
	default:
	}
	return TrainResponse{}, &AIError{
		Category: ErrCloudNotConfigured,
		Module:   req.ModuleID,
		Message:  fmt.Sprintf("sagemaker backend not configured (region=%s, endpoint=%s)", s.Region, s.EndpointName),
	}
}

// Explain requests an explanation from SageMaker. Returns a not-configured error in this stub.
func (s *SageMakerBackend) Explain(ctx context.Context, req ExplainRequest) (ExplainResponse, error) {
	select {
	case <-ctx.Done():
		return ExplainResponse{}, ctx.Err()
	default:
	}
	return ExplainResponse{}, &AIError{
		Category: ErrCloudNotConfigured,
		Module:   req.ModuleID,
		Message:  fmt.Sprintf("sagemaker backend not configured (region=%s, endpoint=%s)", s.Region, s.EndpointName),
	}
}
