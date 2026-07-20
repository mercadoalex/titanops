package ebeecontrol

import (
	"fmt"
	"time"
)

// Config holds the full configuration for the eBeeControl module.
// All intervals and timeouts are configurable with sensible defaults.
type Config struct {
	Discovery          DiscoveryConfig          `json:"discovery" yaml:"discovery"`
	HealthCheck        HealthCheckConfig        `json:"health_check" yaml:"health_check"`
	Deployment         DeploymentConfig         `json:"deployment" yaml:"deployment"`
	Response           ResponseConfig           `json:"response" yaml:"response"`
	Reporting          ReportingConfig          `json:"reporting" yaml:"reporting"`
	Learning           LearningConfig           `json:"learning" yaml:"learning"`
	AuditLog           AuditLogConfig           `json:"audit_log" yaml:"audit_log"`
	Notifications      NotificationsConfig      `json:"notifications" yaml:"notifications"`
	DynatraceIngestion DynatraceIngestionConfig `json:"dynatrace_ingestion" yaml:"dynatrace_ingestion"`
	Tetragon           TetragonConfig           `json:"tetragon" yaml:"tetragon"`
}

// DiscoveryConfig controls the service discovery cycle.
type DiscoveryConfig struct {
	// Interval between discovery cycles. Range: 5m-1440m, default 60m.
	Interval time.Duration `json:"interval" yaml:"interval" validate:"min=300000000000,max=86400000000000"`
}

// HealthCheckConfig controls health monitoring.
type HealthCheckConfig struct {
	// Interval between health checks. Default 30s.
	Interval time.Duration `json:"interval" yaml:"interval"`
	// Timeout for health check responses. Default 5s.
	ResponseTimeout time.Duration `json:"response_timeout" yaml:"response_timeout"`
	// Timeout for individual component checks. Default 10s.
	ComponentTimeout time.Duration `json:"component_timeout" yaml:"component_timeout"`
}

// DeploymentConfig controls honeytoken deployment.
type DeploymentConfig struct {
	// Maximum honeytokens per pod. Range: 1-5, default 5.
	MaxHoneytokensPerPod int `json:"max_honeytokens_per_pod" yaml:"max_honeytokens_per_pod" validate:"min=1,max=5"`
	// Timeout for deployment operations. Default 30s.
	Timeout time.Duration `json:"timeout" yaml:"timeout"`
}

// ResponseConfig controls threat response execution.
type ResponseConfig struct {
	// Timeout for pod isolation operations. Default 10s.
	IsolationTimeout time.Duration `json:"isolation_timeout" yaml:"isolation_timeout"`
	// Max retries for pod isolation. Default 3.
	IsolationMaxRetries int `json:"isolation_max_retries" yaml:"isolation_max_retries"`
	// Interval between isolation retries. Default 5s.
	IsolationRetryInterval time.Duration `json:"isolation_retry_interval" yaml:"isolation_retry_interval"`
	// Max retries for IP blocking. Default 3.
	IPBlockMaxRetries int `json:"ip_block_max_retries" yaml:"ip_block_max_retries"`
	// Interval between IP block retries. Default 5s.
	IPBlockRetryInterval time.Duration `json:"ip_block_retry_interval" yaml:"ip_block_retry_interval"`
}

// ReportingConfig controls forensic report generation.
type ReportingConfig struct {
	// Days to retain forensic reports. Default 90.
	RetentionDays int `json:"retention_days" yaml:"retention_days" validate:"min=1"`
	// Timeout for report generation. Default 60s.
	GenerationTimeout time.Duration `json:"generation_timeout" yaml:"generation_timeout"`
	// Max retries for report generation. Default 3.
	GenerationMaxRetries int `json:"generation_max_retries" yaml:"generation_max_retries"`
}

// LearningConfig controls the AI training feedback loop.
type LearningConfig struct {
	// Interval between retraining cycles. Range: 1h-168h, default 24h.
	RetrainingInterval time.Duration `json:"retraining_interval" yaml:"retraining_interval" validate:"min=3600000000000,max=604800000000000"`
	// Minimum outcome records required before retraining. Default 50.
	MinimumOutcomeRecords int `json:"minimum_outcome_records" yaml:"minimum_outcome_records" validate:"min=1"`
	// Timeout for outcome submission. Default 60s.
	OutcomeSubmissionTimeout time.Duration `json:"outcome_submission_timeout" yaml:"outcome_submission_timeout"`
}

// AuditLogConfig controls audit log retention.
type AuditLogConfig struct {
	// Days to retain audit log entries. Minimum 90.
	RetentionDays int `json:"retention_days" yaml:"retention_days" validate:"min=90"`
}

// NotificationsConfig controls alert delivery.
type NotificationsConfig struct {
	// Endpoint URL for alert notifications.
	ChannelEndpoint string `json:"channel_endpoint" yaml:"channel_endpoint"`
}

// DynatraceIngestionConfig controls metrics and log export to Dynatrace.
type DynatraceIngestionConfig struct {
	// Dynatrace Metrics API v2 endpoint URL.
	MetricsEndpoint string `json:"metrics_endpoint" yaml:"metrics_endpoint"`
	// Dynatrace Log Ingestion API endpoint URL.
	LogEndpoint string `json:"log_endpoint" yaml:"log_endpoint"`
	// Dynatrace API token (metrics.ingest, logs.ingest scopes).
	APIToken string `json:"api_token" yaml:"api_token"`
	// Timeout for ingestion requests. Default 10s.
	RequestTimeout time.Duration `json:"request_timeout" yaml:"request_timeout"`
	// Retry configuration for ingestion.
	Retry RetryConfig `json:"retry" yaml:"retry"`
	// Batch configuration for ingestion.
	Batch BatchConfig `json:"batch" yaml:"batch"`
}

// RetryConfig holds retry parameters with exponential backoff.
type RetryConfig struct {
	// Maximum number of retries. Default 5.
	MaxRetries int `json:"max_retries" yaml:"max_retries" validate:"min=0"`
	// Initial backoff duration. Default 2s.
	InitialBackoff time.Duration `json:"initial_backoff" yaml:"initial_backoff"`
	// Backoff multiplier. Default 2.
	BackoffMultiplier float64 `json:"backoff_multiplier" yaml:"backoff_multiplier" validate:"min=1"`
	// Maximum backoff duration. Default 32s.
	MaxBackoff time.Duration `json:"max_backoff" yaml:"max_backoff"`
}

// BatchConfig holds batching parameters for ingestion.
type BatchConfig struct {
	// Maximum items per batch. Default 100.
	MaxBatchSize int `json:"max_batch_size" yaml:"max_batch_size" validate:"min=1"`
	// Interval between automatic flushes. Default 5s.
	FlushInterval time.Duration `json:"flush_interval" yaml:"flush_interval"`
}

// TetragonConfig controls the Tetragon gRPC client connection.
type TetragonConfig struct {
	// gRPC server address. Default "localhost:54321".
	GRPCAddress string `json:"grpc_address" yaml:"grpc_address"`
	// Reconnect interval on stream failure. Default 5s.
	ReconnectInterval time.Duration `json:"reconnect_interval" yaml:"reconnect_interval"`
	// Maximum reconnect attempts. -1 = unlimited. Default -1.
	MaxReconnectAttempts int `json:"max_reconnect_attempts" yaml:"max_reconnect_attempts"`
	// Event buffer capacity. Default 1000.
	EventBufferCapacity int `json:"event_buffer_capacity" yaml:"event_buffer_capacity" validate:"min=1"`
}

// DefaultConfig returns the default configuration with sensible values.
func DefaultConfig() Config {
	return Config{
		Discovery: DiscoveryConfig{
			Interval: 60 * time.Minute,
		},
		HealthCheck: HealthCheckConfig{
			Interval:         30 * time.Second,
			ResponseTimeout:  5 * time.Second,
			ComponentTimeout: 10 * time.Second,
		},
		Deployment: DeploymentConfig{
			MaxHoneytokensPerPod: 5,
			Timeout:              30 * time.Second,
		},
		Response: ResponseConfig{
			IsolationTimeout:       10 * time.Second,
			IsolationMaxRetries:    3,
			IsolationRetryInterval: 5 * time.Second,
			IPBlockMaxRetries:      3,
			IPBlockRetryInterval:   5 * time.Second,
		},
		Reporting: ReportingConfig{
			RetentionDays:        90,
			GenerationTimeout:    60 * time.Second,
			GenerationMaxRetries: 3,
		},
		Learning: LearningConfig{
			RetrainingInterval:       24 * time.Hour,
			MinimumOutcomeRecords:    50,
			OutcomeSubmissionTimeout: 60 * time.Second,
		},
		AuditLog: AuditLogConfig{
			RetentionDays: 90,
		},
		Notifications: NotificationsConfig{
			ChannelEndpoint: "",
		},
		DynatraceIngestion: DynatraceIngestionConfig{
			MetricsEndpoint: "",
			LogEndpoint:     "",
			APIToken:        "",
			RequestTimeout:  10 * time.Second,
			Retry: RetryConfig{
				MaxRetries:        5,
				InitialBackoff:    2 * time.Second,
				BackoffMultiplier: 2,
				MaxBackoff:        32 * time.Second,
			},
			Batch: BatchConfig{
				MaxBatchSize:  100,
				FlushInterval: 5 * time.Second,
			},
		},
		Tetragon: TetragonConfig{
			GRPCAddress:          "localhost:54321",
			ReconnectInterval:    5 * time.Second,
			MaxReconnectAttempts: -1,
			EventBufferCapacity:  1000,
		},
	}
}

// Validate checks the configuration for constraint violations.
// Returns an error describing the first invalid field found.
func (c *Config) Validate() error {
	// Discovery interval: 5m-1440m
	if c.Discovery.Interval < 5*time.Minute || c.Discovery.Interval > 1440*time.Minute {
		return fmt.Errorf("discovery.interval must be between 5m and 1440m, got %s", c.Discovery.Interval)
	}

	// Health check intervals: must be positive
	if c.HealthCheck.Interval <= 0 {
		return fmt.Errorf("health_check.interval must be positive, got %s", c.HealthCheck.Interval)
	}
	if c.HealthCheck.ResponseTimeout <= 0 {
		return fmt.Errorf("health_check.response_timeout must be positive, got %s", c.HealthCheck.ResponseTimeout)
	}
	if c.HealthCheck.ComponentTimeout <= 0 {
		return fmt.Errorf("health_check.component_timeout must be positive, got %s", c.HealthCheck.ComponentTimeout)
	}

	// Deployment
	if c.Deployment.MaxHoneytokensPerPod < 1 || c.Deployment.MaxHoneytokensPerPod > 5 {
		return fmt.Errorf("deployment.max_honeytokens_per_pod must be between 1 and 5, got %d", c.Deployment.MaxHoneytokensPerPod)
	}
	if c.Deployment.Timeout <= 0 {
		return fmt.Errorf("deployment.timeout must be positive, got %s", c.Deployment.Timeout)
	}

	// Response
	if c.Response.IsolationTimeout <= 0 {
		return fmt.Errorf("response.isolation_timeout must be positive, got %s", c.Response.IsolationTimeout)
	}
	if c.Response.IsolationMaxRetries < 0 {
		return fmt.Errorf("response.isolation_max_retries must be non-negative, got %d", c.Response.IsolationMaxRetries)
	}
	if c.Response.IPBlockMaxRetries < 0 {
		return fmt.Errorf("response.ip_block_max_retries must be non-negative, got %d", c.Response.IPBlockMaxRetries)
	}

	// Learning
	if c.Learning.RetrainingInterval < 1*time.Hour || c.Learning.RetrainingInterval > 168*time.Hour {
		return fmt.Errorf("learning.retraining_interval must be between 1h and 168h, got %s", c.Learning.RetrainingInterval)
	}
	if c.Learning.MinimumOutcomeRecords < 1 {
		return fmt.Errorf("learning.minimum_outcome_records must be at least 1, got %d", c.Learning.MinimumOutcomeRecords)
	}

	// Audit log
	if c.AuditLog.RetentionDays < 90 {
		return fmt.Errorf("audit_log.retention_days must be at least 90, got %d", c.AuditLog.RetentionDays)
	}

	// Reporting
	if c.Reporting.RetentionDays <= 0 {
		return fmt.Errorf("reporting.retention_days must be positive, got %d", c.Reporting.RetentionDays)
	}

	// Dynatrace ingestion retry
	if c.DynatraceIngestion.Retry.MaxRetries < 0 {
		return fmt.Errorf("dynatrace_ingestion.retry.max_retries must be non-negative, got %d", c.DynatraceIngestion.Retry.MaxRetries)
	}
	if c.DynatraceIngestion.Retry.InitialBackoff <= 0 {
		return fmt.Errorf("dynatrace_ingestion.retry.initial_backoff must be positive, got %s", c.DynatraceIngestion.Retry.InitialBackoff)
	}
	if c.DynatraceIngestion.Retry.BackoffMultiplier < 1 {
		return fmt.Errorf("dynatrace_ingestion.retry.backoff_multiplier must be at least 1, got %f", c.DynatraceIngestion.Retry.BackoffMultiplier)
	}
	if c.DynatraceIngestion.Batch.MaxBatchSize <= 0 {
		return fmt.Errorf("dynatrace_ingestion.batch.max_batch_size must be positive, got %d", c.DynatraceIngestion.Batch.MaxBatchSize)
	}
	if c.DynatraceIngestion.Batch.FlushInterval <= 0 {
		return fmt.Errorf("dynatrace_ingestion.batch.flush_interval must be positive, got %s", c.DynatraceIngestion.Batch.FlushInterval)
	}

	// Tetragon
	if c.Tetragon.GRPCAddress == "" {
		return fmt.Errorf("tetragon.grpc_address must not be empty")
	}
	if c.Tetragon.ReconnectInterval <= 0 {
		return fmt.Errorf("tetragon.reconnect_interval must be positive, got %s", c.Tetragon.ReconnectInterval)
	}
	if c.Tetragon.EventBufferCapacity < 1 {
		return fmt.Errorf("tetragon.event_buffer_capacity must be at least 1, got %d", c.Tetragon.EventBufferCapacity)
	}

	return nil
}
