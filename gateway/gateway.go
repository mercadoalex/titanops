// Package gateway implements the TitanOps API gateway serving
// REST endpoints for the dashboard and platform management.
package gateway

import "net/http"

// Gateway is the main struct that wires together all gateway components.
type Gateway struct {
	health       *HealthStore
	actions      *ActionStore
	audit        *AuditStore
	overrides    *OverrideStore
	correlations *CorrelationStore
}

// NewGateway creates a new Gateway with initialized stores.
func NewGateway() *Gateway {
	health := NewHealthStore()
	actions := NewActionStore()
	audit := NewAuditStore()
	overrides := NewOverrideStore(audit, actions)
	correlations := NewCorrelationStore()

	return &Gateway{
		health:       health,
		actions:      actions,
		audit:        audit,
		overrides:    overrides,
		correlations: correlations,
	}
}

// RegisterRoutes registers all API endpoint handlers on the given mux.
func (g *Gateway) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/api/health", g.handleHealth)
	mux.HandleFunc("/api/actions", g.handleActions)
	mux.HandleFunc("/api/correlations", g.handleCorrelations)
	mux.HandleFunc("/api/overrides", g.handleOverrides)
	mux.HandleFunc("/api/audit", g.handleAudit)
	mux.HandleFunc("/api/explain/", g.handleExplain)
}

// Health returns the health store for direct access (e.g., from other services).
func (g *Gateway) Health() *HealthStore {
	return g.health
}

// Actions returns the action store for direct access.
func (g *Gateway) Actions() *ActionStore {
	return g.actions
}

// Audit returns the audit store for direct access.
func (g *Gateway) Audit() *AuditStore {
	return g.audit
}

// Overrides returns the override store for direct access.
func (g *Gateway) Overrides() *OverrideStore {
	return g.overrides
}

// Correlations returns the correlation store for direct access.
func (g *Gateway) Correlations() *CorrelationStore {
	return g.correlations
}
