// execute_remediation tool — K8s action + audit write + approval gate

import type { DbClient } from "../../db/client.js";
import { insertAuditEntry } from "../../db/queries/audit.js";

export interface ExecuteRemediationInput {
  actionType: string;
  target: string;
  reason: string;
}

export interface ExecuteRemediationResult {
  status: "executed" | "awaiting_approval";
  auditId?: string;
}

/** Actions that always require human approval. */
const HIGH_RISK_ACTIONS = new Set(["node_drain"]);

/**
 * Determines if an action is high-risk based on action type and target.
 * - "node_drain" is always high-risk
 * - "pod_delete" is high-risk when the target contains "production"
 */
function isHighRisk(actionType: string, target: string): boolean {
  if (HIGH_RISK_ACTIONS.has(actionType)) return true;
  if (actionType === "pod_delete" && target.toLowerCase().includes("production")) return true;
  return false;
}

/**
 * Handles the execute_remediation MCP tool.
 * If the action is high-risk, returns awaiting_approval without executing.
 * Otherwise, logs to audit trail and returns executed.
 */
export async function handleExecuteRemediation(
  db: DbClient,
  tenantId: string,
  input: ExecuteRemediationInput,
): Promise<ExecuteRemediationResult> {
  if (isHighRisk(input.actionType, input.target)) {
    return { status: "awaiting_approval" };
  }

  const auditId = await db.withTenant(tenantId, (client) =>
    insertAuditEntry(client, {
      tenantId,
      module: "brainops-mcp",
      actionType: input.actionType,
      target: input.target,
      confidence: 1.0,
      reasoning: { reason: input.reason },
      outcome: "executed",
    }),
  );

  return { status: "executed", auditId };
}
