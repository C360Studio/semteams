package agenticmodel

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"os"
	"time"

	"github.com/c360studio/semstreams/agentic"
	"github.com/c360studio/semstreams/model"
	"github.com/c360studio/semstreams/pkg/errs"
	"github.com/c360studio/semstreams/pkg/retry"
	openai "github.com/sashabaranov/go-openai"
)

// Client wraps OpenAI SDK for agentic model requests
type Client struct {
	client       *openai.Client
	endpoint     *model.EndpointConfig
	chunkHandler ChunkHandler
	metrics      *modelMetrics
	logger       *slog.Logger
	throttle     *EndpointThrottle
	retryCfg     RetryConfig
}

// defaultClientRetryConfig is the retry configuration used when the component
// does not supply one. The short initial delay keeps unit tests fast.
var defaultClientRetryConfig = RetryConfig{
	MaxAttempts:    3,
	Backoff:        "exponential",
	InitialDelay:   "100ms",
	MaxDelay:       "30s",
	RateLimitDelay: "5s",
}

// NewClient creates a new client for the given endpoint configuration.
// The default retry config is suitable for unit tests (3 attempts, 100ms initial delay).
// Call SetRetryConfig before ChatCompletion to apply production settings.
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
		retryCfg: defaultClientRetryConfig,
	}, nil
}

// SetChunkHandler sets the callback for receiving streaming deltas.
func (c *Client) SetChunkHandler(handler ChunkHandler) {
	c.chunkHandler = handler
}

// SetMetrics sets the metrics instance for recording streaming metrics.
func (c *Client) SetMetrics(m *modelMetrics) {
	c.metrics = m
}

// SetLogger sets the logger for debug-level request/response logging.
func (c *Client) SetLogger(l *slog.Logger) {
	c.logger = l
}

// SetThrottle attaches a rate/concurrency limiter to this client.
func (c *Client) SetThrottle(t *EndpointThrottle) {
	c.throttle = t
}

// SetRetryConfig replaces the default retry configuration.
// Call this after NewClient to apply production settings.
func (c *Client) SetRetryConfig(cfg RetryConfig) {
	c.retryCfg = cfg
}

// buildChatRequest converts an AgentRequest into an OpenAI ChatCompletionRequest.
func (c *Client) buildChatRequest(req agentic.AgentRequest) openai.ChatCompletionRequest {
	messages := make([]openai.ChatCompletionMessage, len(req.Messages))
	for i, msg := range req.Messages {
		messages[i] = openai.ChatCompletionMessage{
			Role:    msg.Role,
			Content: msg.Content,
			// ReasoningContent is intentionally omitted from outgoing requests.
			// Providers return it in responses but reject it in request messages
			// (e.g., Gemini returns 400 INVALID_ARGUMENT).
		}

		// Handle tool results - include tool_call_id (required by OpenAI API)
		if msg.Role == "tool" && msg.ToolCallID != "" {
			messages[i].ToolCallID = msg.ToolCallID
		}

		// Handle tool result name field (required by Gemini)
		if msg.Role == "tool" && msg.Name != "" {
			messages[i].Name = msg.Name
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

			// Gemini rejects absent content on assistant tool_call messages.
			// Set to single space (standard adapter convention from LiteLLM, etc.).
			if messages[i].Content == "" {
				messages[i].Content = " "
			}
		}
	}

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
	if len(c.endpoint.Options) > 0 {
		chatReq.ChatTemplateKwargs = c.endpoint.Options
	}
	if c.endpoint.ReasoningEffort != "" {
		chatReq.ReasoningEffort = c.endpoint.ReasoningEffort
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

	return chatReq
}

// ChatCompletion sends a chat completion request with retry and throttling.
func (c *Client) ChatCompletion(ctx context.Context, req agentic.AgentRequest) (agentic.AgentResponse, error) {
	chatReq := c.buildChatRequest(req)

	// Log full request payload at debug level for wire-level diagnostics
	if c.logger != nil && c.logger.Enabled(context.Background(), slog.LevelDebug) {
		if payload, err := json.Marshal(chatReq); err == nil {
			c.logger.Debug("OpenAI API request payload",
				slog.String("request_id", req.RequestID),
				slog.String("model", chatReq.Model),
				slog.Int("message_count", len(chatReq.Messages)),
				slog.String("payload", string(payload)))
		}
	}

	// Acquire throttle slot before any network attempt.
	if c.throttle != nil {
		if err := c.throttle.Acquire(ctx); err != nil {
			return agentic.AgentResponse{
				RequestID: req.RequestID,
				Status:    "error",
				Error:     err.Error(),
			}, nil
		}
		defer c.throttle.Release()
	}

	var lastResp agentic.AgentResponse
	lastResp.RequestID = req.RequestID

	retryCfg := c.buildRetryConfig()

	err := retry.Do(ctx, retryCfg, func() error {
		if c.endpoint.Stream {
			resp, err := c.streamChatCompletion(ctx, chatReq, req.RequestID)
			if err != nil {
				// Connection error before the stream opened — retryable.
				if c.logger != nil {
					c.logger.Debug("OpenAI stream connection failed",
						slog.String("request_id", req.RequestID),
						slog.String("model", chatReq.Model),
						slog.String("error", err.Error()))
				}
				if !isRetryable(err) {
					return retry.NonRetryable(err)
				}
				return err
			}
			// Mid-stream errors are returned as AgentResponse{Status:"error"} — not retried.
			lastResp = resp
			return nil
		}

		resp, err := c.client.CreateChatCompletion(ctx, chatReq)
		if err != nil {
			if c.logger != nil {
				c.logger.Debug("OpenAI API request failed",
					slog.String("request_id", req.RequestID),
					slog.String("model", chatReq.Model),
					slog.String("error", err.Error()))
			}

			if isRateLimited(err) {
				if c.metrics != nil {
					c.metrics.recordRateLimitHit(chatReq.Model)
				}
				// Honour Retry-After header if present; otherwise use configured delay.
				waitDur := c.rateLimitWait(err)
				select {
				case <-ctx.Done():
					return retry.NonRetryable(ctx.Err())
				case <-time.After(waitDur):
				}
				return err // retryable — pkg/retry will add its own backoff on top
			}

			if !isRetryable(err) {
				return retry.NonRetryable(err)
			}
			return err
		}

		lastResp = c.convertResponse(resp, req.RequestID)
		return nil
	})

	if err != nil {
		// Unwrap NonRetryableError so the message stays clean.
		var nre *retry.NonRetryableError
		if errors.As(err, &nre) {
			err = nre.Unwrap()
		}
		return agentic.AgentResponse{
			RequestID: req.RequestID,
			Status:    "error",
			Error:     err.Error(),
		}, nil
	}

	return lastResp, nil
}

// buildRetryConfig converts the component RetryConfig into a pkg/retry Config.
func (c *Client) buildRetryConfig() retry.Config {
	initialDelay := c.retryCfg.initialDelayDuration(100 * time.Millisecond)
	maxDelay := c.retryCfg.maxDelayDuration(30 * time.Second)

	return retry.Config{
		MaxAttempts:  c.retryCfg.MaxAttempts,
		InitialDelay: initialDelay,
		MaxDelay:     maxDelay,
		Multiplier:   2.0,
		AddJitter:    true,
	}
}

// rateLimitWait returns the duration to wait after a 429 response.
// It honours the Retry-After header when present.
func (c *Client) rateLimitWait(err error) time.Duration {
	// Try to extract Retry-After from the OpenAI API error header bag.
	// The OpenAI SDK does not directly expose response headers, but the
	// error message sometimes contains the value. We use the configured
	// default and let the operator tune it via RateLimitDelay.
	if ra := retryAfterFromError(err); ra > 0 {
		return ra
	}
	return c.retryCfg.rateLimitDelayDuration(5 * time.Second)
}

// retryAfterFromError attempts to extract a Retry-After wait duration from a
// 429 error. The go-openai SDK does not expose raw HTTP response headers, so
// this currently always returns 0, causing the caller to fall back to the
// configured RateLimitDelay. A future improvement could wrap the HTTP transport
// to capture headers before they are discarded by the SDK.
func retryAfterFromError(_ error) time.Duration {
	return 0
}

// streamChatCompletion handles the streaming path. Connection errors return
// a Go error (retryable). Mid-stream errors return AgentResponse{Status: "error"}
// (not retryable — partial state can't be replayed).
func (c *Client) streamChatCompletion(ctx context.Context, chatReq openai.ChatCompletionRequest, requestID string) (agentic.AgentResponse, error) {
	chatReq.Stream = true
	chatReq.StreamOptions = &openai.StreamOptions{IncludeUsage: true}

	stream, err := c.client.CreateChatCompletionStream(ctx, chatReq)
	if err != nil {
		return agentic.AgentResponse{}, err // retryable connection error
	}
	defer stream.Close()

	acc := &streamAccumulator{}
	streamStart := time.Now()
	firstTokenRecorded := false

	for {
		chunk, err := stream.Recv()
		if err == io.EOF {
			break
		}
		if err != nil {
			// Mid-stream error — not retryable. Preserve any partial tokens
			// the accumulator received before the connection died.
			resp := acc.toAgentResponse(requestID)
			resp.Status = "error"
			resp.Error = err.Error()
			return resp, nil
		}

		// Record usage from the final chunk (has empty choices)
		if chunk.Usage != nil {
			acc.setUsage(chunk.Usage)
		}

		// Process choice deltas
		for _, choice := range chunk.Choices {
			acc.processDelta(choice)

			// Build chunk for handler
			if c.chunkHandler != nil {
				sc := StreamChunk{
					RequestID:      requestID,
					ContentDelta:   choice.Delta.Content,
					ReasoningDelta: choice.Delta.ReasoningContent,
				}
				c.chunkHandler(sc)
			}

			// Record streaming metrics
			if c.metrics != nil {
				c.metrics.recordStreamChunk(chatReq.Model)
				if !firstTokenRecorded && (choice.Delta.Content != "" || choice.Delta.ReasoningContent != "") {
					c.metrics.recordStreamTTFT(chatReq.Model, time.Since(streamStart).Seconds())
					firstTokenRecorded = true
				}
			}
		}
	}

	// Send done signal to handler
	if c.chunkHandler != nil {
		c.chunkHandler(StreamChunk{RequestID: requestID, Done: true})
	}

	return acc.toAgentResponse(requestID), nil
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

// isRetryable checks if an error should trigger a retry.
// Context errors and unrecoverable 4xx responses are never retried.
func isRetryable(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return false
	}

	var apiErr *openai.APIError
	if errors.As(err, &apiErr) {
		return isRetryableStatusCode(apiErr.HTTPStatusCode)
	}
	var reqErr *openai.RequestError
	if errors.As(err, &reqErr) {
		return isRetryableStatusCode(reqErr.HTTPStatusCode)
	}

	// Network-level errors (no HTTP status) are retryable.
	return true
}

// isRetryableStatusCode returns true for HTTP status codes that are worth retrying.
func isRetryableStatusCode(code int) bool {
	if code == 0 {
		// No status code — treat as network error, retryable.
		return true
	}
	switch code {
	case http.StatusTooManyRequests, // 429
		http.StatusInternalServerError, // 500
		http.StatusBadGateway,          // 502
		http.StatusServiceUnavailable,  // 503
		http.StatusGatewayTimeout:      // 504
		return true
	default:
		// All other 4xx errors (400, 401, 403, 404) are client errors — not retryable.
		return false
	}
}

// isRateLimited reports whether the error is an HTTP 429 Too Many Requests.
func isRateLimited(err error) bool {
	var apiErr *openai.APIError
	if errors.As(err, &apiErr) && apiErr.HTTPStatusCode == http.StatusTooManyRequests {
		return true
	}
	var reqErr *openai.RequestError
	if errors.As(err, &reqErr) && reqErr.HTTPStatusCode == http.StatusTooManyRequests {
		return true
	}
	return false
}
