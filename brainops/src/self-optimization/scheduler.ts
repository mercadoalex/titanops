// 6-hour optimization cycle scheduler

/**
 * Simple interval-based scheduler for self-optimization cycles.
 * Uses setInterval rather than a cron library to avoid extra dependencies.
 *
 * The cronExpression is parsed into a millisecond interval for simplicity.
 */
export class SelfOptimizationScheduler {
  private readonly intervalMs: number;
  private readonly runner: () => Promise<void>;
  private timer: ReturnType<typeof setInterval> | null = null;

  constructor(cronExpression: string, runner: () => Promise<void>) {
    this.intervalMs = parseCronToIntervalMs(cronExpression);
    this.runner = runner;
  }

  /**
   * Starts the scheduled optimization cycle.
   */
  start(): void {
    if (this.timer !== null) return;
    this.timer = setInterval(() => {
      void this.runner();
    }, this.intervalMs);
  }

  /**
   * Stops the scheduled optimization cycle.
   */
  stop(): void {
    if (this.timer !== null) {
      clearInterval(this.timer);
      this.timer = null;
    }
  }

  /**
   * Manual trigger for testing or on-demand optimization.
   */
  async runNow(): Promise<void> {
    await this.runner();
  }
}

/**
 * Parses a simplified cron expression into an interval in milliseconds.
 *
 * Supports common patterns:
 * - "0 * /N * * *" (every N hours) → extracts N hours in ms
 * - Falls back to 6 hours if the pattern is unrecognized.
 */
function parseCronToIntervalMs(cron: string): number {
  const SIX_HOURS_MS = 6 * 60 * 60 * 1000;

  const parts = cron.trim().split(/\s+/);
  if (parts.length < 5) return SIX_HOURS_MS;

  const hourField = parts[1];

  // Match */N pattern (every N hours)
  const everyNMatch = hourField.match(/^\*\/(\d+)$/);
  if (everyNMatch) {
    const hours = parseInt(everyNMatch[1], 10);
    return hours * 60 * 60 * 1000;
  }

  // Match single hour value (run once a day at that hour — use 24h interval)
  if (/^\d+$/.test(hourField)) {
    return 24 * 60 * 60 * 1000;
  }

  return SIX_HOURS_MS;
}
