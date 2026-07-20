// get_resolution_playbook tool — resolution lookup by incident

import type { DbClient } from "../../db/client.js";
import { getResolutionByIncident, type Resolution } from "../../db/queries/resolutions.js";

export interface GetPlaybookInput {
  incidentId: string;
}

export interface GetPlaybookResult {
  resolutions: Resolution[];
}

/**
 * Handles the get_resolution_playbook MCP tool.
 * Returns all resolution playbooks associated with the given incident.
 */
export async function handleGetPlaybook(
  db: DbClient,
  tenantId: string,
  input: GetPlaybookInput,
): Promise<GetPlaybookResult> {
  const resolutions = await db.withTenant(tenantId, (client) =>
    getResolutionByIncident(client, input.incidentId),
  );

  return { resolutions };
}
