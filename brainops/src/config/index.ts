// BrainOps configuration — environment variable loading

export interface BrainOpsConfig {
  // CockroachDB
  cockroachdbUri: string;
  cockroachdbPoolSize: number;

  // NATS
  natsUrl: string;
  natsSubject: string;

  // LLM
  bedrockRegion: string;
  bedrockModelId: string;
  embeddingModelId: string;

  // Safety
  rateLimitPerMinute: number;
  circuitBreakerThreshold: number;
  circuitBreakerWindowMs: number;

  // Self-optimization
  selfOptimizationEnabled: boolean;
  selfOptimizationCron: string;
  queryLatencyThresholdMs: number;

  // CockroachDB Managed MCP
  managedMcpEndpoint: string;
  managedMcpApiKey: string;

  // ccloud
  ccloudClusterId: string;
  ccloudServiceAccountKey: string;

  // General
  logLevel: string;
  port: number;
}

/**
 * Loads BrainOps configuration from environment variables.
 * Validates that all required fields are present, throwing a descriptive error
 * listing ALL missing variables (fail-fast, all-at-once).
 */
export function loadConfig(): BrainOpsConfig {
  const missing: string[] = [];

  const cockroachdbUri = env("COCKROACHDB_URI");
  if (!cockroachdbUri) missing.push("COCKROACHDB_URI");

  const natsUrl = env("NATS_URL");
  if (!natsUrl) missing.push("NATS_URL");

  const managedMcpApiKey = env("MANAGED_MCP_API_KEY");
  if (!managedMcpApiKey) missing.push("MANAGED_MCP_API_KEY");

  // ccloudClusterId is required when self-optimization is enabled
  const selfOptimizationEnabled = parseBool(
    env("SELF_OPTIMIZATION_ENABLED"),
    true,
  );
  const ccloudClusterId = env("CCLOUD_CLUSTER_ID") ?? "";
  if (selfOptimizationEnabled && !ccloudClusterId) {
    missing.push("CCLOUD_CLUSTER_ID (required when self-optimization is enabled)");
  }

  if (missing.length > 0) {
    throw new Error(
      `BrainOps configuration error: missing required environment variables:\n` +
        missing.map((v) => `  - ${v}`).join("\n"),
    );
  }

  return {
    // CockroachDB
    cockroachdbUri: cockroachdbUri!,
    cockroachdbPoolSize: parseInt(env("COCKROACHDB_POOL_SIZE") ?? "10", 10),

    // NATS
    natsUrl: natsUrl!,
    natsSubject: env("NATS_SUBJECT") ?? "titanops.correlation.incidents.>",

    // LLM
    bedrockRegion: env("BEDROCK_REGION") ?? "us-east-2",
    bedrockModelId:
      env("BEDROCK_MODEL_ID") ?? "anthropic.claude-3-haiku-20240307",
    embeddingModelId:
      env("EMBEDDING_MODEL_ID") ?? "amazon.titan-embed-text-v2:0",

    // Safety
    rateLimitPerMinute: parseInt(env("RATE_LIMIT_PER_MINUTE") ?? "10", 10),
    circuitBreakerThreshold: parseInt(
      env("CIRCUIT_BREAKER_THRESHOLD") ?? "3",
      10,
    ),
    circuitBreakerWindowMs: parseInt(
      env("CIRCUIT_BREAKER_WINDOW_MS") ?? "300000",
      10,
    ),

    // Self-optimization
    selfOptimizationEnabled,
    selfOptimizationCron: env("SELF_OPTIMIZATION_CRON") ?? "0 */6 * * *",
    queryLatencyThresholdMs: parseInt(
      env("QUERY_LATENCY_THRESHOLD_MS") ?? "100",
      10,
    ),

    // CockroachDB Managed MCP
    managedMcpEndpoint:
      env("MANAGED_MCP_ENDPOINT") ?? "https://cockroachlabs.cloud/mcp",
    managedMcpApiKey: managedMcpApiKey!,

    // ccloud
    ccloudClusterId,
    ccloudServiceAccountKey: env("CCLOUD_SERVICE_ACCOUNT_KEY") ?? "",

    // General
    logLevel: env("LOG_LEVEL") ?? "info",
    port: parseInt(env("PORT") ?? "3001", 10),
  };
}

/** Read an environment variable, returning undefined if empty or unset. */
function env(name: string): string | undefined {
  const value = process.env[name];
  if (value === undefined || value === "") return undefined;
  return value;
}

/** Parse a string as a boolean with a default fallback. */
function parseBool(value: string | undefined, defaultValue: boolean): boolean {
  if (value === undefined) return defaultValue;
  const lower = value.toLowerCase();
  if (lower === "false" || lower === "0" || lower === "no") return false;
  if (lower === "true" || lower === "1" || lower === "yes") return true;
  return defaultValue;
}
