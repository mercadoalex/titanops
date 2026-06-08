# Implementation Plan: OllinAI Platform Integration

## Overview

This plan implements the OllinAI Change Intelligence integration as a first-class module in TitanOps. The implementation follows an incremental approach: first establishing the Go module skeleton and core types, then building the adapter internals (poller, webhook, emitter), extending the correlation engine, adding gateway and dashboard support, and finally wiring Helm and Grafana artifacts. Each step builds on the previous one so there is no orphaned code.

## Tasks

- [x] 1. Set up OllinAI Go module and core types
  - [x] 1.1 Create module skeleton with go.mod and update go.work
    - Create `modules/ollinai/` directory
    - Create `modules/ollinai/go.mod` with module path `github.com/mercadoalex/titanops/modules/ollinai` and Go 1.22
    - Add dependencies on `github.com/mercadoalex/titanops/shared/titanops-export` and `github.com/mercadoalex/titanops/shared/titanops-config`
    - Update `go.work` to include `./modules/ollinai`
    - _Requirements: 5.1, 6.1_

  - [x] 1.2 Define event types, payload structs, and severity mapping
    - Create `modules/ollinai/events.go` with event type constants (`deployment_risk`, `dora_metrics`, `incident_correlation`, `supply_chain_credential_exfil`, `supply_chain_process_anomaly`, `supply_chain_attestation_failure`)
    - Create `modules/ollinai/payload.go` with `DeploymentRiskPayload`, `DORAMetricsPayload`, `IncidentCorrelationPayload`, `SupplyChainPayload` structs and JSON serialization + truncation logic (max 64KB, preserves required fields, sets `payload_truncated` label)
    - Create `modules/ollinai/severity.go` with `MapRiskToSeverity(score int) string` function implementing thresholds (critical ≥80, high ≥60, medium ≥40, low <40)
    - _Requirements: 1.1, 1.2, 1.3, 1.7, 1.8, 7.1, 7.2, 7.3_

  - [x] 1.3 Define configuration struct and validation
    - Create `modules/ollinai/config.go` with `Config` struct matching the design schema (Endpoint, AuthToken, WebhookPort, WebhookHMACKey, RiskPollInterval, DORAPollInterval, NATSUrl, BufferCapacity, MaxPayloadBytes, MetricsPort)
    - Implement `ValidateConfig(cfg *Config) []error` returning per-field errors with field name, value, and constraint
    - Implement defaults population for optional fields
    - Use titanops-config `Load()` with `WithEnvPrefix("TITANOPS_OLLINAI")` and `WithFile()`
    - _Requirements: 6.1, 6.2, 6.4_

  - [x]* 1.4 Write property test for configuration validation (Property 11)
    - **Property 11: Configuration validation rejects invalid configs**
    - **Validates: Requirements 6.2, 6.4**
    - Use `pgregory.net/rapid` to generate Config structs with fields drawn from both valid and invalid ranges
    - Assert that validation returns errors identifying invalid field names when constraints are violated, and returns no errors for valid configs

- [x] 2. Implement event emitter with ring buffer
  - [x] 2.1 Implement EventEmitter interface and ring buffer
    - Create `modules/ollinai/emitter.go` with `NATSEmitter` struct implementing the `EventEmitter` interface (Emit, Flush, BufferLen)
    - Implement ring buffer with configurable capacity (default 1000), oldest-first eviction
    - Implement exponential backoff retry (1s initial, doubling, capped at 60s, max 10 attempts)
    - Assign UUID v4 to EventID and RFC 3339 UTC timestamp at event creation
    - Populate Node, Pod, Namespace from metadata; set `metadata_incomplete` label when any is empty
    - _Requirements: 1.4, 1.5, 1.6, 1.9, 5.1, 5.3, 5.4, 5.5_

  - [x]* 2.2 Write property test for ring buffer capacity invariant (Property 6)
    - **Property 6: Ring buffer capacity invariant**
    - **Validates: Requirements 1.9, 5.3**
    - Generate sequences of events pushed to buffer (capacity 1000); assert length never exceeds capacity and oldest event is evicted first

  - [x]* 2.3 Write property test for EventID uniqueness (Property 4)
    - **Property 4: EventID uniqueness**
    - **Validates: Requirements 1.6, 5.4**
    - Generate sequences of N events; assert all EventIDs are distinct valid UUID v4 strings and all Timestamps are valid RFC 3339 UTC

  - [x]* 2.4 Write property test for metadata population and incomplete label (Property 3)
    - **Property 3: Event metadata population and incomplete label**
    - **Validates: Requirements 1.4, 1.5**
    - Generate combinations of Node/Pod/Namespace (each either non-empty or empty); assert `metadata_incomplete=true` label present iff at least one is empty

  - [x]* 2.5 Write property test for payload serialization and truncation (Property 5)
    - **Property 5: Payload serialization and truncation**
    - **Validates: Requirements 1.7, 1.8**
    - Generate payload structs of varying sizes; assert serialized JSON ≤ 64KB, and `payload_truncated=true` label set when truncation occurs

- [x] 3. Checkpoint - Core module foundation
  - Ensure all tests pass, ask the user if questions arise.

- [x] 4. Implement poller and webhook receiver
  - [x] 4.1 Implement REST API poller
    - Create `modules/ollinai/poller.go` with `Poller` struct
    - Implement two polling loops: deployment risk (default 30s) and DORA metrics (default 5m)
    - Parse OllinAI API JSON responses into payload structs
    - Build `export.Event` with correct Module, EventType, Severity, Labels, and serialized Payload
    - Call `EventEmitter.Emit()` for each event
    - Handle API errors: log warning, increment `poll_errors_total`, set readyz=503 on auth failure
    - _Requirements: 1.1, 1.2, 1.3, 8.2, 8.4_

  - [x] 4.2 Implement webhook HTTP receiver
    - Create `modules/ollinai/webhook.go` with `WebhookServer` struct
    - Implement HMAC-SHA256 signature verification; return HTTP 401 on invalid signature
    - Parse eBPF supply chain event payloads into `SupplyChainPayload`
    - Map event types to correct EventType and Severity per design (credential_exfil→critical, process_anomaly→high, attestation_failure→high)
    - Populate Node from CI/CD runner node identifier, set Timestamp to UTC detection time
    - Include pipeline_id, step_name, repository in Labels (omit unavailable keys)
    - Call `EventEmitter.Emit()` for each event; retry up to 3 times on failure
    - _Requirements: 7.1, 7.2, 7.3, 7.4, 7.5, 7.7_

  - [x]* 4.3 Write property test for severity mapping (Property 1)
    - **Property 1: Severity mapping from risk score**
    - **Validates: Requirements 1.1**
    - Generate risk scores in [0, 100]; assert severity is critical ≥80, high ≥60, medium ≥40, low <40; Module="ollinai", EventType="deployment_risk"

  - [x]* 4.4 Write property test for DORA metrics payload completeness (Property 2)
    - **Property 2: DORA metrics payload completeness**
    - **Validates: Requirements 1.2**
    - Generate DORA metric values; serialize and deserialize; assert all four keys present with matching values

- [x] 5. Implement adapter orchestrator, health, and metrics
  - [x] 5.1 Implement adapter orchestrator
    - Create `modules/ollinai/adapter.go` with `Adapter` struct coordinating Poller, WebhookServer, EventEmitter, HealthChecker, and Metrics
    - Implement `Run(ctx context.Context) error` that starts all sub-components
    - Implement `Shutdown(ctx context.Context) error` with graceful shutdown sequence: stop webhook → stop poller → flush buffer (30s deadline) → close NATS
    - Implement atomic config hot-reload: watch for ConfigMap changes, re-invoke `Load()`, atomically swap on success, log warning and keep previous config on failure
    - _Requirements: 6.3, 6.4, 8.5, 8.6_

  - [x] 5.2 Implement health and readiness endpoints
    - Create `modules/ollinai/health.go` with `HealthChecker` struct tracking NATS and OllinAI connection state
    - `/healthz` returns HTTP 200 when process is running (response ≤ 500ms)
    - `/readyz` returns HTTP 200 iff both NATS and OllinAI connections are active; HTTP 503 otherwise
    - _Requirements: 8.1, 8.2, 8.3, 8.4_

  - [x]* 5.3 Write property test for readyz connection state (Property 12)
    - **Property 12: Readyz reflects connection state**
    - **Validates: Requirements 8.2, 8.3, 8.4**
    - Generate all combinations of (NATS connected/disconnected, OllinAI connected/disconnected); assert HTTP 200 iff both connected, HTTP 503 otherwise

  - [x] 5.4 Implement Prometheus metrics
    - Create `modules/ollinai/metrics.go` exposing all metrics from the design at `/metrics` endpoint
    - Follow naming convention `titanops_ollinai_<metric>_<unit>`
    - Register gauges (risk_score_current, change_failure_rate_ratio, lead_time_hours, deployment_frequency_per_day, mttr_hours, events_buffered_current), counters (deployments_total, supply_chain_events_total, events_emitted_total, events_dropped_total, poll_errors_total), and histogram (poll_duration_seconds)
    - _Requirements: 5.2_

- [x] 6. Checkpoint - Adapter fully functional
  - Ensure all tests pass, ask the user if questions arise.

- [x] 7. Extend correlation engine for OllinAI
  - [x] 7.1 Add deployment risk bonus to scoring
    - Add `DeploymentRiskBonus(riskScore int) int` function to `correlation/scoring.go`
    - Implement: clamp to [0, 100], divide by 5, yielding [0, 20]
    - Integrate bonus into `CalculateConfidence` when an OllinAI deployment_risk event is present in the group
    - Cap total at 100
    - _Requirements: 2.5_

  - [x] 7.2 Extend narrative generation with deployment metadata
    - Extend `GenerateNarrative` in `correlation/narrative.go` to include service name, commit SHA, and deployer from OllinAI event Labels when present and non-empty
    - Omit metadata fields that are missing or empty (no placeholder text)
    - _Requirements: 2.6, 2.8_

  - [x] 7.3 Add OllinAI correlation matching rules
    - Ensure correlation engine matches OllinAI events (deployment_risk, supply_chain_*) with other module events using shared attributes (Node, Pod, Namespace) within the time window
    - OllinAI events that have no cross-module match within the window are discarded without generating a CorrelatedIncident
    - Supply chain events correlated with deployment_risk events in same Namespace increase confidence per standard formula
    - _Requirements: 2.1, 2.2, 2.3, 2.4, 2.7, 7.6_

  - [x]* 7.4 Write property test for deployment risk bonus scoring (Property 8)
    - **Property 8: Deployment risk bonus scoring**
    - **Validates: Requirements 2.5**
    - Generate risk score R in [0, 100] and base confidence B in [0, 100]; assert bonus = floor(R/5) and total = min(B + floor(R/5), 100)

  - [x]* 7.5 Write property test for cross-module correlation matching (Property 7)
    - **Property 7: Cross-module correlation matching**
    - **Validates: Requirements 2.1, 2.2, 2.3, 2.4, 7.6**
    - Generate pairs of OllinAI + other-module events with varying attributes and timestamps; assert CorrelatedIncident generated iff shared attribute exists AND both within time window

  - [x]* 7.6 Write property test for narrative metadata inclusion (Property 9)
    - **Property 9: Narrative metadata inclusion**
    - **Validates: Requirements 2.6, 2.8**
    - Generate OllinAI events with combinations of present/empty service, commit_sha, deployer Labels; assert narrative contains non-empty values and omits empty ones

  - [x]* 7.7 Write property test for single-module non-correlation (Property 10)
    - **Property 10: Single-module non-correlation**
    - **Validates: Requirements 2.7**
    - Generate scenarios where only OllinAI events exist in the window with no matching other-module events; assert no CorrelatedIncident generated

- [x] 8. Checkpoint - Correlation engine extended
  - Ensure all tests pass, ask the user if questions arise.

- [x] 9. Extend gateway API
  - [x] 9.1 Add OllinAI endpoint to gateway
    - Add `OllinAIStore` to `gateway/stores.go` holding recent deployments and DORA metrics from NATS subscription
    - Add `OllinAIResponse` struct to `gateway/types.go` with `RecentDeployments` and `DORAMetrics` fields
    - Add `handleOllinAI` handler to `gateway/handler.go` returning JSON response
    - Register `GET /api/ollinai` route in `gateway/gateway.go` `RegisterRoutes`
    - _Requirements: 4.3, 4.7_

- [x] 10. Implement dashboard components
  - [x] 10.1 Create OllinAIPanel React component
    - Create `dashboard/src/components/OllinAIPanel.tsx` displaying:
      - 10 most recent deployment risk scores with critical severity visual distinction (score ≥ 80)
      - 10 most recent deployments
      - Current DORA metrics (deployment frequency, lead time, change failure rate, MTTR)
    - Implement 30-second auto-refresh polling via `setInterval` + `fetch`
    - Display error message when OllinAI data is unavailable; retry on next cycle
    - _Requirements: 4.1, 4.3, 4.6, 4.7, 4.8_

  - [x] 10.2 Add OllinAI types and API client method
    - Add `DeploymentRiskEntry` and `DORAMetrics` TypeScript interfaces to `dashboard/src/types/index.ts`
    - Add `fetchOllinAI()` method to `dashboard/src/api/client.ts`
    - _Requirements: 4.3_

  - [x] 10.3 Integrate OllinAI into existing dashboard components
    - Add "ollinai" as a selectable module filter in `ActionsFeed.tsx` and `AuditTrail.tsx`
    - Update `CorrelationTimeline.tsx` to display deployment metadata (service, commit SHA, deployer) in narratives for OllinAI incidents
    - Update `ExplainView.tsx` to show deployment details when a correlated incident involves OllinAI
    - Update `ModuleHealth.tsx` to include OllinAI module health status (operational/degraded/unavailable)
    - Wire `OllinAIPanel` into `App.tsx`
    - _Requirements: 4.1, 4.2, 4.4, 4.5_

- [x] 11. Implement Helm chart integration
  - [x] 11.1 Create OllinAI sub-chart
    - Create `helm/charts/ollinai/Chart.yaml` with chart metadata
    - Create `helm/charts/ollinai/values.yaml` with all OllinAI configuration defaults from the design
    - Create `helm/charts/ollinai/templates/deployment.yaml` with Deployment resource, configurable replicas (1–5), resource requests/limits, health/readiness probes pointing to /healthz and /readyz
    - Create `helm/charts/ollinai/templates/service.yaml` for webhook and metrics ports
    - Create `helm/charts/ollinai/templates/rbac.yaml` with ClusterRole rules for get/list/watch on Deployments, ReplicaSets, Pods
    - Use `titanops.componentLabels` helper with component `change-intelligence`
    - _Requirements: 3.2, 3.4, 3.6, 3.7_

  - [x] 11.2 Integrate into umbrella Helm chart
    - Add OllinAI sub-chart dependency in `helm/titanops/Chart.yaml` with condition `ollinai.enabled`
    - Add `ollinai` section to `helm/titanops/values.yaml` with `enabled: false` default
    - Create `helm/titanops/templates/ollinai-deployment.yaml` conditional on `.Values.ollinai.enabled`
    - Add template validation: fail if `ollinai.enabled=true` and `ollinai.endpoint` is empty
    - Create Helm test pod at `helm/titanops/templates/tests/test-ollinai-connection.yaml` verifying NATS and OllinAI API connectivity within 10 seconds
    - _Requirements: 3.1, 3.3, 3.5, 3.8_

- [x] 12. Create Grafana dashboard
  - [x] 12.1 Create OllinAI Grafana dashboard JSON
    - Create `grafana/ollinai-dashboard.json` following the structure of `earthworm-dashboard.json`
    - Datasource input: `DS_PROMETHEUS`, requires Grafana ≥9.0.0
    - Tags: `["titanops", "ollinai"]`, UID: `titanops-ollinai`
    - Include panels: Deployment Risk Score (timeseries with thresholds at 60/80), Deployment Frequency (stat), Lead Time (stat), Change Failure Rate (stat), MTTR (stat), Risk Score Distribution (histogram 0-100), High-Risk Deployments table (score ≥70, last 24h), Supply Chain Events (timeseries by type), Adapter Health (stat)
    - All queries use `titanops_ollinai_` metric prefix
    - _Requirements: 9.1, 9.2, 9.3, 9.4, 9.5_

- [x] 13. Final checkpoint - Ensure all tests pass
  - Ensure all tests pass, ask the user if questions arise.

## Notes

- Tasks marked with `*` are optional and can be skipped for faster MVP
- Each task references specific requirements for traceability
- Checkpoints ensure incremental validation
- Property tests validate universal correctness properties from the design document using `pgregory.net/rapid`
- Unit tests validate specific examples and edge cases
- The Go module uses the existing workspace conventions and shared libraries (titanops-export, titanops-config)
- Dashboard components follow the existing React 18 + TypeScript + Vite 5 patterns

## Task Dependency Graph

```json
{
  "waves": [
    { "id": 0, "tasks": ["1.1"] },
    { "id": 1, "tasks": ["1.2", "1.3"] },
    { "id": 2, "tasks": ["1.4", "2.1"] },
    { "id": 3, "tasks": ["2.2", "2.3", "2.4", "2.5"] },
    { "id": 4, "tasks": ["4.1", "4.2"] },
    { "id": 5, "tasks": ["4.3", "4.4", "5.1", "5.2", "5.4"] },
    { "id": 6, "tasks": ["5.3", "7.1", "7.2", "7.3"] },
    { "id": 7, "tasks": ["7.4", "7.5", "7.6", "7.7"] },
    { "id": 8, "tasks": ["9.1", "10.2"] },
    { "id": 9, "tasks": ["10.1", "10.3"] },
    { "id": 10, "tasks": ["11.1", "12.1"] },
    { "id": 11, "tasks": ["11.2"] }
  ]
}
```
