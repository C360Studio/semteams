/**
 * Claude API Client
 *
 * Client for interacting with Anthropic's Claude API.
 * Handles message sending, tool use, streaming, and error handling.
 *
 * Features:
 * - Initialize Anthropic client with API key
 * - Send messages with optional tools
 * - Parse and execute tool use responses
 * - Support streaming responses
 * - Comprehensive error handling
 * - Configurable timeout and retry logic
 *
 * Usage:
 * ```typescript
 * import { ClaudeClient } from '$lib/server/mcp/claude';
 *
 * const client = new ClaudeClient({ apiKey: 'sk-ant-api-key' });
 *
 * // Send a message
 * const response = await client.sendMessage('Hello Claude');
 *
 * // Send with tools
 * const response = await client.sendMessageWithTools('Generate a flow', tools);
 *
 * // Handle tool use
 * const toolCalls = client.parseToolUse(response);
 * ```
 */

import Anthropic from "@anthropic-ai/sdk";
import type {
  MessageCreateParams,
  Message,
} from "@anthropic-ai/sdk/resources/messages";

/**
 * Claude client configuration
 */
export interface ClaudeClientConfig {
  apiKey?: string;
  timeout?: number; // milliseconds, default 30000
  maxRetries?: number; // default 3
  retryDelayMs?: number; // base delay for exponential backoff, default 1000
}

/**
 * Options for individual API requests
 */
export interface ClaudeRequestOptions {
  /** AbortSignal for request cancellation */
  signal?: AbortSignal;
}

/**
 * Claude tool definition (compatible with Anthropic SDK)
 */
export interface ClaudeTool {
  name: string;
  description: string;
  input_schema: {
    type: "object";
    properties: Record<string, unknown>;
    required: string[];
  };
}

/**
 * Text content block
 */
export interface TextContent {
  type: "text";
  text: string;
}

/**
 * Tool use content block
 */
export interface ToolUseContent {
  type: "tool_use";
  id: string;
  name: string;
  input: Record<string, unknown>;
}

/**
 * Claude content type (text or tool use)
 */
export type ClaudeContent = TextContent | ToolUseContent;

/**
 * Claude message response
 */
export interface ClaudeMessage {
  id: string;
  type: "message";
  role: "assistant";
  content: ClaudeContent[];
  model: string;
  stop_reason: "end_turn" | "tool_use" | "max_tokens" | null;
}

/**
 * Message parameters for sending to Claude
 */
export interface SendMessageParams {
  model?: string;
  max_tokens?: number;
  system?: string;
  messages: Array<{
    role: "user" | "assistant";
    content: string | Array<{ type: string; [key: string]: unknown }>;
  }>;
  tools?: ClaudeTool[];
}

/**
 * Claude API error with additional context
 */
export class ClaudeApiError extends Error {
  constructor(
    message: string,
    public readonly statusCode?: number,
    public readonly isRetryable: boolean = false,
    public readonly originalError?: Error,
  ) {
    super(message);
    this.name = "ClaudeApiError";
  }
}

/**
 * Claude API Client
 *
 * Wraps the Anthropic SDK and provides a simplified interface
 * for sending messages and handling tool use.
 *
 * IMPORTANT: This client is intended for server-side use only.
 * It will throw an error if instantiated in a browser context.
 */
export class ClaudeClient {
  private client: Anthropic;
  private defaultModel = "claude-3-5-sonnet-20241022";
  private defaultMaxTokens = 4096;
  private timeout: number;
  private maxRetries: number;
  private retryDelayMs: number;

  /**
   * Create a new Claude client
   *
   * @param config Client configuration or API key string (for backward compatibility)
   * @throws Error if instantiated in browser context
   */
  constructor(config?: ClaudeClientConfig | string) {
    // Security check: prevent browser-side instantiation
    if (typeof window !== "undefined") {
      throw new Error(
        "ClaudeClient cannot be instantiated in browser context. " +
          "Use server-side API routes instead.",
      );
    }

    // Handle backward compatibility with string API key
    const normalizedConfig: ClaudeClientConfig =
      typeof config === "string" ? { apiKey: config } : config || {};

    this.timeout = normalizedConfig.timeout || 30000; // 30 seconds default
    this.maxRetries = normalizedConfig.maxRetries || 3;
    this.retryDelayMs = normalizedConfig.retryDelayMs || 1000;

    this.client = new Anthropic({
      apiKey: normalizedConfig.apiKey || process.env.ANTHROPIC_API_KEY,
      timeout: this.timeout,
      maxRetries: 0, // We handle retries ourselves for better control
    });
  }

  /**
   * Send a message to Claude
   *
   * @param prompt User prompt
   * @param options Optional message parameters
   * @param requestOptions Optional request options (e.g., AbortSignal)
   * @returns Claude's response
   * @throws ClaudeApiError if request fails or is aborted
   */
  async sendMessage(
    prompt: string,
    options?: {
      model?: string;
      maxTokens?: number;
      systemPrompt?: string;
    },
    requestOptions?: ClaudeRequestOptions,
  ): Promise<ClaudeMessage> {
    const params: MessageCreateParams = {
      model: options?.model || this.defaultModel,
      max_tokens: options?.maxTokens || this.defaultMaxTokens,
      messages: [{ role: "user", content: prompt }],
    };

    if (options?.systemPrompt) {
      params.system = options.systemPrompt;
    }

    return this.executeWithRetry(
      () => this.createMessage(params, requestOptions?.signal),
      requestOptions?.signal,
    );
  }

  /**
   * Send a message with tools
   *
   * @param prompt User prompt
   * @param tools Available tools
   * @param options Optional message parameters
   * @param requestOptions Optional request options (e.g., AbortSignal)
   * @returns Claude's response
   * @throws ClaudeApiError if request fails or is aborted
   */
  async sendMessageWithTools(
    prompt: string,
    tools: ClaudeTool[],
    options?: {
      model?: string;
      maxTokens?: number;
      systemPrompt?: string;
    },
    requestOptions?: ClaudeRequestOptions,
  ): Promise<ClaudeMessage> {
    const params: MessageCreateParams = {
      model: options?.model || this.defaultModel,
      max_tokens: options?.maxTokens || this.defaultMaxTokens,
      messages: [{ role: "user", content: prompt }],
      tools: tools as MessageCreateParams["tools"],
    };

    if (options?.systemPrompt) {
      params.system = options.systemPrompt;
    }

    return this.executeWithRetry(
      () => this.createMessage(params, requestOptions?.signal),
      requestOptions?.signal,
    );
  }

  /**
   * Send messages (multi-turn conversation)
   *
   * @param params Message parameters
   * @param requestOptions Optional request options (e.g., AbortSignal)
   * @returns Claude's response
   * @throws ClaudeApiError if request fails or is aborted
   */
  async sendMessages(
    params: SendMessageParams,
    requestOptions?: ClaudeRequestOptions,
  ): Promise<ClaudeMessage> {
    const createParams: MessageCreateParams = {
      model: params.model || this.defaultModel,
      max_tokens: params.max_tokens || this.defaultMaxTokens,
      messages: params.messages as MessageCreateParams["messages"],
    };

    if (params.system) {
      createParams.system = params.system;
    }

    if (params.tools) {
      createParams.tools = params.tools as MessageCreateParams["tools"];
    }

    return this.executeWithRetry(
      () => this.createMessage(createParams, requestOptions?.signal),
      requestOptions?.signal,
    );
  }

  /**
   * Parse tool use from response
   *
   * Extracts tool use blocks from Claude's response.
   *
   * @param response Claude message response
   * @returns Array of tool use content blocks
   */
  parseToolUse(response: ClaudeMessage): ToolUseContent[] {
    return response.content.filter(
      (block): block is ToolUseContent => block.type === "tool_use",
    );
  }

  /**
   * Send tool result back to Claude
   *
   * @param toolUseId Tool use ID from Claude's response
   * @param result Tool execution result
   * @param isError Whether the result is an error
   * @param conversationHistory Previous conversation history
   * @param tools Available tools (for continued tool use)
   * @param requestOptions Optional request options (e.g., AbortSignal)
   * @returns Claude's response
   * @throws ClaudeApiError if request fails or is aborted
   */
  async sendToolResult(
    toolUseId: string,
    result: unknown,
    isError = false,
    conversationHistory: Array<{
      role: "user" | "assistant";
      content: string | Array<{ type: string; [key: string]: unknown }>;
    }> = [],
    tools?: ClaudeTool[],
    requestOptions?: ClaudeRequestOptions,
  ): Promise<ClaudeMessage> {
    const toolResultMessage: MessageCreateParams["messages"][0] = {
      role: "user",
      content: [
        {
          type: "tool_result",
          tool_use_id: toolUseId,
          content: typeof result === "string" ? result : JSON.stringify(result),
          is_error: isError,
        },
      ],
    };

    const params: MessageCreateParams = {
      model: this.defaultModel,
      max_tokens: this.defaultMaxTokens,
      messages: [
        ...conversationHistory,
        toolResultMessage,
      ] as MessageCreateParams["messages"],
    };

    if (tools) {
      params.tools = tools as MessageCreateParams["tools"];
    }

    return this.executeWithRetry(
      () => this.createMessage(params, requestOptions?.signal),
      requestOptions?.signal,
    );
  }

  /**
   * Stream a message response
   *
   * @param prompt User prompt
   * @param options Optional message parameters
   * @param requestOptions Optional request options (e.g., AbortSignal)
   * @returns Async iterator of message chunks
   * @throws ClaudeApiError if request fails or is aborted
   */
  async *streamMessage(
    prompt: string,
    options?: {
      model?: string;
      maxTokens?: number;
      systemPrompt?: string;
      tools?: ClaudeTool[];
    },
    requestOptions?: ClaudeRequestOptions,
  ): AsyncGenerator<{
    type: string;
    delta?: { text?: string };
    [key: string]: unknown;
  }> {
    // Check if already aborted
    if (requestOptions?.signal?.aborted) {
      throw new ClaudeApiError("Request was cancelled", undefined, false);
    }

    const params: MessageCreateParams = {
      model: options?.model || this.defaultModel,
      max_tokens: options?.maxTokens || this.defaultMaxTokens,
      messages: [{ role: "user", content: prompt }],
    };

    if (options?.systemPrompt) {
      params.system = options.systemPrompt;
    }

    if (options?.tools) {
      params.tools = options.tools as MessageCreateParams["tools"];
    }

    const stream = await this.client.messages.stream(params, {
      signal: requestOptions?.signal,
    });

    for await (const event of stream) {
      // Check for abort during iteration
      if (requestOptions?.signal?.aborted) {
        throw new ClaudeApiError("Request was cancelled", undefined, false);
      }

      yield event as {
        type: string;
        delta?: { text?: string };
        [key: string]: unknown;
      };
    }
  }

  /**
   * Execute API call with retry logic using exponential backoff
   *
   * @param fn Function to execute
   * @param signal Optional AbortSignal for cancellation
   * @returns Result of the function
   * @throws ClaudeApiError after all retries exhausted or if aborted
   */
  private async executeWithRetry<T>(
    fn: () => Promise<T>,
    signal?: AbortSignal,
  ): Promise<T> {
    let lastError: Error | undefined;

    for (let attempt = 0; attempt < this.maxRetries; attempt++) {
      // Check if aborted before each attempt
      if (signal?.aborted) {
        throw new ClaudeApiError("Request was cancelled", undefined, false);
      }

      try {
        return await fn();
      } catch (error) {
        lastError = error instanceof Error ? error : new Error(String(error));

        // Don't retry if aborted
        if (this.isAbortError(error)) {
          throw new ClaudeApiError("Request was cancelled", undefined, false);
        }

        // Check if error is retryable
        if (!this.isRetryableError(error) || attempt === this.maxRetries - 1) {
          throw this.wrapError(error);
        }

        // Check if aborted before sleeping
        if (signal?.aborted) {
          throw new ClaudeApiError("Request was cancelled", undefined, false);
        }

        // Exponential backoff with jitter
        const delay = this.calculateBackoffDelay(attempt);
        await this.sleep(delay);
      }
    }

    throw this.wrapError(lastError || new Error("Unknown error after retries"));
  }

  /**
   * Create a message via Anthropic API
   *
   * @param params Message creation parameters
   * @param signal Optional AbortSignal for cancellation
   */
  private async createMessage(
    params: MessageCreateParams,
    signal?: AbortSignal,
  ): Promise<ClaudeMessage> {
    // Ensure stream is false to get a Message response, not a Stream
    const response = await this.client.messages.create(
      {
        ...params,
        stream: false,
      },
      {
        signal,
      },
    );
    return this.formatResponse(response as Anthropic.Message);
  }

  /**
   * Check if an error is an abort error
   */
  private isAbortError(error: unknown): boolean {
    if (error instanceof Error) {
      return (
        error.name === "AbortError" ||
        error.message.includes("abort") ||
        error.message.includes("cancel")
      );
    }
    return false;
  }

  /**
   * Check if an error is retryable
   *
   * Retryable errors include:
   * - 429 Rate Limit
   * - 500 Internal Server Error
   * - 502 Bad Gateway
   * - 503 Service Unavailable
   * - 504 Gateway Timeout
   * - Network errors
   */
  private isRetryableError(error: unknown): boolean {
    if (error instanceof Anthropic.APIError) {
      const status = error.status;
      return (
        status === 429 ||
        status === 500 ||
        status === 502 ||
        status === 503 ||
        status === 504
      );
    }

    // Network errors are retryable
    if (error instanceof Error) {
      const message = error.message.toLowerCase();
      return (
        message.includes("network") ||
        message.includes("timeout") ||
        message.includes("econnreset") ||
        message.includes("econnrefused")
      );
    }

    return false;
  }

  /**
   * Calculate backoff delay with exponential growth and jitter
   */
  private calculateBackoffDelay(attempt: number): number {
    // Exponential backoff: 1s, 2s, 4s, 8s...
    const exponentialDelay = this.retryDelayMs * Math.pow(2, attempt);
    // Add jitter (0-25% of delay)
    const jitter = exponentialDelay * Math.random() * 0.25;
    // Cap at 30 seconds
    return Math.min(exponentialDelay + jitter, 30000);
  }

  /**
   * Wrap error in ClaudeApiError with proper context
   */
  private wrapError(error: unknown): ClaudeApiError {
    if (error instanceof ClaudeApiError) {
      return error;
    }

    if (error instanceof Anthropic.APIError) {
      return new ClaudeApiError(
        error.message,
        error.status,
        this.isRetryableError(error),
        error,
      );
    }

    if (error instanceof Error) {
      return new ClaudeApiError(
        error.message,
        undefined,
        this.isRetryableError(error),
        error,
      );
    }

    return new ClaudeApiError(String(error));
  }

  /**
   * Sleep for specified duration
   */
  private sleep(ms: number): Promise<void> {
    return new Promise((resolve) => setTimeout(resolve, ms));
  }

  /**
   * Format Anthropic SDK response to our ClaudeMessage format
   *
   * @param response Anthropic SDK message response
   * @returns Formatted Claude message
   */
  private formatResponse(response: Message): ClaudeMessage {
    return {
      id: response.id,
      type: "message",
      role: "assistant",
      content: response.content as ClaudeContent[],
      model: response.model,
      stop_reason: response.stop_reason as
        | "end_turn"
        | "tool_use"
        | "max_tokens"
        | null,
    };
  }
}

/**
 * Create a Claude client instance
 *
 * Factory function for creating a Claude client.
 *
 * @param config Client configuration or API key string (for backward compatibility)
 * @returns Claude client instance
 */
export function createClaudeClient(
  config?: ClaudeClientConfig | string,
): ClaudeClient {
  return new ClaudeClient(config);
}
