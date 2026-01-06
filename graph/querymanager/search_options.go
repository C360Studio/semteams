package querymanager

import "time"

// SearchStrategy represents the type of search to perform.
// This can be explicitly set or auto-inferred from SearchOptions.
type SearchStrategy string

const (
	// StrategyGraphRAG performs GraphRAG community-based search with keyword matching.
	// This is the default when only a text query is provided.
	StrategyGraphRAG SearchStrategy = "graphrag"

	// StrategyGeoGraphRAG performs spatial pre-filtering before GraphRAG search.
	// Used when GeoBounds is provided along with a text query.
	StrategyGeoGraphRAG SearchStrategy = "geo_graphrag"

	// StrategyTemporalGraphRAG performs temporal pre-filtering before GraphRAG search.
	// Used when TimeRange is provided along with a text query.
	StrategyTemporalGraphRAG SearchStrategy = "temporal_graphrag"

	// StrategyHybridGraphRAG combines multiple index pre-filters before GraphRAG search.
	// Used when multiple filter types are provided.
	StrategyHybridGraphRAG SearchStrategy = "hybrid_graphrag"

	// StrategySemantic uses embedding similarity for filtering instead of keywords.
	// Requires embedding infrastructure to be available.
	StrategySemantic SearchStrategy = "semantic"

	// StrategyExact performs exact match search without community context.
	// Used when no text query is provided but filters are specified.
	StrategyExact SearchStrategy = "exact"
)

// SearchOptions provides declarative search configuration with optional index pre-filtering.
// Supports progressive enhancement - unavailable indexes are gracefully skipped.
type SearchOptions struct {
	// Query is the text query for semantic/keyword matching.
	// If empty and filters are present, performs exact index lookup.
	Query string `json:"query,omitempty"`

	// Index filters (optional - can be combined)

	// GeoBounds filters entities by spatial index.
	// Entities must fall within the bounding box.
	GeoBounds *SpatialBounds `json:"geo_bounds,omitempty"`

	// TimeRange filters entities by temporal index.
	// Entities must have timestamps within the range.
	TimeRange *TimeRange `json:"time_range,omitempty"`

	// Predicates filters by predicate index.
	// Entities must have at least one of the specified predicates.
	Predicates []string `json:"predicates,omitempty"`

	// Types filters entities by their type (extracted from entity ID).
	Types []string `json:"types,omitempty"`

	// Behavior configuration

	// RequireAllFilters determines how multiple index filters are combined.
	// true = AND (entity must match all filters)
	// false = OR (entity must match at least one filter)
	// Default: false
	RequireAllFilters bool `json:"require_all_filters,omitempty"`

	// UseEmbeddings enables semantic similarity filtering when available.
	// Falls back to keyword matching if embeddings unavailable.
	// Default: false
	UseEmbeddings bool `json:"use_embeddings,omitempty"`

	// Strategy explicitly sets the search strategy.
	// If empty, strategy is auto-inferred from the options.
	Strategy SearchStrategy `json:"strategy,omitempty"`

	// Result limits

	// Limit is the maximum number of entities to return.
	// Default: 100
	Limit int `json:"limit,omitempty"`

	// Level is the community hierarchy level for GraphRAG search.
	// Level 0 = finest granularity, higher = broader communities.
	// Default: 0
	Level int `json:"level,omitempty"`

	// MaxCommunities limits the number of communities to search.
	// Default: DefaultMaxCommunities (5)
	MaxCommunities int `json:"max_communities,omitempty"`
}

// TimeRange represents temporal query bounds.
// Uses time.Time for precision and timezone support.
type TimeRange struct {
	Start time.Time `json:"start"`
	End   time.Time `json:"end"`
}

// InferStrategy determines the best search strategy based on options.
// Called automatically if Strategy is not explicitly set.
func (o *SearchOptions) InferStrategy() SearchStrategy {
	// If strategy is explicitly set, use it
	if o.Strategy != "" {
		return o.Strategy
	}

	hasGeo := o.GeoBounds != nil
	hasTemporal := o.TimeRange != nil
	hasPredicates := len(o.Predicates) > 0
	hasTypes := len(o.Types) > 0
	hasText := o.Query != ""
	hasAnyFilter := hasGeo || hasTemporal || hasPredicates || hasTypes

	// Semantic strategy when explicitly requested
	if o.UseEmbeddings && hasText {
		if hasAnyFilter {
			return StrategyHybridGraphRAG
		}
		return StrategySemantic
	}

	// Multiple filter types = hybrid
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

	// Single filter type + text
	if hasGeo && hasText {
		return StrategyGeoGraphRAG
	}
	if hasTemporal && hasText {
		return StrategyTemporalGraphRAG
	}

	// Text only = standard GraphRAG
	if hasText {
		return StrategyGraphRAG
	}

	// Filters only = exact lookup
	if hasAnyFilter {
		return StrategyExact
	}

	// Nothing specified - default to GraphRAG (will require query)
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

// Validate checks that the options are valid.
func (o *SearchOptions) Validate() error {
	strategy := o.InferStrategy()

	// Most strategies require a query
	if strategy != StrategyExact && o.Query == "" {
		return ErrQueryRequired
	}

	// TimeRange validation
	if o.TimeRange != nil {
		if o.TimeRange.Start.IsZero() || o.TimeRange.End.IsZero() {
			return ErrInvalidTimeRange
		}
		if o.TimeRange.End.Before(o.TimeRange.Start) {
			return ErrInvalidTimeRange
		}
	}

	// GeoBounds validation
	if o.GeoBounds != nil {
		if o.GeoBounds.North <= o.GeoBounds.South {
			return ErrInvalidGeoBounds
		}
		if o.GeoBounds.North > 90 || o.GeoBounds.South < -90 {
			return ErrInvalidGeoBounds
		}
		// Note: East/West can wrap around 180/-180
	}

	return nil
}
