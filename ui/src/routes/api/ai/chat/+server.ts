/**
 * API Route: AI Chat Streaming
 *
 * SvelteKit server endpoint for AI-assisted conversational flow generation.
 * Streams responses using Server-Sent Events (SSE).
 *
 * Features:
 * - POST endpoint accepting message history and current flow context
 * - Rate limiting to prevent API abuse
 * - Comprehensive input validation
 * - MCP server integration for component catalog and validation
 * - Provider abstraction for AI streaming (Anthropic or OpenAI)
 * - SSE streaming with text, tool_use, and done events
 */

import type { RequestHandler } from "./$types";
import { createMCPServer } from "$lib/server/mcp/server";
import { createAiProvider } from "$lib/server/ai/provider";
import { env } from "$env/dynamic/private";
import type { Flow } from "$lib/types/flow";
import type { AiTool } from "$lib/server/ai/provider";
import {
  checkRateLimit,
  AI_RATE_LIMIT_CONFIG,
} from "$lib/server/middleware/rateLimit";

/**
 * A single chat message in the conversation history.
 */
interface ChatMessage {
  role: "user" | "assistant";
  content: string;
}

/**
 * Request body interface for the chat endpoint.
 */
interface ChatRequest {
  messages: ChatMessage[];
  currentFlow: unknown;
}

/**
 * Get client identifier for rate limiting.
 */
function getClientId(request: Request): string {
  const forwarded = request.headers.get("x-forwarded-for");
  if (forwarded) {
    return forwarded.split(",")[0].trim();
  }
  const realIp = request.headers.get("x-real-ip");
  if (realIp) {
    return realIp;
  }
  return "default-client";
}

/**
 * Validate the chat request body.
 */
function validateChatRequest(
  body: unknown,
): { valid: true; data: ChatRequest } | { valid: false; error: string } {
  if (!body || typeof body !== "object") {
    return { valid: false, error: "Request body must be a JSON object" };
  }

  const b = body as Record<string, unknown>;

  if (!Array.isArray(b.messages)) {
    return { valid: false, error: "messages must be an array" };
  }

  if (b.messages.length === 0) {
    return { valid: false, error: "messages array must not be empty" };
  }

  if (b.currentFlow === undefined || b.currentFlow === null) {
    return { valid: false, error: "currentFlow is required" };
  }

  return {
    valid: true,
    data: {
      messages: b.messages as ChatMessage[],
      currentFlow: b.currentFlow,
    },
  };
}

/**
 * Build the system prompt with component catalog and current flow context.
 */
function buildSystemPrompt(
  catalog: Array<{
    id: string;
    name: string;
    category?: string;
    description?: string;
    ports?: Array<{ direction: string }>;
  }>,
  currentFlow: unknown,
): string {
  const catalogDescription = catalog
    .map((comp) => {
      const inputPorts =
        comp.ports?.filter((p) => p.direction === "input").length ?? 0;
      const outputPorts =
        comp.ports?.filter((p) => p.direction === "output").length ?? 0;
      return `- ${comp.id} (${comp.category ?? "unknown"}): ${comp.description ?? ""}\n  Ports: ${inputPorts} inputs, ${outputPorts} outputs`;
    })
    .join("\n");

  let prompt = `You are a conversational flow generation assistant for SemStreams, a semantic stream processing platform.

Help users create and modify flows (directed graphs of components) through natural conversation.

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

  // Include existing flow context if there are nodes
  if (
    currentFlow &&
    typeof currentFlow === "object" &&
    "nodes" in currentFlow
  ) {
    const flowWithNodes = currentFlow as { nodes?: unknown[] };
    const nodeCount = flowWithNodes.nodes?.length ?? 0;
    if (nodeCount > 0) {
      prompt += `\n\nExisting flow context:
The user currently has a flow with ${nodeCount} existing node(s). When modifying the existing flow, preserve existing nodes unless explicitly asked to remove them.
Current flow data: ${JSON.stringify(currentFlow)}`;
    }
  }

  return prompt;
}

/**
 * POST handler for AI chat streaming.
 *
 * Accepts a message history and current flow context,
 * streams AI responses using SSE.
 */
export const POST: RequestHandler = async ({ request }) => {
  const clientId = getClientId(request);

  // Step 1: Rate limiting
  const rateLimitResult = checkRateLimit(clientId, AI_RATE_LIMIT_CONFIG);

  if (!rateLimitResult.allowed) {
    return new Response(
      JSON.stringify({
        error: `Too many requests. Please wait ${rateLimitResult.retryAfter} seconds before trying again.`,
        code: "RATE_LIMITED",
        retryAfter: rateLimitResult.retryAfter,
      }),
      {
        status: 429,
        headers: {
          "Content-Type": "application/json",
          "Retry-After": String(rateLimitResult.retryAfter),
          "X-RateLimit-Limit": String(rateLimitResult.limit),
          "X-RateLimit-Remaining": "0",
        },
      },
    );
  }

  // Step 2: Parse request body
  let body: unknown;
  try {
    body = await request.json();
  } catch {
    return new Response(
      JSON.stringify({
        error: "Invalid JSON in request body",
        code: "INVALID_JSON",
      }),
      { status: 400, headers: { "Content-Type": "application/json" } },
    );
  }

  // Step 3: Validate request
  const validation = validateChatRequest(body);
  if (!validation.valid) {
    return new Response(
      JSON.stringify({ error: validation.error, code: "INVALID_REQUEST" }),
      { status: 400, headers: { "Content-Type": "application/json" } },
    );
  }

  const { messages, currentFlow } = validation.data;

  // Step 4: Initialize MCP server
  const mcpServer = createMCPServer({
    backendUrl: env.BACKEND_URL || "http://localhost:8080",
  });

  // Step 5: Fetch component catalog via MCP
  let componentCatalog: Array<{
    id: string;
    name: string;
    category?: string;
    description?: string;
    ports?: Array<{ direction: string }>;
  }>;
  try {
    componentCatalog =
      (await mcpServer.getComponentCatalog()) as typeof componentCatalog;
  } catch (err) {
    console.error("Failed to fetch component catalog:", err);
    return new Response(
      JSON.stringify({
        error:
          "Unable to fetch available components. Please check if the backend is running and try again.",
        code: "COMPONENT_CATALOG_FAILED",
        details: err instanceof Error ? err.message : undefined,
      }),
      { status: 503, headers: { "Content-Type": "application/json" } },
    );
  }

  // Step 6: Build system prompt
  const systemPrompt = buildSystemPrompt(componentCatalog, currentFlow);

  // Step 7: Define tools
  const tools: AiTool[] = [
    {
      name: "create_flow",
      description:
        "Creates or updates a flow with nodes and connections based on the conversation.",
      parameters: {
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

  // Step 8: Create AI provider
  const provider = createAiProvider({
    provider: (env.AI_PROVIDER as "anthropic" | "openai") || "anthropic",
    apiKey: env.ANTHROPIC_API_KEY || env.OPENAI_API_KEY || "",
    model: env.AI_MODEL,
  });

  // Step 9: Process AI stream and collect SSE chunks
  const signal = request.signal;
  const encoder = new TextEncoder();
  const chunks: Uint8Array[] = [];

  function buildEvent(event: string, data: unknown): Uint8Array {
    return encoder.encode(`event: ${event}\ndata: ${JSON.stringify(data)}\n\n`);
  }

  try {
    let generatedFlow: Partial<Flow> | undefined;

    for await (const event of provider.streamChat(
      messages,
      systemPrompt,
      tools,
      signal,
    )) {
      if (event.type === "text") {
        chunks.push(buildEvent("text", { content: event.content }));
      } else if (event.type === "tool_use" && event.name === "create_flow") {
        generatedFlow = event.input as Partial<Flow>;
      } else if (event.type === "done") {
        if (generatedFlow) {
          // Validate via MCP
          const tempId = `temp_${Date.now()}`;
          const validationResult = await mcpServer.validateFlow(
            tempId,
            generatedFlow,
          );
          chunks.push(
            buildEvent("done", { flow: generatedFlow, validationResult }),
          );
        } else {
          chunks.push(buildEvent("done", {}));
        }
      }
    }
  } catch (err) {
    if (err instanceof DOMException && err.name === "AbortError") {
      // Silent close on abort — no error event
    } else {
      chunks.push(
        buildEvent("error", {
          message: err instanceof Error ? err.message : "Unknown error",
        }),
      );
    }
  }

  // Step 10: Return collected chunks as a ReadableStream response
  const stream = new ReadableStream({
    start(controller) {
      for (const chunk of chunks) {
        controller.enqueue(chunk);
      }
      controller.close();
    },
  });

  return new Response(stream, {
    status: 200,
    headers: {
      "Content-Type": "text/event-stream",
      "Cache-Control": "no-cache",
      Connection: "keep-alive",
      "X-RateLimit-Limit": String(rateLimitResult.limit),
      "X-RateLimit-Remaining": String(rateLimitResult.remaining),
    },
  });
};
