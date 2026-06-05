package ollinai

import (
	"context"
	"fmt"
	"log"
	"os"
	"sync"
	"sync/atomic"
	"time"
)

const (
	// ConfigReloadInterval is how often the adapter checks for config file changes.
	ConfigReloadInterval = 5 * time.Second

	// ShutdownFlushDeadline is the maximum time allowed for flushing buffered events during shutdown.
	ShutdownFlushDeadline = 30 * time.Second
)

// NATSCloser is the interface for closing a NATS connection.
type NATSCloser interface {
	Close()
}

// Adapter is the top-level component that coordinates polling,
// webhook reception, event emission, and health reporting.
type Adapter struct {
	config      atomic.Value // holds *Config
	configPath  string
	configMtime time.Time

	poller      *Poller
	webhook     *WebhookServer
	emitter     EventEmitter
	healthCheck *HealthChecker
	metrics     *Metrics
	natsCloser  NATSCloser

	logger *log.Logger

	pollerCancel context.CancelFunc
	stopCh       chan struct{}
	done         chan struct{}
}

// AdapterConfig holds the dependencies needed to construct an Adapter.
type AdapterConfig struct {
	// ConfigPath is the file path for the ConfigMap-mounted config file.
	// Used for hot-reload detection via mtime polling.
	ConfigPath string

	// InitialConfig is the validated configuration to start with.
	InitialConfig *Config

	// Poller is the REST API poller component.
	Poller *Poller

	// Webhook is the webhook HTTP receiver component.
	Webhook *WebhookServer

	// Emitter is the event emitter for publishing to NATS.
	Emitter EventEmitter

	// HealthCheck is the health/readiness checker. May be nil.
	HealthCheck *HealthChecker

	// Metrics is the Prometheus metrics component. May be nil.
	Metrics *Metrics

	// NATSCloser is used to close the NATS connection during shutdown. May be nil.
	NATSCloser NATSCloser

	// Logger for adapter messages. If nil, logs to stderr.
	Logger *log.Logger
}

// NewAdapter creates a new Adapter with the given dependencies.
func NewAdapter(cfg AdapterConfig) *Adapter {
	logger := cfg.Logger
	if logger == nil {
		logger = log.New(os.Stderr, "[ollinai-adapter] ", log.LstdFlags)
	}

	a := &Adapter{
		configPath:  cfg.ConfigPath,
		poller:      cfg.Poller,
		webhook:     cfg.Webhook,
		emitter:     cfg.Emitter,
		healthCheck: cfg.HealthCheck,
		metrics:     cfg.Metrics,
		natsCloser:  cfg.NATSCloser,
		logger:      logger,
		stopCh:      make(chan struct{}),
		done:        make(chan struct{}),
	}

	if cfg.InitialConfig != nil {
		a.config.Store(cfg.InitialConfig)
	}

	return a
}

// Config returns the current active configuration.
func (a *Adapter) Config() *Config {
	cfg, _ := a.config.Load().(*Config)
	return cfg
}

// Run starts all sub-components and blocks until ctx is canceled or a fatal error occurs.
// It starts the poller goroutine, webhook server, and config watcher.
func (a *Adapter) Run(ctx context.Context) error {
	var wg sync.WaitGroup
	errCh := make(chan error, 3)

	// Start webhook server
	if a.webhook != nil {
		if err := a.webhook.Start(ctx); err != nil {
			return fmt.Errorf("failed to start webhook server: %w", err)
		}
		a.logger.Println("webhook server started")
	}

	// Start poller in a goroutine
	if a.poller != nil {
		pollerCtx, pollerCancel := context.WithCancel(ctx)
		a.pollerCancel = pollerCancel

		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := a.poller.Start(pollerCtx); err != nil && pollerCtx.Err() == nil {
				errCh <- fmt.Errorf("poller error: %w", err)
			}
		}()
		a.logger.Println("poller started")
	}

	// Start config watcher
	wg.Add(1)
	go func() {
		defer wg.Done()
		a.watchConfig(ctx)
	}()
	a.logger.Println("config watcher started")

	// Wait for context cancellation or fatal error
	select {
	case <-ctx.Done():
		a.logger.Println("context canceled, initiating shutdown")
	case err := <-errCh:
		a.logger.Printf("fatal component error: %v", err)
		return err
	}

	// Signal stop to all watchers
	close(a.stopCh)
	wg.Wait()
	close(a.done)

	return nil
}

// Shutdown performs a graceful shutdown sequence:
// 1. Stop accepting webhooks (server.Shutdown)
// 2. Stop poller (cancel poller context)
// 3. Flush buffer (drain ring buffer → publish) with 30s deadline
// 4. Close NATS connection
func (a *Adapter) Shutdown(ctx context.Context) error {
	a.logger.Println("shutdown initiated")

	// Step 1: Stop webhook server (stop accepting new requests)
	if a.webhook != nil {
		a.logger.Println("stopping webhook server...")
		if err := a.webhook.Shutdown(ctx); err != nil {
			a.logger.Printf("warning: webhook shutdown error: %v", err)
		}
	}

	// Step 2: Stop poller
	if a.pollerCancel != nil {
		a.logger.Println("stopping poller...")
		a.pollerCancel()
	}

	// Step 3: Flush buffered events with deadline
	if a.emitter != nil {
		a.logger.Println("flushing buffered events...")
		flushCtx, flushCancel := context.WithTimeout(ctx, ShutdownFlushDeadline)
		defer flushCancel()

		if err := a.emitter.Flush(flushCtx); err != nil {
			if flushCtx.Err() != nil {
				a.logger.Printf("warning: flush deadline exceeded (30s), discarding remaining events")
			} else {
				a.logger.Printf("warning: flush error: %v", err)
			}
		} else {
			a.logger.Println("buffer flush complete")
		}
	}

	// Step 4: Close NATS connection
	if a.natsCloser != nil {
		a.logger.Println("closing NATS connection...")
		a.natsCloser.Close()
	}

	a.logger.Println("shutdown complete")
	return nil
}

// watchConfig polls the config file for mtime changes and performs hot-reload.
// Uses a simple file-polling approach (check file mtime every 5s).
func (a *Adapter) watchConfig(ctx context.Context) {
	if a.configPath == "" {
		return
	}

	// Initialize mtime from current file
	if info, err := os.Stat(a.configPath); err == nil {
		a.configMtime = info.ModTime()
	}

	ticker := time.NewTicker(ConfigReloadInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-a.stopCh:
			return
		case <-ticker.C:
			a.checkConfigReload()
		}
	}
}

// checkConfigReload checks if the config file has changed and reloads if needed.
func (a *Adapter) checkConfigReload() {
	info, err := os.Stat(a.configPath)
	if err != nil {
		// File might not exist yet or be temporarily unavailable; skip
		return
	}

	mtime := info.ModTime()
	if !mtime.After(a.configMtime) {
		// No change
		return
	}

	a.logger.Printf("config file change detected (mtime: %s), reloading...", mtime.Format(time.RFC3339))

	// Reload config
	newCfg, errs := LoadConfig(a.configPath)
	if errs != nil {
		// Invalid config: log warning and keep previous
		a.logger.Printf("warning: config reload failed, keeping previous config. Errors:")
		for _, e := range errs {
			a.logger.Printf("  - %v", e)
		}
		// Update mtime to avoid repeated reload attempts for the same bad file
		a.configMtime = mtime
		return
	}

	// Valid config: atomic swap
	a.config.Store(newCfg)
	a.configMtime = mtime
	a.logger.Println("config reloaded successfully")
}
