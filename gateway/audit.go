package gateway

import (
	"context"
	"sync"
	"time"
)

// AuditStore provides thread-safe in-memory storage for audit trail entries.
type AuditStore struct {
	mu      sync.RWMutex
	entries []AuditEntry
}

// NewAuditStore creates a new empty audit store.
func NewAuditStore() *AuditStore {
	return &AuditStore{
		entries: make([]AuditEntry, 0),
	}
}

// RecordAction records an autonomous action in the audit trail.
func (s *AuditStore) RecordAction(_ context.Context, action AutonomousAction) {
	entry := AuditEntry{
		Timestamp:    action.Timestamp,
		Module:       action.Module,
		ActionType:   action.ActionType,
		TriggerEvent: action.Reasoning.Observation,
		Confidence:   action.Confidence,
		Reasoning:    action.Reasoning.Analysis,
		Outcome:      action.Outcome,
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	s.entries = append(s.entries, entry)
}

// RecordOverride records an operator override in the audit trail.
func (s *AuditStore) RecordOverride(_ context.Context, req OverrideRequest, outcome string) {
	entry := AuditEntry{
		Timestamp:  time.Now().UTC(),
		Module:     req.ModuleID,
		ActionType: req.Operation,
		Confidence: 0,
		Reasoning:  "operator override",
		Outcome:    outcome,
		OperatorID: req.OperatorID,
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	s.entries = append(s.entries, entry)
}

// Query returns audit entries matching the given filter.
func (s *AuditStore) Query(_ context.Context, filter AuditFilter) []AuditEntry {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var result []AuditEntry
	for _, entry := range s.entries {
		if !filter.Since.IsZero() && entry.Timestamp.Before(filter.Since) {
			continue
		}
		if filter.Module != "" && entry.Module != filter.Module {
			continue
		}
		if filter.ActionType != "" && entry.ActionType != filter.ActionType {
			continue
		}
		result = append(result, entry)
	}
	return result
}

// All returns all audit entries.
func (s *AuditStore) All(_ context.Context) []AuditEntry {
	s.mu.RLock()
	defer s.mu.RUnlock()

	result := make([]AuditEntry, len(s.entries))
	copy(result, s.entries)
	return result
}
