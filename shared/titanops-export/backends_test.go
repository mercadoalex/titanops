package export

import (
	"context"
	"encoding/json"
	"log"
	"os"
	"strings"
	"testing"
	"time"
)

func sampleEvent() Event {
	return Event{
		Namespace: "production",
		Timestamp: time.Date(2024, 6, 15, 10, 30, 0, 0, time.UTC),
		Severity:  "high",
		Module:    "earthworm",
		EventType: "anomaly_detected",
		Payload:   []byte(`{"score": 0.95, "node": "worker-1"}`),
		Node:      "worker-1",
		Pod:       "earthworm-agent-abc123",
		EventID:   "evt-001-uuid",
		Labels:    map[string]string{"cluster": "prod-us-east", "team": "sre"},
	}
}

// --- PrometheusBackend tests ---

func TestPrometheusBackend_Name(t *testing.T) {
	b := NewPrometheusBackend(&PrometheusConfig{Enabled: true, Port: 9090})
	if b.Name() != "prometheus" {
		t.Errorf("expected name 'prometheus', got %q", b.Name())
	}
}

func TestPrometheusBackend_IsEnabled(t *testing.T) {
	tests := []struct {
		name     string
		config   *PrometheusConfig
		expected bool
	}{
		{"nil config", nil, false},
		{"disabled", &PrometheusConfig{Enabled: false}, false},
		{"enabled", &PrometheusConfig{Enabled: true, Port: 9090}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b := NewPrometheusBackend(tt.config)
			if b.IsEnabled() != tt.expected {
				t.Errorf("expected IsEnabled=%v, got %v", tt.expected, b.IsEnabled())
			}
		})
	}
}

func TestPrometheusBackend_Send(t *testing.T) {
	b := NewPrometheusBackend(&PrometheusConfig{Enabled: true, Port: 9090})
	err := b.Send(context.Background(), sampleEvent())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestFormatPrometheus_Output(t *testing.T) {
	event := sampleEvent()
	output, err := FormatPrometheus(event)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify HELP line
	if !strings.Contains(output, "# HELP titanops_earthworm_anomaly_detected_total") {
		t.Error("missing HELP line")
	}
	// Verify TYPE line
	if !strings.Contains(output, "# TYPE titanops_earthworm_anomaly_detected_total counter") {
		t.Error("missing TYPE line")
	}
	// Verify metric line with labels
	if !strings.Contains(output, `namespace="production"`) {
		t.Error("missing namespace label")
	}
	if !strings.Contains(output, `severity="high"`) {
		t.Error("missing severity label")
	}
	if !strings.Contains(output, `module="earthworm"`) {
		t.Error("missing module label")
	}
	if !strings.Contains(output, `node="worker-1"`) {
		t.Error("missing node label")
	}
	if !strings.Contains(output, `pod="earthworm-agent-abc123"`) {
		t.Error("missing pod label")
	}
}

func TestFormatPrometheus_SanitizesMetricName(t *testing.T) {
	event := sampleEvent()
	event.Module = "my-module"
	event.EventType = "some.event/type"

	output, err := FormatPrometheus(event)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify special chars are replaced with underscore
	if !strings.Contains(output, "titanops_my_module_some_event_type_total") {
		t.Errorf("metric name not properly sanitized: %s", output)
	}
}

// --- OTLPBackend tests ---

func TestOTLPBackend_Name(t *testing.T) {
	b := NewOTLPBackend(&OTLPConfig{Enabled: true, Endpoint: "http://localhost:4318"})
	if b.Name() != "otlp" {
		t.Errorf("expected name 'otlp', got %q", b.Name())
	}
}

func TestOTLPBackend_IsEnabled(t *testing.T) {
	tests := []struct {
		name     string
		config   *OTLPConfig
		expected bool
	}{
		{"nil config", nil, false},
		{"disabled", &OTLPConfig{Enabled: false}, false},
		{"enabled", &OTLPConfig{Enabled: true, Endpoint: "http://localhost:4318"}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b := NewOTLPBackend(tt.config)
			if b.IsEnabled() != tt.expected {
				t.Errorf("expected IsEnabled=%v, got %v", tt.expected, b.IsEnabled())
			}
		})
	}
}

func TestOTLPBackend_Send(t *testing.T) {
	b := NewOTLPBackend(&OTLPConfig{Enabled: true, Endpoint: "http://localhost:4318"})
	err := b.Send(context.Background(), sampleEvent())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestFormatOTLP_Output(t *testing.T) {
	event := sampleEvent()
	data, err := FormatOTLP(event)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var record otlpLogRecord
	if err := json.Unmarshal(data, &record); err != nil {
		t.Fatalf("failed to unmarshal OTLP JSON: %v", err)
	}

	if record.TimeUnixNano != event.Timestamp.UnixNano() {
		t.Errorf("expected timeUnixNano=%d, got %d", event.Timestamp.UnixNano(), record.TimeUnixNano)
	}
	if record.SeverityNumber != 17 { // high -> ERROR
		t.Errorf("expected severityNumber=17, got %d", record.SeverityNumber)
	}
	if record.SeverityText != "HIGH" {
		t.Errorf("expected severityText='HIGH', got %q", record.SeverityText)
	}

	// Verify resource attributes
	found := false
	for _, attr := range record.Resource.Attributes {
		if attr.Key == "service.name" && attr.Value.StringValue == "titanops-earthworm" {
			found = true
		}
	}
	if !found {
		t.Error("missing service.name resource attribute")
	}
}

func TestFormatOTLP_SeverityMapping(t *testing.T) {
	tests := []struct {
		severity string
		number   int
	}{
		{"critical", 21},
		{"high", 17},
		{"medium", 13},
		{"low", 9},
		{"informational", 5},
		{"unknown", 0},
	}
	for _, tt := range tests {
		t.Run(tt.severity, func(t *testing.T) {
			event := sampleEvent()
			event.Severity = tt.severity
			data, err := FormatOTLP(event)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			var record otlpLogRecord
			if err := json.Unmarshal(data, &record); err != nil {
				t.Fatalf("failed to unmarshal: %v", err)
			}
			if record.SeverityNumber != tt.number {
				t.Errorf("severity %q: expected number %d, got %d", tt.severity, tt.number, record.SeverityNumber)
			}
		})
	}
}

// --- SplunkBackend tests ---

func TestSplunkBackend_Name(t *testing.T) {
	b := NewSplunkBackend(&SplunkConfig{Enabled: true, HECUrl: "http://localhost:8088", HECToken: "test"})
	if b.Name() != "splunk" {
		t.Errorf("expected name 'splunk', got %q", b.Name())
	}
}

func TestSplunkBackend_IsEnabled(t *testing.T) {
	tests := []struct {
		name     string
		config   *SplunkConfig
		expected bool
	}{
		{"nil config", nil, false},
		{"disabled", &SplunkConfig{Enabled: false}, false},
		{"enabled", &SplunkConfig{Enabled: true, HECUrl: "http://localhost:8088", HECToken: "tok"}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b := NewSplunkBackend(tt.config)
			if b.IsEnabled() != tt.expected {
				t.Errorf("expected IsEnabled=%v, got %v", tt.expected, b.IsEnabled())
			}
		})
	}
}

func TestSplunkBackend_Send(t *testing.T) {
	b := NewSplunkBackend(&SplunkConfig{Enabled: true, HECUrl: "http://localhost:8088", HECToken: "tok"})
	err := b.Send(context.Background(), sampleEvent())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestFormatSplunkHEC_Output(t *testing.T) {
	event := sampleEvent()
	data, err := FormatSplunkHEC(event)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var hecEvent splunkHECEvent
	if err := json.Unmarshal(data, &hecEvent); err != nil {
		t.Fatalf("failed to unmarshal Splunk HEC JSON: %v", err)
	}

	if hecEvent.Source != "titanops:earthworm" {
		t.Errorf("expected source 'titanops:earthworm', got %q", hecEvent.Source)
	}
	if hecEvent.SourceType != "_json" {
		t.Errorf("expected sourcetype '_json', got %q", hecEvent.SourceType)
	}
	if hecEvent.Host != "worker-1" {
		t.Errorf("expected host 'worker-1', got %q", hecEvent.Host)
	}

	// Verify event fields
	eventData := hecEvent.Event
	if eventData["event_id"] != "evt-001-uuid" {
		t.Errorf("expected event_id 'evt-001-uuid', got %v", eventData["event_id"])
	}
	if eventData["severity"] != "high" {
		t.Errorf("expected severity 'high', got %v", eventData["severity"])
	}
}

// --- DynatraceBackend tests ---

func TestDynatraceBackend_Name(t *testing.T) {
	b := NewDynatraceBackend(&DynatraceConfig{Enabled: true, APIUrl: "http://localhost", APIToken: "tok"})
	if b.Name() != "dynatrace" {
		t.Errorf("expected name 'dynatrace', got %q", b.Name())
	}
}

func TestDynatraceBackend_IsEnabled(t *testing.T) {
	tests := []struct {
		name     string
		config   *DynatraceConfig
		expected bool
	}{
		{"nil config", nil, false},
		{"disabled", &DynatraceConfig{Enabled: false}, false},
		{"enabled", &DynatraceConfig{Enabled: true, APIUrl: "http://dt.local", APIToken: "tok"}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b := NewDynatraceBackend(tt.config)
			if b.IsEnabled() != tt.expected {
				t.Errorf("expected IsEnabled=%v, got %v", tt.expected, b.IsEnabled())
			}
		})
	}
}

func TestDynatraceBackend_Send(t *testing.T) {
	b := NewDynatraceBackend(&DynatraceConfig{Enabled: true, APIUrl: "http://dt.local", APIToken: "tok"})
	err := b.Send(context.Background(), sampleEvent())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestFormatDynatrace_Output(t *testing.T) {
	event := sampleEvent()
	data, err := FormatDynatrace(event)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var dtEvent dynatraceEvent
	if err := json.Unmarshal(data, &dtEvent); err != nil {
		t.Fatalf("failed to unmarshal Dynatrace JSON: %v", err)
	}

	if dtEvent.EventType != "ERROR_EVENT" { // high severity
		t.Errorf("expected eventType 'ERROR_EVENT', got %q", dtEvent.EventType)
	}
	if !strings.Contains(dtEvent.Title, "earthworm") {
		t.Error("title should contain module name")
	}
	if !strings.Contains(dtEvent.Title, "high") {
		t.Error("title should contain severity")
	}
	if dtEvent.Properties["node"] != "worker-1" {
		t.Errorf("expected node property 'worker-1', got %q", dtEvent.Properties["node"])
	}
	if dtEvent.Properties["label.cluster"] != "prod-us-east" {
		t.Errorf("expected label.cluster property, got %q", dtEvent.Properties["label.cluster"])
	}
}

func TestFormatDynatrace_SeverityMapping(t *testing.T) {
	tests := []struct {
		severity  string
		eventType string
	}{
		{"critical", "AVAILABILITY_EVENT"},
		{"high", "ERROR_EVENT"},
		{"medium", "PERFORMANCE_EVENT"},
		{"low", "RESOURCE_CONTENTION_EVENT"},
		{"informational", "CUSTOM_INFO"},
		{"unknown", "CUSTOM_INFO"},
	}
	for _, tt := range tests {
		t.Run(tt.severity, func(t *testing.T) {
			event := sampleEvent()
			event.Severity = tt.severity
			data, err := FormatDynatrace(event)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			var dtEvent dynatraceEvent
			if err := json.Unmarshal(data, &dtEvent); err != nil {
				t.Fatalf("failed to unmarshal: %v", err)
			}
			if dtEvent.EventType != tt.eventType {
				t.Errorf("severity %q: expected %q, got %q", tt.severity, tt.eventType, dtEvent.EventType)
			}
		})
	}
}

// --- WebhookBackend tests ---

func TestWebhookBackend_Name(t *testing.T) {
	b := NewWebhookBackend(WebhookConfig{Endpoint: "https://hooks.example.com/alert"}, nil)
	expected := "webhook:https://hooks.example.com/alert"
	if b.Name() != expected {
		t.Errorf("expected name %q, got %q", expected, b.Name())
	}
}

func TestWebhookBackend_IsEnabled(t *testing.T) {
	tests := []struct {
		name     string
		endpoint string
		expected bool
	}{
		{"empty endpoint", "", false},
		{"valid endpoint", "https://hooks.example.com/alert", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b := NewWebhookBackend(WebhookConfig{Endpoint: tt.endpoint}, nil)
			if b.IsEnabled() != tt.expected {
				t.Errorf("expected IsEnabled=%v, got %v", tt.expected, b.IsEnabled())
			}
		})
	}
}

func TestWebhookBackend_SeverityFiltering(t *testing.T) {
	logger := log.New(os.Stderr, "", 0)
	b := NewWebhookBackend(WebhookConfig{
		Endpoint: "https://hooks.example.com/alert",
		Events:   []string{"critical", "high"},
	}, logger)

	tests := []struct {
		severity string
		filtered bool // true means event should be filtered out (not dispatched)
	}{
		{"critical", false},
		{"high", false},
		{"medium", true},
		{"low", true},
		{"informational", true},
	}

	for _, tt := range tests {
		t.Run(tt.severity, func(t *testing.T) {
			if b.matchesSeverity(tt.severity) == tt.filtered {
				if tt.filtered {
					t.Errorf("severity %q should be filtered out", tt.severity)
				} else {
					t.Errorf("severity %q should pass filter", tt.severity)
				}
			}
		})
	}
}

func TestWebhookBackend_NoFilterPassesAll(t *testing.T) {
	b := NewWebhookBackend(WebhookConfig{
		Endpoint: "https://hooks.example.com/alert",
		Events:   nil, // no filter
	}, nil)

	severities := []string{"critical", "high", "medium", "low", "informational"}
	for _, s := range severities {
		if !b.matchesSeverity(s) {
			t.Errorf("with no filter, severity %q should pass", s)
		}
	}
}

func TestWebhookBackend_Timeout(t *testing.T) {
	tests := []struct {
		name       string
		timeoutSec int
		expected   time.Duration
	}{
		{"default", 0, 10 * time.Second},
		{"configured", 5, 5 * time.Second},
		{"over max", 15, 10 * time.Second},
		{"negative", -1, 10 * time.Second},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b := NewWebhookBackend(WebhookConfig{
				Endpoint:   "https://hooks.example.com",
				TimeoutSec: tt.timeoutSec,
			}, nil)
			if b.Timeout() != tt.expected {
				t.Errorf("expected timeout %v, got %v", tt.expected, b.Timeout())
			}
		})
	}
}

func TestWebhookBackend_MaxRetries(t *testing.T) {
	tests := []struct {
		name       string
		maxRetries int
		expected   int
	}{
		{"default", 0, 3},
		{"configured", 5, 5},
		{"negative", -1, 3},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b := NewWebhookBackend(WebhookConfig{
				Endpoint:   "https://hooks.example.com",
				MaxRetries: tt.maxRetries,
			}, nil)
			if b.MaxRetries() != tt.expected {
				t.Errorf("expected maxRetries %d, got %d", tt.expected, b.MaxRetries())
			}
		})
	}
}

func TestWebhookBackend_Send_FilteredEvent(t *testing.T) {
	b := NewWebhookBackend(WebhookConfig{
		Endpoint: "https://hooks.example.com/alert",
		Events:   []string{"critical"},
	}, nil)

	// Low severity should be filtered out - no error
	event := sampleEvent()
	event.Severity = "low"
	err := b.Send(context.Background(), event)
	if err != nil {
		t.Fatalf("filtered event should not return error, got: %v", err)
	}
}

func TestWebhookBackend_Send_PassingEvent(t *testing.T) {
	b := NewWebhookBackend(WebhookConfig{
		Endpoint: "https://hooks.example.com/alert",
		Events:   []string{"critical", "high"},
	}, nil)

	event := sampleEvent()
	event.Severity = "high"
	err := b.Send(context.Background(), event)
	if err != nil {
		t.Fatalf("passing event should not return error, got: %v", err)
	}
}

func TestFormatWebhook_Output(t *testing.T) {
	event := sampleEvent()
	data, err := FormatWebhook(event)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var payload webhookPayload
	if err := json.Unmarshal(data, &payload); err != nil {
		t.Fatalf("failed to unmarshal webhook JSON: %v", err)
	}

	if payload.EventID != "evt-001-uuid" {
		t.Errorf("expected event_id 'evt-001-uuid', got %q", payload.EventID)
	}
	if payload.Namespace != "production" {
		t.Errorf("expected namespace 'production', got %q", payload.Namespace)
	}
	if payload.Severity != "high" {
		t.Errorf("expected severity 'high', got %q", payload.Severity)
	}
	if payload.Module != "earthworm" {
		t.Errorf("expected module 'earthworm', got %q", payload.Module)
	}
	if payload.Node != "worker-1" {
		t.Errorf("expected node 'worker-1', got %q", payload.Node)
	}
	if payload.Labels["cluster"] != "prod-us-east" {
		t.Errorf("expected label cluster 'prod-us-east', got %q", payload.Labels["cluster"])
	}

	// Verify timestamp is UTC RFC3339
	if !strings.Contains(payload.Timestamp, "2024-06-15T10:30:00") {
		t.Errorf("timestamp not in expected format: %q", payload.Timestamp)
	}
}

func TestWebhookBackend_CaseInsensitiveSeverity(t *testing.T) {
	b := NewWebhookBackend(WebhookConfig{
		Endpoint: "https://hooks.example.com/alert",
		Events:   []string{"Critical", "HIGH"},
	}, nil)

	if !b.matchesSeverity("critical") {
		t.Error("should match 'critical' case-insensitively")
	}
	if !b.matchesSeverity("CRITICAL") {
		t.Error("should match 'CRITICAL' case-insensitively")
	}
	if !b.matchesSeverity("high") {
		t.Error("should match 'high' case-insensitively")
	}
	if b.matchesSeverity("medium") {
		t.Error("should not match 'medium'")
	}
}
