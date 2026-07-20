package ebeecontrol

import (
	"context"

	export "github.com/mercadoalex/titanops/shared/titanops-export"
)

// EventEmitter defines the interface for publishing events to the TitanOps event bus.
// Implementations must emit events within 5 seconds of the triggering action.
type EventEmitter interface {
	// Emit publishes an event to the correlation engine via the event bus.
	// Returns an error if the event cannot be published.
	Emit(ctx context.Context, event export.Event) error
}

// NoopEmitter is an EventEmitter that discards all events.
// Useful for testing or running without the correlation engine.
type NoopEmitter struct{}

// Emit discards the event and always returns nil.
func (NoopEmitter) Emit(_ context.Context, _ export.Event) error {
	return nil
}

// ChannelEmitter is an EventEmitter that sends events to a Go channel.
// Useful for testing or in-process event routing.
type ChannelEmitter struct {
	ch chan<- export.Event
}

// NewChannelEmitter creates an emitter that sends events to the provided channel.
// If the channel is full, Emit blocks until space is available or context is cancelled.
func NewChannelEmitter(ch chan<- export.Event) *ChannelEmitter {
	return &ChannelEmitter{ch: ch}
}

// Emit sends the event to the channel, respecting context cancellation.
func (e *ChannelEmitter) Emit(ctx context.Context, event export.Event) error {
	select {
	case e.ch <- event:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}
