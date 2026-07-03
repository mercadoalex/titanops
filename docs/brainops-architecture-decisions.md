# BrainOps Architecture Decisions

## ADR: Why LangGraph for the Agent Framework

### Context

BrainOps needs an agent framework that supports:
- Persistent state checkpointing to CockroachDB
- TypeScript (our agent/UI language)
- Human-in-the-loop approval gates for high-risk actions
- Auditable reasoning chains (visible in dashboard)
- Compatibility with the CockroachDB hackathon ecosystem

### Decision

**Use LangGraph.js** as the agent orchestration framework.

### Why LangGraph

| Reason | Detail |
|--------|--------|
| **Built-in checkpointing to Postgres** | `@langchain/langgraph-checkpoint-postgres` works with CockroachDB out of the box. This is THE hackathon feature — agent memory in CockroachDB. Other frameworks require custom persistence code. |
| **TypeScript native** | LangGraph.js is production-ready. Fits our language strategy (no Python). |
| **State graph model** | Explicit nodes + edges = auditable reasoning chain. You can see exactly where the agent is and what it decided. |
| **Human-in-the-loop built-in** | LangGraph has native "interrupt" points — perfect for our approval gates on high-risk actions. |
| **CockroachDB blog alignment** | CockroachDB's own docs and blog posts show LangGraph + CockroachDB checkpointer as the reference pattern. Judges will recognize this. |

### Alternatives Considered

| Framework | Language | Pros | Why Not |
|-----------|----------|------|---------|
| **Strands (AWS)** | Python | AWS-native, Bedrock integration | Python (adds a language), no native CockroachDB checkpointer |
| **CrewAI** | Python | Multi-agent orchestration | Python, no Postgres checkpointer, overkill for single-agent use case |
| **AutoGen** | Python | Microsoft-backed, flexible | Python, complex setup, memory is an afterthought |
| **Mastra** | TypeScript | TS-native, clean API | Newer, less mature, no built-in Postgres checkpointer |
| **Custom (raw code)** | Go or TS | Full control, no dependencies | We'd have to build checkpointing, state graphs, interrupts from scratch — weeks of work |
| **Vercel AI SDK** | TypeScript | Good for chat/streaming | Not designed for autonomous agents with persistent state — it's for UI-facing apps |

### The Deciding Factors

1. **CockroachDB checkpointer exists and works** — saves 1-2 weeks of building custom persistence
2. **TypeScript** — no new language added to the project
3. **The hackathon judges expect it** — CockroachDB's own content (blogs, webinars, Memori partnership) showcases LangGraph as the canonical agentic framework for their platform
4. **Human-in-the-loop interrupts** — we need approval gates, and LangGraph handles this natively
5. **State graph = explainability** — the dashboard can visualize exactly which node the agent is on and what it decided at each step

### One Honest Downside

LangGraph adds a dependency on the LangChain ecosystem. If we later decide LangChain's abstractions are too heavy, we'd need to migrate. But for the hackathon and initial production, it's the fastest path to a working agent with persistent CockroachDB memory.

### Conclusion

LangGraph isn't the "best" framework universally. It's the best fit for this specific project because of the CockroachDB checkpointer, TypeScript support, and alignment with what the hackathon judges expect to see.

---

## ADR: LangGraph Checkpointer — Pod Restart Survival

### The Problem

A normal AI agent keeps its reasoning state in memory (RAM). If the pod/container crashes or gets restarted (which happens constantly in Kubernetes — scaling, deployments, node failures), all state is lost. The agent starts from zero.

```
BrainOps receives incident → starts reasoning → searches memory → 
  decides to renew cert → CRASH → pod restarts → 
  "What was I doing?" → starts completely over
```

### The Solution

LangGraph builds agents as state graphs — each step (receive, search, reason, act, remember) is a node, and the agent moves through them.

A checkpointer saves the agent's full state to CockroachDB after every node transition:

```
Node 1: receive_incident   → [state saved to CockroachDB] ✓
Node 2: search_memory      → [state saved to CockroachDB] ✓
Node 3: reason             → [state saved to CockroachDB] ✓
Node 4: act (renew cert)   → CRASH HERE
                              ↓
Pod restarts → reads last checkpoint from CockroachDB
  → "I was at Node 4, about to execute renew_cert"
  → resumes from exactly that point
```

### In Code (simplified)

```typescript
import { StateGraph } from "@langchain/langgraph";
import { PostgresSaver } from "@langchain/langgraph-checkpoint-postgres";

// CockroachDB is Postgres-compatible, so this just works
const checkpointer = PostgresSaver.fromConnString(process.env.COCKROACHDB_URI);

const agent = new StateGraph(AgentState)
  .addNode("receive", receiveIncident)
  .addNode("search", searchMemory)
  .addNode("reason", reasonWithContext)
  .addNode("act", executeRemediation)
  .addNode("remember", storeResolution)
  .compile({ checkpointer });  // ← This is the magic line
```

Every time the agent moves between nodes, the full state (what incident it's processing, what it found in memory, what action it decided on) gets serialized to the `ollinai.checkpoints` table in CockroachDB.

### Why This Matters

- **Reliability**: Kubernetes kills pods all the time (OOM, node scaling, deploys). Agent doesn't lose work.
- **Hackathon demo**: "Watch — I kill the pod mid-reasoning, it comes back and finishes the job."
- **Production**: In multi-step remediations (renew cert → verify → reboot → verify again), losing state mid-way could leave infrastructure in a broken half-fixed state.
- **Audit**: Checkpoints are queryable — you can see exactly where the agent was in its reasoning at any point in time.

### Why CockroachDB (not Redis or local file)

- **Survives node failures** (not just pod restarts) — data is distributed
- **Serializable consistency** — no corrupted half-written state
- **Multi-tenant** — RLS means one tenant's agent can't read another's checkpoints
- **Same database** as incidents, embeddings, audit — one operational store, one backup strategy

---

## ADR: Language Strategy

### Decision

| Layer | Language | Components |
|-------|----------|-----------|
| **Kernel** | Rust + C | eBPF programs (Tlapix/Aya, Tetragon, sched_ext, cilium/ebpf) |
| **Backend** | Go | Modules (Earthworm, Quack, eBeeControl), correlation engine, gateway, shared libs |
| **Agent + UI** | TypeScript | BrainOps (LangGraph.js), dashboard (React), OllinAI module |

### Rationale

- **4 languages total** with clear boundaries — no overlap or ambiguity
- **No Python** — despite LangGraph being Python-first, the JS version is mature and avoids adding a 5th language
- **Go for all backend services** — once eBeeControl is rewritten from TypeScript, all platform services are Go
- **TypeScript for all user-facing and agent code** — dashboard, BrainOps, OllinAI module share runtime and tooling

---

## ADR: Infrastructure as Code (Layered Terraform)

### Decision

All shared infrastructure lives in `titanops/infra/` with feature flags. Modules deploy via Helm on top.

### Layers

```
Layer 0: Single Module (module's own deploy/ directory, kind/minikube)
Layer 1: Platform Base (EKS + VPC + IRSA, modules toggleable)
Layer 2: Intelligence (CockroachDB + BrainOps, opt-in)
```

### Cost Control

| Configuration | What's Provisioned | Monthly Cost |
|--------------|-------------------|-------------|
| Just Quack | EKS + 1 node | ~$75 |
| Platform without AI | All modules + NATS + correlation | ~$100 |
| Full BrainOps | Everything + CockroachDB (free tier) | ~$130 |

### Rule

- If used by multiple modules or is platform-level → `titanops/infra/`
- If only used by one standalone module → `<module>/deploy/`
- CockroachDB, NATS, BrainOps IRSA = shared platform infrastructure
