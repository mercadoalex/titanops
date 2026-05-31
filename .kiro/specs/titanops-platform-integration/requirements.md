# Requirements Document

## Introduction

TitanOps is an autonomous AiOps platform for Kubernetes consisting of four modules (Earthworm, Tlapix, eBeeControl, Quack) that share an architecture pattern of "eBPF (kernel observation) → AI (analysis/decision) → Autonomous Action." This document specifies the requirements for the platform integration layer that unifies these modules into a cohesive product with shared infrastructure, observability integrations, cross-module correlation, and a unified management interface.

## Glossary

- **TitanOps_Platform**: The unified platform layer that orchestrates, integrates, and correlates the four autonomous AiOps modules for Kubernetes
- **Module**: One of the four autonomous AiOps components (Earthworm, Tlapix, eBeeControl, Quack) that operates independently or as part of the platform
- **Umbrella_Chart**: A Helm chart that installs all four modules as toggleable sub-chart dependencies with shared configuration
- **Correlation_Engine**: The platform component that detects related signals across modules within configurable time windows and generates correlated incidents
- **AI_Layer**: The unified AI inference and training abstraction that supports local ONNX models and optional cloud AI providers
- **Export_Adapter**: A configurable component that translates module telemetry into backend-specific formats (Prometheus, OTLP, Splunk HEC, Dynatrace API, webhooks)
- **Dashboard**: The TitanOps React-based command center UI for viewing autonomous decisions, actions, and cross-module correlation
- **Shared_Library**: A Go package in the titanops/ platform repository that provides common functionality to multiple modules
- **Event_Schema**: The standardized protobuf or JSON format used by all modules to emit events with common fields (node, pod, namespace, timestamp, severity, module)
- **ONNX_Model**: A local machine learning model in ONNX format used for inference without cloud dependencies
- **Helm_Values**: The YAML configuration file that controls module enablement, export backends, and AI provider selection

## Requirements

### Requirement 1: Independent Module Installation

**User Story:** As a platform operator, I want to install any single TitanOps module independently via Helm, so that I can adopt modules incrementally without committing to the full platform.

#### Acceptance Criteria

1. WHEN an operator runs `helm install` with a module's chart and no other TitanOps modules are present in the cluster, THE Module SHALL deploy all pods to Ready state and pass readiness probes within 120 seconds without requiring additional configuration or manual intervention
2. THE Module SHALL expose a Prometheus-compatible metrics endpoint at `/metrics` on a configurable port that defaults to 9090 and accepts any valid port number between 1024 and 65535
3. THE Module SHALL include a Grafana dashboard JSON file in the chart's `grafana/` directory that is importable into Grafana 9.x or later and visualizes the metrics exposed by the module's `/metrics` endpoint
4. WHEN no custom values are provided, THE Module SHALL use default `values.yaml` settings that produce a deployment where all pods reach Ready state and the `/metrics` endpoint returns HTTP 200 with at least one module-specific metric
5. IF a module dependency is unavailable during installation, THEN THE Module SHALL fail the Helm install and output an error message that identifies each missing dependency by name and required version
6. THE Module SHALL include a demo video (Quack demo available at https://www.youtube.com/watch?v=UJwHtyW7msY&t=42s) demonstrating installation and core functionality with a duration no longer than three minutes

### Requirement 2: Unified Umbrella Helm Chart

**User Story:** As a platform operator, I want to install the entire TitanOps platform with a single Helm command, so that I can deploy and manage all modules as a coordinated system.

#### Acceptance Criteria

1. WHEN an operator runs `helm install titanops`, THE Umbrella_Chart SHALL install all enabled modules (tlapix, earthworm, ebeecontrol, quack) as sub-chart dependencies
2. THE Umbrella_Chart SHALL support toggling each module via `<module>.enabled` boolean values in Helm_Values, with all four modules enabled by default
3. THE Umbrella_Chart SHALL provision a shared ServiceAccount and ClusterRole that grants the union of permissions required by all currently enabled modules
4. WHEN at least one module is enabled, THE Umbrella_Chart SHALL provision a shared ConfigMap containing cluster name and OTLP endpoint values sourced from Helm_Values
5. WHEN a module is disabled via Helm_Values, THE Umbrella_Chart SHALL exclude that module's resources from the rendered manifests and omit that module's permissions from the shared ClusterRole
6. THE Umbrella_Chart SHALL declare a compatibility matrix in Chart.yaml specifying minimum supported versions for each module sub-chart
7. IF a required sub-chart dependency cannot be resolved at the version specified in the compatibility matrix, THEN THE Umbrella_Chart SHALL fail the install with an error message indicating which module version constraint was not satisfied

### Requirement 3: Grafana Dashboard Pack

**User Story:** As a platform operator, I want pre-built Grafana dashboards for the platform overview and each module, so that I can visualize the health and activity of all TitanOps components immediately after installation.

#### Acceptance Criteria

1. THE TitanOps_Platform SHALL provide an overview dashboard displaying module health as a status indicator (active, degraded, or inactive) for each of the four modules, events per second as a time-series panel, count of active alerts, and cumulative AI decision counts
2. THE TitanOps_Platform SHALL provide a per-module dashboard for each of the four modules containing at minimum three domain-specific panels: Tlapix (cert inventory, expiry timeline, anomaly count), Earthworm (node heartbeat status, anomaly heatmap, prediction timeline), eBeeControl (honeytoken map, access events, threat classification), and Quack (priority decisions, latency impact, model confidence)
3. THE TitanOps_Platform SHALL provide a correlation dashboard displaying cross-module events on a shared timeline with a configurable time window defaulting to 30 minutes
4. WHEN a module is disabled, THE overview dashboard SHALL indicate the module as inactive rather than displaying errors
5. THE TitanOps_Platform SHALL distribute dashboards as importable Grafana JSON files compatible with Grafana 9.0 or later, located in a `grafana/` directory within each module's repository
6. IF a required Prometheus datasource is not configured in Grafana, THEN THE dashboard SHALL display a descriptive placeholder message indicating the missing datasource name rather than rendering empty or broken panels

### Requirement 4: Integration Export Adapters

**User Story:** As a platform operator, I want every module to support exporting telemetry to my existing observability backend, so that TitanOps integrates with my current monitoring stack without requiring backend changes.

#### Acceptance Criteria

1. THE Export_Adapter SHALL support Prometheus exposition format, OTLP, Splunk HEC, Dynatrace API, and webhook destinations
2. WHILE multiple export backends are enabled, THE Export_Adapter SHALL send telemetry to all configured backends concurrently, such that a failure in one backend does not delay or prevent delivery to other backends
3. THE Export_Adapter SHALL be configurable per module via the Helm_Values export section
4. IF an export backend is unreachable, THEN THE Export_Adapter SHALL buffer events locally up to a maximum of 1000 events and retry delivery using exponential backoff starting at 1 second with a maximum interval of 60 seconds, for up to 10 retry attempts per event
5. IF the local buffer reaches its maximum capacity of 1000 events, THEN THE Export_Adapter SHALL discard the oldest buffered events to make room for new events and emit a warning indicating the number of discarded events
6. WHEN a webhook destination is configured, THE Export_Adapter SHALL filter events by the severity levels specified in the webhook configuration before dispatching, where severity levels are one or more of: critical, high, medium, low
7. THE Export_Adapter SHALL use an identical configuration schema structure (same field names, types, and validation rules) across all modules for each backend type

### Requirement 5: Cross-Module Correlation Engine

**User Story:** As a platform operator, I want the platform to automatically detect related signals across modules and generate correlated incidents, so that I can identify complex threats and issues that no single module can detect alone.

#### Acceptance Criteria

1. THE Correlation_Engine SHALL consume events from all enabled modules via a shared event bus
2. WHEN events from at least 2 enabled modules share matching attributes (node, pod, namespace) within a configurable time window (default: 120 seconds, configurable range: 10 to 600 seconds), THE Correlation_Engine SHALL generate a correlated incident
3. THE Correlation_Engine SHALL assign a confidence score between 0 and 100 to each correlated incident based on signal strength and pattern matching
4. THE Correlation_Engine SHALL include a narrative in each generated incident that identifies the contributing modules, the matched attributes, and the temporal sequence of correlated events
5. WHEN a correlated incident exceeds a configurable confidence threshold (default: 80, configurable range: 1 to 100), THE Correlation_Engine SHALL execute the configured auto-action (isolate pod, alert operator, generate forensic report)
6. THE Correlation_Engine SHALL emit correlated incidents to all configured export backends using the same Export_Adapter infrastructure
7. IF an auto-action fails to execute, THEN THE Correlation_Engine SHALL retain the correlated incident, record the failure reason, and alert the operator indicating which auto-action failed and for which incident

### Requirement 6: Unified AI Layer

**User Story:** As a platform operator, I want a single AI configuration that applies to all modules with local-first inference and optional cloud backends, so that the platform works offline by default and can leverage cloud AI when available.

#### Acceptance Criteria

1. THE AI_Layer SHALL perform inference using local ONNX models with zero cloud dependencies as the default configuration
2. THE AI_Layer SHALL support pluggable cloud AI providers including Gemini, Bedrock, Vertex AI, and SageMaker as optional backends for train and explain operations
3. WHEN a cloud AI provider is configured but fails to respond within 5 seconds or returns a connection error, THE AI_Layer SHALL fall back to local ONNX inference and log a warning that includes the provider name and failure reason
4. THE AI_Layer SHALL expose a unified provider interface with train, predict, and explain operations where each operation accepts a module identifier and returns a structured result or an error indication
5. THE AI_Layer SHALL restrict cloud provider usage to offline operations (model training, explanation generation) and SHALL route all predict operations exclusively through local ONNX inference
6. WHILE the AI_Layer is configured with provider set to "local", THE AI_Layer SHALL operate without any outbound network requests for AI operations
7. IF a local ONNX model file is missing or fails to load for a given module, THEN THE AI_Layer SHALL reject predict requests for that module with an error indicating the model is unavailable and SHALL log the module name and file path that failed to load

### Requirement 7: Shared Event Schema

**User Story:** As a platform developer, I want all modules to emit events in a standardized schema, so that the correlation engine and export adapters can process events uniformly regardless of source module.

#### Acceptance Criteria

1. THE Event_Schema SHALL include the following required fields: namespace, timestamp, severity, module, event_type, and payload, and the following optional fields: node and pod
2. THE Event_Schema SHALL use protobuf as the wire format for inter-module communication
3. THE Event_Schema SHALL represent the timestamp field as UTC in RFC 3339 format with millisecond precision
4. THE Event_Schema SHALL define severity levels as: critical, high, medium, low, and informational
5. THE Event_Schema SHALL constrain the payload field to a maximum size of 64 KB per event
6. WHEN a module emits an event, THE Module SHALL populate all required Event_Schema fields before publishing to the event bus
7. IF a module emits an event with missing required fields, THEN THE event bus SHALL reject the event, return a validation error to the emitting module indicating which fields are missing, and log the validation failure
8. IF a module emits an event with a payload exceeding 64 KB, THEN THE event bus SHALL reject the event and return a size-limit error to the emitting module

### Requirement 8: TitanOps Dashboard

**User Story:** As a platform operator, I want a dedicated command center UI that displays autonomous decisions, actions, and cross-module correlations, so that I can understand and override the platform's autonomous behavior.

#### Acceptance Criteria

1. THE Dashboard SHALL display the health status of all four modules (Earthworm, Tlapix, eBeeControl, Quack) where each module reports one of the following states: operational, degraded, or unavailable
2. THE Dashboard SHALL display the most recent autonomous actions, up to 50 entries within the last 24 hours by default, with each entry showing the full reasoning chain (observation → analysis → action)
3. THE Dashboard SHALL display the cross-module correlation timeline showing events grouped by correlation ID across modules within a configurable time window defaulting to the last 60 minutes
4. THE Dashboard SHALL provide human override controls that allow an operator to approve, reject, or pause a pending autonomous action, where rejecting an action cancels its execution and pausing an action prevents further autonomous actions for that module until the operator resumes it
5. THE Dashboard SHALL provide an audit trail of all autonomous actions recording for each entry: timestamp, module name, action type, trigger event, AI confidence score, decision reasoning, outcome status, and operator identity if overridden
6. WHEN an operator queries "why did TitanOps perform action X", THE Dashboard SHALL display the AI confidence score as a value between 0.0 and 1.0 and the decision reasoning including the observation data, analysis summary, and selected action with alternatives considered
7. IF a module's health status changes from operational to degraded or unavailable, THEN THE Dashboard SHALL display a visual indicator on that module's status within 5 seconds of the state change
8. IF an operator submits an override action (approve, reject, or pause), THEN THE Dashboard SHALL display a confirmation of the override result within 3 seconds and record the override in the audit trail

### Requirement 9: Shared Go Libraries

**User Story:** As a platform developer, I want shared Go libraries for common functionality (AI inference, K8s client, eBPF event handling, export, config), so that modules avoid reimplementing the same patterns independently.

#### Acceptance Criteria

1. THE Shared_Library titanops-ai SHALL provide ONNX model loading from a file path, inference execution accepting feature vectors and returning prediction scores, and a cloud backend interface defining train, predict, and explain methods that each cloud provider (Gemini, Bedrock, Vertex, SageMaker) must implement
2. THE Shared_Library titanops-k8s SHALL provide Kubernetes client patterns including secret reading by name and namespace, pod listing by label selector, and pod deletion by name and namespace
3. THE Shared_Library titanops-export SHALL provide Prometheus metrics registration with custom collector support, OTLP export to a configurable endpoint, and webhook dispatch to configured URLs with a maximum timeout of 10 seconds per request and a maximum of 3 retry attempts on failure
4. THE Shared_Library titanops-config SHALL provide configuration loading from Helm_Values environment variables and file sources, merged into a single typed configuration struct, and validation that returns a list of field-level errors for any values that violate defined constraints
5. THE Shared_Library SHALL follow semantic versioning via Go module git tags using the format shared/vMAJOR.MINOR.PATCH
6. THE Shared_Library SHALL maintain a one-way dependency direction where modules import shared libraries but shared libraries never import modules
7. IF a shared library operation fails (model file not found, Kubernetes API unreachable, webhook endpoint unresponsive, or configuration file unparseable), THEN THE Shared_Library SHALL return a typed error value indicating the failure category and cause without panicking or terminating the calling process
8. THE Shared_Library SHALL be importable as Go modules using the path github.com/mercadoalex/titanops/shared/titanops-{ai,k8s,export,config} and each library SHALL compile independently without requiring the other shared libraries as dependencies

### Requirement 10: Earthworm AI-Driven Autonomous Decisions

**User Story:** As a platform operator, I want the Earthworm module to use AI/ML models for anomaly detection and autonomous decision-making, so that cluster health issues are identified and acted upon without manual intervention.

#### Acceptance Criteria

1. THE Earthworm module SHALL use a local ONNX anomaly detection model loaded via the titanops-ai shared library to analyze cluster heartbeat signals and produce a health anomaly confidence score between 0.0 and 1.0 for each monitored node
2. WHEN the Earthworm anomaly model produces a confidence score equal to or above the configured threshold (configurable between 0.1 and 1.0, default 0.75), THE Earthworm module SHALL execute an autonomous remediation action limited to pod restart, node cordon, or workload rescheduling
3. THE Earthworm module SHALL integrate with the unified AI_Layer using the titanops-ai shared library for model loading and inference
4. WHEN the Earthworm module takes an autonomous action, THE Earthworm module SHALL emit an event within 5 seconds containing the node identifier, the anomaly confidence score, the heartbeat metrics that triggered detection, the remediation action taken, and a timestamp
5. IF the Earthworm anomaly model fails to load or does not respond within 10 seconds, THEN THE Earthworm module SHALL fall back to rule-based threshold detection using configured static thresholds and log a degraded-mode warning indicating the failure reason
6. IF the Earthworm module receives a confidence score below the configured threshold, THEN THE Earthworm module SHALL log the anomaly observation without executing any remediation action

### Requirement 11: Platform Versioning and Compatibility

**User Story:** As a platform operator, I want clear semantic versioning across all platform components, so that I can upgrade safely and understand the impact of version changes.

#### Acceptance Criteria

1. THE TitanOps_Platform SHALL version all components (module Helm charts, shared libraries, Umbrella Chart, and Dashboard) using semantic versioning (MAJOR.MINOR.PATCH)
2. WHEN a shared library introduces a breaking API change (removal of a public function or type, rename of a public function or type, change to an existing function signature, or removal of a configuration field), THE Shared_Library SHALL increment the major version number
3. THE Umbrella_Chart SHALL declare minimum compatible versions for each module sub-chart in its dependency specification
4. WHEN a module is upgraded to a new major version, THE Umbrella_Chart SHALL reject the dependency resolution until the compatibility matrix file is updated to reference the new major version
5. THE TitanOps_Platform SHALL publish release notes documenting all breaking changes, new features, and bug fixes no later than the time the version tag is published
6. WHEN a public API element in a shared library is marked as deprecated, THE Shared_Library SHALL retain the deprecated element for at least one minor version release before removing it in a subsequent major version increment
