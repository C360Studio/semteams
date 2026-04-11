/**
 * OpenAI AI Provider
 *
 * Implementation of AiProvider using the OpenAI SDK.
 * Supports OpenAI and any OpenAI-compatible API (via baseUrl).
 */

import OpenAI from "openai";
import type { AiProvider, AiMessage, AiTool, AiStreamEvent } from "./provider";

/**
 * OpenAI implementation of the AiProvider interface.
 */
export class OpenAiProvider implements AiProvider {
  private client: OpenAI;
  private model: string;

  constructor(apiKey: string, model?: string, baseUrl?: string) {
    this.client = new OpenAI({
      apiKey,
      dangerouslyAllowBrowser: true,
      ...(baseUrl ? { baseURL: baseUrl } : {}),
    });
    this.model = model ?? "gpt-4o";
  }

  async *streamChat(
    messages: AiMessage[],
    systemPrompt: string,
    tools: AiTool[],
    signal?: AbortSignal,
  ): AsyncGenerator<AiStreamEvent> {
    const openaiMessages: OpenAI.Chat.ChatCompletionMessageParam[] = [
      { role: "system", content: systemPrompt },
      ...messages.map((m) => ({
        role: m.role as "user" | "assistant",
        content: m.content,
      })),
    ];

    const openaiTools: OpenAI.Chat.ChatCompletionTool[] = tools.map((t) => ({
      type: "function" as const,
      function: {
        name: t.name,
        description: t.description,
        parameters: t.parameters,
      },
    }));

    const requestParams: OpenAI.Chat.ChatCompletionCreateParamsStreaming = {
      model: this.model,
      messages: openaiMessages,
      stream: true,
      ...(openaiTools.length > 0 ? { tools: openaiTools } : {}),
    };

    const stream = await this.client.chat.completions.create(requestParams, {
      signal,
    });

    // Accumulate tool call arguments across chunks
    const toolCallAccumulators = new Map<
      number,
      { name: string; arguments: string }
    >();

    for await (const chunk of stream) {
      const choice = chunk.choices[0];
      if (!choice) continue;

      const delta = choice.delta;

      // Text content
      if (delta.content) {
        yield { type: "text", content: delta.content };
      }

      // Tool calls
      if (delta.tool_calls) {
        for (const toolCallDelta of delta.tool_calls) {
          const idx = toolCallDelta.index;
          if (!toolCallAccumulators.has(idx)) {
            toolCallAccumulators.set(idx, {
              name: toolCallDelta.function?.name ?? "",
              arguments: "",
            });
          }
          const acc = toolCallAccumulators.get(idx)!;
          if (toolCallDelta.function?.name) {
            acc.name = toolCallDelta.function.name;
          }
          if (toolCallDelta.function?.arguments) {
            acc.arguments += toolCallDelta.function.arguments;
          }
        }
      }

      // When a choice finishes, emit accumulated tool calls
      if (choice.finish_reason === "tool_calls") {
        for (const [, acc] of toolCallAccumulators) {
          let input: Record<string, unknown> = {};
          try {
            input = JSON.parse(acc.arguments) as Record<string, unknown>;
          } catch {
            // Emit empty input if JSON is malformed
          }
          yield { type: "tool_use", name: acc.name, input };
        }
        toolCallAccumulators.clear();
      }

      if (
        choice.finish_reason === "stop" ||
        choice.finish_reason === "tool_calls"
      ) {
        yield { type: "done" };
      }
    }

    // Emit any remaining tool calls if stream ended without explicit finish_reason
    if (toolCallAccumulators.size > 0) {
      for (const [, acc] of toolCallAccumulators) {
        let input: Record<string, unknown> = {};
        try {
          input = JSON.parse(acc.arguments) as Record<string, unknown>;
        } catch {
          // Emit empty input if JSON is malformed
        }
        yield { type: "tool_use", name: acc.name, input };
      }
      yield { type: "done" };
    }
  }
}
