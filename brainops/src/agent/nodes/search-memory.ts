// Search memory node — finds similar incidents via vector search and retrieves playbooks

import type { AgentStore } from "../ports.js";
import type {
  AgentStateType,
  ResolutionPlaybook,
  SimilarIncident,
  RemediationAction,
} from "../state.js";

/**
 * Factory that creates the search_memory node with injected store dependency.
 * Performs vector similarity search and retrieves the resolution playbook for the top match.
 */
export function createSearchMemoryNode(store: AgentStore) {
  return async (state: AgentStateType): Promise<Partial<AgentStateType>> => {
    const { incident, tenantId } = state;

    if (!incident) {
      throw new Error("search_memory node: no incident in state");
    }

    // Placeholder embedding — actual embedding comes from the embedding pipeline (task 6)
    const placeholderEmbedding = new Array<number>(1536).fill(0);

    // Search for similar past incidents using vector similarity
    const similarIncidents: SimilarIncident[] = await store.searchSimilar({
      tenantId,
      embedding: placeholderEmbedding,
      limit: 5,
      minSimilarity: 0.7,
    });

    // Retrieve the resolution playbook for the most similar incident (if any)
    let playbook: ResolutionPlaybook | null = null;

    if (similarIncidents.length > 0) {
      const topMatch = similarIncidents[0];
      const resolutions = await store.getResolutionsByIncident(
        tenantId,
        topMatch.incidentId,
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
