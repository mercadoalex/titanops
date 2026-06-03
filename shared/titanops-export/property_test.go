package export

import (
	"context"
	"errors"
	"fmt"
	"log"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"pgregory.net/rapid"
)

// --- Test helpers / generators ---

var testSeverities = []string{"critical", "high", "medium", "low", "informational"}
var testModules = []string{"tlapix", "earthworm", "ebeecontrol", "quack", "correlation"}

func genValidEvent() *rapid.Generator[Event] {
	return rapid.Custom[Event](func(t *rapid.T) Event {
		return Event{
			Namespace: rapid.SampledFrom([]string{"default", "kube-system", "titanops", "monitoring"}).Draw(t, "namespace"),
			Timestamp: time.Date(2024, 6, 15, 10, 30, 0, 0, time.UTC).Add(
				time.Duration(rapid.Int64Range(0, 365*24*60*60*1000).Draw(t, "tsOffset")) * time.Millisecond,
			),
			Severity:  rapid.SampledFrom(testSeverities).Draw(t, "severity"),
			Module:    rapid.SampledFrom(testModules).Draw(t, "module"),
			EventType: rapid.SampledFrom([]string{"node_anomaly_detected", "cert_expiry_warning", "honeytoken_accessed", "scheduling_decision"}).Draw(t, "eventType"),
			Payload:   []byte(rapid.StringMatching(`[a-zA-Z0-9]{1,100}`).Draw(t, "payload")),
			Node:      rapid.SampledFrom([]string{"node-01", "node-02", "worker-a"}).Draw(t, "node"),
			Pod:       rapid.SampledFrom([]string{"app-pod-1", "web-frontend", "api-backend"}).Draw(t, "pod"),
			EventID:   fmt.Sprintf("%08x-%04x-%04x-%04x-%012x", rapid.IntRange(0, 0xFFFFFFFF).Draw(t, "u1"), rapid.IntRange(0, 0xFFFF).Draw(t, "u2"), rapid.IntRange(0, 0xFFFF).Draw(t, "u3"), rapid.IntRange(0, 0xFFFF).Draw(t, "u4"), rapid.Int64Range(0, 0xFFFFFFFFFFFF).Draw(t, "u5")),
			Labels:    map[string]string{"app": "titanops", "env": "test"},
		}
	})
}

func genSeveritySet() *rapid.Generator[[]string] {
	return rapid.Custom[[]string](func(t *rapid.T) []string {
		n := rapid.IntRange(1, len(testSeverities)).Draw(t, "setSize")
		// Pick n unique severities
		perm := rapid.Permutation(testSeverities)
		drawn := perm.Draw(t, "severityPerm")
		return drawn[:n]
	})
}

// pbtMockBackend is a test backend that tracks calls and optionally fails.
type pbtMockBackend struct {
	name      string
	enabled   bool
	failErr   error
	sendCount atomic.Int64
}

func (m *pbtMockBackend) Name() string      { return m.name }
func (m *pbtMockBackend) IsEnabled() bool    { return m.enabled }
func (m *pbtMockBackend) Send(_ context.Context, _ Event) error {
	m.sendCount.Add(1)
	return m.failErr
}

// Feature: titanops-platform-integration, Property 3: Export adapter produces correctly formatted output per backend
// **Validates: Requirements 4.1, 4.3**
func TestProperty3_ExportAdapterProducesCorrectlyFormattedOutput(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		event := genValidEvent().Draw(t, "event")
		backendType := rapid.SampledFrom([]string{"prometheus", "otlp", "splunk", "dynatrace", "webhook"}).Draw(t, "backend")

		switch backendType {
		case "prometheus":
			output, err := FormatPrometheus(event)
			if err != nil {
				t.Fatalf("FormatPrometheus failed: %v", err)
			}
			if output == "" {
				t.Fatal("FormatPrometheus returned empty output")
			}
			// Prometheus exposition format must have HELP and TYPE lines
			if !strings.Contains(output, "# HELP") {
				t.Fatal("Prometheus output missing # HELP line")
			}
			if !strings.Contains(output, "# TYPE") {
				t.Fatal("Prometheus output missing # TYPE line")
			}
			// Must contain metric name with titanops prefix
			if !strings.Contains(output, "titanops_") {
				t.Fatal("Prometheus output missing titanops_ metric prefix")
			}

		case "otlp":
			output, err := FormatOTLP(event)
			if err != nil {
				t.Fatalf("FormatOTLP failed: %v", err)
			}
			if len(output) == 0 {
				t.Fatal("FormatOTLP returned empty output")
			}
			// Must be valid JSON (contains braces)
			outStr := string(output)
			if !strings.HasPrefix(outStr, "{") || !strings.HasSuffix(outStr, "}") {
				t.Fatal("FormatOTLP output is not valid JSON")
			}
			// Must contain expected fields
			if !strings.Contains(outStr, "timeUnixNano") {
				t.Fatal("OTLP output missing timeUnixNano field")
			}
			if !strings.Contains(outStr, "severityText") {
				t.Fatal("OTLP output missing severityText field")
			}

		case "splunk":
			output, err := FormatSplunkHEC(event)
			if err != nil {
				t.Fatalf("FormatSplunkHEC failed: %v", err)
			}
			if len(output) == 0 {
				t.Fatal("FormatSplunkHEC returned empty output")
			}
			outStr := string(output)
			if !strings.HasPrefix(outStr, "{") || !strings.HasSuffix(outStr, "}") {
				t.Fatal("FormatSplunkHEC output is not valid JSON")
			}
			// Must contain Splunk-specific fields
			if !strings.Contains(outStr, "source") {
				t.Fatal("Splunk output missing source field")
			}
			if !strings.Contains(outStr, "sourcetype") {
				t.Fatal("Splunk output missing sourcetype field")
			}
			if !strings.Contains(outStr, "event") {
				t.Fatal("Splunk output missing event field")
			}

		case "dynatrace":
			output, err := FormatDynatrace(event)
			if err != nil {
				t.Fatalf("FormatDynatrace failed: %v", err)
			}
			if len(output) == 0 {
				t.Fatal("FormatDynatrace returned empty output")
			}
			outStr := string(output)
			if !strings.HasPrefix(outStr, "{") || !strings.HasSuffix(outStr, "}") {
				t.Fatal("FormatDynatrace output is not valid JSON")
			}
			// Must contain Dynatrace-specific fields
			if !strings.Contains(outStr, "eventType") {
				t.Fatal("Dynatrace output missing eventType field")
			}
			if !strings.Contains(outStr, "title") {
				t.Fatal("Dynatrace output missing title field")
			}
			if !strings.Contains(outStr, "properties") {
				t.Fatal("Dynatrace output missing properties field")
			}

		case "webhook":
			output, err := FormatWebhook(event)
			if err != nil {
				t.Fatalf("FormatWebhook failed: %v", err)
			}
			if len(output) == 0 {
				t.Fatal("FormatWebhook returned empty output")
			}
			outStr := string(output)
			if !strings.HasPrefix(outStr, "{") || !strings.HasSuffix(outStr, "}") {
				t.Fatal("FormatWebhook output is not valid JSON")
			}
			// Must contain common event fields
			if !strings.Contains(outStr, "event_id") {
				t.Fatal("Webhook output missing event_id field")
			}
			if !strings.Contains(outStr, "timestamp") {
				t.Fatal("Webhook output missing timestamp field")
			}
			if !strings.Contains(outStr, "severity") {
				t.Fatal("Webhook output missing severity field")
			}
		}
	})
}

// Feature: titanops-platform-integration, Property 4: Concurrent export isolation
// **Validates: Requirements 4.2**
func TestProperty4_ConcurrentExportIsolation(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		// Generate 2-5 backends with random failure patterns
		numBackends := rapid.IntRange(2, 5).Draw(t, "numBackends")
		backends := make([]Backend, numBackends)
		failPattern := make([]bool, numBackends)

		for i := 0; i < numBackends; i++ {
			shouldFail := rapid.Bool().Draw(t, fmt.Sprintf("fail[%d]", i))
			failPattern[i] = shouldFail

			var failErr error
			if shouldFail {
				failErr = errors.New("simulated backend failure")
			}
			backends[i] = &pbtMockBackend{
				name:    fmt.Sprintf("backend-%d", i),
				enabled: true,
				failErr: failErr,
			}
		}

		exporter := NewMultiExporter(backends...)
		event := genValidEvent().Draw(t, "event")

		ctx := context.Background()
		results := exporter.Export(ctx, event)

		// Must get one result per enabled backend
		if len(results) != numBackends {
			t.Fatalf("expected %d results, got %d", numBackends, len(results))
		}

		// Verify each backend's result matches its failure pattern
		for i, result := range results {
			expectedName := fmt.Sprintf("backend-%d", i)
			if result.Backend != expectedName {
				t.Fatalf("result[%d] backend name mismatch: expected %q, got %q", i, expectedName, result.Backend)
			}

			if failPattern[i] {
				// This backend should have failed
				if result.Success {
					t.Fatalf("backend %q should have failed but succeeded", expectedName)
				}
				if result.Error == nil {
					t.Fatalf("backend %q failed but has nil error", expectedName)
				}
			} else {
				// This backend should have succeeded
				if !result.Success {
					t.Fatalf("backend %q should have succeeded but failed: %v", expectedName, result.Error)
				}
			}
		}

		// Verify ALL non-failing backends were actually called
		for i, b := range backends {
			mb := b.(*pbtMockBackend)
			if !failPattern[i] && mb.sendCount.Load() == 0 {
				t.Fatalf("non-failing backend %q was never called", mb.name)
			}
		}
	})
}

// Feature: titanops-platform-integration, Property 5: Export buffer growth, eviction, and retry
// **Validates: Requirements 4.4, 4.5**
func TestProperty5_ExportBufferGrowthEvictionAndRetry(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		// Generate buffer capacity and event count
		capacity := rapid.IntRange(5, 50).Draw(t, "capacity") // smaller for fast tests
		numEvents := rapid.IntRange(0, capacity*3).Draw(t, "numEvents")

		rb := NewRingBuffer("test-backend", &RingBufferConfig{
			Capacity:       capacity,
			InitialBackoff: 1 * time.Second,
			MaxBackoff:     60 * time.Second,
			MaxRetries:     10,
			Logger:         log.New(log.Writer(), "[test] ", 0),
		})

		// Push events into the buffer
		for i := 0; i < numEvents; i++ {
			event := Event{
				Namespace: "test-ns",
				Timestamp: time.Now().UTC(),
				Severity:  "medium",
				Module:    "tlapix",
				EventType: "test_event",
				Payload:   []byte(fmt.Sprintf("event-%d", i)),
				EventID:   fmt.Sprintf("evt-%d", i),
			}
			rb.Push(event)

			// PROPERTY: buffer never exceeds capacity
			currentLen := rb.Len()
			if currentLen > capacity {
				t.Fatalf("buffer exceeded capacity: len=%d, capacity=%d after %d pushes", currentLen, capacity, i+1)
			}
		}

		// PROPERTY: final buffer size is min(numEvents, capacity)
		expectedLen := numEvents
		if expectedLen > capacity {
			expectedLen = capacity
		}
		actualLen := rb.Len()
		if actualLen != expectedLen {
			t.Fatalf("expected buffer len %d, got %d (numEvents=%d, capacity=%d)", expectedLen, actualLen, numEvents, capacity)
		}

		// PROPERTY: if we pushed more than capacity, oldest events were evicted
		if numEvents > capacity {
			expectedDropped := numEvents - capacity
			actualDropped := rb.Dropped()
			if actualDropped != expectedDropped {
				t.Fatalf("expected %d dropped events, got %d (numEvents=%d, capacity=%d)", expectedDropped, actualDropped, numEvents, capacity)
			}
		} else {
			if rb.Dropped() != 0 {
				t.Fatalf("no events should be dropped when numEvents(%d) <= capacity(%d), got %d dropped", numEvents, capacity, rb.Dropped())
			}
		}

		// PROPERTY: capacity is always what we configured
		if rb.Capacity() != capacity {
			t.Fatalf("expected capacity %d, got %d", capacity, rb.Capacity())
		}
	})
}

// Feature: titanops-platform-integration, Property 6: Webhook severity filtering
// **Validates: Requirements 4.6**
func TestProperty6_WebhookSeverityFiltering(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		// Generate a random severity filter set (1-5 severities)
		filterSet := genSeveritySet().Draw(t, "filterSet")

		// Generate a random event
		event := genValidEvent().Draw(t, "event")

		// Create webhook backend with the filter set
		config := WebhookConfig{
			Endpoint:   "https://webhook.example.com/test",
			Events:     filterSet,
			TimeoutSec: 10,
			MaxRetries: 3,
		}
		webhook := NewWebhookBackend(config, log.New(log.Writer(), "[test] ", 0))

		// Determine if the event's severity is in the filter set
		severityInFilter := false
		for _, s := range filterSet {
			if strings.EqualFold(s, event.Severity) {
				severityInFilter = true
				break
			}
		}

		// Call the matchesSeverity method to check filtering logic
		matches := webhook.matchesSeverity(event.Severity)

		if severityInFilter && !matches {
			t.Fatalf("severity %q is in filter set %v but matchesSeverity returned false", event.Severity, filterSet)
		}
		if !severityInFilter && matches {
			t.Fatalf("severity %q is NOT in filter set %v but matchesSeverity returned true", event.Severity, filterSet)
		}

		// Also verify via Send: if severity doesn't match, Send should succeed (filtered out = no error)
		// If severity matches, Send should also succeed (webhook format succeeds, no real HTTP)
		ctx := context.Background()
		err := webhook.Send(ctx, event)
		if err != nil {
			t.Fatalf("webhook Send returned unexpected error: %v", err)
		}
	})
}
