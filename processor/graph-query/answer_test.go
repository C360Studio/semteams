package graphquery

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/c360studio/semstreams/graph/llm"
)

func TestTemplateAnswerSynthesizer_Synthesize(t *testing.T) {
	s := &TemplateAnswerSynthesizer{}

	t.Run("empty summaries", func(t *testing.T) {
		answer, model, err := s.Synthesize(context.Background(), "test query", nil, 0)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if answer != "" {
			t.Errorf("expected empty answer, got %q", answer)
		}
		if model != "" {
			t.Errorf("expected empty model, got %q", model)
		}
	})

	t.Run("produces template answer", func(t *testing.T) {
		summaries := []CommunitySummary{
			{Summary: "Game quest entities.", MemberCount: 10, Relevance: 0.9},
		}
		answer, model, err := s.Synthesize(context.Background(), "show quests", summaries, 10)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !strings.Contains(answer, "10 entities") {
			t.Errorf("answer missing entity count: %s", answer)
		}
		if model != "" {
			t.Errorf("template synthesizer should return empty model, got %q", model)
		}
	})
}

func TestLLMAnswerSynthesizer_Synthesize(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		client := &mockLLMClient{
			response: &llm.ChatResponse{Content: "The quests involve dragon slaying and merchant routes."},
		}
		s := NewLLMAnswerSynthesizer(client, "gemini-flash", nil)

		summaries := []CommunitySummary{
			{Summary: "Quest entities on board1.", MemberCount: 10, Relevance: 0.9},
		}
		answer, model, err := s.Synthesize(context.Background(), "what quests exist?", summaries, 10)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if answer != "The quests involve dragon slaying and merchant routes." {
			t.Errorf("unexpected answer: %q", answer)
		}
		if model != "gemini-flash" {
			t.Errorf("model = %q, want gemini-flash", model)
		}

		// Verify the prompt was constructed correctly
		if !strings.Contains(client.lastRequest.UserPrompt, "what quests exist?") {
			t.Error("user prompt should contain the query")
		}
		if !strings.Contains(client.lastRequest.UserPrompt, "Quest entities on board1.") {
			t.Error("user prompt should contain community summary")
		}
		if client.lastRequest.SystemPrompt == "" {
			t.Error("system prompt should be set")
		}
	})

	t.Run("falls back on error", func(t *testing.T) {
		client := &mockLLMClient{
			err: fmt.Errorf("connection refused"),
		}
		s := NewLLMAnswerSynthesizer(client, "gemini-flash", nil)

		summaries := []CommunitySummary{
			{Summary: "Quest entities.", MemberCount: 5, Relevance: 0.8},
		}
		answer, model, err := s.Synthesize(context.Background(), "test", summaries, 5)
		// Error is logged internally, not returned — fallback is transparent
		if err != nil {
			t.Fatalf("expected nil error on fallback, got %v", err)
		}
		// Should fall back to template answer
		if !strings.Contains(answer, "5 entities") {
			t.Errorf("fallback answer missing entity count: %s", answer)
		}
		// Model should be empty on fallback
		if model != "" {
			t.Errorf("model should be empty on fallback, got %q", model)
		}
	})
}

func TestBuildAnswerPrompt(t *testing.T) {
	summaries := []CommunitySummary{
		{
			Summary:     "Game quest entities on board1.",
			MemberCount: 12,
			Relevance:   0.85,
			Keywords:    []string{"quest", "reward", "dragon"},
			Entities: []EntityDigest{
				{ID: "q1", Type: "quest", Label: "Dragon Slayer"},
			},
		},
		{
			Summary:     "Player agents.",
			MemberCount: 8,
			Relevance:   0.6,
		},
	}

	prompt := buildAnswerPrompt("show me all quests", summaries, 20)

	if !strings.Contains(prompt, "show me all quests") {
		t.Error("prompt should contain the query")
	}
	if !strings.Contains(prompt, "20 matching entities") {
		t.Error("prompt should contain total entity count")
	}
	if !strings.Contains(prompt, "2 clusters") {
		t.Error("prompt should contain cluster count")
	}
	if !strings.Contains(prompt, "12 entities") {
		t.Error("prompt should contain member count")
	}
	if !strings.Contains(prompt, "Dragon Slayer [quest]") {
		t.Error("prompt should contain representative entity")
	}
	if !strings.Contains(prompt, "quest, reward, dragon") {
		t.Error("prompt should contain keywords")
	}
	if !strings.Contains(prompt, "Player agents.") {
		t.Error("prompt should contain second cluster summary")
	}
}

func TestBuildAnswerPrompt_LimitsTo5Clusters(t *testing.T) {
	summaries := make([]CommunitySummary, 8)
	for i := range summaries {
		summaries[i] = CommunitySummary{
			Summary:     fmt.Sprintf("Cluster %c.", 'A'+i),
			MemberCount: 5,
			Relevance:   0.5,
		}
	}
	prompt := buildAnswerPrompt("test", summaries, 40)

	if !strings.Contains(prompt, "Cluster E.") {
		t.Error("should include 5th cluster")
	}
	if strings.Contains(prompt, "Cluster F.") {
		t.Error("should not include 6th cluster")
	}
}

// mockLLMClient implements llm.Client for testing.
type mockLLMClient struct {
	response    *llm.ChatResponse
	err         error
	lastRequest llm.ChatRequest
}

func (m *mockLLMClient) ChatCompletion(_ context.Context, req llm.ChatRequest) (*llm.ChatResponse, error) {
	m.lastRequest = req
	return m.response, m.err
}

func (m *mockLLMClient) Model() string { return "mock" }
func (m *mockLLMClient) Close() error  { return nil }
