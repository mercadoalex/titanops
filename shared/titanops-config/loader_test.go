package config

import (
	"os"
	"path/filepath"
	"testing"
)

// --- Test structs ---

type SimpleConfig struct {
	Host string `validate:"required"`
	Port int    `validate:"min=1024,max=65535"`
}

type ConfigWithEnum struct {
	Provider string `validate:"required,oneof=local gemini bedrock vertex sagemaker"`
	Port     int    `validate:"min=1024,max=65535"`
}

type NestedConfig struct {
	Server  ServerConfig
	Logging LoggingConfig
}

type ServerConfig struct {
	Host string `validate:"required"`
	Port int    `validate:"min=1024,max=65535"`
}

type LoggingConfig struct {
	Level string `validate:"oneof=debug info warn error"`
}

type FloatConfig struct {
	Threshold float64 `validate:"min=0.1,max=1.0"`
}

// --- Load function tests ---

func TestLoad_NilTarget(t *testing.T) {
	_, err := Load(nil)
	if err == nil {
		t.Fatal("expected error for nil target")
	}
}

func TestLoad_NonPointerTarget(t *testing.T) {
	cfg := SimpleConfig{}
	_, err := Load(cfg)
	if err == nil {
		t.Fatal("expected error for non-pointer target")
	}
}

func TestLoad_NonStructPointer(t *testing.T) {
	s := "hello"
	_, err := Load(&s)
	if err == nil {
		t.Fatal("expected error for non-struct pointer target")
	}
}

func TestLoad_EmptyStructNoErrors(t *testing.T) {
	type EmptyConfig struct {
		Name string
	}
	cfg := EmptyConfig{}
	errs, err := Load(&cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(errs) != 0 {
		t.Fatalf("expected no validation errors, got %v", errs)
	}
}

// --- Defaults tests ---

func TestLoad_WithDefaults(t *testing.T) {
	defaults := SimpleConfig{
		Host: "localhost",
		Port: 8080,
	}
	cfg := SimpleConfig{}
	errs, err := Load(&cfg, WithDefaults(defaults))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(errs) != 0 {
		t.Fatalf("expected no validation errors, got %v", errs)
	}
	if cfg.Host != "localhost" {
		t.Errorf("expected Host=localhost, got %s", cfg.Host)
	}
	if cfg.Port != 8080 {
		t.Errorf("expected Port=8080, got %d", cfg.Port)
	}
}

func TestLoad_WithDefaultsPointer(t *testing.T) {
	defaults := &SimpleConfig{
		Host: "localhost",
		Port: 9090,
	}
	cfg := SimpleConfig{}
	errs, err := Load(&cfg, WithDefaults(defaults))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(errs) != 0 {
		t.Fatalf("expected no validation errors, got %v", errs)
	}
	if cfg.Host != "localhost" {
		t.Errorf("expected Host=localhost, got %s", cfg.Host)
	}
	if cfg.Port != 9090 {
		t.Errorf("expected Port=9090, got %d", cfg.Port)
	}
}

// --- File loading tests ---

func TestLoad_WithYAMLFile(t *testing.T) {
	dir := t.TempDir()
	yamlPath := filepath.Join(dir, "config.yaml")
	content := []byte("host: filehost\nport: 3000\n")
	if err := os.WriteFile(yamlPath, content, 0644); err != nil {
		t.Fatal(err)
	}

	cfg := SimpleConfig{}
	errs, err := Load(&cfg, WithFile(yamlPath))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(errs) != 0 {
		t.Fatalf("expected no validation errors, got %v", errs)
	}
	if cfg.Host != "filehost" {
		t.Errorf("expected Host=filehost, got %s", cfg.Host)
	}
	if cfg.Port != 3000 {
		t.Errorf("expected Port=3000, got %d", cfg.Port)
	}
}

func TestLoad_WithJSONFile(t *testing.T) {
	dir := t.TempDir()
	jsonPath := filepath.Join(dir, "config.json")
	content := []byte(`{"Host":"jsonhost","Port":4000}`)
	if err := os.WriteFile(jsonPath, content, 0644); err != nil {
		t.Fatal(err)
	}

	cfg := SimpleConfig{}
	errs, err := Load(&cfg, WithFile(jsonPath))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(errs) != 0 {
		t.Fatalf("expected no validation errors, got %v", errs)
	}
	if cfg.Host != "jsonhost" {
		t.Errorf("expected Host=jsonhost, got %s", cfg.Host)
	}
	if cfg.Port != 4000 {
		t.Errorf("expected Port=4000, got %d", cfg.Port)
	}
}

func TestLoad_WithMissingFile(t *testing.T) {
	cfg := SimpleConfig{}
	_, err := Load(&cfg, WithFile("/nonexistent/config.yaml"))
	if err == nil {
		t.Fatal("expected error for missing file")
	}
}

func TestLoad_WithInvalidYAML(t *testing.T) {
	dir := t.TempDir()
	yamlPath := filepath.Join(dir, "config.yaml")
	content := []byte("invalid: [yaml: without: closing\n")
	if err := os.WriteFile(yamlPath, content, 0644); err != nil {
		t.Fatal(err)
	}

	cfg := SimpleConfig{}
	_, err := Load(&cfg, WithFile(yamlPath))
	if err == nil {
		t.Fatal("expected error for invalid YAML")
	}
}

// --- Environment variable tests ---

func TestLoad_WithEnvPrefix(t *testing.T) {
	t.Setenv("TEST_HOST", "envhost")
	t.Setenv("TEST_PORT", "5000")

	cfg := SimpleConfig{}
	errs, err := Load(&cfg, WithEnvPrefix("TEST"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(errs) != 0 {
		t.Fatalf("expected no validation errors, got %v", errs)
	}
	if cfg.Host != "envhost" {
		t.Errorf("expected Host=envhost, got %s", cfg.Host)
	}
	if cfg.Port != 5000 {
		t.Errorf("expected Port=5000, got %d", cfg.Port)
	}
}

func TestLoad_EnvNestedStruct(t *testing.T) {
	t.Setenv("APP_SERVER_HOST", "nested-env-host")
	t.Setenv("APP_SERVER_PORT", "7000")
	t.Setenv("APP_LOGGING_LEVEL", "debug")

	cfg := NestedConfig{}
	errs, err := Load(&cfg, WithEnvPrefix("APP"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(errs) != 0 {
		t.Fatalf("expected no validation errors, got %v", errs)
	}
	if cfg.Server.Host != "nested-env-host" {
		t.Errorf("expected Server.Host=nested-env-host, got %s", cfg.Server.Host)
	}
	if cfg.Server.Port != 7000 {
		t.Errorf("expected Server.Port=7000, got %d", cfg.Server.Port)
	}
	if cfg.Logging.Level != "debug" {
		t.Errorf("expected Logging.Level=debug, got %s", cfg.Logging.Level)
	}
}

// --- Merge priority tests ---

func TestLoad_MergePriority_EnvOverridesFile(t *testing.T) {
	dir := t.TempDir()
	yamlPath := filepath.Join(dir, "config.yaml")
	content := []byte("host: filehost\nport: 3000\n")
	if err := os.WriteFile(yamlPath, content, 0644); err != nil {
		t.Fatal(err)
	}

	t.Setenv("MERGE_HOST", "envhost")
	// PORT not set, should remain from file

	cfg := SimpleConfig{}
	errs, err := Load(&cfg, WithFile(yamlPath), WithEnvPrefix("MERGE"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(errs) != 0 {
		t.Fatalf("expected no validation errors, got %v", errs)
	}
	if cfg.Host != "envhost" {
		t.Errorf("expected Host=envhost (env override), got %s", cfg.Host)
	}
	if cfg.Port != 3000 {
		t.Errorf("expected Port=3000 (from file), got %d", cfg.Port)
	}
}

func TestLoad_MergePriority_FileOverridesDefaults(t *testing.T) {
	dir := t.TempDir()
	yamlPath := filepath.Join(dir, "config.yaml")
	content := []byte("host: filehost\n")
	if err := os.WriteFile(yamlPath, content, 0644); err != nil {
		t.Fatal(err)
	}

	defaults := SimpleConfig{
		Host: "defaulthost",
		Port: 8080,
	}

	cfg := SimpleConfig{}
	errs, err := Load(&cfg, WithDefaults(defaults), WithFile(yamlPath))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(errs) != 0 {
		t.Fatalf("expected no validation errors, got %v", errs)
	}
	if cfg.Host != "filehost" {
		t.Errorf("expected Host=filehost (file override), got %s", cfg.Host)
	}
	if cfg.Port != 8080 {
		t.Errorf("expected Port=8080 (from defaults), got %d", cfg.Port)
	}
}

func TestLoad_MergePriority_FullChain(t *testing.T) {
	dir := t.TempDir()
	yamlPath := filepath.Join(dir, "config.yaml")
	content := []byte("host: filehost\nport: 3000\n")
	if err := os.WriteFile(yamlPath, content, 0644); err != nil {
		t.Fatal(err)
	}

	defaults := SimpleConfig{
		Host: "defaulthost",
		Port: 1024,
	}

	t.Setenv("FULL_PORT", "9000")
	// HOST not set in env, should come from file

	cfg := SimpleConfig{}
	errs, err := Load(&cfg, WithDefaults(defaults), WithFile(yamlPath), WithEnvPrefix("FULL"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(errs) != 0 {
		t.Fatalf("expected no validation errors, got %v", errs)
	}
	if cfg.Host != "filehost" {
		t.Errorf("expected Host=filehost (from file), got %s", cfg.Host)
	}
	if cfg.Port != 9000 {
		t.Errorf("expected Port=9000 (from env), got %d", cfg.Port)
	}
}

// --- Validation tests ---

func TestLoad_ValidationRequired(t *testing.T) {
	cfg := SimpleConfig{}
	errs, err := Load(&cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(errs) == 0 {
		t.Fatal("expected validation errors for required field")
	}
	found := false
	for _, e := range errs {
		if e.Field == "Host" && e.Message == "field is required" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected validation error for Host required, got %v", errs)
	}
}

func TestLoad_ValidationMinMax(t *testing.T) {
	cfg := SimpleConfig{
		Host: "localhost",
		Port: 80, // below min of 1024
	}
	errs, err := Load(&cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(errs) == 0 {
		t.Fatal("expected validation errors for port out of range")
	}
	found := false
	for _, e := range errs {
		if e.Field == "Port" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected validation error for Port, got %v", errs)
	}
}

func TestLoad_ValidationMaxExceeded(t *testing.T) {
	cfg := SimpleConfig{
		Host: "localhost",
		Port: 70000, // above max of 65535
	}
	errs, err := Load(&cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(errs) == 0 {
		t.Fatal("expected validation errors for port above max")
	}
	found := false
	for _, e := range errs {
		if e.Field == "Port" && e.Value == 70000 {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected validation error for Port=70000, got %v", errs)
	}
}

func TestLoad_ValidationOneOf(t *testing.T) {
	cfg := ConfigWithEnum{
		Provider: "invalid_provider",
		Port:     9090,
	}
	errs, err := Load(&cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	found := false
	for _, e := range errs {
		if e.Field == "Provider" && e.Value == "invalid_provider" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected validation error for Provider=invalid_provider, got %v", errs)
	}
}

func TestLoad_ValidationOneOfValid(t *testing.T) {
	cfg := ConfigWithEnum{
		Provider: "local",
		Port:     9090,
	}
	errs, err := Load(&cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(errs) != 0 {
		t.Fatalf("expected no validation errors, got %v", errs)
	}
}

func TestLoad_ValidationFloat(t *testing.T) {
	cfg := FloatConfig{Threshold: 0.05} // below min of 0.1
	errs, err := Load(&cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(errs) == 0 {
		t.Fatal("expected validation error for threshold below min")
	}
	found := false
	for _, e := range errs {
		if e.Field == "Threshold" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected validation error for Threshold, got %v", errs)
	}
}

func TestLoad_ValidationFloatAboveMax(t *testing.T) {
	cfg := FloatConfig{Threshold: 1.5} // above max of 1.0
	errs, err := Load(&cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(errs) == 0 {
		t.Fatal("expected validation error for threshold above max")
	}
}

func TestLoad_ValidationNestedStruct(t *testing.T) {
	cfg := NestedConfig{
		Server: ServerConfig{
			Host: "", // required
			Port: 80, // below min
		},
		Logging: LoggingConfig{
			Level: "invalid_level",
		},
	}
	errs, err := Load(&cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(errs) < 3 {
		t.Fatalf("expected at least 3 validation errors, got %d: %v", len(errs), errs)
	}
}

// --- toEnvName tests ---

func TestToEnvName(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"Host", "HOST"},
		{"Port", "PORT"},
		{"FieldName", "FIELD_NAME"},
		{"OTLPEndpoint", "O_T_L_P_ENDPOINT"},
		{"A", "A"},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := toEnvName(tt.input)
			if got != tt.expected {
				t.Errorf("toEnvName(%q) = %q, want %q", tt.input, got, tt.expected)
			}
		})
	}
}

// --- ConfigError tests ---

func TestConfigError_Error(t *testing.T) {
	e := &ConfigError{Op: "load", Err: os.ErrNotExist}
	if e.Error() != "config load: file does not exist" {
		t.Errorf("unexpected error message: %s", e.Error())
	}

	e2 := &ConfigError{Op: "load_file", Path: "/etc/config.yaml", Err: os.ErrNotExist}
	expected := "config load_file [/etc/config.yaml]: file does not exist"
	if e2.Error() != expected {
		t.Errorf("unexpected error message: %s, want: %s", e2.Error(), expected)
	}
}

func TestValidationError_Error(t *testing.T) {
	ve := ValidationError{
		Field:   "Port",
		Value:   80,
		Message: "value must be at least 1024",
	}
	expected := "field Port: value must be at least 1024 (value: 80)"
	if ve.Error() != expected {
		t.Errorf("unexpected error message: %s, want: %s", ve.Error(), expected)
	}
}

// --- Bool and uint env tests ---

func TestLoad_EnvBoolAndUint(t *testing.T) {
	type BoolConfig struct {
		Enabled bool
		Count   uint `validate:"min=1,max=100"`
	}

	t.Setenv("BC_ENABLED", "true")
	t.Setenv("BC_COUNT", "42")

	cfg := BoolConfig{}
	errs, err := Load(&cfg, WithEnvPrefix("BC"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(errs) != 0 {
		t.Fatalf("expected no validation errors, got %v", errs)
	}
	if !cfg.Enabled {
		t.Error("expected Enabled=true")
	}
	if cfg.Count != 42 {
		t.Errorf("expected Count=42, got %d", cfg.Count)
	}
}
