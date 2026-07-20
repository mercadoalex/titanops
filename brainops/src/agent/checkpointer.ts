import { PostgresSaver } from "@langchain/langgraph-checkpoint-postgres";

/**
 * Creates a PostgresSaver checkpointer configured for CockroachDB.
 * CockroachDB is Postgres-compatible, so PostgresSaver works directly.
 *
 * @param connectionString - CockroachDB connection string (postgres:// URI)
 * @returns Configured PostgresSaver instance with tables initialized
 */
export async function createCheckpointer(
  connectionString: string,
): Promise<PostgresSaver> {
  const checkpointer = PostgresSaver.fromConnString(connectionString);
  await checkpointer.setup();
  return checkpointer;
}
