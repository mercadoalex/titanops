import { describe, it, expect, vi } from "vitest";
import type { DbClient } from "../src/db/client.js";
import { handleExecuteRemediation } from "../src/mcp/tools/execute-remediation.js";

const mockClient = {
  query: vi.fn().mockResolvedValue({ rows: [{ id: "test-id" }] }),
};

const mockDb: DbClient = {
  pool: null as unknown as DbClient["pool"],
  withTenant: async <T>(_tenantId: string, fn: (client: unknown) => Promise<T>): Promise<T> => {
    return fn(mockClient);
  },
  healthCheck: async () => true,
};

describe("execute-remediation tool", () => {
  it("high-risk action (node_drain) returns awaiting_approval", async () => {
    const result = await handleExecuteRemediation(mockDb, "tenant-1", {
      actionType: "node_drain",
      target: "node-1",
      reason: "unresponsive node",
    });

    expect(result.status).toBe("awaiting_approval");
    expect(result.auditId).toBeUndefined();
  });

  it("low-risk action (pod_restart) returns executed", async () => {
    const result = await handleExecuteRemediation(mockDb, "tenant-1", {
      actionType: "pod_restart",
      target: "pod-xyz",
      reason: "crash loop",
    });

    expect(result.status).toBe("executed");
    expect(result.auditId).toBe("test-id");
  });
});
