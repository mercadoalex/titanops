// Reason node — builds prompt context and produces a ReasoningResult via LLM

import type { AgentStore } from "../ports.js";
import type { AgentStateType, ReasoningResult, RemediationAction } from "../state.js";

/**
 * Factory that creates the reason node with injected store dependency.
 * Builds a prompt from the incident, similar incidents, and playbook context,
 * then calls the LLM to produce a ReasoningResult.
 *
 * NOTE: LLM integration is a placeholder — returns a mock result for now.
 */
export function createReasonNode(_store: AgentStore) {
  return async (state: AgentStateType): Promise<Partial<AgentStateType>> => {
    const { incident, similarIncidents, playbook } = state;

    if (!incident) {
      throw new Error("reason node: no incident in state");
    }

    // Build context prompt (will be sent to Bedrock in a later task)
    const _promptParts: string[] = [
      `Incident: ${incident.narrative} (severity=${incident.severity})`,
      `Modules: ${incident.modules.join(", ")}`,
    ];

    if (similarIncidents.length > 0) {
      _promptParts.push(
        `Similar incidents: ${similarIncidents.map((s) => `${s.incidentId} (sim=${s.similarity.toFixed(2)})`).join(", ")}`,
      );
    }

    if (playbook) {
      _promptParts.push(
        `Playbook available: ${playbook.id} (success=${playbook.success}, reuse=${playbook.reuseCount})`,
      );
    }

    // --- Placeholder LLM call ---
    // In production this sends _promptParts to Bedrock and parses the structured response.
    const reasoning: ReasoningResult = {
      observation: `Received incident with severity ${incident.severity} affecting ${incident.modules.join(", ")}`,
      analysis: playbook
        ? `Found existing playbook ${playbook.id} with ${playbook.reuseCount} prior uses`
        : "No prior playbook found; recommending conservative action",
      selectedAction: playbook ? "reuse_playbook" : "pod_restart",
      alternatives: ["node_cordon", "scale_up"],
      confidence: playbook ? 0.85 : 0.6,
      memoryContext: similarIncidents.length > 0
        ? `Based on ${similarIncidents.length} similar incidents`
        : "No prior memory available",
    };

    // Derive the action from reasoning
    const action: RemediationAction = {
      actionType: reasoning.selectedAction,
      target: incident.podName ?? incident.nodeId ?? "unknown",
      reason: reasoning.analysis,
      riskLevel: reasoning.confidence >= 0.8 ? "low" : "medium",
    };

    return {
      reasoning,
      action,
      currentStep: "act",
    };
  };
}
