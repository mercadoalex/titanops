package export

import (
	"context"
	"fmt"
	"log"
	"math"
	"sync"
	"time"
)

const (
	// DefaultBufferCapacity is the maximum number of events per backend buffer.
	DefaultBufferCapacity = 1000

	// DefaultInitialBackoff is the initial retry delay.
	DefaultInitialBackoff = 1 * time.Second

	// DefaultMaxBackoff is the maximum retry delay.
	DefaultMaxBackoff = 60 * time.Second

	// DefaultMaxRetries is the maximum number of retry attempts per event.
	DefaultMaxRetries = 10
)

// BufferFullError indicates the buffer was full and events were evicted.
type BufferFullError struct {
	BackendName  string
	DiscardCount int
}

func (e *BufferFullError) Error() string {
	return fmt.Sprintf("backend %q buffer full: discarded %d oldest events", e.BackendName, e.DiscardCount)
}

// RetryExhaustedError indicates an event was discarded after max retries.
type RetryExhaustedError struct {
	BackendName string
	EventID     string
	Attempts    int
}

func (e *RetryExhaustedError) Error() string {
	return fmt.Sprintf("backend %q: event %q discarded after %d failed retries", e.BackendName, e.EventID, e.Attempts)
}

// bufferedEvent wraps an event with retry state for the ring buffer.
type bufferedEvent struct {
	event    Event
	attempts int
	nextTry  time.Time
}

// RingBuffer is a thread-safe ring buffer with oldest-first eviction
// and exponential backoff retry for a single backend.
type RingBuffer struct {
	mu             sync.Mutex
	items          []bufferedEvent
	capacity       int
	dropped        int
	backendName    string
	initialBackoff time.Duration
	maxBackoff     time.Duration
	maxRetries     int
	logger         *log.Logger
}

// RingBufferConfig configures a RingBuffer instance.
type RingBufferConfig struct {
	Capacity       int
	InitialBackoff time.Duration
	MaxBackoff     time.Duration
	MaxRetries     int
	Logger         *log.Logger
}

// NewRingBuffer creates a new ring buffer for the given backend.
func NewRingBuffer(backendName string, cfg *RingBufferConfig) *RingBuffer {
	capacity := DefaultBufferCapacity
	initialBackoff := DefaultInitialBackoff
	maxBackoff := DefaultMaxBackoff
	maxRetries := DefaultMaxRetries
	var logger *log.Logger

	if cfg != nil {
		if cfg.Capacity > 0 {
			capacity = cfg.Capacity
		}
		if cfg.InitialBackoff > 0 {
			initialBackoff = cfg.InitialBackoff
		}
		if cfg.MaxBackoff > 0 {
			maxBackoff = cfg.MaxBackoff
		}
		if cfg.MaxRetries > 0 {
			maxRetries = cfg.MaxRetries
		}
		logger = cfg.Logger
	}

	return &RingBuffer{
		items:          make([]bufferedEvent, 0, capacity),
		capacity:       capacity,
		backendName:    backendName,
		initialBackoff: initialBackoff,
		maxBackoff:     maxBackoff,
		maxRetries:     maxRetries,
		logger:         logger,
	}
}

// Push adds an event to the buffer. If the buffer is full, the oldest event
// is evicted and a warning is emitted with the discard count.
func (rb *RingBuffer) Push(event Event) {
	rb.mu.Lock()
	defer rb.mu.Unlock()

	if len(rb.items) >= rb.capacity {
		// Evict oldest event
		rb.items = rb.items[1:]
		rb.dropped++
		if rb.logger != nil {
			rb.logger.Printf("[WARN] backend %q buffer full: evicted oldest event (total discarded: %d)", rb.backendName, rb.dropped)
		}
	}

	rb.items = append(rb.items, bufferedEvent{
		event:    event,
		attempts: 0,
		nextTry:  time.Now(),
	})
}

// Peek returns the next event ready for retry without removing it.
// Returns nil if the buffer is empty or no events are ready.
func (rb *RingBuffer) Peek() *Event {
	rb.mu.Lock()
	defer rb.mu.Unlock()

	now := time.Now()
	for i := range rb.items {
		if rb.items[i].nextTry.Before(now) || rb.items[i].nextTry.Equal(now) {
			return &rb.items[i].event
		}
	}
	return nil
}

// MarkSuccess removes the first event from the buffer (it was sent successfully).
func (rb *RingBuffer) MarkSuccess() {
	rb.mu.Lock()
	defer rb.mu.Unlock()

	if len(rb.items) == 0 {
		return
	}

	// Find and remove the first ready event
	now := time.Now()
	for i := range rb.items {
		if rb.items[i].nextTry.Before(now) || rb.items[i].nextTry.Equal(now) {
			rb.items = append(rb.items[:i], rb.items[i+1:]...)
			return
		}
	}
}

// MarkFailure increments the retry count for the first ready event and applies
// exponential backoff. If max retries are exhausted, the event is discarded.
// Returns a RetryExhaustedError if the event was permanently discarded.
func (rb *RingBuffer) MarkFailure() error {
	rb.mu.Lock()
	defer rb.mu.Unlock()

	if len(rb.items) == 0 {
		return nil
	}

	now := time.Now()
	for i := range rb.items {
		if rb.items[i].nextTry.Before(now) || rb.items[i].nextTry.Equal(now) {
			rb.items[i].attempts++

			if rb.items[i].attempts >= rb.maxRetries {
				// Discard event after max retries
				eventID := rb.items[i].event.EventID
				rb.items = append(rb.items[:i], rb.items[i+1:]...)
				rb.dropped++

				err := &RetryExhaustedError{
					BackendName: rb.backendName,
					EventID:     eventID,
					Attempts:    rb.maxRetries,
				}
				if rb.logger != nil {
					rb.logger.Printf("[ERROR] %s", err.Error())
				}
				return err
			}

			// Apply exponential backoff
			backoff := rb.calculateBackoff(rb.items[i].attempts)
			rb.items[i].nextTry = now.Add(backoff)
			return nil
		}
	}
	return nil
}

// calculateBackoff returns the backoff duration for the given attempt number.
// Formula: min(initialBackoff * 2^(attempt-1), maxBackoff)
func (rb *RingBuffer) calculateBackoff(attempt int) time.Duration {
	backoff := float64(rb.initialBackoff) * math.Pow(2, float64(attempt-1))
	if backoff > float64(rb.maxBackoff) {
		return rb.maxBackoff
	}
	return time.Duration(backoff)
}

// Len returns the current number of events in the buffer.
func (rb *RingBuffer) Len() int {
	rb.mu.Lock()
	defer rb.mu.Unlock()
	return len(rb.items)
}

// Capacity returns the maximum capacity of the buffer.
func (rb *RingBuffer) Capacity() int {
	return rb.capacity
}

// Dropped returns the cumulative count of discarded events.
func (rb *RingBuffer) Dropped() int {
	rb.mu.Lock()
	defer rb.mu.Unlock()
	return rb.dropped
}

// Info returns the current BufferInfo state.
func (rb *RingBuffer) Info() BufferInfo {
	rb.mu.Lock()
	defer rb.mu.Unlock()
	return BufferInfo{
		Capacity: rb.capacity,
		Used:     len(rb.items),
		Dropped:  rb.dropped,
	}
}

// Drain removes and returns all events from the buffer.
// This is useful for testing or shutdown scenarios.
func (rb *RingBuffer) Drain() []Event {
	rb.mu.Lock()
	defer rb.mu.Unlock()

	events := make([]Event, len(rb.items))
	for i, item := range rb.items {
		events[i] = item.event
	}
	rb.items = rb.items[:0]
	return events
}

// ProcessBuffer attempts to send buffered events to the backend.
// It processes one event at a time. Returns when no events are ready for retry.
func ProcessBuffer(ctx context.Context, rb *RingBuffer, backend Backend) error {
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		event := rb.Peek()
		if event == nil {
			return nil // No events ready for retry
		}

		err := backend.Send(ctx, *event)
		if err != nil {
			retryErr := rb.MarkFailure()
			if retryErr != nil {
				// Event was permanently discarded
				return retryErr
			}
			// Event still in buffer with updated backoff, stop processing for now
			return nil
		}

		rb.MarkSuccess()
	}
}
