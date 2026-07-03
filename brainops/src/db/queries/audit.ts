// Audit log query methods — parameterized queries for ollinai.audit_log

import pg from "pg";

type PoolClient = pg.PoolClient;

export interface AuditEntryInput {
  tenantId: string;
  module: string;
  actionType: string;
  target: string;
  triggerEventId?: string;
  confidence: number;
  reasoning: unknown;
  outcome: string;
  operatorId?: string;
  durationMs?: number;
}

export interface AuditEntry {
  id: string;
  tenantId: string;
  module: string;
  actionType: string;
  target: string;
  triggerEventId: string | null;
  confidence: number;
  reasoning: unknown;
  outcome: string;
  operatorId: string | null;
  durationMs: number | null;
  createdAt: Date;
}

export interface QueryAuditLogFilter {
  tenantId: string;
  module?: string;
  days?: number;
  limit?: number;
}

/**
 * Inserts a new audit log entry. Returns the generated UUID.
 */
export async function insertAuditEntry(
  client: PoolClient,
  input: AuditEntryInput,
): Promise<string> {
  const result = await client.query<{ id: string }>(
    `INSERT INTO ollinai.audit_log (
      tenant_id, module, action_type, target, trigger_event_id,
      confidence, reasoning, outcome, operator_id, duration_ms
    ) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
    RETURNING id`,
    [
      input.tenantId,
      input.module,
      input.actionType,
      input.target,
      input.triggerEventId ?? null,
      input.confidence,
      JSON.stringify(input.reasoning),
      input.outcome,
      input.operatorId ?? null,
      input.durationMs ?? null,
    ],
  );
  return result.rows[0].id;
}

/**
 * Queries the audit log with optional filters: module, days lookback, and limit.
 * Always scoped to a specific tenant.
 */
export async function queryAuditLog(
  client: PoolClient,
  filter: QueryAuditLogFilter,
): Promise<AuditEntry[]> {
  const conditions: string[] = ["tenant_id = $1"];
  const params: unknown[] = [filter.tenantId];
  let paramIdx = 2;

  if (filter.module) {
    conditions.push(`module = $${paramIdx++}`);
    params.push(filter.module);
  }

  if (filter.days) {
    conditions.push(`created_at >= now() - $${paramIdx++}::INTERVAL`);
    params.push(`${filter.days} days`);
  }

  const limit = filter.limit ?? 100;
  const whereClause = conditions.join(" AND ");

  const result = await client.query(
    `SELECT id, tenant_id, module, action_type, target, trigger_event_id,
            confidence, reasoning, outcome, operator_id, duration_ms, created_at
     FROM ollinai.audit_log
     WHERE ${whereClause}
     ORDER BY created_at DESC
     LIMIT $${paramIdx}`,
    [...params, limit],
  );

  return result.rows.map(mapRowToAuditEntry);
}

function mapRowToAuditEntry(row: Record<string, unknown>): AuditEntry {
  return {
    id: row.id as string,
    tenantId: row.tenant_id as string,
    module: row.module as string,
    actionType: row.action_type as string,
    target: row.target as string,
    triggerEventId: (row.trigger_event_id as string) ?? null,
    confidence: row.confidence as number,
    reasoning: row.reasoning,
    outcome: row.outcome as string,
    operatorId: (row.operator_id as string) ?? null,
    durationMs: (row.duration_ms as number) ?? null,
    createdAt: row.created_at as Date,
  };
}
