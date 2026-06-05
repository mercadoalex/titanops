package ollinai

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()

	if cfg.WebhookPort != 8090 {
		t.Errorf("expected WebhookPort=8090, got %d", cfg.WebhookPort)
	}
	if cfg.RiskPollInterval != 30*time.Second {
		t.Errorf("expected RiskPollInterval=30s, got %s", cfg.RiskPollInterval)
	}
	if cfg.DORAPollInterval != 5*time.Minute {
		t.Errorf("expected DORAPollInterval=5m, got %s", cfg.DORAPollInterval)
	}
	if cfg.NATSUrl != "nats://titanops-nats:4222" {
		t.Errorf("expected NATSUrl=nats://titanops-nats:4222, got %s", cfg.NATSUrl)
	}
	if cfg.BufferCapacity != 1000 {
		t.Errorf("expected BufferCapacity=1000, got %d", cfg.BufferCapacity)
	}
	if cfg.MaxPayloadBytes != 65536 {
		t.Errorf("expected MaxPayloadBytes=65536, got %d", cfg.MaxPayloadBytes)
	}
	if cfg.MetricsPort != 9090 {
		t.Errorf("expected MetricsPort=9090, got %d", cfg.MetricsPort)
	}
}

func TestValidateConfig_ValidConfig(t *testing.T) {
	cfg := &Config{
		Endpoint:         "https://api.ollinai.com",
		AuthToken:        "secret-token",
		WebhookPort:      8090,
		RiskPollInterval: 30 * time.Second,
		DORAPollInterval: 5 * time.Minute,
		NATSUrl:          "nats://titanops-nats:4222",
		BufferCapacity:   1000,
		MaxPayloadBytes:  65536,
		MetricsPort:      9090,
	}

	errs := ValidateConfig(cfg)
	if len(errs) != 0 {
		t.Fatalf("expected no errors, got %v", errs)
	}
}

func TestValidateConfig_NilConfig(t *testing.T) {
	errs := ValidateConfig(nil)
	if len(errs) != 1 {
		t.Fatalf("expected 1 error for nil config, got %d", len(errs))
	}
}

func TestValidateConfig_MissingRequired(t *testing.T) {
	cfg := &Config{
		WebhookPort:      8090,
		RiskPollInterval: 30 * time.Second,
		DORAPollInterval: 5 * time.Minute,
		NATSUrl:          "nats://titanops-nats:4222",
		BufferCapacity:   1000,
		MaxPayloadBytes:  65536,
		MetricsPort:      9090,
	}

	errs := ValidateConfig(cfg)
	if len(errs) == 0 {
		t.Fatal("expected validation errors for missing required fields")
	}

	// Should report errors for Endpoint and AuthToken
	fieldErrors := make(map[string]bool)
	for _, err := range errs {
		fieldErrors[err.Error()] = true
	}
	foundEndpoint := false
	foundAuthToken := false
	for errMsg := range fieldErrors {
		if contains(errMsg, "Endpoint") {
			foundEndpoint = true
		}
		if contains(errMsg, "AuthToken") {
			foundAuthToken = true
		}
	}
	if !foundEndpoint {
		t.Error("expected error for missing Endpoint")
	}
	if !foundAuthToken {
		t.Error("expected error for missing AuthToken")
	}
}

func TestValidateConfig_InvalidEndpointURL(t *testing.T) {
	cfg := &Config{
		Endpoint:         "not-a-url",
		AuthToken:        "token",
		WebhookPort:      8090,
		RiskPollInterval: 30 * time.Second,
		DORAPollInterval: 5 * time.Minute,
		NATSUrl:          "nats://titanops-nats:4222",
		BufferCapacity:   1000,
		MaxPayloadBytes:  65536,
		MetricsPort:      9090,
	}

	errs := ValidateConfig(cfg)
	if len(errs) == 0 {
		t.Fatal("expected validation error for invalid Endpoint URL")
	}
	found := false
	for _, err := range errs {
		if contains(err.Error(), "Endpoint") {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected error for Endpoint, got %v", errs)
	}
}

func TestValidateConfig_PortOutOfRange(t *testing.T) {
	cfg := &Config{
		Endpoint:         "https://api.ollinai.com",
		AuthToken:        "token",
		WebhookPort:      80, // below min 1024
		RiskPollInterval: 30 * time.Second,
		DORAPollInterval: 5 * time.Minute,
		NATSUrl:          "nats://titanops-nats:4222",
		BufferCapacity:   1000,
		MaxPayloadBytes:  65536,
		MetricsPort:      9090,
	}

	errs := ValidateConfig(cfg)
	if len(errs) == 0 {
		t.Fatal("expected validation error for WebhookPort out of range")
	}
	found := false
	for _, err := range errs {
		if contains(err.Error(), "WebhookPort") {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected error for WebhookPort, got %v", errs)
	}
}

func TestValidateConfig_RiskPollIntervalTooLow(t *testing.T) {
	cfg := &Config{
		Endpoint:         "https://api.ollinai.com",
		AuthToken:        "token",
		WebhookPort:      8090,
		RiskPollInterval: 2 * time.Second, // below min 5s
		DORAPollInterval: 5 * time.Minute,
		NATSUrl:          "nats://titanops-nats:4222",
		BufferCapacity:   1000,
		MaxPayloadBytes:  65536,
		MetricsPort:      9090,
	}

	errs := ValidateConfig(cfg)
	if len(errs) == 0 {
		t.Fatal("expected validation error for RiskPollInterval below minimum")
	}
	found := false
	for _, err := range errs {
		if contains(err.Error(), "RiskPollInterval") {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected error for RiskPollInterval, got %v", errs)
	}
}

func TestValidateConfig_RiskPollIntervalTooHigh(t *testing.T) {
	cfg := &Config{
		Endpoint:         "https://api.ollinai.com",
		AuthToken:        "token",
		WebhookPort:      8090,
		RiskPollInterval: 10 * time.Minute, // above max 300s
		DORAPollInterval: 5 * time.Minute,
		NATSUrl:          "nats://titanops-nats:4222",
		BufferCapacity:   1000,
		MaxPayloadBytes:  65536,
		MetricsPort:      9090,
	}

	errs := ValidateConfig(cfg)
	if len(errs) == 0 {
		t.Fatal("expected validation error for RiskPollInterval above maximum")
	}
	found := false
	for _, err := range errs {
		if contains(err.Error(), "RiskPollInterval") {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected error for RiskPollInterval, got %v", errs)
	}
}

func TestValidateConfig_DORAPollIntervalTooLow(t *testing.T) {
	cfg := &Config{
		Endpoint:         "https://api.ollinai.com",
		AuthToken:        "token",
		WebhookPort:      8090,
		RiskPollInterval: 30 * time.Second,
		DORAPollInterval: 10 * time.Second, // below min 30s
		NATSUrl:          "nats://titanops-nats:4222",
		BufferCapacity:   1000,
		MaxPayloadBytes:  65536,
		MetricsPort:      9090,
	}

	errs := ValidateConfig(cfg)
	if len(errs) == 0 {
		t.Fatal("expected validation error for DORAPollInterval below minimum")
	}
	found := false
	for _, err := range errs {
		if contains(err.Error(), "DORAPollInterval") {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected error for DORAPollInterval, got %v", errs)
	}
}

func TestValidateConfig_DORAPollIntervalTooHigh(t *testing.T) {
	cfg := &Config{
		Endpoint:         "https://api.ollinai.com",
		AuthToken:        "token",
		WebhookPort:      8090,
		RiskPollInterval: 30 * time.Second,
		DORAPollInterval: 1 * time.Hour, // above max 30m
		NATSUrl:          "nats://titanops-nats:4222",
		BufferCapacity:   1000,
		MaxPayloadBytes:  65536,
		MetricsPort:      9090,
	}

	errs := ValidateConfig(cfg)
	if len(errs) == 0 {
		t.Fatal("expected validation error for DORAPollInterval above maximum")
	}
	found := false
	for _, err := range errs {
		if contains(err.Error(), "DORAPollInterval") {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected error for DORAPollInterval, got %v", errs)
	}
}

func TestValidateConfig_BufferCapacityOutOfRange(t *testing.T) {
	cfg := &Config{
		Endpoint:         "https://api.ollinai.com",
		AuthToken:        "token",
		WebhookPort:      8090,
		RiskPollInterval: 30 * time.Second,
		DORAPollInterval: 5 * time.Minute,
		NATSUrl:          "nats://titanops-nats:4222",
		BufferCapacity:   50, // below min 100
		MaxPayloadBytes:  65536,
		MetricsPort:      9090,
	}

	errs := ValidateConfig(cfg)
	if len(errs) == 0 {
		t.Fatal("expected validation error for BufferCapacity out of range")
	}
	found := false
	for _, err := range errs {
		if contains(err.Error(), "BufferCapacity") {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected error for BufferCapacity, got %v", errs)
	}
}

func TestValidateConfig_InvalidNATSUrl(t *testing.T) {
	cfg := &Config{
		Endpoint:         "https://api.ollinai.com",
		AuthToken:        "token",
		WebhookPort:      8090,
		RiskPollInterval: 30 * time.Second,
		DORAPollInterval: 5 * time.Minute,
		NATSUrl:          "not-a-url",
		BufferCapacity:   1000,
		MaxPayloadBytes:  65536,
		MetricsPort:      9090,
	}

	errs := ValidateConfig(cfg)
	if len(errs) == 0 {
		t.Fatal("expected validation error for invalid NATSUrl")
	}
	found := false
	for _, err := range errs {
		if contains(err.Error(), "NATSUrl") {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected error for NATSUrl, got %v", errs)
	}
}

func TestLoadConfig_WithEnvVars(t *testing.T) {
	t.Setenv("TITANOPS_OLLINAI_ENDPOINT", "https://env.ollinai.com")
	t.Setenv("TITANOPS_OLLINAI_AUTH_TOKEN", "env-token")

	cfg, errs := LoadConfig("")
	if len(errs) != 0 {
		t.Fatalf("expected no errors, got %v", errs)
	}
	if cfg.Endpoint != "https://env.ollinai.com" {
		t.Errorf("expected Endpoint from env, got %s", cfg.Endpoint)
	}
	if cfg.AuthToken != "env-token" {
		t.Errorf("expected AuthToken from env, got %s", cfg.AuthToken)
	}
	// Defaults should be applied
	if cfg.WebhookPort != 8090 {
		t.Errorf("expected default WebhookPort=8090, got %d", cfg.WebhookPort)
	}
}

func TestLoadConfig_WithFile(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "ollinai.yaml")
	content := []byte(`endpoint: "https://file.ollinai.com"
authToken: "file-token"
webhookPort: 9000
`)
	if err := os.WriteFile(cfgPath, content, 0644); err != nil {
		t.Fatal(err)
	}

	cfg, errs := LoadConfig(cfgPath)
	if len(errs) != 0 {
		t.Fatalf("expected no errors, got %v", errs)
	}
	if cfg.Endpoint != "https://file.ollinai.com" {
		t.Errorf("expected Endpoint from file, got %s", cfg.Endpoint)
	}
	if cfg.AuthToken != "file-token" {
		t.Errorf("expected AuthToken from file, got %s", cfg.AuthToken)
	}
	if cfg.WebhookPort != 9000 {
		t.Errorf("expected WebhookPort=9000 from file, got %d", cfg.WebhookPort)
	}
	// Check defaults applied for unset fields
	if cfg.BufferCapacity != 1000 {
		t.Errorf("expected default BufferCapacity=1000, got %d", cfg.BufferCapacity)
	}
}

func TestLoadConfig_EnvOverridesFile(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "ollinai.yaml")
	content := []byte(`endpoint: "https://file.ollinai.com"
authToken: "file-token"
webhookPort: 9000
`)
	if err := os.WriteFile(cfgPath, content, 0644); err != nil {
		t.Fatal(err)
	}

	t.Setenv("TITANOPS_OLLINAI_WEBHOOK_PORT", "9500")

	cfg, errs := LoadConfig(cfgPath)
	if len(errs) != 0 {
		t.Fatalf("expected no errors, got %v", errs)
	}
	// Env should override file
	if cfg.WebhookPort != 9500 {
		t.Errorf("expected WebhookPort=9500 from env override, got %d", cfg.WebhookPort)
	}
}

func TestLoadConfig_MissingFileReturnsError(t *testing.T) {
	_, errs := LoadConfig("/nonexistent/config.yaml")
	if len(errs) == 0 {
		t.Fatal("expected error for missing config file")
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsSubstr(s, substr))
}

func containsSubstr(s, sub string) bool {
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
