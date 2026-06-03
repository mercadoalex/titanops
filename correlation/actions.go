package correlation

import (
	"context"
	"fmt"
)

// ActionExecutor defines the interface for executing auto-actions
// when a correlated incident meets the confidence threshold.
type ActionExecutor interface {
	// Execute performs the given auto-action in the context of a correlated incident.
	// Returns an error if the action fails; the engine will record the failure
	// and alert the operator.
	Execute(ctx context.Context, action AutoAction, incident CorrelatedIncident) error
}

// AutoAction represents an action to be taken in response to a correlated incident.
type AutoAction struct {
	// Type is the action identifier: isolate_pod, alert_operator, forensic_report.
	Type string
	// Parameters holds action-specific configuration values.
	Parameters map[string]string
}

// ValidActionTypes contains the accepted auto-action type values.
var ValidActionTypes = map[string]bool{
	"isolate_pod":     true,
	"alert_operator":  true,
	"forensic_report": true,
}

// ActionError is a typed error for action execution failures.
type ActionError struct {
	// ActionType is the type of action that failed.
	ActionType string
	// IncidentID is the incident the action was for.
	IncidentID string
	// Cause describes why the action failed.
	Cause string
}

func (e *ActionError) Error() string {
	return fmt.Sprintf("action %q failed for incident %s: %s", e.ActionType, e.IncidentID, e.Cause)
}

// NoOpExecutor is an ActionExecutor that does nothing (useful for testing).
type NoOpExecutor struct{}

// Execute implements ActionExecutor by always succeeding.
func (n *NoOpExecutor) Execute(_ context.Context, _ AutoAction, _ CorrelatedIncident) error {
	return nil
}

// FailingExecutor is an ActionExecutor that always fails (useful for testing).
type FailingExecutor struct {
	Err error
}

// Execute implements ActionExecutor by always returning the configured error.
func (f *FailingExecutor) Execute(_ context.Context, action AutoAction, incident CorrelatedIncident) error {
	if f.Err != nil {
		return f.Err
	}
	return &ActionError{
		ActionType: action.Type,
		IncidentID: incident.IncidentID,
		Cause:      "executor configured to fail",
	}
}

// RecordingExecutor records all executed actions (useful for testing).
type RecordingExecutor struct {
	Executed []ExecutedAction
	Err      error
}

// ExecutedAction records the details of an executed action.
type ExecutedAction struct {
	Action   AutoAction
	Incident CorrelatedIncident
}

// Execute implements ActionExecutor by recording the action and optionally returning an error.
func (r *RecordingExecutor) Execute(_ context.Context, action AutoAction, incident CorrelatedIncident) error {
	r.Executed = append(r.Executed, ExecutedAction{
		Action:   action,
		Incident: incident,
	})
	return r.Err
}
