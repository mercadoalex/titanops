// Bedrock Titan embedding generation

import {
  BedrockRuntimeClient,
  InvokeModelCommand,
} from "@aws-sdk/client-bedrock-runtime";

/**
 * Generates text embeddings using Amazon Bedrock Titan Embeddings.
 */
export class EmbeddingGenerator {
  private client: BedrockRuntimeClient;
  private modelId: string;

  constructor(region: string, modelId: string) {
    this.client = new BedrockRuntimeClient({ region });
    this.modelId = modelId;
  }

  /**
   * Generates an embedding vector from a text input.
   * Returns a 1536-dimensional vector or null on failure.
   */
  async generate(text: string): Promise<number[] | null> {
    try {
      const body = JSON.stringify({
        inputText: text,
      });

      const command = new InvokeModelCommand({
        modelId: this.modelId,
        contentType: "application/json",
        accept: "application/json",
        body: new TextEncoder().encode(body),
      });

      const response = await this.client.send(command);
      const responseBody = JSON.parse(
        new TextDecoder().decode(response.body),
      ) as { embedding: number[] };

      return responseBody.embedding;
    } catch (err) {
      console.error(
        "[EmbeddingGenerator] Failed to generate embedding:",
        err instanceof Error ? err.message : err,
      );
      return null;
    }
  }
}
