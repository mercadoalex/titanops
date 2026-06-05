package ollinai

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"time"

	export "github.com/mercadoalex/titanops/shared/titanops-export"
)

// PollerConfig holds configuration for the Poller.
type PollerConfig struct {
	// Client is the HTTP client used for API requests.
	Client *http.Client
	// Endpoint is the OllinAI API base URL.
	Endpoint string
	// AuthToken is the bearer token for authentication.
	AuthToken string
	// RiskInterval is the polling interval for deployment risk data. Default: 30s.
	RiskInterval time.Duration
	// DORAInterval is the polling interval for DORA metrics. Default: 5m.
	DORAInterval time.Duration
	// Emitter is the event emitter for publishing events to NATS.
	Emitter EventEmitter
	// OnError is called when a polling error occurs. It receives the error category
	// and error for external handling (e.g., incrementing poll_errors_total, setting health degraded).
	OnError func(category ErrorCategory, err error)
	// Logger for warning/error messages. Optional.
	Logger *log.Logger
}

// Poller periodically fetches data from the OllinAI REST API and emits events.
type Poller struct {
	client       *http.Client
	endpoint     string
	authToken    string
	riskInterval time.Duration
	doraInterval time.Duration
	emitter      EventEmitter
	onError      func(category ErrorCategory, err error)
	logger       *log.Logger
}

// Error category constants for the poller.
const (
	ErrOllinAPIUnavailable ErrorCategory = "ollinai_api_unavailable"
	ErrOllinAPITimeout     ErrorCategory = "ollinai_api_timeout"
	ErrOllinAPIAuth        ErrorCategory = "ollinai_auth_failed"
)

// ErrorCategory represents a typed error category for the adapter.
type ErrorCategory string

// apiDeploymentRiskEntry represents a single deployment risk entry from the OllinAI API.
type apiDeploymentRiskEntry struct {
	Service     string   `json:"service"`
	CommitSHA   string   `json:"commit_sha"`
	Deployer    string   `json:"deployer"`
	RiskScore   int      `json:"risk_score"`
	RiskFactors []string `json:"risk_factors"`
	PipelineID  string   `json:"pipeline_id"`
	Environment string   `json:"environment"`
	Node        string   `json:"node"`
	Pod         string   `json:"pod"`
	Namespace   string   `json:"namespace"`
}

// apiDORAMetricsResponse represents the DORA metrics response from the OllinAI API.
type apiDORAMetricsResponse struct {
	DeploymentFrequency  float64 `json:"deployment_frequency"`
	LeadTimeForChanges   float64 `json:"lead_time_for_changes"`
	ChangeFailureRate    float64 `json:"change_failure_rate"`
	TimeToRestoreService float64 `json:"time_to_restore_service"`
}

// NewPoller creates a new Poller with the given configuration.
func NewPoller(cfg PollerConfig) *Poller {
	client := cfg.Client
	if client == nil {
		client = &http.Client{Timeout: 30 * time.Second}
	}

	riskInterval := cfg.RiskInterval
	if riskInterval <= 0 {
		riskInterval = 30 * time.Second
	}

	doraInterval := cfg.DORAInterval
	if doraInterval <= 0 {
		doraInterval = 5 * time.Minute
	}

	return &Poller{
		client:       client,
		endpoint:     cfg.Endpoint,
		authToken:    cfg.AuthToken,
		riskInterval: riskInterval,
		doraInterval: doraInterval,
		emitter:      cfg.Emitter,
		onError:      cfg.OnError,
		logger:       cfg.Logger,
	}
}

// Start begins the two polling loops for deployment risk and DORA metrics.
// It blocks until ctx is canceled.
func (p *Poller) Start(ctx context.Context) error {
	riskTicker := time.NewTicker(p.riskInterval)
	doraTicker := time.NewTicker(p.doraInterval)
	defer riskTicker.Stop()
	defer doraTicker.Stop()

	// Perform initial polls immediately
	p.pollDeploymentRisk(ctx)
	p.pollDORAMetrics(ctx)

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-riskTicker.C:
			p.pollDeploymentRisk(ctx)
		case <-doraTicker.C:
			p.pollDORAMetrics(ctx)
		}
	}
}

// pollDeploymentRisk fetches deployment risk data from the OllinAI API and emits events.
func (p *Poller) pollDeploymentRisk(ctx context.Context) {
	url := p.endpoint + "/api/v1/deployments/risk"

	body, err := p.doRequest(ctx, url)
	if err != nil {
		return // error already reported via doRequest
	}

	var entries []apiDeploymentRiskEntry
	if err := json.Unmarshal(body, &entries); err != nil {
		p.logWarn("failed to parse deployment risk response: %v", err)
		p.reportError(ErrOllinAPIUnavailable, fmt.Errorf("failed to parse deployment risk response: %w", err))
		return
	}

	for _, entry := range entries {
		event := p.buildDeploymentRiskEvent(entry)
		if err := p.emitter.Emit(ctx, event); err != nil {
			p.logWarn("failed to emit deployment risk event for service %s: %v", entry.Service, err)
		}
	}
}

// pollDORAMetrics fetches DORA metrics from the OllinAI API and emits an event.
func (p *Poller) pollDORAMetrics(ctx context.Context) {
	url := p.endpoint + "/api/v1/metrics/dora"

	body, err := p.doRequest(ctx, url)
	if err != nil {
		return // error already reported via doRequest
	}

	var metrics apiDORAMetricsResponse
	if err := json.Unmarshal(body, &metrics); err != nil {
		p.logWarn("failed to parse DORA metrics response: %v", err)
		p.reportError(ErrOllinAPIUnavailable, fmt.Errorf("failed to parse DORA metrics response: %w", err))
		return
	}

	event := p.buildDORAMetricsEvent(metrics)
	if err := p.emitter.Emit(ctx, event); err != nil {
		p.logWarn("failed to emit DORA metrics event: %v", err)
	}
}

// doRequest performs an authenticated GET request to the given URL.
// Returns the response body bytes or an error. On error, it logs a warning
// and reports the error via the OnError callback.
func (p *Poller) doRequest(ctx context.Context, url string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		p.logWarn("failed to create request for %s: %v", url, err)
		p.reportError(ErrOllinAPIUnavailable, fmt.Errorf("failed to create request: %w", err))
		return nil, err
	}

	req.Header.Set("Authorization", "Bearer "+p.authToken)
	req.Header.Set("Accept", "application/json")

	resp, err := p.client.Do(req)
	if err != nil {
		p.logWarn("API request failed for %s: %v", url, err)
		p.reportError(ErrOllinAPITimeout, fmt.Errorf("API request failed: %w", err))
		return nil, err
	}
	defer resp.Body.Close()

	// Handle auth errors: signal health degraded
	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
		p.logWarn("authentication failed for %s: HTTP %d", url, resp.StatusCode)
		p.reportError(ErrOllinAPIAuth, fmt.Errorf("authentication failed: HTTP %d", resp.StatusCode))
		return nil, fmt.Errorf("auth error: HTTP %d", resp.StatusCode)
	}

	// Handle other non-2xx responses
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		p.logWarn("API returned non-2xx for %s: HTTP %d", url, resp.StatusCode)
		p.reportError(ErrOllinAPIUnavailable, fmt.Errorf("API returned HTTP %d", resp.StatusCode))
		return nil, fmt.Errorf("API error: HTTP %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		p.logWarn("failed to read response body from %s: %v", url, err)
		p.reportError(ErrOllinAPIUnavailable, fmt.Errorf("failed to read response body: %w", err))
		return nil, err
	}

	return body, nil
}

// buildDeploymentRiskEvent constructs an export.Event from a deployment risk API entry.
func (p *Poller) buildDeploymentRiskEvent(entry apiDeploymentRiskEntry) export.Event {
	payload := DeploymentRiskPayload{
		Service:     entry.Service,
		CommitSHA:   entry.CommitSHA,
		Deployer:    entry.Deployer,
		RiskScore:   entry.RiskScore,
		RiskFactors: entry.RiskFactors,
		PipelineID:  entry.PipelineID,
		Environment: entry.Environment,
	}

	payloadBytes, truncated, err := SerializePayload(payload)
	if err != nil {
		p.logWarn("failed to serialize deployment risk payload for service %s: %v", entry.Service, err)
		payloadBytes = []byte("{}")
	}

	labels := map[string]string{
		LabelService:    entry.Service,
		LabelCommitSHA:  entry.CommitSHA,
		LabelDeployer:   entry.Deployer,
		LabelPipelineID: entry.PipelineID,
	}

	if truncated {
		labels[LabelPayloadTruncated] = "true"
	}

	return export.Event{
		Module:    ModuleName,
		EventType: EventTypeDeploymentRisk,
		Severity:  MapRiskToSeverity(entry.RiskScore),
		Payload:   payloadBytes,
		Node:      entry.Node,
		Pod:       entry.Pod,
		Namespace: entry.Namespace,
		Labels:    labels,
	}
}

// buildDORAMetricsEvent constructs an export.Event from DORA metrics API data.
func (p *Poller) buildDORAMetricsEvent(metrics apiDORAMetricsResponse) export.Event {
	payload := DORAMetricsPayload{
		DeploymentFrequency:  metrics.DeploymentFrequency,
		LeadTimeForChanges:   metrics.LeadTimeForChanges,
		ChangeFailureRate:    metrics.ChangeFailureRate,
		TimeToRestoreService: metrics.TimeToRestoreService,
	}

	payloadBytes, truncated, err := SerializePayload(payload)
	if err != nil {
		p.logWarn("failed to serialize DORA metrics payload: %v", err)
		payloadBytes = []byte("{}")
	}

	labels := map[string]string{}
	if truncated {
		labels[LabelPayloadTruncated] = "true"
	}

	return export.Event{
		Module:    ModuleName,
		EventType: EventTypeDORAMetrics,
		Severity:  SeverityInformational,
		Payload:   payloadBytes,
		Labels:    labels,
	}
}

// reportError calls the OnError callback if configured.
func (p *Poller) reportError(category ErrorCategory, err error) {
	if p.onError != nil {
		p.onError(category, err)
	}
}

// logWarn logs a warning message if a logger is configured.
func (p *Poller) logWarn(format string, args ...any) {
	if p.logger != nil {
		p.logger.Printf("[WARN] poller: "+format, args...)
	}
}
