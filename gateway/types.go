package gateway

import "time"

// ModuleHealth represents the health status of a single TitanOps module.
type ModuleHealth struct {
	Module string    `json:"module"`
	Status string    `json:"status"` // operational, degraded, unavailable
	Since  time.Time `json:"since"`
}

// AutonomousAction represents an autonomous action taken by a module.
type AutonomousAction struct {
	ID         string         `json:"id"`
	Module     string         `json:"module"`
	ActionType string         `json:"action_type"`
	Confidence float64        `json:"confidence"`
	Reasoning  ReasoningChain `json:"reasoning"`
	Outcome    string         `json:"outcome"` // success, failed, rejected, paused
	Timestamp  time.Time      `json:"timestamp"`
	OverrideBy string         `json:"override_by,omitempty"`
}

// ReasoningChain provides explainability details for an autonomous action.
type ReasoningChain struct {
	Observation  string   `json:"observation"`
	Analysis     string   `json:"analysis"`
	Action       string   `json:"action"`
	Alternatives []string `json:"alternatives,omitempty"`
}

// AuditEntry records an action or override in the audit trail.
type AuditEntry struct {
	Timestamp    time.Time `json:"timestamp"`
	Module       string    `json:"module"`
	ActionType   string    `json:"action_type"`
	TriggerEvent string    `json:"trigger_event"`
	Confidence   float64   `json:"confidence"`
	Reasoning    string    `json:"reasoning"`
	Outcome      string    `json:"outcome"`
	OperatorID   string    `json:"operator_id,omitempty"`
}

// AuditFilter allows filtering audit trail queries.
type AuditFilter struct {
	Since      time.Time `json:"since,omitempty"`
	Module     string    `json:"module,omitempty"`
	ActionType string    `json:"action_type,omitempty"`
}

// OverrideRequest represents an operator override operation.
type OverrideRequest struct {
	ActionID   string `json:"action_id"`
	ModuleID   string `json:"module_id"`
	Operation  string `json:"operation"` // approve, reject, pause, resume
	OperatorID string `json:"operator_id"`
}

// CorrelatedIncident represents a cross-module correlated incident for the API.
type CorrelatedIncident struct {
	IncidentID    string    `json:"incident_id"`
	Modules       []string  `json:"modules"`
	Confidence    int       `json:"confidence"`
	Narrative     string    `json:"narrative"`
	MatchedAttrs  []string  `json:"matched_attributes"`
	DetectedAt    time.Time `json:"detected_at"`
	ActionTaken   bool      `json:"action_taken"`
	ActionOutcome string    `json:"action_outcome,omitempty"`
}
