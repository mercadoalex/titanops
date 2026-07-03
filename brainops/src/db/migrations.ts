// Schema migration runner — idempotent, ordered execution of SQL files

import fs from "node:fs";
import path from "node:path";
import pg from "pg";

type Pool = pg.Pool;

const MIGRATIONS_TABLE = "ollinai.schema_migrations";

/**
 * Ensures the schema_migrations tracking table exists.
 */
async function ensureMigrationsTable(pool: Pool): Promise<void> {
  await pool.query(`
    CREATE SCHEMA IF NOT EXISTS ollinai;
    CREATE TABLE IF NOT EXISTS ${MIGRATIONS_TABLE} (
      filename TEXT PRIMARY KEY,
      applied_at TIMESTAMPTZ DEFAULT now()
    );
  `);
}

/**
 * Returns the set of migrations that have already been applied.
 */
async function getAppliedMigrations(pool: Pool): Promise<Set<string>> {
  const result = await pool.query<{ filename: string }>(
    `SELECT filename FROM ${MIGRATIONS_TABLE} ORDER BY filename`,
  );
  return new Set(result.rows.map((r) => r.filename));
}

/**
 * Reads all .sql files from the migrations/ directory, sorted alphabetically.
 */
function getMigrationFiles(migrationsDir: string): string[] {
  if (!fs.existsSync(migrationsDir)) {
    return [];
  }
  return fs
    .readdirSync(migrationsDir)
    .filter((f) => f.endsWith(".sql"))
    .sort();
}

/**
 * Runs pending migrations in alphabetical order.
 * Idempotent — skips migrations that have already been applied.
 * Logs each migration as it executes.
 */
export async function runMigrations(pool: Pool): Promise<void> {
  const migrationsDir = path.resolve(
    path.dirname(new URL(import.meta.url).pathname),
    "../../migrations",
  );

  await ensureMigrationsTable(pool);
  const applied = await getAppliedMigrations(pool);
  const files = getMigrationFiles(migrationsDir);

  for (const filename of files) {
    if (applied.has(filename)) {
      continue;
    }

    const filePath = path.join(migrationsDir, filename);
    const sql = fs.readFileSync(filePath, "utf-8");

    console.log(`[migrations] Applying: ${filename}`);
    await pool.query(sql);
    await pool.query(
      `INSERT INTO ${MIGRATIONS_TABLE} (filename) VALUES ($1)`,
      [filename],
    );
    console.log(`[migrations] Applied: ${filename}`);
  }
}
