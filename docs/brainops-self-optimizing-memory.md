# BrainOps: Self-Optimizing Agent Memory

## The Meta-Level Intelligence Story

> "The agent optimizes its own memory infrastructure."

Most AI agents treat their database as a black box — they read and write data but have zero awareness of how that database is performing. When queries degrade, a human DBA must intervene.

BrainOps is different. It uses CockroachDB Agent Skills to **monitor, diagnose, and fix performance issues in the very database that stores its memory.** This creates a self-sustaining feedback loop: the smarter the agent gets (more memory), the more it needs to optimize its storage — and it has the skills to do exactly that.

---

## Why This Matters

### Traditional Agent Architecture (Fragile)

```
Agent writes data → Database gets slow → Human DBA investigates
→ DBA adds index → Performance recovers → Weeks of degradation
```

### BrainOps Architecture (Self-Healing)

```
Agent writes data → Agent detects own queries slowing
→ Agent uses Skills to diagnose → Agent recommends/executes fix
→ Performance recovers → Minutes of degradation (or zero)
```

This is what separates a production-grade agentic system from a demo: **the agent maintains the infrastructure it depends on.**

---

## The Self-Optimization Loop

BrainOps runs a periodic self-optimization cycle (every 6 hours by default, configurable):

```
┌─────────────────────────────────────────────────────────┐
│              BrainOps Self-Optimization Loop              │
│                                                          │
│  ┌──────────┐    ┌──────────┐    ┌──────────────────┐   │
│  │  DETECT  │───▶│ DIAGNOSE │───▶│ RECOMMEND / FIX  │   │
│  └──────────┘    └──────────┘    └──────────────────┘   │
│       │                                     │            │
│       │         CockroachDB Agent Skills    │            │
│       ▼                                     ▼            │
│  profiling-         analyzing-         analyzing-schema- │
│  statement-         range-             change-storage-   │
│  fingerprints       distribution       risk              │
│                                                          │
│  "Which queries    "Is data           "Is it safe to    │
│   are slow?"       hotspotted?"        add this index?" │
└─────────────────────────────────────────────────────────┘
```

---

## Phase 1: DETECT — Profiling Statement Fingerprints

**Skill:** `profiling-statement-fingerprints`

BrainOps queries CockroachDB's internal statistics to identify degrading query patterns against its own memory tables.

### What It Does

The skill ranks statement fingerprints from `crdb_internal.statement_statistics` by latency, execution count, and error rate. BrainOps filters for queries touching its memory tables:

- `ollinai.incidents` — incident history lookups
- `ollinai.incident_embeddings` — vector similarity search
- `ollinai.resolutions` — playbook retrieval
- `ollinai.audit_log` — compliance queries
- `ollinai.checkpoints` — agent state reads/writes

### Implementation

```python
async def detect_slow_memory_queries(self) -> list[SlowQuery]:
    """
    Phase 1: Use profiling-statement-fingerprints skill to find
    degrading queries against BrainOps memory tables.
    """
    # Query CockroachDB internal statistics via Managed MCP Server
    result = await self.cockroachdb_mcp.execute_sql("""
        SELECT
            fingerprint_id,
            metadata->>'query' AS query_text,
            statistics->'statistics'->'latencyInfo'->>'mean' AS mean_latency_s,
            statistics->'statistics'->'cnt' AS execution_count,
            statistics->'statistics'->'numRows'->>'mean' AS avg_rows
        FROM crdb_internal.statement_statistics
        WHERE metadata->>'query' LIKE '%ollinai.%'
          AND statistics->'statistics'->'latencyInfo'->>'mean' > '0.1'
        ORDER BY (statistics->'statistics'->'latencyInfo'->>'mean')::FLOAT DESC
        LIMIT 20
    """)

    slow_queries = []
    for row in result:
        slow_queries.append(SlowQuery(
            fingerprint_id=row["fingerprint_id"],
            query_text=row["query_text"],
            mean_latency_ms=float(row["mean_latency_s"]) * 1000,
            execution_count=int(row["execution_count"]),
            avg_rows=float(row["avg_rows"])
        ))

    return slow_queries
```

### Trigger Thresholds

| Metric | Warning | Critical | Action |
|--------|---------|----------|--------|
| Mean latency | >100ms | >500ms | Proceed to Phase 2 |
| P99 latency | >1s | >5s | Proceed to Phase 2 + alert operator |
| Error rate | >1% | >5% | Immediate alert + circuit breaker |
| Execution count spike | >2x baseline | >5x baseline | Investigate (possible agent loop) |

### What BrainOps Learns

- Which memory tables are under pressure
- Whether vector search is the bottleneck (common as embeddings grow)
- Whether checkpoint writes are contending with reads
- Whether a specific tenant is generating disproportionate load (RLS-visible)

---

## Phase 2: DIAGNOSE — Analyzing Range Distribution

**Skill:** `analyzing-range-distribution`

Once slow queries are identified, BrainOps diagnoses the root cause by examining how data is distributed across CockroachDB ranges.

### What It Does

CockroachDB splits tables into ranges (~512MB each) distributed across nodes. If one range gets disproportionately hot (many writes or reads to the same key space), performance degrades. This is the most common cause of vector search slowdowns at scale.

### Common Patterns BrainOps Detects

| Pattern | Cause | Impact on BrainOps |
|---------|-------|-------------------|
| **Hotspot on recent incidents** | All new incidents land in the same range (sorted by `created_at DESC`) | Incident writes contend with each other |
| **Embedding index imbalance** | Vector index partitions unevenly distributed | Some similarity searches hit one node |
| **Tenant skew** | One tenant generates 90% of incidents | RLS doesn't prevent range-level contention |
| **Checkpoint write contention** | Agent checkpoints all go to same thread_id range | State persistence slows down |

### Implementation

```python
async def diagnose_range_distribution(self, table: str) -> RangeAnalysis:
    """
    Phase 2: Use analyzing-range-distribution skill to identify
    hotspots causing the slow queries detected in Phase 1.
    """
    # Get range distribution for the problematic table
    result = await self.cockroachdb_mcp.execute_sql(f"""
        SELECT
            range_id,
            lease_holder,
            range_size_mb,
            range_size_mb / NULLIF(
                SUM(range_size_mb) OVER (), 0
            ) * 100 AS pct_of_total,
            start_key,
            end_key
        FROM [SHOW RANGES FROM TABLE {table} WITH DETAILS]
        ORDER BY range_size_mb DESC
        LIMIT 20
    """)

    # Detect hotspots: any range > 30% of total data
    total_ranges = len(result)
    hot_ranges = [r for r in result if float(r["pct_of_total"]) > 30]

    # Detect leaseholder imbalance
    leaseholders = {}
    for r in result:
        lh = r["lease_holder"]
        leaseholders[lh] = leaseholders.get(lh, 0) + 1

    max_leases = max(leaseholders.values()) if leaseholders else 0
    avg_leases = sum(leaseholders.values()) / len(leaseholders) if leaseholders else 0
    imbalanced = max_leases > avg_leases * 2

    return RangeAnalysis(
        table=table,
        total_ranges=total_ranges,
        hot_ranges=hot_ranges,
        leaseholder_imbalanced=imbalanced,
        leaseholder_distribution=leaseholders,
        recommendation=self._generate_recommendation(hot_ranges, imbalanced)
    )

def _generate_recommendation(self, hot_ranges, imbalanced) -> str:
    """Generate actionable recommendation based on diagnosis."""
    if hot_ranges and imbalanced:
        return "CRITICAL: Hotspot + leaseholder imbalance. Consider hash-sharded index or scatter ranges."
    elif hot_ranges:
        return "Hotspot detected. Consider splitting the hot range or adding hash-sharded index on primary key."
    elif imbalanced:
        return "Leaseholder imbalance. Ranges may need manual rebalancing or zone config adjustment."
    return "Distribution is healthy. Slowdown may be query-plan related."
```

### Diagnosis Outputs

BrainOps produces a structured diagnosis:

```json
{
  "table": "ollinai.incident_embeddings",
  "total_ranges": 47,
  "hot_ranges": [
    {
      "range_id": 142,
      "lease_holder": 3,
      "range_size_mb": 890,
      "pct_of_total": 38.2,
      "start_key": "/tenant_id=abc123/incident_id=00000",
      "end_key": "/tenant_id=abc123/incident_id=fffff"
    }
  ],
  "leaseholder_imbalanced": true,
  "recommendation": "CRITICAL: Hotspot + leaseholder imbalance. Consider hash-sharded index or scatter ranges."
}
```

---

## Phase 3: EVALUATE — Analyzing Schema Change Storage Risk

**Skill:** `analyzing-schema-change-storage-risk`

Before recommending or executing any fix that involves schema changes (new indexes, altered primary keys), BrainOps evaluates the storage and operational risk.

### Why This Phase Exists

Schema changes in CockroachDB (like adding a vector index) trigger **online backfills** — the database copies and re-indexes data in the background. On large tables this can:
- Consume significant disk space temporarily (up to 2x the table size)
- Generate I/O load that affects concurrent queries
- Take hours on multi-GB tables

BrainOps must evaluate whether the fix is safe before recommending it.

### Implementation

```python
async def evaluate_schema_change_risk(
    self,
    operation: str,
    table: str,
    index_definition: str
) -> SchemaChangeRisk:
    """
    Phase 3: Use analyzing-schema-change-storage-risk skill to
    evaluate whether a proposed fix is safe to apply.
    """
    # Get current table size and index footprint
    size_info = await self.cockroachdb_mcp.execute_sql(f"""
        SELECT
            range_id,
            range_size_mb,
            index_name
        FROM [SHOW RANGES FROM TABLE {table} WITH DETAILS, KEYS, INDEXES]
    """)

    total_size_mb = sum(float(r["range_size_mb"]) for r in size_info)

    # Estimate backfill storage requirement
    # Rule: new index ≈ 30-80% of table size depending on columns indexed
    estimated_backfill_mb = total_size_mb * 0.5  # Conservative estimate

    # Check available disk space via ccloud
    cluster_info = await self.ccloud.cluster_describe()
    available_storage_mb = cluster_info.get("storage_available_mb", float("inf"))

    # Risk assessment
    risk_level = "low"
    if estimated_backfill_mb > available_storage_mb * 0.5:
        risk_level = "critical"  # Would use >50% of available space
    elif estimated_backfill_mb > available_storage_mb * 0.2:
        risk_level = "medium"   # Would use 20-50% of available space
    elif total_size_mb > 10000:  # >10GB table
        risk_level = "medium"   # Large table = long backfill time

    # Estimate time
    estimated_time_min = total_size_mb / 100  # ~100MB/min backfill rate

    return SchemaChangeRisk(
        operation=operation,
        table=table,
        index_definition=index_definition,
        current_table_size_mb=total_size_mb,
        estimated_backfill_mb=estimated_backfill_mb,
        available_storage_mb=available_storage_mb,
        risk_level=risk_level,
        estimated_time_minutes=estimated_time_min,
        safe_to_execute=risk_level in ("low", "medium"),
        recommendation=self._schema_recommendation(risk_level, estimated_time_min)
    )

def _schema_recommendation(self, risk_level: str, time_min: float) -> str:
    if risk_level == "critical":
        return "DO NOT EXECUTE. Insufficient storage for backfill. Scale storage first or archive old data."
    elif risk_level == "medium":
        return f"Safe to execute during low-traffic window. Estimated time: {time_min:.0f} minutes. Recommend operator approval."
    return f"Safe to execute. Low risk. Estimated time: {time_min:.0f} minutes."
```

### Risk Assessment Output

```json
{
  "operation": "CREATE INDEX",
  "table": "ollinai.incident_embeddings",
  "index_definition": "USING vectorsearch (embedding vector_cosine_ops) WITH (lists = 200)",
  "current_table_size_mb": 4200,
  "estimated_backfill_mb": 2100,
  "available_storage_mb": 15000,
  "risk_level": "medium",
  "estimated_time_minutes": 42,
  "safe_to_execute": true,
  "recommendation": "Safe to execute during low-traffic window. Estimated time: 42 minutes. Recommend operator approval."
}
```

---

## Phase 4: RECOMMEND or EXECUTE the Fix

Based on the diagnosis and risk assessment, BrainOps either **recommends** the fix to an operator or **auto-executes** it, depending on the risk level and configured autonomy.

### Decision Matrix

| Risk Level | Table Size | Autonomy Setting | BrainOps Action |
|-----------|-----------|-----------------|-----------------|
| Low | <1GB | `auto` | Execute immediately, log to audit |
| Low | <1GB | `approve` | Recommend to operator, wait for approval |
| Medium | Any | `auto` | Schedule for low-traffic window, alert operator |
| Medium | Any | `approve` | Recommend with full analysis, wait for approval |
| Critical | Any | Any | NEVER auto-execute. Alert operator with urgent flag |

### Fix Categories

BrainOps knows how to apply these categories of fixes:

#### 1. Index Optimization (Most Common)

```sql
-- Problem: Vector search degrading as embeddings grow
-- Fix: Increase vector index lists parameter for better recall/speed tradeoff
DROP INDEX IF EXISTS idx_incident_embedding;
CREATE INDEX idx_incident_embedding ON ollinai.incident_embeddings
    USING vectorsearch (embedding vector_cosine_ops)
    WITH (lists = 200);  -- Increased from 100 for better partitioning at scale
```

#### 2. Hash-Sharded Index (Hotspot Fix)

```sql
-- Problem: Sequential writes causing range hotspot on incidents table
-- Fix: Hash-shard the primary key to scatter writes across ranges
ALTER TABLE ollinai.incidents
    ALTER PRIMARY KEY USING COLUMNS (id)
    WITH (bucket_count = 16);
```

#### 3. Table Partitioning (Tenant Skew)

```sql
-- Problem: One tenant dominates incident volume
-- Fix: Partition by tenant_id for better range distribution
ALTER TABLE ollinai.incidents
    PARTITION BY LIST (tenant_id) (
        PARTITION heavy_tenant VALUES IN ('abc-123-heavy'),
        PARTITION default VALUES IN (DEFAULT)
    );
```

#### 4. TTL Adjustment (Storage Pressure)

```sql
-- Problem: Storage growing faster than expected
-- Fix: Reduce TTL on high-volume tables
ALTER TABLE ollinai.incidents SET (
    ttl_expiration_expression = '((created_at) + INTERVAL ''60 days'')'
    -- Reduced from 90 days to 60 days
);
```

#### 5. Connection Pool Tuning (Checkpoint Contention)

```python
# Problem: Agent checkpoint writes contending with reads
# Fix: Separate connection pools for reads vs writes
WRITE_POOL = create_pool(max_connections=5, application_name="brainops-writer")
READ_POOL = create_pool(max_connections=20, application_name="brainops-reader")
```

### Implementation: The Fix Executor

```python
async def recommend_or_execute_fix(
    self,
    diagnosis: RangeAnalysis,
    risk: SchemaChangeRisk,
    autonomy: str = "approve"  # "auto" or "approve"
) -> FixResult:
    """
    Phase 4: Based on diagnosis and risk, either execute the fix
    or recommend it to the operator.
    """
    # Generate the fix SQL/action
    fix = self._generate_fix(diagnosis, risk)

    # Decision: auto-execute or recommend?
    if risk.risk_level == "critical":
        # NEVER auto-execute critical changes
        return await self._recommend_to_operator(fix, risk, urgent=True)

    if autonomy == "auto" and risk.safe_to_execute:
        if risk.risk_level == "low":
            # Execute immediately
            return await self._execute_fix(fix, risk)
        else:
            # Schedule for low-traffic window
            return await self._schedule_fix(fix, risk)
    else:
        # Recommend to operator
        return await self._recommend_to_operator(fix, risk, urgent=False)


async def _execute_fix(self, fix: FixAction, risk: SchemaChangeRisk) -> FixResult:
    """Execute a low-risk fix immediately."""
    # 1. Write intent to audit log BEFORE execution
    audit_id = await self._write_audit(
        action_type="self_optimization",
        target=fix.table,
        reasoning={
            "diagnosis": fix.diagnosis_summary,
            "risk_level": risk.risk_level,
            "estimated_time": risk.estimated_time_minutes,
            "fix_type": fix.fix_type,
            "sql": fix.sql
        },
        outcome="executing"
    )

    # 2. Verify cluster health via ccloud before executing
    health = await self.ccloud.cluster_describe()
    if health["state"] != "RUNNING":
        await self._update_audit(audit_id, outcome="aborted_unhealthy_cluster")
        return FixResult(success=False, reason="Cluster not healthy")

    # 3. Execute the fix
    try:
        await self.cockroachdb_mcp.execute_sql(fix.sql)
        await self._update_audit(audit_id, outcome="success")
        return FixResult(
            success=True,
            fix_applied=fix.sql,
            estimated_impact=f"Query latency should improve within {risk.estimated_time_minutes:.0f} min"
        )
    except Exception as e:
        await self._update_audit(audit_id, outcome=f"failed: {e}")
        return FixResult(success=False, reason=str(e))


async def _recommend_to_operator(
    self,
    fix: FixAction,
    risk: SchemaChangeRisk,
    urgent: bool
) -> FixResult:
    """Send recommendation to operator via dashboard."""
    recommendation = {
        "type": "memory_optimization",
        "urgent": urgent,
        "summary": f"BrainOps detected {risk.risk_level} performance issue on {fix.table}",
        "diagnosis": fix.diagnosis_summary,
        "proposed_fix": fix.sql,
        "risk_assessment": {
            "level": risk.risk_level,
            "storage_impact_mb": risk.estimated_backfill_mb,
            "estimated_time_min": risk.estimated_time_minutes,
            "safe_to_execute": risk.safe_to_execute
        },
        "operator_actions": [
            "APPROVE — Execute the fix now",
            "SCHEDULE — Execute during next maintenance window",
            "REJECT — Dismiss (BrainOps will re-evaluate next cycle)",
            "MODIFY — Edit the fix SQL before execution"
        ]
    }

    # Push to dashboard via API Gateway
    await self.gateway.create_operator_recommendation(recommendation)

    # Write to audit log
    await self._write_audit(
        action_type="self_optimization_recommendation",
        target=fix.table,
        reasoning=recommendation,
        outcome="awaiting_operator"
    )

    return FixResult(
        success=True,
        awaiting_approval=True,
        recommendation=recommendation
    )
```

---

## Complete Self-Optimization Orchestrator

The full cycle combining all four phases:

```python
class BrainOpsSelfOptimizer:
    """
    Self-optimizing memory layer for BrainOps.
    Uses CockroachDB Agent Skills to monitor and tune its own database.
    """

    MEMORY_TABLES = [
        "ollinai.incidents",
        "ollinai.incident_embeddings",
        "ollinai.resolutions",
        "ollinai.audit_log",
        "ollinai.checkpoints",
        "ollinai.deployments",
        "ollinai.cert_ledger",
    ]

    def __init__(self, cockroachdb_mcp, ccloud, gateway, config):
        self.cockroachdb_mcp = cockroachdb_mcp
        self.ccloud = ccloud
        self.gateway = gateway
        self.config = config  # autonomy level, thresholds, schedule

    async def run_optimization_cycle(self):
        """
        Full self-optimization cycle. Runs every 6 hours (configurable).
        
        Phase 1: DETECT — Find slow queries against memory tables
        Phase 2: DIAGNOSE — Identify root cause (hotspots, imbalance)
        Phase 3: EVALUATE — Assess risk of proposed fix
        Phase 4: FIX — Recommend or execute the fix
        """
        # Phase 1: DETECT
        slow_queries = await self.detect_slow_memory_queries()
        
        if not slow_queries:
            await self._log("Self-optimization: All memory queries healthy.")
            return OptimizationResult(status="healthy", actions_taken=0)

        actions_taken = []

        for query in slow_queries:
            # Identify which table is affected
            table = self._extract_table(query.query_text)
            if table not in self.MEMORY_TABLES:
                continue

            # Phase 2: DIAGNOSE
            diagnosis = await self.diagnose_range_distribution(table)

            if not diagnosis.hot_ranges and not diagnosis.leaseholder_imbalanced:
                # Not a range issue — might be query plan
                await self._log(
                    f"Slow query on {table} is not range-related. "
                    f"May need query rewrite or more resources."
                )
                continue

            # Phase 3: EVALUATE
            fix = self._propose_fix(diagnosis)
            risk = await self.evaluate_schema_change_risk(
                operation=fix.operation,
                table=table,
                index_definition=fix.sql
            )

            # Phase 4: RECOMMEND or EXECUTE
            result = await self.recommend_or_execute_fix(
                diagnosis=diagnosis,
                risk=risk,
                autonomy=self.config.autonomy_level
            )

            actions_taken.append({
                "table": table,
                "diagnosis": diagnosis.recommendation,
                "fix": fix.sql,
                "risk": risk.risk_level,
                "outcome": "executed" if result.success and not result.awaiting_approval
                          else "recommended" if result.awaiting_approval
                          else "failed"
            })

        return OptimizationResult(
            status="optimized" if actions_taken else "healthy",
            actions_taken=len(actions_taken),
            details=actions_taken
        )
```

---

## Dashboard Integration

The operator sees BrainOps self-optimization activity in the TitanOps dashboard:

### Memory Health Panel

```
┌─────────────────────────────────────────────────────┐
│  🧠 BrainOps Memory Health                          │
│                                                      │
│  Vector Search Latency:   ████████░░  42ms (ok)     │
│  Incident Write Latency:  ██████░░░░  28ms (ok)     │
│  Checkpoint Writes:       █████████░  67ms (ok)     │
│  Resolution Lookups:      ██░░░░░░░░  12ms (fast)   │
│                                                      │
│  Last Optimization: 2h ago (no issues found)        │
│  Next Cycle: in 4h                                   │
│  Auto-fixes Applied (30d): 3                        │
│  Recommendations Pending: 1                         │
└─────────────────────────────────────────────────────┘
```

### Pending Recommendation (Operator View)

```
┌─────────────────────────────────────────────────────┐
│  ⚠️  Memory Optimization Recommendation             │
│                                                      │
│  BrainOps detected MEDIUM performance issue          │
│  Table: ollinai.incident_embeddings                  │
│  Issue: Vector search P99 latency > 500ms           │
│  Root Cause: Range hotspot (38% of data in 1 range) │
│                                                      │
│  Proposed Fix:                                       │
│  ┌───────────────────────────────────────────┐      │
│  │ DROP INDEX idx_incident_embedding;         │      │
│  │ CREATE INDEX idx_incident_embedding        │      │
│  │   ON ollinai.incident_embeddings           │      │
│  │   USING vectorsearch (embedding            │      │
│  │     vector_cosine_ops)                     │      │
│  │   WITH (lists = 200);                      │      │
│  └───────────────────────────────────────────┘      │
│                                                      │
│  Risk: MEDIUM | Storage Impact: 2.1GB                │
│  Estimated Time: 42 min                             │
│                                                      │
│  [✅ APPROVE]  [📅 SCHEDULE]  [❌ REJECT]  [✏️ MODIFY]│
└─────────────────────────────────────────────────────┘
```

---

## Why This Is Novel

### What traditional apps do:
- Store data → Wait for DBA to notice problems → Manual investigation → Manual fix

### What typical AI agents do:
- Store data → Hit performance wall → Fail or degrade → Human intervenes

### What BrainOps does:
- Store data → Detect own performance degradation → Diagnose using Agent Skills → Evaluate risk → Auto-fix or recommend → Learn from outcome → Get faster over time

This is **meta-cognitive infrastructure management** — the agent is aware of its own operational health and can take action to maintain it. It's the database equivalent of a self-driving car monitoring its own sensors and recalibrating them while driving.

### Key Insight for Hackathon Judges

> "What makes agentic systems different from traditional apps?"

Traditional apps are passive consumers of infrastructure. Agentic systems should be **active participants** in their own infrastructure management. BrainOps doesn't just use CockroachDB — it understands CockroachDB well enough to maintain it.

This demonstrates:
1. **Deep integration** with CockroachDB tools (not just raw SQL)
2. **Production readiness** (the agent handles its own scaling challenges)
3. **Creativity** (self-optimizing memory is genuinely new in the agentic space)
4. **Understanding of agentic systems** (autonomous maintenance without human intervention for safe operations)

---

## Configuration

```yaml
# BrainOps self-optimization configuration
brainops:
  selfOptimization:
    enabled: true
    schedule: "0 */6 * * *"  # Every 6 hours
    autonomy: "approve"       # "auto" for low-risk, "approve" for all
    thresholds:
      queryLatencyWarningMs: 100
      queryLatencyCriticalMs: 500
      hotspotPercentage: 30
      storageRiskThreshold: 0.5
    autoExecute:
      maxRiskLevel: "low"     # Only auto-execute low-risk fixes
      maxTableSizeMb: 1000    # Only auto-fix tables under 1GB
      requireHealthCheck: true # Always verify cluster health first
```

---

*This document is part of the TitanOps platform specification. BrainOps self-optimization is a Phase 1 hackathon deliverable.*
