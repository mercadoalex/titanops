package gateway

import (
	"context"
	"sync"
	"time"
)

// CorrelationStore provides thread-safe in-memory storage for correlated incidents.
type CorrelationStore struct {
	mu        sync.RWMutex
	incidents []CorrelatedIncident
}

// NewCorrelationStore creates a new empty correlation store.
func NewCorrelationStore() *CorrelationStore {
	return &CorrelationStore{
		incidents: make([]CorrelatedIncident, 0),
	}
}

// Add records a new correlated incident.
func (s *CorrelationStore) Add(_ context.Context, incident CorrelatedIncident) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.incidents = append(s.incidents, incident)
}

// Since returns all incidents detected after the given time.
func (s *CorrelationStore) Since(_ context.Context, since time.Time) []CorrelatedIncident {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var result []CorrelatedIncident
	for _, inc := range s.incidents {
		if inc.DetectedAt.After(since) || inc.DetectedAt.Equal(since) {
			result = append(result, inc)
		}
	}
	return result
}
