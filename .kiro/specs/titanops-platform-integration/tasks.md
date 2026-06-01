# Implementation Plan: TitanOps Platform Integration

## Overview

This plan implements the TitanOps Platform Integration layer in dependency order: shared Go libraries first, then protobuf event schema, correlation engine, API gateway, React dashboard, Helm umbrella chart, Grafana dashboards, and finally the Earthworm AI integration. Each task builds incrementally on previous steps, ensuring no orphaned code.

## Tasks

- [ ] 1. Set up project structure and Go module scaffolding
  - [ ] 1.1 Create directory structure and initialize Go modules
    - Create `shared/titanops-ai/`, `shared/titanops-k8s/`, `shared/titanops-export/`, `shared/titanops-config/` directories
    - Initialize each as an independent Go module with path `github.com/mercadoalex/titanops/shared/titanops-{ai,k8s,export,config}`
    - Create `correlation/`, `gateway/`, `modules/earthworm/` directories with Go modules
    - Add `go.work` workspace file for local development
    - _Requirements: 9.8, 9.5_

  - [ ] 1.2 Define core interfaces and types for shared libraries
    - Create `shared/titanops-ai/provider.go` with Provider interface, PredictRequest/Response, TrainRequest/Response, ExplainRequest/Response types
    - Create `shared/titanops-k8s/client.go` with Client interface
    - Create `shared/titanops-export/exporter.go` with Exporter interface, Config types, BufferInfo, ExportResult
    - Create `shared/titanops-config/loader.go` with Load function signature, ValidationError type, Option types
    - _Requirements: 9.1, 9.2, 9.3, 9.4_

  - [ ] 1.3 Set up testing framework with rapid library
    - Add `pgregory.net/rapid` dependency to all Go modules
    - Create test helper utilities for common generators (random events, random configs, random feature vectors)
    - Create `testutil/generators.go` with shared test data generators
    - _Requirements: 9.5_

- [ ] 2. Implement titanops-config shared library
  - [ ] 2.1 Implement configuration loader with environment and file sources
    - Implement `Load()` function that reads from environment variables (with configurable prefix) and file sources
    - Implement struct tag-based validation (range, required, enum constraints)
    - Implement merge logic: env vars override file values, file values override defaults
    - Return `[]ValidationError` for any constraint violations with field name, value, and message
    - _Requirements: 9.4, 14 (Property 14)_

  - [ ] 2.2 Implement port and field validation logic
    - Implement port range validation [1024, 65535]
    - Implement required field validation
    - Implement enum field validation
    - Implement numeric range validation
    - Return typed errors without panicking
    - _Requirements: 1.2, 9.7_

  - [ ]* 2.3 Write property test for port configuration validation (Property 1)
    - **Property 1: Port configuration accepts valid range and rejects invalid**
    - Generate random integers across full int range, verify acceptance iff in [1024, 65535]
    - **Validates: Requirements 1.2**

  - [ ]* 2.4 Write property test for configuration loading (Property 14)
    - **Property 14: Configuration loading produces valid config or field-level errors**
    - Generate random config inputs (valid and invalid fields), verify either valid struct or field-level errors returned
    - **Validates: Requirements 9.4**

  - [ ]* 2.5 Write property test for no-panic error handling (Property 15)
    - **Property 15: Shared library operations return typed errors without panicking**
    - Generate random failure scenarios (missing files, invalid inputs), verify typed error returned without panic
    - **Validates: Requirements 9.7**

- [ ] 3. Implement titanops-ai shared library
  - [ ] 3.1 Implement LocalProvider with ONNX model loading and inference
    - Implement `NewLocalProvider(modelDir string)` that scans for `{moduleID}-anomaly.onnx` files
    - Implement `Predict()` using ONNX Runtime Go bindings for local inference
    - Return typed `ModelUnavailable` error if model file missing, `ModelLoadFailed` if corrupt
    - Ensure zero outbound network requests during predict operations
    - _Requirements: 6.1, 6.5, 6.6, 6.7_

  - [ ] 3.2 Implement CloudProvider with fallback to local
    - Implement `CloudProvider` struct wrapping `LocalProvider` and `CloudBackend`
    - Implement 5-second timeout for cloud operations
    - Implement automatic fallback to local ONNX on cloud failure with warning log
    - Route all `Predict()` calls exclusively through local provider regardless of config
    - _Requirements: 6.2, 6.3, 6.4, 6.5_

  - [ ] 3.3 Implement cloud backend interface adapters
    - Define `CloudBackend` interface with `Train()` and `Explain()` methods
    - Create stub adapters for Gemini, Bedrock, Vertex AI, SageMaker
    - Each adapter implements timeout handling and connection error detection
    - _Requirements: 6.2, 6.4_

  - [ ]* 3.4 Write property test for local-only predict (Property 9)
    - **Property 9: Predict operations always use local ONNX without network calls**
    - Generate random feature vectors and provider configs, verify no network calls during predict
    - **Validates: Requirements 6.1, 6.5, 6.6**

  - [ ]* 3.5 Write property test for cloud fallback (Property 10)
    - **Property 10: Cloud AI fallback to local on failure**
    - Generate random requests with simulated cloud failures, verify fallback to local with warning log
    - **Validates: Requirements 6.3**

  - [ ]* 3.6 Write property test for missing model error (Property 11)
    - **Property 11: Missing ONNX model returns typed error**
    - Generate random module IDs with missing model files, verify typed error returned without panic
    - **Validates: Requirements 6.7**

- [ ] 4. Implement titanops-k8s shared library
  - [ ] 4.1 Implement Kubernetes client wrapper
    - Implement `ReadSecret()` using client-go to retrieve secrets by namespace/name/key
    - Implement `ListPods()` with label selector filtering
    - Implement `DeletePod()` and `RestartPod()` (delete to trigger controller restart)
    - Implement `CordonNode()` to mark node unschedulable
    - Return typed errors for API unreachable, not found, permission denied scenarios
    - _Requirements: 9.2, 9.7_

  - [ ]* 4.2 Write unit tests for titanops-k8s client
    - Test with fake client-go clientset for secret reading, pod listing, pod deletion
    - Test error conditions: API unreachable, resource not found, permission denied
    - _Requirements: 9.2, 9.7_

- [ ] 5. Implement titanops-export shared library
  - [ ] 5.1 Implement export adapter with concurrent multi-backend dispatch
    - Implement `Exporter` interface with concurrent fan-out to all enabled backends
    - Use goroutines with independent error handling per backend
    - Ensure failure in one backend does not block or delay others
    - Implement `BufferStatus()` returning per-backend buffer utilization
    - _Requirements: 4.1, 4.2_

  - [ ] 5.2 Implement per-backend formatters (Prometheus, OTLP, Splunk HEC, Dynatrace, Webhook)
    - Implement Prometheus exposition format writer
    - Implement OTLP protobuf serializer
    - Implement Splunk HEC JSON formatter
    - Implement Dynatrace API JSON formatter
    - Implement webhook JSON formatter with severity filtering
    - _Requirements: 4.1, 4.3, 4.6, 4.7_

  - [ ] 5.3 Implement buffer management with eviction and exponential backoff retry
    - Implement ring buffer with capacity 1000 per backend
    - Implement oldest-first eviction when buffer full, emit warning with discard count
    - Implement exponential backoff retry: 1s initial, 60s max, 10 max attempts per event
    - Discard event after 10 failed retries, log permanent failure
    - _Requirements: 4.4, 4.5_

  - [ ] 5.4 Implement webhook severity filtering
    - Filter events by configured severity set before dispatch
    - Implement 10-second timeout per webhook request
    - Implement max 3 retry attempts on webhook failure
    - _Requirements: 4.6, 9.3_

  - [ ]* 5.5 Write property test for export format correctness (Property 3)
    - **Property 3: Export adapter produces correctly formatted output per backend**
    - Generate random valid events × backend types, verify output conforms to wire format
    - **Validates: Requirements 4.1, 4.3**

  - [ ]* 5.6 Write property test for concurrent export isolation (Property 4)
    - **Property 4: Concurrent export isolation**
    - Generate random events × random failure patterns, verify non-failing backends complete independently
    - **Validates: Requirements 4.2**

  - [ ]* 5.7 Write property test for buffer behavior (Property 5)
    - **Property 5: Export buffer growth, eviction, and retry**
    - Generate random event sequences (0-2000 length), verify buffer caps at 1000, oldest evicted, backoff correct
    - **Validates: Requirements 4.4, 4.5**

  - [ ]* 5.8 Write property test for webhook severity filtering (Property 6)
    - **Property 6: Webhook severity filtering**
    - Generate random events × random severity filter sets, verify dispatch iff severity in filter set
    - **Validates: Requirements 4.6**

- [ ] 6. Checkpoint - Shared libraries complete
  - Ensure all tests pass, ask the user if questions arise.

- [ ] 7. Implement protobuf event schema
  - [ ] 7.1 Define protobuf schema and generate Go code
    - Create `proto/titanops/events/v1/events.proto` with Event, CorrelatedIncident, AutoAction messages
    - Define Severity and Module enums
    - Configure `buf.yaml` and `buf.gen.yaml` for Go code generation
    - Generate Go bindings with `buf generate`
    - _Requirements: 7.1, 7.2, 7.4_

  - [ ] 7.2 Implement event validation logic
    - Implement required field validation (namespace, timestamp, severity, module, event_type, payload)
    - Implement payload size validation (max 64 KB)
    - Implement timestamp UTC RFC 3339 millisecond precision enforcement
    - Return field-level validation errors identifying missing/invalid fields
    - _Requirements: 7.3, 7.5, 7.6, 7.7, 7.8_

  - [ ]* 7.3 Write property test for event schema validation (Property 12)
    - **Property 12: Event schema validation round-trip and constraints**
    - Generate random events (valid and invalid), random payloads (0-128KB)
    - Verify: round-trip consistency, UTC RFC 3339 timestamps, 64KB limit enforced, missing fields rejected
    - **Validates: Requirements 7.3, 7.5, 7.6, 7.7, 7.8**

- [ ] 8. Implement correlation engine
  - [ ] 8.1 Implement correlation engine core logic
    - Implement event consumption from event bus (gRPC/NATS subscription)
    - Implement time-window matching: group events by shared attributes (node, pod, namespace)
    - Require at least 2 distinct modules for correlation
    - Implement configurable time window (default 120s, range 10-600s)
    - _Requirements: 5.1, 5.2_

  - [ ] 8.2 Implement confidence scoring and narrative generation
    - Implement confidence score calculation (range 0-100) based on signal strength and pattern matching
    - Implement narrative generation including contributing modules, matched attributes, chronological event sequence
    - Implement `CorrelationRule` matching with score weights
    - _Requirements: 5.3, 5.4_

  - [ ] 8.3 Implement auto-action execution with threshold gating
    - Implement configurable confidence threshold (default 80, range 1-100)
    - Execute auto-action (isolate_pod, alert_operator, forensic_report) only when confidence ≥ threshold
    - Record failure reason and alert operator if auto-action fails
    - Emit correlated incidents to export adapters
    - _Requirements: 5.5, 5.6, 5.7_

  - [ ]* 8.4 Write property test for correlation generation (Property 7)
    - **Property 7: Correlation engine generates incidents from matching cross-module events**
    - Generate random event sets (varying modules, attributes, timestamps)
    - Verify: incident generated when ≥2 modules match, confidence in [0,100], narrative contains all modules and attributes
    - **Validates: Requirements 5.2, 5.3, 5.4**

  - [ ]* 8.5 Write property test for auto-action threshold (Property 8)
    - **Property 8: Auto-action executes if and only if confidence exceeds threshold**
    - Generate random confidence scores × random thresholds, verify execution iff score ≥ threshold
    - **Validates: Requirements 5.5**

- [ ] 9. Implement API gateway
  - [ ] 9.1 Implement gateway REST/gRPC endpoints
    - Implement `/api/health` returning ModuleHealth for all modules
    - Implement `/api/actions` returning recent AutonomousAction entries (limit, since params)
    - Implement `/api/correlations` returning CorrelatedIncident timeline
    - Implement `/api/overrides` for approve/reject/pause/resume operations
    - Implement `/api/audit` returning AuditEntry records with filtering
    - Implement `/api/explain/{actionID}` returning explainability details
    - _Requirements: 8.1, 8.2, 8.3, 8.4, 8.5, 8.6_

  - [ ] 9.2 Implement audit trail recording
    - Record all autonomous actions with: timestamp, module, action_type, trigger_event, confidence, reasoning, outcome, operator_id
    - Record all operator overrides in audit trail
    - Implement AuditFilter for querying historical records
    - _Requirements: 8.5, 8.8_

  - [ ] 9.3 Implement operator override controls
    - Implement `ApproveAction()` - approve pending action, record in audit
    - Implement `RejectAction()` - cancel action execution, record in audit
    - Implement `PauseModule()` - prevent autonomous actions for module until resumed
    - Implement `ResumeModule()` - re-enable autonomous actions for module
    - _Requirements: 8.4, 8.8_

  - [ ]* 9.4 Write property test for audit trail completeness (Property 13)
    - **Property 13: Audit trail and explainability completeness**
    - Generate random actions and overrides, verify all required fields present in audit entries
    - Verify explainability response includes confidence [0.0, 1.0] and full reasoning chain
    - **Validates: Requirements 8.5, 8.6**

- [ ] 10. Checkpoint - Core platform services complete
  - Ensure all tests pass, ask the user if questions arise.

- [ ] 11. Implement Earthworm AI integration
  - [ ] 11.1 Implement Earthworm anomaly detection with titanops-ai
    - Integrate Earthworm agent with `titanops-ai` LocalProvider for ONNX model loading
    - Implement heartbeat signal analysis producing confidence score [0.0, 1.0]
    - Implement threshold comparison (configurable 0.1-1.0, default 0.75)
    - Execute remediation (pod_restart, node_cordon, workload_reschedule) when score ≥ threshold
    - Log observation without action when score < threshold
    - _Requirements: 10.1, 10.2, 10.3, 10.6_

  - [ ] 11.2 Implement Earthworm event emission on autonomous action
    - Emit event within 5 seconds of action containing: node ID, confidence score, triggering heartbeat metrics, action type, timestamp
    - Use protobuf Event schema with all required fields populated
    - Publish to shared event bus
    - _Requirements: 10.4_

  - [ ] 11.3 Implement Earthworm model failure fallback to rule-based detection
    - Detect model load failure or inference timeout (>10 seconds)
    - Fall back to rule-based threshold detection using configured static thresholds
    - Log degraded-mode warning with failure reason
    - _Requirements: 10.5_

  - [ ]* 11.4 Write property test for Earthworm threshold decision (Property 16)
    - **Property 16: Earthworm threshold-based remediation decision**
    - Generate random scores × random thresholds, verify: score in [0.0, 1.0], remediation iff score ≥ threshold, log without action otherwise
    - **Validates: Requirements 10.1, 10.2, 10.6**

  - [ ]* 11.5 Write property test for Earthworm event fields (Property 17)
    - **Property 17: Earthworm action event field completeness**
    - Generate random remediation actions, verify emitted event contains node ID, confidence, heartbeat metrics, action type, timestamp
    - **Validates: Requirements 10.4**

  - [ ]* 11.6 Write property test for Earthworm fallback (Property 18)
    - **Property 18: Earthworm model failure triggers rule-based fallback**
    - Simulate model failures (load failure, timeout), verify fallback to rule-based detection with degraded-mode warning
    - **Validates: Requirements 10.5**

- [ ] 12. Implement TitanOps React Dashboard
  - [ ] 12.1 Set up React project and implement module health view
    - Initialize React/TypeScript project with build tooling
    - Implement Module Health view with status indicators (operational, degraded, unavailable)
    - Connect to Gateway `/api/health` endpoint
    - Implement visual indicator update within 5 seconds of state change
    - _Requirements: 8.1, 8.7_

  - [ ] 12.2 Implement actions feed and explainability views
    - Implement Actions Feed showing up to 50 recent actions within last 24 hours
    - Display full reasoning chain (observation → analysis → action) per entry
    - Implement Explainability detail view with confidence score [0.0, 1.0] and alternatives considered
    - Connect to Gateway `/api/actions` and `/api/explain/{actionID}` endpoints
    - _Requirements: 8.2, 8.6_

  - [ ] 12.3 Implement correlation timeline and override controls
    - Implement Correlation Timeline view with events grouped by correlation ID
    - Implement configurable time window (default 60 minutes)
    - Implement Override Controls: approve, reject, pause buttons with confirmation within 3 seconds
    - Connect to Gateway `/api/correlations` and `/api/overrides` endpoints
    - _Requirements: 8.3, 8.4, 8.8_

  - [ ] 12.4 Implement audit trail view
    - Implement Audit Trail view displaying: timestamp, module, action_type, trigger_event, confidence, reasoning, outcome, operator_id
    - Implement filtering and pagination
    - Connect to Gateway `/api/audit` endpoint
    - _Requirements: 8.5_

  - [ ]* 12.5 Write unit tests for Dashboard components
    - Test module health status rendering for all states
    - Test actions feed display and reasoning chain formatting
    - Test override control interactions and confirmation flow
    - _Requirements: 8.1, 8.2, 8.4_

- [ ] 13. Implement Umbrella Helm Chart
  - [ ] 13.1 Create umbrella chart structure with sub-chart dependencies
    - Create `helm/titanops/Chart.yaml` with dependency declarations for all four modules
    - Create `helm/titanops/values.yaml` with global settings and per-module toggles (all enabled by default)
    - Declare compatibility matrix with minimum supported versions per module
    - _Requirements: 2.1, 2.2, 2.6_

  - [ ] 13.2 Implement shared RBAC and ConfigMap templates
    - Create `templates/shared-rbac.yaml` with ServiceAccount and ClusterRole
    - ClusterRole grants union of permissions for enabled modules only
    - Create `templates/shared-configmap.yaml` with cluster name and OTLP endpoint from values
    - Conditionally include resources only when at least one module enabled
    - _Requirements: 2.3, 2.4, 2.5_

  - [ ] 13.3 Implement platform service deployment templates
    - Create `templates/correlation-deployment.yaml` for correlation engine
    - Create `templates/gateway-deployment.yaml` for API gateway
    - Create `templates/event-bus-deployment.yaml` for NATS/event bus
    - Create `templates/dashboard-deployment.yaml` for React dashboard
    - _Requirements: 2.1_

  - [ ] 13.4 Implement module toggle logic and version constraint validation
    - Implement conditional rendering: disabled modules excluded from manifests
    - Implement version constraint checking in Chart.yaml dependencies
    - Fail install with descriptive error if sub-chart version constraint not satisfied
    - _Requirements: 2.5, 2.7, 11.3, 11.4_

  - [ ]* 13.5 Write property test for Helm template rendering (Property 2)
    - **Property 2: Helm template renders only enabled modules with correct RBAC**
    - Generate random boolean combinations for module toggles
    - Verify: only enabled modules in rendered manifests, ClusterRole has exact union of enabled module permissions
    - **Validates: Requirements 2.2, 2.3, 2.5**

- [ ] 14. Implement Grafana Dashboard Pack
  - [ ] 14.1 Create platform overview and correlation dashboards
    - Create `grafana/overview-dashboard.json` with module health indicators, events/sec, active alerts, AI decision counts
    - Create `grafana/correlation-dashboard.json` with cross-module timeline (default 30 min window)
    - Ensure compatibility with Grafana 9.0+
    - Handle disabled modules as "inactive" status (not errors)
    - Display placeholder message for missing Prometheus datasource
    - _Requirements: 3.1, 3.3, 3.4, 3.5, 3.6_

  - [ ] 14.2 Create per-module Grafana dashboards
    - Create Tlapix dashboard: cert inventory, expiry timeline, anomaly count
    - Create Earthworm dashboard: node heartbeat status, anomaly heatmap, prediction timeline
    - Create eBeeControl dashboard: honeytoken map, access events, threat classification
    - Create Quack dashboard: priority decisions, latency impact, model confidence
    - Place in each module's `grafana/` directory
    - _Requirements: 3.2, 3.5_

- [ ] 15. Implement platform versioning
  - [ ] 15.1 Set up semantic versioning infrastructure
    - Configure Go module git tags using format `shared/vMAJOR.MINOR.PATCH`
    - Document versioning policy for shared libraries, Helm charts, and dashboard
    - Implement compatibility matrix file for umbrella chart
    - Document deprecation policy: retain deprecated elements for at least one minor version
    - _Requirements: 11.1, 11.2, 11.3, 11.5, 11.6_

- [ ] 16. Integration wiring and final validation
  - [ ] 16.1 Wire all components together end-to-end
    - Connect modules → event bus → correlation engine → export adapters
    - Connect correlation engine → API gateway → dashboard
    - Wire Earthworm agent with titanops-ai, titanops-k8s, titanops-export
    - Verify one-way dependency direction (modules import shared libs, never reverse)
    - _Requirements: 5.1, 5.6, 9.6, 10.3_

  - [ ]* 16.2 Write integration tests for end-to-end flows
    - Test event flow: module emit → event bus → correlation → export
    - Test Helm chart deployment to kind/k3d cluster
    - Test Grafana dashboard import
    - Test Kubernetes API interactions via titanops-k8s
    - _Requirements: 5.1, 4.1, 3.5, 9.2_

- [ ] 17. Final checkpoint - All tests pass
  - Ensure all tests pass, ask the user if questions arise.

## Notes

- Tasks marked with `*` are optional and can be skipped for faster MVP
- Each task references specific requirements for traceability
- Checkpoints ensure incremental validation at key milestones
- Property tests validate universal correctness properties using the `rapid` Go library with 100+ iterations
- Unit tests validate specific examples and edge cases
- The design specifies Go for platform services, TypeScript/React for dashboard, and protobuf for event schema
- Shared libraries compile independently without cross-dependencies (requirement 9.8)
- All shared library operations return typed errors without panicking (requirement 9.7)

## Task Dependency Graph

```json
{
  "waves": [
    { "id": 0, "tasks": ["1.1"] },
    { "id": 1, "tasks": ["1.2", "1.3"] },
    { "id": 2, "tasks": ["2.1", "2.2", "4.1"] },
    { "id": 3, "tasks": ["2.3", "2.4", "2.5", "3.1", "4.2"] },
    { "id": 4, "tasks": ["3.2", "3.3", "5.1"] },
    { "id": 5, "tasks": ["3.4", "3.5", "3.6", "5.2", "5.3", "5.4"] },
    { "id": 6, "tasks": ["5.5", "5.6", "5.7", "5.8", "7.1"] },
    { "id": 7, "tasks": ["7.2"] },
    { "id": 8, "tasks": ["7.3", "8.1"] },
    { "id": 9, "tasks": ["8.2", "8.3"] },
    { "id": 10, "tasks": ["8.4", "8.5", "9.1"] },
    { "id": 11, "tasks": ["9.2", "9.3", "11.1"] },
    { "id": 12, "tasks": ["9.4", "11.2", "11.3"] },
    { "id": 13, "tasks": ["11.4", "11.5", "11.6", "12.1"] },
    { "id": 14, "tasks": ["12.2", "12.3", "13.1"] },
    { "id": 15, "tasks": ["12.4", "12.5", "13.2", "13.3"] },
    { "id": 16, "tasks": ["13.4", "13.5", "14.1", "14.2"] },
    { "id": 17, "tasks": ["15.1", "16.1"] },
    { "id": 18, "tasks": ["16.2"] }
  ]
}
```
