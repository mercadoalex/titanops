package ollinai

import (
	"fmt"
	"net/url"
	"time"

	config "github.com/mercadoalex/titanops/shared/titanops-config"
)

// Config holds all OllinAI adapter configuration.
type Config struct {
	// Endpoint is the OllinAI external API base URL (required).
	Endpoint string `yaml:"endpoint" validate:"required"`
	// AuthToken is the bearer token for OllinAI API authentication (required).
	AuthToken string `yaml:"authToken" validate:"required"`
	// WebhookPort is the port for receiving eBPF agent webhooks. Default: 8090.
	WebhookPort int `yaml:"webhookPort" validate:"min=1024,max=65535"`
	// WebhookHMACKey is the shared secret for webhook signature verification.
	WebhookHMACKey string `yaml:"webhookHmacKey"`
	// RiskPollInterval is the polling interval for deployment risk data. Default: 30s.
	RiskPollInterval time.Duration `yaml:"riskPollInterval"`
	// DORAPollInterval is the polling interval for DORA metrics. Default: 5m.
	DORAPollInterval time.Duration `yaml:"doraPollInterval"`
	// NATSUrl is the NATS server URL. Default: nats://titanops-nats:4222.
	NATSUrl string `yaml:"natsUrl" validate:"required"`
	// BufferCapacity is the ring buffer size for event buffering. Default: 1000.
	BufferCapacity int `yaml:"bufferCapacity" validate:"min=100,max=10000"`
	// MaxPayloadBytes is the maximum JSON payload size. Default: 65536 (64KB).
	MaxPayloadBytes int `yaml:"maxPayloadBytes" validate:"min=1024,max=65536"`
	// MetricsPort is the Prometheus metrics endpoint port. Default: 9090.
	MetricsPort int `yaml:"metricsPort" validate:"min=1024,max=65535"`
}

// DefaultConfig returns a Config populated with default values for optional fields.
func DefaultConfig() Config {
	return Config{
		WebhookPort:      8090,
		RiskPollInterval: 30 * time.Second,
		DORAPollInterval: 5 * time.Minute,
		NATSUrl:          "nats://titanops-nats:4222",
		BufferCapacity:   1000,
		MaxPayloadBytes:  65536,
		MetricsPort:      9090,
	}
}

// LoadConfig loads the OllinAI configuration using titanops-config with environment
// variable prefix TITANOPS_OLLINAI and optional file source. It applies defaults,
// then loads from file and environment variables (env takes precedence over file,
// file takes precedence over defaults).
func LoadConfig(filePath string) (*Config, []error) {
	defaults := DefaultConfig()
	cfg := Config{}

	opts := []config.Option{
		config.WithDefaults(defaults),
		config.WithEnvPrefix("TITANOPS_OLLINAI"),
	}
	if filePath != "" {
		opts = append(opts, config.WithFile(filePath))
	}

	validationErrors, err := config.Load(&cfg, opts...)
	if err != nil {
		return nil, []error{fmt.Errorf("failed to load config: %w", err)}
	}

	// Combine struct-tag validation errors with custom validation
	var errs []error
	for _, ve := range validationErrors {
		errs = append(errs, ve)
	}

	// Custom validations not covered by struct tags
	customErrors := validateCustom(&cfg)
	errs = append(errs, customErrors...)

	if len(errs) > 0 {
		return nil, errs
	}

	return &cfg, nil
}

// ValidateConfig validates a Config struct and returns per-field errors with
// field name, value, and constraint information. Returns nil if the config is valid.
func ValidateConfig(cfg *Config) []error {
	if cfg == nil {
		return []error{fmt.Errorf("config must not be nil")}
	}

	// Run struct-tag based validation via titanops-config
	validationErrors, _ := config.Load(cfg)

	var errs []error
	for _, ve := range validationErrors {
		errs = append(errs, ve)
	}

	// Custom validations
	customErrors := validateCustom(cfg)
	errs = append(errs, customErrors...)

	return errs
}

// validateCustom performs validations that cannot be expressed via struct tags,
// specifically URL format checks and duration range checks.
func validateCustom(cfg *Config) []error {
	var errs []error

	// Validate Endpoint is a valid URL (if non-empty)
	if cfg.Endpoint != "" {
		if _, err := url.ParseRequestURI(cfg.Endpoint); err != nil {
			errs = append(errs, config.ValidationError{
				Field:   "Endpoint",
				Value:   cfg.Endpoint,
				Message: fmt.Sprintf("must be a valid URL: %v", err),
			})
		}
	}

	// Validate NATSUrl is a valid URL (if non-empty)
	if cfg.NATSUrl != "" {
		parsed, err := url.Parse(cfg.NATSUrl)
		if err != nil || parsed.Scheme == "" || parsed.Host == "" {
			errs = append(errs, config.ValidationError{
				Field:   "NATSUrl",
				Value:   cfg.NATSUrl,
				Message: "must be a valid URL with scheme and host",
			})
		}
	}

	// Validate RiskPollInterval: min=5s, max=300s
	if cfg.RiskPollInterval != 0 {
		if cfg.RiskPollInterval < 5*time.Second {
			errs = append(errs, config.ValidationError{
				Field:   "RiskPollInterval",
				Value:   cfg.RiskPollInterval,
				Message: fmt.Sprintf("must be at least 5s, got %s", cfg.RiskPollInterval),
			})
		}
		if cfg.RiskPollInterval > 300*time.Second {
			errs = append(errs, config.ValidationError{
				Field:   "RiskPollInterval",
				Value:   cfg.RiskPollInterval,
				Message: fmt.Sprintf("must be at most 300s, got %s", cfg.RiskPollInterval),
			})
		}
	}

	// Validate DORAPollInterval: min=30s, max=30m
	if cfg.DORAPollInterval != 0 {
		if cfg.DORAPollInterval < 30*time.Second {
			errs = append(errs, config.ValidationError{
				Field:   "DORAPollInterval",
				Value:   cfg.DORAPollInterval,
				Message: fmt.Sprintf("must be at least 30s, got %s", cfg.DORAPollInterval),
			})
		}
		if cfg.DORAPollInterval > 30*time.Minute {
			errs = append(errs, config.ValidationError{
				Field:   "DORAPollInterval",
				Value:   cfg.DORAPollInterval,
				Message: fmt.Sprintf("must be at most 30m, got %s", cfg.DORAPollInterval),
			})
		}
	}

	return errs
}
