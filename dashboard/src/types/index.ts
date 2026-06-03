/** Module health status returned by GET /api/health */
export interface ModuleHealth {
  module: string;
  status: 'operational' | 'degraded' | 'unavailable';
  since: string; // ISO 8601 timestamp
}

/** Reasoning chain for an autonomous action */
export interface ReasoningChain {
  observation: string;
  analysis: string;
  action: string;
  alternatives?: string[];
}

/** Autonomous action returned by GET /api/actions */
export interface AutonomousAction {
  id: string;
  module: string;
  action_type: string;
  confidence: number; // 0.0 - 1.0
  reasoning: ReasoningChain;
  outcome: 'success' | 'failed' | 'rejected' | 'paused';
  timestamp: string; // ISO 8601 timestamp
  override_by?: string;
}

/** Audit trail entry returned by GET /api/audit */
export interface AuditEntry {
  timestamp: string; // ISO 8601 timestamp
  module: string;
  action_type: string;
  trigger_event: string;
  confidence: number;
  reasoning: string;
  outcome: string;
  operator_id?: string;
}

/** Filter parameters for audit trail queries */
export interface AuditFilter {
  since?: string;
  module?: string;
  action_type?: string;
  page?: number;
  page_size?: number;
}

/** Correlated incident returned by GET /api/correlations */
export interface CorrelatedIncident {
  incident_id: string;
  modules: string[];
  confidence: number; // 0-100
  narrative: string;
  matched_attributes: string[];
  detected_at: string; // ISO 8601 timestamp
  action_taken: boolean;
  action_outcome?: string;
}

/** Override request body for POST /api/overrides */
export interface OverrideRequest {
  action_id: string;
  module_id: string;
  operation: 'approve' | 'reject' | 'pause' | 'resume';
  operator_id: string;
}

/** Explainability detail returned by GET /api/explain/{actionID} */
export interface ExplainDetail {
  action_id: string;
  module: string;
  confidence: number; // 0.0 - 1.0
  reasoning: ReasoningChain;
  trigger_event: string;
  timestamp: string;
}
