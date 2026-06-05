package ollinai

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestMetrics_NewMetrics(t *testing.T) {
	m := NewMetrics()
	if m == nil {
		t.Fatal("NewMetrics() returned nil")
	}
	// Verify all metrics are registered
	if len(m.registry) != 12 {
		t.Errorf("expected 12 registered metrics, got %d", len(m.registry))
	}
}

func TestMetrics_Inc(t *testing.T) {
	m := NewMetrics()
	m.Inc(MetricDeploymentsTotal)
	m.Inc(MetricDeploymentsTotal)
	m.Inc(MetricDeploymentsTotal)

	got := m.GetCounter(MetricDeploymentsTotal)
	if got != 3 {
		t.Errorf("expected counter=3, got %d", got)
	}
}

func TestMetrics_Add(t *testing.T) {
	m := NewMetrics()
	m.Add(MetricEventsEmittedTotal, 5)
	m.Add(MetricEventsEmittedTotal, 3)

	got := m.GetCounter(MetricEventsEmittedTotal)
	if got != 8 {
		t.Errorf("expected counter=8, got %d", got)
	}
}

func TestMetrics_Set(t *testing.T) {
	m := NewMetrics()
	m.Set(MetricDeploymentRiskScoreCurrent, 73.5)

	got := m.Get(MetricDeploymentRiskScoreCurrent)
	if got != 73.5 {
		t.Errorf("expected gauge=73.5, got %f", got)
	}
}

func TestMetrics_Observe(t *testing.T) {
	m := NewMetrics()
	m.Observe(MetricPollDurationSeconds, 0.05)
	m.Observe(MetricPollDurationSeconds, 0.15)
	m.Observe(MetricPollDurationSeconds, 1.5)

	// Verify histogram was recorded (via handler output)
	handler := m.Handler()
	req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	body := rec.Body.String()
	if !strings.Contains(body, "titanops_ollinai_poll_duration_seconds_count 3") {
		t.Errorf("expected histogram count=3, body:\n%s", body)
	}
	if !strings.Contains(body, "titanops_ollinai_poll_duration_seconds_bucket{le=\"+Inf\"} 3") {
		t.Errorf("expected +Inf bucket=3, body:\n%s", body)
	}
}

func TestMetrics_Handler_ContentType(t *testing.T) {
	m := NewMetrics()
	handler := m.Handler()
	req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	ct := rec.Header().Get("Content-Type")
	if ct != "text/plain; version=0.0.4; charset=utf-8" {
		t.Errorf("unexpected Content-Type: %s", ct)
	}
}

func TestMetrics_Handler_ContainsAllMetrics(t *testing.T) {
	m := NewMetrics()
	// Set some values to exercise formatting
	m.Inc(MetricDeploymentsTotal)
	m.Set(MetricDeploymentRiskScoreCurrent, 85.0)
	m.Observe(MetricPollDurationSeconds, 0.1)

	handler := m.Handler()
	req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	body := rec.Body.String()

	expectedMetrics := []string{
		MetricDeploymentRiskScoreCurrent,
		MetricDeploymentsTotal,
		MetricChangeFailureRateRatio,
		MetricLeadTimeHours,
		MetricDeploymentFrequencyPerDay,
		MetricMTTRHours,
		MetricSupplyChainEventsTotal,
		MetricEventsEmittedTotal,
		MetricEventsBufferedCurrent,
		MetricEventsDroppedTotal,
		MetricPollErrorsTotal,
		MetricPollDurationSeconds,
	}

	for _, name := range expectedMetrics {
		if !strings.Contains(body, "# HELP "+name) {
			t.Errorf("missing HELP for %s", name)
		}
		if !strings.Contains(body, "# TYPE "+name) {
			t.Errorf("missing TYPE for %s", name)
		}
	}
}

func TestMetrics_Handler_PrometheusFormat(t *testing.T) {
	m := NewMetrics()
	m.Inc(MetricDeploymentsTotal)
	m.Set(MetricLeadTimeHours, 2.5)

	handler := m.Handler()
	req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	body := rec.Body.String()

	// Counter should show integer value
	if !strings.Contains(body, "titanops_ollinai_deployments_total 1") {
		t.Errorf("expected counter line, got:\n%s", body)
	}

	// Gauge should show float value
	if !strings.Contains(body, "titanops_ollinai_lead_time_hours 2.5") {
		t.Errorf("expected gauge line with 2.5, got:\n%s", body)
	}

	// TYPE declarations
	if !strings.Contains(body, "# TYPE titanops_ollinai_deployments_total counter") {
		t.Errorf("expected counter TYPE declaration, got:\n%s", body)
	}
	if !strings.Contains(body, "# TYPE titanops_ollinai_lead_time_hours gauge") {
		t.Errorf("expected gauge TYPE declaration, got:\n%s", body)
	}
	if !strings.Contains(body, "# TYPE titanops_ollinai_poll_duration_seconds histogram") {
		t.Errorf("expected histogram TYPE declaration, got:\n%s", body)
	}
}

func TestMetrics_IncUnknownMetric(t *testing.T) {
	m := NewMetrics()
	// Should not panic on unknown metric
	m.Inc("unknown_metric")
	m.Set("unknown_metric", 42.0)
	m.Observe("unknown_metric", 1.0)
}
