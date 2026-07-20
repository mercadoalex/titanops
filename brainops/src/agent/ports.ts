/**
 * Agent Store — port interface for all persistence operations needed by agent nodes.
 *
 * This is the boundary between the agent engine layer (src/agent/) and the
 * infrastructure layer (src/db/). Agent nodes depend ONLY on this interface,
 * never on DbClient, pg, or query functions directly.
 *
 * Implementations:
 * - DbAgentStore (src/agent/store.ts)      — real CockroachDB adapter
 * - MockAgentStore (src/agent/ports.mock.ts) — deterministic in-memory mock
 */

import type {
  SimilarIncident,
  RemediationAction,
  ReasoningResult,
} from "./state.js";

// --- Incident Operations ---

export interface InsertIncidentInput {
  tenantId: string;
  correlationId: string;
  modules: string[];
  severity: number;
  narrative: string;
  contributingEvents: Record<string, unknown>[];
  confidenceScore: number;
  nodeId?: string;
  namespace?: string;
  podName?: string;
}

// --- Similarity Search Operations ---

export interface SearchSimilarInput {
  tenantId: string;
  embedding: number[];
  limit?: number;
  minSimilarity?: number;
}

// --- Resolution Operations ---

export interface InsertResolutionInput {
  tenantId: string;
  incidentId: string;
  actionSequence: RemediationAction[];
  success: boolean;
  durationMs: number;
}

export interface ResolutionRecord {
  id: string;
  incidentId: string;
  actionSequence: unknown;
  success: boolean;
  durationMs: number;
  reuseCount: number;
}

// --- Audit Operations ---

export interface InsertAuditInput {
  tenantId: string;
  module: string;
  actionType: string;
  target: string;
  triggerEventId?: string;
  confidence: number;
  reasoning: ReasoningResult | Record<string, unknown>;
  outcome: string;
  durationMs?: number;
}

// --- The Port Interface ---

/**
 * AgentStore defines all persistence operations the agent engine needs.
 * No infrastructure types (pg.PoolClient, DbClient) leak through this boundary.
 */
export interface AgentStore {
  /**
   * Persists a new correlated incident. Returns the generated ID.
   */
  insertIncident(input: InsertIncidentInput): Promise<string>;

  /**
   * Searches for similar past incidents using vector similarity.
   */
  searchSimilar(input: SearchSimilarInput): Promise<SimilarIncident[]>;

  /**
   * Retrieves resolution playbooks for a given incident ID.
   */
  getResolutionsByIncident(
    tenantId: string,
    incidentId: string,
  ): Promise<ResolutionRecord[]>;

  /**
   * Stores a new resolution playbook. Returns the generated ID.
   */
  insertResolution(input: InsertResolutionInput): Promise<string>;

  /**
   * Increments the reuse count on an existing resolution.
   */
  incrementReuseCount(tenantId: string, resolutionId: string): Promise<void>;

  /**
   * Writes an entry to the audit trail.
   */
  insertAudit(input: InsertAuditInput): Promise<string>;
}
