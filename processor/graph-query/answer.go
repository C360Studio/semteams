package graphquery

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	"github.com/c360studio/semstreams/graph/llm"
)

// AnswerSynthesizer produces a natural language answer from community summaries
// in response to a globalSearch query.
type AnswerSynthesizer interface {
	// Synthesize produces an answer to the query based on community summaries.
	// Returns the answer text and the model name used (empty for template fallback).
	Synthesize(ctx context.Context, query string, summaries []CommunitySummary, totalEntities int) (answer string, model string, err error)

	// Close releases any resources held by the synthesizer.
	Close() error
}

// answerSynthesisSystemPrompt is the system prompt for LLM-backed answer synthesis.
const answerSynthesisSystemPrompt = `You are a knowledge graph query assistant. Given a user query and summaries of related knowledge clusters, synthesize a concise answer that directly addresses the query.

Each cluster summary describes a group of related entities in the knowledge graph. Use the cluster summaries, representative entities, and keywords to construct your answer. Reference specific entities by name when relevant.

Be direct and factual. If the clusters don't contain enough information to fully answer the query, say what is known and what is missing. Do not speculate beyond the provided data.`

// LLMAnswerSynthesizer uses an LLM to produce query-focused answers from
// community summaries. Falls back to template synthesis on LLM error.
type LLMAnswerSynthesizer struct {
	client    llm.Client
	modelName string
	logger    *slog.Logger
}

// NewLLMAnswerSynthesizer creates an LLM-backed answer synthesizer.
func NewLLMAnswerSynthesizer(client llm.Client, modelName string, logger *slog.Logger) *LLMAnswerSynthesizer {
	if logger == nil {
		logger = slog.Default()
	}
	return &LLMAnswerSynthesizer{client: client, modelName: modelName, logger: logger}
}

// Close releases the LLM client resources.
func (s *LLMAnswerSynthesizer) Close() error {
	if s.client != nil {
		return s.client.Close()
	}
	return nil
}

// answerSynthesisMaxTokens is the maximum tokens for the LLM answer response.
const answerSynthesisMaxTokens = 500

// answerSynthesisTemperature controls randomness in answer generation (low = factual).
var answerSynthesisTemperature = 0.3

// Synthesize produces a query-focused answer by sending community summaries to the LLM.
// On LLM failure, falls back to template synthesis and logs the error internally.
func (s *LLMAnswerSynthesizer) Synthesize(ctx context.Context, query string, summaries []CommunitySummary, totalEntities int) (string, string, error) {
	if len(summaries) == 0 {
		return "", "", nil
	}

	userPrompt := buildAnswerPrompt(query, summaries, totalEntities)

	resp, err := s.client.ChatCompletion(ctx, llm.ChatRequest{
		SystemPrompt: answerSynthesisSystemPrompt,
		UserPrompt:   userPrompt,
		MaxTokens:    answerSynthesisMaxTokens,
		Temperature:  &answerSynthesisTemperature,
	})
	if err != nil {
		s.logger.Warn("LLM answer synthesis failed, using template fallback",
			slog.String("query", query),
			slog.Any("error", err))
		return synthesizeAnswer(summaries, totalEntities), "", nil
	}

	return resp.Content, s.modelName, nil
}

// TemplateAnswerSynthesizer produces answers from community summaries using
// string templates. No LLM call required — used as fallback when no
// answer_synthesis endpoint is configured.
type TemplateAnswerSynthesizer struct{}

// Synthesize produces a template-based answer.
func (s *TemplateAnswerSynthesizer) Synthesize(_ context.Context, _ string, summaries []CommunitySummary, totalEntities int) (string, string, error) {
	return synthesizeAnswer(summaries, totalEntities), "", nil
}

// Close is a no-op for the template synthesizer.
func (s *TemplateAnswerSynthesizer) Close() error { return nil }

// buildAnswerPrompt constructs the user prompt for LLM answer synthesis.
func buildAnswerPrompt(query string, summaries []CommunitySummary, totalEntities int) string {
	var b strings.Builder
	b.WriteString(fmt.Sprintf("Query: %s\n\n", query))
	b.WriteString(fmt.Sprintf("The knowledge graph contains %d matching entities across %d clusters.\n\n", totalEntities, len(summaries)))

	limit := len(summaries)
	if limit > MaxAnswerClusters {
		limit = MaxAnswerClusters
	}

	for i, s := range summaries[:limit] {
		b.WriteString(fmt.Sprintf("Cluster %d", i+1))
		if s.MemberCount > 0 {
			b.WriteString(fmt.Sprintf(" (%d entities, %.0f%% match)", s.MemberCount, s.Relevance*100))
		}
		b.WriteString(":\n")

		if s.Summary != "" {
			b.WriteString(s.Summary)
			b.WriteByte('\n')
		}

		if len(s.Entities) > 0 {
			names := make([]string, len(s.Entities))
			for j, e := range s.Entities {
				names[j] = fmt.Sprintf("%s [%s]", e.Label, e.Type)
			}
			b.WriteString(fmt.Sprintf("Representatives: %s\n", strings.Join(names, ", ")))
		}

		if len(s.Keywords) > 0 {
			kwLimit := len(s.Keywords)
			if kwLimit > 5 {
				kwLimit = 5
			}
			b.WriteString(fmt.Sprintf("Keywords: %s\n", strings.Join(s.Keywords[:kwLimit], ", ")))
		}

		b.WriteByte('\n')
	}

	b.WriteString("Synthesize a concise answer to the query based on these clusters.")
	return b.String()
}
