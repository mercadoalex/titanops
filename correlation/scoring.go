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
	// MaxDeploymentRiskBonus is the maximum bonus from OllinAI deployment risk.
	MaxDeploymentRiskBonus = 20
)

// DeploymentRiskBonus normalizes an OllinAI risk score (0-100) to a 0-20 bonus.
// The input is clamped to [0, 100] before dividing by 5.
func DeploymentRiskBonus(riskScore int) int {
	if riskScore < 0 {
		return 0
	}
	if riskScore > 100 {
		return MaxDeploymentRiskBonus
	}
	return riskScore / 5 // 0-100 → 0-20
}

// CalculateConfidence computes the confidence score for a set of correlated events.
//
// Scoring formula:
//   - Base score: number of distinct modules × 20 (2 modules = 40, 3 = 60, 4 = 80)
//   - Attribute bonus: +10 per matching attribute type (node, pod, namespace)
//   - Proximity bonus: +10 if all events within 10s, +5 if within 30s
//   - OllinAI deployment risk bonus: +0-20 when a deployment_risk event is present
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

	// OllinAI deployment risk bonus: add normalized risk score (0-20)
	// when an OllinAI deployment_risk event is present in the group.
	score += ollinAIDeploymentRiskBonusFromEvents(events)

	// Cap at maximum.
	if score > MaxConfidence {
		score = MaxConfidence
	}

	return score
}

// ollinAIDeploymentRiskBonusFromEvents checks if an OllinAI deployment_risk event
// is present in the event group and returns the deployment risk bonus.
// If multiple deployment_risk events exist, uses the highest risk score.
func ollinAIDeploymentRiskBonusFromEvents(events []export.Event) int {
	maxBonus := 0
	for _, ev := range events {
		if ev.Module == "ollinai" && ev.EventType == "deployment_risk" {
			riskScore := extractRiskScore(ev)
			bonus := DeploymentRiskBonus(riskScore)
			if bonus > maxBonus {
				maxBonus = bonus
			}
		}
	}
	return maxBonus
}

// extractRiskScore extracts the risk_score from an OllinAI deployment_risk event's Labels.
// Returns 0 if the label is missing or unparseable.
func extractRiskScore(ev export.Event) int {
	if ev.Labels == nil {
		return 0
	}
	scoreStr, ok := ev.Labels["risk_score"]
	if !ok {
		return 0
	}
	var score int
	for _, c := range scoreStr {
		if c >= '0' && c <= '9' {
			score = score*10 + int(c-'0')
		} else {
			return 0
		}
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
