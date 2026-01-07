package clustering

import (
	"context"
	"testing"

	gtypes "github.com/c360/semstreams/graph"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mockSimilarityFinder implements SimilarityFinder for testing
type mockSimilarityFinder struct {
	similarities map[string][]gtypes.SimilarityHit
}

func newMockSimilarityFinder() *mockSimilarityFinder {
	return &mockSimilarityFinder{
		similarities: make(map[string][]gtypes.SimilarityHit),
	}
}

func (m *mockSimilarityFinder) addSimilarity(entityID string, hits []gtypes.SimilarityHit) {
	m.similarities[entityID] = hits
}

func (m *mockSimilarityFinder) FindSimilarEntities(
	_ context.Context,
	entityID string,
	threshold float64,
	limit int,
) ([]gtypes.SimilarityHit, error) {
	hits := m.similarities[entityID]
	// Apply threshold filter
	var filtered []gtypes.SimilarityHit
	for _, hit := range hits {
		if hit.Similarity >= threshold {
			filtered = append(filtered, hit)
		}
		if len(filtered) >= limit {
			break
		}
	}
	return filtered, nil
}

// TestSemanticGraphProvider_GetNeighbors_CombinesExplicitAndVirtual tests that
// GetNeighbors returns both explicit neighbors and virtual semantic neighbors.
func TestSemanticGraphProvider_GetNeighbors_CombinesExplicitAndVirtual(t *testing.T) {
	ctx := context.Background()

	// Create base provider with explicit edges
	baseProvider := NewMockGraphProvider()
	baseProvider.AddEntity("A")
	baseProvider.AddEntity("B")
	baseProvider.AddEntity("C")
	baseProvider.AddEntity("D")
	baseProvider.AddEdge("A", "B", 1.0) // Explicit edge A->B

	// Create similarity finder with virtual edges
	finder := newMockSimilarityFinder()
	finder.addSimilarity("A", []gtypes.SimilarityHit{
		{EntityID: "C", Similarity: 0.8}, // Virtual neighbor via similarity
		{EntityID: "D", Similarity: 0.7}, // Virtual neighbor via similarity
		{EntityID: "B", Similarity: 0.9}, // Already explicit neighbor
	})

	// Create semantic provider
	config := SemanticProviderConfig{
		SimilarityThreshold: 0.6,
		MaxVirtualNeighbors: 5,
	}
	provider := NewSemanticGraphProvider(baseProvider, finder, config, nil)

	// Get neighbors
	neighbors, err := provider.GetNeighbors(ctx, "A", "both")
	require.NoError(t, err)

	// Should include B (explicit) + C, D (virtual), but not B again (deduplicated)
	assert.Len(t, neighbors, 3)
	assert.Contains(t, neighbors, "B") // Explicit
	assert.Contains(t, neighbors, "C") // Virtual
	assert.Contains(t, neighbors, "D") // Virtual
}

// TestSemanticGraphProvider_GetEdgeWeight_ExplicitTakesPrecedence tests that
// explicit edges have priority over virtual edges.
func TestSemanticGraphProvider_GetEdgeWeight_ExplicitTakesPrecedence(t *testing.T) {
	ctx := context.Background()

	// Create base provider with explicit edge
	baseProvider := NewMockGraphProvider()
	baseProvider.AddEntity("A")
	baseProvider.AddEntity("B")
	baseProvider.AddEdge("A", "B", 0.9) // Explicit edge with 0.9 weight

	// Create similarity finder that would return 0.7 for the same pair
	finder := newMockSimilarityFinder()
	finder.addSimilarity("A", []gtypes.SimilarityHit{
		{EntityID: "B", Similarity: 0.7}, // Would be 0.7 if virtual
	})

	config := DefaultSemanticProviderConfig()
	provider := NewSemanticGraphProvider(baseProvider, finder, config, nil)

	// Get edge weight
	weight, err := provider.GetEdgeWeight(ctx, "A", "B")
	require.NoError(t, err)

	// Should return explicit weight (0.9), not virtual weight (0.7)
	assert.Equal(t, 0.9, weight)
}

// TestSemanticGraphProvider_GetEdgeWeight_ReturnsVirtualIfNoExplicit tests that
// virtual similarity is used when no explicit edge exists.
func TestSemanticGraphProvider_GetEdgeWeight_ReturnsVirtualIfNoExplicit(t *testing.T) {
	ctx := context.Background()

	// Create base provider WITHOUT edge
	baseProvider := NewMockGraphProvider()
	baseProvider.AddEntity("A")
	baseProvider.AddEntity("B")
	// No edge between A and B

	// Create similarity finder with virtual similarity
	finder := newMockSimilarityFinder()
	finder.addSimilarity("A", []gtypes.SimilarityHit{
		{EntityID: "B", Similarity: 0.75},
	})

	config := DefaultSemanticProviderConfig()
	provider := NewSemanticGraphProvider(baseProvider, finder, config, nil)

	// First call GetNeighbors to populate cache
	_, _ = provider.GetNeighbors(ctx, "A", "both")

	// Now get edge weight
	weight, err := provider.GetEdgeWeight(ctx, "A", "B")
	require.NoError(t, err)

	// Should return virtual weight from cache
	assert.InDelta(t, 0.75, weight, 0.01)
}

// TestSemanticGraphProvider_GetNeighbors_RespectsThreshold tests that
// similarity threshold is applied to virtual neighbors.
func TestSemanticGraphProvider_GetNeighbors_RespectsThreshold(t *testing.T) {
	ctx := context.Background()

	baseProvider := NewMockGraphProvider()
	baseProvider.AddEntity("A")
	baseProvider.AddEntity("B")
	baseProvider.AddEntity("C")

	finder := newMockSimilarityFinder()
	finder.addSimilarity("A", []gtypes.SimilarityHit{
		{EntityID: "B", Similarity: 0.8}, // Above threshold
		{EntityID: "C", Similarity: 0.4}, // Below threshold
	})

	config := SemanticProviderConfig{
		SimilarityThreshold: 0.6, // Set threshold at 0.6
		MaxVirtualNeighbors: 5,
	}
	provider := NewSemanticGraphProvider(baseProvider, finder, config, nil)

	neighbors, err := provider.GetNeighbors(ctx, "A", "both")
	require.NoError(t, err)

	// Should only include B (above threshold), not C
	assert.Len(t, neighbors, 1)
	assert.Contains(t, neighbors, "B")
}

// TestSemanticGraphProvider_GetNeighbors_RespectsLimit tests that
// max virtual neighbors limit is applied.
func TestSemanticGraphProvider_GetNeighbors_RespectsLimit(t *testing.T) {
	ctx := context.Background()

	baseProvider := NewMockGraphProvider()
	baseProvider.AddEntity("A")
	for i := 0; i < 10; i++ {
		baseProvider.AddEntity(string(rune('B' + i)))
	}

	// Add 10 similar entities
	finder := newMockSimilarityFinder()
	var hits []gtypes.SimilarityHit
	for i := 0; i < 10; i++ {
		hits = append(hits, gtypes.SimilarityHit{
			EntityID:   string(rune('B' + i)),
			Similarity: 0.9 - float64(i)*0.01,
		})
	}
	finder.addSimilarity("A", hits)

	config := SemanticProviderConfig{
		SimilarityThreshold: 0.6,
		MaxVirtualNeighbors: 3, // Limit to 3
	}
	provider := NewSemanticGraphProvider(baseProvider, finder, config, nil)

	neighbors, err := provider.GetNeighbors(ctx, "A", "both")
	require.NoError(t, err)

	// Should only return 3 neighbors (the limit)
	assert.Len(t, neighbors, 3)
}

// TestSemanticGraphProvider_ClearCache tests cache clearing
func TestSemanticGraphProvider_ClearCache(t *testing.T) {
	ctx := context.Background()

	baseProvider := NewMockGraphProvider()
	baseProvider.AddEntity("A")
	baseProvider.AddEntity("B")

	finder := newMockSimilarityFinder()
	finder.addSimilarity("A", []gtypes.SimilarityHit{
		{EntityID: "B", Similarity: 0.8},
	})

	config := DefaultSemanticProviderConfig()
	provider := NewSemanticGraphProvider(baseProvider, finder, config, nil)

	// Populate cache
	_, _ = provider.GetNeighbors(ctx, "A", "both")

	entities, edges := provider.GetCacheStats()
	assert.Greater(t, entities, 0)
	assert.Greater(t, edges, 0)

	// Clear cache
	provider.ClearCache()

	entities, edges = provider.GetCacheStats()
	assert.Equal(t, 0, entities)
	assert.Equal(t, 0, edges)
}

// TestSemanticGraphProvider_NilFinder tests graceful degradation with nil finder
func TestSemanticGraphProvider_NilFinder(t *testing.T) {
	ctx := context.Background()

	baseProvider := NewMockGraphProvider()
	baseProvider.AddEntity("A")
	baseProvider.AddEntity("B")
	baseProvider.AddEdge("A", "B", 1.0)

	// Create provider with nil finder
	config := DefaultSemanticProviderConfig()
	provider := NewSemanticGraphProvider(baseProvider, nil, config, nil)

	// Should still work, returning only explicit neighbors
	neighbors, err := provider.GetNeighbors(ctx, "A", "both")
	require.NoError(t, err)
	assert.Len(t, neighbors, 1)
	assert.Contains(t, neighbors, "B")
}
