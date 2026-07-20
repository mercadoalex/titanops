/**
 * MockAgentStore — deterministic in-memory implementation of AgentStore.
 *
 * Used in mock mode (TITANOPS_MODE=mock) and in unit tests. All data is held
 * in memory with no external dependencies. Given the same sequence of calls,
 * outputs are deterministic.
 */

import type {
  AgentStore,
  InsertIncidentInput,
  SearchSimilarInput,
  ResolutionRecord,
  InsertResolutionInput,
  InsertAuditInput,
} from "./ports.js";
import type { SimilarIncident } from "./state.js";

/** Stored incident record in the mock. */
export interface MockIncidentRecord {
  id: string;
  input: InsertIncidentInput;
  createdAt: Date;
}

/** Stored audit record in the mock. */
export interface MockAuditRecord {
  id: string;
  input: InsertAuditInput;
  createdAt: Date;
}

/** Configuration for the mock store. */
export interface MockAgentStoreConfig {
  /** Pre-seeded similar incidents returned by searchSimilar. Default: empty. */
  similarIncidents?: SimilarIncident[];
  /** Pre-seeded resolutions indexed by incidentId. Default: empty. */
  resolutions?: Map<string, ResolutionRecord[]>;
}

/**
 * Creates an in-memory AgentStore for testing and mock mode.
 * All writes are captured and queryable via the returned store's public arrays.
 */
export function createMockAgentStore(
  config: MockAgentStoreConfig = {},
): MockAgentStore {
  return new MockAgentStore(config);
}

export class MockAgentStore implements AgentStore {
  /** All incidents inserted during the session. */
  readonly incidents: MockIncidentRecord[] = [];
  /** All audit entries inserted during the session. */
  readonly audits: MockAuditRecord[] = [];
  /** All resolutions inserted during the session. */
  readonly insertedResolutions: ResolutionRecord[] = [];
  /** Tracks incrementReuseCount calls: resolutionId → count of increments. */
  readonly reuseIncrements: Map<string, number> = new Map();

  private nextId = 1;
  private similarIncidents: SimilarIncident[];
  private resolutions: Map<string, ResolutionRecord[]>;

  constructor(config: MockAgentStoreConfig = {}) {
    this.similarIncidents = config.similarIncidents ?? [];
    this.resolutions = config.resolutions ?? new Map();
  }

  async insertIncident(input: InsertIncidentInput): Promise<string> {
    const id = `mock-incident-${this.nextId++}`;
    this.incidents.push({ id, input, createdAt: new Date() });
    return id;
  }

  async searchSimilar(_input: SearchSimilarInput): Promise<SimilarIncident[]> {
    return this.similarIncidents;
  }

  async getResolutionsByIncident(
    _tenantId: string,
    incidentId: string,
  ): Promise<ResolutionRecord[]> {
    return this.resolutions.get(incidentId) ?? [];
  }

  async insertResolution(input: InsertResolutionInput): Promise<string> {
    const id = `mock-resolution-${this.nextId++}`;
    const record: ResolutionRecord = {
      id,
      incidentId: input.incidentId,
      actionSequence: input.actionSequence,
      success: input.success,
      durationMs: input.durationMs,
      reuseCount: 0,
    };
    this.insertedResolutions.push(record);

    // Also index it so future getResolutionsByIncident calls find it
    const existing = this.resolutions.get(input.incidentId) ?? [];
    existing.push(record);
    this.resolutions.set(input.incidentId, existing);

    return id;
  }

  async incrementReuseCount(
    _tenantId: string,
    resolutionId: string,
  ): Promise<void> {
    const current = this.reuseIncrements.get(resolutionId) ?? 0;
    this.reuseIncrements.set(resolutionId, current + 1);

    // Also update the in-memory resolution record
    for (const [, records] of this.resolutions) {
      for (const rec of records) {
        if (rec.id === resolutionId) {
          rec.reuseCount++;
        }
      }
    }
  }

  async insertAudit(input: InsertAuditInput): Promise<string> {
    const id = `mock-audit-${this.nextId++}`;
    this.audits.push({ id, input, createdAt: new Date() });
    return id;
  }

  /**
   * Seed similar incidents for subsequent searchSimilar calls.
   * Useful for setting up test scenarios.
   */
  seedSimilarIncidents(incidents: SimilarIncident[]): void {
    this.similarIncidents = incidents;
  }

  /**
   * Seed resolution records for a given incident ID.
   */
  seedResolutions(incidentId: string, records: ResolutionRecord[]): void {
    this.resolutions.set(incidentId, records);
  }

  /**
   * Reset all state — clear stored data and counters.
   */
  reset(): void {
    this.incidents.length = 0;
    this.audits.length = 0;
    this.insertedResolutions.length = 0;
    this.reuseIncrements.clear();
    this.similarIncidents = [];
    this.resolutions.clear();
    this.nextId = 1;
  }
}
