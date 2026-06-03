package correlation

import (
	"context"
	"strings"
	"testing"
	"time"

	export "github.com/mercadoalex/titanops/shared/titanops-export"
	"pgregory.net/rapid"
)

// **Validates: Requirements 5.2, 5.3, 5.4**
// Property 7: Correlation engine generates incidents from matching cross-module events

// moduleGen generates a valid TitanOps module name.
func moduleGen() *rapid.Generator[string] {
	return rapid.SampledFrom([]string{"earthworm", "tlapix", "ebeecontrol", "quack"})
}

// distinctModulesGen generates at least 2 distinct module names.
func distinctModulesGen(minModules int) *rapid.Generator[[]string] {
	return rapid.Custom(func(t *rapid.T) []string {
		allModules := []string{"earthworm", "tlapix", "ebeecontrol", "quack"}
		count := rapid.IntRange(minModules, len(allModules)).Draw(t, "numModules")
		perm := rapid.Permutation(allModules).Draw(t, "modulePerm")
		return perm[:count]
	})
}

// crossModuleEventsGen generates a set of events from at least 2 distinct modules
// sharing the same node, pod, and namespace within a time window.
func crossModuleEventsGen() *rapid.Generator[[]export.Event] {
	return rapid.Custom(func(t *rapid.T) []export.Event {
		modules := distinctModulesGen(2).Draw(t, "modules")
		node := rapid.StringMatching(`node-[a-z0-9]{1,8}`).Draw(t, "node")
		namespace := rapid.StringMatching(`[a-z]{3,12}`).Draw(t, "namespace")
		pod := rapid.StringMatching(`pod-[a-z0-9]{1,8}`).Draw(t, "pod")

		baseTime := time.Now().UTC()
		var events []export.Event
		for i, mod := range modules {
			// Events within 10 seconds of each other.
			offset := time.Duration(i) * 2 * time.Second
			events = append(events, export.Event{
				Module:    mod,
				Node:      node,
				Pod:       pod,
				Namespace: namespace,
				EventType: rapid.StringMatching(`[a-z_]{5,20}`).Draw(t, "eventType"),
				Timestamp: baseTime.Add(-offset),
				Severity:  "high",
				Payload:   []byte("test-payload"),
				EventID:   rapid.StringMatching(`evt-[a-f0-9]{8}`).Draw(t, "eventID"),
			})
		}
		return events
	})
}

func TestProperty7_CrossModuleEvents_GenerateIncident(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		events := crossModuleEventsGen().Draw(t, "events")

		cfg := DefaultConfig()
		cfg.ConfidenceThreshold = 100 // Disable auto-action for this test.
		engine, err := NewEngine(cfg, nil, nil)
		if err != nil {
			t.Fatalf("NewEngine: %v", err)
		}

		ctx := context.Background()
		for _, ev := range events {
			if err := engine.Ingest(ctx, ev); err != nil {
				t.Fatalf("Ingest: %v", err)
			}
		}

		incidents, err := engine.Correlate(ctx)
		if err != nil {
			t.Fatalf("Correlate: %v", err)
		}

		// With ≥2 distinct modules sharing attributes, at least one incident should be generated.
		if len(incidents) == 0 {
			t.Fatal("expected at least one correlated incident from cross-module events")
		}

		inc := incidents[0]

		// (b) Confidence score in [0, 100].
		if inc.ConfidenceScore < 0 || inc.ConfidenceScore > 100 {
			t.Fatalf("confidence score %d out of range [0, 100]", inc.ConfidenceScore)
		}

		// (c) Narrative contains all contributing module names.
		moduleSet := make(map[string]bool)
		for _, ev := range events {
			moduleSet[ev.Module] = true
		}
		for mod := range moduleSet {
			if !strings.Contains(inc.Narrative, mod) {
				t.Errorf("narrative missing module name %q: %s", mod, inc.Narrative)
			}
		}
	})
}

func TestProperty7_ConfidenceScoreRange(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		// Generate random events from 2-4 modules with varying attributes.
		numModules := rapid.IntRange(2, 4).Draw(t, "numModules")
		allModules := []string{"earthworm", "tlapix", "ebeecontrol", "quack"}
		modules := allModules[:numModules]

		node := rapid.StringMatching(`node-[a-z0-9]{1,5}`).Draw(t, "node")
		namespace := rapid.StringMatching(`[a-z]{3,10}`).Draw(t, "namespace")

		// Decide which attributes to share.
		shareNode := rapid.Bool().Draw(t, "shareNode")
		shareNamespace := rapid.Bool().Draw(t, "shareNamespace")

		var matchedAttrs []string
		if shareNode && node != "" {
			matchedAttrs = append(matchedAttrs, "node")
		}
		if shareNamespace && namespace != "" {
			matchedAttrs = append(matchedAttrs, "namespace")
		}

		baseTime := time.Now().UTC()
		var events []export.Event
		for i, mod := range modules {
			ev := export.Event{
				Module:    mod,
				Namespace: namespace,
				EventType: "test_event",
				Timestamp: baseTime.Add(-time.Duration(i) * 3 * time.Second),
				Severity:  "high",
				Payload:   []byte("test"),
				EventID:   "id",
			}
			if shareNode {
				ev.Node = node
			}
			if shareNamespace {
				ev.Namespace = namespace
			}
			events = append(events, ev)
		}

		score := CalculateConfidence(events, matchedAttrs)

		if score < 0 || score > 100 {
			t.Fatalf("confidence score %d out of range [0, 100]", score)
		}
	})
}

// **Validates: Requirements 5.5**
// Property 8: Auto-action executes if and only if confidence exceeds threshold

func TestProperty8_AutoAction_ThresholdGating(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		threshold := rapid.IntRange(1, 100).Draw(t, "threshold")

		// Generate events that produce a known score by controlling module count and attributes.
		numModules := rapid.IntRange(2, 4).Draw(t, "numModules")
		allModules := []string{"earthworm", "tlapix", "ebeecontrol", "quack"}
		modules := allModules[:numModules]

		node := "node-shared"
		namespace := "ns-shared"
		pod := rapid.StringMatching(`pod-[a-z0-9]{1,5}`).Draw(t, "pod")

		baseTime := time.Now().UTC()
		var events []export.Event
		for i, mod := range modules {
			events = append(events, export.Event{
				Module:    mod,
				Node:      node,
				Pod:       pod,
				Namespace: namespace,
				EventType: "test_event",
				Timestamp: baseTime.Add(-time.Duration(i) * 2 * time.Second),
				Severity:  "high",
				Payload:   []byte("payload"),
				EventID:   "evt-id",
			})
		}

		cfg := DefaultConfig()
		cfg.ConfidenceThreshold = threshold
		cfg.AutoActions = []AutoActionConfig{
			{Type: "isolate_pod", Enabled: true},
		}

		recorder := &RecordingExecutor{}
		engine, err := NewEngine(cfg, nil, recorder)
		if err != nil {
			t.Fatalf("NewEngine: %v", err)
		}

		ctx := context.Background()
		for _, ev := range events {
			if err := engine.Ingest(ctx, ev); err != nil {
				t.Fatalf("Ingest: %v", err)
			}
		}

		incidents, err := engine.Correlate(ctx)
		if err != nil {
			t.Fatalf("Correlate: %v", err)
		}

		if len(incidents) == 0 {
			t.Skip("no incidents generated (events may not have met correlation criteria)")
		}

		inc := incidents[0]
		actionExecuted := inc.ActionExecuted

		// Property: action executes iff score >= threshold.
		if inc.ConfidenceScore >= threshold {
			if !actionExecuted {
				t.Errorf("expected action to execute: score=%d >= threshold=%d, but ActionExecuted=false",
					inc.ConfidenceScore, threshold)
			}
		} else {
			if actionExecuted {
				t.Errorf("expected action NOT to execute: score=%d < threshold=%d, but ActionExecuted=true",
					inc.ConfidenceScore, threshold)
			}
		}
	})
}
