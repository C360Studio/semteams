/**
 * API Route: AI Flow Generation
 *
 * SvelteKit server endpoint for AI-assisted flow generation using Claude and MCP.
 * Orchestrates component catalog retrieval, Claude-based generation, and validation.
 *
 * Features:
 * - POST endpoint accepting prompt and optional existing flow
 * - Rate limiting to prevent API abuse
 * - Comprehensive input validation
 * - MCP server integration for component catalog and validation
 * - Claude API integration for natural language flow generation
 * - Actionable error messages
 */

import { json } from "@sveltejs/kit";
import type { RequestHandler } from "./$types";
import { createMCPServer } from "$lib/server/mcp/server";
import { createClaudeClient, ClaudeApiError } from "$lib/server/mcp/claude";
import { env } from "$env/dynamic/private";
import type { Flow, FlowNode, FlowConnection } from "$lib/types/flow";
import type { ValidationResult } from "$lib/types/validation";
import type { ComponentType } from "$lib/types/component";
import type { ClaudeTool } from "$lib/server/mcp/claude";
import {
  checkRateLimit,
  AI_RATE_LIMIT_CONFIG,
} from "$lib/server/middleware/rateLimit";
import {
  validatePrompt,
  CLAUDE_CONFIG,
  ERROR_MESSAGES,
} from "$lib/server/config/ai";

/**
 * Request body interface
 */
interface GenerateFlowRequest {
  prompt: string;
  existingFlow?: Partial<Flow>;
}

/**
 * Tool input from Claude's create_flow tool
 */
interface CreateFlowToolInput {
  nodes?: FlowNode[];
  connections?: FlowConnection[];
}

/**
 * Get client identifier for rate limiting
 */
function getClientId(request: Request): string {
  // Try to get IP from various headers (for proxied requests)
  const forwarded = request.headers.get("x-forwarded-for");
  if (forwarded) {
    return forwarded.split(",")[0].trim();
  }

  const realIp = request.headers.get("x-real-ip");
  if (realIp) {
    return realIp;
  }

  // Fallback to a default (useful for local development)
  return "default-client";
}

/**
 * POST handler for AI flow generation
 *
 * Accepts a natural language prompt and optional existing flow,
 * generates a flow using Claude and MCP tools, and returns the
 * generated flow with validation results.
 */
export const POST: RequestHandler = async ({ request }) => {
  const clientId = getClientId(request);

  // Step 1: Rate limiting
  const rateLimitResult = checkRateLimit(clientId, AI_RATE_LIMIT_CONFIG);

  if (!rateLimitResult.allowed) {
    return json(
      {
        error: ERROR_MESSAGES.rateLimitExceeded(rateLimitResult.retryAfter!),
        code: "RATE_LIMITED",
        retryAfter: rateLimitResult.retryAfter,
      },
      {
        status: 429,
        headers: {
          "Retry-After": String(rateLimitResult.retryAfter),
          "X-RateLimit-Limit": String(rateLimitResult.limit),
          "X-RateLimit-Remaining": "0",
        },
      },
    );
  }

  try {
    // Step 2: Parse request body
    let body: GenerateFlowRequest;
    try {
      body = await request.json();
    } catch {
      return json(
        {
          error: "Invalid JSON in request body",
          code: "INVALID_JSON",
        },
        { status: 400 },
      );
    }

    // Step 3: Validate prompt
    const promptValidation = validatePrompt(body.prompt);
    if (!promptValidation.valid) {
      return json(
        {
          error: promptValidation.error,
          code: "INVALID_PROMPT",
        },
        { status: 400 },
      );
    }

    const { existingFlow } = body;
    const prompt = promptValidation.trimmedPrompt!;

    // Step 4: Initialize MCP server and Claude client
    const mcpServer = createMCPServer({
      backendUrl: env.BACKEND_URL || "http://localhost:8080",
    });

    const claude = createClaudeClient({
      apiKey: env.ANTHROPIC_API_KEY,
      timeout: CLAUDE_CONFIG.timeout,
      maxRetries: CLAUDE_CONFIG.maxRetries,
      retryDelayMs: CLAUDE_CONFIG.retryDelayMs,
    });

    // Step 5: Fetch component catalog via MCP
    let componentCatalog: ComponentType[];
    try {
      componentCatalog = await mcpServer.getComponentCatalog();
    } catch (err) {
      console.error("Failed to fetch component catalog:", err);
      return json(
        {
          error: ERROR_MESSAGES.componentCatalogFailed,
          code: "COMPONENT_CATALOG_FAILED",
          details: err instanceof Error ? err.message : undefined,
        },
        { status: 503 },
      );
    }

    // Step 6: Build system prompt with component catalog
    const systemPrompt = buildSystemPrompt(componentCatalog, existingFlow);

    // Step 7: Define MCP tools for Claude
    const tools: ClaudeTool[] = [
      {
        name: "create_flow",
        description:
          "Creates a new flow with nodes and connections based on the user prompt.",
        input_schema: {
          type: "object",
          properties: {
            nodes: {
              type: "array",
              description: "Array of flow nodes (components)",
              items: {
                type: "object",
                properties: {
                  id: { type: "string", description: "Unique node identifier" },
                  type: {
                    type: "string",
                    description: "Component type ID from catalog",
                  },
                  name: {
                    type: "string",
                    description: "Human-readable node name",
                  },
                  position: {
                    type: "object",
                    properties: {
                      x: { type: "number" },
                      y: { type: "number" },
                    },
                    required: ["x", "y"],
                  },
                  config: {
                    type: "object",
                    description: "Component-specific configuration",
                  },
                },
                required: ["id", "type", "name", "position", "config"],
              },
            },
            connections: {
              type: "array",
              description: "Array of connections between nodes",
              items: {
                type: "object",
                properties: {
                  id: { type: "string", description: "Connection identifier" },
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
          },
          required: ["nodes", "connections"],
        },
      },
    ];

    // Step 8: Send prompt to Claude with tools
    let claudeResponse;
    try {
      claudeResponse = await claude.sendMessageWithTools(prompt, tools, {
        model: env.ANTHROPIC_MODEL || CLAUDE_CONFIG.defaultModel,
        systemPrompt,
        maxTokens: CLAUDE_CONFIG.maxTokens,
      });
    } catch (err) {
      console.error("Claude API request failed:", err);

      if (err instanceof ClaudeApiError) {
        if (err.statusCode === 429) {
          return json(
            {
              error: ERROR_MESSAGES.claudeRateLimited,
              code: "CLAUDE_RATE_LIMITED",
            },
            { status: 503 },
          );
        }
      }

      return json(
        {
          error: ERROR_MESSAGES.claudeApiFailed,
          code: "CLAUDE_API_FAILED",
          details: err instanceof Error ? err.message : undefined,
        },
        { status: 503 },
      );
    }

    // Step 9: Parse tool use from Claude response
    const toolUses = claude.parseToolUse(claudeResponse);
    const createFlowTool = toolUses.find((tool) => tool.name === "create_flow");

    if (!createFlowTool) {
      return json(
        {
          error: ERROR_MESSAGES.noFlowGenerated,
          code: "NO_FLOW_GENERATED",
        },
        { status: 422 },
      );
    }

    // Step 10: Extract generated flow from tool input
    const toolInput = createFlowTool.input as CreateFlowToolInput;
    const generatedFlow: Partial<Flow> = {
      nodes: toolInput.nodes || [],
      connections: toolInput.connections || [],
    };

    // Step 11: Validate generated flow via MCP
    let validationResult: ValidationResult;
    try {
      const tempFlowId = `temp_${Date.now()}`;
      validationResult = (await mcpServer.validateFlow(
        tempFlowId,
        generatedFlow,
      )) as ValidationResult;
    } catch (err) {
      console.error("Flow validation failed:", err);
      return json(
        {
          error: ERROR_MESSAGES.validationFailed,
          code: "VALIDATION_FAILED",
          details: err instanceof Error ? err.message : undefined,
        },
        { status: 500 },
      );
    }

    // Step 12: Return generated flow and validation result
    return json(
      {
        flow: generatedFlow,
        validationResult,
      },
      {
        status: 200,
        headers: {
          "X-RateLimit-Limit": String(rateLimitResult.limit),
          "X-RateLimit-Remaining": String(rateLimitResult.remaining),
        },
      },
    );
  } catch (err) {
    console.error("Unexpected error in AI flow generation:", err);
    return json(
      {
        error: ERROR_MESSAGES.internalError,
        code: "INTERNAL_ERROR",
        details:
          process.env.NODE_ENV === "development" && err instanceof Error
            ? err.message
            : undefined,
      },
      { status: 500 },
    );
  }
};

/**
 * Build system prompt for Claude
 *
 * Constructs a comprehensive system prompt that includes:
 * - Component catalog with available component types
 * - Flow generation guidelines and best practices
 * - Optional existing flow context for modifications
 */
function buildSystemPrompt(
  catalog: ComponentType[],
  existingFlow?: Partial<Flow>,
): string {
  const catalogDescription = catalog
    .map((comp) => {
      const inputPorts =
        comp.ports?.filter((p) => p.direction === "input").length || 0;
      const outputPorts =
        comp.ports?.filter((p) => p.direction === "output").length || 0;
      return `- ${comp.id} (${comp.category}): ${comp.description}\n  Ports: ${inputPorts} inputs, ${outputPorts} outputs`;
    })
    .join("\n");

  let prompt = `You are a flow generation assistant for SemStreams, a semantic stream processing platform.

Your task is to generate flows (directed graphs of components) based on user prompts.

Available component catalog:
${catalogDescription}

Guidelines:
1. Use only component types from the catalog above
2. Create meaningful component names that describe their purpose
3. Position nodes in a left-to-right flow layout (x: 0-1000, y: 0-600)
4. Connect components using their input/output ports
5. Ensure all connections reference valid node IDs and port names
6. Use connection IDs in format: conn_<source>_<target>_<port>
7. Configure components with sensible default values
8. Consider data flow patterns: sources → processors → sinks

When creating flows:
- Sources (HTTP, Kafka) go on the left
- Processors (transformers, filters) go in the middle
- Sinks (HTTP, Kafka) go on the right
- Space nodes evenly for readability`;

  if (existingFlow) {
    prompt += `\n\nExisting flow context:
Nodes: ${existingFlow.nodes?.length || 0}
Connections: ${existingFlow.connections?.length || 0}

When modifying, preserve existing nodes unless explicitly asked to remove them.`;
  }

  return prompt;
}
