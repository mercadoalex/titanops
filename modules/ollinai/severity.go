package ollinai

// Severity level constants.
const (
	SeverityCritical      = "critical"
	SeverityHigh          = "high"
	SeverityMedium        = "medium"
	SeverityLow           = "low"
	SeverityInformational = "informational"
)

// MapRiskToSeverity maps a deployment risk score (0-100) to a severity string.
// Thresholds: critical ≥ 80, high ≥ 60, medium ≥ 40, low < 40.
// Scores outside [0, 100] are clamped to the nearest boundary.
func MapRiskToSeverity(score int) string {
	switch {
	case score >= 80:
		return SeverityCritical
	case score >= 60:
		return SeverityHigh
	case score >= 40:
		return SeverityMedium
	default:
		return SeverityLow
	}
}
