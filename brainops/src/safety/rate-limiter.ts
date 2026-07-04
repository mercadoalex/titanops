// Sliding window rate limiter (per tenant)

/**
 * In-memory sliding window rate limiter.
 * Tracks action timestamps per tenant and allows/rejects based on a per-minute limit.
 */
export class RateLimiter {
  private maxPerMinute: number;
  private windows: Map<string, number[]> = new Map();

  constructor(maxPerMinute: number) {
    this.maxPerMinute = maxPerMinute;
  }

  /**
   * Checks whether the tenant is allowed to perform an action.
   * Prunes timestamps older than 60 seconds and evaluates remaining capacity.
   */
  check(tenantId: string): { allowed: boolean; remaining: number } {
    const now = Date.now();
    const windowStart = now - 60_000;

    let timestamps = this.windows.get(tenantId) ?? [];

    // Prune entries older than 60 seconds
    timestamps = timestamps.filter((ts) => ts > windowStart);

    const remaining = Math.max(0, this.maxPerMinute - timestamps.length);
    const allowed = timestamps.length < this.maxPerMinute;

    if (allowed) {
      timestamps.push(now);
    }

    this.windows.set(tenantId, timestamps);

    return { allowed, remaining: allowed ? remaining - 1 : 0 };
  }
}
