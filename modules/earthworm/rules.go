package earthworm

// RuleThresholds defines the static thresholds used for rule-based
// anomaly detection when the AI model is unavailable.
type RuleThresholds struct {
	// CPUThreshold is the CPU usage level that triggers an anomaly (default 0.9).
	CPUThreshold float64
	// MemoryThreshold is the memory usage level that triggers an anomaly (default 0.85).
	MemoryThreshold float64
	// LatencyThreshold is the latency in milliseconds that triggers an anomaly (default 500).
	LatencyThreshold float64
}

// DefaultRuleThresholds returns sensible default thresholds for rule-based detection.
func DefaultRuleThresholds() RuleThresholds {
	return RuleThresholds{
		CPUThreshold:     0.9,
		MemoryThreshold:  0.85,
		LatencyThreshold: 500,
	}
}

// evaluateRules performs rule-based anomaly detection using static thresholds.
// Returns a synthetic confidence score [0.0, 1.0] and the name of the rule
// that triggered, or an empty string if no rule matched.
func (a *Agent) evaluateRules(heartbeat HeartbeatSignal) (float64, string) {
	thresholds := a.config.RuleThresholds

	// Check each threshold and produce a synthetic score.
	// A triggered rule produces a score proportional to how far
	// the metric exceeds the threshold.

	if heartbeat.CPUUsage >= thresholds.CPUThreshold {
		// Score based on how far above threshold.
		excess := (heartbeat.CPUUsage - thresholds.CPUThreshold) / (1.0 - thresholds.CPUThreshold)
		score := 0.75 + (excess * 0.25) // Range [0.75, 1.0]
		if score > 1.0 {
			score = 1.0
		}
		return score, "cpu_threshold_exceeded"
	}

	if heartbeat.MemoryUsage >= thresholds.MemoryThreshold {
		excess := (heartbeat.MemoryUsage - thresholds.MemoryThreshold) / (1.0 - thresholds.MemoryThreshold)
		score := 0.75 + (excess * 0.25)
		if score > 1.0 {
			score = 1.0
		}
		return score, "memory_threshold_exceeded"
	}

	if heartbeat.Latency >= thresholds.LatencyThreshold {
		excess := (heartbeat.Latency - thresholds.LatencyThreshold) / (1000.0 - thresholds.LatencyThreshold)
		score := 0.75 + (excess * 0.25)
		if score > 1.0 {
			score = 1.0
		}
		return score, "latency_threshold_exceeded"
	}

	// No rule triggered — return 0 score.
	return 0.0, ""
}
