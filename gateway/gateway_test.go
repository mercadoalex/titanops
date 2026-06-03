package gateway

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func setupTestGateway(t *testing.T) (*Gateway, *http.ServeMux) {
	t.Helper()
	gw := NewGateway()
	mux := http.NewServeMux()
	gw.RegisterRoutes(mux)
	return gw, mux
}

func TestHealthEndpoint_ReturnsAllModules(t *testing.T) {
	_, mux := setupTestGateway(t)

	req := httptest.NewRequest(http.MethodGet, "/api/health", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rec.Code)
	}

	var modules []ModuleHealth
	if err := json.NewDecoder(rec.Body).Decode(&modules); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if len(modules) != 4 {
		t.Fatalf("expected 4 modules, got %d", len(modules))
	}

	expectedModules := map[string]bool{
		"earthworm":   false,
		"tlapix":      false,
		"ebeecontrol": false,
		"quack":       false,
	}

	for _, m := range modules {
		if _, ok := expectedModules[m.Module]; !ok {
			t.Errorf("unexpected module: %s", m.Module)
		}
		expectedModules[m.Module] = true
		if m.Status != "operational" {
			t.Errorf("expected status operational for %s, got %s", m.Module, m.Status)
		}
	}

	for name, found := range expectedModules {
		if !found {
			t.Errorf("module %s not found in response", name)
		}
	}
}

func TestActionsEndpoint_ReturnsRecentActions(t *testing.T) {
	gw, mux := setupTestGateway(t)

	ctx := context.Background()
	now := time.Now().UTC()

	// Add some test actions.
	gw.Actions().Add(ctx, AutonomousAction{
		ID:         "act-1",
		Module:     "earthworm",
		ActionType: "pod_restart",
		Confidence: 0.85,
		Reasoning: ReasoningChain{
			Observation: "node heartbeat anomaly",
			Analysis:    "confidence above threshold",
			Action:      "restart pod",
		},
		Outcome:   "success",
		Timestamp: now.Add(-10 * time.Minute),
	})
	gw.Actions().Add(ctx, AutonomousAction{
		ID:         "act-2",
		Module:     "tlapix",
		ActionType: "alert_operator",
		Confidence: 0.72,
		Reasoning: ReasoningChain{
			Observation: "cert expiry detected",
			Analysis:    "certificate expires in 24h",
			Action:      "alert operator",
		},
		Outcome:   "success",
		Timestamp: now.Add(-5 * time.Minute),
	})

	req := httptest.NewRequest(http.MethodGet, "/api/actions?limit=10", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rec.Code)
	}

	var actions []AutonomousAction
	if err := json.NewDecoder(rec.Body).Decode(&actions); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if len(actions) != 2 {
		t.Fatalf("expected 2 actions, got %d", len(actions))
	}

	if actions[0].ID != "act-1" {
		t.Errorf("expected first action ID act-1, got %s", actions[0].ID)
	}
	if actions[1].ID != "act-2" {
		t.Errorf("expected second action ID act-2, got %s", actions[1].ID)
	}
}

func TestActionsEndpoint_LimitParameter(t *testing.T) {
	gw, mux := setupTestGateway(t)
	ctx := context.Background()
	now := time.Now().UTC()

	for i := 0; i < 10; i++ {
		gw.Actions().Add(ctx, AutonomousAction{
			ID:        "act-" + time.Now().Format("150405.000000000"),
			Module:    "earthworm",
			Timestamp: now.Add(time.Duration(i) * time.Minute),
		})
	}

	req := httptest.NewRequest(http.MethodGet, "/api/actions?limit=3", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	var actions []AutonomousAction
	if err := json.NewDecoder(rec.Body).Decode(&actions); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if len(actions) != 3 {
		t.Fatalf("expected 3 actions, got %d", len(actions))
	}
}

func TestOverrideApprove(t *testing.T) {
	gw, mux := setupTestGateway(t)
	ctx := context.Background()

	// Add a pending action.
	gw.Actions().Add(ctx, AutonomousAction{
		ID:         "pending-1",
		Module:     "earthworm",
		ActionType: "pod_restart",
		Confidence: 0.9,
		Outcome:    "pending",
		Timestamp:  time.Now().UTC(),
	})

	body := OverrideRequest{
		ActionID:   "pending-1",
		Operation:  "approve",
		OperatorID: "operator-alice",
	}
	b, _ := json.Marshal(body)

	req := httptest.NewRequest(http.MethodPost, "/api/overrides", bytes.NewReader(b))
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", rec.Code, rec.Body.String())
	}

	// Verify action was updated.
	action, _ := gw.Actions().Get(ctx, "pending-1")
	if action.Outcome != "success" {
		t.Errorf("expected outcome 'success', got %q", action.Outcome)
	}
	if action.OverrideBy != "operator-alice" {
		t.Errorf("expected override_by 'operator-alice', got %q", action.OverrideBy)
	}

	// Verify audit trail.
	entries := gw.Audit().All(ctx)
	if len(entries) == 0 {
		t.Fatal("expected audit entry for approve override")
	}
	found := false
	for _, e := range entries {
		if e.ActionType == "approve" && e.OperatorID == "operator-alice" {
			found = true
		}
	}
	if !found {
		t.Error("audit trail does not contain approve entry")
	}
}

func TestOverrideReject(t *testing.T) {
	gw, mux := setupTestGateway(t)
	ctx := context.Background()

	gw.Actions().Add(ctx, AutonomousAction{
		ID:         "pending-2",
		Module:     "tlapix",
		ActionType: "isolate_pod",
		Confidence: 0.88,
		Outcome:    "pending",
		Timestamp:  time.Now().UTC(),
	})

	body := OverrideRequest{
		ActionID:   "pending-2",
		Operation:  "reject",
		OperatorID: "operator-bob",
	}
	b, _ := json.Marshal(body)

	req := httptest.NewRequest(http.MethodPost, "/api/overrides", bytes.NewReader(b))
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", rec.Code, rec.Body.String())
	}

	action, _ := gw.Actions().Get(ctx, "pending-2")
	if action.Outcome != "rejected" {
		t.Errorf("expected outcome 'rejected', got %q", action.Outcome)
	}
}

func TestOverridePauseAndResume(t *testing.T) {
	gw, mux := setupTestGateway(t)
	ctx := context.Background()

	// Pause earthworm.
	body := OverrideRequest{
		ModuleID:   "earthworm",
		Operation:  "pause",
		OperatorID: "operator-alice",
	}
	b, _ := json.Marshal(body)

	req := httptest.NewRequest(http.MethodPost, "/api/overrides", bytes.NewReader(b))
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200 for pause, got %d: %s", rec.Code, rec.Body.String())
	}

	if !gw.Overrides().IsModulePaused(ctx, "earthworm") {
		t.Error("expected earthworm to be paused")
	}

	// Resume earthworm.
	body = OverrideRequest{
		ModuleID:   "earthworm",
		Operation:  "resume",
		OperatorID: "operator-alice",
	}
	b, _ = json.Marshal(body)

	req = httptest.NewRequest(http.MethodPost, "/api/overrides", bytes.NewReader(b))
	rec = httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200 for resume, got %d: %s", rec.Code, rec.Body.String())
	}

	if gw.Overrides().IsModulePaused(ctx, "earthworm") {
		t.Error("expected earthworm to not be paused after resume")
	}
}

func TestAuditTrail_RecordsActionsAndOverrides(t *testing.T) {
	gw, mux := setupTestGateway(t)
	ctx := context.Background()

	// Record an autonomous action via audit store.
	gw.Audit().RecordAction(ctx, AutonomousAction{
		ID:         "act-audit",
		Module:     "quack",
		ActionType: "workload_reschedule",
		Confidence: 0.92,
		Reasoning: ReasoningChain{
			Observation: "scheduling imbalance",
			Analysis:    "latency increased",
			Action:      "reschedule workload",
		},
		Outcome:   "success",
		Timestamp: time.Now().UTC(),
	})

	// Add a pending action and reject it.
	gw.Actions().Add(ctx, AutonomousAction{
		ID:         "act-for-reject",
		Module:     "quack",
		ActionType: "pod_restart",
		Outcome:    "pending",
		Timestamp:  time.Now().UTC(),
	})

	body := OverrideRequest{
		ActionID:   "act-for-reject",
		Operation:  "reject",
		OperatorID: "operator-carol",
	}
	b, _ := json.Marshal(body)
	req := httptest.NewRequest(http.MethodPost, "/api/overrides", bytes.NewReader(b))
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("override failed: %d", rec.Code)
	}

	// Query audit trail.
	req = httptest.NewRequest(http.MethodGet, "/api/audit", nil)
	rec = httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rec.Code)
	}

	var entries []AuditEntry
	if err := json.NewDecoder(rec.Body).Decode(&entries); err != nil {
		t.Fatalf("failed to decode audit response: %v", err)
	}

	if len(entries) < 2 {
		t.Fatalf("expected at least 2 audit entries, got %d", len(entries))
	}

	// Verify the action record is present.
	foundAction := false
	foundOverride := false
	for _, e := range entries {
		if e.ActionType == "workload_reschedule" && e.Module == "quack" {
			foundAction = true
		}
		if e.ActionType == "reject" && e.OperatorID == "operator-carol" {
			foundOverride = true
		}
	}
	if !foundAction {
		t.Error("audit trail missing the autonomous action record")
	}
	if !foundOverride {
		t.Error("audit trail missing the override record")
	}
}

func TestModulePause_PreventsActions(t *testing.T) {
	gw, _ := setupTestGateway(t)
	ctx := context.Background()

	// Pause earthworm module.
	err := gw.Overrides().PauseModule(ctx, "earthworm", "operator-alice")
	if err != nil {
		t.Fatalf("pause failed: %v", err)
	}

	// Verify module is paused.
	if !gw.Overrides().IsModulePaused(ctx, "earthworm") {
		t.Error("expected earthworm to be paused")
	}

	// Other modules should not be paused.
	if gw.Overrides().IsModulePaused(ctx, "tlapix") {
		t.Error("expected tlapix to not be paused")
	}
	if gw.Overrides().IsModulePaused(ctx, "quack") {
		t.Error("expected quack to not be paused")
	}
}

func TestExplainEndpoint(t *testing.T) {
	gw, mux := setupTestGateway(t)
	ctx := context.Background()

	gw.Actions().Add(ctx, AutonomousAction{
		ID:         "explain-1",
		Module:     "ebeecontrol",
		ActionType: "isolate_pod",
		Confidence: 0.95,
		Reasoning: ReasoningChain{
			Observation:  "honeytoken access detected",
			Analysis:     "potential lateral movement",
			Action:       "isolate pod",
			Alternatives: []string{"alert_operator", "forensic_report"},
		},
		Outcome:   "success",
		Timestamp: time.Now().UTC(),
	})

	req := httptest.NewRequest(http.MethodGet, "/api/explain/explain-1", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var result map[string]interface{}
	if err := json.NewDecoder(rec.Body).Decode(&result); err != nil {
		t.Fatalf("failed to decode explain response: %v", err)
	}

	if result["action_id"] != "explain-1" {
		t.Errorf("expected action_id explain-1, got %v", result["action_id"])
	}
	if result["confidence"].(float64) != 0.95 {
		t.Errorf("expected confidence 0.95, got %v", result["confidence"])
	}

	reasoning, ok := result["reasoning"].(map[string]interface{})
	if !ok {
		t.Fatal("expected reasoning to be a map")
	}
	if reasoning["observation"] != "honeytoken access detected" {
		t.Errorf("expected observation 'honeytoken access detected', got %v", reasoning["observation"])
	}
	if reasoning["analysis"] != "potential lateral movement" {
		t.Errorf("expected analysis 'potential lateral movement', got %v", reasoning["analysis"])
	}
}

func TestExplainEndpoint_NotFound(t *testing.T) {
	_, mux := setupTestGateway(t)

	req := httptest.NewRequest(http.MethodGet, "/api/explain/nonexistent", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected status 404, got %d", rec.Code)
	}
}

func TestCorrelationsEndpoint(t *testing.T) {
	gw, mux := setupTestGateway(t)
	ctx := context.Background()

	gw.Correlations().Add(ctx, CorrelatedIncident{
		IncidentID:    "inc-1",
		Modules:       []string{"earthworm", "tlapix"},
		Confidence:    85,
		Narrative:     "Node anomaly correlated with cert issue",
		MatchedAttrs:  []string{"node"},
		DetectedAt:    time.Now().UTC(),
		ActionTaken:   true,
		ActionOutcome: "success",
	})

	req := httptest.NewRequest(http.MethodGet, "/api/correlations?window=120m", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rec.Code)
	}

	var incidents []CorrelatedIncident
	if err := json.NewDecoder(rec.Body).Decode(&incidents); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if len(incidents) != 1 {
		t.Fatalf("expected 1 incident, got %d", len(incidents))
	}
	if incidents[0].IncidentID != "inc-1" {
		t.Errorf("expected incident ID inc-1, got %s", incidents[0].IncidentID)
	}
}

func TestOverride_InvalidOperation(t *testing.T) {
	_, mux := setupTestGateway(t)

	body := OverrideRequest{
		ActionID:   "act-1",
		Operation:  "invalid",
		OperatorID: "operator-x",
	}
	b, _ := json.Marshal(body)

	req := httptest.NewRequest(http.MethodPost, "/api/overrides", bytes.NewReader(b))
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected status 400, got %d", rec.Code)
	}
}

func TestOverride_MissingOperatorID(t *testing.T) {
	_, mux := setupTestGateway(t)

	body := OverrideRequest{
		ActionID:  "act-1",
		Operation: "approve",
	}
	b, _ := json.Marshal(body)

	req := httptest.NewRequest(http.MethodPost, "/api/overrides", bytes.NewReader(b))
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected status 400, got %d", rec.Code)
	}
}

func TestAuditFilter_ByModule(t *testing.T) {
	gw, mux := setupTestGateway(t)
	ctx := context.Background()

	gw.Audit().RecordAction(ctx, AutonomousAction{
		ID:        "a1",
		Module:    "earthworm",
		Outcome:   "success",
		Timestamp: time.Now().UTC(),
	})
	gw.Audit().RecordAction(ctx, AutonomousAction{
		ID:        "a2",
		Module:    "tlapix",
		Outcome:   "success",
		Timestamp: time.Now().UTC(),
	})

	req := httptest.NewRequest(http.MethodGet, "/api/audit?module=earthworm", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	var entries []AuditEntry
	if err := json.NewDecoder(rec.Body).Decode(&entries); err != nil {
		t.Fatalf("failed to decode: %v", err)
	}

	if len(entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(entries))
	}
	if entries[0].Module != "earthworm" {
		t.Errorf("expected module earthworm, got %s", entries[0].Module)
	}
}
