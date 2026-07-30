// Package platform defines the formal contract between TitanOps modules and the
// platform kernel. Every module implements the Module interface; the kernel
// provides shared services via the Kernel interface.
//
// This is intentionally minimal — a "micro-kernel" contract without the
// infrastructure overhead of dynamic plugin loading. Modules are compiled in
// and registered at startup. The interfaces ensure clean separation of concerns
// and allow independent module development against a stable API.
//
// Design principles:
//   - Modules own their hot path (eBPF → AI → action) with zero kernel indirection.
//   - The kernel provides shared services (K8s client, AI provider, event bus).
//   - Modules communicate only through the event bus — never directly.
//   - Fault isolation: a panicking module is caught and reported, not propagated.
package platform

import (
	"context"
	"encoding/json"
	"time"

	ai "github.com/mercadoalex/titanops/shared/titanops-ai"
	export "github.com/mercadoalex/titanops/shared/titanops-export"
	k8s "github.com/mercadoalex/titanops/shared/titanops-k8s"
)

// Module is the contract every TitanOps module must satisfy.
// The kernel manages the lifecycle of all registered modules.
type Module interface {
	// ID returns the unique module identifier (e.g., "earthworm", "ebeecontrol", "quack").
	ID() string

	// Version returns the semantic version of the module (e.g., "v1.2.0").
	Version() string

	// Start initializes the module and begins its operation.
	// The kernel is provided for access to shared services.
	// The context is canceled when the platform is shutting down.
	Start(ctx context.Context, kernel Kernel) error

	// Stop gracefully shuts down the module.
	// Modules must release all resources and stop goroutines within the context deadline.
	Stop(ctx context.Context) error

	// HealthCheck reports the module's current health status.
	// Must complete within 5 seconds.
	HealthCheck(ctx context.Context) HealthStatus
}

// Kernel provides shared platform services to modules.
// Modules receive this in Start() and use it for all platform interactions.
type Kernel interface {
	// --- Event Bus ---

	// Emit publishes an event to the correlation engine and all subscribers.
	// Events must be emitted within 5 seconds of the triggering action.
	Emit(ctx context.Context, event export.Event) error

	// Subscribe registers a handler for events matching the given filter.
	// Returns a Subscription that can be closed to unsubscribe.
	Subscribe(filter EventFilter, handler EventHandler) Subscription

	// --- Shared Services ---

	// K8sClient returns the shared Kubernetes client.
	// All modules share one client to avoid excessive API server connections.
	K8sClient() k8s.Client

	// AIProvider returns the shared AI inference provider (local ONNX + optional cloud).
	AIProvider() ai.Provider

	// ModuleConfig returns the module-specific configuration as raw JSON.
	// The module is responsible for unmarshaling into its own config struct.
	ModuleConfig(moduleID string) json.RawMessage

	// --- Module Registry ---

	// GetModule returns another registered module by ID.
	// Returns nil if the module is not registered or has not started.
	// Use sparingly — prefer event-based communication.
	GetModule(id string) Module

	// --- Platform Info ---

	// ClusterID returns the unique identifier for the current cluster.
	ClusterID() string

	// Logger returns a structured logger namespaced to the requesting module.
	Logger(moduleID string) Logger
}

// --- Event Bus Types ---

// EventHandler is a callback invoked when a matching event is received.
type EventHandler func(ctx context.Context, event export.Event)

// EventFilter determines which events a subscriber receives.
type EventFilter struct {
	// Module filters events by source module. Empty means all modules.
	Module string
	// EventType filters by event type. Empty means all types.
	EventType string
	// MinSeverity filters events at or above this severity.
	// Valid values: "critical", "high", "medium", "low", "informational".
	// Empty means all severities.
	MinSeverity string
}

// Subscription represents an active event subscription.
// Close it to stop receiving events.
type Subscription interface {
	// Close unsubscribes and releases resources.
	Close()
}

// --- Health Types ---

// HealthState represents the operational state of a module.
type HealthState string

const (
	// HealthHealthy indicates the module is fully operational.
	HealthHealthy HealthState = "healthy"
	// HealthDegraded indicates the module is operational with reduced capability.
	HealthDegraded HealthState = "degraded"
	// HealthUnhealthy indicates the module cannot perform its primary function.
	HealthUnhealthy HealthState = "unhealthy"
)

// HealthStatus is the result of a module health check.
type HealthStatus struct {
	// State is the overall health state.
	State HealthState `json:"state"`
	// Message provides human-readable context about the current state.
	Message string `json:"message,omitempty"`
	// Components reports health of individual sub-components (optional).
	Components map[string]ComponentHealth `json:"components,omitempty"`
	// LastCheck is when this health status was generated.
	LastCheck time.Time `json:"last_check"`
}

// ComponentHealth reports health of a single sub-component within a module.
type ComponentHealth struct {
	State   HealthState `json:"state"`
	Message string      `json:"message,omitempty"`
}

// --- Logger Interface ---

// Logger provides structured logging for modules.
// The kernel implementation prefixes all messages with the module ID.
type Logger interface {
	// Info logs an informational message.
	Info(msg string, keysAndValues ...interface{})
	// Warn logs a warning message.
	Warn(msg string, keysAndValues ...interface{})
	// Error logs an error message.
	Error(msg string, keysAndValues ...interface{})
}

// --- Module Metadata ---

// ModuleInfo provides static metadata about a module for registration and display.
type ModuleInfo struct {
	// ID is the unique module identifier.
	ID string `json:"id"`
	// Version is the semantic version.
	Version string `json:"version"`
	// Description is a one-line summary of the module's purpose.
	Description string `json:"description"`
	// Domain categorizes the module (e.g., "health", "security", "performance", "threat").
	Domain string `json:"domain"`
	// EventTypes lists the event types this module emits.
	EventTypes []string `json:"event_types"`
	// RequiredServices lists shared services this module needs (e.g., "k8s", "ai").
	RequiredServices []string `json:"required_services,omitempty"`
}
