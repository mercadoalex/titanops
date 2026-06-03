package earthworm

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	ai "github.com/mercadoalex/titanops/shared/titanops-ai"
	export "github.com/mercadoalex/titanops/shared/titanops-export"
	k8s "github.com/mercadoalex/titanops/shared/titanops-k8s"
)

// --- Mock implementations ---

// mockProvider implements ai.Provider for testing.
type mockProvider struct {
	predictFunc func(ctx context.Context, req ai.PredictRequest) (ai.PredictResponse, error)
}

func (m *mockProvider) Predict(ctx context.Context, req ai.PredictRequest) (ai.PredictResponse, error) {
	if m.predictFunc != nil {
		return m.predictFunc(ctx, req)
	}
	return ai.PredictResponse{Score: 0.5, Confidence: 0.8}, nil
}

func (m *mockProvider) Train(ctx context.Context, req ai.TrainRequest) (ai.TrainResponse, error) {
	return ai.TrainResponse{}, nil
}

func (m *mockProvider) Explain(ctx context.Context, req ai.ExplainRequest) (ai.ExplainResponse, error) {
	return ai.ExplainResponse{}, nil
}

// mockEmitter implements EventEmitter for testing.
type mockEmitter struct {
	mu     sync.Mutex
	events []export.Event
	err    error
}

func (m *mockEmitter) Emit(ctx context.Context, event export.Event) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.err != nil {
		return m.err
	}
	m.events = append(m.events, event)
	return nil
}

func (m *mockEmitter) getEvents() []export.Event {
	m.mu.Lock()
	defer m.mu.Unlock()
	result := make([]export.Event, len(m.events))
	copy(result, m.events)
	return result
}

// mockK8sClient implements k8s.Client for testing.
type mockK8sClient struct {
	pods         []k8s.PodInfo
	cordonedNodes []string
	restartedPods []string
	deletedPods   []string
	err          error
}

func (m *mockK8sClient) ReadSecret(ctx context.Context, namespace, name, key string) ([]byte, error) {
	return nil, nil
}

func (m *mockK8sClient) ListPods(ctx context.Context, namespace string, selector map[string]string) ([]k8s.PodInfo, error) {
	if m.err != nil {
		return nil, m.err
	}
	return m.pods, nil
}

func (m *mockK8sClient) DeletePod(ctx context.Context, namespace, name string) error {
	if m.err != nil {
		return m.err
	}
	m.deletedPods = append(m.deletedPods, fmt.Sprintf("%s/%s", namespace, name))
	return nil
}

func (m *mockK8sClient) CordonNode(ctx context.Context, nodeName string) error {
	if m.err != nil {
		return m.err
	}
	m.cordonedNodes = append(m.cordonedNodes, nodeName)
	return nil
}

func (m *mockK8sClient) RestartPod(ctx context.Context, namespace, name string) error {
	if m.err != nil {
		return m.err
	}
	m.restartedPods = append(m.restartedPods, fmt.Sprintf("%s/%s", namespace, name))
	return nil
}

// --- Test helpers ---

func newTestAgent(t *testing.T, provider ai.Provider, emitter EventEmitter, k8sClient k8s.Client) *Agent {
	t.Helper()
	cfg := AgentConfig{
		Threshold:      0.75,
		NodeID:         "node-1",
		RuleThresholds: DefaultRuleThresholds(),
	}
	agent, err := NewAgent(cfg, provider, emitter, k8sClient)
	if err != nil {
		t.Fatalf("failed to create agent: %v", err)
	}
	return agent
}

func newTestHeartbeat(cpuUsage, memUsage float64) HeartbeatSignal {
	return HeartbeatSignal{
		NodeID:      "node-1",
		CPUUsage:    cpuUsage,
		MemoryUsage: memUsage,
		DiskIO:      0.3,
		NetworkIO:   0.2,
		Latency:     100,
		Timestamp:   time.Now().UTC(),
	}
}

// --- Unit Tests ---

func TestNewAgent_ValidConfig(t *testing.T) {
	provider := &mockProvider{}
	emitter := &mockEmitter{}
	k8sClient := &mockK8sClient{}

	agent, err := NewAgent(AgentConfig{
		Threshold:      0.8,
		NodeID:         "test-node",
		RuleThresholds: DefaultRuleThresholds(),
	}, provider, emitter, k8sClient)

	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if agent == nil {
		t.Fatal("expected agent to be non-nil")
	}
	if agent.config.Threshold != 0.8 {
		t.Errorf("expected threshold 0.8, got %f", agent.config.Threshold)
	}
}

func TestNewAgent_InvalidThreshold(t *testing.T) {
	provider := &mockProvider{}
	emitter := &mockEmitter{}
	k8sClient := &mockK8sClient{}

	_, err := NewAgent(AgentConfig{
		Threshold: 1.5, // invalid
		NodeID:    "test-node",
	}, provider, emitter, k8sClient)

	if err == nil {
		t.Fatal("expected error for invalid threshold")
	}
}

func TestNewAgent_MissingNodeID(t *testing.T) {
	provider := &mockProvider{}
	emitter := &mockEmitter{}
	k8sClient := &mockK8sClient{}

	_, err := NewAgent(AgentConfig{
		Threshold: 0.75,
		NodeID:    "", // missing
	}, provider, emitter, k8sClient)

	if err == nil {
		t.Fatal("expected error for missing node ID")
	}
}

func TestNewAgent_NilDependencies(t *testing.T) {
	emitter := &mockEmitter{}
	k8sClient := &mockK8sClient{}

	_, err := NewAgent(AgentConfig{
		Threshold: 0.75,
		NodeID:    "test-node",
	}, nil, emitter, k8sClient)

	if err == nil {
		t.Fatal("expected error for nil provider")
	}
}

func TestProcessHeartbeat_ScoreAboveThreshold_RemediationExecuted(t *testing.T) {
	provider := &mockProvider{
		predictFunc: func(ctx context.Context, req ai.PredictRequest) (ai.PredictResponse, error) {
			return ai.PredictResponse{Score: 0.85, Confidence: 0.9}, nil
		},
	}
	emitter := &mockEmitter{}
	k8sClient := &mockK8sClient{
		pods: []k8s.PodInfo{
			{Name: "pod-1", Namespace: "default", NodeName: "node-1", Status: "Running"},
		},
	}

	agent := newTestAgent(t, provider, emitter, k8sClient)
	heartbeat := newTestHeartbeat(0.8, 0.7)

	result, err := agent.ProcessHeartbeat(context.Background(), heartbeat)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result == nil {
		t.Fatal("expected action result, got nil")
	}
	if result.Confidence != 0.85 {
		t.Errorf("expected confidence 0.85, got %f", result.Confidence)
	}
	if result.ActionType == "" {
		t.Error("expected action type to be set")
	}
	if result.NodeID != "node-1" {
		t.Errorf("expected node ID 'node-1', got %q", result.NodeID)
	}
	if result.WasRuleBased {
		t.Error("expected WasRuleBased to be false")
	}

	// Verify remediation was executed.
	if len(k8sClient.restartedPods) == 0 {
		t.Error("expected at least one pod to be restarted")
	}
}

func TestProcessHeartbeat_ScoreBelowThreshold_NoAction(t *testing.T) {
	provider := &mockProvider{
		predictFunc: func(ctx context.Context, req ai.PredictRequest) (ai.PredictResponse, error) {
			return ai.PredictResponse{Score: 0.3, Confidence: 0.7}, nil
		},
	}
	emitter := &mockEmitter{}
	k8sClient := &mockK8sClient{}

	agent := newTestAgent(t, provider, emitter, k8sClient)
	heartbeat := newTestHeartbeat(0.4, 0.3)

	result, err := agent.ProcessHeartbeat(context.Background(), heartbeat)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != nil {
		t.Fatalf("expected nil result (no action), got: %+v", result)
	}

	// Verify no remediation was executed.
	if len(k8sClient.restartedPods) != 0 {
		t.Error("expected no pods to be restarted")
	}
	if len(k8sClient.cordonedNodes) != 0 {
		t.Error("expected no nodes to be cordoned")
	}

	// Verify no events were emitted.
	events := emitter.getEvents()
	if len(events) != 0 {
		t.Errorf("expected no events emitted, got %d", len(events))
	}
}

func TestProcessHeartbeat_ModelFailure_FallsBackToRules(t *testing.T) {
	provider := &mockProvider{
		predictFunc: func(ctx context.Context, req ai.PredictRequest) (ai.PredictResponse, error) {
			return ai.PredictResponse{}, &ai.AIError{
				Category: ai.ErrModelLoadFailed,
				Module:   "earthworm",
				Message:  "model file corrupt",
				Path:     "/models/earthworm-anomaly.onnx",
			}
		},
	}
	emitter := &mockEmitter{}
	k8sClient := &mockK8sClient{
		pods: []k8s.PodInfo{
			{Name: "pod-1", Namespace: "default", NodeName: "node-1", Status: "Running"},
		},
	}

	agent := newTestAgent(t, provider, emitter, k8sClient)
	// High CPU should trigger rule-based detection.
	heartbeat := HeartbeatSignal{
		NodeID:      "node-1",
		CPUUsage:    0.95,
		MemoryUsage: 0.5,
		DiskIO:      0.3,
		NetworkIO:   0.2,
		Latency:     100,
		Timestamp:   time.Now().UTC(),
	}

	result, err := agent.ProcessHeartbeat(context.Background(), heartbeat)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result == nil {
		t.Fatal("expected action result from rule-based fallback")
	}
	if !result.WasRuleBased {
		t.Error("expected WasRuleBased to be true")
	}
	if result.Confidence < 0.75 {
		t.Errorf("expected confidence >= 0.75, got %f", result.Confidence)
	}
}

func TestProcessHeartbeat_ModelUnavailable_FallsBackToRules(t *testing.T) {
	provider := &mockProvider{
		predictFunc: func(ctx context.Context, req ai.PredictRequest) (ai.PredictResponse, error) {
			return ai.PredictResponse{}, &ai.AIError{
				Category: ai.ErrModelUnavailable,
				Module:   "earthworm",
				Message:  "model file not found",
				Path:     "/models/earthworm-anomaly.onnx",
			}
		},
	}
	emitter := &mockEmitter{}
	k8sClient := &mockK8sClient{
		pods: []k8s.PodInfo{
			{Name: "pod-1", Namespace: "default", NodeName: "node-1", Status: "Running"},
		},
	}

	agent := newTestAgent(t, provider, emitter, k8sClient)
	// CPU below threshold — should not trigger rules either.
	heartbeat := newTestHeartbeat(0.5, 0.5)

	result, err := agent.ProcessHeartbeat(context.Background(), heartbeat)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Rule-based should not trigger with low metrics.
	if result != nil {
		t.Fatal("expected no action when metrics are below rule thresholds")
	}
}

func TestProcessHeartbeat_InferenceTimeout_FallsBackToRules(t *testing.T) {
	provider := &mockProvider{
		predictFunc: func(ctx context.Context, req ai.PredictRequest) (ai.PredictResponse, error) {
			return ai.PredictResponse{}, &ai.AIError{
				Category: ai.ErrInferenceTimeout,
				Module:   "earthworm",
				Message:  "inference exceeded 10s",
				Path:     "/models/earthworm-anomaly.onnx",
			}
		},
	}
	emitter := &mockEmitter{}
	k8sClient := &mockK8sClient{
		pods: []k8s.PodInfo{
			{Name: "pod-1", Namespace: "default", NodeName: "node-1", Status: "Running"},
		},
	}

	agent := newTestAgent(t, provider, emitter, k8sClient)
	// High memory to trigger rules.
	heartbeat := HeartbeatSignal{
		NodeID:      "node-1",
		CPUUsage:    0.5,
		MemoryUsage: 0.95,
		DiskIO:      0.3,
		NetworkIO:   0.2,
		Latency:     100,
		Timestamp:   time.Now().UTC(),
	}

	result, err := agent.ProcessHeartbeat(context.Background(), heartbeat)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result == nil {
		t.Fatal("expected action from rule-based fallback")
	}
	if !result.WasRuleBased {
		t.Error("expected WasRuleBased to be true")
	}
}

func TestProcessHeartbeat_EventEmittedOnAction(t *testing.T) {
	provider := &mockProvider{
		predictFunc: func(ctx context.Context, req ai.PredictRequest) (ai.PredictResponse, error) {
			return ai.PredictResponse{Score: 0.9, Confidence: 0.95}, nil
		},
	}
	emitter := &mockEmitter{}
	k8sClient := &mockK8sClient{
		pods: []k8s.PodInfo{
			{Name: "pod-1", Namespace: "default", NodeName: "node-1", Status: "Running"},
		},
	}

	agent := newTestAgent(t, provider, emitter, k8sClient)
	heartbeat := HeartbeatSignal{
		NodeID:      "node-1",
		CPUUsage:    0.85,
		MemoryUsage: 0.7,
		DiskIO:      0.4,
		NetworkIO:   0.3,
		Latency:     200,
		Timestamp:   time.Now().UTC(),
	}

	result, err := agent.ProcessHeartbeat(context.Background(), heartbeat)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result == nil {
		t.Fatal("expected action result")
	}

	// Verify event was emitted.
	events := emitter.getEvents()
	if len(events) != 1 {
		t.Fatalf("expected 1 event emitted, got %d", len(events))
	}

	event := events[0]

	// Verify required event fields per requirement 10.4.
	if event.Node == "" {
		t.Error("event missing node identifier")
	}
	if event.Node != "node-1" {
		t.Errorf("expected node 'node-1', got %q", event.Node)
	}
	if event.Module != "earthworm" {
		t.Errorf("expected module 'earthworm', got %q", event.Module)
	}
	if event.EventType != "autonomous_remediation" {
		t.Errorf("expected event type 'autonomous_remediation', got %q", event.EventType)
	}
	if event.Timestamp.IsZero() {
		t.Error("event missing timestamp")
	}
	if len(event.Payload) == 0 {
		t.Error("event missing payload (should contain heartbeat metrics)")
	}
	if event.Severity != "high" {
		t.Errorf("expected severity 'high', got %q", event.Severity)
	}
	if event.EventID == "" {
		t.Error("event missing event ID")
	}

	// Verify labels contain action type and confidence.
	if event.Labels["action_type"] == "" {
		t.Error("event labels missing action_type")
	}
	if event.Labels["confidence"] == "" {
		t.Error("event labels missing confidence")
	}
	if event.Labels["node_id"] != "node-1" {
		t.Errorf("expected label node_id='node-1', got %q", event.Labels["node_id"])
	}
}

func TestProcessHeartbeat_ContextDeadlineExceeded_FallsBackToRules(t *testing.T) {
	provider := &mockProvider{
		predictFunc: func(ctx context.Context, req ai.PredictRequest) (ai.PredictResponse, error) {
			return ai.PredictResponse{}, context.DeadlineExceeded
		},
	}
	emitter := &mockEmitter{}
	k8sClient := &mockK8sClient{
		pods: []k8s.PodInfo{
			{Name: "pod-1", Namespace: "default", NodeName: "node-1", Status: "Running"},
		},
	}

	agent := newTestAgent(t, provider, emitter, k8sClient)
	// High latency to trigger rule.
	heartbeat := HeartbeatSignal{
		NodeID:      "node-1",
		CPUUsage:    0.5,
		MemoryUsage: 0.5,
		DiskIO:      0.3,
		NetworkIO:   0.2,
		Latency:     600,
		Timestamp:   time.Now().UTC(),
	}

	result, err := agent.ProcessHeartbeat(context.Background(), heartbeat)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result == nil {
		t.Fatal("expected action result from rule-based fallback (latency)")
	}
	if !result.WasRuleBased {
		t.Error("expected WasRuleBased to be true")
	}
}

func TestEvaluateRules_CPUThreshold(t *testing.T) {
	agent := &Agent{
		config: AgentConfig{
			RuleThresholds: DefaultRuleThresholds(),
		},
	}

	heartbeat := HeartbeatSignal{
		CPUUsage:    0.95,
		MemoryUsage: 0.5,
		Latency:     100,
	}

	score, rule := agent.evaluateRules(heartbeat)
	if rule != "cpu_threshold_exceeded" {
		t.Errorf("expected 'cpu_threshold_exceeded', got %q", rule)
	}
	if score < 0.75 || score > 1.0 {
		t.Errorf("expected score in [0.75, 1.0], got %f", score)
	}
}

func TestEvaluateRules_MemoryThreshold(t *testing.T) {
	agent := &Agent{
		config: AgentConfig{
			RuleThresholds: DefaultRuleThresholds(),
		},
	}

	heartbeat := HeartbeatSignal{
		CPUUsage:    0.5,
		MemoryUsage: 0.9,
		Latency:     100,
	}

	score, rule := agent.evaluateRules(heartbeat)
	if rule != "memory_threshold_exceeded" {
		t.Errorf("expected 'memory_threshold_exceeded', got %q", rule)
	}
	if score < 0.75 || score > 1.0 {
		t.Errorf("expected score in [0.75, 1.0], got %f", score)
	}
}

func TestEvaluateRules_LatencyThreshold(t *testing.T) {
	agent := &Agent{
		config: AgentConfig{
			RuleThresholds: DefaultRuleThresholds(),
		},
	}

	heartbeat := HeartbeatSignal{
		CPUUsage:    0.5,
		MemoryUsage: 0.5,
		Latency:     700,
	}

	score, rule := agent.evaluateRules(heartbeat)
	if rule != "latency_threshold_exceeded" {
		t.Errorf("expected 'latency_threshold_exceeded', got %q", rule)
	}
	if score < 0.75 || score > 1.0 {
		t.Errorf("expected score in [0.75, 1.0], got %f", score)
	}
}

func TestEvaluateRules_NoRuleTriggered(t *testing.T) {
	agent := &Agent{
		config: AgentConfig{
			RuleThresholds: DefaultRuleThresholds(),
		},
	}

	heartbeat := HeartbeatSignal{
		CPUUsage:    0.5,
		MemoryUsage: 0.5,
		Latency:     100,
	}

	score, rule := agent.evaluateRules(heartbeat)
	if rule != "" {
		t.Errorf("expected no rule triggered, got %q", rule)
	}
	if score != 0.0 {
		t.Errorf("expected score 0.0, got %f", score)
	}
}

func TestHeartbeatToFeatures(t *testing.T) {
	heartbeat := HeartbeatSignal{
		CPUUsage:    0.8,
		MemoryUsage: 0.6,
		DiskIO:      0.3,
		NetworkIO:   0.2,
		Latency:     500,
	}

	features := heartbeat.ToFeatures()
	if len(features) != 5 {
		t.Fatalf("expected 5 features, got %d", len(features))
	}
	if features[0] != float32(0.8) {
		t.Errorf("expected CPU feature 0.8, got %f", features[0])
	}
	if features[1] != float32(0.6) {
		t.Errorf("expected Memory feature 0.6, got %f", features[1])
	}
	// Latency normalized: 500/1000 = 0.5
	if features[4] != float32(0.5) {
		t.Errorf("expected normalized latency 0.5, got %f", features[4])
	}
}

func TestHeartbeatToFeatures_HighLatencyClamped(t *testing.T) {
	heartbeat := HeartbeatSignal{
		Latency: 2000, // above 1000ms max
	}

	features := heartbeat.ToFeatures()
	if features[4] != float32(1.0) {
		t.Errorf("expected clamped latency 1.0, got %f", features[4])
	}
}

func TestSelectAction_HighConfidenceHighResources_NodeCordon(t *testing.T) {
	heartbeat := HeartbeatSignal{
		CPUUsage:    0.95,
		MemoryUsage: 0.9,
		Latency:     100,
	}

	action := selectAction(heartbeat, 0.95)
	if action != ActionNodeCordon {
		t.Errorf("expected %q, got %q", ActionNodeCordon, action)
	}
}

func TestSelectAction_HighLatencyLowCPU_WorkloadReschedule(t *testing.T) {
	heartbeat := HeartbeatSignal{
		CPUUsage:    0.4,
		MemoryUsage: 0.5,
		Latency:     600,
	}

	action := selectAction(heartbeat, 0.8)
	if action != ActionWorkloadReschedule {
		t.Errorf("expected %q, got %q", ActionWorkloadReschedule, action)
	}
}

func TestSelectAction_Default_PodRestart(t *testing.T) {
	heartbeat := HeartbeatSignal{
		CPUUsage:    0.7,
		MemoryUsage: 0.6,
		Latency:     200,
	}

	action := selectAction(heartbeat, 0.8)
	if action != ActionPodRestart {
		t.Errorf("expected %q, got %q", ActionPodRestart, action)
	}
}

func TestProcessHeartbeat_EventEmitFailure_DoesNotFailAction(t *testing.T) {
	provider := &mockProvider{
		predictFunc: func(ctx context.Context, req ai.PredictRequest) (ai.PredictResponse, error) {
			return ai.PredictResponse{Score: 0.9, Confidence: 0.95}, nil
		},
	}
	emitter := &mockEmitter{err: errors.New("emit failed")}
	k8sClient := &mockK8sClient{
		pods: []k8s.PodInfo{
			{Name: "pod-1", Namespace: "default", NodeName: "node-1", Status: "Running"},
		},
	}

	agent := newTestAgent(t, provider, emitter, k8sClient)
	heartbeat := newTestHeartbeat(0.85, 0.7)

	// Should succeed even though event emission fails.
	result, err := agent.ProcessHeartbeat(context.Background(), heartbeat)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result == nil {
		t.Fatal("expected action result despite emit failure")
	}
}

func TestProcessHeartbeat_ContextCancelled_ReturnsError(t *testing.T) {
	provider := &mockProvider{
		predictFunc: func(ctx context.Context, req ai.PredictRequest) (ai.PredictResponse, error) {
			return ai.PredictResponse{}, context.Canceled
		},
	}
	emitter := &mockEmitter{}
	k8sClient := &mockK8sClient{}

	agent := newTestAgent(t, provider, emitter, k8sClient)
	heartbeat := newTestHeartbeat(0.5, 0.5)

	_, err := agent.ProcessHeartbeat(context.Background(), heartbeat)
	if err == nil {
		t.Fatal("expected error for cancelled context")
	}
}

func TestValidateConfig_DefaultThreshold(t *testing.T) {
	cfg := &AgentConfig{
		NodeID: "test",
	}
	err := cfg.ValidateConfig()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Threshold != defaultThreshold {
		t.Errorf("expected default threshold %f, got %f", defaultThreshold, cfg.Threshold)
	}
}

func TestValidateConfig_DefaultRuleThresholds(t *testing.T) {
	cfg := &AgentConfig{
		NodeID:    "test",
		Threshold: 0.75,
	}
	err := cfg.ValidateConfig()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defaults := DefaultRuleThresholds()
	if cfg.RuleThresholds.CPUThreshold != defaults.CPUThreshold {
		t.Errorf("expected CPU threshold %f, got %f", defaults.CPUThreshold, cfg.RuleThresholds.CPUThreshold)
	}
	if cfg.RuleThresholds.MemoryThreshold != defaults.MemoryThreshold {
		t.Errorf("expected Memory threshold %f, got %f", defaults.MemoryThreshold, cfg.RuleThresholds.MemoryThreshold)
	}
	if cfg.RuleThresholds.LatencyThreshold != defaults.LatencyThreshold {
		t.Errorf("expected Latency threshold %f, got %f", defaults.LatencyThreshold, cfg.RuleThresholds.LatencyThreshold)
	}
}
