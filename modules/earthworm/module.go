package earthworm

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	export "github.com/mercadoalex/titanops/shared/titanops-export"
	platform "github.com/mercadoalex/titanops/shared/titanops-platform"
)

const (
	earthwormModuleID = "earthworm"
	moduleVersion     = "v0.1.0"
)

// EarthwormModule adapts the Earthworm Agent to the platform.Module interface,
// allowing the kernel to manage its lifecycle, health, and event routing.
type EarthwormModule struct {
	agent  *Agent
	kernel platform.Kernel
	cancel context.CancelFunc
}

// New creates a new EarthwormModule ready for registration with the kernel.
func New() platform.Module {
	return &EarthwormModule{}
}

// ID returns the module identifier.
func (m *EarthwormModule) ID() string { return earthwormModuleID }

// Version returns the module's semantic version.
func (m *EarthwormModule) Version() string { return moduleVersion }

// Start initializes the Earthworm agent using shared services from the kernel.
// Configuration is read from the kernel's ModuleConfig; AI provider and K8s client
// are obtained from the kernel's shared services.
func (m *EarthwormModule) Start(ctx context.Context, kernel platform.Kernel) error {
	m.kernel = kernel

	// Parse module-specific configuration.
	cfg := AgentConfig{}
	if raw := kernel.ModuleConfig(earthwormModuleID); raw != nil {
		if err := json.Unmarshal(raw, &cfg); err != nil {
			return fmt.Errorf("parsing earthworm config: %w", err)
		}
	}

	// Validate config (applies defaults for zero values).
	if err := cfg.ValidateConfig(); err != nil {
		return fmt.Errorf("invalid earthworm config: %w", err)
	}

	// Create the agent with kernel-provided services.
	provider := kernel.AIProvider()
	k8sClient := kernel.K8sClient()

	// Wrap kernel.Emit as an EventEmitter.
	emitter := &kernelEmitter{kernel: kernel}

	agent, err := NewAgent(cfg, provider, emitter, k8sClient)
	if err != nil {
		return fmt.Errorf("creating earthworm agent: %w", err)
	}

	m.agent = agent

	logger := kernel.Logger(earthwormModuleID)
	logger.Info("module started", "node", cfg.NodeID, "threshold", cfg.Threshold)

	return nil
}

// Stop gracefully shuts down the Earthworm agent.
func (m *EarthwormModule) Stop(_ context.Context) error {
	if m.cancel != nil {
		m.cancel()
	}
	m.agent = nil
	return nil
}

// HealthCheck reports the Earthworm module's health.
func (m *EarthwormModule) HealthCheck(_ context.Context) platform.HealthStatus {
	if m.agent == nil {
		return platform.HealthStatus{
			State:     platform.HealthUnhealthy,
			Message:   "agent not initialized",
			LastCheck: time.Now().UTC(),
		}
	}
	return platform.HealthStatus{
		State:     platform.HealthHealthy,
		Message:   "agent running",
		LastCheck: time.Now().UTC(),
	}
}

// ProcessHeartbeat exposes the agent's heartbeat processing for external callers
// (e.g., the eBPF event ingestion loop). This is the module's hot path.
func (m *EarthwormModule) ProcessHeartbeat(ctx context.Context, heartbeat HeartbeatSignal) (*ActionResult, error) {
	if m.agent == nil {
		return nil, fmt.Errorf("agent not started")
	}
	return m.agent.ProcessHeartbeat(ctx, heartbeat)
}

// --- kernelEmitter adapts platform.Kernel.Emit to the earthworm.EventEmitter interface ---

type kernelEmitter struct {
	kernel platform.Kernel
}

func (e *kernelEmitter) Emit(ctx context.Context, event export.Event) error {
	return e.kernel.Emit(ctx, event)
}
