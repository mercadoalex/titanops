// query_incident_history tool — filtered incident query

import type { DbClient } from "../../db/client.js";
import { queryIncidents, type Incident } from "../../db/queries/incidents.js";

export interface QueryHistoryInput {
  nodeId?: string;
  namespace?: string;
  timeRangeDays?: number;
  limit?: number;
}

export interface QueryHistoryResult {
  incidents: Incident[];
}

/**
 * Handles the query_incident_history MCP tool.
 * Queries incidents filtered by nodeId, namespace, time range, and limit.
 */
export async function handleQueryHistory(
  db: DbClient,
  tenantId: string,
  input: QueryHistoryInput,
): Promise<QueryHistoryResult> {
  const incidents = await db.withTenant(tenantId, (client) =>
    queryIncidents(client, {
      nodeId: input.nodeId,
      namespace: input.namespace,
      days: input.timeRangeDays,
      limit: input.limit,
    }),
  );

  return { incidents };
}
