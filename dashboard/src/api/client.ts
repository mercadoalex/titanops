import type {
  ModuleHealth,
  AutonomousAction,
  AuditEntry,
  AuditFilter,
  CorrelatedIncident,
  OverrideRequest,
  ExplainDetail,
} from '../types';

const BASE = '/api';

async function request<T>(path: string, init?: RequestInit): Promise<T> {
  const res = await fetch(`${BASE}${path}`, {
    headers: { 'Content-Type': 'application/json' },
    ...init,
  });
  if (!res.ok) {
    throw new Error(`API error ${res.status}: ${res.statusText}`);
  }
  return res.json() as Promise<T>;
}

/** Fetch health status of all modules. */
export function fetchHealth(): Promise<ModuleHealth[]> {
  return request<ModuleHealth[]>('/health');
}

/** Fetch recent autonomous actions. */
export function fetchActions(limit = 50): Promise<AutonomousAction[]> {
  return request<AutonomousAction[]>(`/actions?limit=${limit}`);
}

/** Fetch explainability detail for a specific action. */
export function fetchExplain(actionID: string): Promise<ExplainDetail> {
  return request<ExplainDetail>(`/explain/${encodeURIComponent(actionID)}`);
}

/** Fetch correlated incidents within a time window (minutes). */
export function fetchCorrelations(windowMinutes = 60): Promise<CorrelatedIncident[]> {
  return request<CorrelatedIncident[]>(`/correlations?window=${windowMinutes}m`);
}

/** Submit an operator override (approve, reject, pause, resume). */
export function postOverride(override: OverrideRequest): Promise<void> {
  return request<void>('/overrides', {
    method: 'POST',
    body: JSON.stringify(override),
  });
}

/** Fetch audit trail entries with optional filters. */
export function fetchAudit(filter: AuditFilter = {}): Promise<AuditEntry[]> {
  const params = new URLSearchParams();
  if (filter.since) params.set('since', filter.since);
  if (filter.module) params.set('module', filter.module);
  if (filter.action_type) params.set('action_type', filter.action_type);
  if (filter.page !== undefined) params.set('page', String(filter.page));
  if (filter.page_size !== undefined) params.set('page_size', String(filter.page_size));
  const qs = params.toString();
  return request<AuditEntry[]>(`/audit${qs ? `?${qs}` : ''}`);
}
