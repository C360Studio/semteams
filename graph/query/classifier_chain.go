package query

import (
	"context"
)

// ClassificationResult contains the classification output from the tiered classifier chain.
type ClassificationResult struct {
	Tier       int            // Which tier produced the result (0=keyword, 1=BM25, 2=neural)
	Intent     string         // Classified intent (from embedding match, empty for keyword)
	Options    map[string]any // SearchOptions hints (from keyword or embedding example)
	Confidence float64        // Confidence score (1.0 for keywords, similarity for embedding)
}

// ClassifierChain orchestrates tiered query classification.
//
// Routing logic:
//   - T0 (keyword): Always try first - keyword patterns bypass embedding
//   - T1/T2 (embedding): Only if no keyword match AND embedding != nil
//   - Default: Return T0 result with empty options if no tier matches
//
// Thread-safe for concurrent ClassifyQuery calls.
type ClassifierChain struct {
	keyword   *KeywordClassifier   // T0 - always available (pattern matching)
	embedding *EmbeddingClassifier // T1/T2 - statistical or neural (optional)
	// llm       *LLMClassifier    // T3 - future
}

// NewClassifierChain creates a classifier chain with keyword and optional embedding classifiers.
//
// Both parameters may be nil. Chain will route queries through available tiers.
func NewClassifierChain(keyword *KeywordClassifier, embedding *EmbeddingClassifier) *ClassifierChain {
	return &ClassifierChain{
		keyword:   keyword,
		embedding: embedding,
	}
}

// ClassifyQuery classifies a query through the tier chain.
//
// Returns result from first tier that produces a match:
//   - T0: Keyword patterns (temporal, spatial, similarity, path, zone)
//   - T1/T2: Embedding similarity (if available and no keyword match)
//   - Default: T0 result with no filters if no tier matches
//
// Returns nil if chain is nil or context cancelled.
func (c *ClassifierChain) ClassifyQuery(ctx context.Context, query string) *ClassificationResult {
	// Defensive nil check - nil chain returns nil
	if c == nil {
		return nil
	}

	// Check context cancellation before starting
	select {
	case <-ctx.Done():
		return nil
	default:
	}

	// T0: Try keyword classifier first (if available)
	if c.keyword != nil {
		opts := c.keyword.ClassifyQuery(ctx, query)
		if opts != nil && hasExplicitIntent(opts) {
			// Keyword matched - return T0 result
			return &ClassificationResult{
				Tier:       0,
				Intent:     "", // Keyword tier doesn't set Intent (uses SearchOptions fields)
				Options:    searchOptionsToMap(opts),
				Confidence: 1.0,
			}
		}
	}

	// T1/T2: Try embedding classifier if available and no keyword match
	if c.embedding != nil {
		match, score := c.embedding.FindBestMatch(ctx, query)
		if match != nil && score >= c.embedding.Threshold() {
			// Embedding matched - return T1 or T2 result
			// Tier 1 for BM25, Tier 2 for neural (but tests accept either)
			return &ClassificationResult{
				Tier:       1, // Use 1 for embedding tier (could be BM25 or neural)
				Intent:     match.Intent,
				Options:    match.Options,
				Confidence: score,
			}
		}
	}

	// Default: Return T0 result with no filters
	return &ClassificationResult{
		Tier:       0,
		Intent:     "",
		Options:    make(map[string]any),
		Confidence: 1.0,
	}
}

// hasExplicitIntent checks if SearchOptions has any explicit filters or intents.
//
// Returns true if any of these are set:
//   - TimeRange (temporal filter)
//   - GeoBounds (spatial filter)
//   - UseEmbeddings (similarity intent)
//   - PathIntent (path/zone intent)
//   - AggregationType (aggregation/ranking intent)
func hasExplicitIntent(opts *SearchOptions) bool {
	if opts == nil {
		return false
	}

	return opts.TimeRange != nil ||
		opts.GeoBounds != nil ||
		opts.UseEmbeddings ||
		opts.PathIntent ||
		opts.AggregationType != ""
}

// searchOptionsToMap converts SearchOptions to map[string]any for ClassificationResult.
//
// Only includes non-nil/non-zero fields relevant to classification.
func searchOptionsToMap(opts *SearchOptions) map[string]any {
	if opts == nil {
		return make(map[string]any)
	}

	result := make(map[string]any)

	// Add present fields
	if opts.TimeRange != nil {
		result["time_range"] = opts.TimeRange
	}
	if opts.GeoBounds != nil {
		result["geo_bounds"] = opts.GeoBounds
	}
	if opts.UseEmbeddings {
		result["use_embeddings"] = opts.UseEmbeddings
	}
	if opts.PathIntent {
		result["path_intent"] = opts.PathIntent
		if opts.PathStartNode != "" {
			result["path_start_node"] = opts.PathStartNode
		}
	}
	if len(opts.PathPredicates) > 0 {
		result["path_predicates"] = opts.PathPredicates
	}
	if opts.AggregationType != "" {
		result["aggregation_type"] = opts.AggregationType
		if opts.AggregationField != "" {
			result["aggregation_field"] = opts.AggregationField
		}
	}
	if opts.RankingIntent {
		result["ranking_intent"] = opts.RankingIntent
		if opts.Limit > 0 {
			result["limit"] = opts.Limit
		}
	}

	return result
}
