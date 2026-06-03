package export

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"time"
)

// PrometheusBackend formats events as Prometheus exposition text metrics.
type PrometheusBackend struct {
	config *PrometheusConfig
}

// NewPrometheusBackend creates a new Prometheus backend with the given config.
func NewPrometheusBackend(config *PrometheusConfig) *PrometheusBackend {
	return &PrometheusBackend{config: config}
}

func (p *PrometheusBackend) Name() string { return "prometheus" }

func (p *PrometheusBackend) IsEnabled() bool {
	return p.config != nil && p.config.Enabled
}

// Send formats the event as Prometheus exposition text and logs it.
// Real HTTP serving will be added later.
func (p *PrometheusBackend) Send(_ context.Context, event Event) error {
	_, err := FormatPrometheus(event)
	if err != nil {
		return &BackendFormatError{BackendName: "prometheus", Cause: err}
	}
	return nil
}

// FormatPrometheus formats an event as Prometheus exposition text.
func FormatPrometheus(event Event) (string, error) {
	var sb strings.Builder

	// Sanitize metric name components
	module := sanitizeMetricName(event.Module)
	eventType := sanitizeMetricName(event.EventType)
	metricName := fmt.Sprintf("titanops_%s_%s_total", module, eventType)

	// Write HELP and TYPE lines
	sb.WriteString(fmt.Sprintf("# HELP %s TitanOps event counter\n", metricName))
	sb.WriteString(fmt.Sprintf("# TYPE %s counter\n", metricName))

	// Write the metric with labels
	labels := buildPrometheusLabels(event)
	sb.WriteString(fmt.Sprintf("%s{%s} 1 %d\n", metricName, labels, event.Timestamp.UnixMilli()))

	return sb.String(), nil
}

func sanitizeMetricName(s string) string {
	var sb strings.Builder
	for _, c := range s {
		if (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') || c == '_' {
			sb.WriteRune(c)
		} else {
			sb.WriteRune('_')
		}
	}
	return sb.String()
}

func buildPrometheusLabels(event Event) string {
	labels := []string{
		fmt.Sprintf("namespace=%q", event.Namespace),
		fmt.Sprintf("severity=%q", event.Severity),
		fmt.Sprintf("module=%q", event.Module),
		fmt.Sprintf("event_type=%q", event.EventType),
	}
	if event.Node != "" {
		labels = append(labels, fmt.Sprintf("node=%q", event.Node))
	}
	if event.Pod != "" {
		labels = append(labels, fmt.Sprintf("pod=%q", event.Pod))
	}
	if event.EventID != "" {
		labels = append(labels, fmt.Sprintf("event_id=%q", event.EventID))
	}
	return strings.Join(labels, ",")
}

// OTLPBackend serializes events as JSON for OTLP endpoint.
type OTLPBackend struct {
	config *OTLPConfig
}

// NewOTLPBackend creates a new OTLP backend with the given config.
func NewOTLPBackend(config *OTLPConfig) *OTLPBackend {
	return &OTLPBackend{config: config}
}

func (o *OTLPBackend) Name() string { return "otlp" }

func (o *OTLPBackend) IsEnabled() bool {
	return o.config != nil && o.config.Enabled
}

// Send formats the event as OTLP JSON and logs it.
// Real HTTP client will be added later.
func (o *OTLPBackend) Send(_ context.Context, event Event) error {
	_, err := FormatOTLP(event)
	if err != nil {
		return &BackendFormatError{BackendName: "otlp", Cause: err}
	}
	return nil
}

// otlpLogRecord represents an OTLP log record in JSON format.
type otlpLogRecord struct {
	TimeUnixNano         int64             `json:"timeUnixNano"`
	SeverityNumber       int               `json:"severityNumber"`
	SeverityText         string            `json:"severityText"`
	Body                 string            `json:"body"`
	Attributes           []otlpKeyValue    `json:"attributes"`
	Resource             otlpResource      `json:"resource"`
	TraceID              string            `json:"traceId,omitempty"`
	SpanID               string            `json:"spanId,omitempty"`
}

type otlpKeyValue struct {
	Key   string    `json:"key"`
	Value otlpValue `json:"value"`
}

type otlpValue struct {
	StringValue string `json:"stringValue,omitempty"`
}

type otlpResource struct {
	Attributes []otlpKeyValue `json:"attributes"`
}

// FormatOTLP formats an event as OTLP JSON.
func FormatOTLP(event Event) ([]byte, error) {
	record := otlpLogRecord{
		TimeUnixNano:   event.Timestamp.UnixNano(),
		SeverityNumber: severityToOTLPNumber(event.Severity),
		SeverityText:   strings.ToUpper(event.Severity),
		Body:           string(event.Payload),
		Attributes: []otlpKeyValue{
			{Key: "event_type", Value: otlpValue{StringValue: event.EventType}},
			{Key: "event_id", Value: otlpValue{StringValue: event.EventID}},
			{Key: "namespace", Value: otlpValue{StringValue: event.Namespace}},
			{Key: "module", Value: otlpValue{StringValue: event.Module}},
		},
		Resource: otlpResource{
			Attributes: []otlpKeyValue{
				{Key: "service.name", Value: otlpValue{StringValue: "titanops-" + event.Module}},
			},
		},
	}

	if event.Node != "" {
		record.Attributes = append(record.Attributes, otlpKeyValue{Key: "k8s.node.name", Value: otlpValue{StringValue: event.Node}})
	}
	if event.Pod != "" {
		record.Attributes = append(record.Attributes, otlpKeyValue{Key: "k8s.pod.name", Value: otlpValue{StringValue: event.Pod}})
	}

	data, err := json.Marshal(record)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal OTLP JSON: %w", err)
	}
	return data, nil
}

func severityToOTLPNumber(severity string) int {
	switch strings.ToLower(severity) {
	case "critical":
		return 21 // FATAL
	case "high":
		return 17 // ERROR
	case "medium":
		return 13 // WARN
	case "low":
		return 9 // INFO
	case "informational":
		return 5 // DEBUG
	default:
		return 0 // UNSPECIFIED
	}
}

// SplunkBackend formats events as Splunk HEC JSON payload.
type SplunkBackend struct {
	config *SplunkConfig
}

// NewSplunkBackend creates a new Splunk HEC backend with the given config.
func NewSplunkBackend(config *SplunkConfig) *SplunkBackend {
	return &SplunkBackend{config: config}
}

func (s *SplunkBackend) Name() string { return "splunk" }

func (s *SplunkBackend) IsEnabled() bool {
	return s.config != nil && s.config.Enabled
}

// Send formats the event as Splunk HEC JSON and logs it.
// Real HTTP client will be added later.
func (s *SplunkBackend) Send(_ context.Context, event Event) error {
	_, err := FormatSplunkHEC(event)
	if err != nil {
		return &BackendFormatError{BackendName: "splunk", Cause: err}
	}
	return nil
}

// splunkHECEvent represents a Splunk HTTP Event Collector payload.
type splunkHECEvent struct {
	Time       float64                `json:"time"`
	Host       string                 `json:"host,omitempty"`
	Source     string                 `json:"source"`
	SourceType string                 `json:"sourcetype"`
	Index      string                 `json:"index,omitempty"`
	Event      map[string]interface{} `json:"event"`
}

// FormatSplunkHEC formats an event as Splunk HEC JSON.
func FormatSplunkHEC(event Event) ([]byte, error) {
	host := event.Node
	if host == "" {
		host = event.Pod
	}

	hecEvent := splunkHECEvent{
		Time:       float64(event.Timestamp.UnixMilli()) / 1000.0,
		Host:       host,
		Source:     "titanops:" + event.Module,
		SourceType: "_json",
		Event: map[string]interface{}{
			"event_id":   event.EventID,
			"namespace":  event.Namespace,
			"severity":   event.Severity,
			"module":     event.Module,
			"event_type": event.EventType,
			"payload":    string(event.Payload),
			"node":       event.Node,
			"pod":        event.Pod,
			"labels":     event.Labels,
		},
	}

	data, err := json.Marshal(hecEvent)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal Splunk HEC JSON: %w", err)
	}
	return data, nil
}

// DynatraceBackend formats events as Dynatrace API JSON.
type DynatraceBackend struct {
	config *DynatraceConfig
}

// NewDynatraceBackend creates a new Dynatrace backend with the given config.
func NewDynatraceBackend(config *DynatraceConfig) *DynatraceBackend {
	return &DynatraceBackend{config: config}
}

func (d *DynatraceBackend) Name() string { return "dynatrace" }

func (d *DynatraceBackend) IsEnabled() bool {
	return d.config != nil && d.config.Enabled
}

// Send formats the event as Dynatrace API JSON and logs it.
// Real HTTP client will be added later.
func (d *DynatraceBackend) Send(_ context.Context, event Event) error {
	_, err := FormatDynatrace(event)
	if err != nil {
		return &BackendFormatError{BackendName: "dynatrace", Cause: err}
	}
	return nil
}

// dynatraceEvent represents a Dynatrace ingestion API event.
type dynatraceEvent struct {
	EventType  string            `json:"eventType"`
	Title      string            `json:"title"`
	Timeout    int               `json:"timeout,omitempty"`
	Properties map[string]string `json:"properties"`
	StartTime  int64             `json:"startTime"`
	EndTime    int64             `json:"endTime"`
}

// FormatDynatrace formats an event as Dynatrace API JSON.
func FormatDynatrace(event Event) ([]byte, error) {
	dtEventType := severityToDynatraceEventType(event.Severity)

	properties := map[string]string{
		"event_id":   event.EventID,
		"namespace":  event.Namespace,
		"module":     event.Module,
		"event_type": event.EventType,
		"severity":   event.Severity,
	}
	if event.Node != "" {
		properties["node"] = event.Node
	}
	if event.Pod != "" {
		properties["pod"] = event.Pod
	}
	for k, v := range event.Labels {
		properties["label."+k] = v
	}

	dtEvent := dynatraceEvent{
		EventType:  dtEventType,
		Title:      fmt.Sprintf("TitanOps [%s] %s: %s", event.Module, event.Severity, event.EventType),
		Properties: properties,
		StartTime:  event.Timestamp.UnixMilli(),
		EndTime:    event.Timestamp.UnixMilli(),
	}

	data, err := json.Marshal(dtEvent)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal Dynatrace JSON: %w", err)
	}
	return data, nil
}

func severityToDynatraceEventType(severity string) string {
	switch strings.ToLower(severity) {
	case "critical":
		return "AVAILABILITY_EVENT"
	case "high":
		return "ERROR_EVENT"
	case "medium":
		return "PERFORMANCE_EVENT"
	case "low":
		return "RESOURCE_CONTENTION_EVENT"
	case "informational":
		return "CUSTOM_INFO"
	default:
		return "CUSTOM_INFO"
	}
}

// WebhookBackend formats events as generic JSON with severity filtering.
type WebhookBackend struct {
	config     WebhookConfig
	severities map[string]bool
	timeout    time.Duration
	maxRetries int
	logger     *log.Logger
}

// NewWebhookBackend creates a new webhook backend with severity filtering.
func NewWebhookBackend(config WebhookConfig, logger *log.Logger) *WebhookBackend {
	severities := make(map[string]bool, len(config.Events))
	for _, s := range config.Events {
		severities[strings.ToLower(s)] = true
	}

	timeout := time.Duration(config.TimeoutSec) * time.Second
	if timeout <= 0 || timeout > 10*time.Second {
		timeout = 10 * time.Second
	}

	maxRetries := config.MaxRetries
	if maxRetries <= 0 {
		maxRetries = 3
	}

	return &WebhookBackend{
		config:     config,
		severities: severities,
		timeout:    timeout,
		maxRetries: maxRetries,
		logger:     logger,
	}
}

func (w *WebhookBackend) Name() string {
	return "webhook:" + w.config.Endpoint
}

func (w *WebhookBackend) IsEnabled() bool {
	return w.config.Endpoint != ""
}

// Send formats the event as JSON and dispatches it if it passes the severity filter.
// Real HTTP dispatch will be added later; for now it formats and returns nil.
func (w *WebhookBackend) Send(_ context.Context, event Event) error {
	// Filter by severity before dispatch
	if !w.matchesSeverity(event.Severity) {
		return nil // Event filtered out, not an error
	}

	_, err := FormatWebhook(event)
	if err != nil {
		return &BackendFormatError{BackendName: w.Name(), Cause: err}
	}

	// Real HTTP dispatch with timeout and retries will be added later.
	return nil
}

// matchesSeverity checks if the event severity matches the configured severity filter.
// If no severities are configured, all events pass.
func (w *WebhookBackend) matchesSeverity(severity string) bool {
	if len(w.severities) == 0 {
		return true
	}
	return w.severities[strings.ToLower(severity)]
}

// Timeout returns the configured timeout for webhook requests.
func (w *WebhookBackend) Timeout() time.Duration {
	return w.timeout
}

// MaxRetries returns the configured max retry count.
func (w *WebhookBackend) MaxRetries() int {
	return w.maxRetries
}

// webhookPayload represents the generic JSON webhook payload.
type webhookPayload struct {
	EventID   string            `json:"event_id"`
	Timestamp string            `json:"timestamp"`
	Namespace string            `json:"namespace"`
	Severity  string            `json:"severity"`
	Module    string            `json:"module"`
	EventType string            `json:"event_type"`
	Node      string            `json:"node,omitempty"`
	Pod       string            `json:"pod,omitempty"`
	Payload   string            `json:"payload"`
	Labels    map[string]string `json:"labels,omitempty"`
}

// FormatWebhook formats an event as generic JSON for webhook dispatch.
func FormatWebhook(event Event) ([]byte, error) {
	payload := webhookPayload{
		EventID:   event.EventID,
		Timestamp: event.Timestamp.UTC().Format(time.RFC3339Nano),
		Namespace: event.Namespace,
		Severity:  event.Severity,
		Module:    event.Module,
		EventType: event.EventType,
		Node:      event.Node,
		Pod:       event.Pod,
		Payload:   string(event.Payload),
		Labels:    event.Labels,
	}

	data, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal webhook JSON: %w", err)
	}
	return data, nil
}

// BackendFormatError represents a failure to format an event for a backend.
type BackendFormatError struct {
	BackendName string
	Cause       error
}

func (e *BackendFormatError) Error() string {
	return fmt.Sprintf("backend %q format failed: %v", e.BackendName, e.Cause)
}

func (e *BackendFormatError) Unwrap() error {
	return e.Cause
}
