import { describe, it, expect } from "vitest";
import type { AgentStateType, CorrelatedIncident } from "../src/agent/state.js";
import { createReceiveNode } from "../src/agent/nodes/receive.js";
import { createSearchMemoryNode } from "../src/agent/nodes/search-memory.js";
import { createReasonNode } from "../src/agent/nodes/reason.js";
import { createActNode } from "../src/agent/nodes/act.js";
import { createRememberNode } from "../src/agent/nodes/remember.js";
import { createMockAgentStore } from "../src/agent/ports.mock.js";

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
      const store = createMockAgentStore();
      const receiveNode = createReceiveNode(store);

      const state = makeBaseState();
      const result = await receiveNode(state);

      expect(result.currentStep).toBe("search_memory");
      expect(result.incident?.id).toBe("mock-incident-1");
      expect(store.incidents).toHaveLength(1);
      expect(store.incidents[0].input.tenantId).toBe("tenant-abc");
      expect(store.incidents[0].input.correlationId).toBe("corr-123");
    });
  });

  describe("searchMemoryNode", () => {
    it("returns similarIncidents + playbook + currentStep='reason'", async () => {
      const store = createMockAgentStore({
        similarIncidents: [
          { incidentId: "inc-past-1", similarity: 0.92, narrative: "", modules: [] },
        ],
        resolutions: new Map([
          ["inc-past-1", [{
            id: "res-1",
            incidentId: "inc-past-1",
            actionSequence: [{ actionType: "pod_restart", target: "pod-x", reason: "crash loop", riskLevel: "low" }],
            success: true,
            durationMs: 5000,
            reuseCount: 3,
          }]],
        ]),
      });

      const searchMemoryNode = createSearchMemoryNode(store);
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
      const store = createMockAgentStore();
      const reasonNode = createReasonNode(store);

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
      const store = createMockAgentStore();
      const actNode = createActNode(store);

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
      expect(store.audits).toHaveLength(1);
      expect(store.audits[0].input.actionType).toBe("pod_restart");
    });

    it("high-risk action: returns currentStep='awaiting_approval'", async () => {
      const store = createMockAgentStore();
      const actNode = createActNode(store);

      const state = makeBaseState({
        action: { actionType: "node_drain", target: "node-1", reason: "unresponsive", riskLevel: "high" },
      });

      const result = await actNode(state);

      expect(result.currentStep).toBe("awaiting_approval");
      expect(result.outcome).toBeUndefined();
      // No audit call for high-risk — just gates
      expect(store.audits).toHaveLength(0);
    });
  });

  describe("rememberNode", () => {
    it("returns currentStep='complete' and stores resolution", async () => {
      const store = createMockAgentStore();
      const rememberNode = createRememberNode(store);

      const state = makeBaseState({
        action: { actionType: "pod_restart", target: "pod-xyz", reason: "crash loop", riskLevel: "low" },
        outcome: { success: true, durationMs: 120 },
        playbook: null,
      });

      const result = await rememberNode(state);

      expect(result.currentStep).toBe("complete");
      expect(store.insertedResolutions).toHaveLength(1);
      expect(store.insertedResolutions[0].incidentId).toBe("corr-123");
      expect(store.insertedResolutions[0].success).toBe(true);
    });
  });
});
