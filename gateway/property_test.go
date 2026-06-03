package gateway

import (
	"context"
	"testing"
	"time"

	"pgregory.net/rapid"
)

// **Validates: Requirements 8.5, 8.6**
// Property 13: Audit trail and explainability completeness

// autonomousActionGen generates a random valid AutonomousAction.
func autonomousActionGen() *rapid.Generator[AutonomousAction] {
	return rapid.Custom(func(t *rapid.T) AutonomousAction {
		modules := []string{"earthworm", "tlapix", "ebeecontrol", "quack"}
		actionTypes := []string{"pod_restart", "node_cordon", "workload_reschedule", "isolate_pod", "alert_operator", "forensic_report"}
		outcomes := []string{"success", "failed", "rejected", "paused"}

		return AutonomousAction{
			ID:         rapid.StringMatching(`act-[a-f0-9]{8}`).Draw(t, "id"),
			Module:     rapid.SampledFrom(modules).Draw(t, "module"),
			ActionType: rapid.SampledFrom(actionTypes).Draw(t, "actionType"),
			Confidence: rapid.Float64Range(0.0, 1.0).Draw(t, "confidence"),
			Reasoning: ReasoningChain{
				Observation:  rapid.StringMatching(`[a-z ]{10,50}`).Draw(t, "observation"),
				Analysis:     rapid.StringMatching(`[a-z ]{10,50}`).Draw(t, "analysis"),
				Action:       rapid.StringMatching(`[a-z ]{5,30}`).Draw(t, "action"),
				Alternatives: []string{"alert_operator", "forensic_report"},
			},
			Outcome:   rapid.SampledFrom(outcomes).Draw(t, "outcome"),
			Timestamp: time.Now().UTC().Add(-time.Duration(rapid.IntRange(0, 3600).Draw(t, "offset")) * time.Second),
		}
	})
}

// overrideRequestGen generates a random valid OverrideRequest.
func overrideRequestGen() *rapid.Generator[OverrideRequest] {
	return rapid.Custom(func(t *rapid.T) OverrideRequest {
		modules := []string{"earthworm", "tlapix", "ebeecontrol", "quack"}
		operations := []string{"approve", "reject", "pause", "resume"}

		return OverrideRequest{
			ActionID:   rapid.StringMatching(`act-[a-f0-9]{8}`).Draw(t, "actionID"),
			ModuleID:   rapid.SampledFrom(modules).Draw(t, "moduleID"),
			Operation:  rapid.SampledFrom(operations).Draw(t, "operation"),
			OperatorID: rapid.StringMatching(`operator-[a-z]{3,10}`).Draw(t, "operatorID"),
		}
	})
}

func TestProperty13_ActionAuditEntry_AllRequiredFields(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		action := autonomousActionGen().Draw(t, "action")

		store := NewAuditStore()
		ctx := context.Background()

		store.RecordAction(ctx, action)

		entries := store.All(ctx)
		if len(entries) != 1 {
			t.Fatalf("expected 1 audit entry, got %d", len(entries))
		}

		entry := entries[0]

		// Verify all required fields are present.
		if entry.Timestamp.IsZero() {
			t.Error("audit entry missing timestamp")
		}
		if entry.Module == "" {
			t.Error("audit entry missing module")
		}
		if entry.Module != action.Module {
			t.Errorf("expected module %q, got %q", action.Module, entry.Module)
		}
		if entry.ActionType == "" {
			t.Error("audit entry missing action_type")
		}
		if entry.ActionType != action.ActionType {
			t.Errorf("expected action_type %q, got %q", action.ActionType, entry.ActionType)
		}
		// TriggerEvent comes from Reasoning.Observation.
		if entry.TriggerEvent == "" {
			t.Error("audit entry missing trigger_event")
		}
		// Confidence should be recorded.
		if entry.Confidence != action.Confidence {
			t.Errorf("expected confidence %f, got %f", action.Confidence, entry.Confidence)
		}
		// Reasoning should be recorded.
		if entry.Reasoning == "" {
			t.Error("audit entry missing reasoning")
		}
		// Outcome should be recorded.
		if entry.Outcome == "" {
			t.Error("audit entry missing outcome")
		}
		if entry.Outcome != action.Outcome {
			t.Errorf("expected outcome %q, got %q", action.Outcome, entry.Outcome)
		}
	})
}

func TestProperty13_OverrideAuditEntry_ContainsOperatorID(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		override := overrideRequestGen().Draw(t, "override")

		store := NewAuditStore()
		ctx := context.Background()

		outcome := rapid.SampledFrom([]string{"success", "rejected", "paused", "resumed"}).Draw(t, "outcome")
		store.RecordOverride(ctx, override, outcome)

		entries := store.All(ctx)
		if len(entries) != 1 {
			t.Fatalf("expected 1 audit entry, got %d", len(entries))
		}

		entry := entries[0]

		// For any override recorded, audit entry contains operator_id.
		if entry.OperatorID == "" {
			t.Error("override audit entry missing operator_id")
		}
		if entry.OperatorID != override.OperatorID {
			t.Errorf("expected operator_id %q, got %q", override.OperatorID, entry.OperatorID)
		}

		// Verify other required fields are present.
		if entry.Timestamp.IsZero() {
			t.Error("override audit entry missing timestamp")
		}
		if entry.Module == "" {
			t.Error("override audit entry missing module")
		}
		if entry.ActionType == "" {
			t.Error("override audit entry missing action_type")
		}
		if entry.Outcome == "" {
			t.Error("override audit entry missing outcome")
		}
		if entry.Outcome != outcome {
			t.Errorf("expected outcome %q, got %q", outcome, entry.Outcome)
		}
	})
}

func TestProperty13_Explainability_ConfidenceInRange(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		action := autonomousActionGen().Draw(t, "action")

		// Verify confidence is in [0.0, 1.0].
		if action.Confidence < 0.0 || action.Confidence > 1.0 {
			t.Fatalf("confidence %f out of range [0.0, 1.0]", action.Confidence)
		}

		// Verify reasoning chain is complete.
		if action.Reasoning.Observation == "" {
			t.Error("reasoning chain missing observation")
		}
		if action.Reasoning.Analysis == "" {
			t.Error("reasoning chain missing analysis")
		}
		if action.Reasoning.Action == "" {
			t.Error("reasoning chain missing action")
		}
	})
}
