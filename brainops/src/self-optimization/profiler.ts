// Statement fingerprint analysis

import type { DbClient } from "../db/client.js";

export interface SlowQuery {
  fingerprint: string;
  queryText: string;
  meanLatencyMs: number;
  executionCount: number;
}

/**
 * Profiles CockroachDB query performance by analyzing statement statistics.
 * Focuses on queries targeting ollinai.* tables.
 */
export class QueryProfiler {
  private readonly db: DbClient;

  constructor(db: DbClient) {
    this.db = db;
  }

  /**
   * Finds queries with mean latency above the given threshold.
   * Queries crdb_internal.statement_statistics for ollinai.* table access.
   */
  async getSlowQueries(
    tenantId: string,
    thresholdMs: number,
  ): Promise<SlowQuery[]> {
    return this.db.withTenant(tenantId, async (client) => {
      const result = await client.query(
        `SELECT
           fingerprint_id AS fingerprint,
           metadata->>'query' AS query_text,
           (statistics->'execution_statistics'->'cnt')::float AS execution_count,
           (statistics->'statistics'->'runLat'->'mean')::float * 1000 AS mean_latency_ms
         FROM crdb_internal.statement_statistics
         WHERE metadata->>'query' LIKE '%ollinai.%'
           AND (statistics->'statistics'->'runLat'->'mean')::float * 1000 > $1
         ORDER BY mean_latency_ms DESC`,
        [thresholdMs],
      );

      return result.rows.map((row) => ({
        fingerprint: row.fingerprint as string,
        queryText: row.query_text as string,
        meanLatencyMs: parseFloat(row.mean_latency_ms as string),
        executionCount: parseInt(row.execution_count as string, 10),
      }));
    });
  }
}
