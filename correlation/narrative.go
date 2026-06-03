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

	return fmt.Sprintf("Correlated incident detected: %s on %s within %s. Confidence: %d%%.",
		moduleString, attrsString, durationString, score)
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
