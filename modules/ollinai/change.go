package ollinai

import "time"

// ChangeType classifies what kind of deployment change was detected.
type ChangeType string

const (
	// ChangeImage indicates a container image tag or digest changed.
	ChangeImage ChangeType = "image"
	// ChangeEnvVar indicates environment variables were modified.
	ChangeEnvVar ChangeType = "env_var"
	// ChangeResource indicates CPU/memory requests or limits changed.
	ChangeResource ChangeType = "resource_limits"
	// ChangeReplica indicates replica count was scaled.
	ChangeReplica ChangeType = "replica_scale"
	// ChangeProbe indicates liveness/readiness probes were modified.
	ChangeProbe ChangeType = "probe"
	// ChangeRolloutStrategy indicates the deployment strategy changed.
	ChangeRolloutStrategy ChangeType = "rollout_strategy"
	// ChangeOther indicates a change that doesn't fit other categories.
	ChangeOther ChangeType = "other"
)

// ChangeSeverity indicates how likely a change type is to cause regression.
type ChangeSeverity string

const (
	ChangeSeverityHigh   ChangeSeverity = "high"
	ChangeSeverityMedium ChangeSeverity = "medium"
	ChangeSeverityLow    ChangeSeverity = "low"
)

// DetectedChange represents a single observed change in a workload spec.
type DetectedChange struct {
	// Type is the classification of this change.
	Type ChangeType `json:"type"`
	// Severity indicates the expected risk level of this change type.
	Severity ChangeSeverity `json:"severity"`
	// Field is the specific spec field that changed (e.g., "containers[0].image").
	Field string `json:"field"`
	// OldValue is the previous value (may be truncated for large values).
	OldValue string `json:"old_value,omitempty"`
	// NewValue is the current value (may be truncated for large values).
	NewValue string `json:"new_value,omitempty"`
}

// DeploymentChange is the full record of a detected workload change,
// combining identity, classification, and timing.
type DeploymentChange struct {
	// WorkloadName is the name of the Deployment/StatefulSet/DaemonSet.
	WorkloadName string `json:"workload_name"`
	// WorkloadKind is "Deployment", "StatefulSet", or "DaemonSet".
	WorkloadKind string `json:"workload_kind"`
	// Namespace is the K8s namespace.
	Namespace string `json:"namespace"`
	// Changes lists all individual changes detected in this rollout.
	Changes []DetectedChange `json:"changes"`
	// DetectedAt is when the change was first observed.
	DetectedAt time.Time `json:"detected_at"`
	// CommitSHA is the git commit associated with this deploy (if available via labels/annotations).
	CommitSHA string `json:"commit_sha,omitempty"`
	// Deployer is the user/system that triggered the deploy (if available).
	Deployer string `json:"deployer,omitempty"`
}

// HighestSeverity returns the highest severity among all detected changes.
func (dc *DeploymentChange) HighestSeverity() ChangeSeverity {
	highest := ChangeSeverityLow
	for _, c := range dc.Changes {
		if severityRank(c.Severity) > severityRank(highest) {
			highest = c.Severity
		}
	}
	return highest
}

// HasChangeType returns true if any detected change matches the given type.
func (dc *DeploymentChange) HasChangeType(t ChangeType) bool {
	for _, c := range dc.Changes {
		if c.Type == t {
			return true
		}
	}
	return false
}

// CheckWindow returns the recommended verification window duration
// based on the highest-severity change type present.
func (dc *DeploymentChange) CheckWindow() time.Duration {
	if dc.HasChangeType(ChangeResource) {
		return 120 * time.Second
	}
	if dc.HasChangeType(ChangeImage) || dc.HasChangeType(ChangeProbe) {
		return 90 * time.Second
	}
	if dc.HasChangeType(ChangeEnvVar) {
		return 60 * time.Second
	}
	if dc.HasChangeType(ChangeReplica) {
		return 20 * time.Second
	}
	return 60 * time.Second
}

// severityRank returns a numeric rank for severity comparison.
func severityRank(s ChangeSeverity) int {
	switch s {
	case ChangeSeverityHigh:
		return 3
	case ChangeSeverityMedium:
		return 2
	case ChangeSeverityLow:
		return 1
	default:
		return 0
	}
}

// ClassifyImageChange creates a DetectedChange for a container image modification.
func ClassifyImageChange(field, oldImage, newImage string) DetectedChange {
	return DetectedChange{
		Type:     ChangeImage,
		Severity: ChangeSeverityHigh,
		Field:    field,
		OldValue: oldImage,
		NewValue: newImage,
	}
}

// ClassifyEnvChange creates a DetectedChange for an environment variable modification.
func ClassifyEnvChange(field, oldVal, newVal string) DetectedChange {
	return DetectedChange{
		Type:     ChangeEnvVar,
		Severity: ChangeSeverityHigh,
		Field:    field,
		OldValue: truncateValue(oldVal, 128),
		NewValue: truncateValue(newVal, 128),
	}
}

// ClassifyResourceChange creates a DetectedChange for a resource limit/request modification.
func ClassifyResourceChange(field, oldVal, newVal string) DetectedChange {
	return DetectedChange{
		Type:     ChangeResource,
		Severity: ChangeSeverityMedium,
		Field:    field,
		OldValue: oldVal,
		NewValue: newVal,
	}
}

// ClassifyReplicaChange creates a DetectedChange for a replica count modification.
func ClassifyReplicaChange(oldReplicas, newReplicas int) DetectedChange {
	return DetectedChange{
		Type:     ChangeReplica,
		Severity: ChangeSeverityLow,
		Field:    "spec.replicas",
		OldValue: intToStr(oldReplicas),
		NewValue: intToStr(newReplicas),
	}
}

// ClassifyProbeChange creates a DetectedChange for a liveness/readiness probe modification.
func ClassifyProbeChange(field, description string) DetectedChange {
	return DetectedChange{
		Type:     ChangeProbe,
		Severity: ChangeSeverityHigh,
		Field:    field,
		NewValue: description,
	}
}

// ClassifyRolloutStrategyChange creates a DetectedChange for a rollout strategy modification.
func ClassifyRolloutStrategyChange(oldStrategy, newStrategy string) DetectedChange {
	return DetectedChange{
		Type:     ChangeRolloutStrategy,
		Severity: ChangeSeverityMedium,
		Field:    "spec.strategy.type",
		OldValue: oldStrategy,
		NewValue: newStrategy,
	}
}

// truncateValue truncates a string to maxLen for storage in change records.
func truncateValue(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}

// intToStr converts an int to string without importing strconv in this file.
func intToStr(n int) string {
	if n == 0 {
		return "0"
	}
	neg := false
	if n < 0 {
		neg = true
		n = -n
	}
	digits := make([]byte, 0, 10)
	for n > 0 {
		digits = append(digits, byte('0'+n%10))
		n /= 10
	}
	if neg {
		digits = append(digits, '-')
	}
	// Reverse
	for i, j := 0, len(digits)-1; i < j; i, j = i+1, j-1 {
		digits[i], digits[j] = digits[j], digits[i]
	}
	return string(digits)
}
