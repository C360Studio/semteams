package agenticmodel

import (
	"context"
	"encoding/json"
	"os"
	"time"

	"github.com/c360studio/semstreams/agentic"
	"github.com/c360studio/semstreams/model"
	"github.com/c360studio/semstreams/pkg/errs"
	openai "github.com/sashabaranov/go-openai"
)

// Client wraps OpenAI SDK for agentic model requests
type Client struct {
	client   *openai.Client
	endpoint *model.EndpointConfig
}

// NewClient creates a new client for the given endpoint configuration
func NewClient(endpoint *model.EndpointConfig) (*Client, error) {
	if endpoint == nil {
		return nil, errs.WrapInvalid(errs.ErrMissingConfig, "Client", "NewClient", "endpoint is nil")
	}
	if endpoint.Model == "" {
		return nil, errs.WrapInvalid(errs.ErrMissingConfig, "Client", "NewClient", "check model")
	}

	// Get API key from environment if specified
	apiKey := ""
	if endpoint.APIKeyEnv != "" {
		apiKey = os.Getenv(endpoint.APIKeyEnv)
	}

	// Create OpenAI client config
	config := openai.DefaultConfig(apiKey)
	if endpoint.URL != "" {
		config.BaseURL = endpoint.URL
	}

	client := openai.NewClientWithConfig(config)

	return &Client{
		client:   client,
		endpoint: endpoint,
	}, nil
}

// ChatCompletion sends a chat completion request
func (c *Client) ChatCompletion(ctx context.Context, req agentic.AgentRequest) (agentic.AgentResponse, error) {
	// Convert AgentRequest to OpenAI format
	messages := make([]openai.ChatCompletionMessage, len(req.Messages))
	for i, msg := range req.Messages {
		messages[i] = openai.ChatCompletionMessage{
			Role:             msg.Role,
			Content:          msg.Content,
			ReasoningContent: msg.ReasoningContent,
		}

		// Handle tool results - include tool_call_id (required by OpenAI API)
		if msg.Role == "tool" && msg.ToolCallID != "" {
			messages[i].ToolCallID = msg.ToolCallID
		}

		// Convert tool calls if present
		if len(msg.ToolCalls) > 0 {
			toolCalls := make([]openai.ToolCall, len(msg.ToolCalls))
			for j, tc := range msg.ToolCalls {
				argsJSON, _ := json.Marshal(tc.Arguments)
				toolCalls[j] = openai.ToolCall{
					ID:   tc.ID,
					Type: openai.ToolTypeFunction,
					Function: openai.FunctionCall{
						Name:      tc.Name,
						Arguments: string(argsJSON),
					},
				}
			}
			messages[i].ToolCalls = toolCalls
		}
	}

	// Build request
	chatReq := openai.ChatCompletionRequest{
		Model:    c.endpoint.Model,
		Messages: messages,
	}

	if req.MaxTokens > 0 {
		chatReq.MaxTokens = req.MaxTokens
	}

	if req.Temperature > 0 {
		chatReq.Temperature = float32(req.Temperature)
	}

	// Forward provider-specific options as chat_template_kwargs
	// (e.g. enable_thinking, thinking_budget for vLLM/ollama thinking models)
	if len(c.endpoint.Options) > 0 {
		chatReq.ChatTemplateKwargs = c.endpoint.Options
	}

	// Convert tools if present
	if len(req.Tools) > 0 {
		tools := make([]openai.Tool, len(req.Tools))
		for i, tool := range req.Tools {
			tools[i] = openai.Tool{
				Type: openai.ToolTypeFunction,
				Function: &openai.FunctionDefinition{
					Name:        tool.Name,
					Description: tool.Description,
					Parameters:  tool.Parameters,
				},
			}
		}
		chatReq.Tools = tools
	}

	// Make request with retry logic
	response := agentic.AgentResponse{
		RequestID: req.RequestID,
	}

	maxAttempts := 3 // Default retry count
	for attempt := 0; attempt < maxAttempts; attempt++ {
		if attempt > 0 {
			// Exponential backoff
			backoffDuration := time.Duration(1<<uint(attempt-1)) * 100 * time.Millisecond
			select {
			case <-ctx.Done():
				response.Status = "error"
				response.Error = ctx.Err().Error()
				return response, nil
			case <-time.After(backoffDuration):
			}
		}

		resp, err := c.client.CreateChatCompletion(ctx, chatReq)
		if err != nil {
			// Check if this is a retryable error
			if attempt < maxAttempts-1 && isRetryable(err) {
				continue
			}

			// Final error - return error response
			response.Status = "error"
			response.Error = err.Error()
			return response, nil
		}

		// Success - convert response
		return c.convertResponse(resp, req.RequestID), nil
	}

	// Should not reach here, but handle it
	response.Status = "error"
	response.Error = "maximum retry attempts exceeded"
	return response, nil
}

// convertResponse converts OpenAI response to AgentResponse
func (c *Client) convertResponse(resp openai.ChatCompletionResponse, requestID string) agentic.AgentResponse {
	response := agentic.AgentResponse{
		RequestID: requestID,
	}

	if len(resp.Choices) == 0 {
		response.Status = "error"
		response.Error = "no choices in response"
		return response
	}

	choice := resp.Choices[0]

	// Map finish reason to status
	switch choice.FinishReason {
	case "stop", "length":
		response.Status = "complete"
	case "tool_calls":
		response.Status = "tool_call"
	default:
		response.Status = "complete"
	}

	// Convert message
	response.Message = agentic.ChatMessage{
		Role:             choice.Message.Role,
		Content:          choice.Message.Content,
		ReasoningContent: choice.Message.ReasoningContent,
	}

	// Convert tool calls if present
	if len(choice.Message.ToolCalls) > 0 {
		response.Status = "tool_call"
		toolCalls := make([]agentic.ToolCall, len(choice.Message.ToolCalls))
		for i, tc := range choice.Message.ToolCalls {
			// Parse arguments JSON
			var args map[string]any
			if tc.Function.Arguments != "" {
				json.Unmarshal([]byte(tc.Function.Arguments), &args)
			}

			toolCalls[i] = agentic.ToolCall{
				ID:        tc.ID,
				Name:      tc.Function.Name,
				Arguments: args,
			}
		}
		response.Message.ToolCalls = toolCalls
	}

	// Convert token usage
	response.TokenUsage = agentic.TokenUsage{
		PromptTokens:     resp.Usage.PromptTokens,
		CompletionTokens: resp.Usage.CompletionTokens,
	}

	return response
}

// Close releases resources held by the client
func (c *Client) Close() error {
	// OpenAI SDK client doesn't require explicit cleanup
	// HTTP connections are managed by http.Client's connection pooling
	return nil
}

// isRetryable checks if an error should trigger a retry
func isRetryable(err error) bool {
	if err == nil {
		return false
	}

	// Check for context errors (not retryable)
	if err == context.Canceled || err == context.DeadlineExceeded {
		return false
	}

	// For now, retry all other errors
	// In production, would check specific HTTP status codes
	return true
}
