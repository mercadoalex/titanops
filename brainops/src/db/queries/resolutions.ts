// Resolution query methods — parameterized queries for ollinai.resolutions

import pg from "pg";

type PoolClient = pg.PoolClient;

export interface ResolutionInput {
  incidentId: string;
  tenantId: string;
  actionSequence: unknown;
  success: boolean;
  durationMs: number;
  context?: unknown;
}

export interface Resolution {
  id: string;
  tenantId: string;
  incidentId: string;
  actionSequence: unknown;
  success: boolean;
  durationMs: number;
  context: unknown | null;
  reuseCount: number;
  createdAt: Date;
}

/**
 * Inserts a new resolution record. Returns the generated UUID.
 */
export async function insertResolution(
  client: PoolClient,
  input: ResolutionInput,
): Promise<string> {
  const result = await client.query<{ id: string }>(
    `INSERT INTO ollinai.resolutions (
      tenant_id, incident_id, action_sequence, success, duration_ms, context
    ) VALUES ($1, $2, $3, $4, $5, $6)
    RETURNING id`,
    [
      input.tenantId,
      input.incidentId,
      JSON.stringify(input.actionSequence),
      input.success,
      input.durationMs,
      input.context ? JSON.stringify(input.context) : null,
    ],
  );
  return result.rows[0].id;
}

/**
 * Queries resolutions for a given incident, ordered by most recent first.
 */
export async function getResolutionByIncident(
  client: PoolClient,
  incidentId: string,
): Promise<Resolution[]> {
  const result = await client.query(
    `SELECT id, tenant_id, incident_id, action_sequence, success,
            duration_ms, context, reuse_count, created_at
     FROM ollinai.resolutions
     WHERE incident_id = $1
     ORDER BY created_at DESC`,
    [incidentId],
  );

  return result.rows.map(mapRowToResolution);
}

/**
 * Increments the reuse_count for a resolution (called when a playbook is re-applied).
 */
export async function incrementReuseCount(
  client: PoolClient,
  resolutionId: string,
): Promise<void> {
  await client.query(
    `UPDATE ollinai.resolutions
     SET reuse_count = reuse_count + 1
     WHERE id = $1`,
    [resolutionId],
  );
}

function mapRowToResolution(row: Record<string, unknown>): Resolution {
  return {
    id: row.id as string,
    tenantId: row.tenant_id as string,
    incidentId: row.incident_id as string,
    actionSequence: row.action_sequence,
    success: row.success as boolean,
    durationMs: row.duration_ms as number,
    context: row.context ?? null,
    reuseCount: row.reuse_count as number,
    createdAt: row.created_at as Date,
  };
}
