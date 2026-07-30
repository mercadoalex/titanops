// Package alertmanager implements a webhook receiver that converts
// Prometheus AlertManager alerts into TitanOps export.Event format
// for ingestion into the correlation engine.
package alertmanager

import (
	"encoding/json"
	"strings"
	"time"

	"github.com/google/uuid"

	export "github.com/mercadoalex/titanops/shared/titanops-export"
)

// AlertManagerPayload is the top-level JSON structure sent by AlertManager
// when it fires a webhook notification.
type AlertManagerPayload struct {
	// Version is the webhook payload version (e.g., "4").
	Version string `json:"version"`
	// Receiver is the AlertManager receiver name that triggered this webhook.
	Receiver string `json:"receiver"`
	// Status is the group status: "firing" or "resolved".
	Status string `json:"status"`
	// Alerts contains the individual alert instances.
	Alerts []Alert `json:"alerts"`
	// GroupLabels are the labels used to group alerts together.
	GroupLabels map[string]string `json:"groupLabels"`
	// CommonLabels are labels shared by all alerts in the group.
	CommonLabels map[string]string `json:"commonLabels"`
	// CommonAnnotations are annotations shared by all alerts in the group.
	CommonAnnotations map[string]string `json:"commonAnnotations"`
	// ExternalURL is the URL of the AlertManager instance.
	ExternalURL string `json:"externalURL"`
	// GroupKey identifies the alert group.
	GroupKey string `json:"groupKey"`
}

// Alert represents a single alert instance within an AlertManager webhook payload.
type Alert struct {
	// Status is "firing" or "resolved".
	Status string `json:"status"`
	// Labels contains all alert labels (alertname, severity, namespace, pod, node, etc).
	Labels map[string]string `json:"labels"`
	// Annotations contains descriptive metadata (summary, description, runbook_url, etc).
	Annotations map[string]string `json:"annotations"`
	// StartsAt is when the alert started firing.
	StartsAt time.Time `json:"startsAt"`
	// EndsAt is when the alert was resolved (zero value if still firing).
	EndsAt time.Time `json:"endsAt"`
	// GeneratorURL links back to the Prometheus query that generated the alert.
	GeneratorURL string `json:"generatorURL"`
	// Fingerprint is a unique hash identifying this alert instance.
	Fingerprint string `json:"fingerprint"`
}

// ModuleID is the module identifier used for events originating from AlertManager.
const ModuleID = "alertmanager"

// MapAlertToEvent converts a single AlertManager Alert into a TitanOps export.Event.
//
// Mapping rules:
//   - Module: always "alertmanager"
//   - EventType: labels["alertname"] (lowercased, spaces replaced with underscores)
//   - Severity: from labels["severity"] if present, otherwise inferred from alert status
//   - Namespace: from labels["namespace"] if present
//   - Node: from labels["node"] or labels["instance"] (hostname portion)
//   - Pod: from labels["pod"] or labels["pod_name"] if present
//   - Timestamp: alert's StartsAt
//   - Payload: full alert JSON
//   - Labels: all alert labels + annotations merged, plus receiver metadata
func MapAlertToEvent(alert Alert, receiver string) (export.Event, error) {
	// Build event type from alertname.
	eventType := normalizeEventType(alert.Labels["alertname"])
	if eventType == "" {
		eventType = "unknown_alert"
	}

	// Determine severity.
	severity := mapSeverity(alert.Labels["severity"], alert.Status)

	// Extract K8s metadata from labels.
	namespace := firstNonEmpty(alert.Labels["namespace"], alert.Labels["exported_namespace"])
	node := extractNode(alert.Labels)
	pod := firstNonEmpty(alert.Labels["pod"], alert.Labels["pod_name"])

	// Timestamp: use StartsAt if available, otherwise now.
	timestamp := alert.StartsAt
	if timestamp.IsZero() {
		timestamp = time.Now().UTC()
	}

	// Build payload as the full alert JSON.
	payload, err := json.Marshal(alert)
	if err != nil {
		return export.Event{}, err
	}

	// Merge labels: alert labels + annotations + metadata.
	mergedLabels := buildLabels(alert, receiver)

	return export.Event{
		Module:    ModuleID,
		EventType: eventType,
		Severity:  severity,
		Namespace: namespace,
		Node:      node,
		Pod:       pod,
		Timestamp: timestamp,
		Payload:   payload,
		EventID:   uuid.New().String(),
		Labels:    mergedLabels,
	}, nil
}

// MapPayloadToEvents converts an entire AlertManager webhook payload into a slice of events.
// Only "firing" alerts are converted by default. Resolved alerts are included if includeResolved is true.
func MapPayloadToEvents(payload AlertManagerPayload, includeResolved bool) ([]export.Event, error) {
	events := make([]export.Event, 0, len(payload.Alerts))

	for _, alert := range payload.Alerts {
		if !includeResolved && alert.Status == "resolved" {
			continue
		}

		event, err := MapAlertToEvent(alert, payload.Receiver)
		if err != nil {
			// Skip individual alerts that fail to map; don't fail the whole batch.
			continue
		}
		events = append(events, event)
	}

	return events, nil
}

// normalizeEventType converts an alertname to a consistent event_type format.
// Example: "HighMemoryUsage" → "high_memory_usage"
func normalizeEventType(alertname string) string {
	if alertname == "" {
		return ""
	}

	// Insert underscores before uppercase letters (camelCase → snake_case).
	var b strings.Builder
	for i, r := range alertname {
		if i > 0 && r >= 'A' && r <= 'Z' {
			prev := rune(alertname[i-1])
			if prev >= 'a' && prev <= 'z' {
				b.WriteRune('_')
			}
		}
		b.WriteRune(r)
	}

	result := strings.ToLower(b.String())
	// Replace spaces and dashes with underscores.
	result = strings.ReplaceAll(result, " ", "_")
	result = strings.ReplaceAll(result, "-", "_")
	return result
}

// mapSeverity converts AlertManager severity label to TitanOps severity.
// Falls back to inferring from alert status if the label is missing.
//
// Mapping:
//
//	"critical", "page"         → "critical"
//	"warning", "high"          → "high"
//	"info", "informational"    → "informational"
//	"medium"                   → "medium"
//	(unknown or empty)         → "high" if firing, "informational" if resolved
func mapSeverity(severityLabel string, status string) string {
	switch strings.ToLower(severityLabel) {
	case "critical", "page", "emergency":
		return "critical"
	case "warning", "high", "error":
		return "high"
	case "medium":
		return "medium"
	case "info", "informational", "low", "none":
		return "informational"
	default:
		// No severity label: infer from status.
		if status == "firing" {
			return "high"
		}
		return "informational"
	}
}

// extractNode extracts the node name from alert labels.
// Tries "node" first, then falls back to the hostname portion of "instance".
func extractNode(labels map[string]string) string {
	if node := labels["node"]; node != "" {
		return node
	}
	if nodeName := labels["node_name"]; nodeName != "" {
		return nodeName
	}
	// Extract hostname from instance label (e.g., "worker-03:9100" → "worker-03").
	if instance := labels["instance"]; instance != "" {
		host, _, _ := strings.Cut(instance, ":")
		return host
	}
	return ""
}

// buildLabels merges alert labels, annotations, and receiver metadata into a flat map.
func buildLabels(alert Alert, receiver string) map[string]string {
	labels := make(map[string]string, len(alert.Labels)+len(alert.Annotations)+3)

	// Copy alert labels.
	for k, v := range alert.Labels {
		labels["alert_"+k] = v
	}

	// Copy annotations with prefix.
	for k, v := range alert.Annotations {
		labels["annotation_"+k] = v
	}

	// Add metadata.
	labels["source"] = "alertmanager"
	labels["receiver"] = receiver
	labels["alert_status"] = alert.Status

	if alert.Fingerprint != "" {
		labels["fingerprint"] = alert.Fingerprint
	}
	if alert.GeneratorURL != "" {
		labels["generator_url"] = alert.GeneratorURL
	}

	return labels
}

// firstNonEmpty returns the first non-empty string from the arguments.
func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if v != "" {
			return v
		}
	}
	return ""
}
