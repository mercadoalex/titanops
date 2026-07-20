// High-risk action approval gate

/**
 * Determines if an action is high-risk and requires human approval.
 * Returns true for destructive actions targeting production environments.
 */
export function isHighRisk(actionType: string, target: string): boolean {
  const destructiveActions = ["node_drain", "node_delete", "pod_delete"];
  const targetLower = target.toLowerCase();

  return (
    destructiveActions.includes(actionType) &&
    (targetLower.includes("production") || targetLower.includes("prod"))
  );
}

/**
 * Represents a pending approval request for a high-risk action.
 */
export interface PendingApproval {
  id: string;
  tenantId: string;
  actionType: string;
  target: string;
  reason: string;
  requestedAt: Date;
}

/**
 * Manages pending approval requests for high-risk remediation actions.
 * In-memory store — production would back this with CockroachDB.
 */
export class ApprovalGate {
  private pending: Map<string, PendingApproval> = new Map();

  /**
   * Stores a pending approval request.
   */
  requestApproval(action: PendingApproval): void {
    this.pending.set(action.id, action);
  }

  /**
   * Returns all pending approvals for a given tenant.
   */
  getPending(tenantId: string): PendingApproval[] {
    const results: PendingApproval[] = [];
    for (const approval of this.pending.values()) {
      if (approval.tenantId === tenantId) {
        results.push(approval);
      }
    }
    return results;
  }

  /**
   * Approves a pending action (removes from pending queue).
   */
  approve(id: string): void {
    this.pending.delete(id);
  }

  /**
   * Rejects a pending action (removes from pending queue).
   */
  reject(id: string): void {
    this.pending.delete(id);
  }
}
