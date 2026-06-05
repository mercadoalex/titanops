package ollinai

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	export "github.com/mercadoalex/titanops/shared/titanops-export"
)

// mockEmitter is a test double for EventEmitter that records emitted events.
type mockEmitter struct {
	events    []export.Event
	failCount int // number of times to return an error before succeeding
	callCount int
}

func (m *mockEmitter) Emit(_ context.Context, event export.Event) error {
	m.callCount++
	if m.failCount > 0 {
		m.failCount--
		return fmt.Errorf("emit failed (remaining failures: %d)", m.failCount)
	}
	m.events = append(m.events, event)
	return nil
}

func (m *mockEmitter) Flush(_ context.Context) error { return nil }
func (m *mockEmitter) BufferLen() int                { return len(m.events) }

func newTestWebhookServer(hmacKey string, emitter EventEmitter) *WebhookServer {
	return NewWebhookServer(WebhookServerConfig{
		Port:    0, // not used in tests with httptest
		Emitter: emitter,
		HMACKey: hmacKey,
	})
}

func computeHMAC(body []byte, key string) string {
	mac := hmac.New(sha256.New, []byte(key))
	mac.Write(body)
	return hex.EncodeToString(mac.Sum(nil))
}

func TestWebhook_ValidCredentialExfil(t *testing.T) {
	emitter := &mockEmitter{}
	ws := newTestWebhookServer("secret-key", emitter)

	body := `{
		"event_type": "credential_exfil",
		"node": "runner-node-01",
		"timestamp": "2024-06-15T10:30:00.000Z",
		"pipeline_id": "pipe-123",
		"step_name": "npm install",
		"repository": "org/repo",
		"description": "Detected credential exfiltration",
		"evidence": "process tree: npm → curl → exfil.io"
	}`

	sig := computeHMAC([]byte(body), "secret-key")

	req := httptest.NewRequest(http.MethodPost, WebhookPath, strings.NewReader(body))
	req.Header.Set(WebhookSignatureHeader, sig)
	rr := httptest.NewRecorder()

	ws.handleWebhook(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}
	if len(emitter.events) != 1 {
		t.Fatalf("expected 1 event emitted, got %d", len(emitter.events))
	}

	event := emitter.events[0]
	if event.Module != ModuleName {
		t.Errorf("expected Module=%q, got %q", ModuleName, event.Module)
	}
	if event.EventType != EventTypeSupplyChainCredentialExfil {
		t.Errorf("expected EventType=%q, got %q", EventTypeSupplyChainCredentialExfil, event.EventType)
	}
	if event.Severity != SeverityCritical {
		t.Errorf("expected Severity=%q, got %q", SeverityCritical, event.Severity)
	}
	if event.Node != "runner-node-01" {
		t.Errorf("expected Node=%q, got %q", "runner-node-01", event.Node)
	}
	if event.Labels[LabelPipelineID] != "pipe-123" {
		t.Errorf("expected label pipeline_id=%q, got %q", "pipe-123", event.Labels[LabelPipelineID])
	}
	if event.Labels[LabelStepName] != "npm install" {
		t.Errorf("expected label step_name=%q, got %q", "npm install", event.Labels[LabelStepName])
	}
	if event.Labels[LabelRepository] != "org/repo" {
		t.Errorf("expected label repository=%q, got %q", "org/repo", event.Labels[LabelRepository])
	}

	// Verify payload deserialization
	var payload SupplyChainPayload
	if err := json.Unmarshal(event.Payload, &payload); err != nil {
		t.Fatalf("failed to unmarshal payload: %v", err)
	}
	if payload.Description != "Detected credential exfiltration" {
		t.Errorf("unexpected description: %q", payload.Description)
	}
}

func TestWebhook_ProcessAnomaly(t *testing.T) {
	emitter := &mockEmitter{}
	ws := newTestWebhookServer("", emitter) // no HMAC key = skip verification

	body := `{
		"event_type": "process_anomaly",
		"node": "runner-node-02",
		"timestamp": "2024-06-15T11:00:00.000Z",
		"pipeline_id": "pipe-456",
		"step_name": "build",
		"repository": "org/repo2",
		"description": "Unauthorized process ancestry detected"
	}`

	req := httptest.NewRequest(http.MethodPost, WebhookPath, strings.NewReader(body))
	rr := httptest.NewRecorder()

	ws.handleWebhook(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}

	event := emitter.events[0]
	if event.EventType != EventTypeSupplyChainProcessAnomaly {
		t.Errorf("expected EventType=%q, got %q", EventTypeSupplyChainProcessAnomaly, event.EventType)
	}
	if event.Severity != SeverityHigh {
		t.Errorf("expected Severity=%q, got %q", SeverityHigh, event.Severity)
	}
}

func TestWebhook_AttestationFailure(t *testing.T) {
	emitter := &mockEmitter{}
	ws := newTestWebhookServer("", emitter)

	body := `{
		"event_type": "attestation_failure",
		"node": "runner-node-03",
		"timestamp": "2024-06-15T12:00:00.000Z",
		"description": "Build attestation verification failed"
	}`

	req := httptest.NewRequest(http.MethodPost, WebhookPath, strings.NewReader(body))
	rr := httptest.NewRecorder()

	ws.handleWebhook(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}

	event := emitter.events[0]
	if event.EventType != EventTypeSupplyChainAttestationFailure {
		t.Errorf("expected EventType=%q, got %q", EventTypeSupplyChainAttestationFailure, event.EventType)
	}
	if event.Severity != SeverityHigh {
		t.Errorf("expected Severity=%q, got %q", SeverityHigh, event.Severity)
	}
	// pipeline_id, step_name, repository are empty -> should not be in labels
	if _, ok := event.Labels[LabelPipelineID]; ok {
		t.Error("expected pipeline_id label to be omitted when empty")
	}
	if _, ok := event.Labels[LabelStepName]; ok {
		t.Error("expected step_name label to be omitted when empty")
	}
	if _, ok := event.Labels[LabelRepository]; ok {
		t.Error("expected repository label to be omitted when empty")
	}
}

func TestWebhook_InvalidSignature(t *testing.T) {
	emitter := &mockEmitter{}
	ws := newTestWebhookServer("secret-key", emitter)

	body := `{"event_type": "credential_exfil", "node": "n1", "timestamp": "2024-06-15T10:30:00Z"}`

	req := httptest.NewRequest(http.MethodPost, WebhookPath, strings.NewReader(body))
	req.Header.Set(WebhookSignatureHeader, "invalid-signature")
	rr := httptest.NewRecorder()

	ws.handleWebhook(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", rr.Code)
	}
	if len(emitter.events) != 0 {
		t.Error("expected no events emitted on invalid signature")
	}
}

func TestWebhook_MissingSignature(t *testing.T) {
	emitter := &mockEmitter{}
	ws := newTestWebhookServer("secret-key", emitter)

	body := `{"event_type": "credential_exfil", "node": "n1", "timestamp": "2024-06-15T10:30:00Z"}`

	req := httptest.NewRequest(http.MethodPost, WebhookPath, strings.NewReader(body))
	// No signature header
	rr := httptest.NewRecorder()

	ws.handleWebhook(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", rr.Code)
	}
}

func TestWebhook_NoHMACKey_SkipsVerification(t *testing.T) {
	emitter := &mockEmitter{}
	ws := newTestWebhookServer("", emitter) // empty key = skip verification

	body := `{"event_type": "credential_exfil", "node": "n1", "timestamp": "2024-06-15T10:30:00Z", "description": "test"}`

	req := httptest.NewRequest(http.MethodPost, WebhookPath, strings.NewReader(body))
	// No signature header, but should still work
	rr := httptest.NewRecorder()

	ws.handleWebhook(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}
}

func TestWebhook_MethodNotAllowed(t *testing.T) {
	emitter := &mockEmitter{}
	ws := newTestWebhookServer("", emitter)

	req := httptest.NewRequest(http.MethodGet, WebhookPath, nil)
	rr := httptest.NewRecorder()

	ws.handleWebhook(rr, req)

	if rr.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405, got %d", rr.Code)
	}
}

func TestWebhook_InvalidJSON(t *testing.T) {
	emitter := &mockEmitter{}
	ws := newTestWebhookServer("", emitter)

	body := `not valid json`

	req := httptest.NewRequest(http.MethodPost, WebhookPath, strings.NewReader(body))
	rr := httptest.NewRecorder()

	ws.handleWebhook(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rr.Code)
	}
}

func TestWebhook_UnknownEventType(t *testing.T) {
	emitter := &mockEmitter{}
	ws := newTestWebhookServer("", emitter)

	body := `{"event_type": "unknown_type", "node": "n1", "timestamp": "2024-06-15T10:30:00Z"}`

	req := httptest.NewRequest(http.MethodPost, WebhookPath, strings.NewReader(body))
	rr := httptest.NewRecorder()

	ws.handleWebhook(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rr.Code)
	}
}

func TestWebhook_EmitRetry_EventualSuccess(t *testing.T) {
	emitter := &mockEmitter{failCount: 2} // fail twice, succeed on third
	ws := newTestWebhookServer("", emitter)

	body := `{"event_type": "credential_exfil", "node": "n1", "timestamp": "2024-06-15T10:30:00Z", "description": "test"}`

	req := httptest.NewRequest(http.MethodPost, WebhookPath, strings.NewReader(body))
	rr := httptest.NewRecorder()

	ws.handleWebhook(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200 after retry, got %d", rr.Code)
	}
	if emitter.callCount != 3 {
		t.Errorf("expected 3 emit calls, got %d", emitter.callCount)
	}
}

func TestWebhook_EmitRetry_AllFail(t *testing.T) {
	emitter := &mockEmitter{failCount: 5} // more failures than max retries
	ws := newTestWebhookServer("", emitter)

	body := `{"event_type": "credential_exfil", "node": "n1", "timestamp": "2024-06-15T10:30:00Z", "description": "test"}`

	req := httptest.NewRequest(http.MethodPost, WebhookPath, strings.NewReader(body))
	rr := httptest.NewRecorder()

	ws.handleWebhook(rr, req)

	// Should respond 202 when all retries fail
	if rr.Code != http.StatusAccepted {
		t.Fatalf("expected 202 when all retries fail, got %d", rr.Code)
	}
	if emitter.callCount != WebhookEmitMaxRetries {
		t.Errorf("expected %d emit calls, got %d", WebhookEmitMaxRetries, emitter.callCount)
	}
}

func TestWebhook_TimestampParsing_InvalidFallsBackToNow(t *testing.T) {
	emitter := &mockEmitter{}
	ws := newTestWebhookServer("", emitter)

	body := `{"event_type": "credential_exfil", "node": "n1", "timestamp": "not-a-timestamp", "description": "test"}`

	req := httptest.NewRequest(http.MethodPost, WebhookPath, strings.NewReader(body))
	rr := httptest.NewRecorder()

	ws.handleWebhook(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}
	// Timestamp should be set to something (current time), not zero
	if emitter.events[0].Timestamp.IsZero() {
		t.Error("expected non-zero timestamp when parsing fails")
	}
}

func TestWebhook_OmitsEmptyLabels(t *testing.T) {
	emitter := &mockEmitter{}
	ws := newTestWebhookServer("", emitter)

	// Only pipeline_id is set; step_name and repository are empty
	body := `{
		"event_type": "process_anomaly",
		"node": "runner-01",
		"timestamp": "2024-06-15T10:30:00Z",
		"pipeline_id": "pipe-789",
		"step_name": "",
		"repository": "",
		"description": "test"
	}`

	req := httptest.NewRequest(http.MethodPost, WebhookPath, strings.NewReader(body))
	rr := httptest.NewRecorder()

	ws.handleWebhook(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}

	labels := emitter.events[0].Labels
	if labels[LabelPipelineID] != "pipe-789" {
		t.Errorf("expected pipeline_id label, got %q", labels[LabelPipelineID])
	}
	if _, ok := labels[LabelStepName]; ok {
		t.Error("expected step_name to be omitted when empty")
	}
	if _, ok := labels[LabelRepository]; ok {
		t.Error("expected repository to be omitted when empty")
	}
}
