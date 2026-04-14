package teamsmodel

import openai "github.com/sashabaranov/go-openai"

// ProviderAdapter normalizes request/response payloads for a specific
// LLM provider's OpenAI-compatible endpoint. Adapters handle quirks
// that would otherwise cause 400 errors or silent data corruption.
type ProviderAdapter interface {
	// Name returns the provider identifier (e.g., "gemini", "openai").
	Name() string

	// NormalizeRequest adjusts the ChatCompletionRequest before sending.
	// Called after the generic request is built, before the HTTP call.
	NormalizeRequest(req *openai.ChatCompletionRequest)

	// NormalizeMessages adjusts the message array before sending.
	// Called during request building for message-level fixes.
	NormalizeMessages(messages []openai.ChatCompletionMessage) []openai.ChatCompletionMessage

	// NormalizeStreamDelta adjusts a streaming tool call delta.
	// Returns the corrected tool call index, or -1 as a sentinel meaning
	// "allocate the next available index" (used when the provider omits it).
	NormalizeStreamDelta(delta openai.ToolCall, lastIndex int) int

	// NormalizeResponse adjusts the ChatCompletionResponse after receiving.
	// Called before the response is converted to AgentResponse.
	NormalizeResponse(resp *openai.ChatCompletionResponse)
}

// defaultAdapter is the fallback used when no provider-specific adapter is set.
// Package-level singleton avoids repeated allocation of the stateless struct.
var defaultAdapter ProviderAdapter = &GenericAdapter{}

// AdapterFor returns the appropriate adapter for the given provider name.
// Falls back to GenericAdapter for unknown providers.
func AdapterFor(provider string) ProviderAdapter {
	switch provider {
	case "gemini":
		return &GeminiAdapter{}
	case "openai":
		return &OpenAIAdapter{}
	default:
		return &GenericAdapter{}
	}
}
