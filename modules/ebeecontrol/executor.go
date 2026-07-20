package ebeecontrol

import (
	"context"
	"fmt"
	"sort"
	"time"

	"github.com/google/uuid"
)

// ResponseExecutorDeps provides the operations needed to execute response actions.
// All functions are injected for testability.
type ResponseExecutorDeps struct {
	// IsolatePod isolates a pod by ID (e.g., applying a deny-all NetworkPolicy).
	IsolatePod func(ctx context.Context, podID string) error
	// BlockIP blocks network access for a pod (e.g., egress deny).
	BlockIP func(ctx context.Context, podID string) error
	// DeployHoneytokens deploys additional honeytokens in a namespace.
	DeployHoneytokens func(ctx context.Context, namespace string, count int) error
	// SendAlert sends a notification/alert message.
	SendAlert func(ctx context.Context, message string) error
}

// ResponseExecutionResult contains the results of executing a full response plan.
type ResponseExecutionResult struct {
	// Actions lists all actions that were attempted, with their results.
	Actions []ResponseAction
	// AllSucceeded is true if every action completed successfully.
	AllSucceeded bool
	// CriticalFailures lists descriptions of any critical failures requiring escalation.
	CriticalFailures []string
}

// ExecuteResponse executes a response plan based on the threat assessment.
//
// Actions are executed in priority order (lower priority number = higher priority).
// Each action has retry logic:
//   - pod_isolation: 3 retries, 5s interval; alert on each failure; critical alert on exhaustion
//   - ip_block: 3 retries, 5s interval; alert on each failure
//   - additional_honeytokens: deploy 2+ honeytokens, no retry
//
// All actions are logged with type, target, timestamp, classification, and result.
func ExecuteResponse(ctx context.Context, plan ResponsePlan, assessment ThreatAssessment, cfg ResponseConfig, deps ResponseExecutorDeps) ResponseExecutionResult {
	result := ResponseExecutionResult{}

	// Sort actions by priority (lower number = higher priority).
	sorted := make([]PlannedAction, len(plan.Actions))
	copy(sorted, plan.Actions)
	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i].Priority < sorted[j].Priority
	})

	for _, planned := range sorted {
		action := executeSingleAction(ctx, planned, plan, assessment, cfg, deps, &result.CriticalFailures)
		result.Actions = append(result.Actions, action)
	}

	result.AllSucceeded = true
	for _, a := range result.Actions {
		if a.Result == ActionFailure {
			result.AllSucceeded = false
			break
		}
	}

	return result
}

// executeSingleAction dispatches a single planned action with appropriate retry logic.
func executeSingleAction(
	ctx context.Context,
	planned PlannedAction,
	plan ResponsePlan,
	assessment ThreatAssessment,
	cfg ResponseConfig,
	deps ResponseExecutorDeps,
	criticalFailures *[]string,
) ResponseAction {
	actionID := uuid.New().String()
	timestamp := time.Now().UTC()

	var result ActionResult
	var retryCount int

	switch planned.ActionType {
	case ActionPodIsolation:
		result, retryCount = executePodIsolation(ctx, plan.PodID, cfg, deps, criticalFailures)
	case ActionIPBlock:
		result, retryCount = executeIPBlock(ctx, plan.PodID, cfg, deps)
	case ActionAdditionalHoneytokens:
		result, retryCount = executeHoneytokenDeployment(ctx, plan.Namespace, deps)
	default:
		result = ActionFailure
	}

	return ResponseAction{
		ActionID:             actionID,
		ActionType:           planned.ActionType,
		Target:               planned.Target,
		Timestamp:            timestamp,
		ThreatClassification: assessment.Classification,
		Result:               result,
		RetryCount:           retryCount,
	}
}

// executePodIsolation isolates a pod with retry logic.
// - maxRetries retries, retryInterval between attempts
// - Alert on each failure
// - Critical alert on exhaustion
func executePodIsolation(
	ctx context.Context,
	podID string,
	cfg ResponseConfig,
	deps ResponseExecutorDeps,
	criticalFailures *[]string,
) (ActionResult, int) {
	maxRetries := cfg.IsolationMaxRetries
	interval := cfg.IsolationRetryInterval

	var lastErr error
	for attempt := 0; attempt <= maxRetries; attempt++ {
		if attempt > 0 {
			// Alert on retry.
			_ = deps.SendAlert(ctx, fmt.Sprintf(
				"Pod isolation failed for %s (attempt %d): %v. Retrying...",
				podID, attempt, lastErr,
			))
			select {
			case <-ctx.Done():
				return ActionFailure, attempt
			case <-time.After(interval):
			}
		}

		if err := deps.IsolatePod(ctx, podID); err != nil {
			lastErr = err
			continue
		}
		return ActionSuccess, attempt
	}

	// All retries exhausted — critical alert.
	msg := fmt.Sprintf(
		"CRITICAL: Pod isolation failed for %s after all retries exhausted. Manual intervention required. Last error: %v",
		podID, lastErr,
	)
	_ = deps.SendAlert(ctx, msg)
	*criticalFailures = append(*criticalFailures, fmt.Sprintf("Pod isolation failed for %s: %v", podID, lastErr))

	return ActionFailure, maxRetries
}

// executeIPBlock blocks IP access for a pod with retry logic.
// - maxRetries retries, retryInterval between attempts
// - Alert on each failure
func executeIPBlock(
	ctx context.Context,
	podID string,
	cfg ResponseConfig,
	deps ResponseExecutorDeps,
) (ActionResult, int) {
	maxRetries := cfg.IPBlockMaxRetries
	interval := cfg.IPBlockRetryInterval

	var lastErr error
	for attempt := 0; attempt <= maxRetries; attempt++ {
		if attempt > 0 {
			_ = deps.SendAlert(ctx, fmt.Sprintf(
				"IP block failed for %s (attempt %d): %v. Retrying...",
				podID, attempt, lastErr,
			))
			select {
			case <-ctx.Done():
				return ActionFailure, attempt
			case <-time.After(interval):
			}
		}

		if err := deps.BlockIP(ctx, podID); err != nil {
			lastErr = err
			continue
		}
		return ActionSuccess, attempt
	}

	_ = deps.SendAlert(ctx, fmt.Sprintf(
		"IP block failed for %s after all retries exhausted. Last error: %v",
		podID, lastErr,
	))

	return ActionFailure, maxRetries
}

// executeHoneytokenDeployment deploys 2 additional honeytokens in the namespace.
// No retry — single attempt.
func executeHoneytokenDeployment(
	ctx context.Context,
	namespace string,
	deps ResponseExecutorDeps,
) (ActionResult, int) {
	if err := deps.DeployHoneytokens(ctx, namespace, 2); err != nil {
		return ActionFailure, 0
	}
	return ActionSuccess, 0
}
