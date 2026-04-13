package teamsmodel

import openai "github.com/sashabaranov/go-openai"

// GenericAdapter applies cross-provider safe normalizations that are either
// required by multiple providers or harmless for all known providers.
// It is the fallback when no provider-specific adapter is registered.
type GenericAdapter struct{}

// Name returns "generic".
func (a *GenericAdapter) Name() string { return "generic" }

// NormalizeRequest is a no-op for the generic adapter.
func (a *GenericAdapter) NormalizeRequest(_ *openai.ChatCompletionRequest) {}

// NormalizeMessages applies normalizations that are safe across all providers:
//
//  1. Tool result messages get a non-empty name field. The name field is optional
//     in the OpenAI spec but required by Gemini. Setting it universally is harmless.
//
//  2. Assistant messages with tool_calls get a non-empty content field. Gemini rejects
//     absent content; setting it to a single space is a widely-used convention
//     (LiteLLM, OpenAI proxy, etc.) and accepted by all known providers.
//
// reasoning_content omission is handled structurally during message conversion
// (the field is never copied into the outgoing openai.ChatCompletionMessage).
func (a *GenericAdapter) NormalizeMessages(messages []openai.ChatCompletionMessage) []openai.ChatCompletionMessage {
	for i := range messages {
		if messages[i].Role == "tool" && messages[i].Name == "" {
			messages[i].Name = "unknown_tool"
		}
		if messages[i].Role == "assistant" && len(messages[i].ToolCalls) > 0 && messages[i].Content == "" {
			messages[i].Content = " "
		}
	}
	return messages
}

// NormalizeStreamDelta infers the tool call index when the provider omits it.
// When an explicit index is provided, it is used directly. When absent, a
// non-empty ID signals a new tool call (return -1 sentinel so the accumulator
// allocates the next index), and an empty ID is an argument continuation
// (reuse lastIndex). This matches the behavior required by Gemini and is
// harmless for providers that always supply an explicit index.
func (a *GenericAdapter) NormalizeStreamDelta(delta openai.ToolCall, lastIndex int) int {
	if delta.Index != nil {
		return *delta.Index
	}
	if delta.ID != "" {
		return -1 // sentinel: caller must allocate next index via nextToolIndex
	}
	return lastIndex
}

// NormalizeResponse is a no-op for the generic adapter.
func (a *GenericAdapter) NormalizeResponse(_ *openai.ChatCompletionResponse) {}
