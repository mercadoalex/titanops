package ollinai

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"math"
	"sync"
	"time"

	"github.com/google/uuid"
	export "github.com/mercadoalex/titanops/shared/titanops-export"
)

const (
	// DefaultBufferCapacity is the default ring buffer capacity for the emitter.
	DefaultBufferCapacity = 1000

	// DefaultInitialBackoff is the initial retry delay for failed publishes.
	DefaultInitialBackoff = 1 * time.Second

	// DefaultMaxBackoff is the maximum retry delay.
	DefaultMaxBackoff = 60 * time.Second

	// DefaultMaxRetries is the maximum number of retry attempts per event.
	DefaultMaxRetries = 10

	// NATSSubject is the NATS subject used for OllinAI events.
	NATSSubject = "titanops.ollinai.events"
)

// EventEmitter publishes events to the NATS event bus with buffering and retry.
type EventEmitter interface {
	// Emit publishes an event. If NATS is unavailable, buffers locally.
	Emit(ctx context.Context, event export.Event) error
	// Flush drains all buffered events to NATS (used during shutdown).
	Flush(ctx context.Context) error
	// BufferLen returns current buffer occupancy.
	BufferLen() int
}

// NATSPublisher is the interface for publishing to NATS.
// This allows testing with a mock. The real NATS connection will be wired later.
type NATSPublisher interface {
	// Publish sends data to the given NATS subject.
	Publish(subject string, data []byte) error
	// IsConnected returns true if the NATS connection is active.
	IsConnected() bool
}

// bufferedEvent wraps an event with retry metadata for the ring buffer.
type bufferedEvent struct {
	event    export.Event
	attempts int
	nextTry  time.Time
}

// NATSEmitter implements EventEmitter using a NATS publisher and ring buffer.
type NATSEmitter struct {
	mu             sync.Mutex
	publisher      NATSPublisher
	buffer         []bufferedEvent
	capacity       int
	dropped        int
	initialBackoff time.Duration
	maxBackoff     time.Duration
	maxRetries     int
	subject        string
	logger         *log.Logger
}

// NATSEmitterConfig configures the NATSEmitter.
type NATSEmitterConfig struct {
	// Publisher is the NATS publisher interface.
	Publisher NATSPublisher
	// BufferCapacity is the ring buffer capacity. Default: 1000.
	BufferCapacity int
	// InitialBackoff is the initial retry delay. Default: 1s.
	InitialBackoff time.Duration
	// MaxBackoff is the maximum retry delay. Default: 60s.
	MaxBackoff time.Duration
	// MaxRetries is the maximum retry attempts per event. Default: 10.
	MaxRetries int
	// Subject is the NATS subject. Default: "titanops.ollinai.events".
	Subject string
	// Logger for warning/error messages. Optional.
	Logger *log.Logger
}

// NewNATSEmitter creates a new NATSEmitter with the given configuration.
func NewNATSEmitter(cfg NATSEmitterConfig) *NATSEmitter {
	capacity := cfg.BufferCapacity
	if capacity <= 0 {
		capacity = DefaultBufferCapacity
	}

	initialBackoff := cfg.InitialBackoff
	if initialBackoff <= 0 {
		initialBackoff = DefaultInitialBackoff
	}

	maxBackoff := cfg.MaxBackoff
	if maxBackoff <= 0 {
		maxBackoff = DefaultMaxBackoff
	}

	maxRetries := cfg.MaxRetries
	if maxRetries <= 0 {
		maxRetries = DefaultMaxRetries
	}

	subject := cfg.Subject
	if subject == "" {
		subject = NATSSubject
	}

	return &NATSEmitter{
		publisher:      cfg.Publisher,
		buffer:         make([]bufferedEvent, 0, capacity),
		capacity:       capacity,
		initialBackoff: initialBackoff,
		maxBackoff:     maxBackoff,
		maxRetries:     maxRetries,
		subject:        subject,
		logger:         cfg.Logger,
	}
}

// Emit publishes an event to NATS. If NATS is unavailable, the event is buffered
// in the ring buffer. Before publishing, it assigns a UUID v4 EventID and UTC
// RFC 3339 timestamp if not already set, and applies the metadata_incomplete label
// when Node, Pod, or Namespace is empty.
func (e *NATSEmitter) Emit(ctx context.Context, event export.Event) error {
	// Assign EventID if not set
	if event.EventID == "" {
		event.EventID = uuid.New().String()
	}

	// Assign Timestamp if zero
	if event.Timestamp.IsZero() {
		event.Timestamp = time.Now().UTC()
	} else {
		// Ensure timestamp is UTC
		event.Timestamp = event.Timestamp.UTC()
	}

	// Check metadata completeness
	if event.Node == "" || event.Pod == "" || event.Namespace == "" {
		if event.Labels == nil {
			event.Labels = make(map[string]string)
		}
		event.Labels[LabelMetadataIncomplete] = "true"
	}

	// Try to publish directly if NATS is connected
	if e.publisher != nil && e.publisher.IsConnected() {
		data, err := serializeEvent(event)
		if err != nil {
			return fmt.Errorf("failed to serialize event: %w", err)
		}

		if err := e.publisher.Publish(e.subject, data); err == nil {
			return nil
		}
		// Publish failed; fall through to buffer
	}

	// Buffer the event
	e.pushToBuffer(event)
	return nil
}

// Flush attempts to publish all buffered events to NATS.
// It processes events in order, respecting backoff timers.
// Events that exceed max retries are discarded with a warning.
func (e *NATSEmitter) Flush(ctx context.Context) error {
	e.mu.Lock()
	defer e.mu.Unlock()

	if e.publisher == nil || !e.publisher.IsConnected() {
		return fmt.Errorf("NATS not connected, cannot flush")
	}

	var remaining []bufferedEvent

	for i := range e.buffer {
		select {
		case <-ctx.Done():
			// Keep unprocessed events in buffer
			remaining = append(remaining, e.buffer[i:]...)
			e.buffer = remaining
			return ctx.Err()
		default:
		}

		item := &e.buffer[i]
		data, err := serializeEvent(item.event)
		if err != nil {
			e.logWarn("failed to serialize event %s during flush: %v", item.event.EventID, err)
			// Discard malformed events
			continue
		}

		if err := e.publisher.Publish(e.subject, data); err != nil {
			item.attempts++
			if item.attempts >= e.maxRetries {
				e.logWarn("event %s discarded after %d failed attempts during flush", item.event.EventID, item.attempts)
				e.dropped++
				continue
			}
			// Keep in buffer for later retry
			item.nextTry = time.Now().Add(e.calculateBackoff(item.attempts))
			remaining = append(remaining, *item)
		}
		// Success: event is not added to remaining
	}

	e.buffer = remaining
	return nil
}

// BufferLen returns the current number of events in the ring buffer.
func (e *NATSEmitter) BufferLen() int {
	e.mu.Lock()
	defer e.mu.Unlock()
	return len(e.buffer)
}

// pushToBuffer adds an event to the ring buffer, evicting the oldest event if full.
func (e *NATSEmitter) pushToBuffer(event export.Event) {
	e.mu.Lock()
	defer e.mu.Unlock()

	if len(e.buffer) >= e.capacity {
		// Evict oldest event
		e.buffer = e.buffer[1:]
		e.dropped++
		e.logWarn("ring buffer full (capacity %d): evicted oldest event (total dropped: %d)", e.capacity, e.dropped)
	}

	e.buffer = append(e.buffer, bufferedEvent{
		event:    event,
		attempts: 0,
		nextTry:  time.Now(),
	})
}

// DrainBuffer returns all buffered events and clears the buffer.
// Used for testing and shutdown scenarios.
func (e *NATSEmitter) DrainBuffer() []export.Event {
	e.mu.Lock()
	defer e.mu.Unlock()

	events := make([]export.Event, len(e.buffer))
	for i, item := range e.buffer {
		events[i] = item.event
	}
	e.buffer = e.buffer[:0]
	return events
}

// Dropped returns the total number of events dropped (evicted or retry-exhausted).
func (e *NATSEmitter) Dropped() int {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.dropped
}

// calculateBackoff returns the backoff duration for the given attempt number.
// Formula: min(initialBackoff * 2^(attempt-1), maxBackoff)
func (e *NATSEmitter) calculateBackoff(attempt int) time.Duration {
	backoff := float64(e.initialBackoff) * math.Pow(2, float64(attempt-1))
	if backoff > float64(e.maxBackoff) {
		return e.maxBackoff
	}
	return time.Duration(backoff)
}

func (e *NATSEmitter) logWarn(format string, args ...any) {
	if e.logger != nil {
		e.logger.Printf("[WARN] "+format, args...)
	}
}

// serializeEvent serializes an export.Event to JSON for NATS publishing.
func serializeEvent(event export.Event) ([]byte, error) {
	return json.Marshal(event)
}
