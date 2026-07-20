// ccloud CLI wrapper (JSON output)

import { execFile } from "node:child_process";
import { promisify } from "node:util";

const execFileAsync = promisify(execFile);

export interface ClusterHealth {
  state: string;
  nodeCount: number;
  regions: string[];
  version: string;
}

export interface BackupInfo {
  id: string;
  state: string;
  createdAt: Date;
}

/**
 * Wrapper around the `ccloud` CLI for CockroachDB Cloud operations.
 * All commands are executed with --format json for structured output.
 */
export class CcloudClient {
  private readonly clusterId: string;
  private readonly serviceAccountKey: string | undefined;

  constructor(clusterId: string, serviceAccountKey?: string) {
    this.clusterId = clusterId;
    this.serviceAccountKey = serviceAccountKey;
  }

  /**
   * Spawns the `ccloud` CLI with given arguments and parses JSON output.
   */
  private async exec(args: string[]): Promise<Record<string, unknown>> {
    const fullArgs = [...args, "--format", "json"];

    if (this.serviceAccountKey) {
      fullArgs.push("--api-key", this.serviceAccountKey);
    }

    try {
      const { stdout } = await execFileAsync("ccloud", fullArgs);
      return JSON.parse(stdout) as Record<string, unknown>;
    } catch (error: unknown) {
      const err = error as { stderr?: string; message?: string };
      const message = err.stderr?.trim() || err.message || "ccloud command failed";
      throw new Error(`ccloud error: ${message}`);
    }
  }

  /**
   * Describes the cluster and returns health information.
   */
  async clusterDescribe(): Promise<ClusterHealth> {
    const result = await this.exec(["cluster", "describe", this.clusterId]);

    const state = result["state"] as string | undefined;
    if (!state) {
      throw new Error("Unable to retrieve cluster state from ccloud output");
    }

    return {
      state,
      nodeCount: (result["node_count"] as number) ?? 0,
      regions: (result["regions"] as string[]) ?? [],
      version: (result["version"] as string) ?? "unknown",
    };
  }

  /**
   * Lists backups for the cluster.
   */
  async backupList(): Promise<BackupInfo[]> {
    const result = await this.exec([
      "backup",
      "list",
      "--cluster",
      this.clusterId,
    ]);

    const backups = (result["backups"] as Record<string, unknown>[]) ?? [];

    return backups.map((b) => ({
      id: b["id"] as string,
      state: b["state"] as string,
      createdAt: new Date(b["created_at"] as string),
    }));
  }

  /**
   * Returns true if the most recent backup is less than 24 hours old.
   */
  async isBackupHealthy(): Promise<boolean> {
    const backups = await this.backupList();
    if (backups.length === 0) return false;

    // Sort by createdAt descending to find the most recent
    const sorted = backups.sort(
      (a, b) => b.createdAt.getTime() - a.createdAt.getTime(),
    );
    const latest = sorted[0];

    const twentyFourHoursMs = 24 * 60 * 60 * 1000;
    const age = Date.now() - latest.createdAt.getTime();

    return age < twentyFourHoursMs;
  }
}
