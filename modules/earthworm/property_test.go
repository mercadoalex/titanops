package earthworm

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	ai "github.com/mercadoalex/titanops/shared/titanops-ai"
	export "github.com/mercadoalex/titanops/shared/titanops-export"
	k8s "github.com/mercadoalex/titanops/shared/titanops-k8s"
	"pgregory.net/rapid"
)

// **Validates: Requirements 10.1, 10.2, 10.6**
// Property 16: Earthworm threshold-based remediation decision

// heartbeatGen generates a random HeartbeatSignal with metrics in valid ranges.
func heartbeatGen() *rapid.Generator[HeartbeatSignal] {
	return rapid.Custom(func(t *rapid.T) HeartbeatSignal {
		return HeartbeatSignal{
			NodeID:      rapid.StringMatching(`node-[a-z0-9]{1,8}`).Draw(t, "nodeID"),
			CPUUsage:    rapid.Float64Range(0.0, 1.0).Draw(t, "cpuUsage"),
			MemoryUsage: rapid.Float64Range(0.0, 1.0).Draw(t, "memoryUsage"),
			DiskIO:      rapid.Float64Range(0.0, 1.0).Draw(t, "diskIO"),
			NetworkIO:   rapid.Float64Range(0.0, 1.0).Draw(t, "networkIO"),
			Latency:     rapid.Float64Range(0.0, 1000.0).Draw(t, "latency"),
			Timestamp:   time.Now().UTC(),
		}
	})
}

// scoreProvider returns a mock AI provider that always returns the given score.
func scoreProvider(score float64) *mockPropProvider {
	return &mockPropProvider{score: score}
}

// mockPropProvider implements ai.Provider with a fixed score for property tests.
type mockPropProvider struct {
	score float64
}

func (m *mockPropProvider) Predict(_ context.Context, _ ai.PredictRequest) (ai.PredictResponse, error) {
	return ai.PredictResponse{Score: m.score, Confidence: m.score}, nil
}

func (m *mockPropProvider) Train(_ context.Context, _ ai.TrainRequest) (ai.TrainResponse, error) {
	return ai.TrainResponse{}, nil
}

func (m *mockPropProvider) Explain(_ context.Context, _ ai.ExplainRequest) (ai.ExplainResponse, error) {
	return ai.ExplainResponse{}, nil
}

// mockPropEmitter captures emitted events for property verification.
type mockPropEmitter struct {
	events []export.Event
}

func (m *mockPropEmitter) Emit(_ context.Context, event export.Event) error {
	m.events = append(m.events, event)
	return nil
}

// mockPropK8s implements k8s.Client for property tests.
type mockPropK8s struct {
	pods []k8s.PodInfo
}

func (m *mockPropK8s) ReadSecret(_ context.Context, _, _, _ string) ([]byte, error) {
	return nil, nil
}

func (m *mockPropK8s) ListPods(_ context.Context, _ string, _ map[string]string) ([]k8s.PodInfo, error) {
	return m.pods, nil
}

func (m *mockPropK8s) DeletePod(_ context.Context, _, _ string) error {
	return nil
}

func (m *mockPropK8s) CordonNode(_ context.Context, _ string) error {
	return nil
}

func (m *mockPropK8s) RestartPod(_ context.Context, _, _ string) error {
	return nil
}

func TestProperty16_ThresholdBasedRemediation(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		score := rapid.Float64Range(0.0, 1.0).Draw(t, "score")
		threshold := rapid.Float64Range(0.1, 1.0).Draw(t, "threshold")
		heartbeat := heartbeatGen().Draw(t, "heartbeat")

		provider := scoreProvider(score)
		emitter := &mockPropEmitter{}
		k8sClient := &mockPropK8s{
			pods: []k8s.PodInfo{
				{Name: "pod-1", Namespace: "default", NodeName: heartbeat.NodeID, Status: "Running"},
			},
		}

		cfg := AgentConfig{
			Threshold:      threshold,
			NodeID:         heartbeat.NodeID,
			RuleThresholds: DefaultRuleThresholds(),
		}
		agent, err := NewAgent(cfg, provider, emitter, k8sClient)
		if err != nil {
			t.Fatalf("NewAgent: %v", err)
		}

		result, err := agent.ProcessHeartbeat(context.Background(), heartbeat)
		if err != nil {
			t.Fatalf("ProcessHeartbeat: %v", err)
		}

		if score >= threshold {
			// (b) Remediation SHALL execute.
			if result == nil {
				t.Fatalf("expected remediation for score=%.4f >= threshold=%.4f, got nil", score, threshold)
			}
			// Confidence in [0.0, 1.0].
			if result.Confidence < 0.0 || result.Confidence > 1.0 {
				t.Errorf("confidence %.4f out of range [0.0, 1.0]", result.Confidence)
			}
		} else {
			// (c) Log without action.
			if result != nil {
				t.Fatalf("expected no remediation for score=%.4f < threshold=%.4f, got action %q",
					score, threshold, result.ActionType)
			}
		}
	})
}

// **Validates: Requirements 10.4**
// Property 17: Earthworm action event field completeness

func TestProperty17_ActionEventFieldCompleteness(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		heartbeat := heartbeatGen().Draw(t, "heartbeat")
		// Use a score that will always trigger remediation.
		score := rapid.Float64Range(0.75, 1.0).Draw(t, "score")

		provider := scoreProvider(score)
		emitter := &mockPropEmitter{}
		k8sClient := &mockPropK8s{
			pods: []k8s.PodInfo{
				{Name: "pod-1", Namespace: "default", NodeName: heartbeat.NodeID, Status: "Running"},
			},
		}

		cfg := AgentConfig{
			Threshold:      0.75,
			NodeID:         heartbeat.NodeID,
			RuleThresholds: DefaultRuleThresholds(),
		}
		agent, err := NewAgent(cfg, provider, emitter, k8sClient)
		if err != nil {
			t.Fatalf("NewAgent: %v", err)
		}

		result, err := agent.ProcessHeartbeat(context.Background(), heartbeat)
		if err != nil {
			t.Fatalf("ProcessHeartbeat: %v", err)
		}
		if result == nil {
			t.Fatal("expected remediation action for score >= threshold")
		}

		// Verify event was emitted.
		if len(emitter.events) == 0 {
			t.Fatal("expected event to be emitted on remediation action")
		}

		event := emitter.events[0]

		// Parse payload to verify required fields.
		var payload map[string]interface{}
		if err := json.Unmarshal(event.Payload, &payload); err != nil {
			t.Fatalf("failed to unmarshal event payload: %v", err)
		}

		// Contains node_id.
		nodeID, ok := payload["node_id"]
		if !ok || nodeID == "" {
			t.Error("event payload missing node_id")
		}

		// Contains confidence.
		confidence, ok := payload["confidence"]
		if !ok {
			t.Error("event payload missing confidence")
		}
		if conf, ok := confidence.(float64); ok {
			if conf < 0.0 || conf > 1.0 {
				t.Errorf("confidence %f out of range [0.0, 1.0]", conf)
			}
		}

		// Contains heartbeat metrics.
		hbData, ok := payload["heartbeat"]
		if !ok {
			t.Error("event payload missing heartbeat metrics")
		}
		if hb, ok := hbData.(map[string]interface{}); ok {
			if _, has := hb["cpu_usage"]; !has {
				t.Error("heartbeat missing cpu_usage")
			}
			if _, has := hb["memory_usage"]; !has {
				t.Error("heartbeat missing memory_usage")
			}
		}

		// Contains action_type.
		actionType, ok := payload["action_type"]
		if !ok || actionType == "" {
			t.Error("event payload missing action_type")
		}

		// Verify event-level timestamp.
		if event.Timestamp.IsZero() {
			t.Error("event missing timestamp")
		}

		// Verify event-level node field.
		if event.Node == "" {
			t.Error("event missing Node field")
		}
	})
}

// **Validates: Requirements 10.5**
// Property 18: Earthworm model failure triggers rule-based fallback

// failingProvider always returns an AI error simulating model failure.
type failingProvider struct {
	category ai.ErrorCategory
}

func (f *failingProvider) Predict(_ context.Context, _ ai.PredictRequest) (ai.PredictResponse, error) {
	return ai.PredictResponse{}, &ai.AIError{
		Category: f.category,
		Module:   "earthworm",
		Message:  "simulated model failure",
		Path:     "/models/earthworm-anomaly.onnx",
	}
}

func (f *failingProvider) Train(_ context.Context, _ ai.TrainRequest) (ai.TrainResponse, error) {
	return ai.TrainResponse{}, nil
}

func (f *failingProvider) Explain(_ context.Context, _ ai.ExplainRequest) (ai.ExplainResponse, error) {
	return ai.ExplainResponse{}, nil
}

func TestProperty18_ModelFailure_RuleBasedFallback(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		// Simulate different failure types.
		failureType := rapid.SampledFrom([]ai.ErrorCategory{
			ai.ErrModelUnavailable,
			ai.ErrModelLoadFailed,
			ai.ErrInferenceTimeout,
		}).Draw(t, "failureType")

		// Generate heartbeat with at least one metric above rule thresholds
		// to ensure rule-based detection triggers.
		triggerMetric := rapid.SampledFrom([]string{"cpu", "memory", "latency"}).Draw(t, "triggerMetric")

		heartbeat := HeartbeatSignal{
			NodeID:      rapid.StringMatching(`node-[a-z0-9]{1,8}`).Draw(t, "nodeID"),
			CPUUsage:    rapid.Float64Range(0.0, 0.5).Draw(t, "cpuUsage"),
			MemoryUsage: rapid.Float64Range(0.0, 0.5).Draw(t, "memUsage"),
			DiskIO:      rapid.Float64Range(0.0, 0.5).Draw(t, "diskIO"),
			NetworkIO:   rapid.Float64Range(0.0, 0.5).Draw(t, "netIO"),
			Latency:     rapid.Float64Range(0.0, 200.0).Draw(t, "latency"),
			Timestamp:   time.Now().UTC(),
		}

		// Push one metric above threshold to trigger rule-based detection.
		switch triggerMetric {
		case "cpu":
			heartbeat.CPUUsage = rapid.Float64Range(0.91, 1.0).Draw(t, "highCPU")
		case "memory":
			heartbeat.MemoryUsage = rapid.Float64Range(0.86, 1.0).Draw(t, "highMem")
		case "latency":
			heartbeat.Latency = rapid.Float64Range(501.0, 999.0).Draw(t, "highLatency")
		}

		provider := &failingProvider{category: failureType}
		emitter := &mockPropEmitter{}
		k8sClient := &mockPropK8s{
			pods: []k8s.PodInfo{
				{Name: "pod-1", Namespace: "default", NodeName: heartbeat.NodeID, Status: "Running"},
			},
		}

		cfg := AgentConfig{
			Threshold:      0.75, // Rule-based detection produces scores ≥ 0.75.
			NodeID:         heartbeat.NodeID,
			RuleThresholds: DefaultRuleThresholds(),
		}
		agent, err := NewAgent(cfg, provider, emitter, k8sClient)
		if err != nil {
			t.Fatalf("NewAgent: %v", err)
		}

		// Capture log output to verify degraded-mode warning.
		var logBuf strings.Builder
		agent.logger.SetOutput(&logBuf)

		result, err := agent.ProcessHeartbeat(context.Background(), heartbeat)
		if err != nil {
			t.Fatalf("ProcessHeartbeat should not fail on model failure: %v", err)
		}

		// Verify fallback to rules occurred.
		if result == nil {
			t.Fatal("expected rule-based fallback to produce a result")
		}
		if !result.WasRuleBased {
			t.Error("expected WasRuleBased=true for model failure scenario")
		}

		// Verify degraded-mode warning was logged.
		logOutput := logBuf.String()
		if !strings.Contains(logOutput, "degraded-mode") && !strings.Contains(logOutput, "falling back") {
			t.Errorf("expected degraded-mode or fallback warning in logs, got: %s", logOutput)
		}
	})
}
