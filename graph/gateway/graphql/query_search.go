// Package graphql provides GraphQL gateway types and query definitions.
package graphql

import "time"

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

// Search strategy constants for query routing.
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
