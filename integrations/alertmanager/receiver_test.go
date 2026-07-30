package alertmanager

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	export "github.com/mercadoalex/titanops/shared/titanops-export"
)

// --- Test Helpers ---

// mockIngester records all ingested events for assertions.
type mockIngester struct {
	mu     sync.Mutex
	events []export.Event
	err    error // if set, Ingest returns this error
}

func (m *mockIngester) Ingest(_ context.Context, event export.Event) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.err != nil {
		return m.err
	}
	m.events = append(m.events, event)
	return nil
}

func (m *mockIngester) Events() []export.Event {
	m.mu.Lock()
	defer m.mu.Unlock()
	cp := make([]export.Event, len(m.events))
	copy(cp, m.events)
	return cp
}

// samplePayload returns a realistic AlertManager webhook payload.
func samplePayload() AlertManagerPayload {
	return AlertManagerPayload{
		Version:  "4",
		Receiver: "titanops-webhook",
		Status:   "firing",
		Alerts: []Alert{
			{
				Status: "firing",
				Labels: map[string]string{
					"alertname": "HighMemoryUsage",
					"severity":  "critical",
					"namespace": "production",
					"pod":       "api-server-7b4c",
					"node":      "worker-03",
					"job":       "kubelet",
				},
				Annotations: map[string]string{
					"summary":     "Memory usage above 90% on api-server-7b4c",
					"description": "Pod api-server-7b4c in namespace production is using 94% memory.",
				},
				StartsAt:     time.Date(2026, 7, 22, 10, 0, 0, 0, time.UTC),
				EndsAt:       time.Time{},
				GeneratorURL: "http://prometheus:9090/graph?g0.expr=container_memory_usage_bytes",
				Fingerprint:  "abc123def456",
			},
			{
				Status: "firing",
				Labels: map[string]string{
					"alertname": "NodeCPUHigh",
					"severity":  "warning",
					"namespace": "kube-system",
					"instance":  "worker-03:9100",
					"job":       "node-exporter",
				},
				Annotations: map[string]string{
					"summary": "CPU usage above 85% on worker-03",
				},
				StartsAt:    time.Date(2026, 7, 22, 10, 0, 30, 0, time.UTC),
				Fingerprint: "def789ghi012",
			},
		},
		GroupLabels:       map[string]string{"alertname": "HighMemoryUsage"},
		CommonLabels:      map[string]string{"job": "kubelet"},
		CommonAnnotations: map[string]string{},
		ExternalURL:       "http://alertmanager:9093",
		GroupKey:          "{}:{alertname=\"HighMemoryUsage\"}",
	}
}

func payloadJSON(t *testing.T, p AlertManagerPayload) []byte {
	t.Helper()
	data, err := json.Marshal(p)
	if err != nil {
		t.Fatalf("failed to marshal payload: %v", err)
	}
	return data
}

// --- Mapping Tests ---

func TestNormalizeEventType(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"HighMemoryUsage", "high_memory_usage"},
		{"NodeCPUHigh", "node_cpuhigh"},
		{"simple", "simple"},
		{"KubePodCrashLooping", "kube_pod_crash_looping"},
		{"", ""},
		{"already_snake_case", "already_snake_case"},
		{"With Spaces", "with_spaces"},
		{"with-dashes", "with_dashes"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := normalizeEventType(tt.input)
			if got != tt.expected {
				t.Errorf("normalizeEventType(%q) = %q, want %q", tt.input, got, tt.expected)
			}
		})
	}
}

func TestMapSeverity(t *testing.T) {
	tests := []struct {
		label    string
		status   string
		expected string
	}{
		{"critical", "firing", "critical"},
		{"page", "firing", "critical"},
		{"emergency", "firing", "critical"},
		{"warning", "firing", "high"},
		{"high", "firing", "high"},
		{"error", "firing", "high"},
		{"medium", "firing", "medium"},
		{"info", "firing", "informational"},
		{"informational", "firing", "informational"},
		{"low", "resolved", "informational"},
		{"none", "firing", "informational"},
		{"", "firing", "high"},             // No label, firing → high
		{"", "resolved", "informational"},  // No label, resolved → informational
		{"CRITICAL", "firing", "critical"}, // Case insensitive
	}

	for _, tt := range tests {
		name := tt.label + "/" + tt.status
		if tt.label == "" {
			name = "(empty)/" + tt.status
		}
		t.Run(name, func(t *testing.T) {
			got := mapSeverity(tt.label, tt.status)
			if got != tt.expected {
				t.Errorf("mapSeverity(%q, %q) = %q, want %q", tt.label, tt.status, got, tt.expected)
			}
		})
	}
}

func TestExtractNode(t *testing.T) {
	tests := []struct {
		name     string
		labels   map[string]string
		expected string
	}{
		{"direct node label", map[string]string{"node": "worker-03"}, "worker-03"},
		{"node_name label", map[string]string{"node_name": "worker-05"}, "worker-05"},
		{"from instance", map[string]string{"instance": "worker-07:9100"}, "worker-07"},
		{"instance without port", map[string]string{"instance": "worker-07"}, "worker-07"},
		{"node takes precedence", map[string]string{"node": "a", "instance": "b:9100"}, "a"},
		{"empty labels", map[string]string{}, ""},
		{"nil labels", nil, ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := extractNode(tt.labels)
			if got != tt.expected {
				t.Errorf("extractNode(%v) = %q, want %q", tt.labels, got, tt.expected)
			}
		})
	}
}

func TestMapAlertToEvent(t *testing.T) {
	alert := Alert{
		Status: "firing",
		Labels: map[string]string{
			"alertname": "HighMemoryUsage",
			"severity":  "critical",
			"namespace": "production",
			"pod":       "api-server-7b4c",
			"node":      "worker-03",
		},
		Annotations: map[string]string{
			"summary": "Memory above 90%",
		},
		StartsAt:    time.Date(2026, 7, 22, 10, 0, 0, 0, time.UTC),
		Fingerprint: "abc123",
	}

	event, err := MapAlertToEvent(alert, "test-receiver")
	if err != nil {
		t.Fatalf("MapAlertToEvent returned error: %v", err)
	}

	// Check core fields.
	if event.Module != ModuleID {
		t.Errorf("Module = %q, want %q", event.Module, ModuleID)
	}
	if event.EventType != "high_memory_usage" {
		t.Errorf("EventType = %q, want %q", event.EventType, "high_memory_usage")
	}
	if event.Severity != "critical" {
		t.Errorf("Severity = %q, want %q", event.Severity, "critical")
	}
	if event.Namespace != "production" {
		t.Errorf("Namespace = %q, want %q", event.Namespace, "production")
	}
	if event.Node != "worker-03" {
		t.Errorf("Node = %q, want %q", event.Node, "worker-03")
	}
	if event.Pod != "api-server-7b4c" {
		t.Errorf("Pod = %q, want %q", event.Pod, "api-server-7b4c")
	}
	if !event.Timestamp.Equal(alert.StartsAt) {
		t.Errorf("Timestamp = %v, want %v", event.Timestamp, alert.StartsAt)
	}
	if event.EventID == "" {
		t.Error("EventID should not be empty")
	}

	// Check labels.
	if event.Labels["source"] != "alertmanager" {
		t.Errorf("Labels[source] = %q, want %q", event.Labels["source"], "alertmanager")
	}
	if event.Labels["receiver"] != "test-receiver" {
		t.Errorf("Labels[receiver] = %q, want %q", event.Labels["receiver"], "test-receiver")
	}
	if event.Labels["fingerprint"] != "abc123" {
		t.Errorf("Labels[fingerprint] = %q, want %q", event.Labels["fingerprint"], "abc123")
	}
	if event.Labels["annotation_summary"] != "Memory above 90%" {
		t.Errorf("Labels[annotation_summary] = %q, want %q", event.Labels["annotation_summary"], "Memory above 90%")
	}

	// Check payload is valid JSON.
	var raw map[string]interface{}
	if err := json.Unmarshal(event.Payload, &raw); err != nil {
		t.Errorf("Payload is not valid JSON: %v", err)
	}
}

func TestMapAlertToEvent_MissingAlertname(t *testing.T) {
	alert := Alert{
		Status: "firing",
		Labels: map[string]string{
			"severity": "warning",
		},
		StartsAt: time.Date(2026, 7, 22, 10, 0, 0, 0, time.UTC),
	}

	event, err := MapAlertToEvent(alert, "test")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if event.EventType != "unknown_alert" {
		t.Errorf("EventType = %q, want %q", event.EventType, "unknown_alert")
	}
}

func TestMapPayloadToEvents_FiltersResolved(t *testing.T) {
	payload := AlertManagerPayload{
		Receiver: "test",
		Alerts: []Alert{
			{Status: "firing", Labels: map[string]string{"alertname": "A"}, StartsAt: time.Now()},
			{Status: "resolved", Labels: map[string]string{"alertname": "B"}, StartsAt: time.Now()},
			{Status: "firing", Labels: map[string]string{"alertname": "C"}, StartsAt: time.Now()},
		},
	}

	// Without resolved.
	events, err := MapPayloadToEvents(payload, false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(events) != 2 {
		t.Errorf("expected 2 events (firing only), got %d", len(events))
	}

	// With resolved.
	events, err = MapPayloadToEvents(payload, true)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(events) != 3 {
		t.Errorf("expected 3 events (all), got %d", len(events))
	}
}

// --- Receiver HTTP Tests ---

func newTestReceiver(t *testing.T, ingester EventIngester) *Receiver {
	t.Helper()
	cfg := DefaultReceiverConfig()
	r, err := NewReceiver(cfg, ingester)
	if err != nil {
		t.Fatalf("NewReceiver failed: %v", err)
	}
	return r
}

func TestReceiver_HandleWebhook_Success(t *testing.T) {
	ingester := &mockIngester{}
	r := newTestReceiver(t, ingester)

	payload := samplePayload()
	body := payloadJSON(t, payload)

	req := httptest.NewRequest(http.MethodPost, "/webhooks/alertmanager", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	r.handleWebhook(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", rec.Code, rec.Body.String())
	}

	// Check response body.
	var resp webhookResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to parse response: %v", err)
	}
	if resp.Status != "ok" {
		t.Errorf("response status = %q, want %q", resp.Status, "ok")
	}
	if resp.Ingested != 2 {
		t.Errorf("response ingested = %d, want %d", resp.Ingested, 2)
	}

	// Check ingested events.
	events := ingester.Events()
	if len(events) != 2 {
		t.Fatalf("expected 2 ingested events, got %d", len(events))
	}

	// First event should be HighMemoryUsage.
	if events[0].EventType != "high_memory_usage" {
		t.Errorf("events[0].EventType = %q, want %q", events[0].EventType, "high_memory_usage")
	}
	if events[0].Severity != "critical" {
		t.Errorf("events[0].Severity = %q, want %q", events[0].Severity, "critical")
	}
	if events[0].Node != "worker-03" {
		t.Errorf("events[0].Node = %q, want %q", events[0].Node, "worker-03")
	}

	// Second event — node extracted from instance label.
	if events[1].EventType != "node_cpuhigh" {
		t.Errorf("events[1].EventType = %q, want %q", events[1].EventType, "node_cpuhigh")
	}
	if events[1].Node != "worker-03" {
		t.Errorf("events[1].Node = %q, want %q (from instance)", events[1].Node, "worker-03")
	}

	// Check metrics.
	stats := r.Stats()
	if stats.EventsReceived != 1 {
		t.Errorf("EventsReceived = %d, want 1", stats.EventsReceived)
	}
	if stats.EventsMapped != 2 {
		t.Errorf("EventsMapped = %d, want 2", stats.EventsMapped)
	}
	if stats.EventsIngested != 2 {
		t.Errorf("EventsIngested = %d, want 2", stats.EventsIngested)
	}
}

func TestReceiver_HandleWebhook_MethodNotAllowed(t *testing.T) {
	ingester := &mockIngester{}
	r := newTestReceiver(t, ingester)

	req := httptest.NewRequest(http.MethodGet, "/webhooks/alertmanager", nil)
	rec := httptest.NewRecorder()

	r.handleWebhook(rec, req)

	if rec.Code != http.StatusMethodNotAllowed {
		t.Errorf("expected status 405, got %d", rec.Code)
	}
}

func TestReceiver_HandleWebhook_InvalidJSON(t *testing.T) {
	ingester := &mockIngester{}
	r := newTestReceiver(t, ingester)

	req := httptest.NewRequest(http.MethodPost, "/webhooks/alertmanager",
		strings.NewReader("not json at all"))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	r.handleWebhook(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected status 400, got %d", rec.Code)
	}

	stats := r.Stats()
	if stats.Errors != 1 {
		t.Errorf("Errors = %d, want 1", stats.Errors)
	}
}

func TestReceiver_HandleWebhook_EmptyAlerts(t *testing.T) {
	ingester := &mockIngester{}
	r := newTestReceiver(t, ingester)

	payload := AlertManagerPayload{
		Receiver: "test",
		Alerts:   []Alert{},
	}
	body := payloadJSON(t, payload)

	req := httptest.NewRequest(http.MethodPost, "/webhooks/alertmanager", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	r.handleWebhook(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", rec.Code)
	}

	events := ingester.Events()
	if len(events) != 0 {
		t.Errorf("expected 0 events, got %d", len(events))
	}
}

func TestReceiver_HandleWebhook_WrongContentType(t *testing.T) {
	ingester := &mockIngester{}
	r := newTestReceiver(t, ingester)

	req := httptest.NewRequest(http.MethodPost, "/webhooks/alertmanager",
		strings.NewReader("{}"))
	req.Header.Set("Content-Type", "text/plain")
	rec := httptest.NewRecorder()

	r.handleWebhook(rec, req)

	if rec.Code != http.StatusUnsupportedMediaType {
		t.Errorf("expected status 415, got %d", rec.Code)
	}
}

func TestReceiver_HandleWebhook_PayloadTooLarge(t *testing.T) {
	ingester := &mockIngester{}
	cfg := DefaultReceiverConfig()
	cfg.MaxPayloadBytes = 50 // Tiny limit.
	r, err := NewReceiver(cfg, ingester)
	if err != nil {
		t.Fatal(err)
	}

	payload := samplePayload()
	body := payloadJSON(t, payload) // Will be > 50 bytes.

	req := httptest.NewRequest(http.MethodPost, "/webhooks/alertmanager", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	r.handleWebhook(rec, req)

	if rec.Code != http.StatusRequestEntityTooLarge {
		t.Errorf("expected status 413, got %d", rec.Code)
	}
}

func TestReceiver_HandleWebhook_IngesterError(t *testing.T) {
	ingester := &mockIngester{err: context.DeadlineExceeded}
	r := newTestReceiver(t, ingester)

	payload := samplePayload()
	body := payloadJSON(t, payload)

	req := httptest.NewRequest(http.MethodPost, "/webhooks/alertmanager", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	r.handleWebhook(rec, req)

	// Should still return 200 (partial success) but with 0 ingested.
	if rec.Code != http.StatusOK {
		t.Errorf("expected status 200 (partial success), got %d", rec.Code)
	}

	var resp webhookResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to parse response: %v", err)
	}
	if resp.Ingested != 0 {
		t.Errorf("expected 0 ingested (all failed), got %d", resp.Ingested)
	}

	stats := r.Stats()
	if stats.Errors != 2 { // 2 alerts, both fail.
		t.Errorf("Errors = %d, want 2", stats.Errors)
	}
}

func TestReceiver_Healthz(t *testing.T) {
	ingester := &mockIngester{}
	r := newTestReceiver(t, ingester)

	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	rec := httptest.NewRecorder()

	r.handleHealthz(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", rec.Code)
	}

	var resp map[string]string
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to parse response: %v", err)
	}
	if resp["status"] != "healthy" {
		t.Errorf("status = %q, want %q", resp["status"], "healthy")
	}
}

func TestNewReceiver_NilIngester(t *testing.T) {
	cfg := DefaultReceiverConfig()
	_, err := NewReceiver(cfg, nil)
	if err == nil {
		t.Error("expected error for nil ingester, got nil")
	}
}
