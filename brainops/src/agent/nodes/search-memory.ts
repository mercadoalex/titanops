// Search memory node — finds similar incidents via vector search and retrieves playbooks

import type { DbClient } from "../../db/client.js";
import { searchSimilar } from "../../db/queries/embeddings.js";
import { getResolutionByIncident } from "../../db/queries/resolutions.js";
import type {
  AgentStateType,
  ResolutionPlaybook,
  SimilarIncident,
  RemediationAction,
} from "../state.js";

/**
 * Factory that creates the search_memory node with injected DB dependency.
 * Performs vector similarity search and retrieves the resolution playbook for the top match.
 */
export function createSearchMemoryNode(db: DbClient) {
  return async (state: AgentStateType): Promise<Partial<AgentStateType>> => {
    const { incident, tenantId } = state;

    if (!incident) {
      throw new Error("search_memory node: no incident in state");
    }

    // Placeholder embedding — actual embedding comes from the embedding pipeline (task 6)
    const placeholderEmbedding = new Array<number>(1536).fill(0);

    // Search for similar past incidents using vector similarity
    const rawSimilar = await db.withTenant(tenantId, (client) =>
      searchSimilar(client, {
        embedding: placeholderEmbedding,
        limit: 5,
        minSimilarity: 0.7,
      }),
    );

    const similarIncidents: SimilarIncident[] = rawSimilar.map((row) => ({
      incidentId: row.incidentId,
      similarity: row.similarity,
      narrative: "",
      modules: [],
    }));

    // Retrieve the resolution playbook for the most similar incident (if any)
    let playbook: ResolutionPlaybook | null = null;

    if (similarIncidents.length > 0) {
      const topMatch = similarIncidents[0];
      const resolutions = await db.withTenant(tenantId, (client) =>
        getResolutionByIncident(client, topMatch.incidentId),
      );

      if (resolutions.length > 0) {
        const res = resolutions[0];
        playbook = {
          id: res.id,
          incidentId: res.incidentId,
          actionSequence: res.actionSequence as RemediationAction[],
          success: res.success,
          durationMs: res.durationMs,
          reuseCount: res.reuseCount,
        };
      }
    }

    return {
      similarIncidents,
      playbook,
      currentStep: "reason",
    };
  };
}
