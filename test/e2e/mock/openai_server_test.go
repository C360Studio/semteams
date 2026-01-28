package mock

import (
	"bytes"
	"encoding/json"
	"net/http"
	"testing"
	"time"
)

func TestOpenAIServer_Start(t *testing.T) {
	server := NewOpenAIServer()
	err := server.Start(":0")
	if err != nil {
		t.Fatalf("failed to start server: %v", err)
	}
	defer server.Stop()

	if server.Addr() == "" {
		t.Error("expected non-empty address")
	}

	if server.URL() == "" {
		t.Error("expected non-empty URL")
	}
}

func TestOpenAIServer_HealthEndpoint(t *testing.T) {
	server := NewOpenAIServer()
	if err := server.Start(":0"); err != nil {
		t.Fatalf("failed to start server: %v", err)
	}
	defer server.Stop()

	resp, err := http.Get(server.URL() + "/health")
	if err != nil {
		t.Fatalf("health check failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected status 200, got %d", resp.StatusCode)
	}

	var result map[string]string
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if result["status"] != "ok" {
		t.Errorf("expected status ok, got %s", result["status"])
	}
}

func TestOpenAIServer_SimpleCompletion(t *testing.T) {
	server := NewOpenAIServer().
		WithCompletionContent("Test response content")

	if err := server.Start(":0"); err != nil {
		t.Fatalf("failed to start server: %v", err)
	}
	defer server.Stop()

	req := ChatCompletionRequest{
		Model: "test-model",
		Messages: []ChatMessage{
			{Role: "user", Content: "Hello"},
		},
	}

	resp := makeRequest(t, server.URL()+"/v1/chat/completions", req)

	if resp.Model != "test-model" {
		t.Errorf("expected model test-model, got %s", resp.Model)
	}

	if len(resp.Choices) != 1 {
		t.Fatalf("expected 1 choice, got %d", len(resp.Choices))
	}

	choice := resp.Choices[0]
	if choice.Message.Role != "assistant" {
		t.Errorf("expected role assistant, got %s", choice.Message.Role)
	}

	if choice.Message.Content != "Test response content" {
		t.Errorf("expected 'Test response content', got %s", choice.Message.Content)
	}

	if choice.FinishReason != "stop" {
		t.Errorf("expected finish_reason stop, got %s", choice.FinishReason)
	}
}

func TestOpenAIServer_ToolCallFlow(t *testing.T) {
	server := NewOpenAIServer().
		WithToolArgs("query_entity", `{"entity_id": "test-entity-001"}`).
		WithCompletionContent("Analysis based on tool results")

	if err := server.Start(":0"); err != nil {
		t.Fatalf("failed to start server: %v", err)
	}
	defer server.Stop()

	// First request: with tools, should return tool_call
	req1 := ChatCompletionRequest{
		Model: "test-model",
		Messages: []ChatMessage{
			{Role: "user", Content: "Analyze entity"},
		},
		Tools: []Tool{{
			Type: "function",
			Function: FunctionDef{
				Name:        "query_entity",
				Description: "Query an entity",
				Parameters:  map[string]any{"type": "object"},
			},
		}},
	}

	resp1 := makeRequest(t, server.URL()+"/v1/chat/completions", req1)

	if len(resp1.Choices) != 1 {
		t.Fatalf("expected 1 choice, got %d", len(resp1.Choices))
	}

	choice1 := resp1.Choices[0]
	if choice1.FinishReason != "tool_calls" {
		t.Errorf("expected finish_reason tool_calls, got %s", choice1.FinishReason)
	}

	if len(choice1.Message.ToolCalls) != 1 {
		t.Fatalf("expected 1 tool call, got %d", len(choice1.Message.ToolCalls))
	}

	toolCall := choice1.Message.ToolCalls[0]
	if toolCall.Function.Name != "query_entity" {
		t.Errorf("expected tool name query_entity, got %s", toolCall.Function.Name)
	}

	if toolCall.Function.Arguments != `{"entity_id": "test-entity-001"}` {
		t.Errorf("unexpected arguments: %s", toolCall.Function.Arguments)
	}

	if toolCall.ID == "" {
		t.Error("expected non-empty tool call ID")
	}

	// Second request: with tool results, should return completion
	req2 := ChatCompletionRequest{
		Model: "test-model",
		Messages: []ChatMessage{
			{Role: "user", Content: "Analyze entity"},
			{Role: "assistant", ToolCalls: choice1.Message.ToolCalls},
			{Role: "tool", ToolCallID: toolCall.ID, Content: `{"id": "test-entity-001", "type": "sensor"}`},
		},
		Tools: []Tool{{
			Type: "function",
			Function: FunctionDef{
				Name:        "query_entity",
				Description: "Query an entity",
				Parameters:  map[string]any{"type": "object"},
			},
		}},
	}

	resp2 := makeRequest(t, server.URL()+"/v1/chat/completions", req2)

	if len(resp2.Choices) != 1 {
		t.Fatalf("expected 1 choice, got %d", len(resp2.Choices))
	}

	choice2 := resp2.Choices[0]
	if choice2.FinishReason != "stop" {
		t.Errorf("expected finish_reason stop, got %s", choice2.FinishReason)
	}

	if choice2.Message.Content != "Analysis based on tool results" {
		t.Errorf("unexpected content: %s", choice2.Message.Content)
	}
}

func TestOpenAIServer_RequestTracking(t *testing.T) {
	server := NewOpenAIServer()
	if err := server.Start(":0"); err != nil {
		t.Fatalf("failed to start server: %v", err)
	}
	defer server.Stop()

	if server.RequestCount() != 0 {
		t.Errorf("expected 0 requests, got %d", server.RequestCount())
	}

	req := ChatCompletionRequest{
		Model: "test-model",
		Messages: []ChatMessage{
			{Role: "user", Content: "Hello"},
		},
	}

	makeRequest(t, server.URL()+"/v1/chat/completions", req)

	if server.RequestCount() != 1 {
		t.Errorf("expected 1 request, got %d", server.RequestCount())
	}

	lastReq := server.LastRequest()
	if lastReq == nil {
		t.Fatal("expected last request to be set")
	}

	if lastReq.Model != "test-model" {
		t.Errorf("expected model test-model, got %s", lastReq.Model)
	}
}

func TestOpenAIServer_RequestDelay(t *testing.T) {
	server := NewOpenAIServer().
		WithRequestDelay(100 * time.Millisecond)

	if err := server.Start(":0"); err != nil {
		t.Fatalf("failed to start server: %v", err)
	}
	defer server.Stop()

	req := ChatCompletionRequest{
		Model: "test-model",
		Messages: []ChatMessage{
			{Role: "user", Content: "Hello"},
		},
	}

	start := time.Now()
	makeRequest(t, server.URL()+"/v1/chat/completions", req)
	elapsed := time.Since(start)

	if elapsed < 100*time.Millisecond {
		t.Errorf("expected at least 100ms delay, got %v", elapsed)
	}
}

func TestOpenAIServer_UnknownTool(t *testing.T) {
	server := NewOpenAIServer()
	if err := server.Start(":0"); err != nil {
		t.Fatalf("failed to start server: %v", err)
	}
	defer server.Stop()

	req := ChatCompletionRequest{
		Model: "test-model",
		Messages: []ChatMessage{
			{Role: "user", Content: "Do something"},
		},
		Tools: []Tool{{
			Type: "function",
			Function: FunctionDef{
				Name:        "unknown_tool",
				Description: "Unknown tool",
				Parameters:  map[string]any{"type": "object"},
			},
		}},
	}

	resp := makeRequest(t, server.URL()+"/v1/chat/completions", req)

	// Should still return tool call with empty args
	if len(resp.Choices) != 1 {
		t.Fatalf("expected 1 choice, got %d", len(resp.Choices))
	}

	toolCall := resp.Choices[0].Message.ToolCalls[0]
	if toolCall.Function.Arguments != "{}" {
		t.Errorf("expected empty args for unknown tool, got %s", toolCall.Function.Arguments)
	}
}

func TestOpenAIServer_UsageStats(t *testing.T) {
	server := NewOpenAIServer()
	if err := server.Start(":0"); err != nil {
		t.Fatalf("failed to start server: %v", err)
	}
	defer server.Stop()

	req := ChatCompletionRequest{
		Model: "test-model",
		Messages: []ChatMessage{
			{Role: "user", Content: "Hello"},
		},
	}

	resp := makeRequest(t, server.URL()+"/v1/chat/completions", req)

	if resp.Usage.PromptTokens <= 0 {
		t.Error("expected positive prompt tokens")
	}

	if resp.Usage.CompletionTokens <= 0 {
		t.Error("expected positive completion tokens")
	}

	if resp.Usage.TotalTokens != resp.Usage.PromptTokens+resp.Usage.CompletionTokens {
		t.Error("total tokens should equal prompt + completion")
	}
}

func makeRequest(t *testing.T, url string, req ChatCompletionRequest) ChatCompletionResponse {
	t.Helper()

	body, err := json.Marshal(req)
	if err != nil {
		t.Fatalf("failed to marshal request: %v", err)
	}

	resp, err := http.Post(url, "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected status 200, got %d", resp.StatusCode)
	}

	var result ChatCompletionResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	return result
}
