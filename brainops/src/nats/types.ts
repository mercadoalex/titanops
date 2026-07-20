// NATS message types for incident events from the correlation engine

/**
 * Incident message received from the correlation engine via NATS.
 * Matches the correlation engine output format.
 */
export interface NatsIncidentMessage {
  correlationId: string;
  tenantId: string;
  modules: string[];
  severity: number;
  narrative: string;
  confidenceScore: number;
  contributingEvents: Record<string, unknown>[];
  nodeId?: string;
  namespace?: string;
  podName?: string;
  /** ISO 8601 timestamp */
  timestamp: string;
}

/**
 * Validates and parses an unknown payload into a NatsIncidentMessage.
 * Throws if the message is missing required fields or has wrong types.
 */
export function parseIncidentMessage(data: unknown): NatsIncidentMessage {
  if (data === null || typeof data !== "object") {
    throw new Error("Invalid incident message: payload must be a non-null object");
  }

  const obj = data as Record<string, unknown>;

  const requiredStrings = ["correlationId", "tenantId", "narrative", "timestamp"] as const;
  for (const field of requiredStrings) {
    if (typeof obj[field] !== "string" || (obj[field] as string).length === 0) {
      throw new Error(`Invalid incident message: "${field}" must be a non-empty string`);
    }
  }

  if (!Array.isArray(obj.modules)) {
    throw new Error('Invalid incident message: "modules" must be an array');
  }

  if (typeof obj.severity !== "number" || !Number.isFinite(obj.severity)) {
    throw new Error('Invalid incident message: "severity" must be a finite number');
  }

  if (typeof obj.confidenceScore !== "number" || !Number.isFinite(obj.confidenceScore)) {
    throw new Error('Invalid incident message: "confidenceScore" must be a finite number');
  }

  if (!Array.isArray(obj.contributingEvents)) {
    throw new Error('Invalid incident message: "contributingEvents" must be an array');
  }

  // Optional fields
  if (obj.nodeId !== undefined && typeof obj.nodeId !== "string") {
    throw new Error('Invalid incident message: "nodeId" must be a string if provided');
  }
  if (obj.namespace !== undefined && typeof obj.namespace !== "string") {
    throw new Error('Invalid incident message: "namespace" must be a string if provided');
  }
  if (obj.podName !== undefined && typeof obj.podName !== "string") {
    throw new Error('Invalid incident message: "podName" must be a string if provided');
  }

  return {
    correlationId: obj.correlationId as string,
    tenantId: obj.tenantId as string,
    modules: obj.modules as string[],
    severity: obj.severity as number,
    narrative: obj.narrative as string,
    confidenceScore: obj.confidenceScore as number,
    contributingEvents: obj.contributingEvents as Record<string, unknown>[],
    nodeId: obj.nodeId as string | undefined,
    namespace: obj.namespace as string | undefined,
    podName: obj.podName as string | undefined,
    timestamp: obj.timestamp as string,
  };
}
