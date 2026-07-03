# Requirements Document

## Introduction

BrainOps is the platform intelligence layer for TitanOps — an autonomous AiOps platform for Kubernetes. BrainOps is a LangGraph.js agent (TypeScript) that uses CockroachDB as its persistent memory layer. It subscribes to correlated incidents from the NATS event bus, reasons over historical data, and orchestrates multi-step remediations. Modules remain autonomous (they act locally); BrainOps enhances decisions with historical reasoning and pattern recognition. BrainOps integrates all four CockroachDB hackathon tools: Managed MCP Server, Distributed Vector Indexing, ccloud CLI, and Agent Skills Repo.

## Glossary

- **BrainOps**: The TitanOps platform intelligence agent — a LangGraph.js TypeScript service that uses CockroachDB for persistent memory
- **CockroachDB_Memory**: The CockroachDB Serverless cluster storing agent state (checkpoints, incidents, embeddings, resolutions, deployments, audit)
- **MCP_Server**: Model Context Protocol server exposing structured tools to the agent for querying memory and executing actions
- **Managed_MCP**: CockroachDB Cloud's managed MCP endpoint at cockroachlabs.cloud/mcp for read-only SQL and schema exploration
- **Custom_MCP**: TitanOps-specific MCP tools that compose CockroachDB queries with Kubernetes actions and business logic
- **LangGraph_Checkpointer**: The @langchain/langgraph-checkpoint-postgres adapter that persists agent state to CockroachDB
- **Distributed_Vector_Index**: CockroachDB's C-SPANN vector index for semantic similarity search on incident embeddings
- **Agent_Skills**: Curated machine-executable skills from the cockroachdb/cursor-plugin repo for database self-optimization
- **Resolution_Playbook**: A stored record of what actions fixed a past incident, with success rate and duration
- **NATS_Subscription**: BrainOps subscribes to the NATS event bus for high-confidence correlated incidents from the correlation engine
- **Approval_Gate**: A mechanism requiring human confirmation before executing high-risk remediation actions
- **Circuit_Breaker**: A safety mechanism that pauses the agent after repeated failures

## Requirements

### Requirement 1: CockroachDB Schema and Database Setup

**User Story:** As a platform operator, I want BrainOps's memory schema deployed to CockroachDB automatically, so that the agent has a structured persistent memory layer from first deployment.

#### Acceptance Criteria

1. WHEN BrainOps is deployed for the first time, THE system SHALL create the `ollinai` schema in CockroachDB containing tables for checkpoints, incidents, incident_embeddings, resolutions, deployments, cert_ledger, and audit_log
2. THE schema migration SHALL be idempotent such that running it multiple times produces no errors and does not duplicate data
3. THE incidents table SHALL include indexes on (tenant_id, created_at DESC), (tenant_id, node_id, created_at DESC), and (tenant_id, namespace, created_at DESC) for efficient temporal and spatial queries
4. THE incident_embeddings table SHALL include a distributed vector index using CockroachDB's C-SPANN algorithm with vector_cosine_ops for similarity search
5. ALL tables SHALL include a tenant_id column with Row-Level Security (RLS) policies enforcing `tenant_id = current_setting('app.tenant_id')::UUID`
6. THE incidents table SHALL have Row-Level TTL configured to auto-delete records older than 90 days, and audit_log older than 365 days
7. THE checkpoints and resolutions tables SHALL have no TTL (agent memory is retained indefinitely)


### Requirement 2: LangGraph Agent with CockroachDB Checkpointer

**User Story:** As a platform operator, I want BrainOps to persist its reasoning state in CockroachDB, so that the agent survives pod restarts and resumes from its last checkpoint without losing context.

#### Acceptance Criteria

1. THE BrainOps agent SHALL use @langchain/langgraph-checkpoint-postgres to persist state to CockroachDB after each reasoning step
2. WHEN the BrainOps pod is terminated and restarted, THE agent SHALL resume execution from the last saved checkpoint within 10 seconds of startup
3. THE agent SHALL implement a state graph with nodes for: receive_incident, search_memory, reason, act, remember
4. WHEN the agent completes a remediation action, THE agent SHALL save the outcome to CockroachDB (resolution + audit entry) before checkpointing
5. THE checkpointer SHALL set the tenant_id session variable before every database operation to enforce RLS isolation
6. IF the CockroachDB connection is unavailable during checkpoint save, THEN THE agent SHALL retry with exponential backoff (1s, 2s, 4s) up to 3 times and log a warning if all retries fail

### Requirement 3: Custom MCP Server Tools

**User Story:** As the BrainOps agent, I want structured tools for querying memory and executing actions, so that I can reason over historical data and orchestrate remediations safely.

#### Acceptance Criteria

1. THE Custom_MCP server SHALL expose the tool `search_similar_incidents(description, limit)` that generates an embedding for the input description and queries CockroachDB's distributed vector index returning up to `limit` (default 5) similar past incidents with cosine similarity scores
2. THE Custom_MCP server SHALL expose the tool `query_incident_history(node_id, namespace, time_range_days)` that returns incidents matching the specified filters from CockroachDB, ordered by created_at DESC
3. THE Custom_MCP server SHALL expose the tool `get_resolution_playbook(incident_id)` that returns the resolution actions, success status, and duration for a given incident
4. THE Custom_MCP server SHALL expose the tool `execute_remediation(action_type, target, reason)` that executes a Kubernetes action (pod restart, node cordon, cert renewal) and writes a record to the audit_log
5. THE Custom_MCP server SHALL expose the tool `store_resolution(incident_id, actions, success, duration_ms)` that writes a resolution record to CockroachDB for future playbook reuse
6. THE Custom_MCP server SHALL expose the tool `get_node_history(node_id, days)` that returns all incidents and audit entries for a node within the specified time window
7. ALL Custom_MCP tools SHALL validate tenant_id and set the CockroachDB session variable before executing queries
8. THE execute_remediation tool SHALL enforce an approval gate for high-risk actions (node_drain, pod_delete in production namespaces) by writing a pending_approval record and awaiting operator confirmation

### Requirement 4: CockroachDB Cloud Managed MCP Server Integration

**User Story:** As BrainOps, I want to connect to the CockroachDB Cloud Managed MCP endpoint for schema exploration and direct SQL queries, so that I can discover and query my memory without custom proxy infrastructure.

#### Acceptance Criteria

1. THE BrainOps agent SHALL connect to the CockroachDB Cloud Managed MCP endpoint at cockroachlabs.cloud/mcp using an API key stored in AWS Secrets Manager
2. THE agent SHALL use the Managed_MCP for read-only operations: schema discovery, SQL SELECT queries, and vector search queries
3. THE agent SHALL NOT execute write operations (INSERT, UPDATE, DELETE) through the Managed_MCP — all writes go through Custom_MCP tools
4. ALL queries executed via the Managed_MCP SHALL be logged by CockroachDB's built-in audit logging
5. THE connection configuration SHALL be a single JSON config snippet compatible with MCP-compatible clients

### Requirement 5: Vector Embedding Pipeline

**User Story:** As a platform operator, I want incidents automatically embedded and stored for similarity search, so that BrainOps can answer "have I seen this before?" in sub-100ms.

#### Acceptance Criteria

1. WHEN the correlation engine writes a new incident to CockroachDB, THE embedding pipeline SHALL generate a vector embedding of the incident narrative using AWS Bedrock (Titan Embeddings model)
2. THE embedding SHALL be stored in the incident_embeddings table with the incident_id, tenant_id, model_version, and a 1536-dimension vector
3. THE distributed vector index (C-SPANN) SHALL enable similarity search returning results in under 100ms for tables with up to 1 million embeddings
4. IF the embedding generation fails (Bedrock unavailable), THEN THE system SHALL retry up to 3 times with exponential backoff and, if all retries fail, log a warning and skip the embedding without blocking incident storage
5. THE search_similar_incidents MCP tool SHALL query the vector index using cosine similarity and return incidents with similarity score above 0.7 by default (configurable threshold)

### Requirement 6: NATS Event Bus Subscription

**User Story:** As the BrainOps agent, I want to subscribe to high-confidence correlated incidents from the NATS event bus, so that I can enhance the correlation engine's decisions with historical reasoning.

#### Acceptance Criteria

1. THE BrainOps agent SHALL subscribe to the NATS subject `titanops.correlation.incidents.>` for all correlated incidents
2. WHEN a correlated incident with confidence score >= 70 is received, THE agent SHALL trigger the reasoning pipeline (search memory → reason → act → remember)
3. WHEN a correlated incident with confidence score < 70 is received, THE agent SHALL store the incident in CockroachDB for future reference without triggering active remediation
4. IF the NATS connection is lost, THE agent SHALL attempt reconnection with exponential backoff and log a degraded-mode warning
5. THE agent SHALL process incidents sequentially per tenant (no concurrent reasoning for the same tenant) to avoid conflicting actions


### Requirement 7: Self-Optimization via Agent Skills

**User Story:** As a platform operator, I want BrainOps to monitor and optimize its own CockroachDB memory layer, so that query performance remains healthy as data grows without manual DBA intervention.

#### Acceptance Criteria

1. THE agent SHALL run a self-optimization cycle every 6 hours (configurable) that uses the `profiling-statement-fingerprints` skill to identify queries against ollinai tables with mean latency exceeding 100ms
2. WHEN slow queries are detected, THE agent SHALL use the `analyzing-range-distribution` skill to check for hotspots (any range holding more than 30% of table data)
3. WHEN a fix is proposed (new index, hash-sharding, TTL adjustment), THE agent SHALL use the `analyzing-schema-change-storage-risk` skill to evaluate whether the fix is safe to apply
4. IF the risk level is "low" and the table size is under 1GB, THE agent MAY auto-execute the fix and log it to audit_trail
5. IF the risk level is "medium" or "critical", THE agent SHALL recommend the fix to the operator via the dashboard and await approval
6. THE self-optimization cycle SHALL write its findings (healthy, recommendation, or auto-fix) to the audit_log for observability

### Requirement 8: ccloud CLI Integration

**User Story:** As the BrainOps agent, I want to verify CockroachDB cluster health before executing critical remediations, so that I don't make decisions based on stale or corrupted memory.

#### Acceptance Criteria

1. BEFORE executing any remediation action via execute_remediation, THE agent SHALL run `ccloud cluster describe --format json` and verify the cluster state is "RUNNING"
2. IF the cluster state is not "RUNNING", THE agent SHALL abort the remediation, log the failure, and alert the operator
3. THE agent SHALL use a ccloud service account with CLUSTER_READER role (read-only access to cluster metadata)
4. THE agent SHALL parse all ccloud output as JSON (the CLI is designed for machine consumption with --format json)
5. THE agent SHALL periodically (every 1 hour) run `ccloud backup list --format json` to verify that automated backups are completing successfully and alert if the last successful backup is older than 24 hours

### Requirement 9: Security and Multi-Tenancy

**User Story:** As a platform operator, I want BrainOps to enforce strict tenant isolation and prevent runaway autonomous actions, so that the agent is safe for multi-tenant production use.

#### Acceptance Criteria

1. THE agent SHALL set `SET app.tenant_id = '<tenant_uuid>'` on every CockroachDB connection before executing queries, ensuring RLS policies are enforced
2. THE agent SHALL enforce a rate limit of 10 MCP tool calls per minute per tenant, rejecting additional calls with a rate-limit error
3. THE agent SHALL implement a circuit breaker that pauses all autonomous actions for a tenant if more than 3 remediation actions fail within 5 minutes
4. WHEN the circuit breaker triggers, THE agent SHALL alert the operator and require manual reset before resuming autonomous actions for that tenant
5. THE agent SHALL classify actions as low-risk (log, alert) or high-risk (node drain, pod delete, cert rotation in production) and require approval gates for high-risk actions
6. ALL tool invocations, reasoning chains, and outcomes SHALL be recorded in the audit_log table with: timestamp, tenant_id, module, action_type, target, confidence, reasoning (JSONB), outcome, operator_id

### Requirement 10: AWS Deployment and Configuration

**User Story:** As a platform operator, I want BrainOps deployable via Helm chart on our existing EKS cluster, so that it integrates with the platform's existing deployment workflow.

#### Acceptance Criteria

1. THE BrainOps service SHALL be deployable as a Kubernetes Deployment in the titanops namespace via the umbrella Helm chart with `brainops.enabled=true`
2. THE Helm chart SHALL configure the BrainOps pod with an IRSA service account that grants access to AWS Secrets Manager (for CockroachDB connection string) and AWS Bedrock (for LLM inference)
3. THE CockroachDB connection string SHALL be retrieved from AWS Secrets Manager at startup (secret ARN from Terraform output)
4. THE BrainOps pod SHALL expose `/healthz` (returns 200 when process is running) and `/readyz` (returns 200 when connected to both NATS and CockroachDB)
5. IF CockroachDB is unreachable at startup, THE pod SHALL fail readiness checks and NOT receive traffic until connection is established
6. THE Helm chart SHALL support configuring: LLM provider (bedrock model ID), NATS URL, log level, rate limits, circuit breaker thresholds, self-optimization schedule
7. THE BrainOps container image SHALL be built as a multi-stage Docker build producing a minimal Node.js 20 Alpine image
