# Module Integration Guide

## Overview

Each TitanOps module was built standalone with a specific purpose. The platform strategy doesn't change what modules do internally — it adds a thin integration layer on top so they can participate in cross-module correlation and agent-enhanced reasoning.

**Key principle: don't change what works inside modules. Add a thin integration layer on top.**

---

## The Integration Contract

```
┌─────────────────────────────────────────────────────────────┐
│                    Integration Contract                       │
│                                                              │
│  Module MUST:                                                │
│  1. Emit events in protobuf schema → NATS                   │
│  2. Expose /metrics (Prometheus) + /healthz                  │
│  3. Accept config via environment variables (Helm values)    │
│                                                              │
│  Module MUST NOT:                                            │
│  4. Know about other modules                                 │
│  5. Talk to CockroachDB directly (that's BrainOps's job)    │
│  6. Depend on the platform to function standalone            │
└─────────────────────────────────────────────────────────────┘
```

---

## How Each Module Integrates

| Module | Today (standalone) | Platform Integration (additive) |
|--------|-------------------|-------------------------------|
| **Earthworm** | eBPF → ONNX → cordon/restart | + Publish events to NATS via titanops-export |
| **Tlapix** | eBPF → detect → renew | + Publish events to NATS (already has OTLP export) |
| **Quack** | sched_ext → rebalance | + Publish events to NATS via titanops-export |
| **eBeeControl** | Tetragon → classify → isolate | + Publish events to NATS (rewrite to Go adds shared libs) |
| **OllinAI** | CI/CD → risk score → DORA | + OllinAI Adapter publishes to NATS |

**The integration is ONE thing per module: publish a protobuf event to NATS when something significant happens.** That's it. Everything else stays the same.

---

## The Glue: titanops-export Library

Each Go module adds ONE import to participate in the platform:

```go
// In earthworm/main.go (or wherever the module emits actions)
import "github.com/mercadoalex/titanops/shared/titanops-export"

// After Earthworm cordons a node:
event := export.Event{
    Module:    "earthworm",
    EventType: "node_cordoned",
    Severity:  export.SeverityHigh,
    Node:      nodeName,
    Namespace: namespace,
    Payload:   marshaledDetails,
    Timestamp: time.Now().UTC(),
}
exporter.Publish(ctx, event)  // → Goes to NATS
```

For Tlapix (Rust), the integration is a small Go sidecar or an OTEL-to-NATS bridge configured via the Helm chart.

---

## Preventing Chaos: The Rules

| Rule | Why |
|------|-----|
| **Modules never import other modules** | No coupling between Earthworm and Tlapix code |
| **Modules only import shared libraries** | One-way dependency: module → shared lib |
| **Shared libraries never import modules** | Prevents circular dependencies |
| **The event schema is versioned (protobuf)** | Module A can't break Module B by changing a field |
| **Platform features are additive** | Module works without NATS, without BrainOps, without dashboard |
| **Config comes from environment** | Helm values → env vars → module reads. No hardcoded platform assumptions |
| **Graceful degradation on missing platform** | If NATS_URL is empty, skip publishing — module still functions |

---

## The Integration Sequence (Per Module)

For each existing module, the integration is a single PR:

```
Step 1: Add titanops-export as a Go dependency (or OTLP bridge for Rust)
Step 2: At each "significant action" point, emit an event
Step 3: Add NATS_URL to the module's Helm values.yaml
Step 4: Done. Module still works standalone (if NATS_URL is empty, skip publishing)
```

**Estimated effort per module: 1-2 hours.** Not a rewrite. Not a refactor. Just "also tell the platform what you just did."

---

## The Contract Document: proto/events.proto

This is the single source of truth for what an "event" looks like. All modules must emit events conforming to this schema:

```protobuf
// proto/events.proto — versioned, backward-compatible changes only
syntax = "proto3";
package titanops.events.v1;

import "google/protobuf/timestamp.proto";

enum Severity {
    SEVERITY_UNSPECIFIED = 0;
    SEVERITY_INFORMATIONAL = 1;
    SEVERITY_LOW = 2;
    SEVERITY_MEDIUM = 3;
    SEVERITY_HIGH = 4;
    SEVERITY_CRITICAL = 5;
}

enum Module {
    MODULE_UNSPECIFIED = 0;
    MODULE_EARTHWORM = 1;
    MODULE_TLAPIX = 2;
    MODULE_EBEECONTROL = 3;
    MODULE_QUACK = 4;
    MODULE_OLLINAI = 5;
    MODULE_CORRELATION = 6;
}

message Event {
    string namespace = 1;
    google.protobuf.Timestamp timestamp = 2;
    Severity severity = 3;
    Module module = 4;
    string event_type = 5;
    bytes payload = 6;          // max 64KB, module-specific JSON
    optional string node = 7;
    optional string pod = 8;
    string event_id = 9;        // UUID v4
    map<string, string> labels = 10;
}
```

If a module emits events matching this schema → the platform works (correlation, BrainOps, dashboard). If it doesn't → it still works standalone, just without cross-module intelligence.

---

## Event Types Per Module

### Earthworm Events

| event_type | Severity | When Emitted |
|-----------|----------|-------------|
| `heartbeat_degraded` | HIGH | Node heartbeat anomaly detected |
| `node_cordoned` | HIGH | Autonomous cordon executed |
| `pod_restarted` | MEDIUM | Autonomous pod restart executed |
| `anomaly_detected` | MEDIUM | ONNX model detects anomaly below action threshold |

### Tlapix Events

| event_type | Severity | When Emitted |
|-----------|----------|-------------|
| `cert_expiring` | HIGH | Certificate within 7 days of expiry |
| `shadow_cert_detected` | CRITICAL | Unknown certificate found |
| `cert_renewed` | INFORMATIONAL | Successful autonomous renewal |
| `cert_rotation_failed` | CRITICAL | Renewal attempt failed |

### Quack Events

| event_type | Severity | When Emitted |
|-----------|----------|-------------|
| `cpu_anomaly` | MEDIUM | Anomalous CPU scheduling pattern |
| `noisy_neighbor_detected` | HIGH | Pod consuming unfair CPU share |
| `rebalance_executed` | INFORMATIONAL | CPU rebalance completed |

### eBeeControl Events

| event_type | Severity | When Emitted |
|-----------|----------|-------------|
| `honeytoken_accessed` | CRITICAL | Deception token triggered |
| `threat_classified` | HIGH | Threat classification complete |
| `pod_isolated` | HIGH | Autonomous pod isolation executed |

### OllinAI Events

| event_type | Severity | When Emitted |
|-----------|----------|-------------|
| `deployment_risk` | Mapped from score | Deployment risk score computed |
| `dora_metrics` | INFORMATIONAL | DORA metrics updated |
| `supply_chain_credential_exfil` | CRITICAL | eBPF detected credential theft |
| `supply_chain_process_anomaly` | HIGH | Unauthorized process in CI runner |

---

## Where Code Lives (Final State)

```
github.com/mercadoalex/
│
├── titanops/                    ← Platform core
│   ├── shared/titanops-export/  ← Event publishing library (Go)
│   ├── shared/titanops-ai/      ← ONNX + cloud AI interface (Go)
│   ├── shared/titanops-k8s/     ← K8s client helpers (Go)
│   ├── shared/titanops-config/  ← Config loading (Go)
│   ├── correlation/             ← Subscribes to NATS, generates incidents
│   ├── gateway/                 ← REST API for dashboard
│   ├── dashboard/               ← React UI
│   ├── brainops/                ← LangGraph agent + MCP server (TypeScript)
│   ├── proto/                   ← Event schema (protobuf) — THE CONTRACT
│   ├── helm/                    ← Umbrella chart + module sub-charts
│   └── infra/                   ← Terraform (EKS, CockroachDB, NATS)
│
├── earthworm/                   ← Adds: import titanops-export, emit events
├── tlapix/                      ← Adds: OTLP → NATS bridge (or Go sidecar)
├── quack/                       ← Adds: import titanops-export, emit events
├── ebeecontrol/                 ← Phase 2-3: rewrite to Go, use shared libs
└── OllinAI/                     ← Adds: OllinAI Adapter (Go) publishes to NATS
```

---

## Priority Order for Integration

| # | Module | Effort | Why This Order |
|---|--------|--------|---------------|
| 1 | **Earthworm** | 1h | Already Go, simplest integration, health events most valuable for demos |
| 2 | **Tlapix** | 2h | Already has OTLP export — bridge to NATS is straightforward |
| 3 | **Quack** | 1h | Go, same pattern as Earthworm |
| 4 | **OllinAI** | 4h | Need Go adapter sidecar (OllinAI is TS/Rust) |
| 5 | **eBeeControl** | Later | Rewrite to Go is a bigger effort (Phase 2-3) |

---

## Graceful Degradation Matrix

| Platform Component | If Missing | Module Behavior |
|-------------------|-----------|-----------------|
| NATS event bus | Not deployed | Module works standalone, events not published |
| Correlation Engine | Not deployed | No cross-module incidents generated |
| BrainOps | Not deployed | No historical reasoning, modules still act autonomously |
| CockroachDB | Not deployed | BrainOps disabled, everything else works |
| Dashboard | Not deployed | No UI, but all automation still functions |
| Prometheus | Not configured | No metrics export, module still operates |

**Nothing is required for a module to function. Everything is additive.**

---

## Technical Debt Prevention Checklist

Before merging any integration PR, verify:

- [ ] Module still passes all tests with `NATS_URL=""` (graceful degradation)
- [ ] Module doesn't import any other module (only shared libs)
- [ ] Events conform to proto/events.proto schema
- [ ] Helm values.yaml documents all new env vars
- [ ] No hardcoded platform URLs or assumptions
- [ ] /healthz and /metrics endpoints still work
- [ ] Integration is behind a feature flag or env var check
