// LangGraph state graph definition — wires all agent nodes into a compiled graph

import { END, StateGraph } from "@langchain/langgraph";
import { createCheckpointer } from "./checkpointer.js";
import { AgentState, type AgentStateType } from "./state.js";
import { createReceiveNode } from "./nodes/receive.js";
import { createSearchMemoryNode } from "./nodes/search-memory.js";
import { createReasonNode } from "./nodes/reason.js";
import { createActNode } from "./nodes/act.js";
import { createRememberNode } from "./nodes/remember.js";
import { createDbClient } from "../db/client.js";
import { loadConfig } from "../config/index.js";

/**
 * Creates and compiles the BrainOps agent state graph.
 *
 * Flow: receive → search_memory → reason → act → remember → END
 * Conditional edge from "act": if awaiting_approval → END (human-in-the-loop)
 *
 * @param connectionString - CockroachDB connection string for the checkpointer
 * @returns The compiled LangGraph runnable
 */
export async function createAgentGraph(connectionString: string) {
  // Initialize checkpointer for durable state
  const checkpointer = await createCheckpointer(connectionString);

  // Create DB client for node dependency injection
  const config = loadConfig();
  const db = createDbClient(config);

  // Build node functions via factories
  const receiveNode = createReceiveNode(db);
  const searchMemoryNode = createSearchMemoryNode(db);
  const reasonNode = createReasonNode(db);
  const actNode = createActNode(db);
  const rememberNode = createRememberNode(db);

  // Construct state graph
  const graph = new StateGraph(AgentState)
    .addNode("receive", receiveNode)
    .addNode("search_memory", searchMemoryNode)
    .addNode("reason", reasonNode)
    .addNode("act", actNode)
    .addNode("remember", rememberNode)
    .addEdge("__start__", "receive")
    .addEdge("receive", "search_memory")
    .addEdge("search_memory", "reason")
    .addEdge("reason", "act")
    .addConditionalEdges("act", (state: AgentStateType) => {
      if (state.currentStep === "awaiting_approval") {
        return "__end__";
      }
      return "remember";
    }, {
      __end__: END,
      remember: "remember",
    })
    .addEdge("remember", "__end__");

  // Compile with checkpointer for durable execution
  // Type assertion needed due to version mismatch between langgraph and checkpoint-postgres
  // eslint-disable-next-line @typescript-eslint/no-explicit-any
  const compiled = graph.compile({ checkpointer: checkpointer as any });

  return compiled;
}
