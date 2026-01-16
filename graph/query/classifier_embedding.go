package query

import (
	"context"
	"fmt"
	"math"
	"strings"
	"sync"

	"github.com/c360/semstreams/graph/embedding"
)

// Embedder interface for vector generation.
//
// This is a simplified interface for the query classifier that supports
// both single and batch embedding operations.
type Embedder interface {
	// Embed generates a vector for a single text.
	Embed(text string) ([]float32, error)

	// EmbedBatch generates vectors for multiple texts.
	EmbedBatch(texts []string) ([][]float32, error)
}

// EmbeddingClassifier classifies queries by finding similar domain examples.
//
// The classifier starts with BM25 vectors (warm cache - no external service needed)
// and can be upgraded to neural vectors later when embeddings are available.
//
// Thread-safe for concurrent FindBestMatch calls during vector upgrades.
type EmbeddingClassifier struct {
	examples  []Example    // All loaded examples with vectors
	embedder  Embedder     // Current embedder (BM25 or neural)
	threshold float64      // Minimum similarity threshold
	mu        sync.RWMutex // Protects examples during upgrades
}

// bm25Adapter adapts graph/embedding.BM25Embedder to the Embedder interface.
type bm25Adapter struct {
	embedder *embedding.BM25Embedder
}

func (b *bm25Adapter) Embed(text string) ([]float32, error) {
	vecs, err := b.embedder.Generate(context.Background(), []string{text})
	if err != nil {
		return nil, err
	}
	if len(vecs) == 0 {
		return nil, fmt.Errorf("no embedding generated")
	}
	return vecs[0], nil
}

func (b *bm25Adapter) EmbedBatch(texts []string) ([][]float32, error) {
	return b.embedder.Generate(context.Background(), texts)
}

// NewEmbeddingClassifier creates classifier with BM25 warm cache.
//
// Generates BM25 vectors for all examples immediately (no external service needed).
// Returns a classifier ready for statistical similarity matching.
func NewEmbeddingClassifier(domains []*DomainExamples, threshold float64) *EmbeddingClassifier {
	// Aggregate all examples from all domains
	var allExamples []Example
	for _, domain := range domains {
		if domain == nil {
			continue
		}
		allExamples = append(allExamples, domain.Examples...)
	}

	// Create BM25 embedder for initial vectors
	bm25 := embedding.NewBM25Embedder(embedding.BM25Config{
		Dimensions: 384, // Standard embedding dimension
		K1:         1.5,
		B:          0.75,
	})
	adapter := &bm25Adapter{embedder: bm25}

	// Generate BM25 vectors for all examples
	if len(allExamples) > 0 {
		queries := make([]string, len(allExamples))
		for i, ex := range allExamples {
			queries[i] = ex.Query
		}

		vectors, err := adapter.EmbedBatch(queries)
		if err == nil {
			// Attach vectors to examples
			for i := range allExamples {
				if i < len(vectors) {
					allExamples[i].Vector = vectors[i]
				}
			}
		}
	}

	return &EmbeddingClassifier{
		examples:  allExamples,
		embedder:  adapter,
		threshold: threshold,
	}
}

// FindBestMatch finds the most similar example to the query.
//
// Returns nil if no match above threshold or context cancelled.
// Thread-safe - uses read lock to allow concurrent calls during vector upgrades.
func (c *EmbeddingClassifier) FindBestMatch(ctx context.Context, query string) (*Example, float64) {
	// Defensive nil check
	if c == nil {
		return nil, 0.0
	}

	// Check for empty/whitespace query
	if strings.TrimSpace(query) == "" {
		return nil, 0.0
	}

	// Check context cancellation before expensive operation
	select {
	case <-ctx.Done():
		return nil, 0.0
	default:
	}

	// Take read lock to safely access embedder and examples
	c.mu.RLock()
	embedder := c.embedder
	examples := c.examples
	c.mu.RUnlock()

	// Generate query vector (outside lock to avoid blocking during slow ops)
	queryVec, err := embedder.Embed(query)
	if err != nil {
		return nil, 0.0
	}

	// Re-acquire read lock for iteration (examples slice may have been replaced)
	c.mu.RLock()
	defer c.mu.RUnlock()

	// Use current examples (may have been updated, but that's fine)
	examples = c.examples

	// Check for context cancellation during search
	select {
	case <-ctx.Done():
		return nil, 0.0
	default:
	}

	// Empty classifier
	if len(examples) == 0 {
		return nil, 0.0
	}

	var bestMatch *Example
	var bestScore float64

	for i := range examples {
		// Check context periodically for long example lists
		if i%100 == 0 {
			select {
			case <-ctx.Done():
				return nil, 0.0
			default:
			}
		}

		score := cosineSimilarity(queryVec, examples[i].Vector)
		if score > bestScore {
			bestScore = score
			bestMatch = &examples[i]
		}
	}

	// Only return match if above threshold
	if bestScore >= c.threshold {
		return bestMatch, bestScore
	}

	return nil, bestScore
}

// UpgradeVectors replaces current vectors with neural vectors from new embedder.
//
// Thread-safe - uses write lock to prevent concurrent reads during upgrade.
// On error, preserves old vectors (rollback).
func (c *EmbeddingClassifier) UpgradeVectors(embedder Embedder) error {
	if embedder == nil {
		return fmt.Errorf("embedder cannot be nil")
	}

	// Empty classifier - nothing to upgrade
	if len(c.examples) == 0 {
		c.mu.Lock()
		c.embedder = embedder
		c.mu.Unlock()
		return nil
	}

	// Collect all queries
	queries := make([]string, len(c.examples))
	for i, ex := range c.examples {
		queries[i] = ex.Query
	}

	// Generate new vectors
	newVectors, err := embedder.EmbedBatch(queries)
	if err != nil {
		return fmt.Errorf("failed to generate embeddings: %w", err)
	}

	// Verify we got the right number of vectors
	if len(newVectors) != len(c.examples) {
		return fmt.Errorf("dimension mismatch: expected %d vectors, got %d", len(c.examples), len(newVectors))
	}

	// Atomic upgrade with write lock
	c.mu.Lock()
	defer c.mu.Unlock()

	// Replace vectors
	for i := range c.examples {
		c.examples[i].Vector = newVectors[i]
	}

	// Update embedder
	c.embedder = embedder

	return nil
}

// Threshold returns the similarity threshold.
func (c *EmbeddingClassifier) Threshold() float64 {
	return c.threshold
}

// cosineSimilarity computes cosine similarity between two vectors.
//
// Returns value in range [0, 1] where 1.0 is identical and 0.0 is orthogonal.
// Handles zero-length vectors gracefully.
func cosineSimilarity(a, b []float32) float64 {
	if len(a) != len(b) || len(a) == 0 {
		return 0.0
	}

	var dotProduct, normA, normB float64

	for i := 0; i < len(a); i++ {
		dotProduct += float64(a[i]) * float64(b[i])
		normA += float64(a[i]) * float64(a[i])
		normB += float64(b[i]) * float64(b[i])
	}

	// Handle zero vectors
	if normA == 0.0 || normB == 0.0 {
		return 0.0
	}

	return dotProduct / (math.Sqrt(normA) * math.Sqrt(normB))
}
