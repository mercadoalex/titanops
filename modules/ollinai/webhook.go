package ollinai

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"time"

	export "github.com/mercadoalex/titanops/shared/titanops-export"
)

const (
	// WebhookPath is the URL path for receiving eBPF supply chain events.
	WebhookPath = "/webhook/supply-chain"

	// WebhookSignatureHeader is the HTTP header containing the HMAC-SHA256 signature.
	WebhookSignatureHeader = "X-OllinAI-Signature"

	// WebhookMaxBodySize is the maximum allowed request body size (1 MB).
	WebhookMaxBodySize = 1 << 20

	// WebhookEmitMaxRetries is the maximum number of emit retries for webhook events.
	WebhookEmitMaxRetries = 3
)

// WebhookRequest represents the JSON body sent by the OllinAI eBPF agent.
type WebhookRequest struct {
	EventType   string `json:"event_type"`
	Node        string `json:"node"`
	Timestamp   string `json:"timestamp"`
	PipelineID  string `json:"pipeline_id"`
	StepName    string `json:"step_name"`
	Repository  string `json:"repository"`
	Description string `json:"description"`
	Evidence    string `json:"evidence"`
}

// WebhookServer receives push notifications from OllinAI's eBPF agent.
type WebhookServer struct {
	server  *http.Server
	emitter EventEmitter
	hmacKey []byte
	logger  *log.Logger
}

// WebhookServerConfig configures the WebhookServer.
type WebhookServerConfig struct {
	// Port is the HTTP listen port. Required.
	Port int
	// Emitter is the EventEmitter to publish events to. Required.
	Emitter EventEmitter
	// HMACKey is the shared secret for HMAC-SHA256 signature verification.
	// If empty, signature verification is skipped (development mode).
	HMACKey string
	// Logger for warning/error messages. Optional.
	Logger *log.Logger
}

// NewWebhookServer creates a new WebhookServer with the given configuration.
func NewWebhookServer(cfg WebhookServerConfig) *WebhookServer {
	ws := &WebhookServer{
		emitter: cfg.Emitter,
		hmacKey: []byte(cfg.HMACKey),
		logger:  cfg.Logger,
	}

	mux := http.NewServeMux()
	mux.HandleFunc(WebhookPath, ws.handleWebhook)

	ws.server = &http.Server{
		Addr:         fmt.Sprintf(":%d", cfg.Port),
		Handler:      mux,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 10 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	return ws
}

// Start begins listening for incoming webhook POSTs on the configured port.
func (ws *WebhookServer) Start(ctx context.Context) error {
	errCh := make(chan error, 1)
	go func() {
		if err := ws.server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			errCh <- err
		}
		close(errCh)
	}()

	select {
	case err := <-errCh:
		return fmt.Errorf("webhook server failed to start: %w", err)
	case <-time.After(100 * time.Millisecond):
		// Server started successfully
		return nil
	}
}

// Shutdown gracefully stops the HTTP server.
func (ws *WebhookServer) Shutdown(ctx context.Context) error {
	return ws.server.Shutdown(ctx)
}

// handleWebhook processes incoming webhook requests from the OllinAI eBPF agent.
func (ws *WebhookServer) handleWebhook(w http.ResponseWriter, r *http.Request) {
	// Only accept POST method
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Read body with size limit
	body, err := io.ReadAll(io.LimitReader(r.Body, WebhookMaxBodySize))
	if err != nil {
		http.Error(w, "Failed to read request body", http.StatusBadRequest)
		return
	}
	defer r.Body.Close()

	// Verify HMAC signature (skip if key is empty/not configured)
	if len(ws.hmacKey) > 0 {
		signature := r.Header.Get(WebhookSignatureHeader)
		if !ws.verifySignature(body, signature) {
			http.Error(w, "Invalid signature", http.StatusUnauthorized)
			return
		}
	}

	// Parse JSON body
	var req WebhookRequest
	if err := json.Unmarshal(body, &req); err != nil {
		http.Error(w, "Invalid JSON payload", http.StatusBadRequest)
		return
	}

	// Map event type to EventType and Severity
	eventType, severity, err := ws.mapEventType(req.EventType)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	// Parse timestamp
	ts, err := time.Parse(time.RFC3339Nano, req.Timestamp)
	if err != nil {
		// Fall back to current UTC time if timestamp is unparseable
		ts = time.Now().UTC()
	} else {
		ts = ts.UTC()
	}

	// Build SupplyChainPayload
	payload := SupplyChainPayload{
		PipelineID:  req.PipelineID,
		StepName:    req.StepName,
		Repository:  req.Repository,
		Description: req.Description,
		Evidence:    req.Evidence,
	}

	// Serialize payload
	payloadBytes, truncated, err := SerializePayload(payload)
	if err != nil {
		ws.logWarn("failed to serialize supply chain payload: %v", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	// Build labels (omit unavailable keys)
	labels := make(map[string]string)
	if req.PipelineID != "" {
		labels[LabelPipelineID] = req.PipelineID
	}
	if req.StepName != "" {
		labels[LabelStepName] = req.StepName
	}
	if req.Repository != "" {
		labels[LabelRepository] = req.Repository
	}
	if truncated {
		labels[LabelPayloadTruncated] = "true"
	}

	// Build export.Event
	event := export.Event{
		Module:    ModuleName,
		EventType: eventType,
		Severity:  severity,
		Node:      req.Node,
		Timestamp: ts,
		Payload:   payloadBytes,
		Labels:    labels,
	}

	// Emit event with retry (up to 3 times)
	ctx := r.Context()
	var emitErr error
	for attempt := 0; attempt < WebhookEmitMaxRetries; attempt++ {
		emitErr = ws.emitter.Emit(ctx, event)
		if emitErr == nil {
			break
		}
		ws.logWarn("emit attempt %d/%d failed: %v", attempt+1, WebhookEmitMaxRetries, emitErr)
	}

	if emitErr != nil {
		// All retries failed: log warning and respond 202 (accepted but not guaranteed)
		ws.logWarn("all %d emit attempts failed for event type %s: %v", WebhookEmitMaxRetries, eventType, emitErr)
		w.WriteHeader(http.StatusAccepted)
		return
	}

	// Success
	w.WriteHeader(http.StatusOK)
}

// verifySignature verifies the HMAC-SHA256 signature of the request body.
// The expected signature is hex-encoded.
func (ws *WebhookServer) verifySignature(body []byte, signature string) bool {
	if signature == "" {
		return false
	}

	mac := hmac.New(sha256.New, ws.hmacKey)
	mac.Write(body)
	expectedMAC := mac.Sum(nil)
	expectedSignature := hex.EncodeToString(expectedMAC)

	return hmac.Equal([]byte(expectedSignature), []byte(signature))
}

// mapEventType maps the raw event_type from the webhook request to the
// corresponding EventType constant and Severity.
func (ws *WebhookServer) mapEventType(rawType string) (string, string, error) {
	switch rawType {
	case "credential_exfil":
		return EventTypeSupplyChainCredentialExfil, SeverityCritical, nil
	case "process_anomaly":
		return EventTypeSupplyChainProcessAnomaly, SeverityHigh, nil
	case "attestation_failure":
		return EventTypeSupplyChainAttestationFailure, SeverityHigh, nil
	default:
		return "", "", fmt.Errorf("unknown event type: %s", rawType)
	}
}

func (ws *WebhookServer) logWarn(format string, args ...any) {
	if ws.logger != nil {
		ws.logger.Printf("[WARN] "+format, args...)
	}
}
