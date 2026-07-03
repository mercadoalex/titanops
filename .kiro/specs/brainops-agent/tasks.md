# Tasks

## Task 1: Project Scaffolding and Configuration

- [x] 1.1 Initialize `brainops/` TypeScript project with package.json, tsconfig.json, and dependencies (langgraph, @langchain/langgraph-checkpoint-postgres, pg, nats, @aws-sdk/client-bedrock-runtime, @aws-sdk/client-secrets-manager)
- [x] 1.2 Create `src/config/index.ts` with BrainOpsConfig interface and environment variable loading
- [x] 1.3 Create Dockerfile (multi-stage build, Node.js 20 Alpine)
- [x] 1.4 Create `src/health/endpoints.ts` with /healthz and /readyz HTTP endpoints
- [x] 1.5 Create `src/index.ts` entry point that wires startup: load config → connect DB → connect NATS → start agent → start health server

## Task 2: CockroachDB Schema and Database Client
> depends on: Task 1

- [~] 2.1 Create `migrations/001_initial_schema.sql` with full schema (checkpoints, incidents, incident_embeddings, resolutions, deployments, audit_log, cert_ledger) including RLS policies, TTL, and vector index
- [~] 2.2 Create `src/db/client.ts` with connection pool setup, tenant_id session management, and health check method
- [~] 2.3 Create `src/db/migrations.ts` with idempotent migration runner (runs on startup)
- [~] 2.4 Create `src/db/queries/incidents.ts` with insert, query by node/namespace/time, and get-by-id methods
- [~] 2.5 Create `src/db/queries/embeddings.ts` with insert and vector similarity search (cosine, with threshold filter)
- [~] 2.6 Create `src/db/queries/resolutions.ts` with insert, query by incident, and update reuse_count
- [~] 2.7 Create `src/db/queries/audit.ts` with insert and query methods

## Task 3: LangGraph Agent with Checkpointer
> depends on: Task 2

- [~] 3.1 Create `src/agent/state.ts` with AgentState annotation and all type interfaces
- [~] 3.2 Create `src/agent/checkpointer.ts` that configures PostgresSaver with CockroachDB connection string
- [~] 3.3 Create `src/agent/nodes/receive.ts` — stores incident, generates embedding, transitions to search
- [~] 3.4 Create `src/agent/nodes/search-memory.ts` — vector search + playbook lookup + node history
- [~] 3.5 Create `src/agent/nodes/reason.ts` — calls Bedrock with context, returns reasoning chain
- [~] 3.6 Create `src/agent/nodes/act.ts` — checks safety, executes remediation, writes audit
- [~] 3.7 Create `src/agent/nodes/remember.ts` — stores resolution, updates playbook reuse_count
- [~] 3.8 Create `src/agent/graph.ts` — assembles nodes into StateGraph, compiles with checkpointer

## Task 4: Custom MCP Server
> depends on: Task 2

- [~] 4.1 Create `src/mcp/server.ts` with MCP server setup and tool registration
- [~] 4.2 Create `src/mcp/tools/search-similar.ts` — generate embedding + vector search
- [~] 4.3 Create `src/mcp/tools/query-history.ts` — filtered incident query
- [~] 4.4 Create `src/mcp/tools/get-playbook.ts` — resolution lookup
- [~] 4.5 Create `src/mcp/tools/execute-remediation.ts` — K8s action + audit write + approval gate
- [~] 4.6 Create `src/mcp/tools/store-resolution.ts` — write resolution record
- [~] 4.7 Create `src/mcp/tools/get-node-history.ts` — combined incident + audit query

## Task 5: NATS Subscriber
> depends on: Task 3

- [~] 5.1 Create `src/nats/subscriber.ts` with connection, subscription to `titanops.correlation.incidents.>`, and reconnect logic
- [~] 5.2 Create `src/nats/types.ts` with event deserialization from protobuf/JSON
- [~] 5.3 Wire subscriber to agent graph: on incident received → invoke agent with tenant context

## Task 6: Embedding Pipeline
> depends on: Task 2

- [~] 6.1 Create `src/embeddings/generator.ts` using AWS Bedrock Titan Embeddings (1536 dimensions)
- [~] 6.2 Create `src/embeddings/pipeline.ts` — incident narrative → embedding → store to CockroachDB
- [~] 6.3 Add retry logic (3 attempts, exponential backoff) for embedding generation failures

## Task 7: Safety Layer
> depends on: Task 1

- [~] 7.1 Create `src/safety/rate-limiter.ts` with sliding window (10 calls/min per tenant)
- [~] 7.2 Create `src/safety/circuit-breaker.ts` with configurable threshold and auto-pause
- [~] 7.3 Create `src/safety/approval-gate.ts` with high-risk action detection and pending_approval writes

## Task 8: CockroachDB Cloud Managed MCP Client
> depends on: Task 1

- [~] 8.1 Create `src/mcp/managed-mcp.ts` with connection to cockroachlabs.cloud/mcp endpoint
- [~] 8.2 Implement read-only query execution via Managed MCP
- [~] 8.3 Implement schema discovery via Managed MCP

## Task 9: ccloud CLI Integration
> depends on: Task 1

- [~] 9.1 Create `src/ccloud/client.ts` with subprocess wrapper for ccloud commands (JSON output parsing)
- [~] 9.2 Implement `clusterDescribe()` — verify cluster state is RUNNING
- [~] 9.3 Implement `backupList()` — verify last backup within 24 hours

## Task 10: Self-Optimization
> depends on: Task 2, Task 8

- [~] 10.1 Create `src/self-optimization/scheduler.ts` with cron-based 6-hour cycle
- [~] 10.2 Create `src/self-optimization/profiler.ts` — query crdb_internal.statement_statistics for slow queries
- [~] 10.3 Create `src/self-optimization/range-analyzer.ts` — SHOW RANGES analysis for hotspots
- [~] 10.4 Create `src/self-optimization/schema-risk.ts` — evaluate storage impact of proposed fixes
- [~] 10.5 Create orchestrator that runs detect → diagnose → evaluate → recommend/fix cycle

## Task 11: Helm Chart
> depends on: Task 1

- [~] 11.1 Create `helm/charts/brainops/Chart.yaml` and `values.yaml`
- [~] 11.2 Create deployment.yaml with IRSA service account, resource limits, env vars from Secrets Manager
- [~] 11.3 Create service.yaml for health endpoints
- [~] 11.4 Add `brainops.enabled` toggle to umbrella chart values

## Task 12: Tests
> depends on: Task 3, Task 4, Task 7

- [~] 12.1 Unit tests for agent state transitions (each node)
- [~] 12.2 Unit tests for MCP tool input validation
- [~] 12.3 Unit tests for rate limiter and circuit breaker
- [~] 12.4 Integration test: full agent flow with test CockroachDB (docker-compose)
- [~] 12.5 Integration test: checkpoint save/restore (simulate pod restart)
