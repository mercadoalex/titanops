// Package main is the TitanOps platform entry point that wires together
// all platform components: the correlation engine, API gateway, export adapters,
// and AI provider. This demonstrates the integration architecture where:
//
//   modules → event bus → correlation engine → export adapters
//   correlation engine → API gateway → dashboard
//
// The cmd package imports everything; nothing imports cmd (one-way dependency direction).
package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/mercadoalex/titanops/correlation"
	"github.com/mercadoalex/titanops/gateway"
	ai "github.com/mercadoalex/titanops/shared/titanops-ai"
	export "github.com/mercadoalex/titanops/shared/titanops-export"
)

// PlatformConfig holds the top-level configuration for the TitanOps platform.
type PlatformConfig struct {
	// HTTPAddr is the address for the API gateway to listen on.
	HTTPAddr string
	// ModelDir is the path to ONNX model files for AI inference.
	ModelDir string
	// CorrelationTimeWindow is the correlation engine's sliding window.
	CorrelationTimeWindow time.Duration
	// ConfidenceThreshold is the minimum confidence for auto-action execution.
	ConfidenceThreshold int
	// ExportConfig holds the telemetry export backend configuration.
	ExportConfig export.Config
}

// DefaultPlatformConfig returns sensible defaults for platform configuration.
func DefaultPlatformConfig() PlatformConfig {
	return PlatformConfig{
		HTTPAddr:              ":8080",
		ModelDir:              "/opt/titanops/models",
		CorrelationTimeWindow: 120 * time.Second,
		ConfidenceThreshold:   80,
		ExportConfig: export.Config{
			Prometheus: &export.PrometheusConfig{
				Enabled: true,
				Port:    9090,
			},
		},
	}
}

// loadConfig loads platform configuration from environment variables with defaults.
func loadConfig() PlatformConfig {
	cfg := DefaultPlatformConfig()

	if addr := os.Getenv("TITANOPS_HTTP_ADDR"); addr != "" {
		cfg.HTTPAddr = addr
	}
	if dir := os.Getenv("TITANOPS_MODEL_DIR"); dir != "" {
		cfg.ModelDir = dir
	}

	return cfg
}

func main() {
	log.SetFlags(log.LstdFlags | log.Lshortfile)
	log.Println("TitanOps Platform starting...")

	// 1. Load configuration
	cfg := loadConfig()
	log.Printf("Config: addr=%s, models=%s, window=%s, threshold=%d",
		cfg.HTTPAddr, cfg.ModelDir, cfg.CorrelationTimeWindow, cfg.ConfidenceThreshold)

	// 2. Create AI provider (local ONNX inference, zero cloud dependencies)
	var aiProvider ai.Provider
	provider, err := ai.NewLocalProvider(cfg.ModelDir)
	if err != nil {
		log.Printf("WARN: AI provider unavailable (models dir: %s): %v", cfg.ModelDir, err)
		log.Println("WARN: AI predictions will not be available until models are configured")
	} else {
		aiProvider = provider
		log.Println("AI provider initialized (local ONNX)")
	}

	// 3. Create export adapters (fan-out telemetry to all configured backends)
	exporter := createExporter(cfg.ExportConfig)
	log.Println("Export adapters initialized")

	// 4. Create correlation engine (cross-module event correlation)
	correlationCfg := correlation.EngineConfig{
		TimeWindow:          cfg.CorrelationTimeWindow,
		ConfidenceThreshold: cfg.ConfidenceThreshold,
		AutoActions: []correlation.AutoActionConfig{
			{Type: "isolate_pod", Enabled: true},
			{Type: "alert_operator", Enabled: true},
			{Type: "forensic_report", Enabled: true},
		},
	}

	actionExecutor := &correlation.NoOpExecutor{} // Replace with real executor in production
	engine, err := correlation.NewEngine(correlationCfg, exporter, actionExecutor)
	if err != nil {
		log.Fatalf("Failed to create correlation engine: %v", err)
	}
	log.Printf("Correlation engine initialized (window=%s, threshold=%d)",
		correlationCfg.TimeWindow, correlationCfg.ConfidenceThreshold)

	// 5. Create API gateway (serves dashboard and exposes platform state)
	gw := gateway.NewGateway()
	log.Println("API gateway initialized")

	// 6. Register routes on HTTP mux
	mux := http.NewServeMux()
	gw.RegisterRoutes(mux)

	// Health check endpoint for Kubernetes readiness/liveness probes
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		fmt.Fprintf(w, "ok")
	})

	// Platform info endpoint
	mux.HandleFunc("/api/info", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(w, `{"platform":"titanops","version":"0.1.0","components":{"correlation":true,"gateway":true,"ai":%t}}`,
			aiProvider != nil)
	})

	log.Println("Routes registered: /api/health, /api/actions, /api/correlations, /api/overrides, /api/audit, /api/explain/, /healthz, /api/info")

	// 7. Start HTTP server with graceful shutdown
	server := &http.Server{
		Addr:         cfg.HTTPAddr,
		Handler:      mux,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 15 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	// Channel to signal shutdown
	shutdown := make(chan os.Signal, 1)
	signal.Notify(shutdown, syscall.SIGINT, syscall.SIGTERM)

	// Start server in background
	go func() {
		log.Printf("HTTP server listening on %s", cfg.HTTPAddr)
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("HTTP server error: %v", err)
		}
	}()

	// Log integration summary
	log.Println("=== TitanOps Platform Integration ===")
	log.Println("  Modules → Event Bus → Correlation Engine → Export Adapters")
	log.Println("  Correlation Engine → API Gateway → Dashboard")
	log.Println("  One-way dependency: cmd imports all; nothing imports cmd")
	log.Println("======================================")

	// Wait for shutdown signal
	sig := <-shutdown
	log.Printf("Shutdown signal received: %v", sig)

	// Graceful shutdown with timeout
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := server.Shutdown(ctx); err != nil {
		log.Fatalf("Server shutdown failed: %v", err)
	}

	log.Println("TitanOps Platform stopped")

	// Suppress unused variable warnings for components used in integration
	_ = engine
	_ = aiProvider
	_ = exporter
}

// createExporter builds the multi-exporter from the platform configuration.
func createExporter(cfg export.Config) export.Exporter {
	var backends []export.Backend

	if cfg.Prometheus != nil && cfg.Prometheus.Enabled {
		backends = append(backends, &prometheusBackend{port: cfg.Prometheus.Port})
	}
	if cfg.OTLP != nil && cfg.OTLP.Enabled {
		backends = append(backends, &otlpBackend{endpoint: cfg.OTLP.Endpoint})
	}
	if cfg.Splunk != nil && cfg.Splunk.Enabled {
		backends = append(backends, &splunkBackend{hecURL: cfg.Splunk.HECUrl})
	}
	if cfg.Dynatrace != nil && cfg.Dynatrace.Enabled {
		backends = append(backends, &dynatraceBackend{apiURL: cfg.Dynatrace.APIUrl})
	}
	for _, wh := range cfg.Webhooks {
		backends = append(backends, &webhookBackend{endpoint: wh.Endpoint, events: wh.Events})
	}

	return export.NewMultiExporter(backends...)
}

// Backend stubs - these demonstrate the wiring; real implementations
// are in the titanops-export library.

type prometheusBackend struct{ port int }

func (b *prometheusBackend) Name() string                                    { return "prometheus" }
func (b *prometheusBackend) Send(_ context.Context, _ export.Event) error    { return nil }
func (b *prometheusBackend) IsEnabled() bool                                 { return true }

type otlpBackend struct{ endpoint string }

func (b *otlpBackend) Name() string                                    { return "otlp" }
func (b *otlpBackend) Send(_ context.Context, _ export.Event) error    { return nil }
func (b *otlpBackend) IsEnabled() bool                                 { return true }

type splunkBackend struct{ hecURL string }

func (b *splunkBackend) Name() string                                    { return "splunk" }
func (b *splunkBackend) Send(_ context.Context, _ export.Event) error    { return nil }
func (b *splunkBackend) IsEnabled() bool                                 { return true }

type dynatraceBackend struct{ apiURL string }

func (b *dynatraceBackend) Name() string                                    { return "dynatrace" }
func (b *dynatraceBackend) Send(_ context.Context, _ export.Event) error    { return nil }
func (b *dynatraceBackend) IsEnabled() bool                                 { return true }

type webhookBackend struct {
	endpoint string
	events   []string
}

func (b *webhookBackend) Name() string                                    { return "webhook" }
func (b *webhookBackend) Send(_ context.Context, _ export.Event) error    { return nil }
func (b *webhookBackend) IsEnabled() bool                                 { return true }
