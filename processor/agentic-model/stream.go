package agenticmodel

import (
	"encoding/json"
	"sort"
	"strings"

	"github.com/c360studio/semstreams/agentic"
	openai "github.com/sashabaranov/go-openai"
)

// StreamChunk represents a single streaming delta for real-time monitoring.
// Chunks are ephemeral — published via core NATS (fire-and-forget), not JetStream.
type StreamChunk struct {
	RequestID      string `json:"request_id"`
	ContentDelta   string `json:"content_delta,omitempty"`
	ReasoningDelta string `json:"reasoning_delta,omitempty"`
	Done           bool   `json:"done,omitempty"`
}

// ChunkHandler is a callback for receiving streaming deltas.
// Implementations must be safe for concurrent use if the handler is shared.
type ChunkHandler func(chunk StreamChunk)

// streamAccumulator aggregates streaming deltas into a complete AgentResponse.
type streamAccumulator struct {
	content          strings.Builder
	reasoning        strings.Builder
	role             string
	finishReason     openai.FinishReason
	toolCalls        map[int]*openai.ToolCall
	promptTokens     int
	completionTokens int
}

// processDelta incorporates a single streaming choice delta.
func (a *streamAccumulator) processDelta(choice openai.ChatCompletionStreamChoice) {
	delta := choice.Delta

	if delta.Role != "" {
		a.role = delta.Role
	}

	if delta.Content != "" {
		a.content.WriteString(delta.Content)
	}

	if delta.ReasoningContent != "" {
		a.reasoning.WriteString(delta.ReasoningContent)
	}

	if choice.FinishReason != "" {
		a.finishReason = choice.FinishReason
	}

	// Aggregate tool call deltas by index
	for _, tc := range delta.ToolCalls {
		idx := 0
		if tc.Index != nil {
			idx = *tc.Index
		}

		if a.toolCalls == nil {
			a.toolCalls = make(map[int]*openai.ToolCall)
		}

		existing, ok := a.toolCalls[idx]
		if !ok {
			existing = &openai.ToolCall{
				Type: openai.ToolTypeFunction,
			}
			a.toolCalls[idx] = existing
		}

		if tc.ID != "" {
			existing.ID = tc.ID
		}
		if tc.Function.Name != "" {
			existing.Function.Name += tc.Function.Name
		}
		if tc.Function.Arguments != "" {
			existing.Function.Arguments += tc.Function.Arguments
		}
	}
}

// setUsage records token counts from the final stream chunk.
func (a *streamAccumulator) setUsage(usage *openai.Usage) {
	if usage == nil {
		return
	}
	a.promptTokens = usage.PromptTokens
	a.completionTokens = usage.CompletionTokens
}

// toAgentResponse builds the complete response from accumulated deltas.
func (a *streamAccumulator) toAgentResponse(requestID string) agentic.AgentResponse {
	resp := agentic.AgentResponse{
		RequestID: requestID,
		Message: agentic.ChatMessage{
			Role:             a.role,
			Content:          a.content.String(),
			ReasoningContent: a.reasoning.String(),
		},
		TokenUsage: agentic.TokenUsage{
			PromptTokens:     a.promptTokens,
			CompletionTokens: a.completionTokens,
		},
	}

	// Map finish reason to status
	switch a.finishReason {
	case "tool_calls":
		resp.Status = "tool_call"
	default:
		resp.Status = "complete"
	}

	// Sort tool calls by index and convert to agentic types
	if len(a.toolCalls) > 0 {
		resp.Status = "tool_call"

		indices := make([]int, 0, len(a.toolCalls))
		for idx := range a.toolCalls {
			indices = append(indices, idx)
		}
		sort.Ints(indices)

		toolCalls := make([]agentic.ToolCall, 0, len(indices))
		for _, idx := range indices {
			tc := a.toolCalls[idx]
			var args map[string]any
			if tc.Function.Arguments != "" {
				json.Unmarshal([]byte(tc.Function.Arguments), &args)
			}
			toolCalls = append(toolCalls, agentic.ToolCall{
				ID:        tc.ID,
				Name:      tc.Function.Name,
				Arguments: args,
			})
		}
		resp.Message.ToolCalls = toolCalls
	}

	return resp
}
