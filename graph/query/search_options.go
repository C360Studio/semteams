package query

import "time"

// SearchStrategy represents the type of search to perform.
type SearchStrategy string

// Search strategy constants for query routing.
const (
	StrategyGraphRAG         SearchStrategy = "graphrag"
	StrategyGeoGraphRAG      SearchStrategy = "geo_graphrag"
	StrategyTemporalGraphRAG SearchStrategy = "temporal_graphrag"
	StrategyHybridGraphRAG   SearchStrategy = "hybrid_graphrag"
	StrategyPathRAG          SearchStrategy = "pathrag"
	StrategySemantic         SearchStrategy = "semantic"
	StrategyExact            SearchStrategy = "exact"
	StrategyAggregation      SearchStrategy = "aggregation"
)

// Aggregation type constants for aggregation queries.
const (
	AggregationCount = "count"
	AggregationAvg   = "avg"
	AggregationSum   = "sum"
	AggregationMin   = "min"
	AggregationMax   = "max"
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
	PathIntent        bool           `json:"path_intent,omitempty"`
	PathStartNode     string         `json:"path_start_node,omitempty"`
	PathPredicates    []string       `json:"path_predicates,omitempty"`
	// AggregationType specifies the type of aggregation (count, avg, sum, min, max).
	// Use the AggregationCount / AggregationAvg / … constants.
	AggregationType string `json:"aggregation_type,omitempty"`
	// AggregationField is the property or attribute to aggregate on (e.g. "temperature").
	// Empty means aggregate over entity count.
	AggregationField string `json:"aggregation_field,omitempty"`
	// RankingIntent is true when the query requests a ranked/top-N result set.
	RankingIntent bool `json:"ranking_intent,omitempty"`
}

// TimeRange represents temporal query bounds.
type TimeRange struct {
	Start time.Time `json:"start"`
	End   time.Time `json:"end"`
}

// SpatialBounds represents geographic bounding box.
type SpatialBounds struct {
	North float64 `json:"north"`
	South float64 `json:"south"`
	East  float64 `json:"east"`
	West  float64 `json:"west"`
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
	hasPath := o.PathIntent && o.PathStartNode != ""
	hasAnyFilter := hasGeo || hasTemporal || hasPredicates || hasTypes

	// Aggregation intent always routes to the dedicated aggregation strategy.
	// Combined filters (e.g. temporal + count) still route here so the
	// aggregation executor can apply them.
	if o.AggregationType != "" {
		return StrategyAggregation
	}

	// Path intent with extractable entity routes to PathRAG
	if hasPath {
		// If combined with temporal, use hybrid strategy
		if hasTemporal {
			return StrategyHybridGraphRAG
		}
		return StrategyPathRAG
	}

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
		len(o.Types) > 0 ||
		o.AggregationType != ""
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
