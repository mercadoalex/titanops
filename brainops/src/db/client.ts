// CockroachDB connection pool with tenant isolation

import pg from "pg";
import type { BrainOpsConfig } from "../config/index.js";

const { Pool } = pg;
type Pool = pg.Pool;
type PoolClient = pg.PoolClient;

export interface DbClient {
  pool: Pool;
  withTenant: <T>(tenantId: string, fn: (client: PoolClient) => Promise<T>) => Promise<T>;
  healthCheck: () => Promise<boolean>;
}

/**
 * Creates a pg.Pool configured for CockroachDB (SSL required).
 */
export function createDbClient(config: BrainOpsConfig): DbClient {
  const pool = new Pool({
    connectionString: config.cockroachdbUri,
    max: config.cockroachdbPoolSize,
    ssl: { rejectUnauthorized: false },
  });

  return {
    pool,
    withTenant: <T>(tenantId: string, fn: (client: PoolClient) => Promise<T>) =>
      withTenant(pool, tenantId, fn),
    healthCheck: () => healthCheck(pool),
  };
}

/**
 * Acquires a client from the pool, sets the RLS tenant context via
 * `SET app.tenant_id`, executes the provided function, then releases.
 */
export async function withTenant<T>(
  pool: Pool,
  tenantId: string,
  fn: (client: PoolClient) => Promise<T>,
): Promise<T> {
  const client = await pool.connect();
  try {
    await client.query("SET app.tenant_id = $1", [tenantId]);
    return await fn(client);
  } finally {
    client.release();
  }
}

/**
 * Simple connectivity check — executes `SELECT 1`.
 */
export async function healthCheck(pool: Pool): Promise<boolean> {
  try {
    await pool.query("SELECT 1");
    return true;
  } catch {
    return false;
  }
}
