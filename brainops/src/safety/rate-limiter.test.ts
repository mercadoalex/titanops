import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";
import { RateLimiter } from "./rate-limiter.js";

describe("RateLimiter", () => {
  beforeEach(() => {
    vi.useFakeTimers();
  });

  afterEach(() => {
    vi.useRealTimers();
  });

  it("allows calls within limit", () => {
    const limiter = new RateLimiter(10);
    for (let i = 0; i < 10; i++) {
      const result = limiter.check("tenant-1");
      expect(result.allowed).toBe(true);
    }
  });

  it("blocks calls over limit", () => {
    const limiter = new RateLimiter(10);
    for (let i = 0; i < 10; i++) {
      limiter.check("tenant-1");
    }
    const result = limiter.check("tenant-1");
    expect(result.allowed).toBe(false);
    expect(result.remaining).toBe(0);
  });

  it("resets after 60s", () => {
    const limiter = new RateLimiter(10);
    for (let i = 0; i < 10; i++) {
      limiter.check("tenant-1");
    }
    expect(limiter.check("tenant-1").allowed).toBe(false);

    // Advance time by 60 seconds
    vi.advanceTimersByTime(60_001);

    const result = limiter.check("tenant-1");
    expect(result.allowed).toBe(true);
  });

  it("isolates per tenant", () => {
    const limiter = new RateLimiter(10);
    for (let i = 0; i < 10; i++) {
      limiter.check("tenant-1");
    }
    expect(limiter.check("tenant-1").allowed).toBe(false);

    // Different tenant should still have full capacity
    const result = limiter.check("tenant-2");
    expect(result.allowed).toBe(true);
  });
});
