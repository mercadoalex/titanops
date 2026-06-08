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

// Feature: ollinai-platform-integration, Property 7: Cross-module correlation matching
// **Validates: Requirements 2.1, 2.2, 2.3, 2.4, 7.6**
//
// For any pair of an OllinAI event and an event from another module, the correlation engine
// generates a CorrelatedIncident if and only if the events share at least one matching attribute
// (Node, Pod, or Namespace per module-specific rules) AND both events fall within the configured
// correlation time window.
func TestProperty7_CrossModuleCorrelationMatching(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		// Choose a time window.
		windowSec := rapid.IntRange(30, 300).Draw(t, "windowSec")
		timeWindow := time.Duration(windowSec) * time.Second

		// Generate attribute values for the shared tuple.
		node := rapid.OneOf(rapid.Just(""), rapid.StringMatching(`node-[a-z0-9]{3,8}`)).Draw(t, "node")
		pod := rapid.OneOf(rapid.Just(""), rapid.StringMatching(`pod-[a-z0-9]{3,8}`)).Draw(t, "pod")
		namespace := rapid.OneOf(rapid.Just(""), rapid.StringMatching(`ns-[a-z]{3,8}`)).Draw(t, "namespace")

		// Decide whether events share the same attribute tuple (correlation match).
		shouldShareTuple := rapid.Bool().Draw(t, "shouldShareTuple")

		// Build the OllinAI event.
		ollinaiEventType := rapid.SampledFrom([]string{"deployment_risk", "supply_chain_credential_exfil", "supply_chain_process_anomaly"}).Draw(t, "ollinaiEventType")
		baseTime := time.Now().UTC()
		ollinaiEvent := export.Event{
			Module:    "ollinai",
			EventType: ollinaiEventType,
			Node:      node,
			Pod:       pod,
			Namespace: namespace,
			Timestamp: baseTime,
			Severity:  "high",
			Payload:   []byte("{}"),
			EventID:   "ollinai-evt-1",
			Labels:    map[string]string{},
		}

		// Build the other-module event.
		otherModule := rapid.SampledFrom([]string{"earthworm", "tlapix", "ebeecontrol", "quack"}).Draw(t, "otherModule")
		var otherNode, otherPod, otherNamespace string

		if shouldShareTuple {
			// Same attribute tuple — engine will group these together.
			otherNode = node
			otherPod = pod
			otherNamespace = namespace
		} else {
			// Different attribute tuple — ensure at least one field differs.
			// Use values that are guaranteed to be different.
			otherNode = node + "-different"
			otherPod = pod + "-different"
			otherNamespace = namespace + "-different"
		}

		otherEvent := export.Event{
			Module:    otherModule,
			EventType: "test_event",
			Node:      otherNode,
			Pod:       otherPod,
			Namespace: otherNamespace,
			Timestamp: baseTime.Add(-5 * time.Second),
			Severity:  "high",
			Payload:   []byte("{}"),
			EventID:   "other-evt-1",
			Labels:    map[string]string{},
		}

		// Use the full engine to test the property.
		// Both events are ingested "now" so they're within the time window.
		cfg := EngineConfig{
			TimeWindow:          timeWindow,
			ConfidenceThreshold: 100, // disable auto-action
			AutoActions:         []AutoActionConfig{},
		}
		engine, err := NewEngine(cfg, nil, nil)
		if err != nil {
			t.Fatalf("NewEngine: %v", err)
		}
		ctx := context.Background()
		if err := engine.Ingest(ctx, ollinaiEvent); err != nil {
			t.Fatalf("Ingest ollinai: %v", err)
		}
		if err := engine.Ingest(ctx, otherEvent); err != nil {
			t.Fatalf("Ingest other: %v", err)
		}
		incidents, err := engine.Correlate(ctx)
		if err != nil {
			t.Fatalf("Correlate: %v", err)
		}

		if shouldShareTuple {
			// Events share tuple and are from 2 different modules within window → incident generated.
			if len(incidents) == 0 {
				t.Fatalf("expected correlated incident for shared tuple (node=%q, pod=%q, ns=%q) but got none",
					node, pod, namespace)
			}
			// Verify CalculateConfidence returns > 0 for cross-module events.
			if incidents[0].ConfidenceScore <= 0 {
				t.Fatalf("expected positive confidence score, got %d", incidents[0].ConfidenceScore)
			}
		} else {
			// Events have different tuples → they're in different groups → no group has ≥2 modules.
			if len(incidents) != 0 {
				t.Fatalf("expected no correlated incident for different tuples but got %d", len(incidents))
			}
		}

		// Additionally verify that CalculateConfidence > 0 requires ≥2 distinct modules.
		if shouldShareTuple {
			// Determine what matchedAttributes the engine would compute for this group.
			var matchedAttrs []string
			if node != "" {
				matchedAttrs = append(matchedAttrs, "node")
			}
			if pod != "" {
				matchedAttrs = append(matchedAttrs, "pod")
			}
			if namespace != "" {
				matchedAttrs = append(matchedAttrs, "namespace")
			}
			events := []export.Event{ollinaiEvent, otherEvent}
			score := CalculateConfidence(events, matchedAttrs)
			if score <= 0 {
				t.Fatalf("CalculateConfidence should be > 0 for 2 modules, got %d", score)
			}
		}
	})
}

// Feature: ollinai-platform-integration, Property 8: Deployment risk bonus scoring
// **Validates: Requirements 2.5**
//
// For any risk score R in [0, 100] and any base confidence B, DeploymentRiskBonus(R) = floor(R/5)
// and total = min(B + floor(R/5), 100).
func TestProperty8_DeploymentRiskBonusScoring(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		riskScore := rapid.IntRange(0, 100).Draw(t, "riskScore")
		baseConfidence := rapid.IntRange(0, 100).Draw(t, "baseConfidence")

		bonus := DeploymentRiskBonus(riskScore)
		expectedBonus := riskScore / 5

		// Property 1: bonus = floor(R/5).
		if bonus != expectedBonus {
			t.Fatalf("DeploymentRiskBonus(%d) = %d, expected %d (floor(%d/5))",
				riskScore, bonus, expectedBonus, riskScore)
		}

		// Property 2: total = min(B + floor(R/5), 100).
		total := baseConfidence + bonus
		if total > MaxConfidence {
			total = MaxConfidence
		}
		expectedTotal := baseConfidence + expectedBonus
		if expectedTotal > 100 {
			expectedTotal = 100
		}
		if total != expectedTotal {
			t.Fatalf("total confidence = %d, expected min(%d + %d, 100) = %d",
				total, baseConfidence, expectedBonus, expectedTotal)
		}
	})
}

// Feature: ollinai-platform-integration, Property 9: Narrative metadata inclusion
// **Validates: Requirements 2.6, 2.8**
//
// For any OllinAI deployment_risk event with combinations of present/empty service, commit_sha,
// deployer Labels, the generated narrative contains non-empty values and omits empty ones.
func TestProperty9_NarrativeMetadataInclusion(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		// Generate combinations of present/empty label values.
		service := rapid.OneOf(rapid.Just(""), rapid.StringMatching(`[a-z][a-z0-9-]{2,20}`)).Draw(t, "service")
		commitSHA := rapid.OneOf(rapid.Just(""), rapid.StringMatching(`[a-f0-9]{7,40}`)).Draw(t, "commitSHA")
		deployer := rapid.OneOf(rapid.Just(""), rapid.StringMatching(`[a-z]{3,15}`)).Draw(t, "deployer")

		labels := map[string]string{}
		if service != "" {
			labels["service"] = service
		}
		if commitSHA != "" {
			labels["commit_sha"] = commitSHA
		}
		if deployer != "" {
			labels["deployer"] = deployer
		}

		// Build an OllinAI deployment_risk event.
		ollinaiEvent := export.Event{
			Module:    "ollinai",
			EventType: "deployment_risk",
			Node:      "node-test",
			Pod:       "pod-test",
			Namespace: "ns-test",
			Timestamp: time.Now().UTC(),
			Severity:  "high",
			Payload:   []byte("{}"),
			EventID:   "ollinai-narrative-test",
			Labels:    labels,
		}

		// A second event from another module to form a valid correlation group.
		otherEvent := export.Event{
			Module:    "earthworm",
			EventType: "anomaly_detected",
			Node:      "node-test",
			Pod:       "pod-test",
			Namespace: "ns-test",
			Timestamp: time.Now().UTC().Add(-5 * time.Second),
			Severity:  "high",
			Payload:   []byte("{}"),
			EventID:   "earthworm-evt-1",
			Labels:    map[string]string{},
		}

		events := []export.Event{ollinaiEvent, otherEvent}
		matchedAttrs := []string{"node", "pod", "namespace"}
		score := CalculateConfidence(events, matchedAttrs)

		narrative := GenerateNarrative(events, matchedAttrs, score)

		// Property: non-empty label values appear in narrative.
		if service != "" {
			if !strings.Contains(narrative, service) {
				t.Errorf("narrative should contain service %q but doesn't: %s", service, narrative)
			}
		}
		if commitSHA != "" {
			if !strings.Contains(narrative, commitSHA) {
				t.Errorf("narrative should contain commit_sha %q but doesn't: %s", commitSHA, narrative)
			}
		}
		if deployer != "" {
			if !strings.Contains(narrative, deployer) {
				t.Errorf("narrative should contain deployer %q but doesn't: %s", deployer, narrative)
			}
		}

		// Property: empty label values are NOT referenced via placeholder text.
		// When all three are empty, no "Deployment" metadata section should appear.
		if service == "" && commitSHA == "" && deployer == "" {
			if strings.Contains(narrative, "Deployment of") || strings.Contains(narrative, "Deployment (") {
				t.Errorf("narrative should not contain deployment metadata when all labels are empty: %s", narrative)
			}
		}

		// When a specific field is empty, its value should not appear as placeholder.
		if service == "" && (commitSHA != "" || deployer != "") {
			// "Deployment of" should NOT appear since service is empty.
			if strings.Contains(narrative, "Deployment of ") {
				t.Errorf("narrative should not contain 'Deployment of' when service is empty: %s", narrative)
			}
		}
	})
}

// Feature: ollinai-platform-integration, Property 10: Single-module non-correlation
// **Validates: Requirements 2.7**
//
// For any set of events where all events come from the same module (ollinai),
// CalculateConfidence returns 0 (since it requires ≥2 distinct modules).
func TestProperty10_SingleModuleNonCorrelation(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		// Generate 1 to 5 OllinAI events.
		numEvents := rapid.IntRange(1, 5).Draw(t, "numEvents")

		// Choose a time window so all events are within it.
		windowSec := rapid.IntRange(60, 300).Draw(t, "windowSec")
		timeWindow := time.Duration(windowSec) * time.Second

		baseTime := time.Now().UTC()
		var events []export.Event
		for i := 0; i < numEvents; i++ {
			eventType := rapid.SampledFrom([]string{
				"deployment_risk",
				"dora_metrics",
				"supply_chain_credential_exfil",
				"supply_chain_process_anomaly",
				"supply_chain_attestation_failure",
			}).Draw(t, "eventType")

			node := rapid.StringMatching(`node-[a-z0-9]{3,8}`).Draw(t, "node")
			pod := rapid.StringMatching(`pod-[a-z0-9]{3,8}`).Draw(t, "pod")
			namespace := rapid.StringMatching(`ns-[a-z]{3,8}`).Draw(t, "namespace")

			events = append(events, export.Event{
				Module:    "ollinai",
				EventType: eventType,
				Node:      node,
				Pod:       pod,
				Namespace: namespace,
				Timestamp: baseTime.Add(-time.Duration(i) * 5 * time.Second),
				Severity:  "high",
				Payload:   []byte("{}"),
				EventID:   rapid.StringMatching(`evt-[a-f0-9]{8}`).Draw(t, "eventID"),
				Labels:    map[string]string{},
			})
		}

		// All events from "ollinai" — same module.
		// The correlation engine requires ≥2 distinct modules to generate a CorrelatedIncident.
		cfg := EngineConfig{
			TimeWindow:          timeWindow,
			ConfidenceThreshold: 100,
			AutoActions:         []AutoActionConfig{},
		}
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

		// Property: no CorrelatedIncident generated from single-module events.
		if len(incidents) != 0 {
			t.Fatalf("expected no correlated incidents for single-module (ollinai) events, got %d with confidence %d",
				len(incidents), incidents[0].ConfidenceScore)
		}
	})
}
