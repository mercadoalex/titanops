# Technology Evaluations

Record of technology assessments for TitanOps. Each evaluation captures the context at the time of decision, the reasoning, and the outcome so future contributors don't revisit settled questions without new data.

---

## Mojo Language

**Evaluated:** July 2026  
**Context:** Considered for Quack module hot-path inference (sched_ext scoring, CPU anomaly detection) and potentially other modules.  
**Decision:** Not adopted. Revisit in 12-18 months.

### What Mojo offers

- Python-like syntax with MLIR-backed compilation to native CPU/GPU kernels
- 20x-180x speedups over pure Python on compute kernels (arxiv.org/abs/2606.16059)
- 1.0 Beta released mid-2026 with semantic versioning starting (modular.com/blog/the-path-to-mojo-1-0)
- Designed to solve the "two-language problem" (prototype in Python, rewrite hot path in C++)

### Why not for TitanOps (now)

| Factor | Assessment |
|--------|-----------|
| Performance bottleneck location | TitanOps hot path is eBPF map reads → Go processing → ONNX inference. The bottleneck is kernel scheduling and BPF I/O, not inference speed |
| eBPF integration | None. Mojo targets GPU/CPU compute via MLIR. No BPF backend, no libbpf bindings |
| Go interop | Does not exist. Mojo interops with Python only. Would require FFI bridge or sidecar |
| Language count | Adding a 4th language (Go, Rust, TypeScript, Mojo) increases hiring/onboarding friction |
| Maturity | 1.0 Beta with breaking changes planned for 2.0. Standard library has few stabilized features |
| Current solution | Go + ONNX Runtime (C library via cgo) already provides near-native inference speed |

### Revisit conditions

- Mojo reaches stable 1.x with Go or C FFI story
- Profiling reveals ONNX inference as actual bottleneck (>10% of hot-path latency)
- Mojo develops BPF or kernel-level integration
- Team grows to where a dedicated ML/inference engineer could own a Mojo component

---

## LangSmith (LLM Observability & Evaluation)

**Evaluated:** July 2026  
**Context:** Considered for BrainOps agent observability and as potential replacement for custom evaluation framework.  
**Decision:** Adopt for BrainOps tracing only. Do NOT replace the platform-wide structured evaluation framework.

### What LangSmith offers

- Automatic tracing for LangGraph/LangChain agents (BrainOps already uses `@langchain/langgraph`)
- Evaluation datasets with LLM-as-judge scoring
- Production monitoring: latency, token usage, error rates per agent node
- Annotation queues for human review of agent decisions
- <50ms overhead per trace (markaicode.com)

### Adoption scope

**Use for:**
- BrainOps LLM agent tracing (tool calls, reasoning chains, memory retrievals)
- Debugging agent decision loops in development
- Token cost monitoring in production

**Do NOT use for:**
- Platform-wide evaluation (correlation scoring, Quack anomaly detection, Earthworm thresholds) — these remain in the YAML-based structured evaluation framework (Requirement 14)
- Go module observability — use Prometheus + OTLP as defined in engineering standards
- CI regression gating — LLM-as-judge is non-deterministic, not suitable for pass/fail gates

### Why not platform-wide

| Concern | Detail |
|---------|--------|
| Scope mismatch | LangSmith only covers LLM interactions. Go modules (Correlation, Earthworm, Quack) don't use LLMs |
| Determinism | Custom eval framework uses YAML scenarios with deterministic mock-mode outputs. LangSmith evals are non-deterministic (LLM-graded) |
| Data residency | Custom eval reports are git-committed JSON in `eval/reports/`. LangSmith data lives in their cloud |
| Vendor lock-in | Tight coupling to LangChain ecosystem. Switching agent frameworks means ripping out observability |
| Cost | Pricing scales linearly with trace volume |

### Alternative considered: Langfuse

Open-source, self-hosted, framework-agnostic. If vendor lock-in becomes a concern or LangSmith pricing doesn't scale, Langfuse is the fallback with equivalent tracing capabilities. No migration needed for the custom eval framework since they serve different purposes.

### Integration plan

1. Add `langsmith` SDK to BrainOps dependencies
2. Set `LANGCHAIN_TRACING_V2=true` and `LANGCHAIN_API_KEY` in BrainOps deployment
3. Traces flow automatically from existing LangGraph agent — no code changes needed
4. Use LangSmith datasets for BrainOps-specific LLM quality regression (subjective output quality)
5. Keep platform eval framework (YAML scenarios, `TITANOPS_MODE=mock`) for all deterministic scoring validation

---

## Decision Log

| Date | Technology | Module | Decision | Rationale |
|------|-----------|--------|----------|-----------|
| 2026-07 | Mojo | Quack / All | Deferred | No eBPF story, no Go interop, solves wrong bottleneck |
| 2026-07 | LangSmith | BrainOps | Adopted (narrow scope) | Natural fit for LangGraph tracing, but not a platform-wide eval replacement |
| 2026-07 | Custom Eval Framework | All modules | Adopted | Deterministic, CI-friendly, covers all scoring modules |

---

## VictoriaMetrics Patterns & Techniques

**Evaluated:** July 2026  
**Context:** Studied VictoriaMetrics (github.com/VictoriaMetrics/VictoriaMetrics) for applicable performance patterns, storage techniques, and architectural insights. TitanOps is not a competing TSDB — the goal is to learn from VM's Go performance discipline and apply it to our event pipeline, correlation engine, and export layer.  
**Decision:** Adopt selectively. Six techniques evaluated, two implemented immediately.

### Techniques Evaluated

#### 1. fastcache — GC-free in-memory cache (Adopted)

VM built a custom cache library (github.com/VictoriaMetrics/fastcache) that stores millions of entries with near-zero GC overhead. Data lives in 64KB byte chunks instead of pointer-rich maps, reducing pointer count from O(entries) to O(chunks).

**Applied to TitanOps:** Correlation engine event deduplication cache. Under high event volume, the dedup cache must hold thousands of recent event fingerprints without adding GC pressure to the hot path.

**Source:** [VictoriaMetrics/fastcache](https://github.com/VictoriaMetrics/fastcache)

#### 2. Stream pre-aggregation at the edge (Future)

vmagent aggregates metrics (sum, count, avg, quantiles) during the scrape-to-store pipeline before shipping to storage. This reduces cardinality at the source.

**Applicability to TitanOps:** Modules could pre-aggregate eBPF events before publishing to NATS (e.g., "5 anomalies on node-01 in last 30s, max_score=0.82") instead of shipping every raw event. Reduces correlation engine load and NATS bandwidth.

**Status:** Deferred. Requires changes to titanops-export library. Revisit when event volume exceeds single-node correlation capacity.

#### 3. Event deduplication with configurable windows (Adopted)

VM's deduplication collapses near-duplicate samples within a configurable time window during merge. When two HA instances scrape the same target, only one sample per window survives.

**Applied to TitanOps:** Correlation engine dedup pass before correlation. When multiple modules or HA instances emit near-identical events (same module + event_type + node within a configurable window), collapse them to prevent inflated confidence scores.

#### 4. sync.Pool for serialization buffer reuse (Adopted)

VM uses sync.Pool pervasively to avoid allocations on the hot path — buffer reuse for marshaling, request handling, and encoding. Their codebase is described as a "living textbook of Go high-performance programming."

**Applied to TitanOps:** Event serialization in titanops-export. Every `json.Marshal(event)` currently allocates a fresh `[]byte`. Pooling these buffers reduces GC pressure under sustained load.

#### 5. LSM-style tiered event buffering (Future)

VM uses in-memory parts → small parts (disk) → big parts (disk) with background merge. Parts are merged when the output size is ≥7.5× the largest input part.

**Applicability to TitanOps:** If the titanops-export ring buffer needs to survive longer NATS outages, a disk-backed tier between memory and NATS would prevent event loss. Current ring buffer evicts on overflow.

**Status:** Deferred. Current 1000-event buffer is sufficient for typical outage durations (<60s). Revisit if SLO requires zero event loss.

#### 6. Columnar storage with per-column compression (Future)

VM stores timestamps, values, and indexes in separate files. Only the metaindex is memory-resident. Each column uses type-appropriate compression (delta-of-delta for timestamps, XOR/Gorilla for values).

**Applicability to TitanOps:** Relevant if TitanOps builds an event replay/forensics feature. Storing event payloads column-wise would give 5-10× better compression than row-wise JSON.

**Status:** Deferred. No forensics feature planned currently.

### Architectural Validation

VM's shared-nothing cluster architecture (vmstorage nodes are independent, no cross-node communication, routing by consistent hashing at vminsert) validates TitanOps's existing design principles:

- "Modules never import other modules" — same shared-nothing at module level
- "Worker-per-core, route by affinity" — same pattern VM uses for storage sharding
- "Batch at boundaries, serialize once per batch" — VM does the same at flush time

### Decision Log Update

| Date | Technology | Module | Decision | Rationale |
|------|-----------|--------|----------|-----------|
| 2026-07 | fastcache | Correlation | Adopted | GC-free dedup cache for event fingerprints on hot path |
| 2026-07 | Event dedup window | Correlation | Adopted | Prevents inflated confidence from duplicate events |
| 2026-07 | sync.Pool buffers | titanops-export | Adopted | Reduces GC pressure in serialization path |
| 2026-07 | Stream pre-aggregation | titanops-export | Deferred | Revisit when event volume exceeds single-node capacity |
| 2026-07 | LSM tiered buffering | titanops-export | Deferred | Current ring buffer sufficient for <60s outages |
| 2026-07 | Columnar storage | Platform | Deferred | No forensics feature planned |
