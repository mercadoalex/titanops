---
inclusion: auto
---

# TitanOps Engineering Standards

When writing or reviewing code for the TitanOps platform, follow these standards derived from world-class infrastructure projects (Cilium, OpenTelemetry Collector, Prometheus, CockroachDB, Argo CD, NATS, Falco, Istio, etcd).

## Full Reference

#[[file:docs/engineering-standards.md]]

## Key Rules (Always Apply)

### Code Organization
- Use `pkg/` for public importable packages, `internal/` for private implementation
- One `cmd/` entry point per binary, no business logic in main
- Test files live next to the code they test

### Error Handling
- Never `panic()` or `log.Fatal()` in library code — return typed errors
- Wrap errors with context: `fmt.Errorf("loading model %s: %w", path, err)`
- Use `errors.Is()` and `errors.As()` for error checking
- Log at the boundary (handler/controller), not deep in libraries

### Metrics Naming
- Format: `titanops_<module>_<metric>_<unit>`
- Use `_total` for counters, `_seconds`/`_bytes` for histograms, `_current` for gauges
- Never use unbounded label values (no pod IDs, no UUIDs)

### Testing
- Property-based tests for correctness properties (100+ iterations, `pgregory.net/rapid`)
- Run with `-race` flag always
- Integration tests use build tags (`//go:build integration`)
- Tag property tests: `// Feature: titanops-platform-integration, Property N: title`

### Interfaces
- 1-5 methods per interface
- Accept interfaces, return structs
- Always use `context.Context` as first parameter

### Configuration
- Validate all config at startup — fail fast with clear errors
- Provide sensible defaults for zero-config install
- Use struct tags for declarative validation

### Hot Path (eBPF event → inference → action)
- Zero allocations in event processing loop
- No network calls (local ONNX only)
- No locks on read path (lock-free ring buffers)

### Helm Charts
- Label all resources with `app.kubernetes.io/*` standard labels
- Include `app.kubernetes.io/part-of: titanops` on everything
- Use conditional rendering for toggleable modules
- Principle of least privilege for RBAC

### Security
- Never log secrets or tokens
- IRSA for AWS access — no static credentials
- Pin all dependency versions
- Run `govulncheck` in CI

### Every Component Must Have
- `/healthz` (liveness), `/readyz` (readiness), `/metrics` (Prometheus)
- Graceful shutdown within 30 seconds
- README.md, CHANGELOG.md
- No `golangci-lint` warnings
