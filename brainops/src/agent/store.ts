/**
 * DbAgentStore — real CockroachDB implementation of the AgentStore interface.
 *
 * This is the infrastructure adapter that bridges the agent engine (ports.ts)
 * to the database layer (src/db/). It lives at the boundary — the agent nodes
 * never see DbClient or pg directly; they only see the AgentStore interface.
 */

import type { DbClient } from "../db/client.js";
import { insertIncident } from "../db/queries/incidents.js";
import { searchSimilar } from "../db/queries/embeddings.js";
import {
  getResolutionByIncident,
  insertResolution,
  incrementReuseCount,
} from "../db/queries/resolutions.js";
import { insertAuditEntry } from "../db/queries/audit.js";
import type {
  AgentStore,
  InsertIncidentInput,
  SearchSimilarInput,
  ResolutionRecord,
  InsertResolutionInput,
  InsertAuditInput,
} from "./ports.js";
import type { SimilarIncident } from "./state.js";

/**
 * Creates an AgentStore backed by the real CockroachDB client.
 * This is the only place where DbClient is used by the agent subsystem.
 */
export function createDbAgentStore(db: DbClient): AgentStore {
  return {
    async insertIncident(input: InsertIncidentInput): Promise<string> {
      return db.withTenant(input.tenantId, (client) =>
        insertIncident(client, {
          tenantId: input.tenantId,
          correlationId: input.correlationId,
          modules: input.modules,
          severity: input.severity,
          narrative: input.narrative,
          contributingEvents: input.contributingEvents,
          confidenceScore: input.confidenceScore,
          nodeId: input.nodeId,
          namespace: input.namespace,
          podName: input.podName,
        }),
      );
    },

    async searchSimilar(input: SearchSimilarInput): Promise<SimilarIncident[]> {
      const rows = await db.withTenant(input.tenantId, (client) =>
        searchSimilar(client, {
          embedding: input.embedding,
          limit: input.limit,
          minSimilarity: input.minSimilarity,
        }),
      );

      return rows.map((row) => ({
        incidentId: row.incidentId,
        similarity: row.similarity,
        narrative: "",
        modules: [],
      }));
    },

    async getResolutionsByIncident(
      tenantId: string,
      incidentId: string,
    ): Promise<ResolutionRecord[]> {
      const rows = await db.withTenant(tenantId, (client) =>
        getResolutionByIncident(client, incidentId),
      );

      return rows.map((row) => ({
        id: row.id,
        incidentId: row.incidentId,
        actionSequence: row.actionSequence,
        success: row.success,
        durationMs: row.durationMs,
        reuseCount: row.reuseCount,
      }));
    },

    async insertResolution(input: InsertResolutionInput): Promise<string> {
      return db.withTenant(input.tenantId, (client) =>
        insertResolution(client, {
          incidentId: input.incidentId,
          tenantId: input.tenantId,
          actionSequence: input.actionSequence,
          success: input.success,
          durationMs: input.durationMs,
        }),
      );
    },

    async incrementReuseCount(
      tenantId: string,
      resolutionId: string,
    ): Promise<void> {
      await db.withTenant(tenantId, (client) =>
        incrementReuseCount(client, resolutionId),
      );
    },

    async insertAudit(input: InsertAuditInput): Promise<string> {
      return db.withTenant(input.tenantId, (client) =>
        insertAuditEntry(client, {
          tenantId: input.tenantId,
          module: input.module,
          actionType: input.actionType,
          target: input.target,
          triggerEventId: input.triggerEventId,
          confidence: input.confidence,
          reasoning: input.reasoning,
          outcome: input.outcome,
          durationMs: input.durationMs,
        }),
      );
    },
  };
}
