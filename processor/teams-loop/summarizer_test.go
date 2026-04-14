package teamsloop_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/c360studio/semstreams/agentic"
	"github.com/c360studio/semstreams/graph/llm"
	teamsloop "github.com/c360studio/semteams/processor/teams-loop"
)

// mockLLMClient implements llm.Client for testing.
type mockLLMClient struct {
	response *llm.ChatResponse
	err      error
	// lastReq captures the most recent ChatRequest for assertion.
	lastReq llm.ChatRequest
}

func (m *mockLLMClient) ChatCompletion(_ context.Context, req llm.ChatRequest) (*llm.ChatResponse, error) {
	m.lastReq = req
	if m.err != nil {
		return nil, m.err
	}
	return m.response, nil
}

func (m *mockLLMClient) Model() string { return "mock-model" }
func (m *mockLLMClient) Close() error  { return nil }

func TestLLMSummarizer_Summarize(t *testing.T) {
	messages := []agentic.ChatMessage{
		{Role: "user", Content: "What is the capital of France?"},
		{Role: "assistant", Content: "The capital of France is Paris."},
	}

	client := &mockLLMClient{
		response: &llm.ChatResponse{Content: "User asked about France capital; answer: Paris."},
	}
	summarizer := teamsloop.NewLLMSummarizer(client, nil)

	summary, err := summarizer.Summarize(context.Background(), messages, 512)

	if err != nil {
		t.Fatalf("Summarize() unexpected error: %v", err)
	}
	if summary != "User asked about France capital; answer: Paris." {
		t.Errorf("Summarize() = %q, want exact mock response", summary)
	}

	// Verify system prompt was passed
	if !strings.Contains(client.lastReq.SystemPrompt, "summarizer") {
		t.Errorf("SystemPrompt does not mention summarizer, got: %q", client.lastReq.SystemPrompt)
	}

	// Verify user prompt contains message roles and content
	if !strings.Contains(client.lastReq.UserPrompt, "[user]") {
		t.Errorf("UserPrompt missing [user] role marker, got: %q", client.lastReq.UserPrompt)
	}
	if !strings.Contains(client.lastReq.UserPrompt, "France") {
		t.Errorf("UserPrompt missing message content, got: %q", client.lastReq.UserPrompt)
	}

	// Verify MaxTokens matches provided budget
	if client.lastReq.MaxTokens != 512 {
		t.Errorf("MaxTokens = %d, want 512", client.lastReq.MaxTokens)
	}
}

func TestLLMSummarizer_Summarize_Error(t *testing.T) {
	client := &mockLLMClient{
		err: errors.New("upstream LLM unavailable"),
	}
	summarizer := teamsloop.NewLLMSummarizer(client, nil)

	messages := []agentic.ChatMessage{
		{Role: "user", Content: "Some message"},
	}

	_, err := summarizer.Summarize(context.Background(), messages, 256)

	if err == nil {
		t.Fatal("Summarize() expected error, got nil")
	}
	if !strings.Contains(err.Error(), "summarization LLM call failed") {
		t.Errorf("error = %q, want to contain 'summarization LLM call failed'", err.Error())
	}
	if !strings.Contains(err.Error(), "upstream LLM unavailable") {
		t.Errorf("error = %q, want to contain original error message", err.Error())
	}
}

func TestFormatMessagesForSummary_Roles(t *testing.T) {
	// Access via the Summarize path — use a capturing mock to inspect UserPrompt.
	client := &mockLLMClient{
		response: &llm.ChatResponse{Content: "ok"},
	}
	summarizer := teamsloop.NewLLMSummarizer(client, nil)

	messages := []agentic.ChatMessage{
		{Role: "user", Content: "Hello"},
		{Role: "assistant", Content: "Hi there"},
		{Role: "tool", Content: "tool output", ToolCallID: "call-1"},
	}

	_, err := summarizer.Summarize(context.Background(), messages, 256)
	if err != nil {
		t.Fatalf("Summarize() unexpected error: %v", err)
	}

	prompt := client.lastReq.UserPrompt
	for _, role := range []string{"user", "assistant", "tool"} {
		if !strings.Contains(prompt, "["+role+"]") {
			t.Errorf("UserPrompt missing [%s] role marker", role)
		}
	}
	if !strings.Contains(prompt, "Hello") || !strings.Contains(prompt, "Hi there") {
		t.Errorf("UserPrompt missing message content: %q", prompt)
	}
}

func TestFormatMessagesForSummary_ToolCalls(t *testing.T) {
	client := &mockLLMClient{
		response: &llm.ChatResponse{Content: "ok"},
	}
	summarizer := teamsloop.NewLLMSummarizer(client, nil)

	messages := []agentic.ChatMessage{
		{
			Role:    "assistant",
			Content: "",
			ToolCalls: []agentic.ToolCall{
				{ID: "call-1", Name: "search_graph", Arguments: map[string]any{"query": "France"}},
			},
		},
	}

	_, err := summarizer.Summarize(context.Background(), messages, 256)
	if err != nil {
		t.Fatalf("Summarize() unexpected error: %v", err)
	}

	prompt := client.lastReq.UserPrompt
	if !strings.Contains(prompt, "tool_call") {
		t.Errorf("UserPrompt missing tool_call annotation, got: %q", prompt)
	}
	if !strings.Contains(prompt, "search_graph") {
		t.Errorf("UserPrompt missing tool name 'search_graph', got: %q", prompt)
	}
}
