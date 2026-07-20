// Range distribution check

import type { DbClient } from "../db/client.js";

export interface HotRange {
  rangeId: number;
  sizeMb: number;
  pctOfTotal: number;
}

export interface RangeAnalysis {
  totalRanges: number;
  hotRanges: HotRange[];
  leaseholderImbalanced: boolean;
  recommendation: string;
}

/**
 * Analyzes CockroachDB range distribution for hotspot detection.
 */
export class RangeAnalyzer {
  private readonly db: DbClient;

  constructor(db: DbClient) {
    this.db = db;
  }

  /**
   * Runs SHOW RANGES for a given table and analyzes the distribution.
   * A range is considered "hot" if it holds >30% of the total table data.
   */
  async analyzeTable(
    tenantId: string,
    table: string,
  ): Promise<RangeAnalysis> {
    return this.db.withTenant(tenantId, async (client) => {
      const result = await client.query(
        `SHOW RANGES FROM TABLE ollinai.${table}`,
      );

      const ranges = result.rows;
      const totalRanges = ranges.length;

      if (totalRanges === 0) {
        return {
          totalRanges: 0,
          hotRanges: [],
          leaseholderImbalanced: false,
          recommendation: "No ranges found for the table.",
        };
      }

      // Calculate total size across all ranges
      const rangeSizes = ranges.map((r) => ({
        rangeId: parseInt(r.range_id as string, 10),
        sizeMb: parseFloat(r.range_size_mb as string) || 0,
        leaseHolder: parseInt(r.lease_holder as string, 10),
      }));

      const totalSizeMb = rangeSizes.reduce((sum, r) => sum + r.sizeMb, 0);

      // Identify hot ranges (>30% of total)
      const hotRanges: HotRange[] = [];
      for (const range of rangeSizes) {
        const pct = totalSizeMb > 0 ? (range.sizeMb / totalSizeMb) * 100 : 0;
        if (pct > 30) {
          hotRanges.push({
            rangeId: range.rangeId,
            sizeMb: range.sizeMb,
            pctOfTotal: Math.round(pct * 100) / 100,
          });
        }
      }

      // Check leaseholder imbalance — if one node holds >50% of leases
      const leaseHolderCounts = new Map<number, number>();
      for (const range of rangeSizes) {
        const count = leaseHolderCounts.get(range.leaseHolder) ?? 0;
        leaseHolderCounts.set(range.leaseHolder, count + 1);
      }

      let leaseholderImbalanced = false;
      for (const count of leaseHolderCounts.values()) {
        if (count / totalRanges > 0.5) {
          leaseholderImbalanced = true;
          break;
        }
      }

      // Generate recommendation
      const recommendation = generateRecommendation(
        hotRanges,
        leaseholderImbalanced,
      );

      return {
        totalRanges,
        hotRanges,
        leaseholderImbalanced,
        recommendation,
      };
    });
  }
}

function generateRecommendation(
  hotRanges: HotRange[],
  leaseholderImbalanced: boolean,
): string {
  const issues: string[] = [];

  if (hotRanges.length > 0) {
    issues.push(
      `${hotRanges.length} hot range(s) detected (>30% of data). Consider splitting or rebalancing.`,
    );
  }

  if (leaseholderImbalanced) {
    issues.push(
      "Leaseholder imbalance detected. Consider running ALTER TABLE ... SCATTER to redistribute.",
    );
  }

  if (issues.length === 0) {
    return "Range distribution is healthy. No action required.";
  }

  return issues.join(" ");
}
