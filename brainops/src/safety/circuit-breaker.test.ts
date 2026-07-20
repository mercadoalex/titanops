import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";
import { CircuitBreaker } from "./circuit-breaker.js";

describe("CircuitBreaker", () => {
  beforeEach(() => {
    vi.useFakeTimers();
  });

  afterEach(() => {
    vi.useRealTimers();
  });

  it("isOpen is false initially", () => {
    const breaker = new CircuitBreaker(3, 60_000);
    expect(breaker.isOpen("tenant-1")).toBe(false);
  });

  it("isOpen is true after threshold failures within window", () => {
    const breaker = new CircuitBreaker(3, 60_000);
    breaker.recordFailure("tenant-1");
    breaker.recordFailure("tenant-1");
    breaker.recordFailure("tenant-1");
    expect(breaker.isOpen("tenant-1")).toBe(true);
  });

  it("isOpen is false after window expires", () => {
    const breaker = new CircuitBreaker(3, 60_000);
    breaker.recordFailure("tenant-1");
    breaker.recordFailure("tenant-1");
    breaker.recordFailure("tenant-1");
    expect(breaker.isOpen("tenant-1")).toBe(true);

    // Advance past the window
    vi.advanceTimersByTime(60_001);

    expect(breaker.isOpen("tenant-1")).toBe(false);
  });

  it("reset() clears the breaker", () => {
    const breaker = new CircuitBreaker(3, 60_000);
    breaker.recordFailure("tenant-1");
    breaker.recordFailure("tenant-1");
    breaker.recordFailure("tenant-1");
    expect(breaker.isOpen("tenant-1")).toBe(true);

    breaker.reset("tenant-1");
    expect(breaker.isOpen("tenant-1")).toBe(false);
  });
});
