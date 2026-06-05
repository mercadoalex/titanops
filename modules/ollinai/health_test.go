package ollinai

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestHealthz_AlwaysReturns200(t *testing.T) {
	hc := NewHealthChecker()

	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	w := httptest.NewRecorder()

	hc.Healthz(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", w.Code)
	}
	if w.Body.String() != "ok" {
		t.Errorf("expected body 'ok', got %q", w.Body.String())
	}
	if ct := w.Header().Get("Content-Type"); ct != "text/plain; charset=utf-8" {
		t.Errorf("expected Content-Type 'text/plain; charset=utf-8', got %q", ct)
	}
}

func TestHealthz_Returns200EvenWhenDisconnected(t *testing.T) {
	hc := NewHealthChecker()
	// Both disconnected by default
	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	w := httptest.NewRecorder()

	hc.Healthz(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200 regardless of connection state, got %d", w.Code)
	}
}

func TestReadyz_BothConnected(t *testing.T) {
	hc := NewHealthChecker()
	hc.SetNATSConnected(true)
	hc.SetOllinConnected(true)

	req := httptest.NewRequest(http.MethodGet, "/readyz", nil)
	w := httptest.NewRecorder()

	hc.Readyz(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200 when both connected, got %d", w.Code)
	}

	var resp readyzResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if !resp.Ready {
		t.Error("expected ready=true when both connected")
	}
}

func TestReadyz_NATSDisconnected(t *testing.T) {
	hc := NewHealthChecker()
	hc.SetNATSConnected(false)
	hc.SetOllinConnected(true)

	req := httptest.NewRequest(http.MethodGet, "/readyz", nil)
	w := httptest.NewRecorder()

	hc.Readyz(w, req)

	if w.Code != http.StatusServiceUnavailable {
		t.Errorf("expected status 503 when NATS disconnected, got %d", w.Code)
	}

	var resp readyzResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if resp.Ready {
		t.Error("expected ready=false when NATS disconnected")
	}
	if resp.NATS {
		t.Error("expected nats=false in response")
	}
	if !resp.OllinAI {
		t.Error("expected ollinai=true in response")
	}
}

func TestReadyz_OllinAIDisconnected(t *testing.T) {
	hc := NewHealthChecker()
	hc.SetNATSConnected(true)
	hc.SetOllinConnected(false)

	req := httptest.NewRequest(http.MethodGet, "/readyz", nil)
	w := httptest.NewRecorder()

	hc.Readyz(w, req)

	if w.Code != http.StatusServiceUnavailable {
		t.Errorf("expected status 503 when OllinAI disconnected, got %d", w.Code)
	}

	var resp readyzResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if resp.Ready {
		t.Error("expected ready=false when OllinAI disconnected")
	}
	if !resp.NATS {
		t.Error("expected nats=true in response")
	}
	if resp.OllinAI {
		t.Error("expected ollinai=false in response")
	}
}

func TestReadyz_BothDisconnected(t *testing.T) {
	hc := NewHealthChecker()
	hc.SetNATSConnected(false)
	hc.SetOllinConnected(false)

	req := httptest.NewRequest(http.MethodGet, "/readyz", nil)
	w := httptest.NewRecorder()

	hc.Readyz(w, req)

	if w.Code != http.StatusServiceUnavailable {
		t.Errorf("expected status 503 when both disconnected, got %d", w.Code)
	}

	var resp readyzResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if resp.Ready {
		t.Error("expected ready=false when both disconnected")
	}
	if resp.NATS {
		t.Error("expected nats=false in response")
	}
	if resp.OllinAI {
		t.Error("expected ollinai=false in response")
	}
}

func TestSetNATSConnected(t *testing.T) {
	hc := NewHealthChecker()

	if hc.NATSConnected() {
		t.Error("expected NATS initially disconnected")
	}

	hc.SetNATSConnected(true)
	if !hc.NATSConnected() {
		t.Error("expected NATS connected after SetNATSConnected(true)")
	}

	hc.SetNATSConnected(false)
	if hc.NATSConnected() {
		t.Error("expected NATS disconnected after SetNATSConnected(false)")
	}
}

func TestSetOllinConnected(t *testing.T) {
	hc := NewHealthChecker()

	if hc.OllinConnected() {
		t.Error("expected OllinAI initially disconnected")
	}

	hc.SetOllinConnected(true)
	if !hc.OllinConnected() {
		t.Error("expected OllinAI connected after SetOllinConnected(true)")
	}

	hc.SetOllinConnected(false)
	if hc.OllinConnected() {
		t.Error("expected OllinAI disconnected after SetOllinConnected(false)")
	}
}
