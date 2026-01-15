// Package graphql provides GraphQL gateway types and query definitions.
// Types prefixed with QM are internal query types (using graph.EntityState).
// Types without prefix are GraphQL output types (using Entity).
package graphql

import (
	"context"
	"time"

	gtypes "github.com/c360/semstreams/graph"
	"github.com/c360/semstreams/graph/clustering"
)

// Querier handles all read operations for the graph.
// Implementations include NATSQuerier (NATS request/reply).
// Returns internal types (QM prefixed) which resolvers convert to GraphQL types.
type Querier interface {
	// Entity queries
	GetEntity(ctx context.Context, id string) (*gtypes.EntityState, error)
	GetEntities(ctx context.Context, ids []string) ([]*gtypes.EntityState, error)
	GetEntityByAlias(ctx context.Context, aliasOrID string) (*gtypes.EntityState, error)

	// Entity ID resolution for NL queries
	// Resolves partial IDs (e.g., "temp-sensor-001") to full 6-part IDs
	ResolvePartialEntityID(ctx context.Context, partial string) (string, error)

	// Complex queries
	ExecutePath(ctx context.Context, start string, pattern PathPattern) (*QueryResult, error)
	GetGraphSnapshot(ctx context.Context, bounds QueryBounds) (*QMGraphSnapshot, error)
	QueryRelationships(ctx context.Context, entityID string, direction Direction) ([]*QMRelationship, error)

	// GraphRAG search operations
	LocalSearch(ctx context.Context, entityID string, query string, level int) (*QMLocalSearchResult, error)
	GlobalSearch(ctx context.Context, query string, level int, maxCommunities int) (*QMGlobalSearchResult, error)
	GlobalSearchWithOptions(ctx context.Context, opts *SearchOptions) (*QMGlobalSearchResult, error)

	// Community operations
	GetCommunity(ctx context.Context, communityID string) (*clustering.Community, error)
	GetEntityCommunity(ctx context.Context, entityID string, level int) (*clustering.Community, error)
	GetCommunitiesByLevel(ctx context.Context, level int) ([]*clustering.Community, error)

	// Query operations delegated to index manager
	QueryByPredicate(ctx context.Context, predicate string) ([]string, error)
	QuerySpatial(ctx context.Context, bounds SpatialBounds) ([]string, error)
	QueryTemporal(ctx context.Context, start, end time.Time) ([]string, error)

	// Cache management
	InvalidateEntity(entityID string) error
	WarmCache(ctx context.Context, entityIDs []string) error
	GetCacheStats() CacheStats

	// EntityID hierarchy navigation
	ListWithPrefix(ctx context.Context, prefix string) ([]string, error)
	GetHierarchyStats(ctx context.Context, prefix string) (*HierarchyStats, error)

	// Similarity search
	SearchSimilar(ctx context.Context, query string, limit int) (*QMSimilaritySearchResult, error)
}

// Direction represents query direction for relationships.
type Direction string

// Direction constants for relationship queries.
const (
	DirectionIncoming Direction = "incoming"
	DirectionOutgoing Direction = "outgoing"
	DirectionBoth     Direction = "both"
)

// PathPattern represents a graph traversal pattern.
type PathPattern struct {
	MaxDepth        int                    `json:"max_depth"`
	EdgeTypes       []string               `json:"edge_types"`
	NodeTypes       []string               `json:"node_types"`
	Filters         map[string]interface{} `json:"filters"`
	Direction       Direction              `json:"direction"`
	IncludeSelf     bool                   `json:"include_self"`
	MaxNodes        int                    `json:"max_nodes,omitempty"`
	MaxTime         time.Duration          `json:"max_time,omitempty"`
	DecayFactor     float64                `json:"decay_factor,omitempty"`
	IncludeSiblings bool                   `json:"include_siblings,omitempty"`
}

// QueryBounds represents spatial/temporal bounds for graph snapshots.
type QueryBounds struct {
	Spatial     *SpatialBounds  `json:"spatial,omitempty"`
	Temporal    *TemporalBounds `json:"temporal,omitempty"`
	EntityTypes []string        `json:"entity_types,omitempty"`
	MaxEntities int             `json:"max_entities,omitempty"`
}

// SpatialBounds represents spatial query bounds.
type SpatialBounds struct {
	North float64 `json:"north"`
	South float64 `json:"south"`
	East  float64 `json:"east"`
	West  float64 `json:"west"`
}

// TemporalBounds represents temporal query bounds.
type TemporalBounds struct {
	Start time.Time `json:"start"`
	End   time.Time `json:"end"`
}
