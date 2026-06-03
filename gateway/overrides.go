package gateway

import (
	"context"
	"fmt"
	"sync"
)

// OverrideStore manages operator override state and paused modules.
type OverrideStore struct {
	mu            sync.RWMutex
	pausedModules map[string]string // module ID → operator ID who paused it
	audit         *AuditStore
	actions       *ActionStore
}

// NewOverrideStore creates a new override store wired to the audit and action stores.
func NewOverrideStore(audit *AuditStore, actions *ActionStore) *OverrideStore {
	return &OverrideStore{
		pausedModules: make(map[string]string),
		audit:         audit,
		actions:       actions,
	}
}

// ApproveAction approves a pending action and records the override in the audit trail.
func (s *OverrideStore) ApproveAction(ctx context.Context, actionID, operatorID string) error {
	action, ok := s.actions.Get(ctx, actionID)
	if !ok {
		return fmt.Errorf("action %q not found", actionID)
	}

	s.actions.UpdateOutcome(ctx, actionID, "success", operatorID)

	req := OverrideRequest{
		ActionID:   actionID,
		ModuleID:   action.Module,
		Operation:  "approve",
		OperatorID: operatorID,
	}
	s.audit.RecordOverride(ctx, req, "approved")
	return nil
}

// RejectAction cancels a pending action and records the override in the audit trail.
func (s *OverrideStore) RejectAction(ctx context.Context, actionID, operatorID string) error {
	action, ok := s.actions.Get(ctx, actionID)
	if !ok {
		return fmt.Errorf("action %q not found", actionID)
	}

	s.actions.UpdateOutcome(ctx, actionID, "rejected", operatorID)

	req := OverrideRequest{
		ActionID:   actionID,
		ModuleID:   action.Module,
		Operation:  "reject",
		OperatorID: operatorID,
	}
	s.audit.RecordOverride(ctx, req, "rejected")
	return nil
}

// PauseModule prevents autonomous actions for the specified module until resumed.
func (s *OverrideStore) PauseModule(ctx context.Context, moduleID, operatorID string) error {
	s.mu.Lock()
	s.pausedModules[moduleID] = operatorID
	s.mu.Unlock()

	req := OverrideRequest{
		ModuleID:   moduleID,
		Operation:  "pause",
		OperatorID: operatorID,
	}
	s.audit.RecordOverride(ctx, req, "paused")
	return nil
}

// ResumeModule re-enables autonomous actions for the specified module.
func (s *OverrideStore) ResumeModule(ctx context.Context, moduleID, operatorID string) error {
	s.mu.Lock()
	delete(s.pausedModules, moduleID)
	s.mu.Unlock()

	req := OverrideRequest{
		ModuleID:   moduleID,
		Operation:  "resume",
		OperatorID: operatorID,
	}
	s.audit.RecordOverride(ctx, req, "resumed")
	return nil
}

// IsModulePaused returns true if the specified module is currently paused.
func (s *OverrideStore) IsModulePaused(_ context.Context, moduleID string) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	_, paused := s.pausedModules[moduleID]
	return paused
}
