// get_node_history tool — combined incident + audit query for a node

import type { DbClient } from "../../db/client.js";
import { queryIncidents, type Incident } from "../../db/queries/incidents.js";
import { queryAuditLog, type AuditEntry } from "../../db/queries/audit.js";

export interface GetNodeHistoryInput {
  nodeId: string;
  days?: number;
}

export interface NodeHistoryEntry {
  type: "incident" | "audit";
  timestamp: Date;
  data: Incident | AuditEntry;
}

export interface GetNodeHistoryResult {
  history: NodeHistoryEntry[];
}

/**
 * Handles the get_node_history MCP tool.
 * Queries both incidents and audit entries for the given node,
 * returning a combined history sorted by time (most recent first).
 */
export async function handleGetNodeHistory(
  db: DbClient,
  tenantId: string,
  input: GetNodeHistoryInput,
): Promise<GetNodeHistoryResult> {
  const days = input.days ?? 30;

  const [incidents, auditEntries] = await Promise.all([
    db.withTenant(tenantId, (client) =>
      queryIncidents(client, {
        nodeId: input.nodeId,
        days,
      }),
    ),
    db.withTenant(tenantId, (client) =>
      queryAuditLog(client, {
        tenantId,
        days,
      }),
    ),
  ]);

  // Filter audit entries to those targeting this node
  const nodeAuditEntries = auditEntries.filter(
    (entry) => entry.target.includes(input.nodeId),
  );

  const history: NodeHistoryEntry[] = [
    ...incidents.map((incident) => ({
      type: "incident" as const,
      timestamp: incident.createdAt ?? new Date(0),
      data: incident,
    })),
    ...nodeAuditEntries.map((entry) => ({
      type: "audit" as const,
      timestamp: entry.createdAt,
      data: entry,
    })),
  ];

  // Sort by timestamp descending (most recent first)
  history.sort((a, b) => b.timestamp.getTime() - a.timestamp.getTime());

  return { history };
}
