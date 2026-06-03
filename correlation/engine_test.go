package correlation

import (
	"context"
	"errors"
	"testing"
	"time"

	export "github.com/mercadoalex/titanops/shared/titanops-export"
)

// helper to create a test event with given module, node, pod, namespace, and timestamp offset.
func makeEvent(module, node, pod, namespace, eventType string, offset time.Duration) export.Event {
	return export.Event{
		Module:    module,
		Node:      node,
		Pod:       pod,
		Namespace: namespace,
		EventType: eventType,
		Timestamp: time.Now().UTC().Add(offset),
		Severity:  "high",
		Payload:   []byte("test payload"),
		EventID:   "test-event-id",
	}
}

func TestCorrelation_TwoModulesMatchingNode(t *testing.T) {
	cfg := DefaultConfig()
	cfg.ConfidenceThreshold = 100 // disable auto-action for this test
	recorder := &RecordingExecutor{}
	engine, err := NewEngine(cfg, nil, recorder)
	if err != nil {
		t.Fatalf("NewEngine: %v", err)
	}

	ctx := context.Background()

	// Ingest events from 2 different modules, same node.
	ev1 := makeEvent("earthworm", "node-1", "", "default", "anomaly_detected", 0)
	ev2 := makeEvent("tlapix", "node-1", "", "default", "cert_expiring", -5*time.Second)

	if err := engine.Ingest(ctx, ev1); err != nil {
		t.Fatalf("Ingest ev1: %v", err)
	}
	if err := engine.Ingest(ctx, ev2); err != nil {
		t.Fatalf("Ingest ev2: %v", err)
	}

	incidents, err := engine.Correlate(ctx)
	if err != nil {
		t.Fatalf("Correlate: %v", err)
	}

	if len(incidents) == 0 {
		t.Fatal("expected at least one correlated incident, got 0")
	}

	inc := incidents[0]

	// Verify contributing events.
	if len(inc.ContributingEvents) < 2 {
		t.Errorf("expected at least 2 contributing events, got %d", len(inc.ContributingEvents))
	}

	// Verify confidence score is in valid range.
	if inc.ConfidenceScore < 0 || inc.ConfidenceScore > 100 {
		t.Errorf("confidence score %d out of range [0,100]", inc.ConfidenceScore)
	}

	// Verify matched attributes include "node".
	found := false
	for _, attr := range inc.MatchedAttributes {
		if attr == "node" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected 'node' in matched attributes, got %v", inc.MatchedAttributes)
	}

	// Verify narrative is non-empty.
	if inc.Narrative == "" {
		t.Error("expected non-empty narrative")
	}
}

func TestCorrelation_SameModuleNoCorrelation(t *testing.T) {
	cfg := DefaultConfig()
	engine, err := NewEngine(cfg, nil, nil)
	if err != nil {
		t.Fatalf("NewEngine: %v", err)
	}

	ctx := context.Background()

	// Ingest events from the same module.
	ev1 := makeEvent("earthworm", "node-1", "", "default", "anomaly_1", 0)
	ev2 := makeEvent("earthworm", "node-1", "", "default", "anomaly_2", -2*time.Second)

	if err := engine.Ingest(ctx, ev1); err != nil {
		t.Fatalf("Ingest ev1: %v", err)
	}
	if err := engine.Ingest(ctx, ev2); err != nil {
		t.Fatalf("Ingest ev2: %v", err)
	}

	incidents, err := engine.Correlate(ctx)
	if err != nil {
		t.Fatalf("Correlate: %v", err)
	}

	if len(incidents) != 0 {
		t.Errorf("expected no correlated incidents from same module, got %d", len(incidents))
	}
}

func TestCorrelation_EventsOutsideTimeWindow(t *testing.T) {
	cfg := DefaultConfig()
	cfg.TimeWindow = 10 * time.Second // Very short window.
	engine, err := NewEngine(cfg, nil, nil)
	if err != nil {
		t.Fatalf("NewEngine: %v", err)
	}

	ctx := context.Background()

	// Ingest first event.
	ev1 := makeEvent("earthworm", "node-1", "", "default", "anomaly", 0)
	if err := engine.Ingest(ctx, ev1); err != nil {
		t.Fatalf("Ingest ev1: %v", err)
	}

	// Simulate time passing beyond the window by directly manipulating the event timestamps.
	engine.mu.Lock()
	engine.events[0].ReceivedAt = time.Now().Add(-20 * time.Second) // Way outside window.
	engine.mu.Unlock()

	// Ingest second event from different module.
	ev2 := makeEvent("tlapix", "node-1", "", "default", "cert_expiring", 0)
	if err := engine.Ingest(ctx, ev2); err != nil {
		t.Fatalf("Ingest ev2: %v", err)
	}

	incidents, err := engine.Correlate(ctx)
	if err != nil {
		t.Fatalf("Correlate: %v", err)
	}

	// The first event should have been pruned, so no correlation.
	if len(incidents) != 0 {
		t.Errorf("expected no correlated incidents for events outside time window, got %d", len(incidents))
	}
}

func TestConfidenceScoring_TwoModules(t *testing.T) {
	now := time.Now().UTC()
	events := []export.Event{
		{Module: "earthworm", Node: "node-1", Namespace: "default", Timestamp: now},
		{Module: "tlapix", Node: "node-1", Namespace: "default", Timestamp: now.Add(5 * time.Second)},
	}
	matchedAttrs := []string{"node", "namespace"}

	score := CalculateConfidence(events, matchedAttrs)

	// 2 modules × 20 = 40
	// 2 attributes × 10 = 20
	// Within 10s → +10
	// Total: 70
	expected := 70
	if score != expected {
		t.Errorf("expected confidence score %d, got %d", expected, score)
	}
}

func TestConfidenceScoring_ThreeModules(t *testing.T) {
	now := time.Now().UTC()
	events := []export.Event{
		{Module: "earthworm", Node: "node-1", Namespace: "default", Timestamp: now},
		{Module: "tlapix", Node: "node-1", Namespace: "default", Timestamp: now.Add(5 * time.Second)},
		{Module: "ebeecontrol", Node: "node-1", Namespace: "default", Timestamp: now.Add(8 * time.Second)},
	}
	matchedAttrs := []string{"node", "namespace"}

	score := CalculateConfidence(events, matchedAttrs)

	// 3 modules × 20 = 60
	// 2 attributes × 10 = 20
	// Within 10s → +10
	// Total: 90
	expected := 90
	if score != expected {
		t.Errorf("expected confidence score %d, got %d", expected, score)
	}
}

func TestConfidenceScoring_FourModulesCapAt100(t *testing.T) {
	now := time.Now().UTC()
	events := []export.Event{
		{Module: "earthworm", Node: "node-1", Pod: "pod-a", Namespace: "default", Timestamp: now},
		{Module: "tlapix", Node: "node-1", Pod: "pod-a", Namespace: "default", Timestamp: now.Add(2 * time.Second)},
		{Module: "ebeecontrol", Node: "node-1", Pod: "pod-a", Namespace: "default", Timestamp: now.Add(4 * time.Second)},
		{Module: "quack", Node: "node-1", Pod: "pod-a", Namespace: "default", Timestamp: now.Add(6 * time.Second)},
	}
	matchedAttrs := []string{"node", "pod", "namespace"}

	score := CalculateConfidence(events, matchedAttrs)

	// 4 modules × 20 = 80
	// 3 attributes × 10 = 30
	// Within 10s → +10
	// Total: 120 → capped at 100
	if score != 100 {
		t.Errorf("expected confidence score 100 (capped), got %d", score)
	}
}

func TestConfidenceScoring_ProximityBonus30s(t *testing.T) {
	now := time.Now().UTC()
	events := []export.Event{
		{Module: "earthworm", Node: "node-1", Namespace: "default", Timestamp: now},
		{Module: "tlapix", Node: "node-1", Namespace: "default", Timestamp: now.Add(20 * time.Second)},
	}
	matchedAttrs := []string{"node"}

	score := CalculateConfidence(events, matchedAttrs)

	// 2 modules × 20 = 40
	// 1 attribute × 10 = 10
	// 20s span → within 30s → +5
	// Total: 55
	expected := 55
	if score != expected {
		t.Errorf("expected confidence score %d, got %d", expected, score)
	}
}

func TestConfidenceScoring_NoProximityBonusBeyond30s(t *testing.T) {
	now := time.Now().UTC()
	events := []export.Event{
		{Module: "earthworm", Node: "node-1", Namespace: "default", Timestamp: now},
		{Module: "tlapix", Node: "node-1", Namespace: "default", Timestamp: now.Add(60 * time.Second)},
	}
	matchedAttrs := []string{"node"}

	score := CalculateConfidence(events, matchedAttrs)

	// 2 modules × 20 = 40
	// 1 attribute × 10 = 10
	// 60s span → no proximity bonus
	// Total: 50
	expected := 50
	if score != expected {
		t.Errorf("expected confidence score %d, got %d", expected, score)
	}
}

func TestAutoAction_ExecutesAboveThreshold(t *testing.T) {
	cfg := DefaultConfig()
	cfg.ConfidenceThreshold = 50 // Low threshold to trigger action.
	recorder := &RecordingExecutor{}
	engine, err := NewEngine(cfg, nil, recorder)
	if err != nil {
		t.Fatalf("NewEngine: %v", err)
	}

	ctx := context.Background()

	// Ingest events from 3 modules to get high confidence.
	now := time.Now().UTC()
	ev1 := export.Event{Module: "earthworm", Node: "node-1", Pod: "pod-a", Namespace: "default", EventType: "anomaly", Timestamp: now, Severity: "high", Payload: []byte("p")}
	ev2 := export.Event{Module: "tlapix", Node: "node-1", Pod: "pod-a", Namespace: "default", EventType: "cert_issue", Timestamp: now.Add(2 * time.Second), Severity: "high", Payload: []byte("p")}
	ev3 := export.Event{Module: "ebeecontrol", Node: "node-1", Pod: "pod-a", Namespace: "default", EventType: "honeytoken_access", Timestamp: now.Add(4 * time.Second), Severity: "critical", Payload: []byte("p")}

	for _, ev := range []export.Event{ev1, ev2, ev3} {
		if err := engine.Ingest(ctx, ev); err != nil {
			t.Fatalf("Ingest: %v", err)
		}
	}

	incidents, err := engine.Correlate(ctx)
	if err != nil {
		t.Fatalf("Correlate: %v", err)
	}

	if len(incidents) == 0 {
		t.Fatal("expected at least one incident")
	}

	// Verify action was executed.
	if len(recorder.Executed) == 0 {
		t.Fatal("expected auto-action to be executed")
	}
	if !incidents[0].ActionExecuted {
		t.Error("expected ActionExecuted to be true")
	}
}

func TestAutoAction_DoesNotExecuteBelowThreshold(t *testing.T) {
	cfg := DefaultConfig()
	cfg.ConfidenceThreshold = 99 // Very high threshold.
	recorder := &RecordingExecutor{}
	engine, err := NewEngine(cfg, nil, recorder)
	if err != nil {
		t.Fatalf("NewEngine: %v", err)
	}

	ctx := context.Background()

	// Ingest events from 2 modules (confidence will be less than 99).
	now := time.Now().UTC()
	ev1 := export.Event{Module: "earthworm", Node: "node-1", Namespace: "default", EventType: "anomaly", Timestamp: now, Severity: "high", Payload: []byte("p")}
	ev2 := export.Event{Module: "tlapix", Node: "node-1", Namespace: "default", EventType: "cert_issue", Timestamp: now.Add(60 * time.Second), Severity: "high", Payload: []byte("p")}

	for _, ev := range []export.Event{ev1, ev2} {
		if err := engine.Ingest(ctx, ev); err != nil {
			t.Fatalf("Ingest: %v", err)
		}
	}

	incidents, err := engine.Correlate(ctx)
	if err != nil {
		t.Fatalf("Correlate: %v", err)
	}

	if len(incidents) == 0 {
		t.Fatal("expected at least one incident")
	}

	// Verify action was NOT executed.
	if len(recorder.Executed) != 0 {
		t.Errorf("expected no auto-action execution, got %d", len(recorder.Executed))
	}
	if incidents[0].ActionExecuted {
		t.Error("expected ActionExecuted to be false")
	}
}

func TestAutoAction_FailureRecorded(t *testing.T) {
	cfg := DefaultConfig()
	cfg.ConfidenceThreshold = 50
	failErr := errors.New("pod isolation failed: network timeout")
	executor := &FailingExecutor{Err: failErr}
	engine, err := NewEngine(cfg, nil, executor)
	if err != nil {
		t.Fatalf("NewEngine: %v", err)
	}

	ctx := context.Background()

	now := time.Now().UTC()
	ev1 := export.Event{Module: "earthworm", Node: "node-1", Pod: "pod-a", Namespace: "default", EventType: "anomaly", Timestamp: now, Severity: "critical", Payload: []byte("p")}
	ev2 := export.Event{Module: "tlapix", Node: "node-1", Pod: "pod-a", Namespace: "default", EventType: "cert_issue", Timestamp: now.Add(3 * time.Second), Severity: "critical", Payload: []byte("p")}
	ev3 := export.Event{Module: "ebeecontrol", Node: "node-1", Pod: "pod-a", Namespace: "default", EventType: "honeytoken", Timestamp: now.Add(5 * time.Second), Severity: "critical", Payload: []byte("p")}

	for _, ev := range []export.Event{ev1, ev2, ev3} {
		if err := engine.Ingest(ctx, ev); err != nil {
			t.Fatalf("Ingest: %v", err)
		}
	}

	incidents, err := engine.Correlate(ctx)
	if err != nil {
		t.Fatalf("Correlate: %v", err)
	}

	if len(incidents) == 0 {
		t.Fatal("expected at least one incident")
	}

	inc := incidents[0]
	if inc.ActionExecuted {
		t.Error("expected ActionExecuted to be false when action fails")
	}
	if inc.ActionError == "" {
		t.Error("expected ActionError to be set when action fails")
	}
	if inc.ActionError != failErr.Error() {
		t.Errorf("expected ActionError %q, got %q", failErr.Error(), inc.ActionError)
	}
}

func TestValidateConfig_ValidDefault(t *testing.T) {
	cfg := DefaultConfig()
	if err := ValidateConfig(cfg); err != nil {
		t.Errorf("expected default config to be valid, got error: %v", err)
	}
}

func TestValidateConfig_InvalidTimeWindow(t *testing.T) {
	cfg := DefaultConfig()
	cfg.TimeWindow = 5 * time.Second // Below minimum.
	err := ValidateConfig(cfg)
	if err == nil {
		t.Fatal("expected error for time window below 10s")
	}

	cfg.TimeWindow = 700 * time.Second // Above maximum.
	err = ValidateConfig(cfg)
	if err == nil {
		t.Fatal("expected error for time window above 600s")
	}
}

func TestValidateConfig_InvalidThreshold(t *testing.T) {
	cfg := DefaultConfig()
	cfg.ConfidenceThreshold = 0
	err := ValidateConfig(cfg)
	if err == nil {
		t.Fatal("expected error for threshold 0")
	}

	cfg.ConfidenceThreshold = 101
	err = ValidateConfig(cfg)
	if err == nil {
		t.Fatal("expected error for threshold 101")
	}
}

func TestNarrative_ContainsModulesAndAttributes(t *testing.T) {
	now := time.Now().UTC()
	events := []export.Event{
		{Module: "earthworm", EventType: "anomaly_detected", Timestamp: now, Node: "node-1", Namespace: "prod"},
		{Module: "tlapix", EventType: "cert_expiring", Timestamp: now.Add(5 * time.Second), Node: "node-1", Namespace: "prod"},
	}
	matchedAttrs := []string{"node", "namespace"}

	narrative := GenerateNarrative(events, matchedAttrs, 70)

	// Verify narrative contains module names.
	if !containsStr(narrative, "earthworm") {
		t.Errorf("narrative missing module 'earthworm': %s", narrative)
	}
	if !containsStr(narrative, "tlapix") {
		t.Errorf("narrative missing module 'tlapix': %s", narrative)
	}

	// Verify narrative contains matched attributes.
	if !containsStr(narrative, "node") {
		t.Errorf("narrative missing attribute 'node': %s", narrative)
	}
	if !containsStr(narrative, "namespace") {
		t.Errorf("narrative missing attribute 'namespace': %s", narrative)
	}

	// Verify narrative contains confidence.
	if !containsStr(narrative, "70%") {
		t.Errorf("narrative missing confidence '70%%': %s", narrative)
	}
}

func TestGetIncidents_FilterByMinConfidence(t *testing.T) {
	cfg := DefaultConfig()
	cfg.ConfidenceThreshold = 100 // Don't trigger actions.
	engine, err := NewEngine(cfg, nil, nil)
	if err != nil {
		t.Fatalf("NewEngine: %v", err)
	}

	ctx := context.Background()

	// Create two sets of events with different confidence levels.
	now := time.Now().UTC()

	// High confidence: 3 modules with 3 matching attributes + proximity.
	ev1 := export.Event{Module: "earthworm", Node: "node-1", Pod: "pod-a", Namespace: "default", EventType: "a", Timestamp: now, Severity: "high", Payload: []byte("p")}
	ev2 := export.Event{Module: "tlapix", Node: "node-1", Pod: "pod-a", Namespace: "default", EventType: "b", Timestamp: now.Add(2 * time.Second), Severity: "high", Payload: []byte("p")}
	ev3 := export.Event{Module: "ebeecontrol", Node: "node-1", Pod: "pod-a", Namespace: "default", EventType: "c", Timestamp: now.Add(4 * time.Second), Severity: "high", Payload: []byte("p")}

	for _, ev := range []export.Event{ev1, ev2, ev3} {
		if err := engine.Ingest(ctx, ev); err != nil {
			t.Fatalf("Ingest: %v", err)
		}
	}

	_, err = engine.Correlate(ctx)
	if err != nil {
		t.Fatalf("Correlate: %v", err)
	}

	// Get incidents with high min confidence.
	incidents, err := engine.GetIncidents(ctx, IncidentFilter{MinConfidence: 90})
	if err != nil {
		t.Fatalf("GetIncidents: %v", err)
	}

	for _, inc := range incidents {
		if inc.ConfidenceScore < 90 {
			t.Errorf("got incident with score %d, expected >= 90", inc.ConfidenceScore)
		}
	}
}

func TestEngine_ContextCancellation(t *testing.T) {
	cfg := DefaultConfig()
	engine, err := NewEngine(cfg, nil, nil)
	if err != nil {
		t.Fatalf("NewEngine: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	// All operations should respect context cancellation.
	err = engine.Ingest(ctx, export.Event{})
	if err == nil {
		t.Error("expected error from Ingest with cancelled context")
	}

	_, err = engine.Correlate(ctx)
	if err == nil {
		t.Error("expected error from Correlate with cancelled context")
	}

	_, err = engine.GetIncidents(ctx, IncidentFilter{})
	if err == nil {
		t.Error("expected error from GetIncidents with cancelled context")
	}
}

// containsStr checks if substr is present in s.
func containsStr(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && contains(s, substr))
}

func contains(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
