// search_similar_incidents tool — generates placeholder embedding + vector search

import type { DbClient } from "../../db/client.js";
import { searchSimilar, type SimilarIncident } from "../../db/queries/embeddings.js";

export interface SearchSimilarInput {
  description: string;
  limit?: number;
  minSimilarity?: number;
}

export interface SearchSimilarResult {
  incidents: SimilarIncident[];
}

/**
 * Handles the search_similar_incidents MCP tool.
 * Generates a placeholder embedding (1536 zeros — real embedding comes from task 6)
 * and performs a cosine similarity search against stored incident embeddings.
 */
export async function handleSearchSimilar(
  db: DbClient,
  tenantId: string,
  input: SearchSimilarInput,
): Promise<SearchSimilarResult> {
  // Placeholder embedding — will be replaced with real Bedrock Titan embedding in task 6
  const placeholderEmbedding = new Array<number>(1536).fill(0);

  const incidents = await db.withTenant(tenantId, (client) =>
    searchSimilar(client, {
      embedding: placeholderEmbedding,
      limit: input.limit,
      minSimilarity: input.minSimilarity,
    }),
  );

  return { incidents };
}
