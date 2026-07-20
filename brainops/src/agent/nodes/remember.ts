// Remember node — stores resolution playbook for future reuse

import type { AgentStore } from "../ports.js";
import type { AgentStateType } from "../state.js";

/**
 * Factory that creates the remember node with injected store dependency.
 * Stores the resolution as a playbook or increments the reuse count on an existing one.
 */
export function createRememberNode(store: AgentStore) {
  return async (state: AgentStateType): Promise<Partial<AgentStateType>> => {
    const { incident, action, outcome, playbook, tenantId } = state;

    if (!incident) {
      throw new Error("remember node: no incident in state");
    }

    if (!action || !outcome) {
      throw new Error("remember node: no action/outcome in state");
    }

    if (playbook) {
      // Reusing an existing playbook — increment its reuse count
      await store.incrementReuseCount(tenantId, playbook.id);
    } else {
      // Store a new resolution playbook
      await store.insertResolution({
        incidentId: incident.id,
        tenantId,
        actionSequence: [action],
        success: outcome.success,
        durationMs: outcome.durationMs,
      });
    }

    return {
      currentStep: "complete",
    };
  };
}
