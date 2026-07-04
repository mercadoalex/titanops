import { describe, it, expect, vi } from "vitest";
import type { DbClient } from "../src/db/client.js";
import type { AgentStateType, CorrelatedIncident } from "../src/agent/state.js";
import { createReceiveNode } from "../src/agent/nodes/receive.js";
import { createSearchMemoryNode } from "../src/agent/nodes/search-memory.js";
import { createReasonNode } from "../src/agent/nodes/reason.js";
import { createActNode } from "../src/agent/nodes/act.js";
import { createRememberNode } from "../src/agent/nodes/remember.js";

/**
 * Creates a mock DbClient that tracks withTenant calls and delegates to
 * a provided implementation function.
 */
function createMockDb(impl?: (client: unknown) => Promise<unknown>): DbClient & { calls: Array<{ tenantId: string }> } {
  const calls: Array<{ tenantId: string }> = [];
  const mockClient = {};

  return {
    calls,
    pool: {} as DbClient["pool"],
    healthCheck: () => Promise.resolve(true),
    withTenant: vi.fn(async <T>(tenantId: string, fn: (client: unknown) => Promise<T>): Promise<T> => {
      calls.push({ tenantId });
      if (impl) {
        return impl(mockClient) as Promise<T>;
      }
      return fn(mockClient) as Promise<T>;
    }) as DbClient["withTenant"],
  };
}

function makeIncident(overrides: Partial<CorrelatedIncident> = {}): CorrelatedIncident {
  return {
    id: "corr-123",
    modules: ["kubelet", "network"],
    severity: 3,
    narrative: "Pod CrashLoopBackOff in namespace production",
    confidenceScore: 0.9,
    contributingEvents: [{ type: "pod_restart", count: 5 }],
    nodeId: "node-1",
    namespace: "production",
    podName: "api-server-xyz",
    createdAt: new Date("2024-01-01"),
    ...overrides,
  };
}

function makeBaseState(overrides: Partial<AgentStateType> = {}): AgentStateType {
  return {
    incident: makeIncident(),
    similarIncidents: [],
    playbook: null,
    reasoning: null,
    action: null,
    outcome: null,
    tenantId: "tenant-abc",
    currentStep: "receive",
    ...overrides,
  };
}

describe("Agent Node State Transitions", () => {
  describe("receiveNode", () => {
    it("returns updated state with currentStep='search_memory' and persisted incident ID", async () => {
      const mockDb = createMockDb(async () => "persisted-id-456");
      const receiveNode = createReceiveNode(mockDb);

      const state = makeBaseState();
      const result = await receiveNode(state);

      expect(result.currentStep).toBe("search_memory");
      expect(result.incident?.id).toBe("persisted-id-456");
      expect(mockDb.calls).toHaveLength(1);
      expect(mockDb.calls[0].tenantId).toBe("tenant-abc");
    });
  });

  describe("searchMemoryNode", () => {
    it("returns similarIncidents + playbook + currentStep='reason'", async () => {
      let callCount = 0;
      const mockDb = createMockDb(async () => {
        callCount++;
        if (callCount === 1) {
          // searchSimilar call
          return [
            { incidentId: "inc-past-1", similarity: 0.92, tenantId: "tenant-abc", modelVersion: "v1", createdAt: new Date() },
          ];
        }
        // getResolutionByIncident call
        return [
          {
            id: "res-1",
            incidentId: "inc-past-1",
            actionSequence: [{ actionType: "pod_restart", target: "pod-x", reason: "crash loop", riskLevel: "low" }],
            success: true,
            durationMs: 5000,
            reuseCount: 3,
          },
        ];
      });

      const searchMemoryNode = createSearchMemoryNode(mockDb);
      const state = makeBaseState();
      const result = await searchMemoryNode(state);

      expect(result.currentStep).toBe("reason");
      expect(result.similarIncidents).toHaveLength(1);
      expect(result.similarIncidents![0].incidentId).toBe("inc-past-1");
      expect(result.playbook).not.toBeNull();
      expect(result.playbook!.id).toBe("res-1");
    });
  });

  describe("reasonNode", () => {
    it("returns reasoning + action + currentStep='act'", async () => {
      const mockDb = createMockDb();
      const reasonNode = createReasonNode(mockDb);

      const state = makeBaseState({
        similarIncidents: [
          { incidentId: "inc-1", similarity: 0.85, narrative: "past incident", modules: ["kubelet"] },
        ],
        playbook: null,
      });

      const result = await reasonNode(state);

      expect(result.currentStep).toBe("act");
      expect(result.reasoning).not.toBeNull();
      expect(result.reasoning!.observation).toContain("severity 3");
      expect(result.reasoning!.selectedAction).toBeDefined();
      expect(result.action).not.toBeNull();
      expect(result.action!.actionType).toBeDefined();
      expect(result.action!.target).toBeDefined();
    });
  });

  describe("actNode", () => {
    it("low-risk action: returns outcome + currentStep='remember'", async () => {
      const mockDb = createMockDb(async () => "audit-id-1");
      const actNode = createActNode(mockDb);

      const state = makeBaseState({
        action: { actionType: "pod_restart", target: "pod-xyz", reason: "crash loop", riskLevel: "low" },
        reasoning: {
          observation: "pod crashing",
          analysis: "restart needed",
          selectedAction: "pod_restart",
          alternatives: [],
          confidence: 0.9,
          memoryContext: "test",
        },
      });

      const result = await actNode(state);

      expect(result.currentStep).toBe("remember");
      expect(result.outcome).not.toBeNull();
      expect(result.outcome!.success).toBe(true);
      expect(mockDb.calls.length).toBeGreaterThan(0);
    });

    it("high-risk action: returns currentStep='awaiting_approval'", async () => {
      const mockDb = createMockDb();
      const actNode = createActNode(mockDb);

      const state = makeBaseState({
        action: { actionType: "node_drain", target: "node-1", reason: "unresponsive", riskLevel: "high" },
      });

      const result = await actNode(state);

      expect(result.currentStep).toBe("awaiting_approval");
      expect(result.outcome).toBeUndefined();
      // No DB call for high-risk — just gates
      expect(mockDb.calls).toHaveLength(0);
    });
  });

  describe("rememberNode", () => {
    it("returns currentStep='complete' and stores resolution", async () => {
      const mockDb = createMockDb(async () => "resolution-id-1");
      const rememberNode = createRememberNode(mockDb);

      const state = makeBaseState({
        action: { actionType: "pod_restart", target: "pod-xyz", reason: "crash loop", riskLevel: "low" },
        outcome: { success: true, durationMs: 120 },
        playbook: null,
      });

      const result = await rememberNode(state);

      expect(result.currentStep).toBe("complete");
      expect(mockDb.calls).toHaveLength(1);
      expect(mockDb.calls[0].tenantId).toBe("tenant-abc");
    });
  });
});
