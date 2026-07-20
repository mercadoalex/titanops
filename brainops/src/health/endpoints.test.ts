import { describe, it, expect, afterEach } from "vitest";
import { startHealthServer, HealthChecks } from "./endpoints.js";
import type { Server } from "node:http";

function makeChecks(
  nats: boolean,
  cockroach: boolean | (() => Promise<boolean>),
): HealthChecks {
  return {
    isNatsConnected: () => nats,
    isCockroachdbConnected:
      typeof cockroach === "function"
        ? cockroach
        : () => Promise.resolve(cockroach),
  };
}

async function fetch(port: number, path: string): Promise<{ status: number; body: Record<string, unknown> }> {
  const res = await globalThis.fetch(`http://127.0.0.1:${port}${path}`);
  const body = (await res.json()) as Record<string, unknown>;
  return { status: res.status, body };
}

describe("health endpoints", () => {
  let server: Server;

  afterEach(() => {
    return new Promise<void>((resolve) => {
      if (server) {
        server.close(() => resolve());
      } else {
        resolve();
      }
    });
  });

  it("GET /healthz returns 200 when process is running", async () => {
    const port = 19100;
    server = startHealthServer(port, makeChecks(true, true));
    await waitForServer(server);

    const { status, body } = await fetch(port, "/healthz");
    expect(status).toBe(200);
    expect(body.status).toBe("ok");
  });

  it("GET /readyz returns 200 when NATS and CockroachDB are connected", async () => {
    const port = 19101;
    server = startHealthServer(port, makeChecks(true, true));
    await waitForServer(server);

    const { status, body } = await fetch(port, "/readyz");
    expect(status).toBe(200);
    expect(body.status).toBe("ok");
    expect(body.nats).toBe("connected");
    expect(body.cockroachdb).toBe("connected");
  });

  it("GET /readyz returns 503 when NATS is disconnected", async () => {
    const port = 19102;
    server = startHealthServer(port, makeChecks(false, true));
    await waitForServer(server);

    const { status, body } = await fetch(port, "/readyz");
    expect(status).toBe(503);
    expect(body.status).toBe("unavailable");
    expect(body.nats).toBe("disconnected");
    expect(body.cockroachdb).toBe("connected");
  });

  it("GET /readyz returns 503 when CockroachDB is unreachable", async () => {
    const port = 19103;
    server = startHealthServer(port, makeChecks(true, false));
    await waitForServer(server);

    const { status, body } = await fetch(port, "/readyz");
    expect(status).toBe(503);
    expect(body.status).toBe("unavailable");
    expect(body.nats).toBe("connected");
    expect(body.cockroachdb).toBe("disconnected");
  });

  it("GET /readyz returns 503 when both are disconnected", async () => {
    const port = 19104;
    server = startHealthServer(port, makeChecks(false, false));
    await waitForServer(server);

    const { status, body } = await fetch(port, "/readyz");
    expect(status).toBe(503);
    expect(body.status).toBe("unavailable");
    expect(body.nats).toBe("disconnected");
    expect(body.cockroachdb).toBe("disconnected");
  });

  it("GET /readyz returns 503 when CockroachDB check throws", async () => {
    const port = 19105;
    const checks = makeChecks(true, () => Promise.reject(new Error("connection timeout")));
    server = startHealthServer(port, checks);
    await waitForServer(server);

    const { status, body } = await fetch(port, "/readyz");
    expect(status).toBe(503);
    expect(body.status).toBe("unavailable");
  });

  it("returns 404 for unknown paths", async () => {
    const port = 19106;
    server = startHealthServer(port, makeChecks(true, true));
    await waitForServer(server);

    const { status, body } = await fetch(port, "/unknown");
    expect(status).toBe(404);
    expect(body.error).toBe("Not Found");
  });
});

/** Wait for the server to be ready to accept connections. */
function waitForServer(server: Server): Promise<void> {
  return new Promise((resolve) => {
    if (server.listening) {
      resolve();
    } else {
      server.on("listening", () => resolve());
    }
  });
}
