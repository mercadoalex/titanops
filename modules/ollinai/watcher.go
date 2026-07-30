package ollinai

import (
	"context"
	"fmt"
	"log"
	"os"
	"sync"
	"time"
)

// WorkloadSnapshot represents the observed state of a workload at a point in time.
// The watcher compares consecutive snapshots to detect changes.
type WorkloadSnapshot struct {
	// Name is the workload name.
	Name string
	// Kind is "Deployment", "StatefulSet", or "DaemonSet".
	Kind string
	// Namespace is the K8s namespace.
	Namespace string
	// Generation is the metadata.generation field (increments on spec change).
	Generation int64
	// ObservedAt is when this snapshot was taken.
	ObservedAt time.Time
	// ImageTags maps container name → image:tag.
	ImageTags map[string]string
	// EnvVars maps container name → env var name → value.
	EnvVars map[string]map[string]string
	// Replicas is the desired replica count.
	Replicas int
	// ResourceLimits maps container name → resource → value (e.g., "cpu" → "500m").
	ResourceLimits map[string]map[string]string
	// ResourceRequests maps container name → resource → value.
	ResourceRequests map[string]map[string]string
	// ProbeFingerprints maps container name → "liveness"|"readiness" → fingerprint string.
	ProbeFingerprints map[string]map[string]string
	// StrategyType is the deployment strategy (e.g., "RollingUpdate", "Recreate").
	StrategyType string
	// Labels are the pod template labels.
	Labels map[string]string
	// Annotations are the pod template annotations.
	Annotations map[string]string
}

// WorkloadSource is the interface for fetching current workload state.
// In production this wraps client-go informers; for testing it's a mock.
type WorkloadSource interface {
	// ListWorkloads returns all current workload snapshots in the cluster.
	ListWorkloads(ctx context.Context) ([]WorkloadSnapshot, error)
}

// DeploymentWatcher polls for workload changes by comparing snapshots.
// When a change is detected, it classifies it and invokes the callback.
type DeploymentWatcher struct {
	source     WorkloadSource
	interval   time.Duration
	logger     *log.Logger
	onChangeFn func(DeploymentChange)

	mu       sync.RWMutex
	previous map[string]WorkloadSnapshot // key: "kind/namespace/name"
}

// DeploymentWatcherConfig configures the watcher.
type DeploymentWatcherConfig struct {
	// Source provides workload snapshots.
	Source WorkloadSource
	// PollInterval is how often to check for changes. Default 10s.
	PollInterval time.Duration
	// OnChange is called when a deployment change is detected.
	OnChange func(DeploymentChange)
	// Logger for messages. Optional.
	Logger *log.Logger
}

// NewDeploymentWatcher creates a watcher that detects workload changes.
func NewDeploymentWatcher(cfg DeploymentWatcherConfig) *DeploymentWatcher {
	interval := cfg.PollInterval
	if interval <= 0 {
		interval = 10 * time.Second
	}
	logger := cfg.Logger
	if logger == nil {
		logger = log.New(os.Stderr, "[ollinai:watcher] ", log.LstdFlags|log.Lmsgprefix)
	}

	return &DeploymentWatcher{
		source:     cfg.Source,
		interval:   interval,
		logger:     logger,
		onChangeFn: cfg.OnChange,
		previous:   make(map[string]WorkloadSnapshot),
	}
}

// Start begins the watch loop. Blocks until ctx is canceled.
func (w *DeploymentWatcher) Start(ctx context.Context) error {
	// Take initial snapshot (baseline — no changes emitted for existing state).
	if err := w.captureBaseline(ctx); err != nil {
		w.logger.Printf("warning: initial baseline capture failed: %v", err)
	}

	ticker := time.NewTicker(w.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			w.poll(ctx)
		}
	}
}

// captureBaseline takes an initial snapshot without emitting changes.
func (w *DeploymentWatcher) captureBaseline(ctx context.Context) error {
	snapshots, err := w.source.ListWorkloads(ctx)
	if err != nil {
		return err
	}

	w.mu.Lock()
	defer w.mu.Unlock()
	for _, snap := range snapshots {
		key := workloadKey(snap)
		w.previous[key] = snap
	}

	w.logger.Printf("baseline captured: %d workloads", len(snapshots))
	return nil
}

// poll fetches current state, diffs against previous, and emits changes.
func (w *DeploymentWatcher) poll(ctx context.Context) {
	snapshots, err := w.source.ListWorkloads(ctx)
	if err != nil {
		w.logger.Printf("poll error: %v", err)
		return
	}

	w.mu.Lock()
	defer w.mu.Unlock()

	for _, current := range snapshots {
		key := workloadKey(current)
		prev, existed := w.previous[key]

		if !existed {
			// New workload — store as baseline, no change event.
			w.previous[key] = current
			continue
		}

		// Only diff if generation changed (K8s increments generation on spec mutation).
		if current.Generation == prev.Generation {
			continue
		}

		changes := diffSnapshots(prev, current)
		if len(changes) > 0 {
			dc := DeploymentChange{
				WorkloadName: current.Name,
				WorkloadKind: current.Kind,
				Namespace:    current.Namespace,
				Changes:      changes,
				DetectedAt:   time.Now().UTC(),
				CommitSHA:    extractAnnotation(current.Annotations, "app.kubernetes.io/commit"),
				Deployer:     extractAnnotation(current.Annotations, "app.kubernetes.io/deployed-by"),
			}

			if w.onChangeFn != nil {
				w.onChangeFn(dc)
			}
		}

		w.previous[key] = current
	}
}

// diffSnapshots compares two workload snapshots and returns classified changes.
func diffSnapshots(prev, curr WorkloadSnapshot) []DetectedChange {
	var changes []DetectedChange

	// Check image changes.
	for container, newImage := range curr.ImageTags {
		oldImage, existed := prev.ImageTags[container]
		if !existed || oldImage != newImage {
			changes = append(changes, ClassifyImageChange(
				fmt.Sprintf("containers[%s].image", container),
				oldImage, newImage,
			))
		}
	}

	// Check env var changes.
	for container, newEnvs := range curr.EnvVars {
		oldEnvs := prev.EnvVars[container]
		for envName, newVal := range newEnvs {
			oldVal := ""
			if oldEnvs != nil {
				oldVal = oldEnvs[envName]
			}
			if oldVal != newVal {
				changes = append(changes, ClassifyEnvChange(
					fmt.Sprintf("containers[%s].env[%s]", container, envName),
					oldVal, newVal,
				))
			}
		}
	}

	// Check replica changes.
	if prev.Replicas != curr.Replicas {
		changes = append(changes, ClassifyReplicaChange(prev.Replicas, curr.Replicas))
	}

	// Check resource limit changes.
	for container, newLimits := range curr.ResourceLimits {
		oldLimits := prev.ResourceLimits[container]
		for resource, newVal := range newLimits {
			oldVal := ""
			if oldLimits != nil {
				oldVal = oldLimits[resource]
			}
			if oldVal != newVal {
				changes = append(changes, ClassifyResourceChange(
					fmt.Sprintf("containers[%s].resources.limits[%s]", container, resource),
					oldVal, newVal,
				))
			}
		}
	}

	// Check resource request changes.
	for container, newReqs := range curr.ResourceRequests {
		oldReqs := prev.ResourceRequests[container]
		for resource, newVal := range newReqs {
			oldVal := ""
			if oldReqs != nil {
				oldVal = oldReqs[resource]
			}
			if oldVal != newVal {
				changes = append(changes, ClassifyResourceChange(
					fmt.Sprintf("containers[%s].resources.requests[%s]", container, resource),
					oldVal, newVal,
				))
			}
		}
	}

	// Check probe changes.
	for container, newProbes := range curr.ProbeFingerprints {
		oldProbes := prev.ProbeFingerprints[container]
		for probeType, newFP := range newProbes {
			oldFP := ""
			if oldProbes != nil {
				oldFP = oldProbes[probeType]
			}
			if oldFP != newFP {
				changes = append(changes, ClassifyProbeChange(
					fmt.Sprintf("containers[%s].%sProbe", container, probeType),
					"probe configuration changed",
				))
			}
		}
	}

	// Check strategy changes.
	if prev.StrategyType != curr.StrategyType && curr.StrategyType != "" {
		changes = append(changes, ClassifyRolloutStrategyChange(prev.StrategyType, curr.StrategyType))
	}

	return changes
}

// workloadKey generates a unique map key for a workload.
func workloadKey(s WorkloadSnapshot) string {
	return s.Kind + "/" + s.Namespace + "/" + s.Name
}

// extractAnnotation safely extracts a value from annotations map.
func extractAnnotation(annotations map[string]string, key string) string {
	if annotations == nil {
		return ""
	}
	return annotations[key]
}
