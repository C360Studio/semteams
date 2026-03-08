package query

import (
	"context"
	"regexp"
	"strings"

	"github.com/c360studio/semstreams/graph/llm"
)

// thinkTagPattern matches <think>...</think> blocks from reasoning models (qwen3, deepseek-r1).
var thinkTagPattern = regexp.MustCompile(`(?s)<think>.*?</think>`)

// LLMClientAdapter adapts a graph/llm.Client to the query.LLMClient interface.
//
// This allows the existing OpenAI-compatible LLM infrastructure (ollama, seminstruct,
// shimmy, OpenAI) to be used as the T3 classifier backend.
type LLMClientAdapter struct {
	client llm.Client
}

// NewLLMClientAdapter wraps a graph/llm.Client for use as a query.LLMClient.
func NewLLMClientAdapter(client llm.Client) *LLMClientAdapter {
	return &LLMClientAdapter{client: client}
}

// ClassifyQuery sends the classification prompt to the LLM and returns the raw response.
func (a *LLMClientAdapter) ClassifyQuery(ctx context.Context, prompt string) (string, error) {
	temp := 0.1 // Low temperature for deterministic classification
	resp, err := a.client.ChatCompletion(ctx, llm.ChatRequest{
		SystemPrompt: "You are a query classifier. Return ONLY valid JSON. No markdown, no explanation. Do not use <think> tags.",
		UserPrompt:   prompt,
		MaxTokens:    2048, // Reasoning models need headroom for thinking tokens
		Temperature:  &temp,
	})
	if err != nil {
		return "", err
	}
	return stripThinkTags(resp.Content), nil
}

// stripThinkTags removes <think>...</think> blocks and markdown code fences
// from reasoning model output to extract the raw JSON response.
func stripThinkTags(s string) string {
	// Remove <think>...</think> blocks
	s = thinkTagPattern.ReplaceAllString(s, "")

	// Remove markdown code fences (```json ... ``` or ``` ... ```)
	s = strings.TrimSpace(s)
	if strings.HasPrefix(s, "```") {
		lines := strings.Split(s, "\n")
		// Remove first line (```json or ```) and last line (```)
		if len(lines) >= 3 {
			end := len(lines) - 1
			for end > 0 && strings.TrimSpace(lines[end]) == "```" {
				end--
			}
			s = strings.Join(lines[1:end+1], "\n")
		}
	}

	return strings.TrimSpace(s)
}
