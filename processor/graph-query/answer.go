package graphquery

import (
	"context"
	"fmt"
	"strings"

	"github.com/c360studio/semstreams/graph/llm"
)

// AnswerSynthesizer produces a natural language answer from community summaries
// in response to a globalSearch query.
type AnswerSynthesizer interface {
	// Synthesize produces an answer to the query based on community summaries.
	// Returns the answer text and the model name used (empty for template fallback).
	Synthesize(ctx context.Context, query string, summaries []CommunitySummary, totalEntities int) (answer string, model string, err error)
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
}

// NewLLMAnswerSynthesizer creates an LLM-backed answer synthesizer.
func NewLLMAnswerSynthesizer(client llm.Client, modelName string) *LLMAnswerSynthesizer {
	return &LLMAnswerSynthesizer{client: client, modelName: modelName}
}

// Synthesize produces a query-focused answer by sending community summaries to the LLM.
func (s *LLMAnswerSynthesizer) Synthesize(ctx context.Context, query string, summaries []CommunitySummary, totalEntities int) (string, string, error) {
	if len(summaries) == 0 {
		return "", "", nil
	}

	userPrompt := buildAnswerPrompt(query, summaries, totalEntities)

	temp := 0.3
	resp, err := s.client.ChatCompletion(ctx, llm.ChatRequest{
		SystemPrompt: answerSynthesisSystemPrompt,
		UserPrompt:   userPrompt,
		MaxTokens:    300,
		Temperature:  &temp,
	})
	if err != nil {
		// Fall back to template on LLM error
		return synthesizeAnswer(summaries, totalEntities), "", fmt.Errorf("LLM answer synthesis failed, using template fallback: %w", err)
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

// buildAnswerPrompt constructs the user prompt for LLM answer synthesis.
func buildAnswerPrompt(query string, summaries []CommunitySummary, totalEntities int) string {
	var b strings.Builder
	b.WriteString(fmt.Sprintf("Query: %s\n\n", query))
	b.WriteString(fmt.Sprintf("The knowledge graph contains %d matching entities across %d clusters.\n\n", totalEntities, len(summaries)))

	limit := len(summaries)
	if limit > 5 {
		limit = 5
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
