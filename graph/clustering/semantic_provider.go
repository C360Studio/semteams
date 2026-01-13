// Package clustering provides community detection algorithms and graph providers.
package clustering

import (
	"context"
	"log/slog"
	"sync"
	"sync/atomic"

	gtypes "github.com/c360/semstreams/graph"
	"github.com/c360/semstreams/pkg/errs"
)

// SimilarityFinder abstracts the IndexManager's similarity lookup.
// This interface allows SemanticProvider to work with IndexManager without
// creating import cycles and enables easier testing.
type SimilarityFinder interface {
	// FindSimilarEntities returns entities similar to the given entity.
	// threshold: minimum cosine similarity (0.0-1.0)
	// limit: maximum results to return
	FindSimilarEntities(ctx context.Context, entityID string, threshold float64, limit int) ([]gtypes.SimilarityHit, error)
}

// SemanticProvider wraps a base Provider and adds virtual edges
// based on embedding similarity. This enables LPA clustering to find communities
// even when explicit relationship triples don't exist.
//
// Virtual edges are computed on-demand using cosine similarity between entity embeddings.
// These edges are NOT persisted - they're ephemeral hints for the clustering algorithm.
//
// Explicit edges (from base provider) always take precedence over virtual edges,
// and explicit edge weights are preserved as-is (typically confidence-based).
type SemanticProvider struct {
	base Provider

	// Similarity lookup via IndexManager
	similarityFinder SimilarityFinder

	// Configuration
	similarityThreshold float64 // Minimum similarity for virtual edge (default: 0.6)
	maxVirtualNeighbors int     // Max virtual neighbors per entity (default: 5)

	// Logger for debugging and observability
	logger *slog.Logger

	// Metrics for monitoring virtual edge operations
	virtualEdgeErrors  atomic.Int64
	virtualEdgeSuccess atomic.Int64

	// Cache for similarity scores computed during neighbor discovery
	// Reused by GetEdgeWeight to avoid recomputation
	similarityCache   map[string]map[string]float64
	similarityCacheMu sync.RWMutex
}

// SemanticProviderConfig holds configuration for SemanticProvider
type SemanticProviderConfig struct {
	// SimilarityThreshold is the minimum cosine similarity for virtual edges.
	// Higher values = fewer but stronger virtual connections.
	// Recommended: 0.6 for clustering (stricter than 0.3 used in search)
	SimilarityThreshold float64

	// MaxVirtualNeighbors limits virtual neighbors per entity to control
	// computation cost during LPA iterations.
	// Recommended: 5
	MaxVirtualNeighbors int
}

// DefaultSemanticProviderConfig returns sensible defaults for clustering
func DefaultSemanticProviderConfig() SemanticProviderConfig {
	return SemanticProviderConfig{
		SimilarityThreshold: 0.6,
		MaxVirtualNeighbors: 5,
	}
}

// NewSemanticProvider creates a Provider that augments explicit edges
// with virtual edges based on embedding similarity.
//
// Parameters:
//   - base: The underlying Provider for explicit edges
//   - finder: SimilarityFinder (typically IndexManager) for similarity lookup
//   - config: Configuration for similarity threshold and limits
//   - logger: Optional logger for observability (can be nil)
func NewSemanticProvider(
	base Provider,
	finder SimilarityFinder,
	config SemanticProviderConfig,
	logger *slog.Logger,
) *SemanticProvider {
	// Apply defaults for zero values
	if config.SimilarityThreshold <= 0 {
		config.SimilarityThreshold = 0.6
	}
	if config.MaxVirtualNeighbors <= 0 {
		config.MaxVirtualNeighbors = 5
	}

	return &SemanticProvider{
		base:                base,
		similarityFinder:    finder,
		similarityThreshold: config.SimilarityThreshold,
		maxVirtualNeighbors: config.MaxVirtualNeighbors,
		similarityCache:     make(map[string]map[string]float64),
		logger:              logger,
	}
}

// GetAllEntityIDs delegates to the base provider
func (p *SemanticProvider) GetAllEntityIDs(ctx context.Context) ([]string, error) {
	return p.base.GetAllEntityIDs(ctx)
}

// GetNeighbors returns both explicit neighbors and virtual neighbors from embedding similarity.
// Virtual neighbors are added when:
//   - Similarity exceeds threshold (default 0.6)
//   - Entity has embeddings available
//   - Not already an explicit neighbor
//
// Direction parameter is respected for explicit edges but ignored for virtual edges
// (semantic similarity is symmetric).
func (p *SemanticProvider) GetNeighbors(ctx context.Context, entityID string, direction string) ([]string, error) {
	if entityID == "" {
		return nil, errs.WrapInvalid(errs.ErrMissingConfig, "SemanticProvider", "GetNeighbors", "entityID is empty")
	}

	// 1. Get explicit neighbors from base provider
	explicit, err := p.base.GetNeighbors(ctx, entityID, direction)
	if err != nil {
		return nil, errs.WrapTransient(err, "SemanticProvider", "GetNeighbors", "base provider error")
	}

	// Create set of explicit neighbors for deduplication
	explicitSet := make(map[string]bool, len(explicit))
	for _, id := range explicit {
		explicitSet[id] = true
	}

	// 2. Get virtual neighbors from similarity
	virtualNeighbors, err := p.findVirtualNeighbors(ctx, entityID, explicitSet)
	if err != nil {
		// Log warning but don't fail - explicit edges are sufficient
		if p.logger != nil {
			p.logger.Warn("virtual neighbor lookup failed, continuing with explicit edges",
				"entity_id", entityID,
				"explicit_neighbors", len(explicit),
				"error", err)
		}
	}

	// 3. Combine and return
	result := make([]string, 0, len(explicit)+len(virtualNeighbors))
	result = append(result, explicit...)
	result = append(result, virtualNeighbors...)

	return result, nil
}

// findVirtualNeighbors returns entities similar to entityID that aren't already explicit neighbors.
// Results are cached for reuse by GetEdgeWeight.
func (p *SemanticProvider) findVirtualNeighbors(
	ctx context.Context,
	entityID string,
	explicitSet map[string]bool,
) ([]string, error) {
	if p.similarityFinder == nil {
		return nil, nil // No similarity finder configured
	}

	// Check context before expensive operation
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	// Find similar entities
	hits, err := p.similarityFinder.FindSimilarEntities(
		ctx,
		entityID,
		p.similarityThreshold,
		p.maxVirtualNeighbors+len(explicitSet), // Request extra to account for explicit filtering
	)
	if err != nil {
		p.virtualEdgeErrors.Add(1)
		if p.logger != nil {
			p.logger.Warn("similarity lookup failed",
				"entity_id", entityID,
				"error", err)
		}
		return nil, err
	}

	// Collect cache updates locally first (avoid lock contention during iteration)
	cacheUpdates := make(map[string]float64, len(hits))
	var virtualNeighbors []string

	for _, hit := range hits {
		// Collect for batch cache update
		cacheUpdates[hit.EntityID] = hit.Similarity

		// Skip if already an explicit neighbor
		if explicitSet[hit.EntityID] {
			continue
		}

		virtualNeighbors = append(virtualNeighbors, hit.EntityID)

		// Apply limit
		if len(virtualNeighbors) >= p.maxVirtualNeighbors {
			break
		}
	}

	// Batch update cache under single lock acquisition
	p.batchCacheSimilarity(entityID, cacheUpdates)
	p.virtualEdgeSuccess.Add(1)

	return virtualNeighbors, nil
}

// batchCacheSimilarity stores multiple similarity scores under a single lock acquisition.
// This avoids the race condition of releasing and re-acquiring locks between updates.
func (p *SemanticProvider) batchCacheSimilarity(sourceEntity string, scores map[string]float64) {
	if len(scores) == 0 {
		return
	}

	p.similarityCacheMu.Lock()
	defer p.similarityCacheMu.Unlock()

	// Ensure source entity map exists
	if p.similarityCache[sourceEntity] == nil {
		p.similarityCache[sourceEntity] = make(map[string]float64)
	}

	// Apply all updates bidirectionally
	for targetEntity, score := range scores {
		// Forward direction
		p.similarityCache[sourceEntity][targetEntity] = score

		// Reverse direction (similarity is symmetric)
		if p.similarityCache[targetEntity] == nil {
			p.similarityCache[targetEntity] = make(map[string]float64)
		}
		p.similarityCache[targetEntity][sourceEntity] = score
	}
}

// cacheSimilarity stores similarity score bidirectionally (used for single updates)
func (p *SemanticProvider) cacheSimilarity(entityA, entityB string, score float64) {
	p.similarityCacheMu.Lock()
	defer p.similarityCacheMu.Unlock()

	if p.similarityCache[entityA] == nil {
		p.similarityCache[entityA] = make(map[string]float64)
	}
	p.similarityCache[entityA][entityB] = score

	if p.similarityCache[entityB] == nil {
		p.similarityCache[entityB] = make(map[string]float64)
	}
	p.similarityCache[entityB][entityA] = score
}

// GetEdgeWeight returns the weight of an edge between two entities.
//
// For explicit edges: delegates to base provider (uses Triple.Confidence)
// For virtual edges: returns cached similarity score
//
// Explicit edges always take precedence - if base returns weight > 0,
// that's used directly. Virtual edge weight is only used when no explicit edge exists.
func (p *SemanticProvider) GetEdgeWeight(ctx context.Context, fromID, toID string) (float64, error) {
	if fromID == "" || toID == "" {
		return 0.0, errs.WrapInvalid(errs.ErrMissingConfig, "SemanticProvider", "GetEdgeWeight", "entity IDs are empty")
	}

	// 1. Try explicit edge first (always takes precedence)
	weight, err := p.base.GetEdgeWeight(ctx, fromID, toID)
	if err != nil {
		return 0.0, errs.WrapTransient(err, "SemanticProvider", "GetEdgeWeight", "base provider error")
	}
	if weight > 0 {
		return weight, nil // Explicit edge exists
	}

	// 2. Check similarity cache for virtual edge
	p.similarityCacheMu.RLock()
	if scores, ok := p.similarityCache[fromID]; ok {
		if score, ok := scores[toID]; ok {
			p.similarityCacheMu.RUnlock()
			return score, nil
		}
	}
	p.similarityCacheMu.RUnlock()

	// 3. Compute similarity on-demand (cache miss)
	if p.similarityFinder != nil {
		hits, err := p.similarityFinder.FindSimilarEntities(ctx, fromID, p.similarityThreshold, 1)
		if err == nil {
			for _, hit := range hits {
				if hit.EntityID == toID {
					p.cacheSimilarity(fromID, toID, hit.Similarity)
					return hit.Similarity, nil
				}
			}
		}
	}

	// No edge found
	return 0.0, nil
}

// ClearCache clears the similarity cache and propagates to wrapped providers.
// Call this between clustering runs to ensure fresh similarity data.
func (p *SemanticProvider) ClearCache() {
	p.similarityCacheMu.Lock()
	p.similarityCache = make(map[string]map[string]float64)
	p.similarityCacheMu.Unlock()

	// Propagate cache clear to wrapped provider
	if cacheClearer, ok := p.base.(interface{ ClearCache() }); ok {
		cacheClearer.ClearCache()
	}
}

// GetCacheStats returns statistics about the similarity cache for monitoring.
func (p *SemanticProvider) GetCacheStats() (entities int, edges int) {
	p.similarityCacheMu.RLock()
	defer p.similarityCacheMu.RUnlock()

	entities = len(p.similarityCache)
	for _, neighbors := range p.similarityCache {
		edges += len(neighbors)
	}
	// Edges are stored bidirectionally, so divide by 2 for actual count
	edges = edges / 2
	return
}
