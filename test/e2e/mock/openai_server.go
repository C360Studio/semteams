// Package mock provides test doubles for external services.
package mock

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net"
	"net/http"
	"strings"
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
	Stream      bool          `json:"stream,omitempty"`
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

// streamChunkResponse matches OpenAI streaming chunk format.
type streamChunkResponse struct {
	ID      string              `json:"id"`
	Object  string              `json:"object"`
	Created int64               `json:"created"`
	Model   string              `json:"model"`
	Choices []streamChunkChoice `json:"choices"`
	Usage   *Usage              `json:"usage,omitempty"`
}

// streamChunkChoice matches OpenAI streaming choice format.
type streamChunkChoice struct {
	Index        int              `json:"index"`
	Delta        streamChunkDelta `json:"delta"`
	FinishReason *string          `json:"finish_reason"`
}

// streamChunkDelta matches OpenAI streaming delta format.
type streamChunkDelta struct {
	Role      string           `json:"role,omitempty"`
	Content   string           `json:"content,omitempty"`
	ToolCalls []streamToolCall `json:"tool_calls,omitempty"`
}

// streamToolCall matches OpenAI streaming tool call delta format.
type streamToolCall struct {
	Index    int          `json:"index"`
	ID       string       `json:"id,omitempty"`
	Type     string       `json:"type,omitempty"`
	Function FunctionCall `json:"function"`
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

	// Fixture-driven responses (takes precedence over the default
	// tool-call-vs-completion heuristic when set). See fixture.go.
	fixture      *Fixture
	fixtureIndex int

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

// WithFixture configures the server to return responses from the given
// fixture. Each chat completion call returns the next response in the
// fixture's sequence; after exhaustion, the last response is repeated.
// Fixture responses take precedence over the default
// tool-call-vs-completion heuristic based on request state.
func (s *OpenAIServer) WithFixture(f *Fixture) *OpenAIServer {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.fixture = f
	s.fixtureIndex = 0
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
	reqNum := s.requestCount
	fixIdx := s.fixtureIndex
	s.lastRequest = &req
	delay := s.requestDelay
	s.mu.Unlock()

	// Diagnostic trace — one line per LLM call so fixture consumption is
	// visible to journey-runners. Includes tool count + whether the
	// request carries prior tool_result messages (useful for distinguishing
	// first-call vs post-tool iterations). Also records per-message role
	// and size breakdown (+ a preview of every system message) so journey
	// tests can verify the caller actually fed expected persona fragments
	// into the prompt — the anti-gaming check that proves persona wiring
	// works, not just that loading succeeded.
	toolCount := len(req.Tools)
	hasToolResults := false
	var msgBreakdown []string
	var sysPreviews []string
	for i, m := range req.Messages {
		if m.Role == "tool" {
			hasToolResults = true
		}
		msgBreakdown = append(msgBreakdown, fmt.Sprintf("%s(%d)", m.Role, len(m.Content)))
		if m.Role == "system" {
			preview := m.Content
			if len(preview) > 500 {
				preview = preview[:500] + "…"
			}
			preview = strings.ReplaceAll(preview, "\n", " ⏎ ")
			sysPreviews = append(sysPreviews, fmt.Sprintf("sys[%d]=%q", i, preview))
		}
	}
	log.Printf("[mock-llm] req#%d (fixture idx=%d) model=%s tools=%d has_tool_results=%v stream=%v msgs=[%s]",
		reqNum, fixIdx, req.Model, toolCount, hasToolResults, req.Stream, strings.Join(msgBreakdown, " "))
	for _, p := range sysPreviews {
		log.Printf("[mock-llm]   req#%d %s", reqNum, p)
	}

	// Apply artificial delay if configured
	if delay > 0 {
		time.Sleep(delay)
	}

	// Determine response. When a fixture is loaded it drives the sequence
	// deterministically; otherwise fall back to the default heuristic
	// (tool call when request has tools and no tool results yet, otherwise
	// a completion).
	var resp ChatCompletionResponse

	s.mu.RLock()
	hasFixture := s.fixture != nil
	s.mu.RUnlock()

	switch {
	case hasFixture:
		resp = s.buildFixtureResponse(req.Model)
	case s.hasToolResults(req.Messages):
		resp = s.buildCompletionResponse(req.Model)
	case len(req.Tools) > 0:
		resp = s.buildToolCallResponse(req.Tools[0], req.Model)
	default:
		resp = s.buildCompletionResponse(req.Model)
	}

	if req.Stream {
		s.writeStreamingResponse(w, resp)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

// writeStreamingResponse converts a ChatCompletionResponse into SSE chunks.
func (s *OpenAIServer) writeStreamingResponse(w http.ResponseWriter, resp ChatCompletionResponse) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming not supported", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	id := resp.ID
	created := resp.Created
	model := resp.Model

	if len(resp.Choices) == 0 {
		return
	}

	choice := resp.Choices[0]

	if len(choice.Message.ToolCalls) > 0 {
		s.writeStreamingToolCalls(w, flusher, id, created, model, choice)
	} else {
		s.writeStreamingContent(w, flusher, id, created, model, choice)
	}

	// Final chunk: usage with empty choices
	usageChunk := streamChunkResponse{
		ID:      id,
		Object:  "chat.completion.chunk",
		Created: created,
		Model:   model,
		Choices: []streamChunkChoice{},
		Usage:   &resp.Usage,
	}
	s.writeSSEChunk(w, flusher, usageChunk)

	// Done sentinel
	fmt.Fprintf(w, "data: [DONE]\n\n")
	flusher.Flush()
}

// writeStreamingContent splits text content into two chunks.
func (s *OpenAIServer) writeStreamingContent(w http.ResponseWriter, flusher http.Flusher, id string, created int64, model string, choice Choice) {
	content := choice.Message.Content
	mid := len(content) / 2

	finishReason := choice.FinishReason

	// Chunk 1: role + first half of content
	s.writeSSEChunk(w, flusher, streamChunkResponse{
		ID: id, Object: "chat.completion.chunk", Created: created, Model: model,
		Choices: []streamChunkChoice{{
			Index: 0,
			Delta: streamChunkDelta{Role: "assistant", Content: content[:mid]},
		}},
	})

	// Chunk 2: second half + finish_reason
	s.writeSSEChunk(w, flusher, streamChunkResponse{
		ID: id, Object: "chat.completion.chunk", Created: created, Model: model,
		Choices: []streamChunkChoice{{
			Index:        0,
			Delta:        streamChunkDelta{Content: content[mid:]},
			FinishReason: &finishReason,
		}},
	})
}

// writeStreamingToolCalls splits tool calls into delta chunks.
func (s *OpenAIServer) writeStreamingToolCalls(w http.ResponseWriter, flusher http.Flusher, id string, created int64, model string, choice Choice) {
	finishReason := choice.FinishReason

	for i, tc := range choice.Message.ToolCalls {
		args := tc.Function.Arguments
		argMid := len(args) / 2

		// Chunk: tool call start (id, name, first half of args)
		s.writeSSEChunk(w, flusher, streamChunkResponse{
			ID: id, Object: "chat.completion.chunk", Created: created, Model: model,
			Choices: []streamChunkChoice{{
				Index: 0,
				Delta: streamChunkDelta{
					Role: "assistant",
					ToolCalls: []streamToolCall{{
						Index:    i,
						ID:       tc.ID,
						Type:     "function",
						Function: FunctionCall{Name: tc.Function.Name, Arguments: args[:argMid]},
					}},
				},
			}},
		})

		// Chunk: remaining args
		s.writeSSEChunk(w, flusher, streamChunkResponse{
			ID: id, Object: "chat.completion.chunk", Created: created, Model: model,
			Choices: []streamChunkChoice{{
				Index: 0,
				Delta: streamChunkDelta{
					ToolCalls: []streamToolCall{{
						Index:    i,
						Function: FunctionCall{Arguments: args[argMid:]},
					}},
				},
			}},
		})
	}

	// Finish reason chunk
	s.writeSSEChunk(w, flusher, streamChunkResponse{
		ID: id, Object: "chat.completion.chunk", Created: created, Model: model,
		Choices: []streamChunkChoice{{
			Index:        0,
			Delta:        streamChunkDelta{},
			FinishReason: &finishReason,
		}},
	})
}

func (s *OpenAIServer) writeSSEChunk(w http.ResponseWriter, flusher http.Flusher, chunk streamChunkResponse) {
	data, _ := json.Marshal(chunk)
	fmt.Fprintf(w, "data: %s\n\n", data)
	flusher.Flush()
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

// buildFixtureResponse returns the next response from the configured
// fixture. Advances the fixture index on each call and repeats the last
// entry once exhausted. Caller must have confirmed s.fixture != nil.
func (s *OpenAIServer) buildFixtureResponse(model string) ChatCompletionResponse {
	s.mu.Lock()
	idx := s.fixtureIndex
	if idx >= len(s.fixture.Responses) {
		// Sequence exhausted — repeat the last entry.
		idx = len(s.fixture.Responses) - 1
	} else {
		s.fixtureIndex++
	}
	entry := s.fixture.Responses[idx]
	s.mu.Unlock()

	base := ChatCompletionResponse{
		ID:      "chatcmpl-mock-" + uuid.New().String()[:8],
		Object:  "chat.completion",
		Created: time.Now().Unix(),
		Model:   model,
		Usage: Usage{
			PromptTokens:     100,
			CompletionTokens: 50,
			TotalTokens:      150,
		},
	}

	if entry.ToolCall != nil {
		callID := "call_" + uuid.New().String()[:8]
		base.Choices = []Choice{{
			Index: 0,
			Message: ChatMessage{
				Role: "assistant",
				ToolCalls: []ToolCall{{
					ID:   callID,
					Type: "function",
					Function: FunctionCall{
						Name:      entry.ToolCall.Name,
						Arguments: entry.ToolCall.ArgumentsJSON,
					},
				}},
			},
			FinishReason: "tool_calls",
		}}
		return base
	}

	// entry.Completion is guaranteed non-nil by Fixture.Validate()
	base.Choices = []Choice{{
		Index: 0,
		Message: ChatMessage{
			Role:    "assistant",
			Content: entry.Completion.Content,
		},
		FinishReason: "stop",
	}}
	return base
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
