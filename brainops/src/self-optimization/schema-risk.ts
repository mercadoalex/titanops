// Schema change risk evaluation

import type { DbClient } from "../db/client.js";

export interface SchemaChangeRisk {
  tableSizeMb: number;
  estimatedBackfillMb: number;
  riskLevel: "low" | "medium" | "critical";
  estimatedTimeMinutes: number;
  safeToExecute: boolean;
}

/**
 * Evaluates the risk of schema changes (e.g., index creation) on CockroachDB tables.
 */
export class SchemaRiskEvaluator {
  private readonly db: DbClient;

  constructor(db: DbClient) {
    this.db = db;
  }

  /**
   * Estimates the storage impact and risk of creating an index on a table.
   * Uses table statistics to approximate backfill cost.
   */
  async evaluateIndexCreation(
    tenantId: string,
    table: string,
  ): Promise<SchemaChangeRisk> {
    return this.db.withTenant(tenantId, async (client) => {
      // Get table size from range statistics
      const sizeResult = await client.query(
        `SELECT
           COALESCE(SUM((range_size_mb)::float), 0) AS total_size_mb
         FROM [SHOW RANGES FROM TABLE ollinai.${table}]`,
      );

      const tableSizeMb = parseFloat(
        (sizeResult.rows[0]?.total_size_mb as string) ?? "0",
      );

      // Estimated backfill is ~30% of table size for a secondary index
      const estimatedBackfillMb = tableSizeMb * 0.3;

      // Estimate time: ~50 MB/min for online schema changes in CockroachDB
      const estimatedTimeMinutes = Math.max(
        1,
        Math.ceil(estimatedBackfillMb / 50),
      );

      // Determine risk level based on table size and estimated time
      const riskLevel = determineRiskLevel(tableSizeMb, estimatedTimeMinutes);

      // Safe to auto-execute only if risk is low
      const safeToExecute = riskLevel === "low";

      return {
        tableSizeMb,
        estimatedBackfillMb,
        riskLevel,
        estimatedTimeMinutes,
        safeToExecute,
      };
    });
  }
}

function determineRiskLevel(
  tableSizeMb: number,
  estimatedTimeMinutes: number,
): "low" | "medium" | "critical" {
  // Critical: >10GB table or >60 min estimated
  if (tableSizeMb > 10_000 || estimatedTimeMinutes > 60) {
    return "critical";
  }

  // Medium: >1GB table or >10 min estimated
  if (tableSizeMb > 1_000 || estimatedTimeMinutes > 10) {
    return "medium";
  }

  return "low";
}
