package agenticmemory

import (
	"context"
	"fmt"

	"github.com/c360/semstreams/message"
)

// LLMClient defines the interface for LLM operations
type LLMClient interface {
	ExtractFacts(ctx context.Context, model string, content string, maxTokens int) ([]message.Triple, error)
}

// LLMExtractor extracts semantic facts from agent responses using an LLM
type LLMExtractor struct {
	config    ExtractionConfig
	llmClient LLMClient
}

// NewLLMExtractor creates a new LLMExtractor instance
func NewLLMExtractor(config ExtractionConfig, llmClient LLMClient) (*LLMExtractor, error) {
	return &LLMExtractor{
		config:    config,
		llmClient: llmClient,
	}, nil
}

// ExtractFacts extracts semantic triples from response content
func (e *LLMExtractor) ExtractFacts(ctx context.Context, loopID, responseContent string) ([]message.Triple, error) {
	// Validate inputs
	if loopID == "" {
		return nil, fmt.Errorf("loopID cannot be empty")
	}

	// Check context cancellation
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	// Handle empty content gracefully
	if responseContent == "" {
		return []message.Triple{}, nil
	}

	// If LLM client is available, use it for extraction
	if e.llmClient != nil {
		triples, err := e.llmClient.ExtractFacts(
			ctx,
			e.config.LLMAssisted.Model,
			responseContent,
			e.config.LLMAssisted.MaxTokens,
		)
		if err != nil {
			return nil, fmt.Errorf("llm extraction failed: %w", err)
		}
		return triples, nil
	}

	// No LLM client available, return empty triples
	return []message.Triple{}, nil
}
