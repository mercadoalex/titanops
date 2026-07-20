package ebeecontrol

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"sort"
	"sync"
	"time"

	"github.com/google/uuid"
	export "github.com/mercadoalex/titanops/shared/titanops-export"
)

// Agent is the main eBeeControl autonomous deception engine.
// It wires all components and runs the continuous loop:
// Discover → Deploy → Detect → Assess → Respond → Report → Learn.
type Agent struct {
	config  Config
	logger  *log.Logger

	// Components
	dynatrace *DynatraceClient
	deployer  Deployer
	tetragon  *TetragonMonitor
	trainer   *Trainer
	emitter   EventEmitter

	// Internal state
	registry  *HoneytokenRegistry
	auditLog  *AuditLog

	// Response executor dependencies
	executorDeps ResponseExecutorDeps

	// Lifecycle
	cancel context.CancelFunc
	wg     sync.WaitGroup
}

// AgentDeps holds the external dependencies injected into the Agent.
type AgentDeps struct {
	Dynatrace *DynatraceClient
	Deployer  Deployer
	Tetragon  *TetragonMonitor
	Trainer   *Trainer
	Emitter   EventEmitter

	// Response execution functions
	IsolatePod        func(ctx context.Context, podID string) error
	BlockIP           func(ctx context.Context, podID string) error
	DeployHoneytokens func(ctx context.Context, namespace string, count int) error
	SendAlert         func(ctx context.Context, message string) error
}

// NewAgent creates a fully wired eBeeControl agent.
func NewAgent(cfg Config, deps AgentDeps) (*Agent, error) {
	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("invalid config: %w", err)
	}
	if deps.Dynatrace == nil {
		return nil, fmt.Errorf("dynatrace client is required")
	}
	if deps.Deployer == nil {
		return nil, fmt.Errorf("deployer is required")
	}
	if deps.Tetragon == nil {
		return nil, fmt.Errorf("tetragon monitor is required")
	}
	if deps.Trainer == nil {
		return nil, fmt.Errorf("trainer is required")
	}
	if deps.Emitter == nil {
		return nil, fmt.Errorf("event emitter is required")
	}

	a := &Agent{
		config:    cfg,
		logger:    log.New(os.Stderr, "[ebeecontrol] ", log.LstdFlags|log.Lmsgprefix),
		dynatrace: deps.Dynatrace,
		deployer:  deps.Deployer,
		tetragon:  deps.Tetragon,
		trainer:   deps.Trainer,
		emitter:   deps.Emitter,
		registry:  NewHoneytokenRegistry(),
		auditLog:  NewAuditLog(cfg.AuditLog.RetentionDays),
		executorDeps: ResponseExecutorDeps{
			IsolatePod:        deps.IsolatePod,
			BlockIP:           deps.BlockIP,
			DeployHoneytokens: deps.DeployHoneytokens,
			SendAlert:         deps.SendAlert,
		},
	}

	return a, nil
}

// Start begins all agent operations: Tetragon monitoring, discovery cycle,
// event flow connection, and retraining scheduler. Non-blocking.
func (a *Agent) Start(ctx context.Context) error {
	agentCtx, cancel := context.WithCancel(ctx)
	a.cancel = cancel

	// Start Tetragon monitor.
	if err := a.tetragon.Start(agentCtx); err != nil {
		cancel()
		return fmt.Errorf("starting tetragon monitor: %w", err)
	}

	// Connect event flow: Tetragon events → full workflow.
	a.tetragon.OnEvent(func(event AccessEvent) {
		go func() {
			if err := a.processAccessEvent(agentCtx, event); err != nil {
				a.logger.Printf("workflow error for event %s: %v", event.EventID, err)
			}
		}()
	})

	// Also connect via Dynatrace callback path.
	a.dynatrace.OnAccessEvent(func(event AccessEvent) {
		go func() {
			if err := a.processAccessEvent(agentCtx, event); err != nil {
				a.logger.Printf("workflow error for event %s: %v", event.EventID, err)
			}
		}()
	})

	// Start discovery cycle.
	a.wg.Add(1)
	go a.runDiscoveryCycle(agentCtx)

	// Start retraining scheduler.
	a.wg.Add(1)
	go a.runRetrainingScheduler(agentCtx)

	a.logger.Printf("agent started — discovery every %s, retraining check every %s",
		a.config.Discovery.Interval, a.config.Learning.RetrainingInterval)

	return nil
}

// Stop gracefully shuts down the agent and waits for goroutines to exit.
func (a *Agent) Stop() {
	if a.cancel != nil {
		a.cancel()
	}
	a.tetragon.Stop()
	a.wg.Wait()
	a.logger.Println("agent stopped")
}

// processAccessEvent runs the full workflow for a single access event:
// Assess → Respond → Report → Learn → Emit.
func (a *Agent) processAccessEvent(ctx context.Context, event AccessEvent) error {
	start := time.Now()

	// 1. Assess threat — query pod context, classify.
	podCtx := a.dynatrace.GetPodContext(ctx, event.PodID, event.Namespace)

	var classification ThreatClassification
	if podCtx != nil {
		classification = ClassifyThreat(*podCtx)
	} else {
		// Missing context → highest-risk defaults.
		classification = ClassifyThreatWithDefaults("", 0, 0)
	}

	assessment := ThreatAssessment{
		AssessmentID:  uuid.New().String(),
		AccessEventID: event.EventID,
		Classification: classification,
		AssessmentTime: time.Now().UTC(),
		AssessmentLatency: time.Since(start),
	}
	if podCtx != nil {
		assessment.Inputs = ThreatInputs{
			NamespaceClassification: podCtx.NamespaceClassification,
			ServiceCriticality:      podCtx.ServiceCriticality,
			DavisAnomalyScore:       podCtx.DavisAnomalyScore,
		}
	} else {
		assessment.Inputs = ThreatInputs{
			NamespaceClassification: NamespaceProduction,
			ServiceCriticality:      5,
			DavisAnomalyScore:       1.0,
		}
	}

	// Mark honeytoken as triggered in registry.
	a.registry.MarkTriggered(event.PodID, event.HoneytokenPath, event.Timestamp)

	// 2. Execute response based on classification.
	plan := GenerateResponsePlan(assessment, event.Namespace, event.PodID)
	responseResult := ExecuteResponse(ctx, plan, assessment, a.config.Response, a.executorDeps)

	// 3. Emit event for correlation engine.
	a.emitWorkflowEvent(ctx, event, assessment, responseResult)

	// 4. Submit outcome data to trainer.
	entry := a.registry.FindByPath(event.PodID, event.HoneytokenPath)
	honeytokenType := HoneytokenDecoyFile
	if entry != nil {
		honeytokenType = entry.Type
	}

	outcomeData := OutcomeData{
		IncidentID:        uuid.New().String(),
		AccessEvent:       event,
		HoneytokenType:    honeytokenType,
		PlacementLocation: event.HoneytokenPath,
		ActionsTaken:      responseResult.Actions,
		Effectiveness: Effectiveness{
			DetectionToResponseLatency: time.Since(start),
			ThreatContained:            responseResult.AllSucceeded,
			FalsePositive:              false,
		},
		Timestamp: time.Now().UTC(),
	}

	if _, err := a.trainer.IngestOutcomeData(outcomeData); err != nil {
		a.logger.Printf("failed to ingest outcome data: %v", err)
	}

	// 5. Audit log.
	a.auditLog.Log(AuditLogEntry{
		EntryID:           uuid.New().String(),
		Timestamp:         time.Now().UTC(),
		DecisionType:      DecisionResponse,
		DecisionRationale: fmt.Sprintf("Full workflow completed for event %s: %s threat", event.EventID, classification),
		InputDataSummary:  fmt.Sprintf("eventId=%s podId=%s namespace=%s classification=%s", event.EventID, event.PodID, event.Namespace, classification),
		Outcome:           fmt.Sprintf("Response: %v. Learning: ingested.", responseResult.AllSucceeded),
		RetentionDays:     a.config.AuditLog.RetentionDays,
	})

	return nil
}

// runDiscoveryCycle periodically discovers high-risk services and deploys honeytokens.
func (a *Agent) runDiscoveryCycle(ctx context.Context) {
	defer a.wg.Done()

	ticker := time.NewTicker(a.config.Discovery.Interval)
	defer ticker.Stop()

	// Run one cycle immediately on start.
	a.executeDiscovery(ctx)

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			a.executeDiscovery(ctx)
		}
	}
}

// executeDiscovery performs a single discovery + deployment cycle.
func (a *Agent) executeDiscovery(ctx context.Context) {
	services, err := a.dynatrace.QueryHighRiskServices(ctx)
	if err != nil {
		a.logger.Printf("discovery failed: %v", err)
		return
	}

	if len(services) == 0 {
		return
	}

	// Rank services by risk score (highest first).
	sort.Slice(services, func(i, j int) bool {
		return services[i].RiskScore > services[j].RiskScore
	})

	totalDeployed := 0
	totalFailed := 0

	for _, svc := range services {
		for _, podID := range svc.PodIdentifiers {
			specs := generateHoneytokenSpecs(svc)
			req := DeploymentRequest{
				PodID:       podID,
				Namespace:   svc.Namespace,
				Honeytokens: specs,
			}

			resp, err := a.deployer.Deploy(ctx, req)
			if err != nil {
				a.logger.Printf("deployment error for pod %s: %v", podID, err)
				totalFailed++
				continue
			}

			if resp.Success {
				totalDeployed += len(resp.DeployedHoneytokens)
				// Register honeytokens and Tetragon paths.
				for _, ht := range resp.DeployedHoneytokens {
					a.registry.Register(ht)
					a.tetragon.RegisterPath(ht.FilePath)
				}
			} else {
				totalFailed++
			}
		}
	}

	a.auditLog.Log(AuditLogEntry{
		EntryID:           uuid.New().String(),
		Timestamp:         time.Now().UTC(),
		DecisionType:      DecisionDeployment,
		DecisionRationale: fmt.Sprintf("Deployed honeytokens after discovery cycle (%d services found)", len(services)),
		InputDataSummary:  fmt.Sprintf("services=%d deployed=%d failed=%d", len(services), totalDeployed, totalFailed),
		Outcome:           fmt.Sprintf("Deployed %d honeytokens total", totalDeployed),
		RetentionDays:     a.config.AuditLog.RetentionDays,
	})

	a.logger.Printf("discovery cycle complete: %d services, %d honeytokens deployed, %d failures",
		len(services), totalDeployed, totalFailed)
}

// runRetrainingScheduler periodically checks if retraining should be triggered.
func (a *Agent) runRetrainingScheduler(ctx context.Context) {
	defer a.wg.Done()

	ticker := time.NewTicker(a.config.Learning.RetrainingInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if a.trainer.ShouldRetrain() {
				result := a.trainer.TriggerRetraining()
				if result.Success && result.NewModel != nil {
					a.auditLog.Log(AuditLogEntry{
						EntryID:           uuid.New().String(),
						Timestamp:         time.Now().UTC(),
						DecisionType:      DecisionModelUpdate,
						DecisionRationale: "New placement model published after retraining",
						InputDataSummary:  fmt.Sprintf("model=%s accuracy=%.0f%%", result.NewModel.VersionID, result.NewModel.ValidationAccuracy),
						Outcome:           result.Reason,
						RetentionDays:     a.config.AuditLog.RetentionDays,
					})
				}
			}
		}
	}
}

// emitWorkflowEvent publishes a correlation event for the completed workflow.
func (a *Agent) emitWorkflowEvent(ctx context.Context, event AccessEvent, assessment ThreatAssessment, result ResponseExecutionResult) {
	payload := map[string]interface{}{
		"access_event_id":  event.EventID,
		"pod_id":           event.PodID,
		"namespace":        event.Namespace,
		"honeytoken_path":  event.HoneytokenPath,
		"classification":   assessment.Classification,
		"all_succeeded":    result.AllSucceeded,
		"actions_count":    len(result.Actions),
		"latency_ms":       assessment.AssessmentLatency.Milliseconds(),
	}

	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		a.logger.Printf("failed to marshal event payload: %v", err)
		return
	}

	evt := export.Event{
		Namespace: event.Namespace,
		Timestamp: time.Now().UTC(),
		Severity:  string(assessment.Classification),
		Module:    "ebeecontrol",
		EventType: "honeytoken_access_response",
		Payload:   payloadBytes,
		Pod:       event.PodID,
		EventID:   fmt.Sprintf("ebc-%s", event.EventID[:8]),
		Labels: map[string]string{
			"classification": string(assessment.Classification),
			"all_succeeded":  fmt.Sprintf("%v", result.AllSucceeded),
			"pod_id":         event.PodID,
			"namespace":      event.Namespace,
		},
	}

	if err := a.emitter.Emit(ctx, evt); err != nil {
		a.logger.Printf("failed to emit event: %v", err)
	}
}

// generateHoneytokenSpecs creates honeytoken specs based on a high-risk service.
func generateHoneytokenSpecs(svc HighRiskService) []HoneytokenSpec {
	return []HoneytokenSpec{
		{
			Type:      HoneytokenDecoySecret,
			Name:      "service-account-token",
			Placement: fmt.Sprintf("/var/run/secrets/%s/token", svc.ServiceName),
		},
		{
			Type:      HoneytokenDecoyFile,
			Name:      "aws-credentials",
			Placement: fmt.Sprintf("/home/app/.aws/credentials-%s", svc.ServiceName),
		},
	}
}

// --- Honeytoken Registry ---

// HoneytokenRegistry tracks all deployed honeytokens.
type HoneytokenRegistry struct {
	mu      sync.RWMutex
	entries map[string]*HoneytokenRegistryEntry
}

// NewHoneytokenRegistry creates an empty registry.
func NewHoneytokenRegistry() *HoneytokenRegistry {
	return &HoneytokenRegistry{
		entries: make(map[string]*HoneytokenRegistryEntry),
	}
}

// Register adds a deployed honeytoken to the registry.
func (r *HoneytokenRegistry) Register(ht DeployedHoneytoken) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.entries[ht.HoneytokenID] = &HoneytokenRegistryEntry{
		HoneytokenID:        ht.HoneytokenID,
		PodID:               ht.PodID,
		Namespace:           ht.Namespace,
		Type:                ht.Type,
		FilePath:            ht.FilePath,
		DeploymentTimestamp: ht.DeploymentTimestamp,
		Status:              HoneytokenActive,
		AccessCount:         0,
	}
}

// MarkTriggered updates a honeytoken's status to triggered based on pod+path match.
func (r *HoneytokenRegistry) MarkTriggered(podID, filePath string, accessTime time.Time) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, entry := range r.entries {
		if entry.PodID == podID && entry.FilePath == filePath {
			entry.Status = HoneytokenTriggered
			entry.LastAccessTimestamp = &accessTime
			entry.AccessCount++
			return
		}
	}
}

// FindByPath finds a registry entry matching pod+path.
func (r *HoneytokenRegistry) FindByPath(podID, filePath string) *HoneytokenRegistryEntry {
	r.mu.RLock()
	defer r.mu.RUnlock()
	for _, entry := range r.entries {
		if entry.PodID == podID && entry.FilePath == filePath {
			cp := *entry
			return &cp
		}
	}
	return nil
}

// GetAll returns all registry entries.
func (r *HoneytokenRegistry) GetAll() []HoneytokenRegistryEntry {
	r.mu.RLock()
	defer r.mu.RUnlock()
	entries := make([]HoneytokenRegistryEntry, 0, len(r.entries))
	for _, e := range r.entries {
		entries = append(entries, *e)
	}
	return entries
}

// --- Audit Log ---

// AuditLog stores autonomous decision records.
type AuditLog struct {
	mu            sync.Mutex
	entries       []AuditLogEntry
	retentionDays int
}

// NewAuditLog creates an audit log with the given retention policy.
func NewAuditLog(retentionDays int) *AuditLog {
	if retentionDays < 90 {
		retentionDays = 90
	}
	return &AuditLog{
		retentionDays: retentionDays,
	}
}

// Log appends an entry to the audit log.
func (al *AuditLog) Log(entry AuditLogEntry) {
	al.mu.Lock()
	defer al.mu.Unlock()
	if entry.RetentionDays == 0 {
		entry.RetentionDays = al.retentionDays
	}
	al.entries = append(al.entries, entry)
}

// Entries returns all audit log entries.
func (al *AuditLog) Entries() []AuditLogEntry {
	al.mu.Lock()
	defer al.mu.Unlock()
	entries := make([]AuditLogEntry, len(al.entries))
	copy(entries, al.entries)
	return entries
}
