// Incident query methods — parameterized queries for ollinai.incidents

import pg from "pg";

type PoolClient = pg.PoolClient;

export interface Incident {
  id?: string;
  tenantId: string;
  correlationId?: string;
  modules: string[];
  severity: number;
  narrative: string;
  contributingEvents: unknown;
  confidenceScore: number;
  resolutionAction?: string;
  resolutionTimeMs?: number;
  nodeId?: string;
  namespace?: string;
  podName?: string;
  createdAt?: Date;
  resolvedAt?: Date;
}

export interface QueryIncidentsFilter {
  nodeId?: string;
  namespace?: string;
  days?: number;
  limit?: number;
}

/**
 * Inserts a new incident into ollinai.incidents. Returns the generated UUID.
 */
export async function insertIncident(
  client: PoolClient,
  incident: Omit<Incident, "id" | "createdAt">,
): Promise<string> {
  const result = await client.query<{ id: string }>(
    `INSERT INTO ollinai.incidents (
      tenant_id, correlation_id, modules, severity, narrative,
      contributing_events, confidence_score, resolution_action,
      resolution_time_ms, node_id, namespace, pod_name, resolved_at
    ) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13)
    RETURNING id`,
    [
      incident.tenantId,
      incident.correlationId ?? null,
      incident.modules,
      incident.severity,
      incident.narrative,
      JSON.stringify(incident.contributingEvents),
      incident.confidenceScore,
      incident.resolutionAction ?? null,
      incident.resolutionTimeMs ?? null,
      incident.nodeId ?? null,
      incident.namespace ?? null,
      incident.podName ?? null,
      incident.resolvedAt ?? null,
    ],
  );
  return result.rows[0].id;
}

/**
 * Queries incidents with optional filters: nodeId, namespace, days lookback, and limit.
 */
export async function queryIncidents(
  client: PoolClient,
  filter: QueryIncidentsFilter = {},
): Promise<Incident[]> {
  const conditions: string[] = [];
  const params: unknown[] = [];
  let paramIdx = 1;

  if (filter.nodeId) {
    conditions.push(`node_id = $${paramIdx++}`);
    params.push(filter.nodeId);
  }

  if (filter.namespace) {
    conditions.push(`namespace = $${paramIdx++}`);
    params.push(filter.namespace);
  }

  if (filter.days) {
    conditions.push(`created_at >= now() - $${paramIdx++}::INTERVAL`);
    params.push(`${filter.days} days`);
  }

  const whereClause = conditions.length > 0 ? `WHERE ${conditions.join(" AND ")}` : "";
  const limit = filter.limit ?? 50;

  const result = await client.query(
    `SELECT id, tenant_id, correlation_id, modules, severity, narrative,
            contributing_events, confidence_score, resolution_action,
            resolution_time_ms, node_id, namespace, pod_name, created_at, resolved_at
     FROM ollinai.incidents
     ${whereClause}
     ORDER BY created_at DESC
     LIMIT $${paramIdx}`,
    [...params, limit],
  );

  return result.rows.map(mapRowToIncident);
}

/**
 * Retrieves a single incident by ID.
 */
export async function getIncidentById(
  client: PoolClient,
  id: string,
): Promise<Incident | null> {
  const result = await client.query(
    `SELECT id, tenant_id, correlation_id, modules, severity, narrative,
            contributing_events, confidence_score, resolution_action,
            resolution_time_ms, node_id, namespace, pod_name, created_at, resolved_at
     FROM ollinai.incidents
     WHERE id = $1`,
    [id],
  );

  if (result.rows.length === 0) return null;
  return mapRowToIncident(result.rows[0]);
}

function mapRowToIncident(row: Record<string, unknown>): Incident {
  return {
    id: row.id as string,
    tenantId: row.tenant_id as string,
    correlationId: row.correlation_id as string | undefined,
    modules: row.modules as string[],
    severity: row.severity as number,
    narrative: row.narrative as string,
    contributingEvents: row.contributing_events,
    confidenceScore: row.confidence_score as number,
    resolutionAction: row.resolution_action as string | undefined,
    resolutionTimeMs: row.resolution_time_ms as number | undefined,
    nodeId: row.node_id as string | undefined,
    namespace: row.namespace as string | undefined,
    podName: row.pod_name as string | undefined,
    createdAt: row.created_at as Date | undefined,
    resolvedAt: row.resolved_at as Date | undefined,
  };
}
