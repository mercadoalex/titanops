// Circuit breaker — pauses actions after repeated failures within a time window

/**
 * Per-tenant circuit breaker that opens after a threshold of failures
 * within a configurable time window.
 */
export class CircuitBreaker {
  private threshold: number;
  private windowMs: number;
  private failures: Map<string, number[]> = new Map();

  constructor(threshold: number, windowMs: number) {
    this.threshold = threshold;
    this.windowMs = windowMs;
  }

  /**
   * Records a failure timestamp for the given tenant.
   */
  recordFailure(tenantId: string): void {
    const timestamps = this.failures.get(tenantId) ?? [];
    timestamps.push(Date.now());
    this.failures.set(tenantId, timestamps);
  }

  /**
   * Returns true if the circuit is open (failures >= threshold within window).
   */
  isOpen(tenantId: string): boolean {
    const now = Date.now();
    const windowStart = now - this.windowMs;

    const timestamps = this.failures.get(tenantId) ?? [];
    const recentFailures = timestamps.filter((ts) => ts > windowStart);

    // Prune old entries while we're here
    this.failures.set(tenantId, recentFailures);

    return recentFailures.length >= this.threshold;
  }

  /**
   * Manual reset — clears all failure history for a tenant (operator action).
   */
  reset(tenantId: string): void {
    this.failures.delete(tenantId);
  }
}
