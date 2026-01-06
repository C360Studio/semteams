// Package querymanager provides the Querier interface and QueryManager implementation.
// This service is responsible for serving all read operations with multi-tier caching,
// KV Watch for cache invalidation, and query optimization.
//
// The QueryManager handles all read/query operations and maintains a multi-tier
// cache hierarchy with KV Watch for real-time cache invalidation.
package querymanager

import (
	"context"
	"time"

	gtypes "github.com/c360/semstreams/graph"
	"github.com/c360/semstreams/graph/clustering"
)

// Querier handles all read operations with multi-tier caching.
// It watches ENTITY_STATES KV for cache invalidation and provides
// optimized query execution with result caching.
type Querier interface {

	// Entity queries (with multi-tier caching)
	GetEntity(ctx context.Context, id string) (*gtypes.EntityState, error)
	GetEntities(ctx context.Context, ids []string) ([]*gtypes.EntityState, error)
	GetEntityByAlias(ctx context.Context, aliasOrID string) (*gtypes.EntityState, error)

	// Complex queries with result caching
	ExecutePath(ctx context.Context, start string, pattern PathPattern) (*QueryResult, error)
	GetGraphSnapshot(ctx context.Context, bounds QueryBounds) (*GraphSnapshot, error)
	QueryRelationships(ctx context.Context, entityID string, direction Direction) ([]*Relationship, error)

	// GraphRAG search operations
	LocalSearch(ctx context.Context, entityID string, query string, level int) (*LocalSearchResult, error)
	GlobalSearch(ctx context.Context, query string, level int, maxCommunities int) (*GlobalSearchResult, error)
	GlobalSearchWithOptions(ctx context.Context, opts *SearchOptions) (*GlobalSearchResult, error)

	// Community operations
	GetCommunity(ctx context.Context, communityID string) (*clustering.Community, error)
	GetEntityCommunity(ctx context.Context, entityID string, level int) (*clustering.Community, error)
	GetCommunitiesByLevel(ctx context.Context, level int) ([]*clustering.Community, error)

	// Query operations delegated to index manager
	QueryByPredicate(ctx context.Context, predicate string) ([]string, error)
	QuerySpatial(ctx context.Context, bounds SpatialBounds) ([]string, error)
	QueryTemporal(ctx context.Context, start, end time.Time) ([]string, error)

	// Cache management (triggered by KV watch)
	InvalidateEntity(entityID string) error
	WarmCache(ctx context.Context, entityIDs []string) error

	// Cache statistics
	GetCacheStats() CacheStats

	// EntityID hierarchy navigation
	// ListWithPrefix returns entity IDs matching the given prefix.
	// Used for EntityID hierarchy navigation (e.g., "c360.logistics" returns all logistics entities).
	ListWithPrefix(ctx context.Context, prefix string) ([]string, error)

	// GetHierarchyStats returns entity counts grouped by next hierarchy level.
	// Used by MCP tools to understand graph structure at each EntityID level.
	GetHierarchyStats(ctx context.Context, prefix string) (*HierarchyStats, error)

	// SearchSimilar performs similarity search using embeddings (BM25 or neural).
	// Returns entities ranked by cosine similarity score to the query text.
	// Works on both statistical (BM25) and semantic (neural) tiers.
	// Used by the similaritySearch GraphQL query.
	SearchSimilar(ctx context.Context, query string, limit int) (*SimilaritySearchResult, error)
}

// SimilaritySearchResult represents the results of a similarity search query
type SimilaritySearchResult struct {
	// Hits are the matched entities with their scores
	Hits []SimilarityHit `json:"hits"`

	// Total is the total number of hits (may be greater than len(Hits) if limited)
	Total int `json:"total"`
}

// SimilarityHit represents a single similarity search result
type SimilarityHit struct {
	// EntityID is the unique identifier of the matched entity
	EntityID string `json:"entity_id"`

	// Score is the similarity score (0-1, higher is more similar)
	Score float64 `json:"score"`
}

// Direction represents query direction
type Direction string

const (
	// DirectionIncoming represents traversing incoming edges to an entity.
	DirectionIncoming Direction = "incoming"
	// DirectionOutgoing represents traversing outgoing edges from an entity.
	DirectionOutgoing Direction = "outgoing"
	// DirectionBoth represents traversing both incoming and outgoing edges.
	DirectionBoth Direction = "both"
)
