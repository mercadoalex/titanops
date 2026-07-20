// Custom MCP server — tool definitions and registration

import { Server } from "@modelcontextprotocol/sdk/server/index.js";
import { StdioServerTransport } from "@modelcontextprotocol/sdk/server/stdio.js";
import {
  CallToolRequestSchema,
  ListToolsRequestSchema,
} from "@modelcontextprotocol/sdk/types.js";
import type { DbClient } from "../db/client.js";
import { handleSearchSimilar } from "./tools/search-similar.js";
import { handleQueryHistory } from "./tools/query-history.js";
import { handleGetPlaybook } from "./tools/get-playbook.js";
import { handleExecuteRemediation } from "./tools/execute-remediation.js";
import { handleStoreResolution } from "./tools/store-resolution.js";
import { handleGetNodeHistory } from "./tools/get-node-history.js";

/**
 * Tool definitions with JSON Schema input validation.
 */
const TOOL_DEFINITIONS = [
  {
    name: "search_similar_incidents",
    description:
      "Search for past incidents similar to the given description using vector similarity.",
    inputSchema: {
      type: "object" as const,
      properties: {
        tenantId: { type: "string", description: "Tenant ID for RLS context" },
        description: { type: "string", description: "Incident description to search against" },
        limit: { type: "number", description: "Maximum number of results (default: 10)" },
        minSimilarity: {
          type: "number",
          description: "Minimum cosine similarity threshold (default: 0.7)",
        },
      },
      required: ["tenantId", "description"],
    },
  },
  {
    name: "query_incident_history",
    description: "Query incident history with optional filters for node, namespace, and time range.",
    inputSchema: {
      type: "object" as const,
      properties: {
        tenantId: { type: "string", description: "Tenant ID for RLS context" },
        nodeId: { type: "string", description: "Filter by Kubernetes node ID" },
        namespace: { type: "string", description: "Filter by Kubernetes namespace" },
        timeRangeDays: { type: "number", description: "Lookback window in days (default: 30)" },
        limit: { type: "number", description: "Maximum number of results (default: 50)" },
      },
      required: ["tenantId"],
    },
  },
  {
    name: "get_resolution_playbook",
    description: "Get resolution playbooks associated with a specific incident.",
    inputSchema: {
      type: "object" as const,
      properties: {
        tenantId: { type: "string", description: "Tenant ID for RLS context" },
        incidentId: { type: "string", description: "Incident ID to look up resolutions for" },
      },
      required: ["tenantId", "incidentId"],
    },
  },
  {
    name: "execute_remediation",
    description:
      "Execute a remediation action. High-risk actions (node_drain, pod_delete on production) require approval.",
    inputSchema: {
      type: "object" as const,
      properties: {
        tenantId: { type: "string", description: "Tenant ID for RLS context" },
        actionType: {
          type: "string",
          description: "Type of remediation action (e.g., pod_restart, node_drain, cert_renew)",
        },
        target: {
          type: "string",
          description: "Target resource identifier (e.g., node name, pod name)",
        },
        reason: { type: "string", description: "Reason for taking this action" },
      },
      required: ["tenantId", "actionType", "target", "reason"],
    },
  },
  {
    name: "store_resolution",
    description: "Store a resolution record documenting how an incident was resolved.",
    inputSchema: {
      type: "object" as const,
      properties: {
        tenantId: { type: "string", description: "Tenant ID for RLS context" },
        incidentId: { type: "string", description: "Incident ID this resolution belongs to" },
        actions: {
          type: "array",
          items: { type: "object" },
          description: "Sequence of actions taken to resolve the incident",
        },
        success: { type: "boolean", description: "Whether the resolution was successful" },
        durationMs: { type: "number", description: "Total duration of resolution in milliseconds" },
      },
      required: ["tenantId", "incidentId", "actions", "success", "durationMs"],
    },
  },
  {
    name: "get_node_history",
    description:
      "Get combined incident and audit history for a specific node, sorted by time.",
    inputSchema: {
      type: "object" as const,
      properties: {
        tenantId: { type: "string", description: "Tenant ID for RLS context" },
        nodeId: { type: "string", description: "Kubernetes node ID to query history for" },
        days: { type: "number", description: "Lookback window in days (default: 30)" },
      },
      required: ["tenantId", "nodeId"],
    },
  },
];

/**
 * Creates and configures the BrainOps MCP server with all tool handlers.
 */
export function createMcpServer(db: DbClient): Server {
  const server = new Server(
    { name: "brainops", version: "0.1.0" },
    { capabilities: { tools: {} } },
  );

  // Register tool list handler
  server.setRequestHandler(ListToolsRequestSchema, async () => ({
    tools: TOOL_DEFINITIONS,
  }));

  // Register tool call handler
  server.setRequestHandler(CallToolRequestSchema, async (request) => {
    const { name, arguments: args } = request.params;
    const tenantId = (args as Record<string, unknown>)?.tenantId as string;

    if (!tenantId) {
      return {
        content: [{ type: "text", text: JSON.stringify({ error: "tenantId is required" }) }],
        isError: true,
      };
    }

    try {
      let result: unknown;

      switch (name) {
        case "search_similar_incidents":
          result = await handleSearchSimilar(db, tenantId, {
            description: (args as Record<string, unknown>).description as string,
            limit: (args as Record<string, unknown>).limit as number | undefined,
            minSimilarity: (args as Record<string, unknown>).minSimilarity as number | undefined,
          });
          break;

        case "query_incident_history":
          result = await handleQueryHistory(db, tenantId, {
            nodeId: (args as Record<string, unknown>).nodeId as string | undefined,
            namespace: (args as Record<string, unknown>).namespace as string | undefined,
            timeRangeDays: (args as Record<string, unknown>).timeRangeDays as number | undefined,
            limit: (args as Record<string, unknown>).limit as number | undefined,
          });
          break;

        case "get_resolution_playbook":
          result = await handleGetPlaybook(db, tenantId, {
            incidentId: (args as Record<string, unknown>).incidentId as string,
          });
          break;

        case "execute_remediation":
          result = await handleExecuteRemediation(db, tenantId, {
            actionType: (args as Record<string, unknown>).actionType as string,
            target: (args as Record<string, unknown>).target as string,
            reason: (args as Record<string, unknown>).reason as string,
          });
          break;

        case "store_resolution":
          result = await handleStoreResolution(db, tenantId, {
            incidentId: (args as Record<string, unknown>).incidentId as string,
            actions: (args as Record<string, unknown>).actions as unknown[],
            success: (args as Record<string, unknown>).success as boolean,
            durationMs: (args as Record<string, unknown>).durationMs as number,
          });
          break;

        case "get_node_history":
          result = await handleGetNodeHistory(db, tenantId, {
            nodeId: (args as Record<string, unknown>).nodeId as string,
            days: (args as Record<string, unknown>).days as number | undefined,
          });
          break;

        default:
          return {
            content: [{ type: "text", text: JSON.stringify({ error: `Unknown tool: ${name}` }) }],
            isError: true,
          };
      }

      return {
        content: [{ type: "text", text: JSON.stringify(result) }],
      };
    } catch (error) {
      const message = error instanceof Error ? error.message : "Unknown error";
      return {
        content: [{ type: "text", text: JSON.stringify({ error: message }) }],
        isError: true,
      };
    }
  });

  return server;
}

/**
 * Starts the MCP server with stdio transport.
 * Used when running the server as a standalone process.
 */
export async function startMcpServer(db: DbClient): Promise<void> {
  const server = createMcpServer(db);
  const transport = new StdioServerTransport();
  await server.connect(transport);
}
