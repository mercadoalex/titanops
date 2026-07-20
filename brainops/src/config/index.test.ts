import { describe, it, expect, beforeEach, afterEach } from "vitest";
import { loadConfig } from "./index.js";

describe("loadConfig", () => {
  const originalEnv = process.env;

  beforeEach(() => {
    process.env = { ...originalEnv };
    // Set minimum required env vars
    process.env["COCKROACHDB_URI"] =
      "postgresql://user:pass@localhost:26257/brain";
    process.env["NATS_URL"] = "nats://localhost:4222";
    process.env["MANAGED_MCP_API_KEY"] = "test-api-key";
    process.env["CCLOUD_CLUSTER_ID"] = "cluster-123";
  });

  afterEach(() => {
    process.env = originalEnv;
  });

  it("loads all required fields from environment variables", () => {
    const config = loadConfig();
    expect(config.cockroachdbUri).toBe(
      "postgresql://user:pass@localhost:26257/brain",
    );
    expect(config.natsUrl).toBe("nats://localhost:4222");
    expect(config.managedMcpApiKey).toBe("test-api-key");
    expect(config.ccloudClusterId).toBe("cluster-123");
  });

  it("applies sensible defaults for optional fields", () => {
    const config = loadConfig();
    expect(config.cockroachdbPoolSize).toBe(10);
    expect(config.natsSubject).toBe("titanops.correlation.incidents.>");
    expect(config.bedrockRegion).toBe("us-east-2");
    expect(config.bedrockModelId).toBe("anthropic.claude-3-haiku-20240307");
    expect(config.embeddingModelId).toBe("amazon.titan-embed-text-v2:0");
    expect(config.rateLimitPerMinute).toBe(10);
    expect(config.circuitBreakerThreshold).toBe(3);
    expect(config.circuitBreakerWindowMs).toBe(300000);
    expect(config.selfOptimizationEnabled).toBe(true);
    expect(config.selfOptimizationCron).toBe("0 */6 * * *");
    expect(config.queryLatencyThresholdMs).toBe(100);
    expect(config.managedMcpEndpoint).toBe("https://cockroachlabs.cloud/mcp");
    expect(config.logLevel).toBe("info");
    expect(config.port).toBe(3001);
  });

  it("overrides defaults when env vars are set", () => {
    process.env["COCKROACHDB_POOL_SIZE"] = "20";
    process.env["BEDROCK_REGION"] = "us-west-2";
    process.env["RATE_LIMIT_PER_MINUTE"] = "25";
    process.env["PORT"] = "8080";
    process.env["LOG_LEVEL"] = "debug";
    process.env["SELF_OPTIMIZATION_ENABLED"] = "false";

    const config = loadConfig();
    expect(config.cockroachdbPoolSize).toBe(20);
    expect(config.bedrockRegion).toBe("us-west-2");
    expect(config.rateLimitPerMinute).toBe(25);
    expect(config.port).toBe(8080);
    expect(config.logLevel).toBe("debug");
    expect(config.selfOptimizationEnabled).toBe(false);
  });

  it("throws listing ALL missing required env vars at once", () => {
    delete process.env["COCKROACHDB_URI"];
    delete process.env["NATS_URL"];
    delete process.env["MANAGED_MCP_API_KEY"];

    expect(() => loadConfig()).toThrow("COCKROACHDB_URI");
    // Verify all are listed in the same error
    try {
      loadConfig();
    } catch (e) {
      const msg = (e as Error).message;
      expect(msg).toContain("COCKROACHDB_URI");
      expect(msg).toContain("NATS_URL");
      expect(msg).toContain("MANAGED_MCP_API_KEY");
    }
  });

  it("requires CCLOUD_CLUSTER_ID when self-optimization is enabled", () => {
    delete process.env["CCLOUD_CLUSTER_ID"];
    // Self-optimization defaults to true
    expect(() => loadConfig()).toThrow("CCLOUD_CLUSTER_ID");
  });

  it("does not require CCLOUD_CLUSTER_ID when self-optimization is disabled", () => {
    delete process.env["CCLOUD_CLUSTER_ID"];
    process.env["SELF_OPTIMIZATION_ENABLED"] = "false";

    const config = loadConfig();
    expect(config.ccloudClusterId).toBe("");
    expect(config.selfOptimizationEnabled).toBe(false);
  });

  it("treats empty string env vars as unset", () => {
    process.env["COCKROACHDB_URI"] = "";
    process.env["NATS_URL"] = "";

    expect(() => loadConfig()).toThrow("COCKROACHDB_URI");
    try {
      loadConfig();
    } catch (e) {
      const msg = (e as Error).message;
      expect(msg).toContain("NATS_URL");
    }
  });
});
