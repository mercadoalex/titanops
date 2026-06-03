// Package testutil provides shared rapid-based generators for property testing
// across all TitanOps modules. These generators produce random but structurally
// valid (or intentionally invalid) test data for events, configs, and feature vectors.
package testutil

import (
	"fmt"
	"time"

	"pgregory.net/rapid"
)

// Severity levels as defined in the TitanOps event schema.
var Severities = []string{"critical", "high", "medium", "low", "informational"}

// Modules as defined in the TitanOps platform.
var Modules = []string{"tlapix", "earthworm", "ebeecontrol", "quack", "correlation"}

// RemediationActions are the autonomous actions Earthworm can take.
var RemediationActions = []string{"pod_restart", "node_cordon", "workload_reschedule"}

// --- Severity Generators ---

// Severity generates a valid severity level string.
func Severity() *rapid.Generator[string] {
	return rapid.SampledFrom(Severities)
}

// SeveritySet generates a non-empty subset of severity levels for webhook filtering.
func SeveritySet() *rapid.Generator[[]string] {
	return rapid.Custom[[]string](func(t *rapid.T) []string {
		n := rapid.IntRange(1, len(Severities)).Draw(t, "severitySetSize")
		indices := rapid.SliceOfN(rapid.IntRange(0, len(Severities)-1), n, n).Draw(t, "severityIndices")
		seen := make(map[string]bool)
		var result []string
		for _, i := range indices {
			s := Severities[i]
			if !seen[s] {
				seen[s] = true
				result = append(result, s)
			}
		}
		if len(result) == 0 {
			result = append(result, Severities[0])
		}
		return result
	})
}

// --- Module Generators ---

// Module generates a valid module identifier string.
func Module() *rapid.Generator[string] {
	return rapid.SampledFrom(Modules)
}

// ModuleSubset generates a subset of module identifiers with at least minCount entries.
func ModuleSubset(minCount int) *rapid.Generator[[]string] {
	return rapid.Custom[[]string](func(t *rapid.T) []string {
		max := len(Modules)
		if minCount > max {
			max = minCount
		}
		n := rapid.IntRange(minCount, max).Draw(t, "moduleSubsetSize")
		// Shuffle and pick first n
		perm := rapid.SliceOfN(rapid.IntRange(0, len(Modules)-1), n, n).Draw(t, "moduleSubsetIndices")
		seen := make(map[string]bool)
		var result []string
		for _, i := range perm {
			m := Modules[i%len(Modules)]
			if !seen[m] {
				seen[m] = true
				result = append(result, m)
			}
		}
		for len(result) < minCount {
			for _, m := range Modules {
				if !seen[m] {
					seen[m] = true
					result = append(result, m)
					break
				}
			}
		}
		return result
	})
}

// --- Feature Vector Generators ---

// FeatureVector generates a []float32 of the specified length with values in [0, 1].
func FeatureVector(length int) *rapid.Generator[[]float32] {
	return rapid.Custom[[]float32](func(t *rapid.T) []float32 {
		vec := make([]float32, length)
		for i := range vec {
			vec[i] = rapid.Float32Range(0, 1).Draw(t, fmt.Sprintf("feature[%d]", i))
		}
		return vec
	})
}

// FeatureVectorVariable generates a []float32 with length between minLen and maxLen.
func FeatureVectorVariable(minLen, maxLen int) *rapid.Generator[[]float32] {
	return rapid.Custom[[]float32](func(t *rapid.T) []float32 {
		n := rapid.IntRange(minLen, maxLen).Draw(t, "featureVectorLen")
		vec := make([]float32, n)
		for i := range vec {
			vec[i] = rapid.Float32Range(0, 1).Draw(t, fmt.Sprintf("feature[%d]", i))
		}
		return vec
	})
}

// FeatureVectorAny generates a []float32 with arbitrary float values (including negatives).
func FeatureVectorAny(minLen, maxLen int) *rapid.Generator[[]float32] {
	return rapid.Custom[[]float32](func(t *rapid.T) []float32 {
		n := rapid.IntRange(minLen, maxLen).Draw(t, "featureVectorLen")
		vec := make([]float32, n)
		for i := range vec {
			vec[i] = rapid.Float32Range(-1000, 1000).Draw(t, fmt.Sprintf("feature[%d]", i))
		}
		return vec
	})
}

// --- Event Generators ---

// Event represents a TitanOps platform event for testing.
// This mirrors the export.Event type without creating a dependency.
type Event struct {
	Namespace string
	Timestamp time.Time
	Severity  string
	Module    string
	EventType string
	Payload   []byte
	Node      string
	Pod       string
	EventID   string
	Labels    map[string]string
}

// ValidEvent generates a structurally valid Event with all required fields populated.
func ValidEvent() *rapid.Generator[Event] {
	return rapid.Custom[Event](func(t *rapid.T) Event {
		return Event{
			Namespace: Namespace().Draw(t, "namespace"),
			Timestamp: Timestamp().Draw(t, "timestamp"),
			Severity:  Severity().Draw(t, "severity"),
			Module:    Module().Draw(t, "module"),
			EventType: EventType().Draw(t, "eventType"),
			Payload:   ValidPayload().Draw(t, "payload"),
			Node:      KubernetesName().Draw(t, "node"),
			Pod:       KubernetesName().Draw(t, "pod"),
			EventID:   UUID().Draw(t, "eventID"),
			Labels:    Labels().Draw(t, "labels"),
		}
	})
}

// InvalidEvent generates an Event with at least one required field missing or invalid.
func InvalidEvent() *rapid.Generator[Event] {
	return rapid.Custom[Event](func(t *rapid.T) Event {
		ev := ValidEvent().Draw(t, "baseEvent")
		// Randomly invalidate one required field
		field := rapid.IntRange(0, 5).Draw(t, "invalidField")
		switch field {
		case 0:
			ev.Namespace = ""
		case 1:
			ev.Timestamp = time.Time{} // zero time
		case 2:
			ev.Severity = ""
		case 3:
			ev.Module = ""
		case 4:
			ev.EventType = ""
		case 5:
			ev.Payload = nil
		}
		return ev
	})
}

// OversizedEvent generates an Event with payload exceeding 64KB.
func OversizedEvent() *rapid.Generator[Event] {
	return rapid.Custom[Event](func(t *rapid.T) Event {
		ev := ValidEvent().Draw(t, "baseEvent")
		// Generate payload between 64KB+1 and 128KB
		size := rapid.IntRange(65537, 131072).Draw(t, "oversizedPayloadSize")
		ev.Payload = make([]byte, size)
		for i := range ev.Payload {
			ev.Payload[i] = byte(rapid.IntRange(0, 255).Draw(t, fmt.Sprintf("byte[%d]", i)))
		}
		return ev
	})
}

// --- Config Generators ---

// PortConfig represents a port configuration for testing.
type PortConfig struct {
	Port int
}

// ValidPort generates a port number in the valid range [1024, 65535].
func ValidPort() *rapid.Generator[int] {
	return rapid.IntRange(1024, 65535)
}

// InvalidPort generates a port number outside the valid range [1024, 65535].
func InvalidPort() *rapid.Generator[int] {
	return rapid.Custom[int](func(t *rapid.T) int {
		// Either below 1024 or above 65535
		if rapid.Bool().Draw(t, "lowPort") {
			return rapid.IntRange(0, 1023).Draw(t, "port")
		}
		return rapid.IntRange(65536, 100000).Draw(t, "port")
	})
}

// AnyPort generates any integer that may or may not be a valid port.
func AnyPort() *rapid.Generator[int] {
	return rapid.IntRange(-1000, 100000)
}

// ExportConfig represents a complete export configuration for testing.
type ExportConfig struct {
	PrometheusEnabled bool
	PrometheusPort    int
	OTLPEnabled       bool
	OTLPEndpoint      string
	SplunkEnabled     bool
	SplunkHECUrl      string
	SplunkHECToken    string
	DynatraceEnabled  bool
	DynatraceAPIUrl   string
	DynatraceAPIToken string
	WebhookEndpoints  []string
	WebhookSeverities []string
}

// ValidExportConfig generates a valid export configuration.
func ValidExportConfig() *rapid.Generator[ExportConfig] {
	return rapid.Custom[ExportConfig](func(t *rapid.T) ExportConfig {
		return ExportConfig{
			PrometheusEnabled: rapid.Bool().Draw(t, "promEnabled"),
			PrometheusPort:    ValidPort().Draw(t, "promPort"),
			OTLPEnabled:       rapid.Bool().Draw(t, "otlpEnabled"),
			OTLPEndpoint:      Endpoint().Draw(t, "otlpEndpoint"),
			SplunkEnabled:     rapid.Bool().Draw(t, "splunkEnabled"),
			SplunkHECUrl:      Endpoint().Draw(t, "splunkUrl"),
			SplunkHECToken:    Token().Draw(t, "splunkToken"),
			DynatraceEnabled:  rapid.Bool().Draw(t, "dtEnabled"),
			DynatraceAPIUrl:   Endpoint().Draw(t, "dtUrl"),
			DynatraceAPIToken: Token().Draw(t, "dtToken"),
			WebhookEndpoints:  rapid.SliceOfN(Endpoint(), 0, 3).Draw(t, "webhookEndpoints"),
			WebhookSeverities: SeveritySet().Draw(t, "webhookSeverities"),
		}
	})
}

// CorrelationConfig represents a correlation engine configuration for testing.
type CorrelationConfig struct {
	TimeWindowSec       int
	ConfidenceThreshold int
	AutoActionsEnabled  bool
}

// ValidCorrelationConfig generates a valid correlation engine configuration.
func ValidCorrelationConfig() *rapid.Generator[CorrelationConfig] {
	return rapid.Custom[CorrelationConfig](func(t *rapid.T) CorrelationConfig {
		return CorrelationConfig{
			TimeWindowSec:       rapid.IntRange(10, 600).Draw(t, "timeWindow"),
			ConfidenceThreshold: rapid.IntRange(1, 100).Draw(t, "confidenceThreshold"),
			AutoActionsEnabled:  rapid.Bool().Draw(t, "autoActions"),
		}
	})
}

// InvalidCorrelationConfig generates a correlation config with at least one out-of-range field.
func InvalidCorrelationConfig() *rapid.Generator[CorrelationConfig] {
	return rapid.Custom[CorrelationConfig](func(t *rapid.T) CorrelationConfig {
		cfg := ValidCorrelationConfig().Draw(t, "baseCfg")
		field := rapid.IntRange(0, 1).Draw(t, "invalidField")
		switch field {
		case 0:
			// TimeWindow out of [10, 600]
			if rapid.Bool().Draw(t, "lowWindow") {
				cfg.TimeWindowSec = rapid.IntRange(0, 9).Draw(t, "timeWindow")
			} else {
				cfg.TimeWindowSec = rapid.IntRange(601, 10000).Draw(t, "timeWindow")
			}
		case 1:
			// ConfidenceThreshold out of [1, 100]
			if rapid.Bool().Draw(t, "lowThreshold") {
				cfg.ConfidenceThreshold = rapid.IntRange(-100, 0).Draw(t, "threshold")
			} else {
				cfg.ConfidenceThreshold = rapid.IntRange(101, 1000).Draw(t, "threshold")
			}
		}
		return cfg
	})
}

// --- Primitive Generators ---

// Namespace generates a valid Kubernetes namespace name.
func Namespace() *rapid.Generator[string] {
	return rapid.Custom[string](func(t *rapid.T) string {
		prefixes := []string{"default", "kube-system", "titanops", "monitoring", "production", "staging"}
		return rapid.SampledFrom(prefixes).Draw(t, "namespace")
	})
}

// Timestamp generates a UTC timestamp with millisecond precision within a reasonable range.
func Timestamp() *rapid.Generator[time.Time] {
	return rapid.Custom[time.Time](func(t *rapid.T) time.Time {
		// Generate timestamps within the last year
		baseTime := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
		offsetMs := rapid.Int64Range(0, 365*24*60*60*1000).Draw(t, "timestampOffsetMs")
		return baseTime.Add(time.Duration(offsetMs) * time.Millisecond)
	})
}

// EventType generates a module-specific event type identifier.
func EventType() *rapid.Generator[string] {
	return rapid.Custom[string](func(t *rapid.T) string {
		types := []string{
			"node_anomaly_detected",
			"cert_expiry_warning",
			"honeytoken_accessed",
			"scheduling_decision",
			"heartbeat_timeout",
			"pod_restart_initiated",
			"correlation_incident",
			"threshold_breach",
		}
		return rapid.SampledFrom(types).Draw(t, "eventType")
	})
}

// ValidPayload generates a byte payload within the 64KB limit.
func ValidPayload() *rapid.Generator[[]byte] {
	return rapid.Custom[[]byte](func(t *rapid.T) []byte {
		size := rapid.IntRange(1, 65536).Draw(t, "payloadSize")
		return make([]byte, size)
	})
}

// KubernetesName generates a valid Kubernetes resource name (DNS subdomain).
func KubernetesName() *rapid.Generator[string] {
	return rapid.Custom[string](func(t *rapid.T) string {
		names := []string{
			"node-01", "node-02", "worker-a", "worker-b",
			"app-pod-xyz", "web-frontend-1", "api-backend-2",
			"monitoring-agent", "titanops-earthworm-0",
		}
		return rapid.SampledFrom(names).Draw(t, "k8sName")
	})
}

// UUID generates a UUID v4 string.
func UUID() *rapid.Generator[string] {
	return rapid.Custom[string](func(t *rapid.T) string {
		b := make([]byte, 16)
		for i := range b {
			b[i] = byte(rapid.IntRange(0, 255).Draw(t, fmt.Sprintf("uuidByte[%d]", i)))
		}
		// Set UUID version 4 and variant bits
		b[6] = (b[6] & 0x0f) | 0x40
		b[8] = (b[8] & 0x3f) | 0x80
		return fmt.Sprintf("%08x-%04x-%04x-%04x-%012x",
			b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
	})
}

// Labels generates a map of arbitrary key-value metadata labels.
func Labels() *rapid.Generator[map[string]string] {
	return rapid.Custom[map[string]string](func(t *rapid.T) map[string]string {
		n := rapid.IntRange(0, 5).Draw(t, "labelCount")
		labels := make(map[string]string, n)
		keys := []string{"app", "env", "team", "version", "component", "tier"}
		values := []string{"titanops", "production", "staging", "v1", "backend", "frontend", "sre"}
		for i := 0; i < n; i++ {
			k := rapid.SampledFrom(keys).Draw(t, fmt.Sprintf("labelKey[%d]", i))
			v := rapid.SampledFrom(values).Draw(t, fmt.Sprintf("labelVal[%d]", i))
			labels[k] = v
		}
		return labels
	})
}

// Endpoint generates a plausible endpoint URL string.
func Endpoint() *rapid.Generator[string] {
	return rapid.Custom[string](func(t *rapid.T) string {
		hosts := []string{"localhost", "collector.local", "monitor.internal", "otel.svc.cluster.local"}
		ports := []int{4317, 8088, 9090, 443, 8080}
		host := rapid.SampledFrom(hosts).Draw(t, "host")
		port := rapid.SampledFrom(ports).Draw(t, "port")
		return fmt.Sprintf("https://%s:%d", host, port)
	})
}

// Token generates a random token string for authentication.
func Token() *rapid.Generator[string] {
	return rapid.Custom[string](func(t *rapid.T) string {
		length := rapid.IntRange(16, 64).Draw(t, "tokenLen")
		chars := "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
		token := make([]byte, length)
		for i := range token {
			token[i] = chars[rapid.IntRange(0, len(chars)-1).Draw(t, fmt.Sprintf("tokenChar[%d]", i))]
		}
		return string(token)
	})
}

// --- Confidence and Threshold Generators ---

// ConfidenceScore generates a confidence score in [0.0, 1.0].
func ConfidenceScore() *rapid.Generator[float64] {
	return rapid.Float64Range(0.0, 1.0)
}

// ConfidenceScore100 generates a confidence score in [0, 100] (integer).
func ConfidenceScore100() *rapid.Generator[int] {
	return rapid.IntRange(0, 100)
}

// Threshold generates a valid threshold in [0.1, 1.0] for Earthworm configuration.
func Threshold() *rapid.Generator[float64] {
	return rapid.Float64Range(0.1, 1.0)
}

// --- Remediation Action Generators ---

// RemediationAction generates a valid remediation action type.
func RemediationAction() *rapid.Generator[string] {
	return rapid.SampledFrom(RemediationActions)
}
