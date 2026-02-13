// Package graph provides index query data types for the graph query system.
package graph

// --- Index Query Data Types ---

// OutgoingEntry represents a single outgoing relationship in the graph.
type OutgoingEntry struct {
	ToEntityID string `json:"to_entity_id"`
	Predicate  string `json:"predicate"`
}

// IncomingEntry represents a single incoming relationship in the graph.
type IncomingEntry struct {
	FromEntityID string `json:"from_entity_id"`
	Predicate    string `json:"predicate"`
}

// ContextEntry represents an entity-predicate pair indexed by context.
type ContextEntry struct {
	EntityID  string `json:"entity_id"`
	Predicate string `json:"predicate"`
}

// PredicateIndexEntry represents entities that have a specific predicate.
type PredicateIndexEntry struct {
	Entities  []string `json:"entities"`
	Predicate string   `json:"predicate"`
	EntityID  string   `json:"entity_id,omitempty"` // backward compat
}

// --- Index Query Response Payloads ---

// OutgoingRelationshipsData contains outgoing relationships for an entity.
type OutgoingRelationshipsData struct {
	Relationships []OutgoingEntry `json:"relationships"`
}

// IncomingRelationshipsData contains incoming relationships for an entity.
type IncomingRelationshipsData struct {
	Relationships []IncomingEntry `json:"relationships"`
}

// AliasData contains the canonical entity ID for an alias lookup.
type AliasData struct {
	CanonicalID *string `json:"canonical_id"` // nil if not found
}

// PredicateData contains entities that have a specific predicate.
type PredicateData struct {
	Entities []string `json:"entities"`
}

// --- Predicate Query Types ---

// PredicateSummary represents a predicate with its entity count.
type PredicateSummary struct {
	Predicate   string `json:"predicate"`
	EntityCount int    `json:"entity_count"`
}

// PredicateListData contains all predicates with their entity counts.
type PredicateListData struct {
	Predicates []PredicateSummary `json:"predicates"`
	Total      int                `json:"total"`
}

// PredicateStatsData contains detailed statistics for a single predicate.
type PredicateStatsData struct {
	Predicate      string   `json:"predicate"`
	EntityCount    int      `json:"entity_count"`
	SampleEntities []string `json:"sample_entities"`
}

// CompoundPredicateQuery represents a query combining multiple predicates.
type CompoundPredicateQuery struct {
	Predicates []string `json:"predicates"`
	Operator   string   `json:"operator"` // "AND" or "OR"
	Limit      int      `json:"limit,omitempty"`
}

// CompoundPredicateData contains entities matching a compound predicate query.
type CompoundPredicateData struct {
	Entities []string `json:"entities"`
	Operator string   `json:"operator"`
	Matched  int      `json:"matched"`
}
