/**
 * AI Provider Abstraction
 *
 * Factory function and types for creating AI provider instances.
 * Supports Anthropic and OpenAI (or OpenAI-compatible) providers.
 */

import { AnthropicProvider } from "./anthropic";
import { OpenAiProvider } from "./openai";

/**
 * A single message in the conversation history.
 */
export interface AiMessage {
  role: "user" | "assistant";
  content: string;
}

/**
 * A tool definition passed to the AI provider.
 */
export interface AiTool {
  name: string;
  description: string;
  parameters: Record<string, unknown>;
}

/**
 * Events emitted by the AI stream.
 */
export type AiStreamEvent =
  | { type: "text"; content: string }
  | { type: "tool_use"; name: string; input: Record<string, unknown> }
  | { type: "done" };

/**
 * Common interface that all AI providers must implement.
 */
export interface AiProvider {
  streamChat(
    messages: AiMessage[],
    systemPrompt: string,
    tools: AiTool[],
    signal?: AbortSignal,
  ): AsyncGenerator<AiStreamEvent>;
}

/**
 * Configuration for creating an AI provider.
 */
export interface AiProviderConfig {
  provider: "anthropic" | "openai";
  apiKey: string;
  model?: string;
  baseUrl?: string;
}

/**
 * Factory function that creates an AI provider based on the given config.
 *
 * @param config Provider configuration
 * @returns An AiProvider instance
 * @throws Error when the provider name is not supported
 */
export function createAiProvider(config: AiProviderConfig): AiProvider {
  switch (config.provider) {
    case "anthropic":
      return new AnthropicProvider(config.apiKey, config.model);
    case "openai":
      return new OpenAiProvider(config.apiKey, config.model, config.baseUrl);
    default:
      throw new Error(
        `Unsupported AI provider: "${(config as { provider: string }).provider}". Supported providers: anthropic, openai`,
      );
  }
}
