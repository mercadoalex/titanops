package ollinai

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"sync"
	"time"

	export "github.com/mercadoalex/titanops/shared/titanops-export"
)

// MetricsSource provides telemetry signals for pre/post deployment comparison.
// In production this queries Prometheus, the correlation engine, or internal metrics.
type MetricsSource interface {
	// QueryErrorRate returns the error rate (0.0-1.0) for a workload in a time window.
	QueryErrorRate(ctx context.Context, namespace, workload string, window time.Duration) (float64, error)
	// QueryLatencyP99 returns the p99 latency in ms for a workload in a time window.
	QueryLatencyP99(ctx context.Context, namespace, workload string, window time.Duration) (float64, error)
	// QueryPodRestarts returns the number of pod restarts for a workload in a time window.
	QueryPodRestarts(ctx context.Context, namespace, workload string, window time.Duration) (float64, error)
	// QueryThroughput returns requests/sec for a workload in a time window.
	QueryThroughput(ctx context.Context, namespace, workload string, window time.Duration) (float64, error)
	// QueryCPUSaturation returns CPU usage ratio (0.0-1.0) for a workload.
	QueryCPUSaturation(ctx context.Context, namespace, workload string, window time.Duration) (float64, error)
}

// VerifierConfig configures the deployment verifier.
type VerifierConfig struct {
	// Metrics provides telemetry signals.
	Metrics MetricsSource
	// Emitter publishes verdict events.
	Emitter EventEmitter
	// BaselineWindow is how far back to look for baseline data. Default 5m.
	BaselineWindow time.Duration
	// ErrorRateThreshold is the max multiplier allowed (e.g., 1.5 = 50% increase). Default 1.5.
	ErrorRateThreshold float64
	// LatencyThreshold is the max multiplier for p99 latency. Default 2.0.
	LatencyThreshold float64
	// RestartThreshold is the max absolute restart count allowed post-deploy. Default 1.
	RestartThreshold float64
	// ThroughputDropThreshold is the min ratio of post/pre throughput. Default 0.7 (30% drop).
	ThroughputDropThreshold float64
	// CPUThreshold is the max CPU saturation allowed. Default 0.9.
	CPUThreshold float64
	// Logger for messages. Optional.
	Logger *log.Logger
}

// Verifier runs deployment verification checks against live telemetry.
type Verifier struct {
	metrics                 MetricsSource
	emitter                 EventEmitter
	baselineWindow          time.Duration
	errorRateThreshold      float64
	latencyThreshold        float64
	restartThreshold        float64
	throughputDropThreshold float64
	cpuThreshold            float64
	logger                  *log.Logger

	mu       sync.Mutex
	verdicts []Verdict
}

// NewVerifier creates a deployment verifier.
func NewVerifier(cfg VerifierConfig) *Verifier {
	if cfg.BaselineWindow <= 0 {
		cfg.BaselineWindow = 5 * time.Minute
	}
	if cfg.ErrorRateThreshold <= 0 {
		cfg.ErrorRateThreshold = 1.5
	}
	if cfg.LatencyThreshold <= 0 {
		cfg.LatencyThreshold = 2.0
	}
	if cfg.RestartThreshold <= 0 {
		cfg.RestartThreshold = 1.0
	}
	if cfg.ThroughputDropThreshold <= 0 {
		cfg.ThroughputDropThreshold = 0.7
	}
	if cfg.CPUThreshold <= 0 {
		cfg.CPUThreshold = 0.9
	}
	logger := cfg.Logger
	if logger == nil {
		logger = log.New(os.Stderr, "[ollinai:verifier] ", log.LstdFlags|log.Lmsgprefix)
	}

	return &Verifier{
		metrics:                 cfg.Metrics,
		emitter:                 cfg.Emitter,
		baselineWindow:          cfg.BaselineWindow,
		errorRateThreshold:      cfg.ErrorRateThreshold,
		latencyThreshold:        cfg.LatencyThreshold,
		restartThreshold:        cfg.RestartThreshold,
		throughputDropThreshold: cfg.ThroughputDropThreshold,
		cpuThreshold:            cfg.CPUThreshold,
		logger:                  logger,
	}
}

// Verify runs the full verification flow for a detected deployment change:
// 1. Wait for the check window (pods to stabilize)
// 2. Capture baseline (pre-deploy signals)
// 3. Capture current (post-deploy signals)
// 4. Compare and produce a verdict
// 5. Emit the verdict as an event
func (v *Verifier) Verify(ctx context.Context, change DeploymentChange) (Verdict, error) {
	startedAt := time.Now().UTC()
	checkWindow := change.CheckWindow()

	v.logger.Printf("verifying %s/%s in %s (window: %s, severity: %s)",
		change.WorkloadKind, change.WorkloadName, change.Namespace, checkWindow, change.HighestSeverity())

	// Wait for check window (allow pods to stabilize after rollout).
	select {
	case <-ctx.Done():
		return Verdict{}, ctx.Err()
	case <-time.After(checkWindow):
	}

	// Run signal checks.
	checks := v.runChecks(ctx, change)

	// Build verdict.
	verdict := NewVerdict(change, checks, startedAt)

	// Store verdict.
	v.mu.Lock()
	v.verdicts = append(v.verdicts, verdict)
	v.mu.Unlock()

	// Emit verdict event.
	if v.emitter != nil {
		if err := v.emitVerdict(ctx, verdict); err != nil {
			v.logger.Printf("failed to emit verdict event: %v", err)
		}
	}

	v.logger.Printf("verdict: %s — %s (duration: %s)", verdict.Status, verdict.Summary, verdict.Duration)
	return verdict, nil
}

// runChecks executes all signal checks for the given change.
func (v *Verifier) runChecks(ctx context.Context, change DeploymentChange) []SignalCheck {
	ns := change.Namespace
	name := change.WorkloadName
	var checks []SignalCheck

	// Select which checks to run based on change type.
	signals := v.selectSignals(change)

	for _, signal := range signals {
		check := v.runSingleCheck(ctx, signal, ns, name)
		if check != nil {
			checks = append(checks, *check)
		}
	}

	return checks
}

// signalType identifies which signal to check.
type signalType string

const (
	signalErrorRate   signalType = "error_rate"
	signalLatencyP99  signalType = "latency_p99"
	signalPodRestarts signalType = "pod_restarts"
	signalThroughput  signalType = "throughput"
	signalCPU         signalType = "cpu_saturation"
)

// selectSignals determines which signals to check based on change types present.
func (v *Verifier) selectSignals(change DeploymentChange) []signalType {
	signals := make(map[signalType]bool)

	for _, c := range change.Changes {
		switch c.Type {
		case ChangeImage:
			signals[signalErrorRate] = true
			signals[signalLatencyP99] = true
			signals[signalPodRestarts] = true
			signals[signalThroughput] = true
		case ChangeEnvVar:
			signals[signalErrorRate] = true
			signals[signalPodRestarts] = true
		case ChangeResource:
			signals[signalCPU] = true
			signals[signalLatencyP99] = true
			signals[signalPodRestarts] = true
		case ChangeReplica:
			signals[signalThroughput] = true
		case ChangeProbe:
			signals[signalPodRestarts] = true
			signals[signalThroughput] = true
		case ChangeRolloutStrategy:
			signals[signalPodRestarts] = true
			signals[signalThroughput] = true
		default:
			signals[signalErrorRate] = true
			signals[signalPodRestarts] = true
		}
	}

	result := make([]signalType, 0, len(signals))
	for s := range signals {
		result = append(result, s)
	}
	return result
}

// runSingleCheck executes one signal check (baseline vs current).
func (v *Verifier) runSingleCheck(ctx context.Context, signal signalType, ns, workload string) *SignalCheck {
	var baseline, current float64
	var err error
	var threshold float64
	var unit string

	switch signal {
	case signalErrorRate:
		baseline, err = v.metrics.QueryErrorRate(ctx, ns, workload, v.baselineWindow)
		if err != nil {
			return nil
		}
		current, err = v.metrics.QueryErrorRate(ctx, ns, workload, 1*time.Minute)
		if err != nil {
			return nil
		}
		threshold = v.errorRateThreshold
		unit = "ratio"

	case signalLatencyP99:
		baseline, err = v.metrics.QueryLatencyP99(ctx, ns, workload, v.baselineWindow)
		if err != nil {
			return nil
		}
		current, err = v.metrics.QueryLatencyP99(ctx, ns, workload, 1*time.Minute)
		if err != nil {
			return nil
		}
		threshold = v.latencyThreshold
		unit = "ms"

	case signalPodRestarts:
		baseline, _ = v.metrics.QueryPodRestarts(ctx, ns, workload, v.baselineWindow)
		current, err = v.metrics.QueryPodRestarts(ctx, ns, workload, 2*time.Minute)
		if err != nil {
			return nil
		}
		threshold = v.restartThreshold
		unit = "count"

	case signalThroughput:
		baseline, err = v.metrics.QueryThroughput(ctx, ns, workload, v.baselineWindow)
		if err != nil {
			return nil
		}
		current, err = v.metrics.QueryThroughput(ctx, ns, workload, 1*time.Minute)
		if err != nil {
			return nil
		}
		threshold = v.throughputDropThreshold
		unit = "req/s"

	case signalCPU:
		baseline, _ = v.metrics.QueryCPUSaturation(ctx, ns, workload, v.baselineWindow)
		current, err = v.metrics.QueryCPUSaturation(ctx, ns, workload, 1*time.Minute)
		if err != nil {
			return nil
		}
		threshold = v.cpuThreshold
		unit = "ratio"

	default:
		return nil
	}

	passed := evaluateCheck(signal, baseline, current, threshold)

	return &SignalCheck{
		Signal:        string(signal),
		BaselineValue: baseline,
		CurrentValue:  current,
		Threshold:     threshold,
		Passed:        passed,
		Unit:          unit,
	}
}

// evaluateCheck determines if a signal check passed based on the signal type.
func evaluateCheck(signal signalType, baseline, current, threshold float64) bool {
	switch signal {
	case signalErrorRate:
		// Pass if current error rate is within threshold multiplier of baseline.
		if baseline == 0 {
			return current == 0
		}
		return current <= baseline*threshold

	case signalLatencyP99:
		// Pass if current latency is within threshold multiplier of baseline.
		if baseline == 0 {
			return true
		}
		return current <= baseline*threshold

	case signalPodRestarts:
		// Pass if post-deploy restarts don't exceed threshold (absolute).
		return current <= threshold

	case signalThroughput:
		// Pass if throughput hasn't dropped below threshold ratio of baseline.
		if baseline == 0 {
			return true
		}
		return current >= baseline*threshold

	case signalCPU:
		// Pass if CPU saturation is below threshold.
		return current < threshold

	default:
		return true
	}
}

// emitVerdict publishes a verdict as a TitanOps event.
func (v *Verifier) emitVerdict(ctx context.Context, verdict Verdict) error {
	payloadBytes, _, err := SerializePayload(verdict)
	if err != nil {
		return fmt.Errorf("serializing verdict: %w", err)
	}

	severity := SeverityInformational
	if verdict.Status == VerdictRegression {
		severity = SeverityHigh
	}

	event := export.Event{
		Module:    ModuleName,
		EventType: EventTypeDeploymentVerification,
		Severity:  severity,
		Payload:   payloadBytes,
		Namespace: verdict.Change.Namespace,
		Labels: map[string]string{
			"workload":  verdict.Change.WorkloadName,
			"kind":      verdict.Change.WorkloadKind,
			"verdict":   string(verdict.Status),
			"commit_sha": verdict.Change.CommitSHA,
		},
		Timestamp: verdict.CompletedAt,
	}

	return v.emitter.Emit(ctx, event)
}

// RecentVerdicts returns the most recent n verdicts.
func (v *Verifier) RecentVerdicts(n int) []Verdict {
	v.mu.Lock()
	defer v.mu.Unlock()

	if n > len(v.verdicts) {
		n = len(v.verdicts)
	}
	// Return newest first.
	result := make([]Verdict, n)
	for i := 0; i < n; i++ {
		result[i] = v.verdicts[len(v.verdicts)-1-i]
	}
	return result
}

// VerdictPayload is the JSON payload for verdict events (for SerializePayload).
type VerdictPayload struct {
	VerdictID    string         `json:"verdict_id"`
	Status       VerdictStatus  `json:"status"`
	WorkloadName string         `json:"workload_name"`
	WorkloadKind string         `json:"workload_kind"`
	Namespace    string         `json:"namespace"`
	Summary      string         `json:"summary"`
	Checks       []SignalCheck  `json:"checks"`
	FailedChecks []SignalCheck  `json:"failed_checks,omitempty"`
	Duration     string         `json:"duration"`
	CommitSHA    string         `json:"commit_sha,omitempty"`
}

// MarshalJSON implements custom JSON serialization for Verdict to use in events.
func (vr Verdict) MarshalJSON() ([]byte, error) {
	return json.Marshal(VerdictPayload{
		VerdictID:    vr.VerdictID,
		Status:       vr.Status,
		WorkloadName: vr.Change.WorkloadName,
		WorkloadKind: vr.Change.WorkloadKind,
		Namespace:    vr.Change.Namespace,
		Summary:      vr.Summary,
		Checks:       vr.Checks,
		FailedChecks: vr.FailedChecks,
		Duration:     vr.Duration.String(),
		CommitSHA:    vr.Change.CommitSHA,
	})
}
