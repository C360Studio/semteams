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

// QMRelationship represents a relationship between entities (internal query type).
type QMRelationship struct {
	FromEntityID string                 `json:"from_entity_id"`
	ToEntityID   string                 `json:"to_entity_id"`
	EdgeType     string                 `json:"edge_type"`
	Properties   map[string]interface{} `json:"properties"`
	Weight       float64                `json:"weight"`
	CreatedAt    time.Time              `json:"created_at"`
	UpdatedAt    time.Time              `json:"updated_at"`
}

// Truncation reason constants for QueryResult.
const (
	TruncationReasonTimeout   = "timeout"
	TruncationReasonCancelled = "cancelled"
	TruncationReasonMaxNodes  = "max_nodes"
)

// QueryResult represents the result of a path query.
type QueryResult struct {
	Entities         []*gtypes.EntityState `json:"entities"`
	Paths            []GraphPath           `json:"paths"`
	Count            int                   `json:"count"`
	Duration         time.Duration         `json:"duration"`
	Cached           bool                  `json:"cached"`
	CacheLayer       string                `json:"cache_layer,omitempty"`
	Error            error                 `json:"error,omitempty"`
	Truncated        bool                  `json:"truncated,omitempty"`
	TruncationReason string                `json:"truncation_reason,omitempty"`
	Scores           map[string]float64    `json:"scores,omitempty"`
}

// QMGraphSnapshot represents a snapshot of entities within bounds (internal query type).
type QMGraphSnapshot struct {
	Entities      []*gtypes.EntityState `json:"entities"`
	Relationships []QMRelationship      `json:"relationships"`
	Bounds        QueryBounds           `json:"bounds"`
	Timestamp     time.Time             `json:"timestamp"`
	Count         int                   `json:"count"`
	Truncated     bool                  `json:"truncated"`
}

// GraphPath represents a path through the entity graph.
type GraphPath struct {
	Entities []string    `json:"entities"`
	Edges    []GraphEdge `json:"edges"`
	Length   int         `json:"length"`
	Weight   float64     `json:"weight"`
}

// GraphEdge represents an edge in a graph path.
type GraphEdge struct {
	From       string                 `json:"from"`
	To         string                 `json:"to"`
	EdgeType   string                 `json:"edge_type"`
	Properties map[string]interface{} `json:"properties"`
	Weight     float64                `json:"weight"`
}

// QMLocalSearchResult represents the result of a local community search (internal query type).
type QMLocalSearchResult struct {
	Entities    []*gtypes.EntityState `json:"entities"`
	CommunityID string                `json:"community_id"`
	Count       int                   `json:"count"`
	Duration    time.Duration         `json:"duration"`
}

// QMGlobalSearchResult represents the result of a global cross-community search (internal query type).
type QMGlobalSearchResult struct {
	Entities           []*gtypes.EntityState `json:"entities"`
	CommunitySummaries []CommunitySummary    `json:"community_summaries"`
	Count              int                   `json:"count"`
	Duration           time.Duration         `json:"duration"`
	Answer             string                `json:"answer,omitempty"`
	AnswerModel        string                `json:"answer_model,omitempty"`
}

// QMSimilaritySearchResult represents the results of a similarity search query (internal query type).
type QMSimilaritySearchResult struct {
	Hits  []QMSimilarityHit `json:"hits"`
	Total int               `json:"total"`
}

// QMSimilarityHit represents a single similarity search result (internal query type).
type QMSimilarityHit struct {
	EntityID string  `json:"entity_id"`
	Score    float64 `json:"score"`
}

// CacheStats represents cache statistics.
type CacheStats struct {
	L1Hits      int64   `json:"l1_hits"`
	L1Misses    int64   `json:"l1_misses"`
	L1Size      int     `json:"l1_size"`
	L1HitRatio  float64 `json:"l1_hit_ratio"`
	L1Evictions int64   `json:"l1_evictions"`
	L2Hits      int64   `json:"l2_hits"`
	L2Misses    int64   `json:"l2_misses"`
	L2Size      int     `json:"l2_size"`
	L2HitRatio  float64 `json:"l2_hit_ratio"`
	L3Hits      int64   `json:"l3_hits"`
	L3Misses    int64   `json:"l3_misses"`
}

// SearchStrategy represents the type of search to perform.
type SearchStrategy string

const (
	StrategyGraphRAG         SearchStrategy = "graphrag"
	StrategyGeoGraphRAG      SearchStrategy = "geo_graphrag"
	StrategyTemporalGraphRAG SearchStrategy = "temporal_graphrag"
	StrategyHybridGraphRAG   SearchStrategy = "hybrid_graphrag"
	StrategySemantic         SearchStrategy = "semantic"
	StrategyExact            SearchStrategy = "exact"
)

// DefaultMaxCommunities is the default number of communities to search.
const DefaultMaxCommunities = 5

// SearchOptions provides declarative search configuration.
type SearchOptions struct {
	Query             string         `json:"query,omitempty"`
	GeoBounds         *SpatialBounds `json:"geo_bounds,omitempty"`
	TimeRange         *TimeRange     `json:"time_range,omitempty"`
	Predicates        []string       `json:"predicates,omitempty"`
	Types             []string       `json:"types,omitempty"`
	RequireAllFilters bool           `json:"require_all_filters,omitempty"`
	UseEmbeddings     bool           `json:"use_embeddings,omitempty"`
	Strategy          SearchStrategy `json:"strategy,omitempty"`
	Limit             int            `json:"limit,omitempty"`
	Level             int            `json:"level,omitempty"`
	MaxCommunities    int            `json:"max_communities,omitempty"`
}

// TimeRange represents temporal query bounds.
type TimeRange struct {
	Start time.Time `json:"start"`
	End   time.Time `json:"end"`
}

// InferStrategy determines the best search strategy based on options.
func (o *SearchOptions) InferStrategy() SearchStrategy {
	if o.Strategy != "" {
		return o.Strategy
	}

	hasGeo := o.GeoBounds != nil
	hasTemporal := o.TimeRange != nil
	hasPredicates := len(o.Predicates) > 0
	hasTypes := len(o.Types) > 0
	hasText := o.Query != ""
	hasAnyFilter := hasGeo || hasTemporal || hasPredicates || hasTypes

	if o.UseEmbeddings && hasText {
		if hasAnyFilter {
			return StrategyHybridGraphRAG
		}
		return StrategySemantic
	}

	filterCount := 0
	if hasGeo {
		filterCount++
	}
	if hasTemporal {
		filterCount++
	}
	if hasPredicates {
		filterCount++
	}
	if hasTypes {
		filterCount++
	}

	if filterCount > 1 && hasText {
		return StrategyHybridGraphRAG
	}

	if hasGeo && hasText {
		return StrategyGeoGraphRAG
	}
	if hasTemporal && hasText {
		return StrategyTemporalGraphRAG
	}

	if hasText {
		return StrategyGraphRAG
	}

	if hasAnyFilter {
		return StrategyExact
	}

	return StrategyGraphRAG
}

// HasIndexFilters returns true if any index filters are specified.
func (o *SearchOptions) HasIndexFilters() bool {
	return o.GeoBounds != nil ||
		o.TimeRange != nil ||
		len(o.Predicates) > 0 ||
		len(o.Types) > 0
}

// SetDefaults applies default values for unset options.
func (o *SearchOptions) SetDefaults() {
	if o.Limit <= 0 {
		o.Limit = 100
	}
	if o.MaxCommunities <= 0 {
		o.MaxCommunities = DefaultMaxCommunities
	}
}
