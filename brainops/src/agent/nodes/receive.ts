// Receive node — stores incident to DB and transitions to search_memory

import type { DbClient } from "../../db/client.js";
import { insertIncident } from "../../db/queries/incidents.js";
import type { AgentStateType } from "../state.js";

/**
 * Factory that creates the receive node function with injected DB dependency.
 * The receive node persists the incoming correlated incident and moves to search_memory.
 */
export function createReceiveNode(db: DbClient) {
  return async (state: AgentStateType): Promise<Partial<AgentStateType>> => {
    const { incident, tenantId } = state;

    if (!incident) {
      throw new Error("receive node: no incident in state");
    }

    // Persist the incident to CockroachDB (tenant-scoped via RLS)
    const incidentId = await db.withTenant(tenantId, (client) =>
      insertIncident(client, {
        tenantId,
        correlationId: incident.id,
        modules: incident.modules,
        severity: incident.severity,
        narrative: incident.narrative,
        contributingEvents: incident.contributingEvents,
        confidenceScore: incident.confidenceScore,
        nodeId: incident.nodeId,
        namespace: incident.namespace,
        podName: incident.podName,
      }),
    );

    // Return updated state — embedding generation is wired in a later task
    return {
      incident: { ...incident, id: incidentId },
      currentStep: "search_memory",
    };
  };
}
