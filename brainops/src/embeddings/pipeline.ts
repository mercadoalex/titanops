// Incident → embed → store flow with retry logic

import type { EmbeddingGenerator } from "./generator.js";
import type { DbClient } from "../db/client.js";
import { insertEmbedding } from "../db/queries/embeddings.js";

/**
 * Orchestrates embedding generation and storage for incidents.
 */
export class EmbeddingPipeline {
  private generator: EmbeddingGenerator;
  private db: DbClient;

  constructor(generator: EmbeddingGenerator, db: DbClient) {
    this.generator = generator;
    this.db = db;
  }

  /**
   * Generates an embedding from the incident narrative and stores it.
   * Uses retry with exponential backoff (3 attempts: 1s, 2s, 4s).
   * If all attempts fail, logs a warning and returns without blocking.
   */
  async processIncident(
    tenantId: string,
    incidentId: string,
    narrative: string,
  ): Promise<void> {
    const embedding = await this.generateWithRetry(narrative);

    if (!embedding) {
      console.warn(
        `[EmbeddingPipeline] All retry attempts failed for incident ${incidentId}. Skipping embedding storage.`,
      );
      return;
    }

    try {
      await this.db.withTenant(tenantId, async (client) => {
        await insertEmbedding(client, {
          incidentId,
          tenantId,
          embedding,
          modelVersion: "titan-embed-v2",
        });
      });

      console.info(
        `[EmbeddingPipeline] Stored embedding for incident ${incidentId} (${embedding.length} dimensions)`,
      );
    } catch (err) {
      console.error(
        `[EmbeddingPipeline] Failed to store embedding for incident ${incidentId}:`,
        err instanceof Error ? err.message : err,
      );
    }
  }

  /**
   * Attempts embedding generation with retry and exponential backoff.
   * 3 attempts with delays of 1s, 2s, 4s between retries.
   */
  private async generateWithRetry(text: string): Promise<number[] | null> {
    const maxAttempts = 3;
    const baseDelayMs = 1000;

    for (let attempt = 1; attempt <= maxAttempts; attempt++) {
      const result = await this.generator.generate(text);
      if (result) {
        return result;
      }

      if (attempt < maxAttempts) {
        const delayMs = baseDelayMs * Math.pow(2, attempt - 1);
        console.warn(
          `[EmbeddingPipeline] Attempt ${attempt}/${maxAttempts} failed. Retrying in ${delayMs}ms...`,
        );
        await sleep(delayMs);
      }
    }

    return null;
  }
}

function sleep(ms: number): Promise<void> {
  return new Promise((resolve) => setTimeout(resolve, ms));
}
