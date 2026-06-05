package ollinai

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	export "github.com/mercadoalex/titanops/shared/titanops-export"
)

// mockNATSPublisher is a test double for NATSPublisher.
type mockNATSPublisher struct {
	mu           sync.Mutex
	connected    bool
	published    []publishedMsg
	publishErr   error
	publishCount int
}

type publishedMsg struct {
	Subject string
	Data    []byte
}

func (m *mockNATSPublisher) Publish(subject string, data []byte) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.publishCount++
	if m.publishErr != nil {
		return m.publishErr
	}
	m.published = append(m.published, publishedMsg{Subject: subject, Data: data})
	return nil
}

func (m *mockNATSPublisher) IsConnected() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.connected
}

func (m *mockNATSPublisher) setConnected(v bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.connected = v
}

func (m *mockNATSPublisher) getPublished() []publishedMsg {
	m.mu.Lock()
	defer m.mu.Unlock()
	return append([]publishedMsg(nil), m.published...)
}

func TestEmit_PublishesWhenConnected(t *testing.T) {
	mock := &mockNATSPublisher{connected: true}
	emitter := NewNATSEmitter(NATSEmitterConfig{
		Publisher:      mock,
		BufferCapacity: 10,
	})

	event := export.Event{
		Module:    ModuleName,
		EventType: EventTypeDeploymentRisk,
		Severity:  SeverityCritical,
		Node:      "node-1",
		Pod:       "pod-1",
		Namespace: "default",
	}

	err := emitter.Emit(context.Background(), event)
	if err != nil {
		t.Fatalf("Emit returned error: %v", err)
	}

	msgs := mock.getPublished()
	if len(msgs) != 1 {
		t.Fatalf("expected 1 published message, got %d", len(msgs))
	}

	if msgs[0].Subject != NATSSubject {
		t.Errorf("expected subject %q, got %q", NATSSubject, msgs[0].Subject)
	}

	// Buffer should be empty since publish succeeded
	if emitter.BufferLen() != 0 {
		t.Errorf("expected buffer length 0, got %d", emitter.BufferLen())
	}
}

func TestEmit_BuffersWhenDisconnected(t *testing.T) {
	mock := &mockNATSPublisher{connected: false}
	emitter := NewNATSEmitter(NATSEmitterConfig{
		Publisher:      mock,
		BufferCapacity: 10,
	})

	event := export.Event{
		Module:    ModuleName,
		EventType: EventTypeDORAMetrics,
		Node:      "node-1",
		Pod:       "pod-1",
		Namespace: "default",
	}

	err := emitter.Emit(context.Background(), event)
	if err != nil {
		t.Fatalf("Emit returned error: %v", err)
	}

	msgs := mock.getPublished()
	if len(msgs) != 0 {
		t.Fatalf("expected 0 published messages when disconnected, got %d", len(msgs))
	}

	if emitter.BufferLen() != 1 {
		t.Errorf("expected buffer length 1, got %d", emitter.BufferLen())
	}
}

func TestEmit_BuffersWhenPublishFails(t *testing.T) {
	mock := &mockNATSPublisher{connected: true, publishErr: fmt.Errorf("nats: connection closed")}
	emitter := NewNATSEmitter(NATSEmitterConfig{
		Publisher:      mock,
		BufferCapacity: 10,
	})

	event := export.Event{
		Module:    ModuleName,
		EventType: EventTypeDeploymentRisk,
		Node:      "node-1",
		Pod:       "pod-1",
		Namespace: "default",
	}

	err := emitter.Emit(context.Background(), event)
	if err != nil {
		t.Fatalf("Emit returned error: %v", err)
	}

	// Event should be buffered since publish failed
	if emitter.BufferLen() != 1 {
		t.Errorf("expected buffer length 1, got %d", emitter.BufferLen())
	}
}

func TestEmit_RingBufferEvictsOldestWhenFull(t *testing.T) {
	mock := &mockNATSPublisher{connected: false}
	capacity := 5
	emitter := NewNATSEmitter(NATSEmitterConfig{
		Publisher:      mock,
		BufferCapacity: capacity,
	})

	// Fill buffer beyond capacity
	for i := 0; i < capacity+3; i++ {
		event := export.Event{
			Module:    ModuleName,
			EventType: EventTypeDeploymentRisk,
			Node:      "node-1",
			Pod:       "pod-1",
			Namespace: fmt.Sprintf("ns-%d", i),
		}
		_ = emitter.Emit(context.Background(), event)
	}

	// Buffer should be at capacity
	if emitter.BufferLen() != capacity {
		t.Errorf("expected buffer length %d, got %d", capacity, emitter.BufferLen())
	}

	// The oldest events should have been evicted; remaining should be ns-3, ns-4, ns-5, ns-6, ns-7
	events := emitter.DrainBuffer()
	if len(events) != capacity {
		t.Fatalf("expected %d drained events, got %d", capacity, len(events))
	}

	// First remaining event should be the one at offset 3 (0, 1, 2 were evicted)
	if events[0].Namespace != "ns-3" {
		t.Errorf("expected first buffered event namespace %q, got %q", "ns-3", events[0].Namespace)
	}

	// Total dropped should be 3
	if emitter.Dropped() != 3 {
		t.Errorf("expected 3 dropped events, got %d", emitter.Dropped())
	}
}

func TestEmit_AssignsUUID(t *testing.T) {
	mock := &mockNATSPublisher{connected: false}
	emitter := NewNATSEmitter(NATSEmitterConfig{
		Publisher:      mock,
		BufferCapacity: 10,
	})

	event := export.Event{
		Module:    ModuleName,
		EventType: EventTypeDeploymentRisk,
		Node:      "node-1",
		Pod:       "pod-1",
		Namespace: "default",
	}

	_ = emitter.Emit(context.Background(), event)

	events := emitter.DrainBuffer()
	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events))
	}

	eventID := events[0].EventID
	if eventID == "" {
		t.Fatal("expected non-empty EventID")
	}

	// Validate UUID v4 format (8-4-4-4-12 hex chars)
	parts := strings.Split(eventID, "-")
	if len(parts) != 5 {
		t.Errorf("expected UUID format (5 groups), got %d groups: %s", len(parts), eventID)
	}
}

func TestEmit_PreservesExistingEventID(t *testing.T) {
	mock := &mockNATSPublisher{connected: false}
	emitter := NewNATSEmitter(NATSEmitterConfig{
		Publisher:      mock,
		BufferCapacity: 10,
	})

	existingID := "existing-uuid-value"
	event := export.Event{
		Module:    ModuleName,
		EventType: EventTypeDeploymentRisk,
		EventID:   existingID,
		Node:      "node-1",
		Pod:       "pod-1",
		Namespace: "default",
	}

	_ = emitter.Emit(context.Background(), event)

	events := emitter.DrainBuffer()
	if events[0].EventID != existingID {
		t.Errorf("expected EventID %q preserved, got %q", existingID, events[0].EventID)
	}
}

func TestEmit_AssignsTimestamp(t *testing.T) {
	mock := &mockNATSPublisher{connected: false}
	emitter := NewNATSEmitter(NATSEmitterConfig{
		Publisher:      mock,
		BufferCapacity: 10,
	})

	before := time.Now().UTC()

	event := export.Event{
		Module:    ModuleName,
		EventType: EventTypeDeploymentRisk,
		Node:      "node-1",
		Pod:       "pod-1",
		Namespace: "default",
	}

	_ = emitter.Emit(context.Background(), event)

	after := time.Now().UTC()

	events := emitter.DrainBuffer()
	ts := events[0].Timestamp

	if ts.Before(before) || ts.After(after) {
		t.Errorf("expected timestamp between %v and %v, got %v", before, after, ts)
	}

	if ts.Location() != time.UTC {
		t.Errorf("expected UTC timestamp, got location %v", ts.Location())
	}
}

func TestEmit_PreservesExistingTimestamp(t *testing.T) {
	mock := &mockNATSPublisher{connected: false}
	emitter := NewNATSEmitter(NATSEmitterConfig{
		Publisher:      mock,
		BufferCapacity: 10,
	})

	existingTime := time.Date(2024, 6, 15, 10, 30, 0, 0, time.UTC)
	event := export.Event{
		Module:    ModuleName,
		EventType: EventTypeDeploymentRisk,
		Timestamp: existingTime,
		Node:      "node-1",
		Pod:       "pod-1",
		Namespace: "default",
	}

	_ = emitter.Emit(context.Background(), event)

	events := emitter.DrainBuffer()
	if !events[0].Timestamp.Equal(existingTime) {
		t.Errorf("expected timestamp %v preserved, got %v", existingTime, events[0].Timestamp)
	}
}

func TestEmit_MetadataIncomplete_NodeEmpty(t *testing.T) {
	mock := &mockNATSPublisher{connected: false}
	emitter := NewNATSEmitter(NATSEmitterConfig{
		Publisher:      mock,
		BufferCapacity: 10,
	})

	event := export.Event{
		Module:    ModuleName,
		EventType: EventTypeDeploymentRisk,
		Node:      "", // empty
		Pod:       "pod-1",
		Namespace: "default",
	}

	_ = emitter.Emit(context.Background(), event)

	events := emitter.DrainBuffer()
	if events[0].Labels[LabelMetadataIncomplete] != "true" {
		t.Errorf("expected metadata_incomplete=true when Node is empty, got labels: %v", events[0].Labels)
	}
}

func TestEmit_MetadataIncomplete_PodEmpty(t *testing.T) {
	mock := &mockNATSPublisher{connected: false}
	emitter := NewNATSEmitter(NATSEmitterConfig{
		Publisher:      mock,
		BufferCapacity: 10,
	})

	event := export.Event{
		Module:    ModuleName,
		EventType: EventTypeDeploymentRisk,
		Node:      "node-1",
		Pod:       "", // empty
		Namespace: "default",
	}

	_ = emitter.Emit(context.Background(), event)

	events := emitter.DrainBuffer()
	if events[0].Labels[LabelMetadataIncomplete] != "true" {
		t.Errorf("expected metadata_incomplete=true when Pod is empty, got labels: %v", events[0].Labels)
	}
}

func TestEmit_MetadataIncomplete_NamespaceEmpty(t *testing.T) {
	mock := &mockNATSPublisher{connected: false}
	emitter := NewNATSEmitter(NATSEmitterConfig{
		Publisher:      mock,
		BufferCapacity: 10,
	})

	event := export.Event{
		Module:    ModuleName,
		EventType: EventTypeDeploymentRisk,
		Node:      "node-1",
		Pod:       "pod-1",
		Namespace: "", // empty
	}

	_ = emitter.Emit(context.Background(), event)

	events := emitter.DrainBuffer()
	if events[0].Labels[LabelMetadataIncomplete] != "true" {
		t.Errorf("expected metadata_incomplete=true when Namespace is empty, got labels: %v", events[0].Labels)
	}
}

func TestEmit_MetadataComplete_NoLabel(t *testing.T) {
	mock := &mockNATSPublisher{connected: false}
	emitter := NewNATSEmitter(NATSEmitterConfig{
		Publisher:      mock,
		BufferCapacity: 10,
	})

	event := export.Event{
		Module:    ModuleName,
		EventType: EventTypeDeploymentRisk,
		Node:      "node-1",
		Pod:       "pod-1",
		Namespace: "default",
	}

	_ = emitter.Emit(context.Background(), event)

	events := emitter.DrainBuffer()
	if val, ok := events[0].Labels[LabelMetadataIncomplete]; ok {
		t.Errorf("expected no metadata_incomplete label when all fields set, got %q", val)
	}
}

func TestEmit_MetadataIncomplete_PreservesExistingLabels(t *testing.T) {
	mock := &mockNATSPublisher{connected: false}
	emitter := NewNATSEmitter(NATSEmitterConfig{
		Publisher:      mock,
		BufferCapacity: 10,
	})

	event := export.Event{
		Module:    ModuleName,
		EventType: EventTypeDeploymentRisk,
		Node:      "", // triggers metadata_incomplete
		Pod:       "pod-1",
		Namespace: "default",
		Labels:    map[string]string{"service": "my-service"},
	}

	_ = emitter.Emit(context.Background(), event)

	events := emitter.DrainBuffer()
	if events[0].Labels["service"] != "my-service" {
		t.Errorf("expected existing label preserved, got labels: %v", events[0].Labels)
	}
	if events[0].Labels[LabelMetadataIncomplete] != "true" {
		t.Errorf("expected metadata_incomplete=true, got labels: %v", events[0].Labels)
	}
}

func TestFlush_PublishesBufferedEvents(t *testing.T) {
	mock := &mockNATSPublisher{connected: false}
	emitter := NewNATSEmitter(NATSEmitterConfig{
		Publisher:      mock,
		BufferCapacity: 10,
	})

	// Buffer some events while disconnected
	for i := 0; i < 3; i++ {
		event := export.Event{
			Module:    ModuleName,
			EventType: EventTypeDeploymentRisk,
			Node:      "node-1",
			Pod:       "pod-1",
			Namespace: fmt.Sprintf("ns-%d", i),
		}
		_ = emitter.Emit(context.Background(), event)
	}

	if emitter.BufferLen() != 3 {
		t.Fatalf("expected 3 buffered events, got %d", emitter.BufferLen())
	}

	// Now connect and flush
	mock.setConnected(true)
	err := emitter.Flush(context.Background())
	if err != nil {
		t.Fatalf("Flush returned error: %v", err)
	}

	// Buffer should be empty
	if emitter.BufferLen() != 0 {
		t.Errorf("expected buffer length 0 after flush, got %d", emitter.BufferLen())
	}

	// All events should have been published
	msgs := mock.getPublished()
	if len(msgs) != 3 {
		t.Errorf("expected 3 published messages after flush, got %d", len(msgs))
	}
}

func TestFlush_ErrorWhenDisconnected(t *testing.T) {
	mock := &mockNATSPublisher{connected: false}
	emitter := NewNATSEmitter(NATSEmitterConfig{
		Publisher:      mock,
		BufferCapacity: 10,
	})

	event := export.Event{
		Module:    ModuleName,
		EventType: EventTypeDeploymentRisk,
		Node:      "node-1",
		Pod:       "pod-1",
		Namespace: "default",
	}
	_ = emitter.Emit(context.Background(), event)

	err := emitter.Flush(context.Background())
	if err == nil {
		t.Fatal("expected error when flushing while disconnected")
	}
}

func TestFlush_RespectsContextCancellation(t *testing.T) {
	mock := &mockNATSPublisher{connected: true}
	emitter := NewNATSEmitter(NATSEmitterConfig{
		Publisher:      mock,
		BufferCapacity: 100,
	})

	// Buffer several events while disconnected
	mock.setConnected(false)
	for i := 0; i < 10; i++ {
		event := export.Event{
			Module:    ModuleName,
			EventType: EventTypeDeploymentRisk,
			Node:      "node-1",
			Pod:       "pod-1",
			Namespace: fmt.Sprintf("ns-%d", i),
		}
		_ = emitter.Emit(context.Background(), event)
	}

	// Create a pre-canceled context
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	mock.setConnected(true)
	err := emitter.Flush(ctx)
	if err == nil {
		t.Fatal("expected context cancellation error")
	}
}

func TestCalculateBackoff(t *testing.T) {
	emitter := NewNATSEmitter(NATSEmitterConfig{
		Publisher:      &mockNATSPublisher{connected: true},
		InitialBackoff: 1 * time.Second,
		MaxBackoff:     60 * time.Second,
	})

	tests := []struct {
		attempt  int
		expected time.Duration
	}{
		{1, 1 * time.Second},   // 1s * 2^0 = 1s
		{2, 2 * time.Second},   // 1s * 2^1 = 2s
		{3, 4 * time.Second},   // 1s * 2^2 = 4s
		{4, 8 * time.Second},   // 1s * 2^3 = 8s
		{5, 16 * time.Second},  // 1s * 2^4 = 16s
		{6, 32 * time.Second},  // 1s * 2^5 = 32s
		{7, 60 * time.Second},  // 1s * 2^6 = 64s -> capped at 60s
		{8, 60 * time.Second},  // capped at 60s
		{10, 60 * time.Second}, // capped at 60s
	}

	for _, tt := range tests {
		got := emitter.calculateBackoff(tt.attempt)
		if got != tt.expected {
			t.Errorf("calculateBackoff(%d) = %v, want %v", tt.attempt, got, tt.expected)
		}
	}
}

func TestNewNATSEmitter_Defaults(t *testing.T) {
	mock := &mockNATSPublisher{connected: true}
	emitter := NewNATSEmitter(NATSEmitterConfig{
		Publisher: mock,
	})

	if emitter.capacity != DefaultBufferCapacity {
		t.Errorf("expected default capacity %d, got %d", DefaultBufferCapacity, emitter.capacity)
	}
	if emitter.initialBackoff != DefaultInitialBackoff {
		t.Errorf("expected default initial backoff %v, got %v", DefaultInitialBackoff, emitter.initialBackoff)
	}
	if emitter.maxBackoff != DefaultMaxBackoff {
		t.Errorf("expected default max backoff %v, got %v", DefaultMaxBackoff, emitter.maxBackoff)
	}
	if emitter.maxRetries != DefaultMaxRetries {
		t.Errorf("expected default max retries %d, got %d", DefaultMaxRetries, emitter.maxRetries)
	}
	if emitter.subject != NATSSubject {
		t.Errorf("expected default subject %q, got %q", NATSSubject, emitter.subject)
	}
}

func TestEmit_UniqueEventIDs(t *testing.T) {
	mock := &mockNATSPublisher{connected: false}
	emitter := NewNATSEmitter(NATSEmitterConfig{
		Publisher:      mock,
		BufferCapacity: 100,
	})

	n := 50
	for i := 0; i < n; i++ {
		event := export.Event{
			Module:    ModuleName,
			EventType: EventTypeDeploymentRisk,
			Node:      "node-1",
			Pod:       "pod-1",
			Namespace: "default",
		}
		_ = emitter.Emit(context.Background(), event)
	}

	events := emitter.DrainBuffer()
	seen := make(map[string]bool, n)
	for _, ev := range events {
		if seen[ev.EventID] {
			t.Fatalf("duplicate EventID found: %s", ev.EventID)
		}
		seen[ev.EventID] = true
	}
}
