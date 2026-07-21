# TitanOps MCP Server

## Why We Built This

TitanOps is an autonomous platform — it observes, decides, and acts without human intervention. That's its strength. But it also creates a problem: **how do humans and AI assistants understand what happened?**

When TitanOps isolates a pod at 3am, the on-call engineer wakes up to a fait accompli. They need answers fast:
- Why was the pod isolated?
- What evidence led to that decision?
- Is the threat real or a false positive?
- What else happened in the cluster around that time?

Previously, they'd dig through dashboards, correlation timelines, and audit logs manually. Now, they ask an AI agent — Claude, GPT, Gemini, Kiro — and it queries TitanOps directly.

The MCP Server is the bridge between TitanOps's autonomous intelligence and AI assistants that can reason about it in natural language.

### The Core Insight

AI SRE tools (like Metoro) are only as good as the data they can access. By exposing TitanOps data via the Model Context Protocol, we allow **any** AI client to:
- Investigate incidents using the same data TitanOps used to make decisions
- Explain autonomous actions in human-friendly language
- Correlate signals across modules without learning our internal APIs
- Verify deployment health without writing custom queries

### What It Replaces

The TitanOps MCP Server replaces `brainops/` — a 124MB TypeScript backend (LangGraph + NATS + MCP tools) that was:
- Written in a different language than the rest of the platform
- Dependent on `node_modules` (45 packages, 3,841 lines of TypeScript)
- Impossible to share code with the Go modules
- A maintenance burden that contradicted the Go-first strategy

The replacement: a **4.6MB Go binary** that does the same job, uses the same language as the platform, and requires zero configuration.

---

## What Is the Model Context Protocol (MCP)?

MCP is an open standard that defines how AI clients (Claude, GPT, Kiro, Cursor) communicate with external tools. Instead of every AI building custom integrations with every service, MCP provides a universal interface:

```
AI Client ←→ MCP Protocol ←→ MCP Server ←→ Your System
```

An MCP server exposes **tools** (functions the AI can call), **resources** (data it can read), and **prompts** (templates for common tasks). The AI client discovers available tools, calls them with structured arguments, and receives structured responses.

Think of it like a REST API, but designed specifically for AI agents — with auto-generated schemas, tool descriptions that guide the AI on when and how to use them, and a transport layer optimized for AI-to-tool communication.

---

## Stdio Transport

### What It Is

The TitanOps MCP Server uses **stdio transport** — it communicates by reading JSON-RPC from `stdin` and writing responses to `stdout`. No HTTP server, no ports, no network.

```
AI Client (Claude/Kiro)  ←→  stdin/stdout  ←→  titanops-mcp process
```

The AI client spawns `titanops-mcp` as a child process, sends tool calls to its stdin, reads results from its stdout. Same principle as Unix pipes (`cat file | grep foo`).

### How It Works

When you configure an AI client:

```json
{"mcpServers": {"titanops": {"command": "/path/to/titanops-mcp"}}}
```

The client:
1. Spawns the `titanops-mcp` process
2. Writes JSON-RPC tool call requests to the process's stdin
3. Reads JSON-RPC responses from the process's stdout
4. Kills the process when the session ends

No server to start. No port to expose. No URL to configure.

### Why Stdio Over HTTP

| Aspect | Stdio | HTTP |
|--------|-------|------|
| Setup | Zero — just point to the binary | Need a port, TLS, maybe auth |
| Security | Process-local, no network exposure | Opens a port on the machine |
| Latency | Zero network hop (IPC between processes) | Loopback HTTP adds overhead |
| Stateful | Yes — persistent connection, supports notifications | Stateless per request |
| Discovery | Client spawns and manages the process lifecycle | Client needs to know host:port |
| Deployment | Single binary, no daemon needed | Requires running a server |

### When Stdio Is Right (Our Case)

- **Local AI clients** (Claude Desktop, Kiro, Cursor) — they run on the same machine and spawn processes
- **Security** — the MCP server never listens on a port, can't be accessed remotely by default
- **Zero config** — no ports, no firewall rules, no DNS, no TLS certificates

### When HTTP Would Be Better (Future)

- MCP server running inside the K8s cluster, AI client connecting remotely
- Multiple AI clients sharing one server instance
- Serverless/container environments where you can't spawn child processes

If needed, `mcp-golang` supports HTTP transport — we can add it as an option without rewriting the tool handlers.

---

## Tools Exposed

The MCP server exposes 6 tools that cover the full TitanOps investigation workflow:

### `get_module_health`

Returns the health status of all TitanOps platform modules.

**When an AI uses this:** "Is anything degraded right now?" or "Check if eBeeControl is healthy."

**Arguments:**
- `module` (optional): Filter by module ID (`earthworm`, `ebeecontrol`, `ollinai`, `quack`)

**Returns:** Module ID, version, state (healthy/degraded/unhealthy), message, component details.

---

### `get_recent_incidents`

Returns correlated incidents detected by the correlation engine.

**When an AI uses this:** "What incidents happened in the last hour?" or "Show me critical incidents in production."

**Arguments:**
- `limit` (optional): Max incidents to return (default 10, max 50)
- `severity` (optional): Minimum severity filter (`critical`, `high`, `medium`, `low`)
- `namespace` (optional): Filter by Kubernetes namespace

**Returns:** Incident ID, confidence score, contributing modules, summary, actions taken, status.

---

### `get_audit_trail`

Returns the audit trail of autonomous decisions.

**When an AI uses this:** "Why did TitanOps cordon worker-3?" or "What decisions has ebeecontrol made today?"

**Arguments:**
- `limit` (optional): Max entries (default 20, max 100)
- `module` (optional): Filter by module
- `decision_type` (optional): Filter by type (`discovery`, `deployment`, `assessment`, `response`, `learning`, `model_update`)

**Returns:** Timestamp, module, decision type, rationale, input data summary, outcome.

---

### `get_deployment_verdicts`

Returns deployment verification results from OllinAI.

**When an AI uses this:** "Was the last deploy to payments-service healthy?" or "Show me all regressions today."

**Arguments:**
- `limit` (optional): Max verdicts (default 10, max 50)
- `status` (optional): Filter by verdict (`healthy`, `regression`, `inconclusive`)
- `namespace` (optional): Filter by namespace

**Returns:** Verdict ID, status, workload name/kind, namespace, signal checks (baseline vs current vs threshold), commit SHA, duration.

---

### `search_events`

Searches the TitanOps event store across all modules.

**When an AI uses this:** "What happened in the production namespace in the last 30 minutes?" or "Show me all ebeecontrol events."

**Arguments:**
- `module` (optional): Source module filter
- `event_type` (optional): Event type filter (e.g., `honeytoken_access_response`, `autonomous_remediation`, `deployment_verification`)
- `severity` (optional): Minimum severity
- `namespace` (optional): Namespace filter
- `limit` (optional): Max events (default 20, max 100)
- `since` (optional): Time filter (ISO 8601 or duration like `1h`, `30m`, `24h`)

**Returns:** Event ID, module, type, severity, namespace, timestamp, labels.

---

### `explain_action`

Provides a full natural-language explanation of a specific autonomous action.

**When an AI uses this:** "Explain what happened with action act-pod-isolation-001" or "Walk me through incident inc-2026-0719-001."

**Arguments:**
- `action_id` (optional): The specific action to explain
- `incident_id` (optional): Explain all actions for an incident

**Returns:** Complete evidence trail:
- **Trigger**: What happened, who did it, where, when
- **Decision**: Classification, reasoning chain, input values, latency
- **Actions**: Each action taken with type, target, result, retries, duration
- **Outcome**: Whether threat was contained, all actions succeeded, learning submitted

---

## Configuration

### Claude Desktop

File: `~/Library/Application Support/Claude/claude_desktop_config.json`

```json
{
  "mcpServers": {
    "titanops": {
      "command": "/path/to/titanops-mcp"
    }
  }
}
```

### Kiro

File: `.kiro/settings/mcp.json` (workspace) or `~/.kiro/settings/mcp.json` (global)

```json
{
  "mcpServers": {
    "titanops": {
      "command": "go",
      "args": ["run", "./cmd/titanops-mcp"],
      "env": {
        "TITANOPS_API_URL": "http://localhost:8080"
      }
    }
  }
}
```

### Build and Install

```bash
# Build the binary
cd titanops/
go build -o titanops-mcp ./cmd/titanops-mcp

# Move to PATH (optional)
mv titanops-mcp /usr/local/bin/

# Or run directly
go run ./cmd/titanops-mcp
```

---

## Architecture

```
┌─────────────────────────────────────────────────┐
│  AI Client (Claude / Kiro / GPT / Gemini)       │
│                                                  │
│  "Why was pod api-server isolated?"             │
│                    │                             │
│                    ▼                             │
│            MCP Tool Call                         │
│    explain_action(action_id="act-001")          │
└────────────────────┬────────────────────────────┘
                     │ stdin (JSON-RPC)
                     ▼
┌─────────────────────────────────────────────────┐
│  titanops-mcp (4.6MB Go binary)                 │
│                                                  │
│  ┌─────────────────────────────────────────┐    │
│  │  Tool Router                             │    │
│  │  get_module_health → handler             │    │
│  │  get_recent_incidents → handler          │    │
│  │  get_audit_trail → handler               │    │
│  │  get_deployment_verdicts → handler       │    │
│  │  search_events → handler                 │    │
│  │  explain_action → handler                │    │
│  └────────────────┬────────────────────────┘    │
│                   │                              │
│                   ▼                              │
│  ┌─────────────────────────────────────────┐    │
│  │  Platform Data Access                    │    │
│  │  (queries RuntimeKernel, Correlation     │    │
│  │   Engine, Event Store, Audit Log)        │    │
│  └─────────────────────────────────────────┘    │
└────────────────────┬────────────────────────────┘
                     │ stdout (JSON-RPC)
                     ▼
┌─────────────────────────────────────────────────┐
│  AI Client receives structured response          │
│                                                  │
│  "The pod was isolated because eBeeControl      │
│   detected a critical threat: honeytoken        │
│   /var/run/secrets/token was accessed by         │
│   process /tmp/.hidden/scanner (PID 4521).      │
│   Classification: critical (production + anomaly │
│   0.92 + criticality 5). Response: isolation    │
│   succeeded in 230ms, IP blocked in 180ms."    │
└─────────────────────────────────────────────────┘
```

---

## Current State & Future

**Current:** The MCP server returns representative data structures that demonstrate the full tool interface and response schemas. This is sufficient for:
- Testing AI client integration
- Validating tool schemas work correctly with Claude/Kiro
- Demonstrating the product's AI investigation capability

**Production wiring (next step):** Connect tool handlers to the actual RuntimeKernel (via gRPC or HTTP calls to `cmd/titanops`), the correlation engine's event store, and the audit log. This turns the mock data into real cluster data.

**Future additions:**
- `rollback_deployment` tool — let the AI suggest and execute rollbacks
- `override_action` tool — let the AI mark false positives
- `get_honeytoken_map` — show deception coverage
- HTTP transport option for remote access
- Authentication for multi-tenant environments
