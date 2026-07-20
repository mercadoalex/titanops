package platform

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"sync"
	"time"

	ai "github.com/mercadoalex/titanops/shared/titanops-ai"
	export "github.com/mercadoalex/titanops/shared/titanops-export"
	k8s "github.com/mercadoalex/titanops/shared/titanops-k8s"
)

const (
	// moduleStartTimeout is the maximum time a module has to start.
	moduleStartTimeout = 30 * time.Second
	// moduleStopTimeout is the maximum time a module has to stop.
	moduleStopTimeout = 15 * time.Second
	// healthCheckTimeout is the maximum time for a health check response.
	healthCheckTimeout = 5 * time.Second
)

// RuntimeKernel is the platform kernel implementation that manages module
// lifecycle, provides shared services, and routes events.
type RuntimeKernel struct {
	mu      sync.RWMutex
	modules map[string]moduleEntry
	subs    []subscription
	logger  *log.Logger

	// Services provided to modules (set before Start).
	emitter   EventEmitter
	k8sClient k8s.Client
	aiProv    ai.Provider
	configs   map[string]json.RawMessage
	clusterID string
}

// moduleEntry tracks a registered module and its state.
type moduleEntry struct {
	module  Module
	state   ModuleState
	started time.Time
}

// ModuleState represents the lifecycle state of a module within the kernel.
type ModuleState string

const (
	ModuleRegistered ModuleState = "registered"
	ModuleStarting   ModuleState = "starting"
	ModuleRunning    ModuleState = "running"
	ModuleStopping   ModuleState = "stopping"
	ModuleStopped    ModuleState = "stopped"
	ModuleFailed     ModuleState = "failed"
)

// subscription is an internal event subscription.
type subscription struct {
	filter  EventFilter
	handler EventHandler
	closed  bool
}

// EventEmitter is the underlying event transport (NATS, channel, etc.).
type EventEmitter interface {
	Emit(ctx context.Context, event export.Event) error
}

// NewKernel creates a new RuntimeKernel.
func NewKernel(clusterID string) *RuntimeKernel {
	return &RuntimeKernel{
		modules:   make(map[string]moduleEntry),
		configs:   make(map[string]json.RawMessage),
		clusterID: clusterID,
		logger:    log.New(os.Stderr, "[titanops:kernel] ", log.LstdFlags|log.Lmsgprefix),
	}
}

// SetEmitter configures the event transport backend.
func (k *RuntimeKernel) SetEmitter(e EventEmitter) {
	k.emitter = e
}

// SetModuleConfig sets the raw JSON config for a module.
func (k *RuntimeKernel) SetModuleConfig(moduleID string, cfg json.RawMessage) {
	k.configs[moduleID] = cfg
}

// Register adds a module to the kernel. Must be called before Start.
func (k *RuntimeKernel) Register(m Module) error {
	k.mu.Lock()
	defer k.mu.Unlock()

	id := m.ID()
	if _, exists := k.modules[id]; exists {
		return fmt.Errorf("module %q already registered", id)
	}

	k.modules[id] = moduleEntry{
		module: m,
		state:  ModuleRegistered,
	}
	k.logger.Printf("registered module: %s %s", id, m.Version())
	return nil
}

// StartAll starts all registered modules with fault isolation.
// A module that panics or fails to start is marked as failed; other modules continue.
func (k *RuntimeKernel) StartAll(ctx context.Context) []error {
	k.mu.Lock()
	ids := make([]string, 0, len(k.modules))
	for id := range k.modules {
		ids = append(ids, id)
	}
	k.mu.Unlock()

	var errs []error
	for _, id := range ids {
		if err := k.startModule(ctx, id); err != nil {
			errs = append(errs, err)
		}
	}

	running := 0
	k.mu.RLock()
	for _, entry := range k.modules {
		if entry.state == ModuleRunning {
			running++
		}
	}
	k.mu.RUnlock()

	k.logger.Printf("started %d/%d modules", running, len(ids))
	return errs
}

// startModule starts a single module with panic recovery and timeout.
func (k *RuntimeKernel) startModule(ctx context.Context, id string) (retErr error) {
	k.mu.Lock()
	entry, exists := k.modules[id]
	if !exists {
		k.mu.Unlock()
		return fmt.Errorf("module %q not registered", id)
	}
	entry.state = ModuleStarting
	k.modules[id] = entry
	k.mu.Unlock()

	// Panic recovery — a module must never crash the platform.
	defer func() {
		if r := recover(); r != nil {
			retErr = fmt.Errorf("module %q panicked during start: %v", id, r)
			k.logger.Printf("PANIC in module %s: %v", id, r)
			k.mu.Lock()
			entry.state = ModuleFailed
			k.modules[id] = entry
			k.mu.Unlock()
		}
	}()

	startCtx, cancel := context.WithTimeout(ctx, moduleStartTimeout)
	defer cancel()

	if err := entry.module.Start(startCtx, k); err != nil {
		k.mu.Lock()
		entry.state = ModuleFailed
		k.modules[id] = entry
		k.mu.Unlock()
		k.logger.Printf("module %s failed to start: %v", id, err)
		return fmt.Errorf("module %q start failed: %w", id, err)
	}

	k.mu.Lock()
	entry.state = ModuleRunning
	entry.started = time.Now().UTC()
	k.modules[id] = entry
	k.mu.Unlock()

	k.logger.Printf("module %s started", id)
	return nil
}

// StopAll gracefully stops all running modules.
func (k *RuntimeKernel) StopAll(ctx context.Context) {
	k.mu.Lock()
	ids := make([]string, 0, len(k.modules))
	for id, entry := range k.modules {
		if entry.state == ModuleRunning {
			ids = append(ids, id)
		}
	}
	k.mu.Unlock()

	for _, id := range ids {
		k.stopModule(ctx, id)
	}

	k.logger.Println("all modules stopped")
}

// stopModule stops a single module with timeout and panic recovery.
func (k *RuntimeKernel) stopModule(ctx context.Context, id string) {
	k.mu.Lock()
	entry, exists := k.modules[id]
	if !exists {
		k.mu.Unlock()
		return
	}
	entry.state = ModuleStopping
	k.modules[id] = entry
	k.mu.Unlock()

	defer func() {
		if r := recover(); r != nil {
			k.logger.Printf("PANIC in module %s during stop: %v", id, r)
		}
		k.mu.Lock()
		entry.state = ModuleStopped
		k.modules[id] = entry
		k.mu.Unlock()
	}()

	stopCtx, cancel := context.WithTimeout(ctx, moduleStopTimeout)
	defer cancel()

	if err := entry.module.Stop(stopCtx); err != nil {
		k.logger.Printf("module %s stop error: %v", id, err)
	} else {
		k.logger.Printf("module %s stopped", id)
	}
}

// HealthCheckAll runs health checks on all running modules and returns results.
func (k *RuntimeKernel) HealthCheckAll(ctx context.Context) map[string]HealthStatus {
	k.mu.RLock()
	running := make(map[string]Module)
	for id, entry := range k.modules {
		if entry.state == ModuleRunning {
			running[id] = entry.module
		}
	}
	k.mu.RUnlock()

	results := make(map[string]HealthStatus, len(running))
	var mu sync.Mutex
	var wg sync.WaitGroup

	for id, m := range running {
		wg.Add(1)
		go func(id string, m Module) {
			defer wg.Done()
			status := k.checkModuleHealth(ctx, id, m)
			mu.Lock()
			results[id] = status
			mu.Unlock()
		}(id, m)
	}

	wg.Wait()
	return results
}

// checkModuleHealth runs a single health check with timeout and panic recovery.
func (k *RuntimeKernel) checkModuleHealth(ctx context.Context, id string, m Module) (status HealthStatus) {
	defer func() {
		if r := recover(); r != nil {
			status = HealthStatus{
				State:     HealthUnhealthy,
				Message:   fmt.Sprintf("health check panicked: %v", r),
				LastCheck: time.Now().UTC(),
			}
		}
	}()

	checkCtx, cancel := context.WithTimeout(ctx, healthCheckTimeout)
	defer cancel()

	return m.HealthCheck(checkCtx)
}

// --- Kernel Interface Implementation ---

// Emit publishes an event via the configured emitter.
func (k *RuntimeKernel) Emit(ctx context.Context, event export.Event) error {
	if k.emitter == nil {
		return fmt.Errorf("no event emitter configured")
	}

	// Also dispatch to local subscribers.
	k.mu.RLock()
	subs := make([]subscription, len(k.subs))
	copy(subs, k.subs)
	k.mu.RUnlock()

	for _, sub := range subs {
		if sub.closed {
			continue
		}
		if matchesFilter(event, sub.filter) {
			go sub.handler(ctx, event)
		}
	}

	return k.emitter.Emit(ctx, event)
}

// Subscribe registers an event handler for matching events.
func (k *RuntimeKernel) Subscribe(filter EventFilter, handler EventHandler) Subscription {
	k.mu.Lock()
	defer k.mu.Unlock()

	sub := &kernelSubscription{
		kernel: k,
		index:  len(k.subs),
	}
	k.subs = append(k.subs, subscription{
		filter:  filter,
		handler: handler,
	})
	return sub
}

// K8sClient returns the shared Kubernetes client.
func (k *RuntimeKernel) K8sClient() k8s.Client {
	return k.k8sClient
}

// SetK8sClient configures the shared Kubernetes client.
func (k *RuntimeKernel) SetK8sClient(c k8s.Client) {
	k.k8sClient = c
}

// AIProvider returns the shared AI provider.
func (k *RuntimeKernel) AIProvider() ai.Provider {
	return k.aiProv
}

// SetAIProvider configures the shared AI provider.
func (k *RuntimeKernel) SetAIProvider(p ai.Provider) {
	k.aiProv = p
}

// ModuleConfig returns the raw JSON configuration for a module.
func (k *RuntimeKernel) ModuleConfig(moduleID string) json.RawMessage {
	return k.configs[moduleID]
}

// GetModule returns a registered module by ID, or nil if not found/not running.
func (k *RuntimeKernel) GetModule(id string) Module {
	k.mu.RLock()
	defer k.mu.RUnlock()
	entry, exists := k.modules[id]
	if !exists || entry.state != ModuleRunning {
		return nil
	}
	return entry.module
}

// ClusterID returns the cluster identifier.
func (k *RuntimeKernel) ClusterID() string {
	return k.clusterID
}

// Logger returns a prefixed logger for the given module.
func (k *RuntimeKernel) Logger(moduleID string) Logger {
	return &prefixLogger{
		logger: log.New(os.Stderr, fmt.Sprintf("[%s] ", moduleID), log.LstdFlags|log.Lmsgprefix),
	}
}

// --- Subscription Implementation ---

type kernelSubscription struct {
	kernel *RuntimeKernel
	index  int
}

func (s *kernelSubscription) Close() {
	s.kernel.mu.Lock()
	defer s.kernel.mu.Unlock()
	if s.index < len(s.kernel.subs) {
		s.kernel.subs[s.index].closed = true
	}
}

// --- Logger Implementation ---

type prefixLogger struct {
	logger *log.Logger
}

func (l *prefixLogger) Info(msg string, keysAndValues ...interface{}) {
	l.logger.Printf("INFO  %s %v", msg, keysAndValues)
}

func (l *prefixLogger) Warn(msg string, keysAndValues ...interface{}) {
	l.logger.Printf("WARN  %s %v", msg, keysAndValues)
}

func (l *prefixLogger) Error(msg string, keysAndValues ...interface{}) {
	l.logger.Printf("ERROR %s %v", msg, keysAndValues)
}

// --- Helpers ---

// matchesFilter checks if an event passes a subscription filter.
func matchesFilter(event export.Event, filter EventFilter) bool {
	if filter.Module != "" && event.Module != filter.Module {
		return false
	}
	if filter.EventType != "" && event.EventType != filter.EventType {
		return false
	}
	if filter.MinSeverity != "" {
		if severityRank(event.Severity) < severityRank(filter.MinSeverity) {
			return false
		}
	}
	return true
}

// severityRank maps severity strings to numeric ranks for comparison.
func severityRank(severity string) int {
	switch severity {
	case "critical":
		return 5
	case "high":
		return 4
	case "medium":
		return 3
	case "low":
		return 2
	case "informational":
		return 1
	default:
		return 0
	}
}
