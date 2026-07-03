// BrainOps — Entry point and startup orchestration

import { loadConfig, type BrainOpsConfig } from "./config/index.js";
import { startHealthServer, type HealthChecks } from "./health/endpoints.js";

// ---------------------------------------------------------------------------
// Placeholder connection types (replaced in Tasks 2 and 5)
// ---------------------------------------------------------------------------

interface DbConnection {
  connected: boolean;
  close(): Promise<void>;
}

interface NatsConnection {
  connected: boolean;
  close(): Promise<void>;
}

// ---------------------------------------------------------------------------
// Placeholder connection factories
// ---------------------------------------------------------------------------

async function connectDb(config: BrainOpsConfig): Promise<DbConnection> {
  console.log(`[BrainOps] Connecting to CockroachDB (pool size: ${config.cockroachdbPoolSize})…`);
  // TODO(task-2): Replace with real pg.Pool connection
  return { connected: true, close: async () => {} };
}

async function connectNats(config: BrainOpsConfig): Promise<NatsConnection> {
  console.log(`[BrainOps] Connecting to NATS at ${config.natsUrl}…`);
  // TODO(task-5): Replace with real NATS subscriber
  return { connected: true, close: async () => {} };
}

// ---------------------------------------------------------------------------
// Main startup sequence
// ---------------------------------------------------------------------------

async function main(): Promise<void> {
  // 1. Load configuration
  console.log("[BrainOps] Loading configuration…");
  const config = loadConfig();
  console.log(`[BrainOps] Config loaded (logLevel=${config.logLevel})`);

  // 2. Connect to CockroachDB
  const db = await connectDb(config);

  // 3. Connect to NATS
  const nats = await connectNats(config);

  // 4. Start health server with real health checks
  const checks: HealthChecks = {
    isNatsConnected: () => nats.connected,
    isCockroachdbConnected: async () => db.connected,
  };
  const healthServer = startHealthServer(config.port, checks);

  // 5. Ready
  console.log("[BrainOps] BrainOps ready");

  // ---------------------------------------------------------------------------
  // Graceful shutdown
  // ---------------------------------------------------------------------------

  const shutdown = async (signal: string) => {
    console.log(`[BrainOps] Received ${signal}, shutting down…`);

    // Close health server
    await new Promise<void>((resolve, reject) => {
      healthServer.close((err) => (err ? reject(err) : resolve()));
    });
    console.log("[BrainOps] Health server closed");

    // Disconnect NATS
    nats.connected = false;
    await nats.close();
    console.log("[BrainOps] NATS disconnected");

    // Close DB pool
    db.connected = false;
    await db.close();
    console.log("[BrainOps] CockroachDB pool closed");

    console.log("[BrainOps] Shutdown complete");
    process.exit(0);
  };

  process.on("SIGTERM", () => void shutdown("SIGTERM"));
  process.on("SIGINT", () => void shutdown("SIGINT"));
}

main().catch((err: unknown) => {
  console.error("[BrainOps] Fatal startup error:", err);
  process.exit(1);
});
