import { createServer, IncomingMessage, ServerResponse, Server } from "node:http";

/**
 * Interface for health check dependencies.
 * Consumers provide implementations that report connectivity status.
 */
export interface HealthChecks {
  /** Returns true when the NATS connection is active. */
  isNatsConnected(): boolean;
  /** Returns true when CockroachDB is reachable (may involve a ping query). */
  isCockroachdbConnected(): Promise<boolean>;
}

/**
 * Starts a lightweight HTTP server exposing health check endpoints:
 *
 * - GET /healthz → 200 when the process is running (liveness)
 * - GET /readyz  → 200 when NATS and CockroachDB are both connected (readiness)
 *                  503 when either dependency is unavailable
 *
 * @param port    TCP port to listen on (default from config: 3001)
 * @param checks  Dependency health check implementations
 * @returns       The underlying http.Server instance (for graceful shutdown)
 */
export function startHealthServer(port: number, checks: HealthChecks): Server {
  const server = createServer(
    async (req: IncomingMessage, res: ServerResponse) => {
      if (req.method !== "GET") {
        res.writeHead(405, { "Content-Type": "application/json" });
        res.end(JSON.stringify({ error: "Method Not Allowed" }));
        return;
      }

      if (req.url === "/healthz") {
        res.writeHead(200, { "Content-Type": "application/json" });
        res.end(JSON.stringify({ status: "ok" }));
        return;
      }

      if (req.url === "/readyz") {
        try {
          const natsOk = checks.isNatsConnected();
          const cockroachOk = await checks.isCockroachdbConnected();

          if (natsOk && cockroachOk) {
            res.writeHead(200, { "Content-Type": "application/json" });
            res.end(
              JSON.stringify({
                status: "ok",
                nats: "connected",
                cockroachdb: "connected",
              }),
            );
          } else {
            res.writeHead(503, { "Content-Type": "application/json" });
            res.end(
              JSON.stringify({
                status: "unavailable",
                nats: natsOk ? "connected" : "disconnected",
                cockroachdb: cockroachOk ? "connected" : "disconnected",
              }),
            );
          }
        } catch {
          res.writeHead(503, { "Content-Type": "application/json" });
          res.end(
            JSON.stringify({
              status: "unavailable",
              error: "Health check failed",
            }),
          );
        }
        return;
      }

      // Unknown path
      res.writeHead(404, { "Content-Type": "application/json" });
      res.end(JSON.stringify({ error: "Not Found" }));
    },
  );

  server.listen(port, () => {
    console.log(`[BrainOps] Health server listening on port ${port}`);
  });

  return server;
}
