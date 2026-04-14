package teamsloop

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	"github.com/c360studio/semstreams/agentic"
	"github.com/c360studio/semstreams/graph/llm"
)

// Summarizer abstracts the LLM call for context compaction.
type Summarizer interface {
	// Summarize generates a concise summary of the given conversation messages.
	// maxTokens limits the response length.
	Summarize(ctx context.Context, messages []agentic.ChatMessage, maxTokens int) (string, error)
}

// LLMSummarizer implements Summarizer using a graph/llm.Client.
type LLMSummarizer struct {
	client llm.Client
	logger *slog.Logger
}

// NewLLMSummarizer creates a summarizer backed by an LLM client.
func NewLLMSummarizer(client llm.Client, logger *slog.Logger) *LLMSummarizer {
	if logger == nil {
		logger = slog.Default()
	}
	return &LLMSummarizer{client: client, logger: logger}
}

const summarizationSystemPrompt = `You are a conversation summarizer for an AI agent system.
Summarize the key decisions, findings, tool results, and current state from this conversation.
Be concise but preserve important details like entity IDs, file paths, specific values, and error messages.
Focus on what happened and what was learned, not the conversation flow itself.`

// Summarize calls the LLM to generate a summary of the conversation messages.
func (s *LLMSummarizer) Summarize(ctx context.Context, messages []agentic.ChatMessage, maxTokens int) (string, error) {
	userPrompt := formatMessagesForSummary(messages)

	temp := 0.3
	resp, err := s.client.ChatCompletion(ctx, llm.ChatRequest{
		SystemPrompt: summarizationSystemPrompt,
		UserPrompt:   userPrompt,
		MaxTokens:    maxTokens,
		Temperature:  &temp,
	})
	if err != nil {
		return "", fmt.Errorf("summarization LLM call failed: %w", err)
	}

	return resp.Content, nil
}

// formatMessagesForSummary converts chat messages into a structured text block
// suitable for the summarization prompt.
func formatMessagesForSummary(messages []agentic.ChatMessage) string {
	var b strings.Builder
	b.WriteString("Conversation to summarize:\n\n")
	for _, msg := range messages {
		fmt.Fprintf(&b, "[%s]: %s\n", msg.Role, msg.Content)
		// Include tool call info if present
		for _, tc := range msg.ToolCalls {
			fmt.Fprintf(&b, "  -> tool_call: %s(%v)\n", tc.Name, tc.Arguments)
		}
	}
	return b.String()
}
