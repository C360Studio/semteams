package query

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/c360studio/semstreams/agentic"
	"github.com/c360studio/semstreams/bridge/trustgraph/client"
)

func TestExecutor_ListTools(t *testing.T) {
	e := NewDefaultExecutor()
	tools := e.ListTools()

	if len(tools) != 1 {
		t.Fatalf("Expected 1 tool, got %d", len(tools))
	}

	tool := tools[0]
	if tool.Name != "trustgraph_query" {
		t.Errorf("Tool name = %q, want trustgraph_query", tool.Name)
	}
	if tool.Description == "" {
		t.Error("Tool description should not be empty")
	}

	// Check parameters
	params := tool.Parameters
	if params == nil {
		t.Fatal("Parameters should not be nil")
	}

	properties, ok := params["properties"].(map[string]any)
	if !ok {
		t.Fatal("Parameters should have properties")
	}

	if _, ok := properties["query"]; !ok {
		t.Error("Parameters should have query property")
	}
	if _, ok := properties["collection"]; !ok {
		t.Error("Parameters should have collection property")
	}

	required, ok := params["required"].([]string)
	if !ok {
		t.Fatal("Parameters should have required array")
	}
	if len(required) != 1 || required[0] != "query" {
		t.Error("query should be the only required parameter")
	}
}

func TestExecutor_Execute_Success(t *testing.T) {
	// Create mock server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := struct {
			Response struct {
				Response string `json:"response"`
			} `json:"response"`
			Complete bool `json:"complete"`
		}{
			Complete: true,
		}
		resp.Response.Response = "The answer based on the documents is 42."

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	e := NewExecutor(Config{
		Endpoint: server.URL,
		FlowID:   "test-flow",
		Timeout:  5 * time.Second,
	})

	call := agentic.ToolCall{
		ID:   "call-123",
		Name: "trustgraph_query",
		Arguments: map[string]any{
			"query": "What is the answer?",
		},
	}

	result, err := e.Execute(context.Background(), call)
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}

	if result.CallID != "call-123" {
		t.Errorf("CallID = %q, want call-123", result.CallID)
	}
	if result.Error != "" {
		t.Errorf("Unexpected error: %s", result.Error)
	}
	if result.Content != "The answer based on the documents is 42." {
		t.Errorf("Content = %q, want 'The answer based on the documents is 42.'", result.Content)
	}
}

func TestExecutor_Execute_WithCollection(t *testing.T) {
	var receivedRequest struct {
		Request struct {
			Collection string `json:"collection"`
		} `json:"request"`
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewDecoder(r.Body).Decode(&receivedRequest)

		resp := struct {
			Response struct {
				Response string `json:"response"`
			} `json:"response"`
			Complete bool `json:"complete"`
		}{
			Complete: true,
		}
		resp.Response.Response = "Result from collection"

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	e := NewExecutor(Config{
		Endpoint: server.URL,
		FlowID:   "test-flow",
		Timeout:  5 * time.Second,
	})

	call := agentic.ToolCall{
		ID:   "call-456",
		Name: "trustgraph_query",
		Arguments: map[string]any{
			"query":      "What procedures exist?",
			"collection": "procedures",
		},
	}

	result, err := e.Execute(context.Background(), call)
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}

	if result.Error != "" {
		t.Errorf("Unexpected error: %s", result.Error)
	}
	if receivedRequest.Request.Collection != "procedures" {
		t.Errorf("Request collection = %q, want procedures", receivedRequest.Request.Collection)
	}
}

func TestExecutor_Execute_MissingQuery(t *testing.T) {
	e := NewDefaultExecutor()

	call := agentic.ToolCall{
		ID:        "call-789",
		Name:      "trustgraph_query",
		Arguments: map[string]any{},
	}

	result, err := e.Execute(context.Background(), call)
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}

	if result.Error == "" {
		t.Error("Expected error for missing query")
	}
	if result.CallID != "call-789" {
		t.Errorf("CallID = %q, want call-789", result.CallID)
	}
}

func TestExecutor_Execute_UnknownTool(t *testing.T) {
	e := NewDefaultExecutor()

	call := agentic.ToolCall{
		ID:   "call-unknown",
		Name: "unknown_tool",
		Arguments: map[string]any{
			"query": "test",
		},
	}

	result, err := e.Execute(context.Background(), call)
	if err == nil {
		t.Error("Expected error for unknown tool")
	}
	if result.Error == "" {
		t.Error("Result should have error for unknown tool")
	}
}

func TestExecutor_Execute_APIError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte("internal server error"))
	}))
	defer server.Close()

	e := NewExecutor(Config{
		Endpoint: server.URL,
		FlowID:   "test-flow",
		Timeout:  1 * time.Second,
	})

	// Override client to have no retries for faster test
	e.client = client.New(client.Config{
		Endpoint:   server.URL,
		Timeout:    1 * time.Second,
		MaxRetries: 0,
	})

	call := agentic.ToolCall{
		ID:   "call-error",
		Name: "trustgraph_query",
		Arguments: map[string]any{
			"query": "This will fail",
		},
	}

	result, err := e.Execute(context.Background(), call)
	// Error should be in result, not as Go error
	if err != nil {
		t.Fatalf("Execute returned Go error: %v", err)
	}
	if result.Error == "" {
		t.Error("Expected error in result for API failure")
	}
}

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()

	if cfg.Endpoint != DefaultEndpoint {
		t.Errorf("Endpoint = %q, want %q", cfg.Endpoint, DefaultEndpoint)
	}
	if cfg.FlowID != DefaultFlowID {
		t.Errorf("FlowID = %q, want %q", cfg.FlowID, DefaultFlowID)
	}
	if cfg.Timeout != DefaultTimeout {
		t.Errorf("Timeout = %v, want %v", cfg.Timeout, DefaultTimeout)
	}
}
