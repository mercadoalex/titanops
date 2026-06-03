// Package correlation implements the cross-module event correlation engine
// that detects related signals across TitanOps modules.
package correlation

import (
	"context"
	"fmt"
	"sync"
	"time"

	export "github.com/mercadoalex/titanops/shared/titanops-export"
)

// EngineConfig holds the configuration for the correlation engine.
type EngineConfig struct {
	// TimeWindow is the sliding window duration for matching events.
	// Default: 120s. Valid range: 10s to 600s.
	TimeWindow time.Duration
	// ConfidenceThreshold is the minimum confidence score required to execute auto-actions.
	// Default: 80. Valid range: 1 to 100.
	ConfidenceThreshold int
	// AutoActions lists the configured auto-action types.
	AutoActions []AutoActionConfig
}

// AutoActionConfig defines a configurable auto-action.
type AutoActionConfig struct {
	// Type is the action type: isolate_pod, alert_operator, forensic_report.
	Type string
	// Enabled controls whether this action is active.
	Enabled bool
	// Parameters holds action-specific key-value configuration.
	Parameters map[string]string
}

// TimedEvent wraps an export.Event with metadata about when it was received.
type TimedEvent struct {
	// Event is the original telemetry event.
	Event export.Event
	// ReceivedAt is the time the event was received by the correlation engine.
	ReceivedAt time.Time
}

// CorrelatedIncident represents a group of correlated events from multiple modules.
type CorrelatedIncident struct {
	// IncidentID is the unique identifier for this incident.
	IncidentID string
	// ContributingEvents are the events that formed this correlation.
	ContributingEvents []export.Event
	// ConfidenceScore is the calculated confidence level (0-100).
	ConfidenceScore int
	// Narrative is a human-readable description of the incident.
	Narrative string
	// MatchedAttributes lists the attribute types that matched (node, pod, namespace).
	MatchedAttributes []string
	// DetectedAt is when the correlation was detected.
	DetectedAt time.Time
	// RecommendedAction is the action to take if confidence exceeds threshold.
	RecommendedAction *AutoAction
	// ActionExecuted indicates whether the auto-action was executed.
	ActionExecuted bool
	// ActionError holds the error message if action execution failed.
	ActionError string
}

// IncidentFilter allows filtering when retrieving incidents.
type IncidentFilter struct {
	// Since returns only incidents detected after this time.
	Since time.Time
	// MinConfidence returns only incidents with at least this confidence score.
	MinConfidence int
	// Modules filters to incidents involving these modules.
	Modules []string
}

// ConfigError is a typed error for invalid configuration.
type ConfigError struct {
	Field   string
	Value   string
	Message string
}

func (e *ConfigError) Error() string {
	return fmt.Sprintf("correlation config error: field %q (%s): %s", e.Field, e.Value, e.Message)
}

// CorrelationError is a typed error for correlation operations.
type CorrelationError struct {
	Operation string
	Cause     string
}

func (e *CorrelationError) Error() string {
	return fmt.Sprintf("correlation error in %s: %s", e.Operation, e.Cause)
}

// DefaultConfig returns the default engine configuration.
func DefaultConfig() EngineConfig {
	return EngineConfig{
		TimeWindow:          120 * time.Second,
		ConfidenceThreshold: 80,
		AutoActions: []AutoActionConfig{
			{Type: "isolate_pod", Enabled: true},
			{Type: "alert_operator", Enabled: true},
			{Type: "forensic_report", Enabled: true},
		},
	}
}

// ValidateConfig validates the engine configuration and returns an error if invalid.
func ValidateConfig(cfg EngineConfig) error {
	if cfg.TimeWindow < 10*time.Second || cfg.TimeWindow > 600*time.Second {
		return &ConfigError{
			Field:   "TimeWindow",
			Value:   cfg.TimeWindow.String(),
			Message: "must be between 10s and 600s",
		}
	}
	if cfg.ConfidenceThreshold < 1 || cfg.ConfidenceThreshold > 100 {
		return &ConfigError{
			Field:   "ConfidenceThreshold",
			Value:   fmt.Sprintf("%d", cfg.ConfidenceThreshold),
			Message: "must be between 1 and 100",
		}
	}
	return nil
}

// Engine processes events from the event bus and generates correlated incidents.
type Engine struct {
	config    EngineConfig
	events    []TimedEvent
	incidents []CorrelatedIncident
	mu        sync.RWMutex
	exporter  export.Exporter
	actions   ActionExecutor
	idGen     func() string
}

// NewEngine creates a new correlation engine with the given configuration.
// The exporter and actions parameters may be nil if not needed.
func NewEngine(cfg EngineConfig, exporter export.Exporter, actions ActionExecutor) (*Engine, error) {
	if err := ValidateConfig(cfg); err != nil {
		return nil, err
	}
	return &Engine{
		config:   cfg,
		events:   make([]TimedEvent, 0),
		exporter: exporter,
		actions:  actions,
		idGen:    defaultIDGen,
	}, nil
}

// defaultIDGen generates a simple incident ID based on timestamp.
func defaultIDGen() string {
	return fmt.Sprintf("inc-%d", time.Now().UnixNano())
}

// Ingest adds an event to the correlation engine's sliding window.
// It is safe for concurrent use.
func (e *Engine) Ingest(ctx context.Context, event export.Event) error {
	if ctx.Err() != nil {
		return ctx.Err()
	}

	e.mu.Lock()
	defer e.mu.Unlock()

	now := time.Now()
	e.events = append(e.events, TimedEvent{
		Event:      event,
		ReceivedAt: now,
	})

	// Prune events outside the time window.
	e.pruneEventsLocked(now)

	return nil
}

// Correlate analyzes the current event window and generates correlated incidents.
// It returns newly detected incidents. It is safe for concurrent use.
func (e *Engine) Correlate(ctx context.Context) ([]CorrelatedIncident, error) {
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}

	e.mu.Lock()
	defer e.mu.Unlock()

	now := time.Now()
	e.pruneEventsLocked(now)

	// Group events by shared attributes.
	groups := e.groupEvents()

	var newIncidents []CorrelatedIncident
	for _, group := range groups {
		// Require at least 2 distinct modules.
		modules := distinctModules(group)
		if len(modules) < 2 {
			continue
		}

		// Calculate matched attributes for this group.
		matchedAttrs := group.matchedAttributes

		// Calculate confidence score.
		score := CalculateConfidence(group.events, matchedAttrs)

		// Generate narrative.
		narrative := GenerateNarrative(group.events, matchedAttrs, score)

		// Determine recommended action.
		var action *AutoAction
		for _, ac := range e.config.AutoActions {
			if ac.Enabled {
				action = &AutoAction{
					Type:       ac.Type,
					Parameters: ac.Parameters,
				}
				break
			}
		}

		incident := CorrelatedIncident{
			IncidentID:         e.idGen(),
			ContributingEvents: group.events,
			ConfidenceScore:    score,
			Narrative:          narrative,
			MatchedAttributes:  matchedAttrs,
			DetectedAt:         now,
			RecommendedAction:  action,
		}

		// Execute auto-action if confidence meets threshold.
		if score >= e.config.ConfidenceThreshold && action != nil && e.actions != nil {
			err := e.actions.Execute(ctx, *action, incident)
			if err != nil {
				incident.ActionError = err.Error()
				// Alert operator on failure.
				e.alertOperatorOnFailure(ctx, incident, err)
			} else {
				incident.ActionExecuted = true
			}
		}

		e.incidents = append(e.incidents, incident)
		newIncidents = append(newIncidents, incident)

		// Emit to export adapters.
		if e.exporter != nil {
			e.emitIncident(ctx, incident)
		}
	}

	return newIncidents, nil
}

// GetIncidents returns incidents matching the given filter.
func (e *Engine) GetIncidents(ctx context.Context, filter IncidentFilter) ([]CorrelatedIncident, error) {
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}

	e.mu.RLock()
	defer e.mu.RUnlock()

	var result []CorrelatedIncident
	for _, inc := range e.incidents {
		if !filter.Since.IsZero() && inc.DetectedAt.Before(filter.Since) {
			continue
		}
		if filter.MinConfidence > 0 && inc.ConfidenceScore < filter.MinConfidence {
			continue
		}
		if len(filter.Modules) > 0 {
			if !incidentInvolvesModules(inc, filter.Modules) {
				continue
			}
		}
		result = append(result, inc)
	}
	return result, nil
}

// pruneEventsLocked removes events outside the time window.
// Must be called with e.mu held.
func (e *Engine) pruneEventsLocked(now time.Time) {
	cutoff := now.Add(-e.config.TimeWindow)
	pruned := make([]TimedEvent, 0, len(e.events))
	for _, te := range e.events {
		if te.ReceivedAt.After(cutoff) || te.ReceivedAt.Equal(cutoff) {
			pruned = append(pruned, te)
		}
	}
	e.events = pruned
}

// eventGroup represents a group of events sharing attributes.
type eventGroup struct {
	events            []export.Event
	matchedAttributes []string
}

// groupEvents groups events by shared attributes (node, pod, namespace).
func (e *Engine) groupEvents() []eventGroup {
	// Build groups based on each shared attribute combination.
	type attrKey struct {
		node      string
		pod       string
		namespace string
	}

	groups := make(map[attrKey]*eventGroup)

	for _, te := range e.events {
		key := attrKey{
			node:      te.Event.Node,
			pod:       te.Event.Pod,
			namespace: te.Event.Namespace,
		}

		if g, ok := groups[key]; ok {
			g.events = append(g.events, te.Event)
		} else {
			// Determine which attributes are shared.
			var matched []string
			if key.node != "" {
				matched = append(matched, "node")
			}
			if key.pod != "" {
				matched = append(matched, "pod")
			}
			if key.namespace != "" {
				matched = append(matched, "namespace")
			}
			groups[key] = &eventGroup{
				events:            []export.Event{te.Event},
				matchedAttributes: matched,
			}
		}
	}

	var result []eventGroup
	for _, g := range groups {
		result = append(result, *g)
	}
	return result
}

// distinctModules returns the unique module names from a group of events.
func distinctModules(group eventGroup) []string {
	seen := make(map[string]bool)
	var modules []string
	for _, ev := range group.events {
		if !seen[ev.Module] {
			seen[ev.Module] = true
			modules = append(modules, ev.Module)
		}
	}
	return modules
}

// incidentInvolvesModules checks if an incident has events from any of the given modules.
func incidentInvolvesModules(inc CorrelatedIncident, modules []string) bool {
	moduleSet := make(map[string]bool, len(modules))
	for _, m := range modules {
		moduleSet[m] = true
	}
	for _, ev := range inc.ContributingEvents {
		if moduleSet[ev.Module] {
			return true
		}
	}
	return false
}

// emitIncident sends a correlated incident as an event to the exporter.
func (e *Engine) emitIncident(ctx context.Context, incident CorrelatedIncident) {
	event := export.Event{
		Namespace: "titanops-system",
		Timestamp: incident.DetectedAt.UTC(),
		Severity:  "high",
		Module:    "correlation",
		EventType: "correlated_incident",
		EventID:   incident.IncidentID,
		Payload:   []byte(incident.Narrative),
	}
	// Best-effort export; don't block on failure.
	_ = e.exporter.Export(ctx, event)
}

// alertOperatorOnFailure emits an alert when an auto-action fails.
func (e *Engine) alertOperatorOnFailure(ctx context.Context, incident CorrelatedIncident, actionErr error) {
	if e.exporter == nil {
		return
	}
	msg := fmt.Sprintf("Auto-action %q failed for incident %s: %s",
		incident.RecommendedAction.Type, incident.IncidentID, actionErr.Error())
	event := export.Event{
		Namespace: "titanops-system",
		Timestamp: time.Now().UTC(),
		Severity:  "critical",
		Module:    "correlation",
		EventType: "action_failure",
		EventID:   fmt.Sprintf("alert-%s", incident.IncidentID),
		Payload:   []byte(msg),
	}
	_ = e.exporter.Export(ctx, event)
}
