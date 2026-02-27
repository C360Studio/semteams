package boid

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"github.com/c360studio/semstreams/graph/clustering"
)

// PageRankCentralityProvider implements CentralityProvider using cached PageRank scores.
// It computes PageRank periodically based on the configured TTL and caches the results.
type PageRankCentralityProvider struct {
	provider clustering.Provider
	logger   *slog.Logger

	mu           sync.RWMutex
	scores       map[string]float64
	ttl          time.Duration
	lastComputed time.Time
}

// NewPageRankCentralityProvider creates a new PageRank-based centrality provider.
// The provider parameter must implement the clustering.Provider interface for graph access.
// TTL controls how often PageRank is recomputed (0 means compute on every request).
func NewPageRankCentralityProvider(provider clustering.Provider, ttl time.Duration, logger *slog.Logger) *PageRankCentralityProvider {
	if logger == nil {
		logger = slog.Default()
	}
	return &PageRankCentralityProvider{
		provider: provider,
		ttl:      ttl,
		scores:   make(map[string]float64),
		logger:   logger,
	}
}

// GetPageRankScores returns PageRank scores for the specified entities.
// If the cached scores are stale (older than TTL), they are recomputed first.
// Returns a map of entity ID to normalized score (0.0-1.0).
func (p *PageRankCentralityProvider) GetPageRankScores(ctx context.Context, entityIDs []string) (map[string]float64, error) {
	// Check if we need to refresh
	p.mu.RLock()
	needsRefresh := time.Since(p.lastComputed) > p.ttl || len(p.scores) == 0
	p.mu.RUnlock()

	if needsRefresh {
		if err := p.refresh(ctx); err != nil {
			return nil, err
		}
	}

	// Extract scores for requested entities
	p.mu.RLock()
	defer p.mu.RUnlock()

	result := make(map[string]float64, len(entityIDs))
	for _, id := range entityIDs {
		if score, ok := p.scores[id]; ok {
			result[id] = score
		}
	}
	return result, nil
}

// refresh recomputes PageRank scores for the entire graph.
func (p *PageRankCentralityProvider) refresh(ctx context.Context) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	// Double-check after acquiring lock (another goroutine may have refreshed)
	if time.Since(p.lastComputed) <= p.ttl && len(p.scores) > 0 {
		return nil
	}

	startTime := time.Now()

	config := clustering.DefaultPageRankConfig()
	result, err := clustering.ComputePageRank(ctx, p.provider, config)
	if err != nil {
		p.logger.Error("Failed to compute PageRank",
			"error", err)
		return err
	}

	p.scores = result.Scores
	p.lastComputed = time.Now()

	elapsed := time.Since(startTime)
	p.logger.Debug("PageRank computed for centrality provider",
		"entity_count", len(result.Scores),
		"iterations", result.Iterations,
		"converged", result.Converged,
		"elapsed", elapsed)

	return nil
}

// Invalidate clears the cached scores, forcing a recomputation on next request.
func (p *PageRankCentralityProvider) Invalidate() {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.scores = make(map[string]float64)
	p.lastComputed = time.Time{}
}

// Compile-time interface compliance check.
var _ CentralityProvider = (*PageRankCentralityProvider)(nil)
