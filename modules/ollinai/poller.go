package ollinai

import (
	"context"
	"errors"
	"log"
	"time"

	export "github.com/mercadoalex/titanops/shared/titanops-export"
)

// PollerConfig holds configuration for the Poller.
type PollerConfig struct {
	// APIClient is the interface for fetching data from the OllinAI API.
	// In live mode this is an HTTPAPIClient; in mock mode a MockAPIClient.
	APIClient OllinAPIClient
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

// Poller periodically fetches data from the OllinAI API and emits events.
// It depends on the OllinAPIClient interface — no HTTP, network, or infrastructure
// imports exist in this file. All infrastructure details are behind the interface.
type Poller struct {
	apiClient    OllinAPIClient
	riskInterval time.Duration
	doraInterval time.Duration
	emitter      EventEmitter
	onError      func(category ErrorCategory, err error)
	logger       *log.Logger
}

// NewPoller creates a new Poller with the given configuration.
func NewPoller(cfg PollerConfig) *Poller {
	riskInterval := cfg.RiskInterval
	if riskInterval <= 0 {
		riskInterval = 30 * time.Second
	}

	doraInterval := cfg.DORAInterval
	if doraInterval <= 0 {
		doraInterval = 5 * time.Minute
	}

	return &Poller{
		apiClient:    cfg.APIClient,
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

// pollDeploymentRisk fetches deployment risk data via the API client and emits events.
func (p *Poller) pollDeploymentRisk(ctx context.Context) {
	entries, err := p.apiClient.FetchDeploymentRisks(ctx)
	if err != nil {
		p.handleAPIError("FetchDeploymentRisks", err)
		return
	}

	for _, entry := range entries {
		event := p.buildDeploymentRiskEvent(entry)
		if err := p.emitter.Emit(ctx, event); err != nil {
			p.logWarn("failed to emit deployment risk event for service %s: %v", entry.Service, err)
		}
	}
}

// pollDORAMetrics fetches DORA metrics via the API client and emits an event.
func (p *Poller) pollDORAMetrics(ctx context.Context) {
	metrics, err := p.apiClient.FetchDORAMetrics(ctx)
	if err != nil {
		p.handleAPIError("FetchDORAMetrics", err)
		return
	}

	event := p.buildDORAMetricsEvent(metrics)
	if err := p.emitter.Emit(ctx, event); err != nil {
		p.logWarn("failed to emit DORA metrics event: %v", err)
	}
}

// handleAPIError extracts the error category from an APIError and reports it.
func (p *Poller) handleAPIError(method string, err error) {
	var apiErr *APIError
	if errors.As(err, &apiErr) {
		p.logWarn("%s: %s", method, apiErr.Message)
		p.reportError(apiErr.Category, err)
	} else {
		p.logWarn("%s: %v", method, err)
		p.reportError(ErrOllinAPIUnavailable, err)
	}
}

// buildDeploymentRiskEvent constructs an export.Event from a deployment risk entry.
func (p *Poller) buildDeploymentRiskEvent(entry DeploymentRiskEntry) export.Event {
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

// buildDORAMetricsEvent constructs an export.Event from DORA metrics.
func (p *Poller) buildDORAMetricsEvent(metrics *DORAMetrics) export.Event {
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
