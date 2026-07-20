// Act node — executes remediation or gates on approval for high-risk actions

import type { AgentStore } from "../ports.js";
import type { AgentStateType, ActionOutcome } from "../state.js";

/**
 * Factory that creates the act node with injected store dependency.
 * If the action is high-risk, transitions to awaiting_approval.
 * Otherwise executes the remediation and logs to the audit trail.
 */
export function createActNode(store: AgentStore) {
  return async (state: AgentStateType): Promise<Partial<AgentStateType>> => {
    const { action, reasoning, incident, tenantId } = state;

    if (!action) {
      throw new Error("act node: no action in state");
    }

    if (!incident) {
      throw new Error("act node: no incident in state");
    }

    // High-risk actions require human approval
    if (action.riskLevel === "high") {
      return {
        currentStep: "awaiting_approval",
      };
    }

    // --- Placeholder remediation execution ---
    // In production this dispatches to the appropriate remediation handler
    const startTime = Date.now();
    // Simulated execution — real implementation calls K8s API, cert managers, etc.
    const outcome: ActionOutcome = {
      success: true,
      durationMs: Date.now() - startTime,
    };

    // Write to audit log via store interface
    await store.insertAudit({
      tenantId,
      module: "brainops-agent",
      actionType: action.actionType,
      target: action.target,
      triggerEventId: incident.id,
      confidence: reasoning?.confidence ?? 0,
      reasoning: reasoning ?? {},
      outcome: outcome.success ? "success" : "failure",
      durationMs: outcome.durationMs,
    });

    return {
      outcome,
      currentStep: "remember",
    };
  };
}
