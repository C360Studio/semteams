// Package graphembedding query handlers
package graphembedding

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"time"

	"github.com/c360/semstreams/component"
	"github.com/c360/semstreams/graph/embedding"
)

// setupQueryHandlers sets up NATS request/reply subscriptions for query handlers
func (c *Component) setupQueryHandlers(ctx context.Context) error {
	// Subscribe to similar entity query
	if err := c.natsClient.SubscribeForRequests(ctx, "graph.embedding.query.similar", c.handleQuerySimilarNATS); err != nil {
		return fmt.Errorf("subscribe similar query: %w", err)
	}

	// Subscribe to text search query
	if err := c.natsClient.SubscribeForRequests(ctx, "graph.embedding.query.search", c.handleQuerySearchNATS); err != nil {
		return fmt.Errorf("subscribe search query: %w", err)
	}

	// Subscribe to capabilities discovery
	if err := c.natsClient.SubscribeForRequests(ctx, "graph.embedding.capabilities", c.handleCapabilitiesNATS); err != nil {
		return fmt.Errorf("subscribe capabilities: %w", err)
	}

	c.logger.Info("query handlers registered",
		"subjects", []string{"graph.embedding.query.similar", "graph.embedding.query.search", "graph.embedding.capabilities"})

	return nil
}

// SimilarRequest is the request format for similar entity queries
type SimilarRequest struct {
	EntityID string `json:"entity_id"`
	Limit    int    `json:"limit"`
}

// SimilarResponse is the response format for similar entity queries
type SimilarResponse struct {
	EntityID string          `json:"entity_id"`
	Similar  []SimilarEntity `json:"similar"`
	Duration string          `json:"duration"`
}

// SimilarEntity represents an entity with similarity score
type SimilarEntity struct {
	EntityID   string  `json:"entity_id"`
	Similarity float64 `json:"similarity"`
}

// SearchRequest is the request format for text search queries
type SearchRequest struct {
	Query string `json:"query"`
	Limit int    `json:"limit"`
}

// SearchResponse is the response format for text search queries
type SearchResponse struct {
	Query    string         `json:"query"`
	Results  []SearchResult `json:"results"`
	Duration string         `json:"duration"`
}

// SearchResult represents a search result with relevance score
type SearchResult struct {
	EntityID   string  `json:"entity_id"`
	Similarity float64 `json:"similarity"`
}

// handleQuerySimilarNATS handles similar entity query requests via NATS request/reply
func (c *Component) handleQuerySimilarNATS(_ context.Context, data []byte) ([]byte, error) {
	start := time.Now()

	// Create context with timeout for KV operations
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Parse request
	var req SimilarRequest
	if err := json.Unmarshal(data, &req); err != nil {
		return nil, fmt.Errorf("invalid request: %w", err)
	}

	// Validate request
	if req.EntityID == "" {
		return nil, fmt.Errorf("invalid request: empty entity_id")
	}

	// Apply defaults
	limit := req.Limit
	if limit <= 0 {
		limit = 10
	}
	if limit > 100 {
		limit = 100
	}

	// Get source entity embedding
	if c.storage == nil {
		return nil, fmt.Errorf("storage not initialized")
	}

	sourceRecord, err := c.storage.GetEmbedding(ctx, req.EntityID)
	if err != nil {
		return nil, fmt.Errorf("get source embedding: %w", err)
	}
	if sourceRecord == nil {
		return nil, fmt.Errorf("not found: %s", req.EntityID)
	}
	if sourceRecord.Status != embedding.StatusGenerated {
		return nil, fmt.Errorf("embedding not ready for %s: status=%s", req.EntityID, sourceRecord.Status)
	}
	if len(sourceRecord.Vector) == 0 {
		return nil, fmt.Errorf("no vector for entity %s", req.EntityID)
	}

	// Find similar entities by scanning all embeddings
	similar, err := c.findSimilarEntities(ctx, req.EntityID, sourceRecord.Vector, limit)
	if err != nil {
		return nil, fmt.Errorf("find similar: %w", err)
	}

	response := SimilarResponse{
		EntityID: req.EntityID,
		Similar:  similar,
		Duration: time.Since(start).String(),
	}

	return json.Marshal(response)
}

// handleQuerySearchNATS handles text search query requests via NATS request/reply
func (c *Component) handleQuerySearchNATS(_ context.Context, data []byte) ([]byte, error) {
	start := time.Now()

	// Create context with timeout
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Parse request
	var req SearchRequest
	if err := json.Unmarshal(data, &req); err != nil {
		return nil, fmt.Errorf("invalid request: %w", err)
	}

	// Validate request
	if req.Query == "" {
		return nil, fmt.Errorf("invalid request: empty query")
	}

	// Apply defaults
	limit := req.Limit
	if limit <= 0 {
		limit = 10
	}
	if limit > 100 {
		limit = 100
	}

	// Check embedder is available
	if c.embedder == nil {
		return nil, fmt.Errorf("embedder not initialized")
	}

	// Generate embedding for query text
	vectors, err := c.embedder.Generate(ctx, []string{req.Query})
	if err != nil {
		return nil, fmt.Errorf("generate query embedding: %w", err)
	}
	if len(vectors) == 0 || len(vectors[0]) == 0 {
		return nil, fmt.Errorf("empty query embedding")
	}

	queryVector := vectors[0]

	// Find similar entities
	results, err := c.findSimilarEntities(ctx, "", queryVector, limit)
	if err != nil {
		return nil, fmt.Errorf("find similar: %w", err)
	}

	// Convert to search results
	searchResults := make([]SearchResult, len(results))
	for i, r := range results {
		searchResults[i] = SearchResult{
			EntityID:   r.EntityID,
			Similarity: r.Similarity,
		}
	}

	response := SearchResponse{
		Query:    req.Query,
		Results:  searchResults,
		Duration: time.Since(start).String(),
	}

	return json.Marshal(response)
}

// findSimilarEntities finds entities similar to the given vector
func (c *Component) findSimilarEntities(ctx context.Context, excludeID string, queryVector []float32, limit int) ([]SimilarEntity, error) {
	if c.storage == nil {
		return nil, fmt.Errorf("storage not initialized")
	}

	// List all entity IDs with embeddings
	entityIDs, err := c.storage.ListGeneratedEntityIDs(ctx)
	if err != nil {
		return nil, fmt.Errorf("list entity IDs: %w", err)
	}

	// Calculate similarities
	type scored struct {
		entityID   string
		similarity float64
	}
	var scores []scored

	for _, entityID := range entityIDs {
		// Skip the source entity
		if entityID == excludeID {
			continue
		}

		record, err := c.storage.GetEmbedding(ctx, entityID)
		if err != nil || record == nil {
			continue
		}
		if record.Status != embedding.StatusGenerated || len(record.Vector) == 0 {
			continue
		}

		// Calculate cosine similarity
		sim := embedding.CosineSimilarity(queryVector, record.Vector)
		scores = append(scores, scored{entityID: entityID, similarity: sim})
	}

	// Sort by similarity descending
	sort.Slice(scores, func(i, j int) bool {
		return scores[i].similarity > scores[j].similarity
	})

	// Take top N
	if len(scores) > limit {
		scores = scores[:limit]
	}

	// Convert to response format
	results := make([]SimilarEntity, len(scores))
	for i, s := range scores {
		results[i] = SimilarEntity{
			EntityID:   s.entityID,
			Similarity: s.similarity,
		}
	}

	return results, nil
}

// handleCapabilitiesNATS handles capability discovery requests via NATS request/reply
func (c *Component) handleCapabilitiesNATS(_ context.Context, _ []byte) ([]byte, error) {
	caps := c.QueryCapabilities()
	return json.Marshal(caps)
}

// Ensure Component implements QueryCapabilityProvider
var _ component.QueryCapabilityProvider = (*Component)(nil)

// QueryCapabilities implements QueryCapabilityProvider interface
func (c *Component) QueryCapabilities() component.QueryCapabilities {
	return component.QueryCapabilities{
		Component: "graph-embedding",
		Version:   "1.0.0",
		Queries: []component.QueryCapability{
			{
				Subject:     "graph.embedding.query.similar",
				Operation:   "findSimilar",
				Description: "Find entities similar to a given entity by embedding similarity",
				Intent:      component.QueryIntent{Type: component.IntentTypeSemantic, Strategy: component.StrategyDirect, Scope: component.ScopeSet},
				EntityTypes: []string{"*"},
				RequestSchema: map[string]any{
					"type": "object",
					"properties": map[string]any{
						"entity_id": map[string]any{
							"type":        "string",
							"description": "Source entity ID to find similar entities for",
						},
						"limit": map[string]any{
							"type":        "integer",
							"description": "Maximum number of similar entities to return (default 10, max 100)",
						},
					},
					"required": []string{"entity_id"},
				},
				ResponseSchema: map[string]any{
					"type": "object",
					"properties": map[string]any{
						"entity_id": map[string]any{"type": "string"},
						"similar": map[string]any{
							"type": "array",
							"items": map[string]any{
								"type": "object",
								"properties": map[string]any{
									"entity_id":  map[string]any{"type": "string"},
									"similarity": map[string]any{"type": "number"},
								},
							},
						},
						"duration": map[string]any{"type": "string"},
					},
				},
			},
			{
				Subject:     "graph.embedding.query.search",
				Operation:   "search",
				Description: "Search for entities by text query using embedding similarity",
				Intent:      component.QueryIntent{Type: component.IntentTypeSemantic, Strategy: component.StrategyDirect, Scope: component.ScopeSet},
				EntityTypes: []string{"*"},
				RequestSchema: map[string]any{
					"type": "object",
					"properties": map[string]any{
						"query": map[string]any{
							"type":        "string",
							"description": "Text query to search for",
						},
						"limit": map[string]any{
							"type":        "integer",
							"description": "Maximum number of results to return (default 10, max 100)",
						},
					},
					"required": []string{"query"},
				},
				ResponseSchema: map[string]any{
					"type": "object",
					"properties": map[string]any{
						"query": map[string]any{"type": "string"},
						"results": map[string]any{
							"type": "array",
							"items": map[string]any{
								"type": "object",
								"properties": map[string]any{
									"entity_id":  map[string]any{"type": "string"},
									"similarity": map[string]any{"type": "number"},
								},
							},
						},
						"duration": map[string]any{"type": "string"},
					},
				},
			},
		},
	}
}
