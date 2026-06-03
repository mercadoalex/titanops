package gateway

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// handleHealth returns the health status of all TitanOps modules.
// GET /api/health
func (g *Gateway) handleHealth(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	modules := g.health.All(r.Context())
	writeJSON(w, http.StatusOK, modules)
}

// handleActions returns recent autonomous action entries.
// GET /api/actions?limit=50&since=2024-01-01T00:00:00Z
func (g *Gateway) handleActions(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	limit := 50
	if l := r.URL.Query().Get("limit"); l != "" {
		if parsed, err := strconv.Atoi(l); err == nil && parsed > 0 {
			limit = parsed
		}
	}

	var since time.Time
	if s := r.URL.Query().Get("since"); s != "" {
		if parsed, err := time.Parse(time.RFC3339, s); err == nil {
			since = parsed
		}
	}

	actions := g.actions.Recent(r.Context(), limit, since)
	if actions == nil {
		actions = []AutonomousAction{}
	}
	writeJSON(w, http.StatusOK, actions)
}

// handleCorrelations returns the correlated incident timeline.
// GET /api/correlations?window=60m
func (g *Gateway) handleCorrelations(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	window := 60 * time.Minute
	if ws := r.URL.Query().Get("window"); ws != "" {
		if parsed, err := time.ParseDuration(ws); err == nil && parsed > 0 {
			window = parsed
		}
	}

	incidents := g.correlations.Since(r.Context(), time.Now().Add(-window))
	if incidents == nil {
		incidents = []CorrelatedIncident{}
	}
	writeJSON(w, http.StatusOK, incidents)
}

// handleOverrides processes operator override operations.
// POST /api/overrides
func (g *Gateway) handleOverrides(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req OverrideRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}

	if req.OperatorID == "" {
		http.Error(w, "operator_id is required", http.StatusBadRequest)
		return
	}

	var err error
	ctx := r.Context()

	switch req.Operation {
	case "approve":
		if req.ActionID == "" {
			http.Error(w, "action_id is required for approve", http.StatusBadRequest)
			return
		}
		err = g.overrides.ApproveAction(ctx, req.ActionID, req.OperatorID)
	case "reject":
		if req.ActionID == "" {
			http.Error(w, "action_id is required for reject", http.StatusBadRequest)
			return
		}
		err = g.overrides.RejectAction(ctx, req.ActionID, req.OperatorID)
	case "pause":
		if req.ModuleID == "" {
			http.Error(w, "module_id is required for pause", http.StatusBadRequest)
			return
		}
		err = g.overrides.PauseModule(ctx, req.ModuleID, req.OperatorID)
	case "resume":
		if req.ModuleID == "" {
			http.Error(w, "module_id is required for resume", http.StatusBadRequest)
			return
		}
		err = g.overrides.ResumeModule(ctx, req.ModuleID, req.OperatorID)
	default:
		http.Error(w, "invalid operation: must be approve, reject, pause, or resume", http.StatusBadRequest)
		return
	}

	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{
		"status":    "ok",
		"operation": req.Operation,
	})
}

// handleAudit returns audit entries with optional filtering.
// GET /api/audit?since=2024-01-01T00:00:00Z&module=earthworm&action_type=approve
func (g *Gateway) handleAudit(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	filter := AuditFilter{}

	if s := r.URL.Query().Get("since"); s != "" {
		if parsed, err := time.Parse(time.RFC3339, s); err == nil {
			filter.Since = parsed
		}
	}
	if m := r.URL.Query().Get("module"); m != "" {
		filter.Module = m
	}
	if a := r.URL.Query().Get("action_type"); a != "" {
		filter.ActionType = a
	}

	entries := g.audit.Query(r.Context(), filter)
	if entries == nil {
		entries = []AuditEntry{}
	}
	writeJSON(w, http.StatusOK, entries)
}

// handleExplain returns explainability details for a specific action.
// GET /api/explain/{actionID}
func (g *Gateway) handleExplain(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Extract actionID from the URL path.
	path := r.URL.Path
	prefix := "/api/explain/"
	if !strings.HasPrefix(path, prefix) {
		http.Error(w, "invalid path", http.StatusBadRequest)
		return
	}
	actionID := strings.TrimPrefix(path, prefix)
	if actionID == "" {
		http.Error(w, "action ID is required", http.StatusBadRequest)
		return
	}

	action, ok := g.actions.Get(r.Context(), actionID)
	if !ok {
		http.Error(w, "action not found", http.StatusNotFound)
		return
	}

	// Return explainability details including confidence and reasoning chain.
	response := map[string]interface{}{
		"action_id":  action.ID,
		"module":     action.Module,
		"confidence": action.Confidence,
		"reasoning":  action.Reasoning,
		"outcome":    action.Outcome,
		"timestamp":  action.Timestamp,
	}
	writeJSON(w, http.StatusOK, response)
}

// writeJSON writes a JSON response with the given status code.
func writeJSON(w http.ResponseWriter, status int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(data)
}
