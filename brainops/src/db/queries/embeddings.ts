// Embedding query methods — vector similarity search for ollinai.incident_embeddings

import pg from "pg";

type PoolClient = pg.PoolClient;

export interface EmbeddingInput {
  incidentId: string;
  tenantId: string;
  embedding: number[];
  modelVersion: string;
}

export interface SimilarIncident {
  incidentId: string;
  tenantId: string;
  similarity: number;
  modelVersion: string;
  createdAt: Date;
}

export interface SearchSimilarOptions {
  embedding: number[];
  limit?: number;
  minSimilarity?: number;
}

/**
 * Inserts an embedding vector for an incident.
 */
export async function insertEmbedding(
  client: PoolClient,
  input: EmbeddingInput,
): Promise<void> {
  const vectorLiteral = `[${input.embedding.join(",")}]`;
  await client.query(
    `INSERT INTO ollinai.incident_embeddings (incident_id, tenant_id, embedding, model_version)
     VALUES ($1, $2, $3::VECTOR, $4)
     ON CONFLICT (tenant_id, incident_id) DO UPDATE
       SET embedding = EXCLUDED.embedding,
           model_version = EXCLUDED.model_version`,
    [input.incidentId, input.tenantId, vectorLiteral, input.modelVersion],
  );
}

/**
 * Performs a cosine similarity search against stored embeddings.
 * Returns incidents ordered by similarity (descending), filtered by optional minimum threshold.
 */
export async function searchSimilar(
  client: PoolClient,
  options: SearchSimilarOptions,
): Promise<SimilarIncident[]> {
  const { embedding, limit = 10, minSimilarity = 0.7 } = options;
  const vectorLiteral = `[${embedding.join(",")}]`;

  const result = await client.query(
    `SELECT incident_id, tenant_id, model_version, created_at,
            1 - (embedding <=> $1::VECTOR) AS similarity
     FROM ollinai.incident_embeddings
     WHERE 1 - (embedding <=> $1::VECTOR) >= $2
     ORDER BY similarity DESC
     LIMIT $3`,
    [vectorLiteral, minSimilarity, limit],
  );

  return result.rows.map((row: Record<string, unknown>) => ({
    incidentId: row.incident_id as string,
    tenantId: row.tenant_id as string,
    similarity: row.similarity as number,
    modelVersion: row.model_version as string,
    createdAt: row.created_at as Date,
  }));
}
