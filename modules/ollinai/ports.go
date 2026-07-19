package ollinai

import "context"

// ErrorCategory represents a typed error category for API and adapter failures.
type ErrorCategory string

// Error category constants for the OllinAI adapter.
const (
	ErrOllinAPIUnavailable ErrorCategory = "ollinai_api_unavailable"
	ErrOllinAPITimeout     ErrorCategory = "ollinai_api_timeout"
	ErrOllinAPIAuth        ErrorCategory = "ollinai_auth_failed"
)

// OllinAPIClient is the interface boundary between the Poller engine and the
// OllinAI REST API infrastructure. The engine depends on this abstraction;
// infrastructure adapters (HTTP client, mock client) implement it.
//
// This follows the architecture discipline: engine packages must not import
// infrastructure packages (net/http, etc). All HTTP details live behind this interface.
type OllinAPIClient interface {
	// FetchDeploymentRisks retrieves the current deployment risk entries from the
	// OllinAI API. Returns a slice of risk entries or an error with category.
	FetchDeploymentRisks(ctx context.Context) ([]DeploymentRiskEntry, error)

	// FetchDORAMetrics retrieves the current DORA metrics from the OllinAI API.
	// Returns the metrics or an error with category.
	FetchDORAMetrics(ctx context.Context) (*DORAMetrics, error)
}

// DeploymentRiskEntry represents a single deployment risk data point
// returned by the OllinAI API. This is the domain type used by the engine;
// it is decoupled from the wire format (JSON struct tags live in the adapter).
type DeploymentRiskEntry struct {
	Service     string
	CommitSHA   string
	Deployer    string
	RiskScore   int
	RiskFactors []string
	PipelineID  string
	Environment string
	Node        string
	Pod         string
	Namespace   string
}

// DORAMetrics represents the four DORA metrics returned by the OllinAI API.
type DORAMetrics struct {
	DeploymentFrequency  float64
	LeadTimeForChanges   float64
	ChangeFailureRate    float64
	TimeToRestoreService float64
}

// APIError is a typed error returned by OllinAPIClient implementations.
// It carries the error category so the engine can report it without knowing
// about HTTP status codes or network-level details.
type APIError struct {
	// Category classifies the failure for metrics and health reporting.
	Category ErrorCategory
	// Message is a human-readable description of what went wrong.
	Message string
	// Cause is the underlying error, if any.
	Cause error
}

func (e *APIError) Error() string {
	if e.Cause != nil {
		return e.Message + ": " + e.Cause.Error()
	}
	return e.Message
}

func (e *APIError) Unwrap() error {
	return e.Cause
}
