// Package export provides configurable telemetry export adapters
// for Prometheus, OTLP, Splunk HEC, Dynatrace, and webhooks.
package export

import (
	"context"
	"time"
)

// Exporter manages concurrent export to all configured backends.
// Failure in one backend does not block or delay delivery to other backends.
type Exporter interface {
	// Export sends an event to all enabled backends concurrently.
	// Returns a result for each backend indicating success or failure.
	// Failure in one backend does not block others.
	Export(ctx context.Context, event Event) []ExportResult

	// BufferStatus returns current buffer utilization per backend.
	// The map key is the backend identifier (e.g., "prometheus", "splunk").
	BufferStatus() map[string]BufferInfo
}

// Config defines the unified export configuration for all backends.
// Each backend is optional and independently configured.
type Config struct {
	// Prometheus configures Prometheus exposition format export.
	Prometheus *PrometheusConfig
	// OTLP configures OpenTelemetry Protocol export.
	OTLP *OTLPConfig
	// Splunk configures Splunk HEC export.
	Splunk *SplunkConfig
	// Dynatrace configures Dynatrace API export.
	Dynatrace *DynatraceConfig
	// Webhooks configures webhook destinations for event dispatch.
	Webhooks []WebhookConfig
}

// PrometheusConfig holds configuration for the Prometheus export backend.
type PrometheusConfig struct {
	// Enabled controls whether Prometheus export is active.
	Enabled bool
	// Port is the metrics endpoint port. Defaults to 9090.
	// Must be in the range [1024, 65535].
	Port int
}

// OTLPConfig holds configuration for the OTLP export backend.
type OTLPConfig struct {
	// Enabled controls whether OTLP export is active.
	Enabled bool
	// Endpoint is the OTLP collector URL.
	Endpoint string
}

// SplunkConfig holds configuration for the Splunk HEC export backend.
type SplunkConfig struct {
	// Enabled controls whether Splunk HEC export is active.
	Enabled bool
	// HECUrl is the Splunk HTTP Event Collector URL.
	HECUrl string
	// HECToken is the authentication token for Splunk HEC.
	HECToken string
}

// DynatraceConfig holds configuration for the Dynatrace export backend.
type DynatraceConfig struct {
	// Enabled controls whether Dynatrace export is active.
	Enabled bool
	// APIUrl is the Dynatrace API endpoint URL.
	APIUrl string
	// APIToken is the authentication token for the Dynatrace API.
	APIToken string
}

// WebhookConfig holds configuration for a single webhook destination.
type WebhookConfig struct {
	// Endpoint is the webhook URL to dispatch events to.
	Endpoint string
	// Events lists severity levels to filter on (e.g., "critical", "high", "medium", "low").
	// Only events matching these severities are dispatched.
	Events []string
	// TimeoutSec is the maximum time in seconds to wait for a response.
	// Defaults to 10, maximum 10.
	TimeoutSec int
	// MaxRetries is the number of retry attempts on failure.
	// Defaults to 3.
	MaxRetries int
}

// BufferInfo represents the buffer utilization state for a single backend.
type BufferInfo struct {
	// Capacity is the maximum number of events the buffer can hold (max 1000).
	Capacity int
	// Used is the current number of events in the buffer.
	Used int
	// Dropped is the cumulative count of events discarded due to buffer overflow.
	Dropped int
}

// ExportResult represents the outcome of exporting to a single backend.
type ExportResult struct {
	// Backend identifies which export backend this result is for.
	Backend string
	// Success indicates whether the export completed without error.
	Success bool
	// Error contains the failure reason if Success is false.
	Error error
}

// Event represents a telemetry event to be exported to configured backends.
type Event struct {
	// Namespace is the Kubernetes namespace where the event originated.
	Namespace string
	// Timestamp is when the event occurred.
	Timestamp time.Time
	// Severity indicates the event's severity level (critical, high, medium, low, informational).
	Severity string
	// Module identifies which TitanOps module generated the event.
	Module string
	// EventType is a module-specific event type identifier.
	EventType string
	// Payload contains the event-specific data (max 64 KB).
	Payload []byte
	// Node is the Kubernetes node name (optional).
	Node string
	// Pod is the Kubernetes pod name (optional).
	Pod string
	// EventID is a unique identifier for this event (UUID v4).
	EventID string
	// Labels contains arbitrary key-value metadata.
	Labels map[string]string
}
