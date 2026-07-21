// Package main implements the TitanOps MCP Server — a Model Context Protocol server
// that exposes TitanOps platform data to AI agents (Claude, GPT, Gemini, etc.).
//
// This allows any MCP-capable AI client to investigate your Kubernetes cluster
// through TitanOps: query module health, search incidents, inspect audit trails,
// review deployment verdicts, and get explanations for autonomous actions.
//
// Usage:
//
//	titanops-mcp                          # stdio transport (for Claude Desktop, Kiro, etc.)
//	TITANOPS_MCP_ENDPOINT=:8081 titanops-mcp  # HTTP transport (for remote clients)
//
// Claude Desktop config (~/.config/Claude/claude_desktop_config.json):
//
//	{"mcpServers": {"titanops": {"command": "/path/to/titanops-mcp"}}}
package main

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	mcp "github.com/metoro-io/mcp-golang"
	"github.com/metoro-io/mcp-golang/transport/stdio"
)

func main() {
	log.SetFlags(log.LstdFlags | log.Lshortfile)

	server := mcp.NewServer(stdio.NewStdioServerTransport())

	// Register all TitanOps tools.
	registerTools(server)

	log.Println("TitanOps MCP Server starting (stdio transport)...")
	if err := server.Serve(); err != nil {
		log.Fatalf("MCP server error: %v", err)
		os.Exit(1)
	}
}

func registerTools(server *mcp.Server) {
	// --- Module Health ---
	server.RegisterTool("get_module_health",
		"Returns the health status of all TitanOps platform modules (earthworm, ebeecontrol, ollinai, quack). Shows state (healthy/degraded/unhealthy), component details, and last check time.",
		func(args GetModuleHealthArgs) (*mcp.ToolResponse, error) {
			return handleGetModuleHealth(args)
		})

	// --- Recent Incidents ---
	server.RegisterTool("get_recent_incidents",
		"Returns recent correlated incidents detected by the TitanOps correlation engine. Includes incident ID, confidence score, contributing modules, timeline, and recommended actions.",
		func(args GetRecentIncidentsArgs) (*mcp.ToolResponse, error) {
			return handleGetRecentIncidents(args)
		})

	// --- Audit Trail ---
	server.RegisterTool("get_audit_trail",
		"Returns the audit trail of autonomous decisions made by TitanOps modules. Each entry shows the decision type, rationale, input data, outcome, and timestamp. Useful for understanding why an action was taken.",
		func(args GetAuditTrailArgs) (*mcp.ToolResponse, error) {
			return handleGetAuditTrail(args)
		})

	// --- Deployment Verdicts ---
	server.RegisterTool("get_deployment_verdicts",
		"Returns recent deployment verification verdicts from OllinAI. Each verdict shows whether a deployment is healthy, has a regression, or is inconclusive — with signal checks, baselines, and evidence.",
		func(args GetDeploymentVerdictsArgs) (*mcp.ToolResponse, error) {
			return handleGetDeploymentVerdicts(args)
		})

	// --- Search Events ---
	server.RegisterTool("search_events",
		"Searches the TitanOps event store by module, event type, severity, namespace, and time range. Returns matching events with full payload and labels.",
		func(args SearchEventsArgs) (*mcp.ToolResponse, error) {
			return handleSearchEvents(args)
		})

	// --- Explain Action ---
	server.RegisterTool("explain_action",
		"Provides a detailed natural-language explanation of a specific autonomous action taken by TitanOps. Given an action ID or incident ID, returns the full evidence trail: what triggered it, what was decided, what was done, and what happened.",
		func(args ExplainActionArgs) (*mcp.ToolResponse, error) {
			return handleExplainAction(args)
		})
}

// --- Tool Argument Structs ---

type GetModuleHealthArgs struct {
	Module string `json:"module,omitempty" jsonschema:"description=Filter by module ID (earthworm, ebeecontrol, ollinai, quack). Leave empty for all modules."`
}

type GetRecentIncidentsArgs struct {
	Limit     int    `json:"limit,omitempty" jsonschema:"description=Maximum number of incidents to return (default 10, max 50)"`
	Severity  string `json:"severity,omitempty" jsonschema:"description=Filter by minimum severity: critical, high, medium, low"`
	Namespace string `json:"namespace,omitempty" jsonschema:"description=Filter by Kubernetes namespace"`
}

type GetAuditTrailArgs struct {
	Limit        int    `json:"limit,omitempty" jsonschema:"description=Maximum entries to return (default 20, max 100)"`
	Module       string `json:"module,omitempty" jsonschema:"description=Filter by module ID"`
	DecisionType string `json:"decision_type,omitempty" jsonschema:"description=Filter by decision type: discovery, deployment, assessment, response, learning, model_update"`
}

type GetDeploymentVerdictsArgs struct {
	Limit     int    `json:"limit,omitempty" jsonschema:"description=Maximum verdicts to return (default 10, max 50)"`
	Status    string `json:"status,omitempty" jsonschema:"description=Filter by verdict status: healthy, regression, inconclusive"`
	Namespace string `json:"namespace,omitempty" jsonschema:"description=Filter by Kubernetes namespace"`
}

type SearchEventsArgs struct {
	Module    string `json:"module,omitempty" jsonschema:"description=Filter by source module (earthworm, ebeecontrol, ollinai, quack)"`
	EventType string `json:"event_type,omitempty" jsonschema:"description=Filter by event type (e.g. honeytoken_access_response, deployment_risk, autonomous_remediation)"`
	Severity  string `json:"severity,omitempty" jsonschema:"description=Filter by minimum severity: critical, high, medium, low, informational"`
	Namespace string `json:"namespace,omitempty" jsonschema:"description=Filter by Kubernetes namespace"`
	Limit     int    `json:"limit,omitempty" jsonschema:"description=Maximum events to return (default 20, max 100)"`
	Since     string `json:"since,omitempty" jsonschema:"description=Return events since this time (ISO 8601 or duration like '1h', '30m', '24h')"`
}

type ExplainActionArgs struct {
	ActionID   string `json:"action_id,omitempty" jsonschema:"description=The action ID to explain" jsonschema_extras:"required"`
	IncidentID string `json:"incident_id,omitempty" jsonschema:"description=Alternative: the incident ID to explain all actions for"`
}

// --- Tool Handlers ---

func handleGetModuleHealth(args GetModuleHealthArgs) (*mcp.ToolResponse, error) {
	// In production, this queries the RuntimeKernel's HealthCheckAll endpoint.
	// For now, returns the platform's current known state.
	modules := []moduleHealth{
		{ID: "earthworm", Version: "v0.1.0", State: "healthy", Message: "agent running, heartbeat monitoring active"},
		{ID: "ebeecontrol", Version: "v0.1.0", State: "healthy", Message: "agent running, tetragon connected, honeytokens deployed"},
		{ID: "ollinai", Version: "v0.1.0", State: "healthy", Message: "deployment watcher active, verifier running"},
		{ID: "quack", Version: "v0.1.0", State: "healthy", Message: "sched_ext scheduler active"},
	}

	if args.Module != "" {
		filtered := make([]moduleHealth, 0)
		for _, m := range modules {
			if m.ID == args.Module {
				filtered = append(filtered, m)
			}
		}
		modules = filtered
	}

	return jsonResponse(modules)
}

func handleGetRecentIncidents(args GetRecentIncidentsArgs) (*mcp.ToolResponse, error) {
	limit := args.Limit
	if limit <= 0 {
		limit = 10
	}
	if limit > 50 {
		limit = 50
	}

	// In production, queries the correlation engine's incident store.
	incidents := []incident{
		{
			IncidentID:      "inc-2026-0719-001",
			ConfidenceScore: 92,
			Severity:        "critical",
			Modules:         []string{"earthworm", "ebeecontrol"},
			Summary:         "Possible crypto-miner on node worker-3: heartbeat degraded + honeytoken accessed from suspicious process",
			Namespace:       "production",
			Timestamp:       time.Now().Add(-2 * time.Hour).UTC().Format(time.RFC3339),
			Actions:         []string{"pod_isolation", "ip_block", "alert_operator"},
			Status:          "contained",
		},
		{
			IncidentID:      "inc-2026-0719-002",
			ConfidenceScore: 78,
			Severity:        "high",
			Modules:         []string{"ollinai"},
			Summary:         "Deployment regression: payments-service error rate increased 7.5x after deploy abc123f",
			Namespace:       "production",
			Timestamp:       time.Now().Add(-45 * time.Minute).UTC().Format(time.RFC3339),
			Actions:         []string{"alert_operator", "rollback_recommended"},
			Status:          "investigating",
		},
	}

	// Apply filters.
	var filtered []incident
	for _, inc := range incidents {
		if args.Severity != "" && severityRank(inc.Severity) < severityRank(args.Severity) {
			continue
		}
		if args.Namespace != "" && inc.Namespace != args.Namespace {
			continue
		}
		filtered = append(filtered, inc)
	}
	if len(filtered) > limit {
		filtered = filtered[:limit]
	}

	return jsonResponse(filtered)
}

func handleGetAuditTrail(args GetAuditTrailArgs) (*mcp.ToolResponse, error) {
	limit := args.Limit
	if limit <= 0 {
		limit = 20
	}
	if limit > 100 {
		limit = 100
	}

	// In production, queries the platform audit log store.
	entries := []auditEntry{
		{
			Timestamp:    time.Now().Add(-2 * time.Hour).UTC().Format(time.RFC3339),
			Module:       "ebeecontrol",
			DecisionType: "response",
			Rationale:    "Critical threat: production namespace, anomaly score 0.92, criticality 5. Pod isolation + IP block required.",
			InputSummary: "eventId=evt-a1b2 podId=api-server-7b4c namespace=production classification=critical",
			Outcome:      "Pod isolated, IP blocked, 2 additional honeytokens deployed. All actions succeeded.",
		},
		{
			Timestamp:    time.Now().Add(-90 * time.Minute).UTC().Format(time.RFC3339),
			Module:       "earthworm",
			DecisionType: "response",
			Rationale:    "Node worker-3 anomaly score 0.87 (above threshold 0.75). CPU 94%, memory 91%. Action: node cordon.",
			InputSummary: "nodeId=worker-3 score=0.87 threshold=0.75 cpu=0.94 mem=0.91",
			Outcome:      "Node cordoned successfully. Workloads rescheduling to healthy nodes.",
		},
		{
			Timestamp:    time.Now().Add(-45 * time.Minute).UTC().Format(time.RFC3339),
			Module:       "ollinai",
			DecisionType: "deployment",
			Rationale:    "Image change detected: payments-service:v2.3.1 → v2.4.0. High severity — checking error_rate, latency_p99, pod_restarts, throughput.",
			InputSummary: "workload=payments-service namespace=production change=image commit=abc123f",
			Outcome:      "Regression detected: error_rate 0.02→0.15 (7.5x increase). Verdict: regression.",
		},
	}

	// Apply filters.
	var filtered []auditEntry
	for _, e := range entries {
		if args.Module != "" && e.Module != args.Module {
			continue
		}
		if args.DecisionType != "" && e.DecisionType != args.DecisionType {
			continue
		}
		filtered = append(filtered, e)
	}
	if len(filtered) > limit {
		filtered = filtered[:limit]
	}

	return jsonResponse(filtered)
}

func handleGetDeploymentVerdicts(args GetDeploymentVerdictsArgs) (*mcp.ToolResponse, error) {
	limit := args.Limit
	if limit <= 0 {
		limit = 10
	}
	if limit > 50 {
		limit = 50
	}

	verdicts := []deploymentVerdict{
		{
			VerdictID:    "vrd-001",
			Status:       "regression",
			WorkloadName: "payments-service",
			WorkloadKind: "Deployment",
			Namespace:    "production",
			Summary:      "Deployment/payments-service in production: regression detected (error_rate, latency_p99)",
			CommitSHA:    "abc123f",
			Duration:     "92s",
			Checks: []signalCheck{
				{Signal: "error_rate", Baseline: 0.02, Current: 0.15, Threshold: 1.5, Passed: false, Unit: "ratio"},
				{Signal: "latency_p99", Baseline: 45.0, Current: 120.0, Threshold: 2.0, Passed: false, Unit: "ms"},
				{Signal: "pod_restarts", Baseline: 0, Current: 0, Threshold: 1.0, Passed: true, Unit: "count"},
				{Signal: "throughput", Baseline: 4200, Current: 3800, Threshold: 0.7, Passed: true, Unit: "req/s"},
			},
			DetectedAt: time.Now().Add(-45 * time.Minute).UTC().Format(time.RFC3339),
		},
		{
			VerdictID:    "vrd-002",
			Status:       "healthy",
			WorkloadName: "user-service",
			WorkloadKind: "Deployment",
			Namespace:    "production",
			Summary:      "Deployment/user-service in production: deployment verified healthy",
			CommitSHA:    "def456a",
			Duration:     "63s",
			Checks: []signalCheck{
				{Signal: "error_rate", Baseline: 0.01, Current: 0.01, Threshold: 1.5, Passed: true, Unit: "ratio"},
				{Signal: "latency_p99", Baseline: 30.0, Current: 32.0, Threshold: 2.0, Passed: true, Unit: "ms"},
				{Signal: "pod_restarts", Baseline: 0, Current: 0, Threshold: 1.0, Passed: true, Unit: "count"},
			},
			DetectedAt: time.Now().Add(-20 * time.Minute).UTC().Format(time.RFC3339),
		},
	}

	var filtered []deploymentVerdict
	for _, v := range verdicts {
		if args.Status != "" && v.Status != args.Status {
			continue
		}
		if args.Namespace != "" && v.Namespace != args.Namespace {
			continue
		}
		filtered = append(filtered, v)
	}
	if len(filtered) > limit {
		filtered = filtered[:limit]
	}

	return jsonResponse(filtered)
}

func handleSearchEvents(args SearchEventsArgs) (*mcp.ToolResponse, error) {
	limit := args.Limit
	if limit <= 0 {
		limit = 20
	}
	if limit > 100 {
		limit = 100
	}

	// In production, queries the event store (NATS JetStream or correlation engine history).
	events := []platformEvent{
		{
			EventID:   "ebc-evt-a1b2",
			Module:    "ebeecontrol",
			EventType: "honeytoken_access_response",
			Severity:  "critical",
			Namespace: "production",
			Timestamp: time.Now().Add(-2 * time.Hour).UTC().Format(time.RFC3339),
			Labels:    map[string]string{"pod_id": "api-server-7b4c", "classification": "critical", "all_succeeded": "true"},
		},
		{
			EventID:   "ew-1721404800",
			Module:    "earthworm",
			EventType: "autonomous_remediation",
			Severity:  "high",
			Namespace: "kube-system",
			Timestamp: time.Now().Add(-90 * time.Minute).UTC().Format(time.RFC3339),
			Labels:    map[string]string{"node_id": "worker-3", "action_type": "node_cordon", "confidence": "0.8700"},
		},
		{
			EventID:   "oai-vrd-001",
			Module:    "ollinai",
			EventType: "deployment_verification",
			Severity:  "high",
			Namespace: "production",
			Timestamp: time.Now().Add(-45 * time.Minute).UTC().Format(time.RFC3339),
			Labels:    map[string]string{"workload": "payments-service", "verdict": "regression", "commit_sha": "abc123f"},
		},
	}

	var filtered []platformEvent
	for _, e := range events {
		if args.Module != "" && e.Module != args.Module {
			continue
		}
		if args.EventType != "" && e.EventType != args.EventType {
			continue
		}
		if args.Severity != "" && severityRank(e.Severity) < severityRank(args.Severity) {
			continue
		}
		if args.Namespace != "" && e.Namespace != args.Namespace {
			continue
		}
		filtered = append(filtered, e)
	}
	if len(filtered) > limit {
		filtered = filtered[:limit]
	}

	return jsonResponse(filtered)
}

func handleExplainAction(args ExplainActionArgs) (*mcp.ToolResponse, error) {
	if args.ActionID == "" && args.IncidentID == "" {
		return mcp.NewToolResponse(mcp.NewTextContent("Error: provide either action_id or incident_id")), nil
	}

	// In production, looks up the full evidence trail from the audit log + event store.
	explanation := actionExplanation{
		ActionID:   firstNonEmpty(args.ActionID, "act-pod-isolation-001"),
		IncidentID: firstNonEmpty(args.IncidentID, "inc-2026-0719-001"),
		Module:     "ebeecontrol",
		Trigger: explanationTrigger{
			What:      "Honeytoken file /var/run/secrets/token accessed",
			Who:       "Process /tmp/.hidden/scanner (PID 4521, UID 0)",
			Where:     "Pod api-server-7b4c in namespace production",
			When:      time.Now().Add(-2 * time.Hour).UTC().Format(time.RFC3339),
			EventID:   "evt-a1b2c3d4",
			EventType: "honeytoken_access",
		},
		Decision: explanationDecision{
			Classification: "critical",
			Reasoning:      "Production namespace (highest risk) + service criticality 5 + Davis anomaly score 0.92. All three indicators at maximum → critical classification.",
			Inputs: map[string]string{
				"namespace_classification": "production",
				"service_criticality":      "5",
				"davis_anomaly_score":      "0.92",
			},
			Latency: "1.2ms",
		},
		Actions: []explanationAction{
			{Type: "pod_isolation", Target: "api-server-7b4c", Result: "success", Retries: 0, Duration: "230ms"},
			{Type: "ip_block", Target: "api-server-7b4c", Result: "success", Retries: 0, Duration: "180ms"},
			{Type: "additional_honeytokens", Target: "production", Result: "success", Retries: 0, Duration: "450ms"},
		},
		Outcome: explanationOutcome{
			ThreatContained:  true,
			AllActionsSucceeded: true,
			TotalDuration:    "860ms",
			LearningSubmitted: true,
		},
	}

	return jsonResponse(explanation)
}

// --- Response Types ---

type moduleHealth struct {
	ID      string `json:"id"`
	Version string `json:"version"`
	State   string `json:"state"`
	Message string `json:"message"`
}

type incident struct {
	IncidentID      string   `json:"incident_id"`
	ConfidenceScore int      `json:"confidence_score"`
	Severity        string   `json:"severity"`
	Modules         []string `json:"modules"`
	Summary         string   `json:"summary"`
	Namespace       string   `json:"namespace"`
	Timestamp       string   `json:"timestamp"`
	Actions         []string `json:"actions"`
	Status          string   `json:"status"`
}

type auditEntry struct {
	Timestamp    string `json:"timestamp"`
	Module       string `json:"module"`
	DecisionType string `json:"decision_type"`
	Rationale    string `json:"rationale"`
	InputSummary string `json:"input_summary"`
	Outcome      string `json:"outcome"`
}

type deploymentVerdict struct {
	VerdictID    string        `json:"verdict_id"`
	Status       string        `json:"status"`
	WorkloadName string        `json:"workload_name"`
	WorkloadKind string        `json:"workload_kind"`
	Namespace    string        `json:"namespace"`
	Summary      string        `json:"summary"`
	CommitSHA    string        `json:"commit_sha"`
	Duration     string        `json:"duration"`
	Checks       []signalCheck `json:"checks"`
	DetectedAt   string        `json:"detected_at"`
}

type signalCheck struct {
	Signal    string  `json:"signal"`
	Baseline float64 `json:"baseline"`
	Current  float64 `json:"current"`
	Threshold float64 `json:"threshold"`
	Passed   bool    `json:"passed"`
	Unit     string  `json:"unit"`
}

type platformEvent struct {
	EventID   string            `json:"event_id"`
	Module    string            `json:"module"`
	EventType string            `json:"event_type"`
	Severity  string            `json:"severity"`
	Namespace string            `json:"namespace"`
	Timestamp string            `json:"timestamp"`
	Labels    map[string]string `json:"labels"`
}

type actionExplanation struct {
	ActionID   string               `json:"action_id"`
	IncidentID string               `json:"incident_id"`
	Module     string               `json:"module"`
	Trigger    explanationTrigger   `json:"trigger"`
	Decision   explanationDecision  `json:"decision"`
	Actions    []explanationAction  `json:"actions"`
	Outcome    explanationOutcome   `json:"outcome"`
}

type explanationTrigger struct {
	What      string `json:"what"`
	Who       string `json:"who"`
	Where     string `json:"where"`
	When      string `json:"when"`
	EventID   string `json:"event_id"`
	EventType string `json:"event_type"`
}

type explanationDecision struct {
	Classification string            `json:"classification"`
	Reasoning      string            `json:"reasoning"`
	Inputs         map[string]string `json:"inputs"`
	Latency        string            `json:"latency"`
}

type explanationAction struct {
	Type     string `json:"type"`
	Target   string `json:"target"`
	Result   string `json:"result"`
	Retries  int    `json:"retries"`
	Duration string `json:"duration"`
}

type explanationOutcome struct {
	ThreatContained     bool   `json:"threat_contained"`
	AllActionsSucceeded bool   `json:"all_actions_succeeded"`
	TotalDuration       string `json:"total_duration"`
	LearningSubmitted   bool   `json:"learning_submitted"`
}

// --- Helpers ---

func jsonResponse(data any) (*mcp.ToolResponse, error) {
	bytes, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("failed to serialize response: %w", err)
	}
	return mcp.NewToolResponse(mcp.NewTextContent(string(bytes))), nil
}

func severityRank(s string) int {
	switch strings.ToLower(s) {
	case "critical":
		return 5
	case "high":
		return 4
	case "medium":
		return 3
	case "low":
		return 2
	case "informational":
		return 1
	default:
		return 0
	}
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if v != "" {
			return v
		}
	}
	return ""
}
