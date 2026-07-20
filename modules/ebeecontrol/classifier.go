package ebeecontrol

// ClassifyThreat determines the threat level based on a full PodContext.
//
// Classification rules (evaluated from highest to lowest severity):
//   - critical: production AND (anomaly > 0.8 OR criticality 5)
//   - high:     production AND (anomaly 0.6-0.8 OR criticality 4)
//   - medium:   production OR anomaly 0.3-0.6 OR criticality 3
//   - low:      non-production AND anomaly < 0.3 AND criticality 1-2
func ClassifyThreat(ctx PodContext) ThreatClassification {
	return classifyFromInputs(ctx.NamespaceClassification, ctx.ServiceCriticality, ctx.DavisAnomalyScore)
}

// ClassifyThreatWithDefaults classifies the threat from partial context,
// substituting highest-risk defaults for any missing/zero fields.
//
// Default values (highest risk):
//   - NamespaceClassification: "production"
//   - ServiceCriticality: 5
//   - DavisAnomalyScore: 1.0
func ClassifyThreatWithDefaults(nsClass NamespaceClassification, criticality int, anomalyScore float64) ThreatClassification {
	if nsClass == "" {
		nsClass = NamespaceProduction
	}
	if criticality == 0 {
		criticality = 5
	}
	if anomalyScore == 0 {
		anomalyScore = 1.0
	}
	return classifyFromInputs(nsClass, criticality, anomalyScore)
}

// classifyFromInputs is the core classification logic operating on resolved input values.
//
// Rules evaluated from highest severity to lowest:
//  1. critical: production AND (anomaly > 0.8 OR criticality 5)
//  2. high:     production AND (anomaly 0.6-0.8 OR criticality 4)
//  3. medium:   production OR anomaly 0.3-0.6 OR criticality 3
//  4. low:      non-production AND anomaly < 0.3 AND criticality 1-2
//
// Edge cases not covered by explicit rules fall through to medium (conservative).
func classifyFromInputs(nsClass NamespaceClassification, criticality int, anomalyScore float64) ThreatClassification {
	isProduction := nsClass == NamespaceProduction

	// Critical: production AND (anomaly > 0.8 OR criticality 5)
	if isProduction && (anomalyScore > 0.8 || criticality == 5) {
		return ThreatCritical
	}

	// High: production AND (anomaly 0.6-0.8 OR criticality 4)
	if isProduction && ((anomalyScore >= 0.6 && anomalyScore <= 0.8) || criticality == 4) {
		return ThreatHigh
	}

	// Medium: production OR anomaly 0.3-0.6 OR criticality 3
	if isProduction || (anomalyScore >= 0.3 && anomalyScore <= 0.6) || criticality == 3 {
		return ThreatMedium
	}

	// Low: non-production AND anomaly < 0.3 AND criticality 1-2
	if !isProduction && anomalyScore < 0.3 && criticality <= 2 {
		return ThreatLow
	}

	// Conservative fallback for edge cases
	return ThreatMedium
}
