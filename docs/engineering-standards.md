# Engineering Standards: TitanOps

## Purpose

This document defines the engineering standards, patterns, and conventions for the TitanOps platform. These are derived from studying world-class open-source infrastructure projects and adapted to our specific domain (autonomous AiOps for Kubernetes).

The goal: every component of TitanOps should meet the quality bar of a CNCF graduated project.

---

## Reference Projects

| Project | What We Take | Domain Relevance |
|---------|-------------|-----------------|
| Cilium | eBPF lifecycle, Go structure, K8s operator patterns | Direct — same kernel-level approach |
| OpenTelemetry Collector | Pipeline architecture, fan-out export, retry/buffer | Direct — our export adapters |
| Prometheus | Metrics naming, exposition format, scrape patterns | Direct — our /metrics endpoints |
| NATS | Pub/sub patterns, JetStream durability, clustering | Direct — our event bus |
| Falco | Rule engine, kernel event processing, security patterns | Direct — eBeeControl + correlation |
| Argo CD | Go + gRPC + React, audit trail, RBAC | Direct — gateway + dashboard |
| CockroachDB | Property-based testing, simulation testing | Quality — our correctness properties |
| Istio | Multi-component Helm, versioning, compatibility matrix | Platform — our umbrella chart |
| etcd | Linearizability testing, upgrade compatibility | Quality — distributed correctness |
| Grafana | Dashboard JSON schema, plugin architecture, streaming | UI — our dashboard + Grafana integration |

---

## 1. Code Organization

### Go Project Structure (from Cilium, Argo CD)

```
module-name/
├── cmd/                    # Entry points (main.go files)
│   └── module-name/
│       └── main.go
├── pkg/                    # Public packages (importable by others)
│   ├── api/                # API types and interfaces
│   ├── config/             # Configuration loading
│   ├── controller/         # Reconciliation loops
│   └── metrics/            # Prometheus metrics definitions
├── internal/               # Private packages (not importable)
│   ├── bpf/                # eBPF program management
│   ├── engine/             # Core business logic
│   └── store/              # State management
├── proto/                  # Protobuf definitions
├── deploy/                 # Helm charts, K8s manifests
│   └── helm/
├── grafana/                # Dashboard JSON files
├── docs/                   # Documentation
├── test/                   # Integration and e2e tests
│   ├── integration/
│   └── e2e/
├── go.mod
├── go.sum
├── Makefile
├── Dockerfile
└── README.md
```

### Rules

- `pkg/` contains stable, versioned APIs that other modules may import
- `internal/` contains implementation details that can change without notice
- One `cmd/` entry point per binary
- No business logic in `cmd/` — only wiring and startup
- Test files live next to the code they test (`*_test.go`)
- Integration tests live in `test/` with build tags

---

## 2. API Design

### gRPC + REST Gateway (from Argo CD, Istio)

- Define APIs in protobuf first (source of truth)
- Generate Go server/client code with `buf`
- Expose REST via grpc-gateway for dashboard consumption
- Version APIs: `/api/v1/`, `/api/v2/`
- Use standard gRPC status codes
- Include request IDs in all responses for tracing

### Interface Design (from OpenTelemetry Collector)

```go
// Good: Small, focused interfaces
type Exporter interface {
    Export(ctx context.Context, event Event) error
    Shutdown(ctx context.Context) error
}

// Bad: God interfaces with 20 methods
type Everything interface {
    Export(...)
    Import(...)
    Transform(...)
    Validate(...)
    // ... 16 more methods
}
```

**Rules:**
- Interfaces should have 1-5 methods
- Accept interfaces, return structs
- Use `context.Context` as first parameter everywhere
- Return errors, never panic in library code

---

## 3. Error Handling

### Typed Errors (from CockroachDB, etcd)

```go
// Define error categories as types
type ErrorCategory string

const (
    ErrModelUnavailable  ErrorCategory = "model_unavailable"
    ErrInferenceTimeout  ErrorCategory = "inference_timeout"
    ErrBackendUnreachable ErrorCategory = "backend_unreachable"
    ErrValidationFailed  ErrorCategory = "validation_failed"
)

type Error struct {
    Category ErrorCategory
    Message  string
    Cause    error
    Module   string
}

func (e *Error) Error() string {
    return fmt.Sprintf("[%s] %s: %s", e.Category, e.Module, e.Message)
}

func (e *Error) Unwrap() error {
    return e.Cause
}
```

### Rules

- Never `panic()` in library code — return errors
- Never `log.Fatal()` in library code — only in `cmd/main.go`
- Wrap errors with context: `fmt.Errorf("loading model %s: %w", path, err)`
- Use `errors.Is()` and `errors.As()` for error checking
- Log at the boundary (handler/controller level), not deep in libraries
- Include structured fields in error logs (module, operation, duration)

---

## 4. Metrics & Observability

### Prometheus Metrics (from Prometheus naming conventions)

**Naming format:** `titanops_<module>_<metric>_<unit>`

```go
// Good
titanops_earthworm_anomaly_detections_total
titanops_earthworm_inference_duration_seconds
titanops_export_buffer_events_current
titanops_correlation_incidents_generated_total

// Bad
detections              // no namespace
earthworm_count         // ambiguous unit
export_buffer_size      // "size" is ambiguous (bytes? count?)
```

**Rules (from Prometheus best practices):**
- Use `_total` suffix for counters
- Use `_seconds` or `_bytes` suffix for histograms/summaries
- Use `_current` or `_info` for gauges
- Prefix all metrics with `titanops_<module>_`
- Use labels for dimensions, not metric name variations
- Keep cardinality bounded — never use unbounded label values (no pod IDs, no UUIDs)

### Structured Logging (from Cilium)

```go
// Use slog (Go 1.21+) with structured fields
logger.Info("autonomous action executed",
    "module", "earthworm",
    "action", "node_cordon",
    "node", nodeName,
    "confidence", score,
    "duration_ms", elapsed.Milliseconds(),
)
```

**Rules:**
- JSON format in production, text format in development
- Always include: timestamp, level, module, message
- Include trace/correlation IDs when available
- Log at appropriate levels:
  - `Error`: Something failed and needs attention
  - `Warn`: Degraded mode, fallback activated
  - `Info`: Significant state changes, autonomous actions taken
  - `Debug`: Detailed flow for troubleshooting

---

## 5. Testing Strategy

### Test Pyramid

```
         ╱╲
        ╱  ╲         E2E Tests (Helm deploy to kind, full flow)
       ╱────╲        ~10 tests, run in CI nightly
      ╱      ╲
     ╱────────╲      Integration Tests (cross-component, real K8s API)
    ╱          ╲     ~50 tests, run in CI on PR
   ╱────────────╲
  ╱              ╲   Property-Based Tests (correctness properties)
 ╱────────────────╲  18 properties × 100+ iterations, run in CI on PR
╱                  ╲
╱────────────────────╲ Unit Tests (isolated, fast, mocked dependencies)
                       ~500+ tests, run locally and in CI
```

### Property-Based Testing (from CockroachDB)

**Library:** `pgregory.net/rapid`

**Pattern:**
```go
func TestPropertyExportBufferEviction(t *testing.T) {
    // Feature: titanops-platform-integration, Property 5: Export buffer growth, eviction, and retry
    rapid.Check(t, func(t *rapid.T) {
        bufferCap := 1000
        eventCount := rapid.IntRange(0, 2000).Draw(t, "eventCount")
        
        buffer := NewRingBuffer(bufferCap)
        for i := 0; i < eventCount; i++ {
            buffer.Push(generateEvent(t))
        }
        
        // Property: buffer never exceeds capacity
        if buffer.Len() > bufferCap {
            t.Fatalf("buffer exceeded capacity: %d > %d", buffer.Len(), bufferCap)
        }
        
        // Property: if events > capacity, oldest are evicted
        if eventCount > bufferCap {
            assert.Equal(t, bufferCap, buffer.Len())
        }
    })
}
```

**Rules:**
- Minimum 100 iterations per property
- Tag every property test with design reference comment
- Properties test invariants, not specific examples
- Use generators for all inputs — never hardcode test data in property tests
- Unit tests complement properties with specific edge cases

### Integration Tests (from Cilium, etcd)

```go
//go:build integration

func TestCorrelationEngineEndToEnd(t *testing.T) {
    // Requires: running NATS, running event bus
    // ...
}
```

**Rules:**
- Use build tags (`//go:build integration`) to separate from unit tests
- Use `testcontainers-go` or `kind` for real dependencies
- Each integration test is self-contained (setup → exercise → verify → teardown)
- Clean up all resources in `t.Cleanup()`

---

## 6. Configuration

### Validation-First (from OpenTelemetry Collector)

```go
type Config struct {
    AI     AIConfig     `yaml:"ai" validate:"required"`
    Export ExportConfig `yaml:"export"`
}

type AIConfig struct {
    Provider string `yaml:"provider" validate:"oneof=local gemini bedrock vertex sagemaker"`
    Local    LocalAIConfig `yaml:"local"`
    Cloud    CloudAIConfig `yaml:"cloud"`
}

type ExportConfig struct {
    Prometheus PrometheusConfig `yaml:"prometheus"`
    OTLP       OTLPConfig       `yaml:"otlp"`
}

type PrometheusConfig struct {
    Enabled bool `yaml:"enabled"`
    Port    int  `yaml:"port" validate:"min=1024,max=65535"`
}
```

**Rules:**
- Validate all configuration at startup — fail fast with clear error messages
- Use struct tags for declarative validation
- Provide sensible defaults for everything (zero-config install)
- Document every field in `values.yaml` with comments
- Never read environment variables deep in code — load once at startup

---

## 7. eBPF Program Management

### Lifecycle (from Cilium)

```
Load → Attach → Monitor → Detach → Unload
```

**Rules:**
- eBPF programs are versioned alongside their userspace counterpart
- Always verify kernel compatibility before loading
- Implement graceful degradation if a BPF feature is unavailable
- Use BPF maps for kernel↔userspace communication (not perf events for hot path)
- Pin BPF maps to `/sys/fs/bpf/titanops/` for persistence across restarts
- Clean up all BPF resources on shutdown

### Ring Buffer Pattern (from Cilium)

```go
// Prefer ring buffers over perf buffers for new code
// - No per-CPU allocation waste
// - Better memory efficiency
// - Preserves event ordering
reader, err := ringbuf.NewReader(objs.Events)
```

---

## 8. Helm Chart Standards

### Chart Structure (from Istio, Cilium)

**Rules:**
- One `values.yaml` with all defaults documented
- Use `{{- if .Values.module.enabled }}` for conditional resources
- Include `NOTES.txt` with post-install instructions
- Include `tests/` directory with Helm test pods
- Use `_helpers.tpl` for shared template functions
- Label all resources with standard labels:

```yaml
labels:
  app.kubernetes.io/name: {{ .Chart.Name }}
  app.kubernetes.io/instance: {{ .Release.Name }}
  app.kubernetes.io/version: {{ .Chart.AppVersion }}
  app.kubernetes.io/component: {{ $component }}
  app.kubernetes.io/part-of: titanops
  app.kubernetes.io/managed-by: {{ .Release.Service }}
```

### RBAC (from Cilium)

- Principle of least privilege — only request permissions each module actually needs
- Use `ClusterRole` for cluster-wide resources (nodes, namespaces)
- Use `Role` for namespace-scoped resources (pods, secrets within titanops namespace)
- Document why each permission is needed in comments

---

## 9. Versioning & Releases

### Semantic Versioning (from etcd, Kubernetes)

**Rules:**
- Stay on `v0.x.x` until API is stable
- Every breaking change bumps MAJOR
- Deprecate before removing (mark in v0.3.0, remove in v0.4.0)
- Tag every release — no "just use main"
- Maintain a `CHANGELOG.md` per component

### Compatibility (from Istio)

- Umbrella chart declares compatibility matrix
- Shared libraries support N-1 consumer versions
- Breaking changes require migration guide
- CI tests against oldest supported version

---

## 10. Security

### Secrets (from Argo CD, Cilium)

- Never log secrets or tokens
- Use Kubernetes Secrets or external secret managers (AWS Secrets Manager)
- IRSA for AWS access — no static credentials in pods
- Rotate credentials without pod restart (watch for secret changes)

### eBPF Security (from Falco)

- Require `CAP_BPF` and `CAP_PERFMON` (not `CAP_SYS_ADMIN`)
- Validate BPF program signatures if possible
- Audit all BPF map access patterns
- Document kernel version requirements per feature

### Supply Chain

- Pin all dependency versions (no floating tags)
- Sign container images with cosign
- Generate SBOM for each release
- Run `govulncheck` in CI

---

## 11. Performance

### Hot Path Rules (from NATS, Cilium)

The hot path is: eBPF event → userspace read → AI inference → action execution

**Rules for hot path code:**
- Zero allocations in the event processing loop (pre-allocate buffers)
- No network calls (inference is local ONNX, actions are BPF map writes)
- No locks on the read path (use lock-free ring buffers)
- Batch BPF map reads when possible
- Profile with `pprof` — measure, don't guess

### Cold Path (acceptable latency)

- Cloud AI calls (training, explanations) — seconds are fine
- Export to backends — buffered, async, retried
- Dashboard API calls — 100ms target
- Helm install — minutes are fine

---

## 12. Documentation

### Per-Component (from Prometheus, Cilium)

Every component must have:
- `README.md` — what it does, how to install, quick start
- `ARCHITECTURE.md` — design decisions, data flow, component interaction
- `CONTRIBUTING.md` — how to build, test, submit changes
- `CHANGELOG.md` — version history with breaking changes highlighted

### API Documentation

- Protobuf files are self-documenting (use comments on every field)
- Generate API docs from proto files
- Include example requests/responses in docs
- Document error codes and their meaning

---

## 13. CI/CD Pipeline

### Stages (from Cilium, CockroachDB)

```yaml
# On every PR:
- lint:           golangci-lint, buf lint, helm lint
- unit-test:      go test -short ./... -race
- property-test:  go test -run Property ./... -count=1
- build:          go build, docker build
- security:       govulncheck, trivy scan

# On merge to main:
- integration:    tests against kind cluster
- helm-test:      helm install + helm test
- publish:        push images, push Helm chart

# Nightly:
- e2e:            full platform deploy + scenario tests
- performance:    benchmark suite with regression detection
```

### Rules

- All tests must pass before merge — no exceptions
- CI runs with `-race` flag (Go race detector)
- Linting is non-negotiable — fix warnings, don't suppress them
- Docker images are multi-stage (build → scratch/distroless)
- Every image has a health check endpoint

---

## 14. Operational Excellence

### Health Checks (from Kubernetes, Istio)

Every component exposes:
- `/healthz` — liveness (is the process alive?)
- `/readyz` — readiness (can it serve traffic?)
- `/metrics` — Prometheus metrics

### Graceful Shutdown (from NATS, etcd)

```go
// Handle SIGTERM gracefully
ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGTERM, syscall.SIGINT)
defer cancel()

// Drain in-flight work
server.Shutdown(ctx)
// Flush buffers
exporter.Flush(ctx)
// Close connections
eventBus.Close()
```

**Rules:**
- Respect Kubernetes termination grace period (default 30s)
- Drain in-flight events before shutdown
- Flush export buffers on shutdown
- Log shutdown reason and duration

---

## Summary: The Quality Bar

A TitanOps component is ready for release when:

1. ✅ All property-based tests pass (100+ iterations each)
2. ✅ All unit tests pass with race detector enabled
3. ✅ Integration tests pass against a real cluster
4. ✅ Helm chart installs cleanly with default values
5. ✅ `/metrics` endpoint exposes correctly-named Prometheus metrics
6. ✅ `/healthz` and `/readyz` endpoints respond correctly
7. ✅ Graceful shutdown completes within 30 seconds
8. ✅ No `golangci-lint` warnings
9. ✅ No known vulnerabilities (`govulncheck` clean)
10. ✅ Documentation is complete (README, ARCHITECTURE, CHANGELOG)
11. ✅ Configuration validates at startup with clear error messages
12. ✅ Errors are typed, wrapped with context, and never panic
