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

// mockSummarizer is a test double for teamsloop.Summarizer.
type mockSummarizer struct {
	// summary is returned on the next Summarize call.
	summary string
	// err causes Summarize to return an error when non-nil.
	err error
	// calls records how many times Summarize was invoked.
	calls int
	// lastMessages captures the messages passed to the most recent call.
	lastMessages []agentic.ChatMessage
	// lastMaxTokens captures the budget from the most recent call.
	lastMaxTokens int
}

func (m *mockSummarizer) Summarize(_ context.Context, messages []agentic.ChatMessage, maxTokens int) (string, error) {
	m.calls++
	m.lastMessages = messages
	m.lastMaxTokens = maxTokens
	if m.err != nil {
		return "", m.err
	}
	return m.summary, nil
}

// fillContextToTokens is defined in helpers_test.go (or similar); declared here
// to avoid re-declaration — it's already defined in context_manager_test.go or
// context_compaction_test.go in the same package. We use the one already defined
// and do not redeclare it here.

func TestCompactor_Compact_WithLLMSummarizer(t *testing.T) {
	config := teamsloop.DefaultContextConfig()
	mock := &mockSummarizer{summary: "LLM-generated summary of the conversation"}

	compactor := teamsloop.NewCompactor(config, teamsloop.WithSummarizer(mock))
	cm := teamsloop.NewContextManager("loop-llm", "gpt-4o", config)

	messages := []agentic.ChatMessage{
		{Role: "user", Content: "What is the weather like?"},
		{Role: "assistant", Content: "It is sunny and 22°C."},
		{Role: "user", Content: "Should I bring an umbrella?"},
		{Role: "assistant", Content: "No umbrella needed today."},
	}
	for _, msg := range messages {
		_ = cm.AddMessage(teamsloop.RegionRecentHistory, msg)
	}

	result, err := compactor.Compact(context.Background(), cm)

	if err != nil {
		t.Fatalf("Compact() error = %v", err)
	}
	if mock.calls != 1 {
		t.Errorf("Summarize() called %d times, want 1", mock.calls)
	}
	if result.Summary != mock.summary {
		t.Errorf("Summary = %q, want %q", result.Summary, mock.summary)
	}
	// Summarizer should have received all 4 messages
	if len(mock.lastMessages) != 4 {
		t.Errorf("Summarize received %d messages, want 4", len(mock.lastMessages))
	}
}

func TestCompactor_Compact_SummarizerError_FallsBack(t *testing.T) {
	config := teamsloop.DefaultContextConfig()
	mock := &mockSummarizer{err: errors.New("LLM service timeout")}

	compactor := teamsloop.NewCompactor(config, teamsloop.WithSummarizer(mock))
	cm := teamsloop.NewContextManager("loop-fallback", "gpt-4o", config)

	_ = cm.AddMessage(teamsloop.RegionRecentHistory, agentic.ChatMessage{
		Role: "user", Content: "A question that will fail summarization",
	})

	result, err := compactor.Compact(context.Background(), cm)

	// Compact itself should not fail — it falls back to stub
	if err != nil {
		t.Fatalf("Compact() should not error on summarizer failure, got: %v", err)
	}
	if result.Summary == "" {
		t.Error("Compact() fallback summary is empty, want stub summary")
	}
	// Stub summary format: "Summary #N (M msgs)"
	if !strings.Contains(result.Summary, "Summary") {
		t.Errorf("Fallback summary = %q, want stub format containing 'Summary'", result.Summary)
	}
	// Model field should be empty when fallback was used
	if result.Model != "" {
		t.Errorf("Model = %q, want empty string on fallback", result.Model)
	}
}

func TestCompactor_Compact_SummaryBudgetClamping(t *testing.T) {
	tests := []struct {
		name          string
		contentLen    int // approximate chars to produce evictedTokens
		wantMinBudget int
		wantMaxBudget int
	}{
		{
			name:          "very small history clamps to min 256",
			contentLen:    4,   // ~1 token → evictedTokens/4 < 256
			wantMinBudget: 256, // budget clamped to 256
			wantMaxBudget: 256,
		},
		{
			name:          "large history clamps to max 2048",
			contentLen:    8192 * 4, // ~8192 tokens → evictedTokens/4 = 2048
			wantMinBudget: 2048,
			wantMaxBudget: 2048,
		},
		{
			name:          "mid-range history within bounds",
			contentLen:    4000, // ~1000 tokens → evictedTokens/4 = 250, clamps to 256
			wantMinBudget: 256,
			wantMaxBudget: 2048,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config := teamsloop.DefaultContextConfig()
			mock := &mockSummarizer{summary: "clamping test summary"}

			compactor := teamsloop.NewCompactor(config, teamsloop.WithSummarizer(mock))
			cm := teamsloop.NewContextManager("loop-clamp", "gpt-4o", config)

			content := strings.Repeat("x", tt.contentLen)
			_ = cm.AddMessage(teamsloop.RegionRecentHistory, agentic.ChatMessage{
				Role: "user", Content: content,
			})

			_, err := compactor.Compact(context.Background(), cm)
			if err != nil {
				t.Fatalf("Compact() error = %v", err)
			}

			if mock.lastMaxTokens < tt.wantMinBudget || mock.lastMaxTokens > tt.wantMaxBudget {
				t.Errorf("budget = %d, want [%d, %d]",
					mock.lastMaxTokens, tt.wantMinBudget, tt.wantMaxBudget)
			}
		})
	}
}

func TestCompactor_Compact_NilSummarizer_UsesStub(t *testing.T) {
	config := teamsloop.DefaultContextConfig()
	// No WithSummarizer option — backward-compatible nil summarizer path
	compactor := teamsloop.NewCompactor(config)
	cm := teamsloop.NewContextManager("loop-stub", "gpt-4o", config)

	_ = cm.AddMessage(teamsloop.RegionRecentHistory, agentic.ChatMessage{
		Role: "user", Content: "A message for stub compaction",
	})

	result, err := compactor.Compact(context.Background(), cm)

	if err != nil {
		t.Fatalf("Compact() error = %v", err)
	}
	if result.Summary == "" {
		t.Error("Compact() stub summary is empty")
	}
	// Stub format check
	if !strings.Contains(result.Summary, "Summary") {
		t.Errorf("Stub summary = %q, expected format 'Summary #N (M msgs)'", result.Summary)
	}
	// Model should be empty when stub is used
	if result.Model != "" {
		t.Errorf("Model = %q, want empty for stub path", result.Model)
	}
}

func TestCompactor_Compact_ModelFieldSet_WithLLMSummarizer(t *testing.T) {
	config := teamsloop.DefaultContextConfig()

	mock := &mockSummarizer{summary: "real LLM summary"}
	compactor := teamsloop.NewCompactor(config, teamsloop.WithSummarizer(mock), teamsloop.WithModelName("fast"))
	cm := teamsloop.NewContextManager("loop-model-field", "gpt-4o", config)

	_ = cm.AddMessage(teamsloop.RegionRecentHistory, agentic.ChatMessage{
		Role: "user", Content: "Message for model field test",
	})

	result, err := compactor.Compact(context.Background(), cm)

	if err != nil {
		t.Fatalf("Compact() error = %v", err)
	}
	if result.Model != "fast" {
		t.Errorf("Model = %q, want 'fast'", result.Model)
	}
}

// TestNewLLMSummarizer_NilLogger verifies that a nil logger is replaced with slog.Default().
func TestNewLLMSummarizer_NilLogger(t *testing.T) {
	client := &mockLLMClient{
		response: &llm.ChatResponse{Content: "summary"},
	}
	// Should not panic
	summarizer := teamsloop.NewLLMSummarizer(client, nil)
	if summarizer == nil {
		t.Fatal("NewLLMSummarizer() returned nil")
	}
}
