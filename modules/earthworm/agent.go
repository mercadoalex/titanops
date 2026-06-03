package earthworm

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"os"
	"time"

	ai "github.com/mercadoalex/titanops/shared/titanops-ai"
	export "github.com/mercadoalex/titanops/shared/titanops-export"
	k8s "github.com/mercadoalex/titanops/shared/titanops-k8s"
)

const (
	// defaultThreshold is the default anomaly confidence threshold.
	defaultThreshold = 0.75
	// inferenceTimeout is the maximum time allowed for model inference.
	inferenceTimeout = 10 * time.Second
	// moduleID is the AI model module identifier for Earthworm.
	moduleID = "earthworm"
	// eventEmitTimeout is the maximum time allowed for event emission.
	eventEmitTimeout = 5 * time.Second
)

// AgentConfig holds the configuration for the Earthworm agent.
type AgentConfig struct {
	// ModelDir is the path to the directory containing ONNX models.
	ModelDir string
	// Threshold is the anomaly confidence score threshold for triggering
	// remediation actions. Range [0.1, 1.0], default 0.75.
	Threshold float64
	// NodeID is the Kubernetes node this agent monitors.
	NodeID string
	// RuleThresholds contains the static thresholds for rule-based fallback.
	RuleThresholds RuleThresholds
}

// ValidateConfig validates the agent configuration and applies defaults.
func (c *AgentConfig) ValidateConfig() error {
	if c.Threshold == 0 {
		c.Threshold = defaultThreshold
	}
	if c.Threshold < 0.1 || c.Threshold > 1.0 {
		return fmt.Errorf("threshold must be in range [0.1, 1.0], got %f", c.Threshold)
	}
	if c.NodeID == "" {
		return fmt.Errorf("node ID is required")
	}
	// Apply default rule thresholds if not set.
	if c.RuleThresholds.CPUThreshold == 0 {
		c.RuleThresholds.CPUThreshold = DefaultRuleThresholds().CPUThreshold
	}
	if c.RuleThresholds.MemoryThreshold == 0 {
		c.RuleThresholds.MemoryThreshold = DefaultRuleThresholds().MemoryThreshold
	}
	if c.RuleThresholds.LatencyThreshold == 0 {
		c.RuleThresholds.LatencyThreshold = DefaultRuleThresholds().LatencyThreshold
	}
	return nil
}

// Agent is the Earthworm autonomous node health agent.
// It analyzes heartbeat signals using AI inference and executes
// remediation actions when anomalies are detected above threshold.
type Agent struct {
	config   AgentConfig
	provider ai.Provider
	emitter  EventEmitter
	k8s      k8s.Client
	logger   *log.Logger
}

// NewAgent creates a new Earthworm agent with the given configuration and dependencies.
// Returns an error if the configuration is invalid.
func NewAgent(cfg AgentConfig, provider ai.Provider, emitter EventEmitter, k8sClient k8s.Client) (*Agent, error) {
	if err := cfg.ValidateConfig(); err != nil {
		return nil, fmt.Errorf("invalid agent config: %w", err)
	}
	if provider == nil {
		return nil, fmt.Errorf("AI provider is required")
	}
	if emitter == nil {
		return nil, fmt.Errorf("event emitter is required")
	}
	if k8sClient == nil {
		return nil, fmt.Errorf("Kubernetes client is required")
	}

	return &Agent{
		config:   cfg,
		provider: provider,
		emitter:  emitter,
		k8s:      k8sClient,
		logger:   log.New(os.Stderr, "[earthworm] ", log.LstdFlags|log.Lmsgprefix),
	}, nil
}

// ProcessHeartbeat analyzes a heartbeat signal for anomalies and takes
// appropriate action based on the confidence score and configured threshold.
//
// If the AI model produces a score >= threshold, remediation is executed
// and an event is emitted. If the score < threshold, the observation is
// logged without action.
//
// If the AI model fails (load error or inference timeout >10s), the agent
// falls back to rule-based threshold detection.
func (a *Agent) ProcessHeartbeat(ctx context.Context, heartbeat HeartbeatSignal) (*ActionResult, error) {
	score, ruleBased, err := a.getAnomalyScore(ctx, heartbeat)
	if err != nil {
		return nil, fmt.Errorf("anomaly detection failed: %w", err)
	}

	// Score below threshold: log observation, no action.
	if score < a.config.Threshold {
		a.logger.Printf("observation: node=%s score=%.4f threshold=%.4f rule_based=%v — no action",
			heartbeat.NodeID, score, a.config.Threshold, ruleBased)
		return nil, nil
	}

	// Score >= threshold: execute remediation.
	actionType := selectAction(heartbeat, score)
	result := &ActionResult{
		ActionType:   actionType,
		NodeID:       heartbeat.NodeID,
		Confidence:   score,
		Heartbeat:    heartbeat,
		Timestamp:    time.Now().UTC(),
		WasRuleBased: ruleBased,
	}

	// Execute the remediation action.
	if err := a.executeRemediation(ctx, result); err != nil {
		return nil, fmt.Errorf("remediation execution failed: %w", err)
	}

	// Emit event for the autonomous action.
	if err := a.emitActionEvent(ctx, result); err != nil {
		// Log but don't fail the action if event emission fails.
		a.logger.Printf("warning: failed to emit action event: %v", err)
	}

	a.logger.Printf("action: node=%s type=%s score=%.4f rule_based=%v",
		heartbeat.NodeID, actionType, score, ruleBased)

	return result, nil
}

// getAnomalyScore attempts AI inference first, falling back to rule-based
// detection on model failure or timeout.
func (a *Agent) getAnomalyScore(ctx context.Context, heartbeat HeartbeatSignal) (float64, bool, error) {
	// Attempt AI inference with timeout.
	inferCtx, cancel := context.WithTimeout(ctx, inferenceTimeout)
	defer cancel()

	features := heartbeat.ToFeatures()
	resp, err := a.provider.Predict(inferCtx, ai.PredictRequest{
		ModuleID: moduleID,
		Features: features,
	})

	if err == nil {
		// AI inference succeeded.
		return resp.Score, false, nil
	}

	// AI inference failed — determine if it's a recoverable failure.
	var aiErr *ai.AIError
	if errors.As(err, &aiErr) {
		switch aiErr.Category {
		case ai.ErrModelUnavailable, ai.ErrModelLoadFailed, ai.ErrInferenceTimeout:
			a.logger.Printf("warning: AI model failure (%s), falling back to rule-based detection: %v",
				aiErr.Category, err)
			return a.fallbackToRules(heartbeat)
		case ai.ErrInvalidInput:
			return 0, false, fmt.Errorf("invalid input for AI inference: %w", err)
		}
	}

	// Context deadline exceeded (inference timeout >10s).
	if errors.Is(err, context.DeadlineExceeded) {
		a.logger.Printf("warning: AI inference timeout (>10s), falling back to rule-based detection")
		return a.fallbackToRules(heartbeat)
	}

	// Context cancelled by caller.
	if errors.Is(err, context.Canceled) {
		return 0, false, err
	}

	// Unknown error — fall back to rules.
	a.logger.Printf("warning: unexpected AI error, falling back to rule-based detection: %v", err)
	return a.fallbackToRules(heartbeat)
}

// fallbackToRules uses static threshold detection when AI is unavailable.
func (a *Agent) fallbackToRules(heartbeat HeartbeatSignal) (float64, bool, error) {
	score, rule := a.evaluateRules(heartbeat)
	if rule != "" {
		a.logger.Printf("degraded-mode: rule-based detection triggered rule=%s score=%.4f", rule, score)
	}
	return score, true, nil
}

// executeRemediation performs the appropriate Kubernetes action.
func (a *Agent) executeRemediation(ctx context.Context, result *ActionResult) error {
	switch result.ActionType {
	case ActionPodRestart:
		// List pods on the affected node and restart the first one.
		pods, err := a.k8s.ListPods(ctx, "", map[string]string{})
		if err != nil {
			return fmt.Errorf("failed to list pods for restart: %w", err)
		}
		for _, pod := range pods {
			if pod.NodeName == result.NodeID {
				if err := a.k8s.RestartPod(ctx, pod.Namespace, pod.Name); err != nil {
					return fmt.Errorf("failed to restart pod %s/%s: %w", pod.Namespace, pod.Name, err)
				}
				return nil
			}
		}
		return nil

	case ActionNodeCordon:
		if err := a.k8s.CordonNode(ctx, result.NodeID); err != nil {
			return fmt.Errorf("failed to cordon node %s: %w", result.NodeID, err)
		}
		return nil

	case ActionWorkloadReschedule:
		// Cordon node to prevent new scheduling, then restart pods.
		if err := a.k8s.CordonNode(ctx, result.NodeID); err != nil {
			return fmt.Errorf("failed to cordon node for reschedule %s: %w", result.NodeID, err)
		}
		pods, err := a.k8s.ListPods(ctx, "", map[string]string{})
		if err != nil {
			return fmt.Errorf("failed to list pods for reschedule: %w", err)
		}
		for _, pod := range pods {
			if pod.NodeName == result.NodeID {
				if err := a.k8s.RestartPod(ctx, pod.Namespace, pod.Name); err != nil {
					a.logger.Printf("warning: failed to restart pod %s/%s during reschedule: %v",
						pod.Namespace, pod.Name, err)
				}
			}
		}
		return nil

	default:
		return fmt.Errorf("unknown action type: %s", result.ActionType)
	}
}

// emitActionEvent publishes an event for an autonomous remediation action.
// The event must be emitted within 5 seconds per requirement 10.4.
func (a *Agent) emitActionEvent(ctx context.Context, result *ActionResult) error {
	emitCtx, cancel := context.WithTimeout(ctx, eventEmitTimeout)
	defer cancel()

	// Build payload with heartbeat metrics and action details.
	payload := map[string]interface{}{
		"node_id":     result.NodeID,
		"confidence":  result.Confidence,
		"action_type": result.ActionType,
		"heartbeat": map[string]interface{}{
			"cpu_usage":    result.Heartbeat.CPUUsage,
			"memory_usage": result.Heartbeat.MemoryUsage,
			"disk_io":      result.Heartbeat.DiskIO,
			"network_io":   result.Heartbeat.NetworkIO,
			"latency":      result.Heartbeat.Latency,
		},
		"rule_based": result.WasRuleBased,
	}

	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("failed to marshal event payload: %w", err)
	}

	event := export.Event{
		Namespace: "titanops",
		Timestamp: result.Timestamp,
		Severity:  "high",
		Module:    "earthworm",
		EventType: "autonomous_remediation",
		Payload:   payloadBytes,
		Node:      result.NodeID,
		EventID:   fmt.Sprintf("ew-%d", result.Timestamp.UnixNano()),
		Labels: map[string]string{
			"action_type":  result.ActionType,
			"confidence":   fmt.Sprintf("%.4f", result.Confidence),
			"rule_based":   fmt.Sprintf("%v", result.WasRuleBased),
			"node_id":      result.NodeID,
		},
	}

	return a.emitter.Emit(emitCtx, event)
}
