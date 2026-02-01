package agenticloop

import (
	"context"
	"fmt"

	"github.com/c360/semstreams/agentic"
)

// CompactionResult contains the results of a compaction operation
type CompactionResult struct {
	Summary       string
	EvictedTokens int
	NewTokens     int
}

// Compactor handles context compaction operations
type Compactor struct {
	config ContextConfig
}

// NewCompactor creates a new compactor
func NewCompactor(config ContextConfig) *Compactor {
	return &Compactor{config: config}
}

// ShouldCompact delegates to the context manager's ShouldCompact
func (c *Compactor) ShouldCompact(cm *ContextManager) bool {
	return cm.ShouldCompact()
}

// Compact performs compaction on the context manager
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

	// Generate summary (placeholder - real impl would call LLM)
	// Include compacted history count to make summaries different across multiple compactions
	compactedCount := len(cm.regions[RegionCompactedHistory])
	summary := fmt.Sprintf("Summary #%d (%d msgs)", compactedCount+1, len(recentHistory))
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
	}, nil
}
