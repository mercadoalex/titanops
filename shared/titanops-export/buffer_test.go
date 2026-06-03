package export

import (
	"context"
	"errors"
	"fmt"
	"log"
	"os"
	"sync"
	"testing"
	"time"
)

func bufferTestEvent(id string) Event {
	return Event{
		Namespace: "default",
		Timestamp: time.Now(),
		Severity:  "high",
		Module:    "earthworm",
		EventType: "anomaly_detected",
		Payload:   []byte(`{"score": 0.95}`),
		EventID:   id,
	}
}

func TestRingBuffer_PushAndLen(t *testing.T) {
	rb := NewRingBuffer("test", nil)

	if rb.Len() != 0 {
		t.Errorf("expected empty buffer, got len %d", rb.Len())
	}

	rb.Push(bufferTestEvent("evt-1"))
	if rb.Len() != 1 {
		t.Errorf("expected len 1, got %d", rb.Len())
	}

	rb.Push(bufferTestEvent("evt-2"))
	if rb.Len() != 2 {
		t.Errorf("expected len 2, got %d", rb.Len())
	}
}

func TestRingBuffer_Capacity(t *testing.T) {
	rb := NewRingBuffer("test", &RingBufferConfig{Capacity: 5})

	if rb.Capacity() != 5 {
		t.Errorf("expected capacity 5, got %d", rb.Capacity())
	}
}

func TestRingBuffer_DefaultCapacity(t *testing.T) {
	rb := NewRingBuffer("test", nil)

	if rb.Capacity() != 1000 {
		t.Errorf("expected default capacity 1000, got %d", rb.Capacity())
	}
}

func TestRingBuffer_EvictionWhenFull(t *testing.T) {
	logger := log.New(os.Stderr, "", 0)
	rb := NewRingBuffer("test", &RingBufferConfig{
		Capacity: 3,
		Logger:   logger,
	})

	rb.Push(bufferTestEvent("evt-1"))
	rb.Push(bufferTestEvent("evt-2"))
	rb.Push(bufferTestEvent("evt-3"))

	if rb.Len() != 3 {
		t.Fatalf("expected len 3, got %d", rb.Len())
	}
	if rb.Dropped() != 0 {
		t.Fatalf("expected 0 dropped, got %d", rb.Dropped())
	}

	// Push a 4th event - should evict oldest
	rb.Push(bufferTestEvent("evt-4"))

	if rb.Len() != 3 {
		t.Errorf("expected len 3 after eviction, got %d", rb.Len())
	}
	if rb.Dropped() != 1 {
		t.Errorf("expected 1 dropped, got %d", rb.Dropped())
	}

	// The oldest event (evt-1) should have been evicted
	event := rb.Peek()
	if event == nil {
		t.Fatal("expected non-nil peek")
	}
	if event.EventID != "evt-2" {
		t.Errorf("expected evt-2 (oldest remaining), got %q", event.EventID)
	}
}

func TestRingBuffer_MultipleEvictions(t *testing.T) {
	rb := NewRingBuffer("test", &RingBufferConfig{Capacity: 2})

	rb.Push(bufferTestEvent("evt-1"))
	rb.Push(bufferTestEvent("evt-2"))
	rb.Push(bufferTestEvent("evt-3"))
	rb.Push(bufferTestEvent("evt-4"))
	rb.Push(bufferTestEvent("evt-5"))

	if rb.Len() != 2 {
		t.Errorf("expected len 2, got %d", rb.Len())
	}
	if rb.Dropped() != 3 {
		t.Errorf("expected 3 dropped, got %d", rb.Dropped())
	}

	// Only the newest 2 should remain
	events := rb.Drain()
	if len(events) != 2 {
		t.Fatalf("expected 2 events after drain, got %d", len(events))
	}
	if events[0].EventID != "evt-4" {
		t.Errorf("expected evt-4, got %q", events[0].EventID)
	}
	if events[1].EventID != "evt-5" {
		t.Errorf("expected evt-5, got %q", events[1].EventID)
	}
}

func TestRingBuffer_Peek(t *testing.T) {
	rb := NewRingBuffer("test", nil)

	// Empty buffer
	if rb.Peek() != nil {
		t.Error("expected nil peek on empty buffer")
	}

	rb.Push(bufferTestEvent("evt-1"))
	event := rb.Peek()
	if event == nil {
		t.Fatal("expected non-nil peek")
	}
	if event.EventID != "evt-1" {
		t.Errorf("expected evt-1, got %q", event.EventID)
	}

	// Peek does not remove
	if rb.Len() != 1 {
		t.Error("peek should not remove event")
	}
}

func TestRingBuffer_MarkSuccess(t *testing.T) {
	rb := NewRingBuffer("test", nil)

	rb.Push(bufferTestEvent("evt-1"))
	rb.Push(bufferTestEvent("evt-2"))

	rb.MarkSuccess()

	if rb.Len() != 1 {
		t.Errorf("expected len 1 after MarkSuccess, got %d", rb.Len())
	}

	event := rb.Peek()
	if event == nil || event.EventID != "evt-2" {
		t.Error("expected evt-2 remaining after removing evt-1")
	}
}

func TestRingBuffer_MarkFailure_ExponentialBackoff(t *testing.T) {
	rb := NewRingBuffer("test", &RingBufferConfig{
		InitialBackoff: 1 * time.Second,
		MaxBackoff:     60 * time.Second,
		MaxRetries:     10,
	})

	rb.Push(bufferTestEvent("evt-1"))

	// First failure - backoff should be 1s
	err := rb.MarkFailure()
	if err != nil {
		t.Fatalf("unexpected error on first failure: %v", err)
	}

	// Event should still be in buffer but not ready
	if rb.Len() != 1 {
		t.Errorf("expected len 1, got %d", rb.Len())
	}

	// Peek should return nil because event is in backoff
	event := rb.Peek()
	if event != nil {
		t.Error("expected nil peek during backoff period")
	}
}

func TestRingBuffer_MarkFailure_MaxRetries(t *testing.T) {
	logger := log.New(os.Stderr, "", 0)
	rb := NewRingBuffer("test", &RingBufferConfig{
		Capacity:       10,
		InitialBackoff: 1 * time.Millisecond, // Fast for testing
		MaxBackoff:     10 * time.Millisecond,
		MaxRetries:     3,
		Logger:         logger,
	})

	rb.Push(bufferTestEvent("evt-1"))

	// Simulate 3 failures to exhaust retries
	// Need to wait between failures for backoff to expire
	for i := 0; i < 2; i++ {
		rb.MarkFailure()
		// Reset the nextTry to make the event immediately available
		rb.mu.Lock()
		if len(rb.items) > 0 {
			rb.items[0].nextTry = time.Now().Add(-1 * time.Second)
		}
		rb.mu.Unlock()
	}

	// This should be the final attempt that triggers discard
	err := rb.MarkFailure()
	if err == nil {
		t.Fatal("expected RetryExhaustedError after max retries")
	}

	var retryErr *RetryExhaustedError
	if !errors.As(err, &retryErr) {
		t.Fatalf("expected RetryExhaustedError, got %T: %v", err, err)
	}
	if retryErr.BackendName != "test" {
		t.Errorf("expected backend 'test', got %q", retryErr.BackendName)
	}
	if retryErr.EventID != "evt-1" {
		t.Errorf("expected event 'evt-1', got %q", retryErr.EventID)
	}
	if retryErr.Attempts != 3 {
		t.Errorf("expected 3 attempts, got %d", retryErr.Attempts)
	}

	// Buffer should be empty now
	if rb.Len() != 0 {
		t.Errorf("expected empty buffer after discard, got %d", rb.Len())
	}
	// Dropped count should reflect the discard
	if rb.Dropped() != 1 {
		t.Errorf("expected 1 dropped, got %d", rb.Dropped())
	}
}

func TestRingBuffer_BackoffCalculation(t *testing.T) {
	rb := NewRingBuffer("test", &RingBufferConfig{
		InitialBackoff: 1 * time.Second,
		MaxBackoff:     60 * time.Second,
		MaxRetries:     10,
	})

	tests := []struct {
		attempt  int
		expected time.Duration
	}{
		{1, 1 * time.Second},   // 1 * 2^0 = 1s
		{2, 2 * time.Second},   // 1 * 2^1 = 2s
		{3, 4 * time.Second},   // 1 * 2^2 = 4s
		{4, 8 * time.Second},   // 1 * 2^3 = 8s
		{5, 16 * time.Second},  // 1 * 2^4 = 16s
		{6, 32 * time.Second},  // 1 * 2^5 = 32s
		{7, 60 * time.Second},  // 1 * 2^6 = 64s capped to 60s
		{8, 60 * time.Second},  // capped
		{9, 60 * time.Second},  // capped
		{10, 60 * time.Second}, // capped
	}

	for _, tt := range tests {
		t.Run(fmt.Sprintf("attempt_%d", tt.attempt), func(t *testing.T) {
			backoff := rb.calculateBackoff(tt.attempt)
			if backoff != tt.expected {
				t.Errorf("attempt %d: expected %v, got %v", tt.attempt, tt.expected, backoff)
			}
		})
	}
}

func TestRingBuffer_Info(t *testing.T) {
	rb := NewRingBuffer("test", &RingBufferConfig{Capacity: 5})

	rb.Push(bufferTestEvent("evt-1"))
	rb.Push(bufferTestEvent("evt-2"))

	info := rb.Info()
	if info.Capacity != 5 {
		t.Errorf("expected capacity 5, got %d", info.Capacity)
	}
	if info.Used != 2 {
		t.Errorf("expected used 2, got %d", info.Used)
	}
	if info.Dropped != 0 {
		t.Errorf("expected dropped 0, got %d", info.Dropped)
	}
}

func TestRingBuffer_Drain(t *testing.T) {
	rb := NewRingBuffer("test", nil)

	rb.Push(bufferTestEvent("evt-1"))
	rb.Push(bufferTestEvent("evt-2"))
	rb.Push(bufferTestEvent("evt-3"))

	events := rb.Drain()
	if len(events) != 3 {
		t.Fatalf("expected 3 events, got %d", len(events))
	}
	if rb.Len() != 0 {
		t.Errorf("expected empty buffer after drain, got %d", rb.Len())
	}
}

func TestRingBuffer_ThreadSafety(t *testing.T) {
	rb := NewRingBuffer("test", &RingBufferConfig{Capacity: 100})

	var wg sync.WaitGroup
	numGoroutines := 20
	eventsPerGoroutine := 50

	// Concurrently push events
	wg.Add(numGoroutines)
	for i := 0; i < numGoroutines; i++ {
		go func(id int) {
			defer wg.Done()
			for j := 0; j < eventsPerGoroutine; j++ {
				rb.Push(bufferTestEvent(fmt.Sprintf("g%d-evt-%d", id, j)))
			}
		}(i)
	}

	// Concurrently read buffer state
	wg.Add(5)
	for i := 0; i < 5; i++ {
		go func() {
			defer wg.Done()
			for j := 0; j < 100; j++ {
				_ = rb.Len()
				_ = rb.Dropped()
				_ = rb.Info()
				rb.Peek()
			}
		}()
	}

	wg.Wait()

	// After all pushes, buffer should be at capacity (100)
	// since 20*50 = 1000 events pushed into cap 100
	if rb.Len() != 100 {
		t.Errorf("expected buffer at capacity (100), got %d", rb.Len())
	}

	// Dropped should be 1000 - 100 = 900
	totalPushed := numGoroutines * eventsPerGoroutine
	expectedDropped := totalPushed - 100
	if rb.Dropped() != expectedDropped {
		t.Errorf("expected %d dropped, got %d", expectedDropped, rb.Dropped())
	}
}

func TestRingBuffer_MarkSuccess_EmptyBuffer(t *testing.T) {
	rb := NewRingBuffer("test", nil)

	// Should not panic on empty buffer
	rb.MarkSuccess()
	if rb.Len() != 0 {
		t.Error("expected buffer to remain empty")
	}
}

func TestRingBuffer_MarkFailure_EmptyBuffer(t *testing.T) {
	rb := NewRingBuffer("test", nil)

	// Should not panic on empty buffer
	err := rb.MarkFailure()
	if err != nil {
		t.Errorf("expected nil error on empty buffer, got %v", err)
	}
}

func TestProcessBuffer_Success(t *testing.T) {
	rb := NewRingBuffer("test", nil)
	rb.Push(bufferTestEvent("evt-1"))
	rb.Push(bufferTestEvent("evt-2"))

	backend := &mockBackend{
		name:    "test",
		enabled: true,
		sendFn: func(ctx context.Context, event Event) error {
			return nil
		},
	}

	err := ProcessBuffer(context.Background(), rb, backend)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if rb.Len() != 0 {
		t.Errorf("expected empty buffer after successful processing, got %d", rb.Len())
	}
}

func TestProcessBuffer_Failure(t *testing.T) {
	rb := NewRingBuffer("test", &RingBufferConfig{
		MaxRetries:     10,
		InitialBackoff: 1 * time.Second,
	})
	rb.Push(bufferTestEvent("evt-1"))

	backend := &mockBackend{
		name:    "test",
		enabled: true,
		sendFn: func(ctx context.Context, event Event) error {
			return errors.New("connection refused")
		},
	}

	err := ProcessBuffer(context.Background(), rb, backend)
	if err != nil {
		t.Fatalf("unexpected error (should return nil for retryable failure): %v", err)
	}

	// Event should still be in buffer with backoff applied
	if rb.Len() != 1 {
		t.Errorf("expected 1 event in buffer (retrying), got %d", rb.Len())
	}
}

func TestProcessBuffer_ContextCancellation(t *testing.T) {
	rb := NewRingBuffer("test", nil)
	rb.Push(bufferTestEvent("evt-1"))

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	backend := &mockBackend{
		name:    "test",
		enabled: true,
		sendFn: func(ctx context.Context, event Event) error {
			return nil
		},
	}

	err := ProcessBuffer(ctx, rb, backend)
	if err != context.Canceled {
		t.Errorf("expected context.Canceled error, got %v", err)
	}
}

func TestBufferFullError(t *testing.T) {
	err := &BufferFullError{BackendName: "splunk", DiscardCount: 5}
	expected := `backend "splunk" buffer full: discarded 5 oldest events`
	if err.Error() != expected {
		t.Errorf("expected %q, got %q", expected, err.Error())
	}
}

func TestRetryExhaustedError(t *testing.T) {
	err := &RetryExhaustedError{BackendName: "otlp", EventID: "evt-123", Attempts: 10}
	expected := `backend "otlp": event "evt-123" discarded after 10 failed retries`
	if err.Error() != expected {
		t.Errorf("expected %q, got %q", expected, err.Error())
	}
}
