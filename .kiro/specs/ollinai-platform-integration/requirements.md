# Requirements Document

## Introduction

This document defines the requirements for integrating OllinAI (Change Intelligence & Deployment Risk) as a first-class module into the TitanOps autonomous AiOps platform. OllinAI ingests deployment events from CI/CD pipelines, correlates them with production incidents, computes risk scores, and provides ML-powered recommendations. The integration connects OllinAI's deployment risk signals to TitanOps' cross-module correlation engine via the NATS event bus, adds OllinAI to the umbrella Helm chart, surfaces OllinAI events in the dashboard, and enables OllinAI's Rust eBPF agent data to flow through the platform pipeline.

## Glossary

- **Correlation_Engine**: The TitanOps Go component that matches events from multiple modules within a configurable time window and generates correlated incidents with confidence scores.
- **NATS_Event_Bus**: The in-cluster NATS pub/sub messaging system used by TitanOps modules to emit events to the Correlation_Engine.
- **OllinAI_Adapter**: A Go sidecar or library component that bridges OllinAI's TypeScript/AWS-based outputs into TitanOps-compatible export.Event messages on the NATS_Event_Bus.
- **Deployment_Risk_Event**: An export.Event emitted by the OllinAI_Adapter when OllinAI computes a deployment risk score, containing risk level, affected services, and CI/CD metadata.
- **DORA_Metrics_Event**: An export.Event emitted by the OllinAI_Adapter when OllinAI computes updated DORA metrics (deployment frequency, lead time, change failure rate, MTTR).
- **Incident_Correlation_Event**: An export.Event emitted by the OllinAI_Adapter when OllinAI detects a correlation between a deployment and a production incident.
- **eBPF_Supply_Chain_Event**: An export.Event emitted by OllinAI's Rust eBPF agent when it detects supply chain security anomalies (credential exfiltration, unauthorized process ancestry, attestation failures).
- **Umbrella_Helm_Chart**: The TitanOps Helm chart at `helm/titanops/` that installs all platform modules as toggleable sub-chart dependencies.
- **titanops-export**: The shared Go library providing the Event struct, Exporter interface, and multi-backend fan-out for Prometheus, OTLP, Splunk, Dynatrace, and webhooks.
- **titanops-config**: The shared Go library providing unified configuration loading, struct validation, and hot reload capabilities.
- **Dashboard**: The TitanOps React command center UI that displays module health, correlation timelines, autonomous actions, and explainability views.
- **Risk_Score**: A numeric value (0-100) computed by OllinAI's ML engine representing the probability of a deployment causing an incident.

## Requirements

### Requirement 1: OllinAI Event Emission to NATS Event Bus

**User Story:** As a platform operator, I want OllinAI's deployment risk events published to the TitanOps NATS event bus, so that the Correlation_Engine can match deployment signals with other module events.

#### Acceptance Criteria

1. WHEN OllinAI computes a deployment risk score, THE OllinAI_Adapter SHALL publish a Deployment_Risk_Event to the NATS_Event_Bus with Module field set to "ollinai", EventType set to "deployment_risk", and Severity mapped from the Risk_Score (critical ≥ 80, high ≥ 60, medium ≥ 40, low < 40).
2. WHEN OllinAI computes updated DORA metrics, THE OllinAI_Adapter SHALL publish a DORA_Metrics_Event to the NATS_Event_Bus with Module field set to "ollinai", EventType set to "dora_metrics", and a JSON Payload containing at minimum deployment_frequency, lead_time_for_changes, change_failure_rate, and time_to_restore_service values.
3. WHEN OllinAI detects a deployment-incident correlation, THE OllinAI_Adapter SHALL publish an Incident_Correlation_Event to the NATS_Event_Bus with Module field set to "ollinai" and EventType set to "incident_correlation".
4. THE OllinAI_Adapter SHALL populate the Node, Pod, and Namespace fields of each export.Event using deployment target metadata from OllinAI's CI/CD pipeline data.
5. IF deployment target metadata from OllinAI's CI/CD pipeline data does not contain a value for the Node, Pod, or Namespace field, THEN THE OllinAI_Adapter SHALL set the missing field to an empty string and include a label with key "metadata_incomplete" and value "true" on the emitted event.
6. THE OllinAI_Adapter SHALL assign a unique UUID v4 to the EventID field and set the Timestamp field to the event occurrence time in UTC (RFC 3339 with millisecond precision) for each emitted event.
7. THE OllinAI_Adapter SHALL serialize the OllinAI-specific payload (risk factors, DORA values, incident details) into the Payload field as JSON with a maximum size of 64 KB.
8. IF the serialized JSON Payload exceeds 64 KB, THEN THE OllinAI_Adapter SHALL truncate the payload to fit within 64 KB by removing optional detail fields while preserving required summary fields, and include a label with key "payload_truncated" and value "true" on the emitted event.
9. IF the NATS_Event_Bus is unavailable, THEN THE OllinAI_Adapter SHALL buffer events in a ring buffer with a capacity of 1000 events using oldest-first eviction when full, and retry delivery with exponential backoff starting at 1 second, doubling per attempt, capped at 60 seconds, for a maximum of 10 retry attempts per event.

### Requirement 2: Cross-Module Correlation Enrichment

**User Story:** As a platform operator, I want deployments correlated with other module signals (heartbeat degradation, security anomalies, performance drops), so that I can identify deployment-caused incidents automatically.

#### Acceptance Criteria

1. WHEN the Correlation_Engine receives a Deployment_Risk_Event and an Earthworm heartbeat degradation event sharing the same Node or Namespace within the configured correlation time window (default 120 seconds, configurable between 10 seconds and 600 seconds), THE Correlation_Engine SHALL generate a CorrelatedIncident linking the deployment to the health degradation with a confidence score between 0 and 100.
2. WHEN the Correlation_Engine receives a Deployment_Risk_Event and a Tlapix certificate anomaly event sharing the same Namespace within the configured correlation time window, THE Correlation_Engine SHALL generate a CorrelatedIncident linking the deployment to the certificate issue with a confidence score between 0 and 100.
3. WHEN the Correlation_Engine receives a Deployment_Risk_Event and an eBeeControl threat event sharing the same Pod or Namespace within the configured correlation time window, THE Correlation_Engine SHALL generate a CorrelatedIncident linking the deployment to the security threat with a confidence score between 0 and 100.
4. WHEN the Correlation_Engine receives a Deployment_Risk_Event and a Quack scheduling anomaly event sharing the same Node within the configured correlation time window, THE Correlation_Engine SHALL generate a CorrelatedIncident linking the deployment to the performance issue with a confidence score between 0 and 100.
5. WHEN the Correlation_Engine calculates the confidence score for a CorrelatedIncident that includes an OllinAI Deployment_Risk_Event, THE Correlation_Engine SHALL add the deployment risk score (normalized to 0–20 range) from the OllinAI event as an additive component in the confidence score calculation, capped at a maximum total of 100.
6. WHEN the Correlation_Engine generates a CorrelatedIncident narrative for an incident that includes an OllinAI Deployment_Risk_Event, THE Correlation_Engine SHALL include the service name, commit SHA, and deployer identity from the deployment event Labels in the CorrelatedIncident narrative.
7. IF the Correlation_Engine receives a Deployment_Risk_Event and no matching event from any other module arrives within the configured correlation time window, THEN THE Correlation_Engine SHALL discard the Deployment_Risk_Event from the active correlation window without generating a CorrelatedIncident.
8. IF the Correlation_Engine receives a Deployment_Risk_Event with missing or empty service name, commit SHA, or deployer Labels, THEN THE Correlation_Engine SHALL still generate the CorrelatedIncident but omit the missing metadata fields from the narrative.

### Requirement 3: Umbrella Helm Chart Integration

**User Story:** As a cluster administrator, I want to install OllinAI alongside other TitanOps modules using the umbrella Helm chart, so that deployment is consistent and toggleable.

#### Acceptance Criteria

1. THE Umbrella_Helm_Chart SHALL include an OllinAI sub-chart dependency in Chart.yaml with a condition toggle `ollinai.enabled`, defaulting to `false`.
2. WHEN `ollinai.enabled` is set to true, THE Umbrella_Helm_Chart SHALL deploy the OllinAI_Adapter as a Deployment resource with a configurable replica count (default: 1, range: 1–5), resource requests (default: cpu 100m, memory 128Mi), and resource limits (default: cpu 500m, memory 256Mi).
3. WHEN `ollinai.enabled` is set to false, THE Umbrella_Helm_Chart SHALL not render or deploy any OllinAI-related Kubernetes resources.
4. THE Umbrella_Helm_Chart SHALL expose OllinAI configuration under the `ollinai` key in values.yaml, including `ollinai.endpoint` (OllinAI external API URL) and `ollinai.authToken` (authentication token reference).
5. IF `ollinai.enabled` is true and `ollinai.endpoint` is empty or not set, THEN THE Umbrella_Helm_Chart SHALL fail template rendering with an error message indicating that the OllinAI endpoint URL is required.
6. THE Umbrella_Helm_Chart SHALL label all OllinAI resources using the existing `titanops.componentLabels` helper with component set to `change-intelligence`, producing standard labels including `app.kubernetes.io/part-of: titanops` and `app.kubernetes.io/component: change-intelligence`.
7. WHEN `ollinai.enabled` is true, THE Umbrella_Helm_Chart SHALL add RBAC rules to the shared ClusterRole granting get, list, and watch permissions on Deployments, ReplicaSets, and Pods in the release namespace.
8. THE Umbrella_Helm_Chart SHALL include a Helm test pod that verifies the OllinAI_Adapter can establish a connection to both the NATS_Event_Bus and the OllinAI external API endpoint, passing when both connections respond within 10 seconds and failing otherwise.

### Requirement 4: Dashboard Integration

**User Story:** As a platform operator, I want OllinAI deployment risk events and DORA metrics visible in the TitanOps dashboard, so that I can see change intelligence alongside other module data.

#### Acceptance Criteria

1. THE Dashboard SHALL display OllinAI module health status on the ModuleHealth panel with status values of operational, degraded, or unavailable, following the same polling interval and display format as other modules.
2. WHEN the Correlation_Engine generates a CorrelatedIncident involving OllinAI events, THE Dashboard SHALL display the incident on the CorrelationTimeline with the narrative including deployment metadata (service name, commit SHA, and deployer).
3. THE Dashboard SHALL provide a dedicated OllinAI panel showing the 10 most recent deployment risk scores, the 10 most recent deployments, and the current DORA metric values (deployment frequency, lead time for changes, change failure rate, and mean time to recovery).
4. WHEN a user selects a correlated incident involving a deployment, THE Dashboard SHALL display the deployment details (service, commit, deployer, risk score, risk factors) in the ExplainView.
5. THE Dashboard SHALL include "ollinai" as a selectable module filter option on the ActionsFeed and AuditTrail components, filtering displayed entries to only those with module value "ollinai" when selected.
6. IF the OllinAI data source is unavailable when the dedicated OllinAI panel loads, THEN THE Dashboard SHALL display an error message indicating that OllinAI data is unavailable and retry fetching on the next polling cycle.
7. THE Dashboard SHALL refresh the dedicated OllinAI panel data every 30 seconds without requiring a manual page reload.
8. WHEN a deployment risk score displayed on the OllinAI panel is 80 or above, THE Dashboard SHALL visually distinguish the entry as critical severity.

### Requirement 5: Shared Library Usage (titanops-export)

**User Story:** As a platform developer, I want OllinAI to use the titanops-export library for telemetry export, so that metrics and events flow through the same pipeline as other modules.

#### Acceptance Criteria

1. THE OllinAI_Adapter SHALL use the titanops-export Exporter interface to export OllinAI events to all configured backends (Prometheus, OTLP, Splunk, Dynatrace, webhooks) concurrently, so that failure in one backend does not block or delay delivery to other backends.
2. THE OllinAI_Adapter SHALL expose Prometheus metrics at a `/metrics` endpoint following the naming convention `titanops_ollinai_<metric>_<unit>` (e.g., `titanops_ollinai_deployment_risk_score_current`, `titanops_ollinai_deployments_total`, `titanops_ollinai_change_failure_rate_ratio`).
3. THE OllinAI_Adapter SHALL use the titanops-export ring buffer with capacity 1000 per backend for buffering events before export, evicting the oldest event first when the buffer is full and retrying failed deliveries with exponential backoff (initial delay 1 s, maximum delay 60 s, maximum 10 attempts per event).
4. THE OllinAI_Adapter SHALL assign a UUID v4 to the EventID field of each event at creation time, so that export backends can deduplicate retried events by EventID.
5. IF an event exceeds 10 retry attempts for a backend, THEN THE OllinAI_Adapter SHALL discard the event from that backend's buffer and emit a warning indicating the backend name, EventID, and number of failed attempts.

### Requirement 6: Shared Library Usage (titanops-config)

**User Story:** As a platform developer, I want OllinAI to use the titanops-config library for configuration, so that it supports the same hot-reload and validation patterns as other modules.

#### Acceptance Criteria

1. THE OllinAI_Adapter SHALL load configuration using the titanops-config `Load()` function with `WithEnvPrefix("TITANOPS_OLLINAI")` for environment variables and `WithFile()` for the ConfigMap-mounted file source, where environment variables take precedence over file values.
2. THE OllinAI_Adapter SHALL validate all configuration at startup using titanops-config struct tag validation and, if any `ValidationError` is returned, SHALL log each error (including field name, rejected value, and violated constraint) and exit with a non-zero status code.
3. WHEN the ConfigMap is updated, THE OllinAI_Adapter SHALL reload configuration by re-invoking the titanops-config `Load()` function and atomically swap the active configuration within 5 seconds of the file change, without interrupting in-flight event processing.
4. IF the OllinAI_Adapter receives an invalid configuration update during hot-reload, THEN THE OllinAI_Adapter SHALL reject the update, log a warning identifying the invalid fields and constraint violations, and continue operating with the previous valid configuration.

### Requirement 7: eBPF Agent Data Flow

**User Story:** As a security engineer, I want OllinAI's Rust eBPF agent data (supply chain security signals) to flow into the TitanOps platform, so that CI/CD pipeline security events participate in cross-module correlation.

#### Acceptance Criteria

1. WHEN OllinAI's eBPF agent detects a credential exfiltration attempt during a build, THE OllinAI_Adapter SHALL publish an eBPF_Supply_Chain_Event to the NATS_Event_Bus with EventType set to "supply_chain_credential_exfil", Severity set to "critical", and Module set to "ollinai".
2. WHEN OllinAI's eBPF agent detects unauthorized process ancestry in a CI/CD runner, THE OllinAI_Adapter SHALL publish an eBPF_Supply_Chain_Event to the NATS_Event_Bus with EventType set to "supply_chain_process_anomaly", Severity set to "high", and Module set to "ollinai".
3. WHEN OllinAI's eBPF agent detects a build attestation failure, THE OllinAI_Adapter SHALL publish an eBPF_Supply_Chain_Event to the NATS_Event_Bus with EventType set to "supply_chain_attestation_failure", Severity set to "high", and Module set to "ollinai".
4. THE OllinAI_Adapter SHALL populate the Node field of eBPF_Supply_Chain_Events with the CI/CD runner node identifier and set the Timestamp field to the UTC time at which the eBPF agent detected the event.
5. THE OllinAI_Adapter SHALL include build metadata in the Labels field of eBPF_Supply_Chain_Events with keys "pipeline_id", "step_name", and "repository", omitting any key whose value is unavailable from the agent payload.
6. THE Correlation_Engine SHALL correlate eBPF_Supply_Chain_Events with Deployment_Risk_Events when they share the same Namespace and occur within the configured time window (default 120 seconds, configurable from 10 to 600 seconds), increasing the CorrelatedIncident confidence score according to the standard scoring formula.
7. IF the OllinAI_Adapter fails to publish an eBPF_Supply_Chain_Event to the NATS_Event_Bus, THEN THE OllinAI_Adapter SHALL retry publication up to 3 times with exponential backoff and, if all retries fail, SHALL log the failure and drop the event without blocking subsequent event processing.

### Requirement 8: Health and Readiness Endpoints

**User Story:** As a cluster administrator, I want the OllinAI_Adapter to expose standard health endpoints, so that Kubernetes can manage its lifecycle correctly.

#### Acceptance Criteria

1. THE OllinAI_Adapter SHALL expose a `/healthz` endpoint that returns HTTP 200 when the process is running and able to accept HTTP requests, with a response time no greater than 500 milliseconds.
2. THE OllinAI_Adapter SHALL expose a `/readyz` endpoint that returns HTTP 200 only when the adapter holds an active connection to both the NATS_Event_Bus and the OllinAI external API, where "active" means the connection has completed an initial handshake and no unrecovered transport error is pending.
3. IF the connection to the NATS_Event_Bus is lost, THEN THE OllinAI_Adapter SHALL return HTTP 503 from the `/readyz` endpoint until the connection is re-established.
4. IF the connection to the OllinAI external API is lost, THEN THE OllinAI_Adapter SHALL return HTTP 503 from the `/readyz` endpoint and continue buffering events locally up to a maximum of 1000 events, evicting the oldest events first when the buffer is full.
5. WHEN a termination signal (SIGTERM or SIGINT) is received, THE OllinAI_Adapter SHALL flush buffered events to the NATS_Event_Bus and complete shutdown within 30 seconds.
6. IF the flush of buffered events cannot complete within the 30-second shutdown deadline, THEN THE OllinAI_Adapter SHALL terminate and discard any remaining unflushed events.

### Requirement 9: Grafana Dashboard

**User Story:** As a platform operator, I want a Grafana dashboard for OllinAI metrics, so that I can visualize deployment risk trends and DORA metrics in my existing Grafana instance.

#### Acceptance Criteria

1. THE OllinAI_Adapter SHALL ship a Grafana dashboard JSON file at `grafana/ollinai-dashboard.json` that is importable into Grafana 9.0.0 or later, declares a Prometheus datasource input named `DS_PROMETHEUS`, and includes panels visualizing deployment risk scores over time, DORA metrics trends, and supply chain security events.
2. THE Grafana dashboard SHALL include panels for deployment frequency (deployments per day), lead time for changes (time from commit to deploy in hours), change failure rate (percentage of deployments causing incidents), and mean time to recovery (hours from incident detection to resolution).
3. THE Grafana dashboard SHALL include a panel showing deployment risk score distribution as a histogram with buckets spanning 0 to 100, and a table of deployments with a risk score of 70 or above that occurred within the last 24 hours.
4. THE Grafana dashboard SHALL use the `titanops_ollinai_` metric prefix for all queries, consistent with other TitanOps module dashboards.
5. THE Grafana dashboard SHALL include the tags `titanops` and `ollinai` in its JSON metadata to allow filtering within Grafana's dashboard list.
