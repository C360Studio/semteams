/**
 * Anthropic AI Provider
 *
 * Implementation of AiProvider using the Anthropic SDK.
 * Streams responses from Claude models.
 */

import Anthropic from "@anthropic-ai/sdk";
import type { AiProvider, AiMessage, AiTool, AiStreamEvent } from "./provider";

/**
 * Anthropic implementation of the AiProvider interface.
 */
export class AnthropicProvider implements AiProvider {
  private client: Anthropic;
  private model: string;

  constructor(apiKey: string, model?: string) {
    this.client = new Anthropic({ apiKey, dangerouslyAllowBrowser: true });
    this.model = model ?? "claude-sonnet-4-20250514";
  }

  async *streamChat(
    messages: AiMessage[],
    systemPrompt: string,
    tools: AiTool[],
    signal?: AbortSignal,
  ): AsyncGenerator<AiStreamEvent> {
    const anthropicMessages: Anthropic.MessageParam[] = messages.map((m) => ({
      role: m.role,
      content: m.content,
    }));

    const anthropicTools: Anthropic.Tool[] = tools.map((t) => ({
      name: t.name,
      description: t.description,
      input_schema: t.parameters as Anthropic.Tool["input_schema"],
    }));

    const streamParams: Anthropic.MessageStreamParams = {
      model: this.model,
      max_tokens: 4096,
      system: systemPrompt,
      messages: anthropicMessages,
      ...(anthropicTools.length > 0 ? { tools: anthropicTools } : {}),
    };

    const stream = this.client.messages.stream(streamParams, {
      signal,
    });

    // Collect tool use blocks as they accumulate
    const toolUseBlocks = new Map<
      number,
      { name: string; jsonAccumulator: string }
    >();

    for await (const event of stream) {
      if (event.type === "content_block_start") {
        if (event.content_block.type === "tool_use") {
          toolUseBlocks.set(event.index, {
            name: event.content_block.name,
            jsonAccumulator: "",
          });
        }
      } else if (event.type === "content_block_delta") {
        if (event.delta.type === "text_delta") {
          yield { type: "text", content: event.delta.text };
        } else if (event.delta.type === "input_json_delta") {
          const block = toolUseBlocks.get(event.index);
          if (block) {
            block.jsonAccumulator += event.delta.partial_json;
          }
        }
      } else if (event.type === "content_block_stop") {
        const block = toolUseBlocks.get(event.index);
        if (block) {
          let input: Record<string, unknown> = {};
          try {
            input = JSON.parse(block.jsonAccumulator) as Record<
              string,
              unknown
            >;
          } catch {
            // Emit empty input if JSON is malformed
          }
          yield { type: "tool_use", name: block.name, input };
          toolUseBlocks.delete(event.index);
        }
      } else if (event.type === "message_stop") {
        yield { type: "done" };
      }
    }
  }
}
