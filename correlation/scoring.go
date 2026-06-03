package correlation

import (
	"time"

	export "github.com/mercadoalex/titanops/shared/titanops-export"
)

// Scoring constants define the scoring formula parameters.
const (
	// ModuleBaseScore is the score contribution per distinct module (× number of modules).
	ModuleBaseScore = 20
	// AttributeBonus is the bonus per matched attribute type (node, pod, namespace).
	AttributeBonus = 10
	// ProximityBonusClose is the bonus when events are within 10 seconds of each other.
	ProximityBonusClose = 10
	// ProximityBonusNear is the bonus when events are within 30 seconds of each other.
	ProximityBonusNear = 5
	// MaxConfidence is the maximum confidence score.
	MaxConfidence = 100
)

// CalculateConfidence computes the confidence score for a set of correlated events.
//
// Scoring formula:
//   - Base score: number of distinct modules × 20 (2 modules = 40, 3 = 60, 4 = 80)
//   - Attribute bonus: +10 per matching attribute type (node, pod, namespace)
//   - Proximity bonus: +10 if all events within 10s, +5 if within 30s
//   - Cap at 100
func CalculateConfidence(events []export.Event, matchedAttributes []string) int {
	if len(events) < 2 {
		return 0
	}

	// Count distinct modules.
	modules := make(map[string]bool)
	for _, ev := range events {
		modules[ev.Module] = true
	}
	numModules := len(modules)

	// Base score: distinct modules × 20.
	score := numModules * ModuleBaseScore

	// Attribute bonus: +10 per matched attribute type.
	score += len(matchedAttributes) * AttributeBonus

	// Proximity bonus based on temporal span of events.
	span := eventTimeSpan(events)
	if span <= 10*time.Second {
		score += ProximityBonusClose
	} else if span <= 30*time.Second {
		score += ProximityBonusNear
	}

	// Cap at maximum.
	if score > MaxConfidence {
		score = MaxConfidence
	}

	return score
}

// eventTimeSpan returns the duration between the earliest and latest event timestamps.
func eventTimeSpan(events []export.Event) time.Duration {
	if len(events) < 2 {
		return 0
	}

	earliest := events[0].Timestamp
	latest := events[0].Timestamp

	for _, ev := range events[1:] {
		if ev.Timestamp.Before(earliest) {
			earliest = ev.Timestamp
		}
		if ev.Timestamp.After(latest) {
			latest = ev.Timestamp
		}
	}

	return latest.Sub(earliest)
}
