import type { ChatIntent, ChatPageContext } from "$lib/types/chat";

// ---------------------------------------------------------------------------
// ToolDefinition — a single AI tool definition with JSON schema parameters
// ---------------------------------------------------------------------------

export interface ToolDefinition {
  name: string;
  description: string;
  parameters: {
    type: "object";
    properties: Record<string, unknown>;
    required: string[];
  };
}

// ---------------------------------------------------------------------------
// ToolRegistryConfig — configuration passed to the tool registry
// ---------------------------------------------------------------------------

export interface ToolRegistryConfig {
  backendUrl: string;
  componentCatalog?: unknown[];
}

// ---------------------------------------------------------------------------
// Tool definitions — one object per tool
// ---------------------------------------------------------------------------

const graphSearchTool: ToolDefinition = {
  name: "graph_search",
  description:
    "Search the SemStreams knowledge graph for entities, communities, and relationships using natural language or structured queries.",
  parameters: {
    type: "object",
    properties: {
      query: {
        type: "string",
        description: "Natural language or keyword search query",
      },
      mode: {
        type: "string",
        enum: ["semantic", "keyword", "hybrid"],
        description: "Search mode to use (default: hybrid)",
      },
      limit: {
        type: "number",
        description: "Maximum number of results to return (default: 10)",
      },
    },
    required: ["query"],
  },
};

const createFlowTool: ToolDefinition = {
  name: "create_flow",
  description:
    "Create or replace a SemStreams pipeline flow with the specified nodes and connections.",
  parameters: {
    type: "object",
    properties: {
      nodes: {
        type: "array",
        description: "Array of flow nodes (components) to include in the flow",
        items: {
          type: "object",
          properties: {
            id: { type: "string" },
            component: { type: "string" },
            type: { type: "string" },
            name: { type: "string" },
            position: {
              type: "object",
              properties: {
                x: { type: "number" },
                y: { type: "number" },
              },
              required: ["x", "y"],
            },
            config: { type: "object" },
          },
          required: ["id", "component", "type", "name", "position", "config"],
        },
      },
      connections: {
        type: "array",
        description: "Array of connections wiring nodes together",
        items: {
          type: "object",
          properties: {
            id: { type: "string" },
            source_node_id: { type: "string" },
            source_port: { type: "string" },
            target_node_id: { type: "string" },
            target_port: { type: "string" },
          },
          required: [
            "id",
            "source_node_id",
            "source_port",
            "target_node_id",
            "target_port",
          ],
        },
      },
      name: {
        type: "string",
        description: "Optional name for the flow",
      },
    },
    required: ["nodes", "connections"],
  },
};

const entityLookupTool: ToolDefinition = {
  name: "entity_lookup",
  description:
    "Look up detailed information about a specific entity in the knowledge graph by its ID.",
  parameters: {
    type: "object",
    properties: {
      entityId: {
        type: "string",
        description:
          "The fully-qualified entity ID (e.g., c360.ops.robotics.gcs.drone.001)",
      },
      includeRelationships: {
        type: "boolean",
        description:
          "Whether to include related entities in the response (default: true)",
      },
    },
    required: ["entityId"],
  },
};

const componentHealthTool: ToolDefinition = {
  name: "component_health",
  description:
    "Query the runtime health status of one or more components in a SemStreams flow.",
  parameters: {
    type: "object",
    properties: {
      flowId: {
        type: "string",
        description: "The ID of the flow to check",
      },
      componentId: {
        type: "string",
        description:
          "Optional specific component node ID to check; omit to check all components",
      },
    },
    required: ["flowId"],
  },
};

const flowStatusTool: ToolDefinition = {
  name: "flow_status",
  description:
    "Get the overall runtime status and metrics for a SemStreams flow, including deployment state and throughput.",
  parameters: {
    type: "object",
    properties: {
      flowId: {
        type: "string",
        description: "The ID of the flow to inspect",
      },
    },
    required: ["flowId"],
  },
};

// ---------------------------------------------------------------------------
// getToolsForContext — select tools based on intent and page context
// ---------------------------------------------------------------------------

export function getToolsForContext(
  _config: ToolRegistryConfig,
  intent: ChatIntent,
  context: ChatPageContext,
): ToolDefinition[] {
  const page = context.page;

  switch (intent) {
    case "search":
      return [graphSearchTool];

    case "flow-create":
      if (page === "flow-builder") {
        return [createFlowTool];
      }
      return [];

    case "explain":
      return [entityLookupTool];

    case "debug":
      return [componentHealthTool, flowStatusTool];

    case "health":
      return [componentHealthTool, flowStatusTool];

    case "general":
      if (page === "flow-builder") {
        return [createFlowTool, graphSearchTool, entityLookupTool];
      }
      // data-view
      return [graphSearchTool, entityLookupTool];

    default:
      // flow-modify and any future intents — return empty rather than throw
      return [];
  }
}
