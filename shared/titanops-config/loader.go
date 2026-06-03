// Package config provides configuration loading from environment variables
// and file sources with struct validation.
package config

import (
	"encoding/json"
	"fmt"
	"os"
	"reflect"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"
)

// Load reads configuration from Helm values (environment variables) and file sources,
// merges them into the target struct, and performs field-level validation.
// It returns a list of validation errors for any constraint violations, or a fatal
// error if configuration cannot be loaded at all (e.g., file not found, unparseable).
//
// Merge priority (highest to lowest): environment variables > file values > defaults.
// The target must be a pointer to a struct with appropriate field tags for validation.
func Load(target interface{}, opts ...Option) ([]ValidationError, error) {
	if target == nil {
		return nil, &ConfigError{Op: "load", Err: fmt.Errorf("target must not be nil")}
	}

	rv := reflect.ValueOf(target)
	if rv.Kind() != reflect.Ptr || rv.IsNil() {
		return nil, &ConfigError{Op: "load", Err: fmt.Errorf("target must be a non-nil pointer to a struct")}
	}
	if rv.Elem().Kind() != reflect.Struct {
		return nil, &ConfigError{Op: "load", Err: fmt.Errorf("target must be a pointer to a struct, got pointer to %s", rv.Elem().Kind())}
	}

	l := &loader{}
	for _, opt := range opts {
		opt(l)
	}

	// Step 1: Apply defaults if provided
	if l.defaults != nil {
		if err := applyDefaults(target, l.defaults); err != nil {
			return nil, &ConfigError{Op: "apply_defaults", Err: err}
		}
	}

	// Step 2: Load from file if provided (overrides defaults)
	if l.filePath != "" {
		if err := loadFromFile(target, l.filePath); err != nil {
			return nil, &ConfigError{Op: "load_file", Path: l.filePath, Err: err}
		}
	}

	// Step 3: Load from environment variables if prefix provided (overrides file)
	if l.envPrefix != "" {
		loadFromEnv(target, l.envPrefix)
	}

	// Step 4: Validate the final config
	validationErrors := validate(target, "")

	return validationErrors, nil
}

// ValidationError represents a single field-level validation failure.
type ValidationError struct {
	// Field is the name or path of the field that failed validation.
	Field string
	// Value is the actual value that was invalid.
	Value interface{}
	// Message describes the constraint that was violated.
	Message string
}

// Error implements the error interface for ValidationError.
func (ve ValidationError) Error() string {
	return fmt.Sprintf("field %s: %s (value: %v)", ve.Field, ve.Message, ve.Value)
}

// ConfigError represents a fatal configuration loading error.
type ConfigError struct {
	Op   string
	Path string
	Err  error
}

func (e *ConfigError) Error() string {
	if e.Path != "" {
		return fmt.Sprintf("config %s [%s]: %v", e.Op, e.Path, e.Err)
	}
	return fmt.Sprintf("config %s: %v", e.Op, e.Err)
}

func (e *ConfigError) Unwrap() error {
	return e.Err
}

// Option configures the behavior of the Load function.
type Option func(*loader)

// loader holds internal configuration state used during loading.
type loader struct {
	envPrefix string
	filePath  string
	defaults  interface{}
}

// WithEnvPrefix sets the prefix used when reading environment variables.
// For example, WithEnvPrefix("TITANOPS") causes the loader to read
// TITANOPS_FIELD_NAME for a field named FieldName.
func WithEnvPrefix(prefix string) Option {
	return func(l *loader) {
		l.envPrefix = prefix
	}
}

// WithFile specifies a file path to load configuration from.
// Supported formats are determined by the file extension (e.g., .yaml, .yml, .json).
func WithFile(path string) Option {
	return func(l *loader) {
		l.filePath = path
	}
}

// WithDefaults provides a struct containing default values that are used
// when neither environment variables nor file sources specify a value.
func WithDefaults(defaults interface{}) Option {
	return func(l *loader) {
		l.defaults = defaults
	}
}

// applyDefaults copies non-zero values from defaults into target.
func applyDefaults(target, defaults interface{}) error {
	targetVal := reflect.ValueOf(target).Elem()
	defaultsVal := reflect.ValueOf(defaults)

	// If defaults is a pointer, dereference it
	if defaultsVal.Kind() == reflect.Ptr {
		if defaultsVal.IsNil() {
			return nil
		}
		defaultsVal = defaultsVal.Elem()
	}

	if defaultsVal.Kind() != reflect.Struct {
		return fmt.Errorf("defaults must be a struct, got %s", defaultsVal.Kind())
	}

	if targetVal.Type() != defaultsVal.Type() {
		return fmt.Errorf("defaults type %s does not match target type %s", defaultsVal.Type(), targetVal.Type())
	}

	copyNonZeroFields(targetVal, defaultsVal)
	return nil
}

// copyNonZeroFields copies non-zero field values from src to dst.
func copyNonZeroFields(dst, src reflect.Value) {
	for i := 0; i < src.NumField(); i++ {
		srcField := src.Field(i)
		dstField := dst.Field(i)

		if !dstField.CanSet() {
			continue
		}

		if srcField.Kind() == reflect.Struct {
			copyNonZeroFields(dstField, srcField)
		} else if !srcField.IsZero() {
			dstField.Set(srcField)
		}
	}
}

// loadFromFile reads a YAML or JSON file and unmarshals it into target.
func loadFromFile(target interface{}, path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("reading file: %w", err)
	}

	if len(data) == 0 {
		return nil
	}

	ext := strings.ToLower(path)
	switch {
	case strings.HasSuffix(ext, ".yaml") || strings.HasSuffix(ext, ".yml"):
		if err := yaml.Unmarshal(data, target); err != nil {
			return fmt.Errorf("parsing YAML: %w", err)
		}
	case strings.HasSuffix(ext, ".json"):
		if err := json.Unmarshal(data, target); err != nil {
			return fmt.Errorf("parsing JSON: %w", err)
		}
	default:
		// Try YAML first, then JSON
		if err := yaml.Unmarshal(data, target); err != nil {
			if err2 := json.Unmarshal(data, target); err2 != nil {
				return fmt.Errorf("unable to parse file (tried YAML and JSON): yaml: %v, json: %v", err, err2)
			}
		}
	}

	return nil
}

// loadFromEnv reads environment variables with the given prefix and sets
// matching struct fields. Uses underscore separation for nested fields.
// For example, prefix "TITANOPS" with field "Port" reads "TITANOPS_PORT".
func loadFromEnv(target interface{}, prefix string) {
	targetVal := reflect.ValueOf(target).Elem()
	loadEnvFields(targetVal, prefix)
}

// loadEnvFields recursively processes struct fields, building env var names.
func loadEnvFields(val reflect.Value, prefix string) {
	valType := val.Type()

	for i := 0; i < val.NumField(); i++ {
		field := val.Field(i)
		fieldType := valType.Field(i)

		if !field.CanSet() {
			continue
		}

		// Build env var name: PREFIX_FIELDNAME (uppercase)
		envName := prefix + "_" + toEnvName(fieldType.Name)

		if field.Kind() == reflect.Struct {
			// Recurse into nested structs
			loadEnvFields(field, envName)
			continue
		}

		envVal, ok := os.LookupEnv(envName)
		if !ok || envVal == "" {
			continue
		}

		setFieldFromString(field, envVal)
	}
}

// toEnvName converts a Go field name to an environment variable name component.
// For example, "FieldName" becomes "FIELD_NAME".
func toEnvName(name string) string {
	var result strings.Builder
	for i, r := range name {
		if i > 0 && r >= 'A' && r <= 'Z' {
			result.WriteByte('_')
		}
		if r >= 'a' && r <= 'z' {
			result.WriteByte(byte(r - 32))
		} else {
			result.WriteRune(r)
		}
	}
	return result.String()
}

// setFieldFromString parses a string value and assigns it to the given field.
func setFieldFromString(field reflect.Value, s string) {
	switch field.Kind() {
	case reflect.String:
		field.SetString(s)
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		if v, err := strconv.ParseInt(s, 10, 64); err == nil {
			field.SetInt(v)
		}
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		if v, err := strconv.ParseUint(s, 10, 64); err == nil {
			field.SetUint(v)
		}
	case reflect.Float32, reflect.Float64:
		if v, err := strconv.ParseFloat(s, 64); err == nil {
			field.SetFloat(v)
		}
	case reflect.Bool:
		if v, err := strconv.ParseBool(s); err == nil {
			field.SetBool(v)
		}
	}
}

// validate performs struct tag-based validation on the target, returning
// all validation errors found.
func validate(target interface{}, prefix string) []ValidationError {
	val := reflect.ValueOf(target)
	if val.Kind() == reflect.Ptr {
		val = val.Elem()
	}
	if val.Kind() != reflect.Struct {
		return nil
	}

	return validateStruct(val, prefix)
}

// validateStruct recursively validates struct fields based on `validate` tags.
func validateStruct(val reflect.Value, prefix string) []ValidationError {
	var errors []ValidationError
	valType := val.Type()

	for i := 0; i < val.NumField(); i++ {
		field := val.Field(i)
		fieldType := valType.Field(i)

		if !fieldType.IsExported() {
			continue
		}

		fieldName := fieldType.Name
		if prefix != "" {
			fieldName = prefix + "." + fieldName
		}

		// Recurse into nested structs
		if field.Kind() == reflect.Struct {
			nested := validateStruct(field, fieldName)
			errors = append(errors, nested...)
			continue
		}

		// Check validate tag
		tag := fieldType.Tag.Get("validate")
		if tag == "" {
			continue
		}

		fieldErrors := validateField(field, fieldName, tag)
		errors = append(errors, fieldErrors...)
	}

	return errors
}

// validateField checks a single field against its validation tag constraints.
func validateField(field reflect.Value, fieldName, tag string) []ValidationError {
	var errors []ValidationError

	parts := strings.Split(tag, ",")
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}

		switch {
		case part == "required":
			if field.IsZero() {
				errors = append(errors, ValidationError{
					Field:   fieldName,
					Value:   field.Interface(),
					Message: "field is required",
				})
			}

		case strings.HasPrefix(part, "min="):
			minStr := strings.TrimPrefix(part, "min=")
			if err := validateMin(field, fieldName, minStr); err != nil {
				errors = append(errors, *err)
			}

		case strings.HasPrefix(part, "max="):
			maxStr := strings.TrimPrefix(part, "max=")
			if err := validateMax(field, fieldName, maxStr); err != nil {
				errors = append(errors, *err)
			}

		case strings.HasPrefix(part, "oneof="):
			valuesStr := strings.TrimPrefix(part, "oneof=")
			if err := validateOneOf(field, fieldName, valuesStr); err != nil {
				errors = append(errors, *err)
			}
		}
	}

	return errors
}

// validateMin checks that a numeric field value is >= the minimum.
func validateMin(field reflect.Value, fieldName, minStr string) *ValidationError {
	switch field.Kind() {
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		min, err := strconv.ParseInt(minStr, 10, 64)
		if err != nil {
			return nil
		}
		if field.Int() < min {
			return &ValidationError{
				Field:   fieldName,
				Value:   field.Interface(),
				Message: fmt.Sprintf("value must be at least %d", min),
			}
		}
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		min, err := strconv.ParseUint(minStr, 10, 64)
		if err != nil {
			return nil
		}
		if field.Uint() < min {
			return &ValidationError{
				Field:   fieldName,
				Value:   field.Interface(),
				Message: fmt.Sprintf("value must be at least %d", min),
			}
		}
	case reflect.Float32, reflect.Float64:
		min, err := strconv.ParseFloat(minStr, 64)
		if err != nil {
			return nil
		}
		if field.Float() < min {
			return &ValidationError{
				Field:   fieldName,
				Value:   field.Interface(),
				Message: fmt.Sprintf("value must be at least %s", minStr),
			}
		}
	}
	return nil
}

// validateMax checks that a numeric field value is <= the maximum.
func validateMax(field reflect.Value, fieldName, maxStr string) *ValidationError {
	switch field.Kind() {
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		max, err := strconv.ParseInt(maxStr, 10, 64)
		if err != nil {
			return nil
		}
		if field.Int() > max {
			return &ValidationError{
				Field:   fieldName,
				Value:   field.Interface(),
				Message: fmt.Sprintf("value must be at most %d", max),
			}
		}
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		max, err := strconv.ParseUint(maxStr, 10, 64)
		if err != nil {
			return nil
		}
		if field.Uint() > max {
			return &ValidationError{
				Field:   fieldName,
				Value:   field.Interface(),
				Message: fmt.Sprintf("value must be at most %d", max),
			}
		}
	case reflect.Float32, reflect.Float64:
		max, err := strconv.ParseFloat(maxStr, 64)
		if err != nil {
			return nil
		}
		if field.Float() > max {
			return &ValidationError{
				Field:   fieldName,
				Value:   field.Interface(),
				Message: fmt.Sprintf("value must be at most %s", maxStr),
			}
		}
	}
	return nil
}

// validateOneOf checks that a string field value is one of the allowed values.
func validateOneOf(field reflect.Value, fieldName, valuesStr string) *ValidationError {
	if field.Kind() != reflect.String {
		return nil
	}

	// Skip oneof validation for zero-value strings (use required tag for that)
	val := field.String()
	if val == "" {
		return nil
	}

	allowed := strings.Fields(valuesStr)
	for _, a := range allowed {
		if val == a {
			return nil
		}
	}

	return &ValidationError{
		Field:   fieldName,
		Value:   val,
		Message: fmt.Sprintf("value must be one of: %s", strings.Join(allowed, ", ")),
	}
}
