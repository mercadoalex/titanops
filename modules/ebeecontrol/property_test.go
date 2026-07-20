package ebeecontrol

import (
	"testing"
	"time"

	"pgregory.net/rapid"
)

// --- Property 1: Threat Classifier Always Returns Valid Classification ---

func TestProperty_ClassifierAlwaysReturnsValid(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		ctx := PodContext{
			Namespace:               "test-ns",
			NamespaceClassification: rapid.SampledFrom([]NamespaceClassification{NamespaceProduction, NamespaceNonProduction}).Draw(t, "nsClass"),
			ServiceCriticality:      rapid.IntRange(1, 5).Draw(t, "criticality"),
			DavisAnomalyScore:       rapid.Float64Range(0.0, 1.0).Draw(t, "anomaly"),
			AnomalyWindowMinutes:    10,
		}

		result := ClassifyThreat(ctx)

		valid := result == ThreatLow || result == ThreatMedium || result == ThreatHigh || result == ThreatCritical
		if !valid {
			t.Fatalf("classifier returned invalid classification: %q", result)
		}
	})
}

// --- Property 2: Production + High Anomaly Always Critical ---

func TestProperty_ProductionHighAnomalyIsCritical(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		anomaly := rapid.Float64Range(0.81, 1.0).Draw(t, "anomaly")
		criticality := rapid.IntRange(1, 5).Draw(t, "criticality")

		ctx := PodContext{
			NamespaceClassification: NamespaceProduction,
			ServiceCriticality:      criticality,
			DavisAnomalyScore:       anomaly,
		}

		result := ClassifyThreat(ctx)
		if result != ThreatCritical {
			t.Fatalf("production + anomaly %.2f should be critical, got %q", anomaly, result)
		}
	})
}

// --- Property 3: Production + Criticality 5 Always Critical ---

func TestProperty_ProductionCriticality5IsCritical(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		anomaly := rapid.Float64Range(0.0, 1.0).Draw(t, "anomaly")

		ctx := PodContext{
			NamespaceClassification: NamespaceProduction,
			ServiceCriticality:      5,
			DavisAnomalyScore:       anomaly,
		}

		result := ClassifyThreat(ctx)
		if result != ThreatCritical {
			t.Fatalf("production + criticality 5 should be critical, got %q (anomaly=%.2f)", result, anomaly)
		}
	})
}

// --- Property 4: Non-Production + Low Anomaly + Low Criticality Is Low ---

func TestProperty_NonProdLowAnomalyIsLow(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		anomaly := rapid.Float64Range(0.0, 0.29).Draw(t, "anomaly")
		criticality := rapid.IntRange(1, 2).Draw(t, "criticality")

		ctx := PodContext{
			NamespaceClassification: NamespaceNonProduction,
			ServiceCriticality:      criticality,
			DavisAnomalyScore:       anomaly,
		}

		result := ClassifyThreat(ctx)
		if result != ThreatLow {
			t.Fatalf("non-prod + anomaly %.2f + criticality %d should be low, got %q", anomaly, criticality, result)
		}
	})
}

// --- Property 5: Defaults Substitute Highest Risk ---

func TestProperty_DefaultsAreHighestRisk(t *testing.T) {
	// With all zero values, ClassifyThreatWithDefaults should apply:
	// namespace=production, criticality=5, anomaly=1.0 → critical
	result := ClassifyThreatWithDefaults("", 0, 0)
	if result != ThreatCritical {
		t.Fatalf("defaults should yield critical, got %q", result)
	}
}

// --- Property 6: Response Planner - Low Threat Produces No Actions ---

func TestProperty_LowThreatNoActions(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		assessment := ThreatAssessment{
			AssessmentID:   "test",
			Classification: ThreatLow,
		}
		ns := rapid.StringMatching(`[a-z]{3,10}`).Draw(t, "namespace")
		pod := rapid.StringMatching(`[a-z]+-[a-z0-9]{4}`).Draw(t, "pod")

		plan := GenerateResponsePlan(assessment, ns, pod)

		if len(plan.Actions) != 0 {
			t.Fatalf("low threat should have 0 actions, got %d", len(plan.Actions))
		}
	})
}

// --- Property 7: High/Critical Threat Always Includes Isolation + Block ---

func TestProperty_HighCriticalIncludesIsolationAndBlock(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		classification := rapid.SampledFrom([]ThreatClassification{ThreatHigh, ThreatCritical}).Draw(t, "class")

		assessment := ThreatAssessment{
			AssessmentID:   "test",
			Classification: classification,
		}

		plan := GenerateResponsePlan(assessment, "ns", "pod-1")

		hasIsolation := false
		hasBlock := false
		hasHoneytokens := false
		for _, a := range plan.Actions {
			switch a.ActionType {
			case ActionPodIsolation:
				hasIsolation = true
			case ActionIPBlock:
				hasBlock = true
			case ActionAdditionalHoneytokens:
				hasHoneytokens = true
			}
		}

		if !hasIsolation {
			t.Fatalf("%s threat should include pod_isolation", classification)
		}
		if !hasBlock {
			t.Fatalf("%s threat should include ip_block", classification)
		}
		if !hasHoneytokens {
			t.Fatalf("%s threat should include additional_honeytokens", classification)
		}
	})
}

// --- Property 8: Medium Threat Has Honeytokens But No Isolation ---

func TestProperty_MediumThreatHoneytokensOnly(t *testing.T) {
	assessment := ThreatAssessment{
		AssessmentID:   "test",
		Classification: ThreatMedium,
	}

	plan := GenerateResponsePlan(assessment, "ns", "pod-1")

	for _, a := range plan.Actions {
		if a.ActionType == ActionPodIsolation || a.ActionType == ActionIPBlock {
			t.Fatalf("medium threat should not include %s", a.ActionType)
		}
	}

	if len(plan.Actions) == 0 {
		t.Fatal("medium threat should have at least one action (honeytokens)")
	}
}

// --- Property 9: Model Publish Guard - Never Publishes Worse Model ---

func TestProperty_PublishGuardNeverPublishesWorse(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		initialAccuracy := rapid.Float64Range(70, 99).Draw(t, "accuracy")

		trainer, err := NewTrainer(TrainerConfig{
			RetrainingInterval:    24 * time.Hour,
			MinimumOutcomeRecords: 1,
		}, &ModelVersion{
			VersionID:          "v1.0.0",
			ValidationAccuracy: initialAccuracy,
			PublishedTimestamp:  time.Now().UTC(),
		})
		if err != nil {
			t.Fatalf("failed to create trainer: %v", err)
		}

		// Ingest enough data to allow retraining.
		for i := 0; i < 5; i++ {
			trainer.IngestOutcomeData(validOutcomeData(i))
		}

		// Force time to pass (hack the last retraining timestamp).
		trainer.mu.Lock()
		trainer.lastRetrainingTimestamp = time.Now().Add(-25 * time.Hour)
		trainer.mu.Unlock()

		// Trigger retraining (simulated accuracy is random 70-99).
		result := trainer.TriggerRetraining()

		currentModel := trainer.GetCurrentModel()

		if result.Success {
			// If published, new accuracy must be >= initial.
			if currentModel.ValidationAccuracy < initialAccuracy {
				t.Fatalf("publish guard violated: published %.0f%% which is less than initial %.0f%%",
					currentModel.ValidationAccuracy, initialAccuracy)
			}
		} else {
			// If not published, model should remain unchanged.
			if currentModel.ValidationAccuracy != initialAccuracy {
				t.Fatalf("model changed despite failed retraining: was %.0f%%, now %.0f%%",
					initialAccuracy, currentModel.ValidationAccuracy)
			}
		}
	})
}

// --- Property 10: Event Buffer Respects Capacity ---

func TestProperty_EventBufferCapacity(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		capacity := rapid.IntRange(1, 100).Draw(t, "capacity")
		eventCount := rapid.IntRange(0, 500).Draw(t, "events")

		buf := NewEventBuffer(capacity)

		for i := 0; i < eventCount; i++ {
			buf.Push(AccessEvent{
				EventID:   rapid.StringMatching(`evt-[0-9]{4}`).Draw(t, "id"),
				Timestamp: time.Now(),
			})
		}

		status := buf.Status()

		// Size never exceeds capacity.
		if status.Size > capacity {
			t.Fatalf("buffer size %d exceeds capacity %d", status.Size, capacity)
		}

		// Size is min(events pushed, capacity).
		expectedSize := eventCount
		if expectedSize > capacity {
			expectedSize = capacity
		}
		if status.Size != expectedSize {
			t.Fatalf("expected size %d, got %d (capacity=%d, pushed=%d)",
				expectedSize, status.Size, capacity, eventCount)
		}

		// Overflow count is correct.
		expectedOverflow := 0
		if eventCount > capacity {
			expectedOverflow = eventCount - capacity
		}
		if status.Overflow != expectedOverflow {
			t.Fatalf("expected overflow %d, got %d", expectedOverflow, status.Overflow)
		}
	})
}

// --- Property 11: Event Buffer Recent Returns Newest First ---

func TestProperty_EventBufferRecentOrder(t *testing.T) {
	buf := NewEventBuffer(10)

	for i := 0; i < 10; i++ {
		buf.Push(AccessEvent{
			EventID:   string(rune('a' + i)),
			Timestamp: time.Now().Add(time.Duration(i) * time.Second),
		})
	}

	recent := buf.Recent(5)
	if len(recent) != 5 {
		t.Fatalf("expected 5 recent events, got %d", len(recent))
	}

	// Newest should be first.
	for i := 1; i < len(recent); i++ {
		if recent[i].Timestamp.After(recent[i-1].Timestamp) {
			t.Fatalf("recent events not in newest-first order at index %d", i)
		}
	}
}

// --- Property 12: Deployment Validation Rejects Invalid Requests ---

func TestProperty_DeployerRejectsInvalid(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		// Empty podId should always fail.
		result := validateDeploymentRequest(DeploymentRequest{
			PodID:     "",
			Namespace: "test",
			Honeytokens: []HoneytokenSpec{
				{Type: HoneytokenDecoyFile, Name: "f", Placement: "/tmp/f"},
			},
		})
		if result == "" {
			t.Fatal("empty podId should be rejected")
		}

		// Empty namespace should always fail.
		result = validateDeploymentRequest(DeploymentRequest{
			PodID:     "pod-1",
			Namespace: "",
			Honeytokens: []HoneytokenSpec{
				{Type: HoneytokenDecoyFile, Name: "f", Placement: "/tmp/f"},
			},
		})
		if result == "" {
			t.Fatal("empty namespace should be rejected")
		}

		// >5 honeytokens should always fail.
		specs := make([]HoneytokenSpec, 6)
		for i := range specs {
			specs[i] = HoneytokenSpec{Type: HoneytokenDecoyFile, Name: "f", Placement: "/tmp/f"}
		}
		result = validateDeploymentRequest(DeploymentRequest{
			PodID:       "pod-1",
			Namespace:   "test",
			Honeytokens: specs,
		})
		if result == "" {
			t.Fatal(">5 honeytokens should be rejected")
		}
	})
}

// --- Helper ---

func validOutcomeData(i int) OutcomeData {
	return OutcomeData{
		IncidentID: "inc-" + string(rune('a'+i)),
		AccessEvent: AccessEvent{
			EventID:           "evt-" + string(rune('a'+i)),
			ProcessID:         1000 + i,
			ProcessBinaryPath: "/usr/bin/test",
			UserID:            1000,
			PodID:             "pod-1",
			Namespace:         "default",
			HoneytokenPath:    "/tmp/token",
			AccessType:        AccessRead,
			Timestamp:         time.Now(),
		},
		HoneytokenType:    HoneytokenDecoyFile,
		PlacementLocation: "/tmp/token",
		ActionsTaken: []ResponseAction{
			{ActionID: "a1", ActionType: ActionAlert, Result: ActionSuccess, Timestamp: time.Now()},
		},
		Effectiveness: Effectiveness{
			DetectionToResponseLatency: 100 * time.Millisecond,
			ThreatContained:            true,
			FalsePositive:              false,
		},
		Timestamp: time.Now(),
	}
}
