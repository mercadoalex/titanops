// store_resolution tool — writes resolution record

import type { DbClient } from "../../db/client.js";
import { insertResolution } from "../../db/queries/resolutions.js";

export interface StoreResolutionInput {
  incidentId: string;
  actions: unknown[];
  success: boolean;
  durationMs: number;
}

export interface StoreResolutionResult {
  resolutionId: string;
}

/**
 * Handles the store_resolution MCP tool.
 * Stores a resolution record for the given incident and returns the generated ID.
 */
export async function handleStoreResolution(
  db: DbClient,
  tenantId: string,
  input: StoreResolutionInput,
): Promise<StoreResolutionResult> {
  const resolutionId = await db.withTenant(tenantId, (client) =>
    insertResolution(client, {
      tenantId,
      incidentId: input.incidentId,
      actionSequence: input.actions,
      success: input.success,
      durationMs: input.durationMs,
    }),
  );

  return { resolutionId };
}
