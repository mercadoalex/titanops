package earthworm

import (
	"context"

	export "github.com/mercadoalex/titanops/shared/titanops-export"
)

// EventEmitter defines the interface for publishing events to the event bus.
// Implementations must emit events within 5 seconds of the triggering action.
type EventEmitter interface {
	// Emit publishes an event to the event bus.
	// Returns an error if the event cannot be published.
	Emit(ctx context.Context, event export.Event) error
}
