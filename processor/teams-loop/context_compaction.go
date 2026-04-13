package teamsloop

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

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

	// Get recent history to compact.
	// If the last message is an assistant message with pending tool_calls
	// (results haven't arrived yet), exclude it — compacting it would orphan
	// the incoming tool results and cause a 400 from the provider API.
	recentHistory := cm.regions[RegionRecentHistory]
	if len(recentHistory) == 0 {
		return CompactionResult{}, nil
	}

	var retained []contextMessage
	last := recentHistory[len(recentHistory)-1]
	if last.Message.Role == "assistant" && len(last.Message.ToolCalls) > 0 {
		retained = []contextMessage{last}
		recentHistory = recentHistory[:len(recentHistory)-1]
		if len(recentHistory) == 0 {
			return CompactionResult{}, nil // Only the pending tool_call — nothing to compact
		}
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
	cm.regions[RegionRecentHistory] = retained // nil if nothing retained, which is a valid empty slice

	// Cap compacted history to prevent unbounded growth
	if len(cm.regions[RegionCompactedHistory]) > maxCompactedMessages {
		c.recompact(ctx, cm)
		// Recalculate newTokens to reflect recompacted region
		newTokens = 0
		for _, m := range cm.regions[RegionCompactedHistory] {
			newTokens += m.Tokens
		}
	}

	return CompactionResult{
		Summary:       summary,
		EvictedTokens: evictedTokens,
		NewTokens:     newTokens,
		Model:         modelUsed,
	}, nil
}

// maxCompactedMessages is the maximum number of summary messages allowed in
// RegionCompactedHistory. When exceeded, all summaries are re-summarized into one.
const maxCompactedMessages = 3

// recompact consolidates all compacted history summaries into a single meta-summary.
// Caller must hold cm.mu.
func (c *Compactor) recompact(ctx context.Context, cm *ContextManager) {
	select {
	case <-ctx.Done():
		c.logger.Warn("skipping recompaction due to context cancellation")
		return
	default:
	}

	compacted := cm.regions[RegionCompactedHistory]
	if len(compacted) <= 1 {
		return
	}

	// Calculate tokens before recompaction
	tokensBefore := 0
	for _, m := range compacted {
		tokensBefore += m.Tokens
	}

	// Extract messages for summarization
	chatMessages := make([]agentic.ChatMessage, len(compacted))
	for i, m := range compacted {
		chatMessages[i] = m.Message
	}

	// Budget: clamp(tokensBefore/4, 256, 2048)
	budget := min(max(tokensBefore/4, 256), 2048)

	var summary string
	if c.summarizer != nil {
		var err error
		summary, err = c.summarizer.Summarize(ctx, chatMessages, budget)
		if err != nil {
			c.logger.Warn("recompaction summarizer failed, using concatenation fallback",
				"error", err,
				"summaries", len(compacted))
			summary = stubRecompaction(compacted)
		}
	} else {
		summary = stubRecompaction(compacted)
	}

	tokensAfter := estimateTokens(summary)

	cm.regions[RegionCompactedHistory] = []contextMessage{{
		Message: agentic.ChatMessage{
			Role:    "system",
			Content: summary,
		},
		Tokens:    tokensAfter,
		Iteration: cm.currentIteration,
	}}

	c.logger.Info("recompacted history summaries",
		"loop_id", cm.loopID,
		"summaries_before", len(compacted),
		"tokens_before", tokensBefore,
		"tokens_after", tokensAfter)
}

// stubRecompaction concatenates existing summaries as a fallback when no summarizer is available.
func stubRecompaction(compacted []contextMessage) string {
	var b strings.Builder
	for i, m := range compacted {
		if i > 0 {
			b.WriteString("\n---\n")
		}
		b.WriteString(m.Message.Content)
	}
	return fmt.Sprintf("[Consolidated summary of %d compactions]\n%s", len(compacted), b.String())
}

// stubSummary generates a placeholder summary string when no LLM summarizer is available.
// It includes the compaction count to ensure consecutive compactions produce distinct summaries.
func stubSummary(compactedCount, msgCount int) string {
	return fmt.Sprintf("Summary #%d (%d msgs)", compactedCount+1, msgCount)
}
