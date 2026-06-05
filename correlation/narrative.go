package correlation

import (
	"fmt"
	"sort"
	"strings"
	"time"

	export "github.com/mercadoalex/titanops/shared/titanops-export"
)

// GenerateNarrative produces a human-readable description of a correlated incident.
//
// Template: "Correlated incident detected: {module1} reported {event_type1} and
// {module2} reported {event_type2} on {matched_attrs} within {duration}.
// Confidence: {score}%."
//
// When an OllinAI deployment_risk event is in the group, deployment metadata
// (service name, commit SHA, deployer) is appended if present and non-empty.
func GenerateNarrative(events []export.Event, matchedAttributes []string, score int) string {
	if len(events) == 0 {
		return "No events to correlate."
	}

	// Sort events chronologically.
	sorted := make([]export.Event, len(events))
	copy(sorted, events)
	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i].Timestamp.Before(sorted[j].Timestamp)
	})

	// Collect distinct module contributions.
	type moduleReport struct {
		module    string
		eventType string
	}
	seen := make(map[string]bool)
	var reports []moduleReport
	for _, ev := range sorted {
		if !seen[ev.Module] {
			seen[ev.Module] = true
			reports = append(reports, moduleReport{
				module:    ev.Module,
				eventType: ev.EventType,
			})
		}
	}

	// Build the module reports string.
	var parts []string
	for _, r := range reports {
		parts = append(parts, fmt.Sprintf("%s reported %s", r.module, r.eventType))
	}

	moduleString := joinWithAnd(parts)

	// Build matched attributes string.
	attrsString := strings.Join(matchedAttributes, ", ")

	// Calculate duration.
	duration := eventTimeSpan(sorted)
	durationString := formatDuration(duration)

	narrative := fmt.Sprintf("Correlated incident detected: %s on %s within %s. Confidence: %d%%.",
		moduleString, attrsString, durationString, score)

	// Append OllinAI deployment metadata if present.
	deploymentMeta := generateOllinAIDeploymentMeta(sorted)
	if deploymentMeta != "" {
		narrative += " " + deploymentMeta
	}

	return narrative
}

// generateOllinAIDeploymentMeta builds a deployment metadata string from OllinAI
// deployment_risk events in the group. Only includes non-empty fields.
// Returns empty string if no OllinAI deployment_risk event is present or all metadata is empty.
func generateOllinAIDeploymentMeta(events []export.Event) string {
	for _, ev := range events {
		if ev.Module == "ollinai" && ev.EventType == "deployment_risk" {
			return buildDeploymentMetaString(ev)
		}
	}
	return ""
}

// buildDeploymentMetaString constructs a narrative fragment from deployment Labels.
// Includes service name, commit SHA, and deployer only when present and non-empty.
// Also includes risk_score if available.
func buildDeploymentMetaString(ev export.Event) string {
	if ev.Labels == nil {
		return ""
	}

	service := ev.Labels["service"]
	commitSHA := ev.Labels["commit_sha"]
	deployer := ev.Labels["deployer"]
	riskScore := ev.Labels["risk_score"]

	// If all metadata fields are empty, return nothing.
	if service == "" && commitSHA == "" && deployer == "" {
		return ""
	}

	var sb strings.Builder
	sb.WriteString("Deployment")

	if service != "" {
		sb.WriteString(" of ")
		sb.WriteString(service)
	}

	// Build parenthetical with commit and deployer.
	var metaParts []string
	if commitSHA != "" {
		metaParts = append(metaParts, "commit "+commitSHA)
	}
	if deployer != "" {
		metaParts = append(metaParts, "by "+deployer)
	}
	if len(metaParts) > 0 {
		sb.WriteString(" (")
		sb.WriteString(strings.Join(metaParts, " "))
		sb.WriteString(")")
	}

	if riskScore != "" {
		sb.WriteString(" with risk score ")
		sb.WriteString(riskScore)
	}

	sb.WriteString(" occurred within the correlation window.")
	return sb.String()
}

// joinWithAnd joins strings with ", " and " and " for the last element.
func joinWithAnd(parts []string) string {
	switch len(parts) {
	case 0:
		return ""
	case 1:
		return parts[0]
	case 2:
		return parts[0] + " and " + parts[1]
	default:
		return strings.Join(parts[:len(parts)-1], ", ") + " and " + parts[len(parts)-1]
	}
}

// formatDuration formats a duration into a human-readable string.
func formatDuration(d time.Duration) string {
	if d < time.Second {
		return "less than 1s"
	}
	seconds := int(d.Seconds())
	if seconds < 60 {
		return fmt.Sprintf("%ds", seconds)
	}
	minutes := seconds / 60
	remaining := seconds % 60
	if remaining == 0 {
		return fmt.Sprintf("%dm", minutes)
	}
	return fmt.Sprintf("%dm%ds", minutes, remaining)
}
