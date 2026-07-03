import { Annotation } from "@langchain/langgraph";

/**
 * Correlated incident received from the correlation engine via NATS.
 */
export interface CorrelatedIncident {
  id: string;
  modules: string[];
  severity: number;
  narrative: string;
  confidenceScore: number;
  nodeId?: string;
  namespace?: string;
  podName?: string;
  contributingEvents: Record<string, unknown>[];
  createdAt: Date;
}

/**
 * A past incident found via vector similarity search.
 */
export interface SimilarIncident {
  incidentId: string;
  /** Cosine similarity score (0-1). */
  similarity: number;
  narrative: string;
  resolutionAction?: string;
  resolutionTimeMs?: number;
  modules: string[];
}

/**
 * Stored resolution playbook describing what fixed a past incident.
 */
export interface ResolutionPlaybook {
  id: string;
  incidentId: string;
  actionSequence: RemediationAction[];
  success: boolean;
  durationMs: number;
  reuseCount: number;
}

/**
 * Result of the LLM reasoning step.
 */
export interface ReasoningResult {
  observation: string;
  analysis: string;
  selectedAction: string;
  alternatives: string[];
  /** Confidence score (0-1). */
  confidence: number;
  /** What memory informed the decision. */
  memoryContext: string;
}

/**
 * A remediation action to be executed against the cluster.
 */
export interface RemediationAction {
  /** e.g. pod_restart, node_cordon, cert_renew */
  actionType: string;
  target: string;
  reason: string;
  riskLevel: "low" | "medium" | "high";
}

/**
 * Outcome of an executed remediation action.
 */
export interface ActionOutcome {
  success: boolean;
  durationMs: number;
  error?: string;
}

/**
 * LangGraph agent state definition using Annotation.Root.
 *
 * Each field is persisted via the CockroachDB checkpointer so the agent
 * can resume from any step after a pod restart.
 */
export const AgentState = Annotation.Root({
  /** Current incident being processed. */
  incident: Annotation<CorrelatedIncident | null>,
  /** Similar past incidents found via vector search. */
  similarIncidents: Annotation<SimilarIncident[]>,
  /** Resolution playbook (if found). */
  playbook: Annotation<ResolutionPlaybook | null>,
  /** Agent's reasoning output. */
  reasoning: Annotation<ReasoningResult | null>,
  /** Action to execute (or executed). */
  action: Annotation<RemediationAction | null>,
  /** Outcome of the action. */
  outcome: Annotation<ActionOutcome | null>,
  /** Tenant context for RLS enforcement. */
  tenantId: Annotation<string>,
  /** Current node in the state graph. */
  currentStep: Annotation<string>,
});

/** Inferred type for the agent state. */
export type AgentStateType = typeof AgentState.State;
