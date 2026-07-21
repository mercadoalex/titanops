package ollinai

import (
	"time"

	"github.com/google/uuid"
)

// VerdictStatus is the outcome of a deployment verification.
type VerdictStatus string

const (
	// VerdictHealthy means no regression was detected.
	VerdictHealthy VerdictStatus = "healthy"
	// VerdictRegression means a regression was detected.
	VerdictRegression VerdictStatus = "regression"
	// VerdictInconclusive means the verification could not determine the outcome.
	VerdictInconclusive VerdictStatus = "inconclusive"
)

// SignalCheck represents a single verification check result.
type SignalCheck struct {
	// Signal is what was checked (e.g., "error_rate", "latency_p99", "pod_restarts").
	Signal string `json:"signal"`
	// BaselineValue is the pre-deployment measurement.
	BaselineValue float64 `json:"baseline_value"`
	// CurrentValue is the post-deployment measurement.
	CurrentValue float64 `json:"current_value"`
	// Threshold is the tolerance threshold (e.g., 1.5 = 50% increase allowed).
	Threshold float64 `json:"threshold"`
	// Passed is whether this check passed (current within threshold of baseline).
	Passed bool `json:"passed"`
	// Unit describes the measurement unit (e.g., "percent", "ms", "count").
	Unit string `json:"unit,omitempty"`
}

// Verdict is the full result of verifying a single deployment change.
type Verdict struct {
	// VerdictID is a unique identifier for this verification.
	VerdictID string `json:"verdict_id"`
	// Status is the outcome: healthy, regression, or inconclusive.
	Status VerdictStatus `json:"status"`
	// Change is the deployment change that triggered verification.
	Change DeploymentChange `json:"change"`
	// Checks lists all signal checks performed.
	Checks []SignalCheck `json:"checks"`
	// FailedChecks lists only the checks that failed.
	FailedChecks []SignalCheck `json:"failed_checks,omitempty"`
	// Summary is a human-readable one-line description of the outcome.
	Summary string `json:"summary"`
	// StartedAt is when verification began.
	StartedAt time.Time `json:"started_at"`
	// CompletedAt is when verification finished.
	CompletedAt time.Time `json:"completed_at"`
	// Duration is how long verification took.
	Duration time.Duration `json:"duration"`
}

// EventTypeDeploymentVerification is the event type for verification verdicts.
const EventTypeDeploymentVerification = "deployment_verification"

// NewVerdict constructs a Verdict from the verification inputs.
func NewVerdict(change DeploymentChange, checks []SignalCheck, startedAt time.Time) Verdict {
	now := time.Now().UTC()

	var failed []SignalCheck
	for _, c := range checks {
		if !c.Passed {
			failed = append(failed, c)
		}
	}

	status := determineStatus(checks, failed)
	summary := buildSummary(change, status, failed)

	return Verdict{
		VerdictID:    uuid.New().String(),
		Status:       status,
		Change:       change,
		Checks:       checks,
		FailedChecks: failed,
		Summary:      summary,
		StartedAt:    startedAt,
		CompletedAt:  now,
		Duration:     now.Sub(startedAt),
	}
}

// determineStatus decides the verdict based on check results.
func determineStatus(all []SignalCheck, failed []SignalCheck) VerdictStatus {
	if len(all) == 0 {
		return VerdictInconclusive
	}
	if len(failed) == 0 {
		return VerdictHealthy
	}
	return VerdictRegression
}

// buildSummary generates a human-readable summary line.
func buildSummary(change DeploymentChange, status VerdictStatus, failed []SignalCheck) string {
	switch status {
	case VerdictHealthy:
		return change.WorkloadKind + "/" + change.WorkloadName + " in " + change.Namespace + ": deployment verified healthy"
	case VerdictRegression:
		signals := ""
		for i, f := range failed {
			if i > 0 {
				signals += ", "
			}
			signals += f.Signal
			if i >= 2 {
				signals += "..."
				break
			}
		}
		return change.WorkloadKind + "/" + change.WorkloadName + " in " + change.Namespace + ": regression detected (" + signals + ")"
	default:
		return change.WorkloadKind + "/" + change.WorkloadName + " in " + change.Namespace + ": verification inconclusive (insufficient data)"
	}
}
