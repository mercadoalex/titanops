// Self-optimization orchestrator — full detect → diagnose → evaluate → recommend pipeline

import type { DbClient } from "../db/client.js";
import type { QueryProfiler, SlowQuery } from "./profiler.js";
import type { RangeAnalyzer } from "./range-analyzer.js";
import type { SchemaRiskEvaluator } from "./schema-risk.js";

export interface OptimizationConfig {
  queryLatencyThresholdMs: number;
  autoExecuteRiskLevel: "low";
}

export interface OptimizationAction {
  table: string;
  diagnosis: string;
  proposedFix: string;
  riskLevel: "low" | "medium" | "critical";
  outcome: "auto_executed" | "recommended" | "skipped";
}

export interface OptimizationResult {
  status: "healthy" | "optimized" | "recommended";
  actionsCount: number;
  details: OptimizationAction[];
}

/**
 * Orchestrates the full self-optimization cycle:
 * 1. Profile — find slow queries
 * 2. Diagnose — analyze range distribution for affected tables
 * 3. Evaluate — assess risk of proposed schema changes
 * 4. Recommend or auto-execute — based on risk level
 */
export class SelfOptimizationOrchestrator {
  private readonly profiler: QueryProfiler;
  private readonly rangeAnalyzer: RangeAnalyzer;
  private readonly schemaRisk: SchemaRiskEvaluator;
  readonly db: DbClient;
  private readonly config: OptimizationConfig;

  constructor(
    profiler: QueryProfiler,
    rangeAnalyzer: RangeAnalyzer,
    schemaRisk: SchemaRiskEvaluator,
    db: DbClient,
    config: OptimizationConfig,
  ) {
    this.profiler = profiler;
    this.rangeAnalyzer = rangeAnalyzer;
    this.schemaRisk = schemaRisk;
    this.db = db;
    this.config = config;
  }

  /**
   * Runs a full optimization cycle for a tenant.
   * Detects slow queries, diagnoses range issues, evaluates risk, and recommends actions.
   */
  async runCycle(tenantId: string): Promise<OptimizationResult> {
    // 1. Profile: find slow queries
    const slowQueries = await this.profiler.getSlowQueries(
      tenantId,
      this.config.queryLatencyThresholdMs,
    );

    if (slowQueries.length === 0) {
      return { status: "healthy", actionsCount: 0, details: [] };
    }

    // 2. Extract affected tables from slow queries
    const affectedTables = extractTablesFromQueries(slowQueries);

    // 3. Diagnose + Evaluate for each table
    const actions: OptimizationAction[] = [];

    for (const table of affectedTables) {
      // Diagnose range distribution
      const rangeAnalysis = await this.rangeAnalyzer.analyzeTable(
        tenantId,
        table,
      );

      // Evaluate schema change risk
      const risk = await this.schemaRisk.evaluateIndexCreation(
        tenantId,
        table,
      );

      // Determine diagnosis
      let diagnosis: string;
      let proposedFix: string;

      if (rangeAnalysis.hotRanges.length > 0) {
        diagnosis = `Hot ranges detected: ${rangeAnalysis.hotRanges.length} range(s) hold >30% of data`;
        proposedFix = "ALTER TABLE ... SPLIT AT to redistribute ranges";
      } else if (rangeAnalysis.leaseholderImbalanced) {
        diagnosis = "Leaseholder imbalance — single node holds >50% of leases";
        proposedFix = "ALTER TABLE ... SCATTER to rebalance leaseholders";
      } else {
        diagnosis = "High query latency without obvious range issues";
        proposedFix = "CREATE INDEX on frequently queried columns";
      }

      // Determine outcome based on risk
      let outcome: OptimizationAction["outcome"];
      if (risk.safeToExecute) {
        outcome = "auto_executed";
      } else if (risk.riskLevel === "critical") {
        outcome = "skipped";
      } else {
        outcome = "recommended";
      }

      actions.push({
        table,
        diagnosis,
        proposedFix,
        riskLevel: risk.riskLevel,
        outcome,
      });
    }

    // Determine overall status
    const hasAutoExecuted = actions.some((a) => a.outcome === "auto_executed");
    const hasRecommended = actions.some(
      (a) => a.outcome === "recommended" || a.outcome === "skipped",
    );

    let status: OptimizationResult["status"];
    if (hasAutoExecuted && !hasRecommended) {
      status = "optimized";
    } else if (hasRecommended || hasAutoExecuted) {
      status = "recommended";
    } else {
      status = "healthy";
    }

    return {
      status,
      actionsCount: actions.length,
      details: actions,
    };
  }
}

/**
 * Extracts unique table names from slow query fingerprints.
 * Looks for ollinai.<table_name> patterns in query text.
 */
function extractTablesFromQueries(queries: SlowQuery[]): string[] {
  const tables = new Set<string>();
  const tablePattern = /ollinai\.(\w+)/g;

  for (const query of queries) {
    let match: RegExpExecArray | null;
    while ((match = tablePattern.exec(query.queryText)) !== null) {
      tables.add(match[1]);
    }
  }

  return [...tables];
}
