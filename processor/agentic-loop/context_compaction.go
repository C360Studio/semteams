package agenticloop

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/c360studio/semstreams/agentic"
)

// CompactionResult contains the results of a compaction operation.
type CompactionResult struct {
	Summary       string
	EvictedTokens int
	NewTokens     int
	Model         string // model used for summarization (empty if stub fallback)
}

// Compactor handles context compaction operations.
type Compactor struct {
	config     ContextConfig
	summarizer Summarizer
	modelName  string // resolved model name for CompactionResult.Model
	logger     *slog.Logger
}

// CompactorOption is a functional option for configuring a Compactor.
type CompactorOption func(*Compactor)

// WithSummarizer injects an LLM-backed summarizer into the Compactor.
// When set, Compact() calls the summarizer instead of the stub fallback.
func WithSummarizer(s Summarizer) CompactorOption {
	return func(c *Compactor) { c.summarizer = s }
}

// WithModelName sets the resolved model name reported in CompactionResult.
func WithModelName(name string) CompactorOption {
	return func(c *Compactor) { c.modelName = name }
}

// WithCompactorLogger sets the logger used by the Compactor.
func WithCompactorLogger(l *slog.Logger) CompactorOption {
	return func(c *Compactor) { c.logger = l }
}

// NewCompactor creates a new compactor. Variadic opts allow optional injection
// of a Summarizer and logger without breaking existing callers.
func NewCompactor(config ContextConfig, opts ...CompactorOption) *Compactor {
	c := &Compactor{
		config: config,
		logger: slog.Default(),
	}
	for _, opt := range opts {
		opt(c)
	}
	return c
}

// ShouldCompact delegates to the context manager's ShouldCompact.
func (c *Compactor) ShouldCompact(cm *ContextManager) bool {
	return cm.ShouldCompact()
}

// Compact performs compaction on the context manager.
// When a Summarizer is injected, it calls the LLM to generate a real summary.
// If the summarizer returns an error, it falls back to a stub summary and logs a warning.
func (c *Compactor) Compact(ctx context.Context, cm *ContextManager) (CompactionResult, error) {
	// Check context cancellation
	select {
	case <-ctx.Done():
		return CompactionResult{}, ctx.Err()
	default:
	}

	cm.mu.Lock()
	defer cm.mu.Unlock()

	// Get recent history to compact
	recentHistory := cm.regions[RegionRecentHistory]
	if len(recentHistory) == 0 {
		return CompactionResult{}, nil
	}

	// Calculate evicted tokens
	evictedTokens := 0
	for _, m := range recentHistory {
		evictedTokens += m.Tokens
	}

	// Extract ChatMessages for summarization
	chatMessages := make([]agentic.ChatMessage, len(recentHistory))
	for i, m := range recentHistory {
		chatMessages[i] = m.Message
	}

	// Calculate summary token budget: clamp(evictedTokens/4, 256, 2048)
	budget := min(max(evictedTokens/4, 256), 2048)

	// Generate summary — prefer LLM, fall back to stub
	compactedCount := len(cm.regions[RegionCompactedHistory])
	var summary string
	var modelUsed string

	if c.summarizer != nil {
		var err error
		summary, err = c.summarizer.Summarize(ctx, chatMessages, budget)
		if err != nil {
			c.logger.Warn("summarizer failed, falling back to stub summary",
				"error", err,
				"msg_count", len(chatMessages))
			summary = stubSummary(compactedCount, len(recentHistory))
		} else {
			modelUsed = c.modelName
		}
	} else {
		summary = stubSummary(compactedCount, len(recentHistory))
	}

	newTokens := estimateTokens(summary)

	// Update regions
	cm.regions[RegionCompactedHistory] = append(cm.regions[RegionCompactedHistory], contextMessage{
		Message: agentic.ChatMessage{
			Role:    "system",
			Content: summary,
		},
		Tokens:    newTokens,
		Iteration: cm.currentIteration,
	})
	cm.regions[RegionRecentHistory] = []contextMessage{}

	return CompactionResult{
		Summary:       summary,
		EvictedTokens: evictedTokens,
		NewTokens:     newTokens,
		Model:         modelUsed,
	}, nil
}

// stubSummary generates a placeholder summary string when no LLM summarizer is available.
// It includes the compaction count to ensure consecutive compactions produce distinct summaries.
func stubSummary(compactedCount, msgCount int) string {
	return fmt.Sprintf("Summary #%d (%d msgs)", compactedCount+1, msgCount)
}
