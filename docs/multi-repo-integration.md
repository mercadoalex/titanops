# Multi-Repo Integration Strategy

## Problem

TitanOps is the integrator platform. Each module (Quack, Earthworm, Tlapix, eBeeControl, OllinAI) lives in its own repository. When a module makes progress — adds event types, adopts mock mode, bumps shared lib versions, produces eval reports — the TitanOps integrator needs to know about it without manual coordination.

---

## Industry Patterns (Reference)

Before describing our solution, here's how established multi-repo projects solve this:

### Istio: Release-Builder

Istio has ~10 repos (istio, envoy, api, proxy, tools, etc.). They coordinate via a dedicated release-builder repo that defines a `manifest.yaml` listing every repo + exact SHA to include in a release, pulls all sources, builds everything together, and produces a manifest output documenting what went in. Integration is validated at build-time, not at push-time.

Key insight: Istio does NOT rely on async notifications between repos. They do periodic integration builds that assemble all components from pinned SHAs.

Source: [istio/release-builder](https://github.com/istio/release-builder)

### Backstage/Spotify: Software Catalog

Backstage uses `catalog-info.yaml` in each repo (the entity descriptor format). A central catalog service crawls registered repos and ingests these descriptors. No push-based notification needed. The catalog is a read-only aggregator — it pulls from repos, not the other way around. Repos don't need to "notify" anyone; the catalog discovers state on its own schedule.

Source: [backstage.io/docs/features/software-catalog/descriptor-format](https://backstage.io/docs/next/features/software-catalog/descriptor-format)

### Buf Schema Registry: Proto Contract Enforcement

For protobuf contracts, Buf Schema Registry (BSR) acts as the single source of truth. Each repo pushes schema changes to the BSR, which validates breaking changes, manages versions, and generates code. The contract (proto schema) lives in a registry, not in individual repos.

Source: [buf.build/docs/bsr](https://buf.build/docs/bsr)

### Helm Umbrella Chart: Package-Manager Coordination

Each module publishes its own Helm chart to a chart repository (OCI, ChartMuseum). The umbrella chart declares `dependencies:` with version constraints. `helm dependency update` resolves them. The coordination mechanism is the package manager itself.

---

## TitanOps Approach: Three Layers

We use a combination of industry tools and a lightweight custom manifest for the gaps:

| Concern | Tool | Why |
|---------|------|-----|
| Event schema evolution | **Buf Schema Registry** | Breaking change detection, code generation, versioned deps |
| Release coordination | **Helm OCI registry** | Package manager handles version resolution |
| Runtime contract (healthz, metrics) | **Kubernetes probes + Prometheus** | Already built into K8s |
| Development visibility | **Simplified manifest** (`.titanops/manifest.yaml`) | Compliance flags, eval status — what no standard tool covers |
| Integration testing | **Periodic integration build** (Istio pattern) | Daily CI that deploys all modules to `kind` and runs cross-module tests |

### Layer 1: Proto Schema Registry (Buf)

The `proto/events.proto` in TitanOps is the canonical event schema. It gets pushed to a Buf registry (buf.build or self-hosted). Each module repo declares a dependency:

```yaml
# In module repo: buf.yaml
deps:
  - buf.build/mercadoalex/titanops-events
```

When a module needs a new event type → PR to the proto schema → Buf validates backward compatibility → modules generate code from the registry version they depend on.

This enforces: event schema evolution, breaking change detection, cross-module type safety.

### Layer 2: Helm Chart Registry (OCI)

Each module publishes its Helm chart to an OCI-compatible registry (GitHub Packages, AWS ECR, or similar). The TitanOps umbrella chart references them with semver constraints:

```yaml
# helm/titanops/Chart.yaml
dependencies:
  - name: quack
    version: ">=0.3.0 <0.4.0"
    repository: "oci://ghcr.io/mercadoalex/charts"
  - name: earthworm
    version: ">=0.2.0 <0.3.0"
    repository: "oci://ghcr.io/mercadoalex/charts"
```

`helm dependency update` resolves compatible versions. Breaking chart changes bump major → umbrella must explicitly adopt.

This enforces: version compatibility, release coordination, independent deployability.

### Layer 3: Integration Manifest (Custom — Development Visibility)

The `.titanops/manifest.yaml` covers what Buf and Helm do NOT handle: architecture compliance status, mock mode adoption, eval coverage, and engineering discipline adherence. This is a lightweight status signal, not a full coordination system.

---

## Solution Detail: Integration Manifests + CI Validation

### 1. Module Manifest (`.titanops/manifest.yaml`)

Every module repo publishes a machine-readable manifest at a known path. This covers architecture compliance and development visibility — the aspects that Buf (schema) and Helm (release versions) don't handle.

```yaml
# .titanops/manifest.yaml
module: quack
version: 0.3.2
repository: github.com/mercadoalex/quack

# Events this module emits (canonical schema lives in Buf registry,
# this list is for quick reference and CI cross-check)
events:
  - type: cpu_anomaly
    severity: medium
    schema_version: 1
  - type: noisy_neighbor_detected
    severity: high
    schema_version: 1
  - type: rebalance_executed
    severity: informational
    schema_version: 1

# Shared library compatibility (Helm chart declares runtime deps,
# this tracks build-time Go module dependencies)
dependencies:
  titanops_export: ">=0.1.0"
  titanops_config: ">=0.1.0"

# Architecture compliance status (per .kiro/steering/architecture.md)
compliance:
  layer_separation: true       # Engine has no infra imports
  mock_mode: true              # TITANOPS_MODE=mock supported
  dry_run_mode: true           # TITANOPS_MODE=dry-run supported
  eval_scenarios: 4            # Number of eval scenarios
  dedup_compatible: true       # Events support dedup fingerprinting

# Health endpoints
health:
  liveness: /healthz
  readiness: /readyz
  metrics: /metrics

# Last successful eval report (relative path within module repo)
eval:
  last_report: eval/reports/latest.json
  scenarios_path: eval/scenarios/
```

### 2. TitanOps Registry (`integration/registry.yaml`)

TitanOps maintains a registry of all known modules and their repo coordinates:

```yaml
# integration/registry.yaml
modules:
  - name: quack
    repo: github.com/mercadoalex/quack
    branch: main
    manifest_path: .titanops/manifest.yaml

  - name: earthworm
    repo: github.com/mercadoalex/titanops
    path: modules/earthworm  # in-repo module
    manifest_path: .titanops/manifest.yaml

  - name: tlapix
    repo: github.com/mercadoalex/tlapix
    branch: main
    manifest_path: .titanops/manifest.yaml

  - name: ebeecontrol
    repo: github.com/mercadoalex/ebeecontrol
    branch: main
    manifest_path: .titanops/manifest.yaml

  - name: ollinai
    repo: github.com/mercadoalex/titanops
    path: modules/ollinai  # in-repo module
    manifest_path: .titanops/manifest.yaml
```

### 3. CI Validation Flow

```
┌─────────────────────────────────────────────────────────────┐
│  Module Repo (e.g., Quack)                                   │
│                                                              │
│  PR merged → GitHub Action runs:                             │
│  1. Run eval suite (TITANOPS_MODE=mock)                      │
│  2. Update .titanops/manifest.yaml (version, eval count)     │
│  3. Push updated manifest                                    │
│  4. Notify TitanOps via repository_dispatch event            │
└─────────────────────┬───────────────────────────────────────┘
                      │ repository_dispatch
                      ▼
┌─────────────────────────────────────────────────────────────┐
│  TitanOps Repo                                               │
│                                                              │
│  Integration check triggered:                                │
│  1. Fetch manifests from all module repos                    │
│  2. Validate event types against proto/events.proto          │
│  3. Verify shared lib version compatibility                  │
│  4. Check compliance flags (mock_mode, eval_scenarios >= 3)  │
│  5. Update integration/status.json (dashboard reads this)    │
│  6. Fail if any module breaks the contract                   │
└─────────────────────────────────────────────────────────────┘
```

### 4. GitHub Actions: Module Side

```yaml
# In each module repo: .github/workflows/titanops-integration.yml
name: TitanOps Integration Notify

on:
  push:
    branches: [main]
    paths:
      - '.titanops/manifest.yaml'
      - 'eval/**'

jobs:
  notify:
    runs-on: ubuntu-latest
    steps:
      - name: Notify TitanOps
        uses: peter-evans/repository-dispatch@v2
        with:
          token: ${{ secrets.TITANOPS_DISPATCH_TOKEN }}
          repository: mercadoalex/titanops
          event-type: module-updated
          client-payload: |
            {
              "module": "${{ github.repository }}",
              "ref": "${{ github.sha }}",
              "version": "..." 
            }
```

### 5. GitHub Actions: TitanOps Side

```yaml
# In titanops repo: .github/workflows/integration-check.yml
name: Integration Contract Check

on:
  repository_dispatch:
    types: [module-updated]
  schedule:
    - cron: '0 6 * * *'  # Daily at 6am UTC
  workflow_dispatch:

jobs:
  validate:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - name: Fetch module manifests
        run: go run ./integration/validator --registry integration/registry.yaml
      - name: Validate contracts
        run: go run ./integration/validator --check-all
```

### 6. What Gets Validated (and by which tool)

| Check | Tool | What breaks the build |
|-------|------|----------------------|
| Event schema compatibility | **Buf BSR** | Module uses event field removed in newer proto version |
| Breaking proto changes | **Buf BSR** | PR to proto repo introduces backward-incompatible change |
| Chart version compatibility | **Helm** | Umbrella chart can't resolve dependency version range |
| Mock mode compliance | **Manifest validator** | `compliance.mock_mode: false` when architecture requires it |
| Eval coverage | **Manifest validator** | `compliance.eval_scenarios < 3` for modules with scoring logic |
| Health endpoints | **Manifest validator** | Module doesn't declare /healthz, /readyz, /metrics |
| Layer separation | **Manifest validator** | `compliance.layer_separation: false` |
| Cross-module integration | **Periodic kind build** | Modules can't communicate via NATS in assembled cluster |

### 7. Benefits

- **No coupling between module repos** — modules don't import each other
- **Async communication** — modules update at their own pace; TitanOps validates periodically
- **Contract-first** — breaking changes are caught before they reach production
- **Self-service** — module teams update their manifest; no manual coordination with platform team
- **Audit trail** — manifest changes are git-tracked in each module repo

### 8. Migration Path

1. Add `.titanops/manifest.yaml` to each external module repo (start with Quack)
2. Create `integration/registry.yaml` in TitanOps
3. Write the validator tool (`integration/validator/main.go`)
4. Add the GitHub Actions workflows
5. In-repo modules (earthworm, ollinai) get manifests too for consistency

### 9. Alternatives Considered

**Git Submodules** — Would pin exact module commits in TitanOps. Rejected because:
- Submodules create merge friction (every module update requires a titanops PR)
- Modules should be independently deployable — submodule pinning implies lockstep releases
- The manifest approach is async and doesn't require TitanOps to "contain" module code

**Fully custom validator (no Buf/Helm)** — Our initial proposal had a single custom tool doing all contract validation. Rejected because:
- Reinvents schema evolution detection that Buf already handles
- Reinvents version resolution that Helm already handles
- Custom tooling has ongoing maintenance cost with no community support
- Professional projects (Istio, Backstage, CNCF) lean on existing registries

**Monorepo** — Move all modules into TitanOps. Rejected because:
- Modules were designed to work independently (Quack predates TitanOps)
- Different teams may own different modules with different release cadences
- Monorepo tooling (Bazel, Nx) adds complexity disproportionate to team size

---

### 10. Migration Path

1. Set up Buf BSR module for `proto/events.proto` (or use `buf.build` public registry)
2. Add `buf.yaml` + `buf.gen.yaml` to each module repo referencing the schema
3. Each module publishes Helm chart to OCI registry on release (GitHub Actions)
4. Add `.titanops/manifest.yaml` to each external module repo (compliance tracking)
5. Create `integration/registry.yaml` in TitanOps listing all modules
6. Write the lightweight manifest validator (only checks compliance fields)
7. Add periodic integration CI that deploys umbrella chart to `kind` and runs smoke tests
8. In-repo modules (earthworm, ollinai) get manifests too for consistency
