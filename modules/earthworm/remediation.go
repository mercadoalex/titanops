package earthworm

import "time"

// Remediation action type constants.
const (
	ActionPodRestart         = "pod_restart"
	ActionNodeCordon         = "node_cordon"
	ActionWorkloadReschedule = "workload_reschedule"
)

// ActionResult records the outcome of a remediation decision,
// whether triggered by AI inference or rule-based fallback.
type ActionResult struct {
	// ActionType is the remediation action taken (pod_restart, node_cordon, workload_reschedule).
	ActionType string
	// NodeID is the Kubernetes node identifier where the action was taken.
	NodeID string
	// Confidence is the anomaly score that triggered the action [0.0, 1.0].
	Confidence float64
	// Heartbeat is the signal that triggered the anomaly detection.
	Heartbeat HeartbeatSignal
	// Timestamp is when the action was executed.
	Timestamp time.Time
	// WasRuleBased indicates whether rule-based fallback was used instead of AI.
	WasRuleBased bool
}

// selectAction determines the remediation action type based on the
// heartbeat signal characteristics. Higher-severity conditions get
// more aggressive remediation.
func selectAction(heartbeat HeartbeatSignal, confidence float64) string {
	// High confidence with high CPU+Memory suggests node-level issue.
	if confidence >= 0.9 && heartbeat.CPUUsage > 0.9 && heartbeat.MemoryUsage > 0.85 {
		return ActionNodeCordon
	}
	// High latency with moderate resource usage suggests workload misplacement.
	if heartbeat.Latency > 500 && heartbeat.CPUUsage < 0.7 {
		return ActionWorkloadReschedule
	}
	// Default: restart affected pods.
	return ActionPodRestart
}
