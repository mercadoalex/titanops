package ollinai

import (
	"fmt"
	"math"
	"net/http"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
)

// Metric type constants for the metrics registry.
const (
	metricTypeCounter   = "counter"
	metricTypeGauge     = "gauge"
	metricTypeHistogram = "histogram"
)

// Default histogram buckets for poll duration (in seconds).
var defaultPollDurationBuckets = []float64{0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5, 10}

// Metric name constants following the naming convention titanops_ollinai_<metric>_<unit>.
const (
	MetricDeploymentRiskScoreCurrent = "titanops_ollinai_deployment_risk_score_current"
	MetricDeploymentsTotal           = "titanops_ollinai_deployments_total"
	MetricChangeFailureRateRatio     = "titanops_ollinai_change_failure_rate_ratio"
	MetricLeadTimeHours              = "titanops_ollinai_lead_time_hours"
	MetricDeploymentFrequencyPerDay  = "titanops_ollinai_deployment_frequency_per_day"
	MetricMTTRHours                  = "titanops_ollinai_mttr_hours"
	MetricSupplyChainEventsTotal     = "titanops_ollinai_supply_chain_events_total"
	MetricEventsEmittedTotal         = "titanops_ollinai_events_emitted_total"
	MetricEventsBufferedCurrent      = "titanops_ollinai_events_buffered_current"
	MetricEventsDroppedTotal         = "titanops_ollinai_events_dropped_total"
	MetricPollErrorsTotal            = "titanops_ollinai_poll_errors_total"
	MetricPollDurationSeconds        = "titanops_ollinai_poll_duration_seconds"
)

// metricInfo holds metadata about a registered metric.
type metricInfo struct {
	name     string
	help     string
	metaType string
}

// histogram holds the state for a histogram metric with fixed buckets.
type histogram struct {
	mu      sync.Mutex
	buckets []float64
	counts  []uint64
	sum     float64
	count   uint64
}

// newHistogram creates a histogram with the given bucket boundaries.
func newHistogram(buckets []float64) *histogram {
	sorted := make([]float64, len(buckets))
	copy(sorted, buckets)
	sort.Float64s(sorted)
	return &histogram{
		buckets: sorted,
		counts:  make([]uint64, len(sorted)),
	}
}

// observe records a value in the histogram.
func (h *histogram) observe(value float64) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.sum += value
	h.count++
	for i, boundary := range h.buckets {
		if value <= boundary {
			h.counts[i]++
		}
	}
}

// Metrics is a simple in-memory metrics registry that exposes values in
// Prometheus text exposition format. It supports counters, gauges, and histograms
// without requiring the prometheus/client_golang dependency.
type Metrics struct {
	// counters stores counter values as int64 (scaled by 1000 for sub-integer precision).
	counters sync.Map // map[string]*atomic.Int64

	// gauges stores gauge values as int64 bits of float64.
	gauges sync.Map // map[string]*atomic.Int64

	// histograms stores histogram instances.
	histograms sync.Map // map[string]*histogram

	// registry holds metric metadata in registration order.
	registry []metricInfo
}

// NewMetrics creates a new Metrics instance with all OllinAI metrics pre-registered.
func NewMetrics() *Metrics {
	m := &Metrics{}

	// Register gauges
	m.registerGauge(MetricDeploymentRiskScoreCurrent, "Latest deployment risk score per service.")
	m.registerGauge(MetricChangeFailureRateRatio, "Current DORA change failure rate.")
	m.registerGauge(MetricLeadTimeHours, "Current DORA lead time for changes in hours.")
	m.registerGauge(MetricDeploymentFrequencyPerDay, "Current DORA deployment frequency per day.")
	m.registerGauge(MetricMTTRHours, "Current DORA mean time to recovery in hours.")
	m.registerGauge(MetricEventsBufferedCurrent, "Events currently in ring buffer.")

	// Register counters
	m.registerCounter(MetricDeploymentsTotal, "Total deployments observed.")
	m.registerCounter(MetricSupplyChainEventsTotal, "Total supply chain security events by type.")
	m.registerCounter(MetricEventsEmittedTotal, "Total events published to NATS.")
	m.registerCounter(MetricEventsDroppedTotal, "Events dropped due to buffer overflow or retry exhaustion.")
	m.registerCounter(MetricPollErrorsTotal, "API polling errors.")

	// Register histogram
	m.registerHistogram(MetricPollDurationSeconds, "API polling latency in seconds.", defaultPollDurationBuckets)

	return m
}

// registerCounter registers a counter metric.
func (m *Metrics) registerCounter(name, help string) {
	val := &atomic.Int64{}
	m.counters.Store(name, val)
	m.registry = append(m.registry, metricInfo{name: name, help: help, metaType: metricTypeCounter})
}

// registerGauge registers a gauge metric.
func (m *Metrics) registerGauge(name, help string) {
	val := &atomic.Int64{}
	m.gauges.Store(name, val)
	m.registry = append(m.registry, metricInfo{name: name, help: help, metaType: metricTypeGauge})
}

// registerHistogram registers a histogram metric.
func (m *Metrics) registerHistogram(name, help string, buckets []float64) {
	h := newHistogram(buckets)
	m.histograms.Store(name, h)
	m.registry = append(m.registry, metricInfo{name: name, help: help, metaType: metricTypeHistogram})
}

// Inc increments a counter by 1.
func (m *Metrics) Inc(name string) {
	if val, ok := m.counters.Load(name); ok {
		val.(*atomic.Int64).Add(1)
	}
}

// Add adds a delta to a counter.
func (m *Metrics) Add(name string, delta int64) {
	if val, ok := m.counters.Load(name); ok {
		val.(*atomic.Int64).Add(delta)
	}
}

// Set sets a gauge to the given float64 value.
func (m *Metrics) Set(name string, value float64) {
	if val, ok := m.gauges.Load(name); ok {
		bits := math.Float64bits(value)
		val.(*atomic.Int64).Store(int64(bits))
	}
}

// Get returns the current float64 value of a gauge.
func (m *Metrics) Get(name string) float64 {
	if val, ok := m.gauges.Load(name); ok {
		bits := uint64(val.(*atomic.Int64).Load())
		return math.Float64frombits(bits)
	}
	return 0
}

// GetCounter returns the current int64 value of a counter.
func (m *Metrics) GetCounter(name string) int64 {
	if val, ok := m.counters.Load(name); ok {
		return val.(*atomic.Int64).Load()
	}
	return 0
}

// Observe records a value in a histogram.
func (m *Metrics) Observe(name string, seconds float64) {
	if val, ok := m.histograms.Load(name); ok {
		val.(*histogram).observe(seconds)
	}
}

// Handler returns an http.Handler that serves the /metrics endpoint in
// Prometheus text exposition format.
func (m *Metrics) Handler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain; version=0.0.4; charset=utf-8")

		var sb strings.Builder

		for _, info := range m.registry {
			sb.WriteString(fmt.Sprintf("# HELP %s %s\n", info.name, info.help))
			sb.WriteString(fmt.Sprintf("# TYPE %s %s\n", info.name, info.metaType))

			switch info.metaType {
			case metricTypeCounter:
				if val, ok := m.counters.Load(info.name); ok {
					v := val.(*atomic.Int64).Load()
					sb.WriteString(fmt.Sprintf("%s %d\n", info.name, v))
				}
			case metricTypeGauge:
				if val, ok := m.gauges.Load(info.name); ok {
					bits := uint64(val.(*atomic.Int64).Load())
					fv := math.Float64frombits(bits)
					sb.WriteString(fmt.Sprintf("%s %s\n", info.name, formatFloat(fv)))
				}
			case metricTypeHistogram:
				if val, ok := m.histograms.Load(info.name); ok {
					h := val.(*histogram)
					h.mu.Lock()
					for i, boundary := range h.buckets {
						sb.WriteString(fmt.Sprintf("%s_bucket{le=\"%s\"} %d\n", info.name, formatFloat(boundary), h.counts[i]))
					}
					sb.WriteString(fmt.Sprintf("%s_bucket{le=\"+Inf\"} %d\n", info.name, h.count))
					sb.WriteString(fmt.Sprintf("%s_sum %s\n", info.name, formatFloat(h.sum)))
					sb.WriteString(fmt.Sprintf("%s_count %d\n", info.name, h.count))
					h.mu.Unlock()
				}
			}

			sb.WriteString("\n")
		}

		fmt.Fprint(w, sb.String())
	})
}

// formatFloat formats a float64 for Prometheus exposition format.
// It avoids scientific notation and trailing zeros for clean output.
func formatFloat(f float64) string {
	if f == float64(int64(f)) {
		return fmt.Sprintf("%d", int64(f))
	}
	// Use enough precision without scientific notation
	s := fmt.Sprintf("%g", f)
	return s
}
