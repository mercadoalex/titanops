package ebeecontrol

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	export "github.com/mercadoalex/titanops/shared/titanops-export"
	platform "github.com/mercadoalex/titanops/shared/titanops-platform"
)

const (
	ebeecontrolModuleID = "ebeecontrol"
	ebeecontrolVersion  = "v0.1.0"
)

// EbeecontrolModule adapts the eBeeControl Agent to the platform.Module interface,
// allowing the kernel to manage its lifecycle, health, and event routing.
type EbeecontrolModule struct {
	agent  *Agent
	kernel platform.Kernel
}

// NewModule creates a new EbeecontrolModule ready for registration with the kernel.
func NewModule() platform.Module {
	return &EbeecontrolModule{}
}

// ID returns the module identifier.
func (m *EbeecontrolModule) ID() string { return ebeecontrolModuleID }

// Version returns the module's semantic version.
func (m *EbeecontrolModule) Version() string { return ebeecontrolVersion }

// Start initializes the eBeeControl agent using shared services from the kernel.
// Configuration is read from the kernel's ModuleConfig; the Dynatrace client,
// deployer, tetragon monitor, and trainer are created internally.
func (m *EbeecontrolModule) Start(ctx context.Context, kernel platform.Kernel) error {
	m.kernel = kernel

	// Parse module-specific configuration.
	cfg := DefaultConfig()
	if raw := kernel.ModuleConfig(ebeecontrolModuleID); raw != nil {
		if err := json.Unmarshal(raw, &cfg); err != nil {
			return fmt.Errorf("parsing ebeecontrol config: %w", err)
		}
	}
	if err := cfg.Validate(); err != nil {
		return fmt.Errorf("invalid ebeecontrol config: %w", err)
	}

	// Create components using kernel services.
	dynatrace := NewDynatraceClient(DynatraceClientConfig{
		EndpointURL:      cfg.Notifications.ChannelEndpoint,
		DiscoveryTimeout: 30 * time.Second,
		MaxRetries:       cfg.DynatraceIngestion.Retry.MaxRetries,
		ContextTimeout:   3 * time.Second,
	}, nil)

	deployer := NewInMemoryDeployer() // Use K8sDeployer in production via config
	tetragon := NewTetragonMonitor(cfg.Tetragon, nil)

	trainer, err := NewTrainer(TrainerConfig{
		RetrainingInterval:    cfg.Learning.RetrainingInterval,
		MinimumOutcomeRecords: cfg.Learning.MinimumOutcomeRecords,
	}, nil)
	if err != nil {
		return fmt.Errorf("creating trainer: %w", err)
	}

	// Wrap kernel.Emit as the EventEmitter.
	emitter := &platformEmitter{kernel: kernel}

	agent, err := NewAgent(cfg, AgentDeps{
		Dynatrace: dynatrace,
		Deployer:  deployer,
		Tetragon:  tetragon,
		Trainer:   trainer,
		Emitter:   emitter,
		IsolatePod: func(ctx context.Context, podID string) error {
			// Delegate to kernel's K8s client when available.
			// In production, this applies a deny-all NetworkPolicy.
			return nil
		},
		BlockIP: func(ctx context.Context, podID string) error {
			return nil
		},
		DeployHoneytokens: func(ctx context.Context, namespace string, count int) error {
			_, deployErr := deployer.Deploy(ctx, DeploymentRequest{
				PodID:     "reactive-deploy",
				Namespace: namespace,
				Honeytokens: []HoneytokenSpec{
					{Type: HoneytokenDecoySecret, Name: "token", Placement: "/var/run/secrets/reactive/token"},
					{Type: HoneytokenDecoyFile, Name: "creds", Placement: "/home/app/.aws/credentials"},
				},
			})
			return deployErr
		},
		SendAlert: func(ctx context.Context, message string) error {
			logger := kernel.Logger(ebeecontrolModuleID)
			logger.Warn("alert", "message", message)
			return nil
		},
	})
	if err != nil {
		return fmt.Errorf("creating ebeecontrol agent: %w", err)
	}

	m.agent = agent

	if err := m.agent.Start(ctx); err != nil {
		return fmt.Errorf("starting ebeecontrol agent: %w", err)
	}

	logger := kernel.Logger(ebeecontrolModuleID)
	logger.Info("module started",
		"discovery_interval", cfg.Discovery.Interval,
		"retraining_interval", cfg.Learning.RetrainingInterval,
	)

	return nil
}

// Stop gracefully shuts down the eBeeControl agent.
func (m *EbeecontrolModule) Stop(_ context.Context) error {
	if m.agent != nil {
		m.agent.Stop()
	}
	return nil
}

// HealthCheck reports the eBeeControl module's health.
func (m *EbeecontrolModule) HealthCheck(_ context.Context) platform.HealthStatus {
	if m.agent == nil {
		return platform.HealthStatus{
			State:     platform.HealthUnhealthy,
			Message:   "agent not initialized",
			LastCheck: time.Now().UTC(),
		}
	}

	components := map[string]platform.ComponentHealth{
		"tetragon": {
			State:   boolToHealth(m.agent.tetragon.IsConnected()),
			Message: fmt.Sprintf("buffer: %s", m.agent.tetragon.BufferStatus()),
		},
	}

	return platform.HealthStatus{
		State:      platform.HealthHealthy,
		Message:    "agent running",
		Components: components,
		LastCheck:  time.Now().UTC(),
	}
}

// boolToHealth maps a boolean to a HealthState.
func boolToHealth(ok bool) platform.HealthState {
	if ok {
		return platform.HealthHealthy
	}
	return platform.HealthDegraded
}

// --- platformEmitter adapts platform.Kernel.Emit to the ebeecontrol.EventEmitter interface ---

type platformEmitter struct {
	kernel platform.Kernel
}

func (e *platformEmitter) Emit(ctx context.Context, event export.Event) error {
	return e.kernel.Emit(ctx, event)
}
