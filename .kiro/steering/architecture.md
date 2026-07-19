---
inclusion: auto
---

# TitanOps Architecture Discipline

These rules enforce layer separation, offline testability, and evaluation rigor across all TitanOps modules. They complement the engineering standards and apply to every code change in this repository.

## 1. Three-Layer Separation

Every module (Go or TypeScript) follows three logical layers:

```
┌─────────────────────────────────────────┐
│  Infrastructure (adapters, drivers)      │  ← K8s client, NATS, HTTP, eBPF, DB
├─────────────────────────────────────────┤
│  Engine / Core (business logic)          │  ← Scoring, correlation, classification
├─────────────────────────────────────────┤
│  Interface (ports, contracts)            │  ← Go interfaces, protobuf types
└─────────────────────────────────────────┘
```

### Hard Rules

- **Engine packages MUST NOT import infrastructure packages.** The engine (scoring, correlation, classification, decision logic) depends only on interfaces and standard library. Infrastructure flows inward through interfaces defined by the core.
- **Infrastructure adapters implement interfaces defined in the core/interface layer.** Never the reverse.
- **`cmd/` wires layers together.** Dependency injection happens at the entry point. Engine code never instantiates its own infrastructure.

### Enforcement Checklist (apply on every PR touching module internals)

- [ ] No `import "k8s.io/..."` in engine packages
- [ ] No `import "github.com/nats-io/..."` in engine packages
- [ ] No direct HTTP/gRPC client usage in engine packages
- [ ] No database driver imports in engine packages
- [ ] Engine testable with zero network, zero containers, zero external state

### Go Module Layout Mapping

| Layer | Location | May import |
|-------|----------|-----------|
| Interface | `pkg/api/`, `internal/ports/` | std, protobuf types |
| Engine | `internal/engine/`, `internal/scoring/` | interfaces, std, shared libs |
| Infrastructure | `internal/adapters/`, `internal/bpf/` | anything (implements interfaces) |

### TypeScript (BrainOps) Mapping

| Layer | Location | May import |
|-------|----------|-----------|
| Interface | `src/*/types.ts`, `src/*/ports.ts` | types only |
| Engine | `src/agent/`, `src/safety/`, `src/self-optimization/` | interfaces, no I/O packages |
| Infrastructure | `src/db/`, `src/nats/`, `src/mcp/`, `src/ccloud/` | anything (implements interfaces) |

---

## 2. Mock/Offline Mode

Every module MUST function in mock mode without external dependencies. Mock mode is the CI default.

### Requirements

- A standard run mode is read from `TITANOPS_MODE` environment variable: `live` | `mock` | `dry-run`
- **`mock`**: All infrastructure adapters are replaced with synthetic/in-memory implementations. Deterministic outputs. No network calls.
- **`dry-run`**: Real sources (eBPF, K8s API) but no actuator side-effects. Actions are logged, not executed.
- **`live`**: Full production behavior.

### Rules

- CI pipelines MUST use `TITANOPS_MODE=mock` unless explicitly testing integration.
- Mock adapters live alongside their real counterparts (e.g., `internal/adapters/nats_mock.go`).
- Mock mode MUST be deterministic given the same seed. Use seeded random generators, not `time.Now()` for synthetic data.
- Every new infrastructure adapter MUST ship with a corresponding mock implementation that satisfies the same interface.
- Property-based tests SHOULD use mock mode exclusively — no flaky network dependencies.

### Dry-Run Actuator Pattern

```go
type Actuator interface {
    Execute(ctx context.Context, action Action) (Result, error)
}

// In dry-run mode:
type DryRunActuator struct{ log *slog.Logger }

func (d *DryRunActuator) Execute(ctx context.Context, action Action) (Result, error) {
    d.log.Info("dry-run: would execute", "action", action.Type, "target", action.Target)
    return Result{Executed: false, Reason: "dry-run"}, nil
}
```

---

## 3. Structured Evaluation Framework

Any change to scoring logic, classification thresholds, or AI model integration MUST include an evaluation report demonstrating no regression.

### Scenario Format

Evaluation scenarios are defined in YAML under `eval/scenarios/<module>/`:

```yaml
# eval/scenarios/correlation/cross_module_basic.yaml
name: "Cross-module correlation - basic case"
description: "Two modules reporting same node within window generates incident"
inputs:
  events:
    - module: earthworm
      event_type: heartbeat_degraded
      node: node-1
      timestamp: "2024-01-01T00:00:00Z"
    - module: quack
      event_type: cpu_anomaly
      node: node-1
      timestamp: "2024-01-01T00:01:30Z"
  config:
    time_window_seconds: 120
    confidence_threshold: 60
expected:
  incidents_generated: 1
  min_confidence: 60
  narrative_contains: ["earthworm", "quack", "node-1"]
```

### Rules

- **Scoring changes need eval reports.** Any PR that modifies confidence scoring, severity mapping, anomaly thresholds, or classification logic MUST run the eval suite and include the report summary in the PR description.
- **No regression gating.** If an eval scenario that previously passed now fails, the PR is blocked until the regression is explained and either the scenario is updated (with justification) or the code is fixed.
- **New scoring features require new scenarios.** Adding a scoring dimension or classification category requires at least 3 eval scenarios covering: normal case, boundary case, and adversarial/edge case.
- **Eval runs in CI** as part of the PR pipeline using `TITANOPS_MODE=mock`. The eval runner produces a structured JSON report.

### Report Format

```
eval/reports/<module>/<date>-<commit-short>.json
```

Contains: pass/fail per scenario, confidence score distributions, delta from previous run, and any regressions flagged.

---

## Quick Reference (Copy-Paste for PR Reviews)

When reviewing any PR in this repository, verify:

1. **Layer separation**: Engine packages have no infrastructure imports
2. **Mock companion**: New adapters include a mock implementation
3. **Eval coverage**: Scoring/threshold changes include eval report
4. **CI mode**: Test configurations default to `TITANOPS_MODE=mock`
5. **Determinism**: Mock mode produces repeatable results (seeded randomness)
