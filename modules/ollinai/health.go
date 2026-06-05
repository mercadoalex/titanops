package ollinai

import (
	"encoding/json"
	"net/http"
	"sync/atomic"
)

// HealthChecker tracks connection state for /healthz and /readyz endpoints.
// External callers (poller, emitter) update connection state via the Set methods.
type HealthChecker struct {
	natsConnected  atomic.Bool
	ollinConnected atomic.Bool
}

// NewHealthChecker creates a new HealthChecker with both connections initially disconnected.
func NewHealthChecker() *HealthChecker {
	return &HealthChecker{}
}

// SetNATSConnected updates the NATS connection state.
func (hc *HealthChecker) SetNATSConnected(v bool) {
	hc.natsConnected.Store(v)
}

// SetOllinConnected updates the OllinAI API connection state.
func (hc *HealthChecker) SetOllinConnected(v bool) {
	hc.ollinConnected.Store(v)
}

// NATSConnected returns the current NATS connection state.
func (hc *HealthChecker) NATSConnected() bool {
	return hc.natsConnected.Load()
}

// OllinConnected returns the current OllinAI connection state.
func (hc *HealthChecker) OllinConnected() bool {
	return hc.ollinConnected.Load()
}

// Healthz handles the /healthz endpoint. It always returns HTTP 200 with body "ok"
// since the process is running if it can serve the request.
func (hc *HealthChecker) Healthz(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	w.Write([]byte("ok"))
}

// readyzResponse is the JSON response body for the /readyz endpoint.
type readyzResponse struct {
	Ready   bool `json:"ready"`
	NATS    bool `json:"nats,omitempty"`
	OllinAI bool `json:"ollinai,omitempty"`
}

// Readyz handles the /readyz endpoint. It returns HTTP 200 with {"ready": true}
// only if both NATS and OllinAI connections are active. Otherwise it returns
// HTTP 503 with {"ready": false, "nats": <bool>, "ollinai": <bool>}.
func (hc *HealthChecker) Readyz(w http.ResponseWriter, r *http.Request) {
	nats := hc.natsConnected.Load()
	ollin := hc.ollinConnected.Load()

	w.Header().Set("Content-Type", "application/json; charset=utf-8")

	if nats && ollin {
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(readyzResponse{Ready: true})
		return
	}

	w.WriteHeader(http.StatusServiceUnavailable)
	json.NewEncoder(w).Encode(readyzResponse{
		Ready:   false,
		NATS:    nats,
		OllinAI: ollin,
	})
}
