package query

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/c360studio/semstreams/agentic"
	"github.com/c360studio/semstreams/bridge/trustgraph/client"
)

// DefaultEndpoint is the default TrustGraph API endpoint.
const DefaultEndpoint = "http://localhost:8088"

// DefaultFlowID is the default GraphRAG flow ID.
const DefaultFlowID = "graph-rag"

// DefaultTimeout is the default query timeout.
const DefaultTimeout = 120 * time.Second

// Config holds configuration for the TrustGraph query executor.
type Config struct {
	// Endpoint is the TrustGraph API base URL
	Endpoint string

	// APIKey is an optional API key for authentication
	APIKey string

	// FlowID is the GraphRAG flow ID to use
	FlowID string

	// Timeout is the query timeout
	Timeout time.Duration
}

// DefaultConfig returns the default configuration, reading from environment variables.
func DefaultConfig() Config {
	endpoint := os.Getenv("TRUSTGRAPH_ENDPOINT")
	if endpoint == "" {
		endpoint = DefaultEndpoint
	}

	apiKey := os.Getenv("TRUSTGRAPH_API_KEY")

	flowID := os.Getenv("TRUSTGRAPH_FLOW_ID")
	if flowID == "" {
		flowID = DefaultFlowID
	}

	timeout := DefaultTimeout
	if timeoutStr := os.Getenv("TRUSTGRAPH_TIMEOUT"); timeoutStr != "" {
		if d, err := time.ParseDuration(timeoutStr); err == nil {
			timeout = d
		}
	}

	return Config{
		Endpoint: endpoint,
		APIKey:   apiKey,
		FlowID:   flowID,
		Timeout:  timeout,
	}
}

// Executor implements the TrustGraph query tool executor.
type Executor struct {
	client *client.Client
	flowID string
}

// NewExecutor creates a new TrustGraph query executor with the given configuration.
func NewExecutor(cfg Config) *Executor {
	c := client.New(client.Config{
		Endpoint:   cfg.Endpoint,
		APIKey:     cfg.APIKey,
		Timeout:    cfg.Timeout,
		MaxRetries: 2, // GraphRAG queries can be slow, limit retries
	})

	return &Executor{
		client: c,
		flowID: cfg.FlowID,
	}
}

// NewDefaultExecutor creates a new executor with default configuration.
func NewDefaultExecutor() *Executor {
	return NewExecutor(DefaultConfig())
}

// ListTools returns the tool definitions provided by this executor.
func (e *Executor) ListTools() []agentic.ToolDefinition {
	return []agentic.ToolDefinition{
		{
			Name:        "trustgraph_query",
			Description: "Query TrustGraph's document knowledge graph using natural language. Use this tool for questions about documents, reports, procedures, or extracted knowledge that isn't in the operational sensor data. TrustGraph excels at answering questions that require reasoning over document content.",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"query": map[string]any{
						"type":        "string",
						"description": "Natural language question to ask the knowledge graph. Be specific and clear about what information you need.",
					},
					"collection": map[string]any{
						"type":        "string",
						"description": "Optional: Specific knowledge collection to query (e.g., 'intelligence', 'procedures'). If not specified, queries all collections.",
					},
				},
				"required": []string{"query"},
			},
		},
	}
}

// Execute executes a tool call and returns the result.
func (e *Executor) Execute(ctx context.Context, call agentic.ToolCall) (agentic.ToolResult, error) {
	switch call.Name {
	case "trustgraph_query":
		return e.executeQuery(ctx, call)
	default:
		return agentic.ToolResult{
			CallID: call.ID,
			Error:  fmt.Sprintf("unknown tool: %s", call.Name),
		}, fmt.Errorf("unknown tool: %s", call.Name)
	}
}

func (e *Executor) executeQuery(ctx context.Context, call agentic.ToolCall) (agentic.ToolResult, error) {
	// Extract query parameter
	query, ok := call.Arguments["query"].(string)
	if !ok || query == "" {
		return agentic.ToolResult{
			CallID: call.ID,
			Error:  "query is required and must be a non-empty string",
		}, nil
	}

	// Extract optional collection parameter
	collection, _ := call.Arguments["collection"].(string)

	// Execute GraphRAG query
	var response string
	var err error

	if collection != "" {
		response, err = e.client.GraphRAGWithCollection(ctx, e.flowID, query, collection)
	} else {
		response, err = e.client.GraphRAG(ctx, e.flowID, query)
	}

	if err != nil {
		// Return error as tool result, not as Go error
		// This allows the agent to see and handle the error
		return agentic.ToolResult{
			CallID: call.ID,
			Error:  fmt.Sprintf("TrustGraph query failed: %v", err),
			Metadata: map[string]any{
				"query":      query,
				"collection": collection,
			},
		}, nil
	}

	return agentic.ToolResult{
		CallID:  call.ID,
		Content: response,
		Metadata: map[string]any{
			"query":      query,
			"collection": collection,
			"flow_id":    e.flowID,
		},
	}, nil
}
