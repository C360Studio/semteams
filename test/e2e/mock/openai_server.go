// Package mock provides test doubles for external services.
package mock

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"sync"
	"time"

	"github.com/google/uuid"
)

// ChatCompletionRequest matches OpenAI API request format.
type ChatCompletionRequest struct {
	Model       string        `json:"model"`
	Messages    []ChatMessage `json:"messages"`
	Tools       []Tool        `json:"tools,omitempty"`
	MaxTokens   int           `json:"max_tokens,omitempty"`
	Temperature float32       `json:"temperature,omitempty"`
}

// ChatMessage matches OpenAI API message format.
type ChatMessage struct {
	Role       string     `json:"role"`
	Content    string     `json:"content,omitempty"`
	ToolCalls  []ToolCall `json:"tool_calls,omitempty"`
	ToolCallID string     `json:"tool_call_id,omitempty"`
}

// Tool matches OpenAI API tool format.
type Tool struct {
	Type     string      `json:"type"`
	Function FunctionDef `json:"function"`
}

// FunctionDef matches OpenAI API function definition.
type FunctionDef struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Parameters  any    `json:"parameters"`
}

// ToolCall matches OpenAI API tool call format.
type ToolCall struct {
	ID       string       `json:"id"`
	Type     string       `json:"type"`
	Function FunctionCall `json:"function"`
}

// FunctionCall matches OpenAI API function call format.
type FunctionCall struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

// ChatCompletionResponse matches OpenAI API response format.
type ChatCompletionResponse struct {
	ID      string   `json:"id"`
	Object  string   `json:"object"`
	Created int64    `json:"created"`
	Model   string   `json:"model"`
	Choices []Choice `json:"choices"`
	Usage   Usage    `json:"usage"`
}

// Choice matches OpenAI API choice format.
type Choice struct {
	Index        int         `json:"index"`
	Message      ChatMessage `json:"message"`
	FinishReason string      `json:"finish_reason"`
}

// Usage matches OpenAI API usage format.
type Usage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
}

// OpenAIServer is a mock OpenAI-compatible server for testing.
type OpenAIServer struct {
	srv      *http.Server
	listener net.Listener
	addr     string
	mu       sync.RWMutex

	// Configurable behavior
	toolArgs          map[string]string // tool name -> default arguments JSON
	completionContent string            // content to return on completion
	requestDelay      time.Duration     // artificial delay per request

	// Response sequencing for multi-turn scenarios
	responseSequence []string // sequence of completion contents
	sequenceIndex    int      // current position in sequence

	// Tracking for assertions
	requestCount int
	lastRequest  *ChatCompletionRequest
}

// NewOpenAIServer creates a new mock OpenAI server.
func NewOpenAIServer() *OpenAIServer {
	return &OpenAIServer{
		toolArgs: map[string]string{
			"query_entity": `{"entity_id": "c360.logistics.environmental.sensor.temperature.temp-sensor-001"}`,
		},
		// Return JSON for workflow condition evaluation
		completionContent: `{"valid": true, "summary": "Analysis complete. Temperature sensor reading exceeds threshold. Recommend monitoring."}`,
	}
}

// WithToolArgs configures the arguments returned for a specific tool.
func (s *OpenAIServer) WithToolArgs(toolName, argsJSON string) *OpenAIServer {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.toolArgs[toolName] = argsJSON
	return s
}

// WithCompletionContent configures the content returned on completion.
func (s *OpenAIServer) WithCompletionContent(content string) *OpenAIServer {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.completionContent = content
	return s
}

// WithRequestDelay configures an artificial delay per request.
func (s *OpenAIServer) WithRequestDelay(d time.Duration) *OpenAIServer {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.requestDelay = d
	return s
}

// WithResponseSequence configures a sequence of completion contents.
// Each call to the chat completion endpoint will return the next response
// in the sequence. After the sequence is exhausted, it returns the last response.
func (s *OpenAIServer) WithResponseSequence(responses []string) *OpenAIServer {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.responseSequence = responses
	s.sequenceIndex = 0
	return s
}

// ResetSequence resets the response sequence to the beginning.
func (s *OpenAIServer) ResetSequence() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.sequenceIndex = 0
}

// SequenceIndex returns the current position in the response sequence.
func (s *OpenAIServer) SequenceIndex() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.sequenceIndex
}

// Start starts the mock server on the given address.
// If addr is empty or ":0", a random available port is used.
func (s *OpenAIServer) Start(addr string) error {
	if addr == "" {
		addr = ":0"
	}

	listener, err := net.Listen("tcp", addr)
	if err != nil {
		return fmt.Errorf("failed to start listener: %w", err)
	}

	s.listener = listener
	s.addr = listener.Addr().String()

	mux := http.NewServeMux()
	mux.HandleFunc("/v1/chat/completions", s.handleChatCompletion)
	mux.HandleFunc("/health", s.handleHealth)

	s.srv = &http.Server{
		Handler:      mux,
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 30 * time.Second,
	}

	go func() {
		// Serve returns when the server is stopped; ErrServerClosed is expected during graceful shutdown
		_ = s.srv.Serve(listener)
	}()

	return nil
}

// Stop stops the mock server gracefully.
func (s *OpenAIServer) Stop() error {
	if s.srv == nil {
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	return s.srv.Shutdown(ctx)
}

// Addr returns the address the server is listening on.
func (s *OpenAIServer) Addr() string {
	return s.addr
}

// URL returns the base URL for the server.
func (s *OpenAIServer) URL() string {
	return "http://" + s.addr
}

// RequestCount returns the number of requests received.
func (s *OpenAIServer) RequestCount() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.requestCount
}

// LastRequest returns the last request received (for assertions).
func (s *OpenAIServer) LastRequest() *ChatCompletionRequest {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.lastRequest
}

func (s *OpenAIServer) handleHealth(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

func (s *OpenAIServer) handleChatCompletion(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req ChatCompletionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body: "+err.Error(), http.StatusBadRequest)
		return
	}

	// Track request
	s.mu.Lock()
	s.requestCount++
	s.lastRequest = &req
	delay := s.requestDelay
	s.mu.Unlock()

	// Apply artificial delay if configured
	if delay > 0 {
		time.Sleep(delay)
	}

	// Determine response based on conversation state
	var resp ChatCompletionResponse

	if s.hasToolResults(req.Messages) {
		// After tool results: return completion
		resp = s.buildCompletionResponse(req.Model)
	} else if len(req.Tools) > 0 {
		// First turn with tools: call the first tool
		resp = s.buildToolCallResponse(req.Tools[0], req.Model)
	} else {
		// No tools: simple completion
		resp = s.buildCompletionResponse(req.Model)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

func (s *OpenAIServer) hasToolResults(messages []ChatMessage) bool {
	for _, msg := range messages {
		if msg.Role == "tool" {
			return true
		}
	}
	return false
}

func (s *OpenAIServer) buildToolCallResponse(tool Tool, model string) ChatCompletionResponse {
	s.mu.RLock()
	args, ok := s.toolArgs[tool.Function.Name]
	if !ok {
		args = "{}"
	}
	s.mu.RUnlock()

	callID := "call_" + uuid.New().String()[:8]

	return ChatCompletionResponse{
		ID:      "chatcmpl-mock-" + uuid.New().String()[:8],
		Object:  "chat.completion",
		Created: time.Now().Unix(),
		Model:   model,
		Choices: []Choice{{
			Index: 0,
			Message: ChatMessage{
				Role: "assistant",
				ToolCalls: []ToolCall{{
					ID:   callID,
					Type: "function",
					Function: FunctionCall{
						Name:      tool.Function.Name,
						Arguments: args,
					},
				}},
			},
			FinishReason: "tool_calls",
		}},
		Usage: Usage{
			PromptTokens:     100,
			CompletionTokens: 50,
			TotalTokens:      150,
		},
	}
}

func (s *OpenAIServer) buildCompletionResponse(model string) ChatCompletionResponse {
	s.mu.Lock()
	content := s.completionContent
	// Use response sequence if configured
	if len(s.responseSequence) > 0 {
		if s.sequenceIndex < len(s.responseSequence) {
			content = s.responseSequence[s.sequenceIndex]
			s.sequenceIndex++
		} else {
			// After sequence exhausted, return last response
			content = s.responseSequence[len(s.responseSequence)-1]
		}
	}
	s.mu.Unlock()

	return ChatCompletionResponse{
		ID:      "chatcmpl-mock-" + uuid.New().String()[:8],
		Object:  "chat.completion",
		Created: time.Now().Unix(),
		Model:   model,
		Choices: []Choice{{
			Index: 0,
			Message: ChatMessage{
				Role:    "assistant",
				Content: content,
			},
			FinishReason: "stop",
		}},
		Usage: Usage{
			PromptTokens:     150,
			CompletionTokens: 75,
			TotalTokens:      225,
		},
	}
}
