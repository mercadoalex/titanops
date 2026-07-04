// CockroachDB Cloud MCP client — read-only SQL via managed HTTP/SSE endpoint

export interface SchemaInfo {
  tables: TableInfo[];
}

export interface TableInfo {
  tableName: string;
  columns: ColumnInfo[];
}

export interface ColumnInfo {
  columnName: string;
  dataType: string;
  isNullable: boolean;
}

/**
 * Managed MCP client for CockroachDB Cloud.
 * Provides read-only SQL access through the managed MCP HTTP endpoint.
 */
export class ManagedMcpClient {
  private readonly endpoint: string;
  private readonly apiKey: string;

  constructor(endpoint: string, apiKey: string) {
    this.endpoint = endpoint;
    this.apiKey = apiKey;
  }

  /**
   * Executes a read-only SQL query via the managed MCP endpoint.
   */
  async executeSql(
    sql: string,
    params?: unknown[],
  ): Promise<Record<string, unknown>[]> {
    const response = await fetch(`${this.endpoint}/sql`, {
      method: "POST",
      headers: {
        "Content-Type": "application/json",
        Authorization: `Bearer ${this.apiKey}`,
      },
      body: JSON.stringify({ sql, params: params ?? [] }),
    });

    if (!response.ok) {
      const body = await response.text();
      throw new Error(
        `MCP SQL request failed (${response.status}): ${body}`,
      );
    }

    const result = (await response.json()) as { rows: Record<string, unknown>[] };
    return result.rows;
  }

  /**
   * Discovers the database schema by querying information_schema.
   */
  async discoverSchema(): Promise<SchemaInfo> {
    const rows = await this.executeSql(
      `SELECT table_name, column_name, data_type, is_nullable
       FROM information_schema.columns
       WHERE table_schema = 'ollinai'
       ORDER BY table_name, ordinal_position`,
    );

    const tableMap = new Map<string, ColumnInfo[]>();

    for (const row of rows) {
      const tableName = row["table_name"] as string;
      const column: ColumnInfo = {
        columnName: row["column_name"] as string,
        dataType: row["data_type"] as string,
        isNullable: (row["is_nullable"] as string) === "YES",
      };
      const existing = tableMap.get(tableName);
      if (existing) {
        existing.push(column);
      } else {
        tableMap.set(tableName, [column]);
      }
    }

    const tables: TableInfo[] = [];
    for (const [tableName, columns] of tableMap) {
      tables.push({ tableName, columns });
    }

    return { tables };
  }

  /**
   * Health check — verifies the managed MCP endpoint is reachable.
   */
  async isConnected(): Promise<boolean> {
    try {
      await this.executeSql("SELECT 1");
      return true;
    } catch {
      return false;
    }
  }

  /**
   * Executes a read-only query, rejecting any write operations.
   * Only SELECT, SHOW, and EXPLAIN statements are allowed.
   */
  async executeReadOnlyQuery(
    sql: string,
  ): Promise<Record<string, unknown>[]> {
    assertReadOnly(sql);
    return this.executeSql(sql);
  }

  /**
   * Returns column names and types for a specific table.
   */
  async getTableSchema(tableName: string): Promise<ColumnInfo[]> {
    const rows = await this.executeSql(
      `SELECT column_name, data_type, is_nullable
       FROM information_schema.columns
       WHERE table_schema = 'ollinai' AND table_name = $1
       ORDER BY ordinal_position`,
      [tableName],
    );

    return rows.map((row) => ({
      columnName: row["column_name"] as string,
      dataType: row["data_type"] as string,
      isNullable: (row["is_nullable"] as string) === "YES",
    }));
  }
}

/**
 * Throws if the SQL is not a read-only statement.
 * Only SELECT, SHOW, and EXPLAIN are permitted.
 */
function assertReadOnly(sql: string): void {
  const trimmed = sql.trim().toUpperCase();
  const allowed = ["SELECT", "SHOW", "EXPLAIN"];
  const firstWord = trimmed.split(/\s+/)[0];

  if (!firstWord || !allowed.includes(firstWord)) {
    throw new Error(
      `Write operations are not permitted. Only SELECT, SHOW, and EXPLAIN statements are allowed. Got: ${firstWord ?? "(empty)"}`,
    );
  }
}
