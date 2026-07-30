package alertmanager

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"sync/atomic"
	"time"

	export "github.com/mercadoalex/titanops/shared/titanops-export"
)

// EventIngester is the interface that the correlation engine (or any consumer)
// must implement to receive events from the AlertManager receiver.
type EventIngester interface {
	// Ingest processes a single event. Implementations should be safe for concurrent use.
	Ingest(ctx context.Context, event export.Event) error
}

// ReceiverConfig holds the configuration for the AlertManager webhook receiver.
type ReceiverConfig struct {
	// ListenAddr is the address to listen on (e.g., ":9095").
	// Default: ":9095"
	ListenAddr string
	// Path is the HTTP path for the webhook endpoint.
	// Default: "/webhooks/alertmanager"
	Path string
	// IncludeResolved controls whether resolved alerts are ingested.
	// Default: false (only firing alerts are processed).
	IncludeResolved bool
	// MaxPayloadBytes is the maximum allowed request body size.
	// Default: 1MB (1 << 20).
	MaxPayloadBytes int64
	// ReadTimeout is the HTTP server read timeout.
	// Default: 10s.
	ReadTimeout time.Duration
	// WriteTimeout is the HTTP server write timeout.
	// Default: 10s.
	WriteTimeout time.Duration
}

// DefaultReceiverConfig returns a ReceiverConfig with sensible defaults.
func DefaultReceiverConfig() ReceiverConfig {
	return ReceiverConfig{
		ListenAddr:      ":9095",
		Path:            "/webhooks/alertmanager",
		IncludeResolved: false,
		MaxPayloadBytes: 1 << 20, // 1 MB
		ReadTimeout:     10 * time.Second,
		WriteTimeout:    10 * time.Second,
	}
}

// Receiver is the AlertManager webhook HTTP receiver.
// It accepts AlertManager webhook payloads, maps them to export.Event,
// and ingests them into the correlation engine.
type Receiver struct {
	config   ReceiverConfig
	ingester EventIngester
	server   *http.Server

	// Metrics (atomic counters for thread safety).
	eventsReceived atomic.Int64
	eventsMapped   atomic.Int64
	eventsIngested atomic.Int64
	errors         atomic.Int64
}

// NewReceiver creates a new AlertManager webhook receiver.
func NewReceiver(cfg ReceiverConfig, ingester EventIngester) (*Receiver, error) {
	if ingester == nil {
		return nil, fmt.Errorf("alertmanager receiver: ingester must not be nil")
	}
	if cfg.ListenAddr == "" {
		cfg.ListenAddr = DefaultReceiverConfig().ListenAddr
	}
	if cfg.Path == "" {
		cfg.Path = DefaultReceiverConfig().Path
	}
	if cfg.MaxPayloadBytes <= 0 {
		cfg.MaxPayloadBytes = DefaultReceiverConfig().MaxPayloadBytes
	}
	if cfg.ReadTimeout <= 0 {
		cfg.ReadTimeout = DefaultReceiverConfig().ReadTimeout
	}
	if cfg.WriteTimeout <= 0 {
		cfg.WriteTimeout = DefaultReceiverConfig().WriteTimeout
	}

	r := &Receiver{
		config:   cfg,
		ingester: ingester,
	}

	mux := http.NewServeMux()
	mux.HandleFunc(cfg.Path, r.handleWebhook)
	mux.HandleFunc("/healthz", r.handleHealthz)
	mux.HandleFunc("/metrics", r.handleMetrics)

	r.server = &http.Server{
		Addr:         cfg.ListenAddr,
		Handler:      mux,
		ReadTimeout:  cfg.ReadTimeout,
		WriteTimeout: cfg.WriteTimeout,
	}

	return r, nil
}

// Start begins listening for AlertManager webhooks. It blocks until the server
// is stopped or encounters an error. Use Stop() to gracefully shut down.
func (r *Receiver) Start() error {
	log.Printf("[alertmanager-receiver] listening on %s%s", r.config.ListenAddr, r.config.Path)
	if err := r.server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		return fmt.Errorf("alertmanager receiver: %w", err)
	}
	return nil
}

// Stop gracefully shuts down the receiver with a 5-second timeout.
func (r *Receiver) Stop(ctx context.Context) error {
	log.Println("[alertmanager-receiver] shutting down...")
	shutdownCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	return r.server.Shutdown(shutdownCtx)
}

// Stats returns the current receiver metrics.
func (r *Receiver) Stats() ReceiverStats {
	return ReceiverStats{
		EventsReceived: r.eventsReceived.Load(),
		EventsMapped:   r.eventsMapped.Load(),
		EventsIngested: r.eventsIngested.Load(),
		Errors:         r.errors.Load(),
	}
}

// ReceiverStats holds operational metrics for the receiver.
type ReceiverStats struct {
	EventsReceived int64 `json:"events_received"`
	EventsMapped   int64 `json:"events_mapped"`
	EventsIngested int64 `json:"events_ingested"`
	Errors         int64 `json:"errors"`
}

// handleWebhook processes incoming AlertManager webhook POST requests.
func (r *Receiver) handleWebhook(w http.ResponseWriter, req *http.Request) {
	// Only accept POST.
	if req.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Enforce content-type.
	ct := req.Header.Get("Content-Type")
	if ct != "" && !isJSONContentType(ct) {
		http.Error(w, "content-type must be application/json", http.StatusUnsupportedMediaType)
		return
	}

	// Limit body size.
	body := http.MaxBytesReader(w, req.Body, r.config.MaxPayloadBytes)
	defer body.Close()

	data, err := io.ReadAll(body)
	if err != nil {
		r.errors.Add(1)
		http.Error(w, "request body too large or unreadable", http.StatusRequestEntityTooLarge)
		return
	}

	r.eventsReceived.Add(1)

	// Parse the AlertManager payload.
	var payload AlertManagerPayload
	if err := json.Unmarshal(data, &payload); err != nil {
		r.errors.Add(1)
		http.Error(w, "invalid JSON payload", http.StatusBadRequest)
		return
	}

	// Validate minimum structure.
	if len(payload.Alerts) == 0 {
		w.WriteHeader(http.StatusOK)
		writeJSONResponse(w, webhookResponse{Status: "ok", Message: "no alerts in payload"})
		return
	}

	// Map alerts to events.
	events, err := MapPayloadToEvents(payload, r.config.IncludeResolved)
	if err != nil {
		r.errors.Add(1)
		http.Error(w, "failed to map alerts", http.StatusInternalServerError)
		return
	}

	r.eventsMapped.Add(int64(len(events)))

	// Ingest events into the correlation engine.
	ctx := req.Context()
	ingested := 0
	for _, event := range events {
		if err := r.ingester.Ingest(ctx, event); err != nil {
			r.errors.Add(1)
			log.Printf("[alertmanager-receiver] ingest error: %v", err)
			continue
		}
		ingested++
	}

	r.eventsIngested.Add(int64(ingested))

	// Respond with success.
	w.WriteHeader(http.StatusOK)
	writeJSONResponse(w, webhookResponse{
		Status:   "ok",
		Message:  fmt.Sprintf("processed %d alerts, ingested %d events", len(payload.Alerts), ingested),
		Ingested: ingested,
	})
}

// handleHealthz is a simple health check endpoint.
func (r *Receiver) handleHealthz(w http.ResponseWriter, _ *http.Request) {
	w.WriteHeader(http.StatusOK)
	writeJSONResponse(w, map[string]string{"status": "healthy"})
}

// handleMetrics returns receiver operational metrics.
func (r *Receiver) handleMetrics(w http.ResponseWriter, _ *http.Request) {
	w.WriteHeader(http.StatusOK)
	writeJSONResponse(w, r.Stats())
}

// webhookResponse is the JSON response sent back to AlertManager.
type webhookResponse struct {
	Status   string `json:"status"`
	Message  string `json:"message"`
	Ingested int    `json:"ingested,omitempty"`
}

// writeJSONResponse writes a JSON response to the http.ResponseWriter.
func writeJSONResponse(w http.ResponseWriter, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(data)
}

// isJSONContentType checks if the content-type header indicates JSON.
func isJSONContentType(ct string) bool {
	// Accept "application/json" with any charset or parameters.
	return len(ct) >= 16 && ct[:16] == "application/json"
}
