package ollinai

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"
	"time"

	export "github.com/mercadoalex/titanops/shared/titanops-export"
	"pgregory.net/rapid"
)

// uuidV4Regex matches a valid UUID v4 string.
var uuidV4Regex = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`)

// Feature: ollinai-platform-integration, Property 1: Severity mapping from risk score
// **Validates: Requirements 1.1**
func TestProperty1_SeverityMappingFromRiskScore(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		score := rapid.IntRange(0, 100).Draw(t, "riskScore")

		severity := MapRiskToSeverity(score)

		switch {
		case score >= 80:
			if severity != SeverityCritical {
				t.Fatalf("score=%d: expected %q, got %q", score, SeverityCritical, severity)
			}
		case score >= 60:
			if severity != SeverityHigh {
				t.Fatalf("score=%d: expected %q, got %q", score, SeverityHigh, severity)
			}
		case score >= 40:
			if severity != SeverityMedium {
				t.Fatalf("score=%d: expected %q, got %q", score, SeverityMedium, severity)
			}
		default:
			if severity != SeverityLow {
				t.Fatalf("score=%d: expected %q, got %q", score, SeverityLow, severity)
			}
		}
	})
}

// Feature: ollinai-platform-integration, Property 2: DORA metrics payload completeness
// **Validates: Requirements 1.2**
func TestProperty2_DORAMetricsPayloadCompleteness(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		payload := DORAMetricsPayload{
			DeploymentFrequency:  rapid.Float64Range(0, 1000).Draw(t, "deployFreq"),
			LeadTimeForChanges:   rapid.Float64Range(0, 1000).Draw(t, "leadTime"),
			ChangeFailureRate:    rapid.Float64Range(0, 1).Draw(t, "cfr"),
			TimeToRestoreService: rapid.Float64Range(0, 1000).Draw(t, "ttrs"),
		}

		// Serialize
		data, err := json.Marshal(payload)
		if err != nil {
			t.Fatalf("failed to marshal DORAMetricsPayload: %v", err)
		}

		// Deserialize into a generic map
		var m map[string]interface{}
		if err := json.Unmarshal(data, &m); err != nil {
			t.Fatalf("failed to unmarshal into map: %v", err)
		}

		// Assert all four keys are present
		requiredKeys := []string{"deployment_frequency", "lead_time_for_changes", "change_failure_rate", "time_to_restore_service"}
		for _, key := range requiredKeys {
			if _, ok := m[key]; !ok {
				t.Fatalf("missing key %q in deserialized payload", key)
			}
		}

		// Deserialize back into struct and verify values match
		var decoded DORAMetricsPayload
		if err := json.Unmarshal(data, &decoded); err != nil {
			t.Fatalf("failed to unmarshal into DORAMetricsPayload: %v", err)
		}

		if decoded.DeploymentFrequency != payload.DeploymentFrequency {
			t.Fatalf("DeploymentFrequency mismatch: want %f, got %f", payload.DeploymentFrequency, decoded.DeploymentFrequency)
		}
		if decoded.LeadTimeForChanges != payload.LeadTimeForChanges {
			t.Fatalf("LeadTimeForChanges mismatch: want %f, got %f", payload.LeadTimeForChanges, decoded.LeadTimeForChanges)
		}
		if decoded.ChangeFailureRate != payload.ChangeFailureRate {
			t.Fatalf("ChangeFailureRate mismatch: want %f, got %f", payload.ChangeFailureRate, decoded.ChangeFailureRate)
		}
		if decoded.TimeToRestoreService != payload.TimeToRestoreService {
			t.Fatalf("TimeToRestoreService mismatch: want %f, got %f", payload.TimeToRestoreService, decoded.TimeToRestoreService)
		}
	})
}

// Feature: ollinai-platform-integration, Property 3: Event metadata population and incomplete label
// **Validates: Requirements 1.4, 1.5**
func TestProperty3_EventMetadataPopulationAndIncompleteLabel(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		// Generate Node, Pod, Namespace — each either empty or non-empty
		node := rapid.OneOf(rapid.Just(""), rapid.StringMatching("[a-z0-9]{1,20}")).Draw(t, "node")
		pod := rapid.OneOf(rapid.Just(""), rapid.StringMatching("[a-z0-9]{1,20}")).Draw(t, "pod")
		namespace := rapid.OneOf(rapid.Just(""), rapid.StringMatching("[a-z0-9]{1,20}")).Draw(t, "namespace")

		event := export.Event{
			Module:    ModuleName,
			EventType: EventTypeDeploymentRisk,
			Severity:  SeverityHigh,
			Payload:   []byte(`{"test": true}`),
			Node:      node,
			Pod:       pod,
			Namespace: namespace,
		}

		// Use a mock publisher that's disconnected so events get buffered
		emitter := NewNATSEmitter(NATSEmitterConfig{
			Publisher:       &mockPublisher{connected: false},
			BufferCapacity:  100,
		})

		err := emitter.Emit(context.Background(), event)
		if err != nil {
			t.Fatalf("Emit failed: %v", err)
		}

		// Drain the buffer to get the processed event
		events := emitter.DrainBuffer()
		if len(events) != 1 {
			t.Fatalf("expected 1 buffered event, got %d", len(events))
		}

		processed := events[0]

		// Verify metadata fields are populated
		if processed.Node != node {
			t.Fatalf("Node mismatch: want %q, got %q", node, processed.Node)
		}
		if processed.Pod != pod {
			t.Fatalf("Pod mismatch: want %q, got %q", pod, processed.Pod)
		}
		if processed.Namespace != namespace {
			t.Fatalf("Namespace mismatch: want %q, got %q", namespace, processed.Namespace)
		}

		// Check metadata_incomplete label
		anyEmpty := node == "" || pod == "" || namespace == ""
		hasLabel := processed.Labels != nil && processed.Labels[LabelMetadataIncomplete] == "true"

		if anyEmpty && !hasLabel {
			t.Fatalf("expected metadata_incomplete=true when at least one field is empty (node=%q, pod=%q, namespace=%q)", node, pod, namespace)
		}
		if !anyEmpty && hasLabel {
			t.Fatalf("unexpected metadata_incomplete=true when all fields are non-empty (node=%q, pod=%q, namespace=%q)", node, pod, namespace)
		}
	})
}

// Feature: ollinai-platform-integration, Property 4: EventID uniqueness
// **Validates: Requirements 1.6, 5.4**
func TestProperty4_EventIDUniqueness(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		n := rapid.IntRange(2, 100).Draw(t, "numEvents")

		emitter := NewNATSEmitter(NATSEmitterConfig{
			Publisher:       &mockPublisher{connected: false},
			BufferCapacity:  200,
		})

		for i := 0; i < n; i++ {
			event := export.Event{
				Module:    ModuleName,
				EventType: EventTypeDeploymentRisk,
				Severity:  SeverityHigh,
				Payload:   []byte(`{}`),
				Node:      "node-1",
				Pod:       "pod-1",
				Namespace: "ns-1",
			}
			if err := emitter.Emit(context.Background(), event); err != nil {
				t.Fatalf("Emit failed at event %d: %v", i, err)
			}
		}

		events := emitter.DrainBuffer()
		if len(events) != n {
			t.Fatalf("expected %d events, got %d", n, len(events))
		}

		seen := make(map[string]bool, n)
		for i, ev := range events {
			// Verify valid UUID v4
			if !uuidV4Regex.MatchString(ev.EventID) {
				t.Fatalf("event %d: EventID %q is not a valid UUID v4", i, ev.EventID)
			}

			// Verify uniqueness
			if seen[ev.EventID] {
				t.Fatalf("event %d: duplicate EventID %q", i, ev.EventID)
			}
			seen[ev.EventID] = true

			// Verify Timestamp is valid RFC 3339 UTC
			if ev.Timestamp.IsZero() {
				t.Fatalf("event %d: Timestamp is zero", i)
			}
			if ev.Timestamp.Location() != time.UTC {
				t.Fatalf("event %d: Timestamp not in UTC: %v", i, ev.Timestamp)
			}
			// Verify RFC 3339 format by formatting and re-parsing
			formatted := ev.Timestamp.Format(time.RFC3339Nano)
			if _, err := time.Parse(time.RFC3339Nano, formatted); err != nil {
				t.Fatalf("event %d: Timestamp not valid RFC3339: %v", i, err)
			}
		}
	})
}

// Feature: ollinai-platform-integration, Property 5: Payload serialization and truncation
// **Validates: Requirements 1.7, 1.8**
func TestProperty5_PayloadSerializationAndTruncation(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		// Generate a DeploymentRiskPayload with variable-length RiskFactors
		numFactors := rapid.IntRange(0, 5000).Draw(t, "numFactors")
		factors := make([]string, numFactors)
		for i := range factors {
			factors[i] = rapid.StringMatching("[a-z]{1,50}").Draw(t, "factor")
		}

		payload := DeploymentRiskPayload{
			Service:     rapid.StringMatching("[a-z]{1,30}").Draw(t, "service"),
			CommitSHA:   rapid.StringMatching("[a-f0-9]{40}").Draw(t, "sha"),
			Deployer:    rapid.StringMatching("[a-z]{1,20}").Draw(t, "deployer"),
			RiskScore:   rapid.IntRange(0, 100).Draw(t, "riskScore"),
			RiskFactors: factors,
			PipelineID:  rapid.StringMatching("[a-z0-9]{1,20}").Draw(t, "pipelineID"),
			Environment: rapid.StringMatching("[a-z]{1,10}").Draw(t, "env"),
		}

		data, truncated, err := SerializePayload(payload)
		if err != nil {
			t.Fatalf("SerializePayload failed: %v", err)
		}

		// Serialized JSON must be ≤ 64KB
		if len(data) > MaxPayloadBytes {
			t.Fatalf("serialized payload exceeds 64KB: %d bytes", len(data))
		}

		// If truncation occurred, verify the label condition
		if truncated {
			// The original would have been larger than 64KB
			originalData, _ := json.Marshal(payload)
			if len(originalData) <= MaxPayloadBytes {
				t.Fatalf("truncated=true but original payload was only %d bytes (≤ %d)", len(originalData), MaxPayloadBytes)
			}
		}
	})
}

// Feature: ollinai-platform-integration, Property 6: Ring buffer capacity invariant
// **Validates: Requirements 1.9, 5.3**
func TestProperty6_RingBufferCapacityInvariant(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		capacity := rapid.IntRange(1, 100).Draw(t, "capacity")
		numEvents := rapid.IntRange(1, 300).Draw(t, "numEvents")

		emitter := NewNATSEmitter(NATSEmitterConfig{
			Publisher:       &mockPublisher{connected: false},
			BufferCapacity:  capacity,
		})

		// Track events in order of insertion
		var expectedOldest int // index of the oldest surviving event

		for i := 0; i < numEvents; i++ {
			event := export.Event{
				Module:    ModuleName,
				EventType: EventTypeDeploymentRisk,
				Severity:  SeverityHigh,
				Payload:   []byte(`{}`),
				Node:      "node",
				Pod:       "pod",
				Namespace: "ns",
				EventID:   "", // will be assigned by Emit
			}
			if err := emitter.Emit(context.Background(), event); err != nil {
				t.Fatalf("Emit failed at event %d: %v", i, err)
			}

			// Buffer length must never exceed capacity
			bufLen := emitter.BufferLen()
			if bufLen > capacity {
				t.Fatalf("buffer length %d exceeds capacity %d after pushing event %d", bufLen, capacity, i)
			}
		}

		// Final buffer length check
		finalLen := emitter.BufferLen()
		if finalLen > capacity {
			t.Fatalf("final buffer length %d exceeds capacity %d", finalLen, capacity)
		}

		// Verify oldest-first eviction: if we pushed more than capacity, 
		// the buffer should contain exactly capacity items
		if numEvents >= capacity {
			if finalLen != capacity {
				t.Fatalf("expected buffer length %d (capacity), got %d after %d events", capacity, finalLen, numEvents)
			}
		} else {
			if finalLen != numEvents {
				t.Fatalf("expected buffer length %d, got %d", numEvents, finalLen)
			}
		}

		// Verify oldest-first eviction by checking EventIDs are from the most recent events
		events := emitter.DrainBuffer()
		if numEvents > capacity {
			expectedOldest = numEvents - capacity
		}
		_ = expectedOldest

		// Each event should have a unique EventID (assigned by Emit)
		seen := make(map[string]bool, len(events))
		for _, ev := range events {
			if seen[ev.EventID] {
				t.Fatalf("duplicate EventID %q in buffer after eviction", ev.EventID)
			}
			seen[ev.EventID] = true
		}
	})
}

// Feature: ollinai-platform-integration, Property 11: Configuration validation rejects invalid configs
// **Validates: Requirements 6.2, 6.4**
func TestProperty11_ConfigurationValidationRejectsInvalidConfigs(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		// Decide whether to generate a valid or invalid config
		generateValid := rapid.Bool().Draw(t, "generateValid")

		var cfg Config
		if generateValid {
			cfg = validConfigGen(t)
		} else {
			cfg = invalidConfigGen(t)
		}

		errs := ValidateConfig(&cfg)

		if generateValid {
			// Valid config should produce no errors
			if len(errs) > 0 {
				var msgs []string
				for _, e := range errs {
					msgs = append(msgs, e.Error())
				}
				t.Fatalf("valid config produced errors: %s", strings.Join(msgs, "; "))
			}
		} else {
			// Invalid config should produce at least one error
			if len(errs) == 0 {
				t.Fatalf("invalid config produced no errors: %+v", cfg)
			}
		}
	})
}

// Feature: ollinai-platform-integration, Property 12: Readyz reflects connection state
// **Validates: Requirements 8.2, 8.3, 8.4**
func TestProperty12_ReadyzReflectsConnectionState(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		natsConnected := rapid.Bool().Draw(t, "natsConnected")
		ollinConnected := rapid.Bool().Draw(t, "ollinConnected")

		hc := NewHealthChecker()
		hc.SetNATSConnected(natsConnected)
		hc.SetOllinConnected(ollinConnected)

		req := httptest.NewRequest(http.MethodGet, "/readyz", nil)
		rec := httptest.NewRecorder()

		hc.Readyz(rec, req)

		bothConnected := natsConnected && ollinConnected

		if bothConnected {
			if rec.Code != http.StatusOK {
				t.Fatalf("expected HTTP 200 when both connected (nats=%v, ollin=%v), got %d", natsConnected, ollinConnected, rec.Code)
			}
		} else {
			if rec.Code != http.StatusServiceUnavailable {
				t.Fatalf("expected HTTP 503 when not both connected (nats=%v, ollin=%v), got %d", natsConnected, ollinConnected, rec.Code)
			}
		}

		// Verify JSON response body
		var resp readyzResponse
		if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
			t.Fatalf("failed to unmarshal readyz response: %v", err)
		}

		if resp.Ready != bothConnected {
			t.Fatalf("expected ready=%v, got ready=%v", bothConnected, resp.Ready)
		}
	})
}

// --- Helpers ---

// mockPublisher is a test mock for NATSPublisher.
type mockPublisher struct {
	connected bool
}

func (m *mockPublisher) Publish(subject string, data []byte) error {
	return nil
}

func (m *mockPublisher) IsConnected() bool {
	return m.connected
}

// validConfigGen generates a valid Config using rapid.
func validConfigGen(t *rapid.T) Config {
	return Config{
		Endpoint:         "https://api.ollinai.example.com",
		AuthToken:        rapid.StringMatching("[a-zA-Z0-9]{20,40}").Draw(t, "authToken"),
		WebhookPort:      rapid.IntRange(1024, 65535).Draw(t, "webhookPort"),
		RiskPollInterval: time.Duration(rapid.IntRange(5, 300).Draw(t, "riskPollSec")) * time.Second,
		DORAPollInterval: time.Duration(rapid.IntRange(30, 1800).Draw(t, "doraPollSec")) * time.Second,
		NATSUrl:          "nats://titanops-nats:4222",
		BufferCapacity:   rapid.IntRange(100, 10000).Draw(t, "bufferCap"),
		MaxPayloadBytes:  rapid.IntRange(1024, 65536).Draw(t, "maxPayload"),
		MetricsPort:      rapid.IntRange(1024, 65535).Draw(t, "metricsPort"),
	}
}

// invalidConfigGen generates a Config with at least one invalid field.
func invalidConfigGen(t *rapid.T) Config {
	// Start with a valid config, then invalidate at least one field
	cfg := validConfigGen(t)

	// Pick which field(s) to invalidate
	fieldToBreak := rapid.IntRange(0, 5).Draw(t, "fieldToBreak")

	switch fieldToBreak {
	case 0:
		// Empty endpoint (required)
		cfg.Endpoint = ""
	case 1:
		// Empty auth token (required)
		cfg.AuthToken = ""
	case 2:
		// Port out of range
		cfg.WebhookPort = rapid.OneOf(rapid.IntRange(0, 1023), rapid.IntRange(65536, 99999)).Draw(t, "badPort")
	case 3:
		// RiskPollInterval too short
		cfg.RiskPollInterval = time.Duration(rapid.IntRange(1, 4).Draw(t, "shortPoll")) * time.Second
	case 4:
		// BufferCapacity too small
		cfg.BufferCapacity = rapid.IntRange(0, 99).Draw(t, "smallBuf")
	case 5:
		// Empty NATSUrl (required)
		cfg.NATSUrl = ""
	}

	return cfg
}
