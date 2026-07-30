package ollinai

import (
	"context"
	"testing"
	"time"

	"pgregory.net/rapid"

	export "github.com/mercadoalex/titanops/shared/titanops-export"
)

// --- Property: Change Classification Always Has Valid Severity ---

func TestProperty_ChangeClassificationValidSeverity(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		changeType := rapid.SampledFrom([]ChangeType{
			ChangeImage, ChangeEnvVar, ChangeResource,
			ChangeReplica, ChangeProbe, ChangeRolloutStrategy,
		}).Draw(t, "type")

		var change DetectedChange
		switch changeType {
		case ChangeImage:
			change = ClassifyImageChange("containers[0].image", "old:v1", "new:v2")
		case ChangeEnvVar:
			change = ClassifyEnvChange("containers[0].env[FOO]", "old", "new")
		case ChangeResource:
			change = ClassifyResourceChange("containers[0].resources.limits[cpu]", "500m", "1000m")
		case ChangeReplica:
			change = ClassifyReplicaChange(2, 4)
		case ChangeProbe:
			change = ClassifyProbeChange("containers[0].livenessProbe", "changed")
		case ChangeRolloutStrategy:
			change = ClassifyRolloutStrategyChange("RollingUpdate", "Recreate")
		}

		valid := change.Severity == ChangeSeverityHigh ||
			change.Severity == ChangeSeverityMedium ||
			change.Severity == ChangeSeverityLow
		if !valid {
			t.Fatalf("invalid severity %q for change type %q", change.Severity, changeType)
		}
	})
}

// --- Property: Image Changes Are Always High Severity ---

func TestProperty_ImageChangesAreHighSeverity(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		old := rapid.StringMatching(`[a-z]+:[a-z0-9]+`).Draw(t, "old")
		new := rapid.StringMatching(`[a-z]+:[a-z0-9]+`).Draw(t, "new")

		change := ClassifyImageChange("containers[0].image", old, new)
		if change.Severity != ChangeSeverityHigh {
			t.Fatalf("image change should be high severity, got %q", change.Severity)
		}
	})
}

// --- Property: Replica Changes Are Always Low Severity ---

func TestProperty_ReplicaChangesAreLowSeverity(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		old := rapid.IntRange(1, 100).Draw(t, "old")
		new := rapid.IntRange(1, 100).Draw(t, "new")

		change := ClassifyReplicaChange(old, new)
		if change.Severity != ChangeSeverityLow {
			t.Fatalf("replica change should be low severity, got %q", change.Severity)
		}
	})
}

// --- Property: CheckWindow Varies By Change Type ---

func TestProperty_CheckWindowByChangeType(t *testing.T) {
	tests := []struct {
		changeType ChangeType
		minWindow  time.Duration
	}{
		{ChangeResource, 120 * time.Second},
		{ChangeImage, 90 * time.Second},
		{ChangeProbe, 90 * time.Second},
		{ChangeEnvVar, 60 * time.Second},
		{ChangeReplica, 20 * time.Second},
	}

	for _, tt := range tests {
		dc := DeploymentChange{
			Changes: []DetectedChange{{Type: tt.changeType}},
		}
		window := dc.CheckWindow()
		if window != tt.minWindow {
			t.Errorf("change type %s: expected window %s, got %s", tt.changeType, tt.minWindow, window)
		}
	}
}

// --- Property: HighestSeverity Returns Maximum ---

func TestProperty_HighestSeverityReturnsMax(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		numChanges := rapid.IntRange(1, 5).Draw(t, "numChanges")
		changes := make([]DetectedChange, numChanges)
		for i := range changes {
			sev := rapid.SampledFrom([]ChangeSeverity{
				ChangeSeverityLow, ChangeSeverityMedium, ChangeSeverityHigh,
			}).Draw(t, "sev")
			changes[i] = DetectedChange{Severity: sev}
		}

		dc := DeploymentChange{Changes: changes}
		highest := dc.HighestSeverity()

		// Verify it's actually the maximum.
		for _, c := range changes {
			if severityRank(c.Severity) > severityRank(highest) {
				t.Fatalf("HighestSeverity returned %q but found %q", highest, c.Severity)
			}
		}
	})
}

// --- Property: Verdict Status Is Correct Based On Checks ---

func TestProperty_VerdictStatusFromChecks(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		numChecks := rapid.IntRange(0, 8).Draw(t, "numChecks")
		checks := make([]SignalCheck, numChecks)
		for i := range checks {
			checks[i] = SignalCheck{
				Signal: "test_signal",
				Passed: rapid.Bool().Draw(t, "passed"),
			}
		}

		change := DeploymentChange{
			WorkloadName: "test",
			WorkloadKind: "Deployment",
			Namespace:    "default",
		}

		verdict := NewVerdict(change, checks, time.Now())

		if numChecks == 0 {
			if verdict.Status != VerdictInconclusive {
				t.Fatalf("0 checks should be inconclusive, got %q", verdict.Status)
			}
			return
		}

		allPassed := true
		for _, c := range checks {
			if !c.Passed {
				allPassed = false
				break
			}
		}

		if allPassed && verdict.Status != VerdictHealthy {
			t.Fatalf("all checks passed but verdict is %q", verdict.Status)
		}
		if !allPassed && verdict.Status != VerdictRegression {
			t.Fatalf("some checks failed but verdict is %q", verdict.Status)
		}
	})
}

// --- Property: Watcher Diff Detects Image Changes ---

func TestProperty_DiffDetectsImageChange(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		oldTag := rapid.StringMatching(`v[0-9]+\\.[0-9]+\\.[0-9]+`).Draw(t, "old")
		newTag := rapid.StringMatching(`v[0-9]+\\.[0-9]+\\.[0-9]+`).Draw(t, "new")

		prev := WorkloadSnapshot{
			Name: "api", Kind: "Deployment", Namespace: "prod",
			Generation: 1,
			ImageTags:  map[string]string{"app": "myimage:" + oldTag},
		}
		curr := WorkloadSnapshot{
			Name: "api", Kind: "Deployment", Namespace: "prod",
			Generation: 2,
			ImageTags:  map[string]string{"app": "myimage:" + newTag},
		}

		changes := diffSnapshots(prev, curr)

		if oldTag == newTag {
			if len(changes) != 0 {
				t.Fatal("same image tag should produce no changes")
			}
			return
		}

		if len(changes) == 0 {
			t.Fatal("different image tags should produce at least one change")
		}

		foundImage := false
		for _, c := range changes {
			if c.Type == ChangeImage {
				foundImage = true
			}
		}
		if !foundImage {
			t.Fatal("should have detected image change")
		}
	})
}

// --- Test: Signal Selection By Change Type ---

func TestVerifier_SignalSelectionByChangeType(t *testing.T) {
	v := NewVerifier(VerifierConfig{
		Metrics: &mockMetricsSource{},
		Emitter: &mockVerifierEmitter{},
	})

	tests := []struct {
		changeType     ChangeType
		expectedSignal signalType
	}{
		{ChangeImage, signalErrorRate},
		{ChangeImage, signalLatencyP99},
		{ChangeEnvVar, signalErrorRate},
		{ChangeResource, signalCPU},
		{ChangeReplica, signalThroughput},
		{ChangeProbe, signalPodRestarts},
	}

	for _, tt := range tests {
		dc := DeploymentChange{Changes: []DetectedChange{{Type: tt.changeType}}}
		signals := v.selectSignals(dc)

		found := false
		for _, s := range signals {
			if s == tt.expectedSignal {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("change type %s should select signal %s", tt.changeType, tt.expectedSignal)
		}
	}
}

// --- Test: evaluateCheck Logic ---

func TestEvaluateCheck(t *testing.T) {
	// Error rate: baseline 0.02, current 0.03, threshold 1.5 → 0.03 <= 0.03 → pass
	if !evaluateCheck(signalErrorRate, 0.02, 0.03, 1.5) {
		t.Error("error rate 0.03 with baseline 0.02 and threshold 1.5 should pass")
	}

	// Error rate: baseline 0.02, current 0.05, threshold 1.5 → 0.05 > 0.03 → fail
	if evaluateCheck(signalErrorRate, 0.02, 0.05, 1.5) {
		t.Error("error rate 0.05 with baseline 0.02 and threshold 1.5 should fail")
	}

	// Pod restarts: current 0, threshold 1 → pass
	if !evaluateCheck(signalPodRestarts, 0, 0, 1.0) {
		t.Error("0 restarts should pass")
	}

	// Pod restarts: current 2, threshold 1 → fail
	if evaluateCheck(signalPodRestarts, 0, 2, 1.0) {
		t.Error("2 restarts with threshold 1 should fail")
	}

	// Throughput: baseline 100, current 80, threshold 0.7 → 80 >= 70 → pass
	if !evaluateCheck(signalThroughput, 100, 80, 0.7) {
		t.Error("throughput 80 with baseline 100 and threshold 0.7 should pass")
	}

	// Throughput: baseline 100, current 60, threshold 0.7 → 60 < 70 → fail
	if evaluateCheck(signalThroughput, 100, 60, 0.7) {
		t.Error("throughput 60 with baseline 100 and threshold 0.7 should fail")
	}
}

// --- Mocks ---

type mockMetricsSource struct{}

func (m *mockMetricsSource) QueryErrorRate(_ context.Context, _, _ string, _ time.Duration) (float64, error) {
	return 0.01, nil
}
func (m *mockMetricsSource) QueryLatencyP99(_ context.Context, _, _ string, _ time.Duration) (float64, error) {
	return 50.0, nil
}
func (m *mockMetricsSource) QueryPodRestarts(_ context.Context, _, _ string, _ time.Duration) (float64, error) {
	return 0, nil
}
func (m *mockMetricsSource) QueryThroughput(_ context.Context, _, _ string, _ time.Duration) (float64, error) {
	return 100.0, nil
}
func (m *mockMetricsSource) QueryCPUSaturation(_ context.Context, _, _ string, _ time.Duration) (float64, error) {
	return 0.3, nil
}

type mockVerifierEmitter struct {
	events []export.Event
}

func (m *mockVerifierEmitter) Emit(_ context.Context, event export.Event) error {
	m.events = append(m.events, event)
	return nil
}
func (m *mockVerifierEmitter) Flush(_ context.Context) error { return nil }
func (m *mockVerifierEmitter) BufferLen() int                { return 0 }
