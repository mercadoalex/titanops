package gateway

import (
	"context"
	"sync"
	"time"
)

// HealthStore provides thread-safe in-memory storage for module health status.
type HealthStore struct {
	mu      sync.RWMutex
	modules []ModuleHealth
}

// NewHealthStore creates a health store initialized with the four TitanOps modules.
func NewHealthStore() *HealthStore {
	now := time.Now().UTC()
	return &HealthStore{
		modules: []ModuleHealth{
			{Module: "earthworm", Status: "operational", Since: now},
			{Module: "tlapix", Status: "operational", Since: now},
			{Module: "ebeecontrol", Status: "operational", Since: now},
			{Module: "quack", Status: "operational", Since: now},
		},
	}
}

// All returns the health status of all modules.
func (s *HealthStore) All(_ context.Context) []ModuleHealth {
	s.mu.RLock()
	defer s.mu.RUnlock()

	result := make([]ModuleHealth, len(s.modules))
	copy(result, s.modules)
	return result
}

// Update updates the status of a specific module.
func (s *HealthStore) Update(_ context.Context, module, status string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	for i := range s.modules {
		if s.modules[i].Module == module {
			s.modules[i].Status = status
			s.modules[i].Since = time.Now().UTC()
			return
		}
	}
}

// ActionStore provides thread-safe in-memory storage for autonomous actions.
type ActionStore struct {
	mu      sync.RWMutex
	actions []AutonomousAction
}

// NewActionStore creates a new empty action store.
func NewActionStore() *ActionStore {
	return &ActionStore{
		actions: make([]AutonomousAction, 0),
	}
}

// Add records a new autonomous action.
func (s *ActionStore) Add(_ context.Context, action AutonomousAction) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.actions = append(s.actions, action)
}

// Get retrieves an action by ID.
func (s *ActionStore) Get(_ context.Context, actionID string) (AutonomousAction, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	for _, a := range s.actions {
		if a.ID == actionID {
			return a, true
		}
	}
	return AutonomousAction{}, false
}

// Recent returns the most recent actions, limited by count and optionally filtered by since time.
func (s *ActionStore) Recent(_ context.Context, limit int, since time.Time) []AutonomousAction {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var filtered []AutonomousAction
	for _, a := range s.actions {
		if !since.IsZero() && a.Timestamp.Before(since) {
			continue
		}
		filtered = append(filtered, a)
	}

	// Return the most recent entries up to limit.
	if limit > 0 && len(filtered) > limit {
		filtered = filtered[len(filtered)-limit:]
	}
	return filtered
}

// UpdateOutcome updates the outcome and override_by fields for an action.
func (s *ActionStore) UpdateOutcome(_ context.Context, actionID, outcome, operatorID string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	for i := range s.actions {
		if s.actions[i].ID == actionID {
			s.actions[i].Outcome = outcome
			s.actions[i].OverrideBy = operatorID
			return
		}
	}
}
