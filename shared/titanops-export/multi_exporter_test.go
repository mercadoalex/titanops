package export

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"
)

// mockBackend is a test double for the Backend interface.
type mockBackend struct {
	name    string
	enabled bool
	sendFn  func(ctx context.Context, event Event) error
}

func (m *mockBackend) Name() string { return m.name }
func (m *mockBackend) IsEnabled() bool { return m.enabled }
func (m *mockBackend) Send(ctx context.Context, event Event) error {
	if m.sendFn != nil {
		return m.sendFn(ctx, event)
	}
	return nil
}

// panicBackend is a backend that panics on Send.
type panicBackend struct {
	name    string
	enabled bool
	msg     string
}

func (p *panicBackend) Name() string    { return p.name }
func (p *panicBackend) IsEnabled() bool  { return p.enabled }
func (p *panicBackend) Send(ctx context.Context, event Event) error {
	panic(p.msg)
}

func testEvent() Event {
	return Event{
		Namespace: "default",
		Timestamp: time.Now(),
		Severity:  "high",
		Module:    "earthworm",
		EventType: "anomaly_detected",
		Payload:   []byte(`{"score": 0.95}`),
		EventID:   "test-event-001",
	}
}

func TestMultiExporter_SingleBackendSuccess(t *testing.T) {
	var called int32
	backend := &mockBackend{
		name:    "prometheus",
		enabled: true,
		sendFn: func(ctx context.Context, event Event) error {
			atomic.AddInt32(&called, 1)
			return nil
		},
	}

	exporter := NewMultiExporter(backend)
	results := exporter.Export(context.Background(), testEvent())

	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if !results[0].Success {
		t.Errorf("expected success, got error: %v", results[0].Error)
	}
	if results[0].Backend != "prometheus" {
		t.Errorf("expected backend 'prometheus', got %q", results[0].Backend)
	}
	if atomic.LoadInt32(&called) != 1 {
		t.Errorf("expected Send called once, got %d", called)
	}
}

func TestMultiExporter_MultipleBackendSuccess(t *testing.T) {
	var callCount int32
	backends := []Backend{
		&mockBackend{
			name:    "prometheus",
			enabled: true,
			sendFn: func(ctx context.Context, event Event) error {
				atomic.AddInt32(&callCount, 1)
				return nil
			},
		},
		&mockBackend{
			name:    "splunk",
			enabled: true,
			sendFn: func(ctx context.Context, event Event) error {
				atomic.AddInt32(&callCount, 1)
				return nil
			},
		},
		&mockBackend{
			name:    "otlp",
			enabled: true,
			sendFn: func(ctx context.Context, event Event) error {
				atomic.AddInt32(&callCount, 1)
				return nil
			},
		},
	}

	exporter := NewMultiExporter(backends...)
	results := exporter.Export(context.Background(), testEvent())

	if len(results) != 3 {
		t.Fatalf("expected 3 results, got %d", len(results))
	}
	for _, r := range results {
		if !r.Success {
			t.Errorf("backend %q failed: %v", r.Backend, r.Error)
		}
	}
	if atomic.LoadInt32(&callCount) != 3 {
		t.Errorf("expected 3 calls, got %d", callCount)
	}
}

func TestMultiExporter_OneBackendFails_OthersSucceed(t *testing.T) {
	errSplunk := errors.New("connection refused")

	backends := []Backend{
		&mockBackend{
			name:    "prometheus",
			enabled: true,
			sendFn: func(ctx context.Context, event Event) error {
				return nil
			},
		},
		&mockBackend{
			name:    "splunk",
			enabled: true,
			sendFn: func(ctx context.Context, event Event) error {
				return errSplunk
			},
		},
		&mockBackend{
			name:    "otlp",
			enabled: true,
			sendFn: func(ctx context.Context, event Event) error {
				return nil
			},
		},
	}

	exporter := NewMultiExporter(backends...)
	results := exporter.Export(context.Background(), testEvent())

	if len(results) != 3 {
		t.Fatalf("expected 3 results, got %d", len(results))
	}

	// Collect results by backend name for deterministic assertions
	resultMap := make(map[string]ExportResult)
	for _, r := range results {
		resultMap[r.Backend] = r
	}

	// Prometheus and OTLP should succeed
	if !resultMap["prometheus"].Success {
		t.Error("prometheus should have succeeded")
	}
	if !resultMap["otlp"].Success {
		t.Error("otlp should have succeeded")
	}

	// Splunk should fail
	if resultMap["splunk"].Success {
		t.Error("splunk should have failed")
	}
	if resultMap["splunk"].Error == nil {
		t.Error("splunk error should not be nil")
	}

	// Verify the error is a BackendSendError wrapping the original
	var sendErr *BackendSendError
	if !errors.As(resultMap["splunk"].Error, &sendErr) {
		t.Errorf("expected BackendSendError, got %T", resultMap["splunk"].Error)
	} else {
		if sendErr.BackendName != "splunk" {
			t.Errorf("expected backend name 'splunk', got %q", sendErr.BackendName)
		}
		if !errors.Is(sendErr.Cause, errSplunk) {
			t.Errorf("expected cause to be errSplunk, got %v", sendErr.Cause)
		}
	}
}

func TestMultiExporter_AllBackendsFail(t *testing.T) {
	backends := []Backend{
		&mockBackend{
			name:    "prometheus",
			enabled: true,
			sendFn: func(ctx context.Context, event Event) error {
				return errors.New("prometheus down")
			},
		},
		&mockBackend{
			name:    "splunk",
			enabled: true,
			sendFn: func(ctx context.Context, event Event) error {
				return errors.New("splunk timeout")
			},
		},
	}

	exporter := NewMultiExporter(backends...)
	results := exporter.Export(context.Background(), testEvent())

	if len(results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(results))
	}
	for _, r := range results {
		if r.Success {
			t.Errorf("backend %q should have failed", r.Backend)
		}
		if r.Error == nil {
			t.Errorf("backend %q error should not be nil", r.Backend)
		}
	}
}

func TestMultiExporter_NoBackends(t *testing.T) {
	exporter := NewMultiExporter()
	results := exporter.Export(context.Background(), testEvent())

	if results != nil {
		t.Errorf("expected nil results for no backends, got %v", results)
	}
}

func TestMultiExporter_DisabledBackendsSkipped(t *testing.T) {
	var enabledCalled int32
	backends := []Backend{
		&mockBackend{
			name:    "prometheus",
			enabled: false, // disabled
			sendFn: func(ctx context.Context, event Event) error {
				t.Error("disabled backend should not be called")
				return nil
			},
		},
		&mockBackend{
			name:    "splunk",
			enabled: true,
			sendFn: func(ctx context.Context, event Event) error {
				atomic.AddInt32(&enabledCalled, 1)
				return nil
			},
		},
	}

	exporter := NewMultiExporter(backends...)
	results := exporter.Export(context.Background(), testEvent())

	if len(results) != 1 {
		t.Fatalf("expected 1 result (only enabled), got %d", len(results))
	}
	if results[0].Backend != "splunk" {
		t.Errorf("expected result for 'splunk', got %q", results[0].Backend)
	}
	if atomic.LoadInt32(&enabledCalled) != 1 {
		t.Error("expected enabled backend to be called once")
	}
}

func TestMultiExporter_BackendPanicRecovery(t *testing.T) {
	backends := []Backend{
		&panicBackend{
			name:    "panicky",
			enabled: true,
			msg:     "unexpected nil pointer",
		},
		&mockBackend{
			name:    "stable",
			enabled: true,
			sendFn: func(ctx context.Context, event Event) error {
				return nil
			},
		},
	}

	exporter := NewMultiExporter(backends...)
	results := exporter.Export(context.Background(), testEvent())

	if len(results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(results))
	}

	resultMap := make(map[string]ExportResult)
	for _, r := range results {
		resultMap[r.Backend] = r
	}

	// Panicking backend should fail gracefully
	if resultMap["panicky"].Success {
		t.Error("panicking backend should not report success")
	}
	var panicErr *BackendPanicError
	if !errors.As(resultMap["panicky"].Error, &panicErr) {
		t.Errorf("expected BackendPanicError, got %T: %v", resultMap["panicky"].Error, resultMap["panicky"].Error)
	} else {
		if panicErr.BackendName != "panicky" {
			t.Errorf("expected backend name 'panicky', got %q", panicErr.BackendName)
		}
	}

	// Stable backend should succeed despite the other one panicking
	if !resultMap["stable"].Success {
		t.Error("stable backend should have succeeded")
	}
}

func TestMultiExporter_ConcurrencyIsolation(t *testing.T) {
	// Verify that a slow backend does not delay a fast one.
	var fastDone int64

	backends := []Backend{
		&mockBackend{
			name:    "slow",
			enabled: true,
			sendFn: func(ctx context.Context, event Event) error {
				time.Sleep(200 * time.Millisecond)
				return nil
			},
		},
		&mockBackend{
			name:    "fast",
			enabled: true,
			sendFn: func(ctx context.Context, event Event) error {
				atomic.StoreInt64(&fastDone, time.Now().UnixNano())
				return nil
			},
		},
	}

	exporter := NewMultiExporter(backends...)
	start := time.Now()
	results := exporter.Export(context.Background(), testEvent())
	totalDuration := time.Since(start)

	if len(results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(results))
	}

	// Both should succeed
	for _, r := range results {
		if !r.Success {
			t.Errorf("backend %q should have succeeded", r.Backend)
		}
	}

	// Total duration should be close to 200ms (the slow backend),
	// not 200ms + fast (which would indicate sequential execution)
	if totalDuration > 400*time.Millisecond {
		t.Errorf("expected concurrent execution (~200ms), took %v", totalDuration)
	}

	// The fast backend should have completed well before the slow one finished
	fastTime := time.Unix(0, atomic.LoadInt64(&fastDone))
	fastDuration := fastTime.Sub(start)
	if fastDuration > 100*time.Millisecond {
		t.Errorf("fast backend took too long (%v), may be blocked by slow backend", fastDuration)
	}
}

func TestMultiExporter_BufferStatus(t *testing.T) {
	backends := []Backend{
		&mockBackend{name: "prometheus", enabled: true},
		&mockBackend{name: "splunk", enabled: true},
		&mockBackend{name: "otlp", enabled: false},
	}

	exporter := NewMultiExporter(backends...)
	status := exporter.BufferStatus()

	if len(status) != 3 {
		t.Fatalf("expected 3 buffer entries, got %d", len(status))
	}

	for _, name := range []string{"prometheus", "splunk", "otlp"} {
		info, ok := status[name]
		if !ok {
			t.Errorf("missing buffer status for %q", name)
			continue
		}
		if info.Capacity != 1000 {
			t.Errorf("backend %q: expected capacity 1000, got %d", name, info.Capacity)
		}
		if info.Used != 0 {
			t.Errorf("backend %q: expected used 0, got %d", name, info.Used)
		}
		if info.Dropped != 0 {
			t.Errorf("backend %q: expected dropped 0, got %d", name, info.Dropped)
		}
	}
}

func TestMultiExporter_BufferStatusEmpty(t *testing.T) {
	exporter := NewMultiExporter()
	status := exporter.BufferStatus()

	if len(status) != 0 {
		t.Errorf("expected empty buffer status, got %d entries", len(status))
	}
}

func TestMultiExporter_AllDisabledBackends(t *testing.T) {
	backends := []Backend{
		&mockBackend{name: "prometheus", enabled: false},
		&mockBackend{name: "splunk", enabled: false},
	}

	exporter := NewMultiExporter(backends...)
	results := exporter.Export(context.Background(), testEvent())

	if results != nil {
		t.Errorf("expected nil results when all backends disabled, got %v", results)
	}
}
