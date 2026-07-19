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
