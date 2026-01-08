// Package component defines the QueryCapabilityProvider interface and related types
package component

// IntentType classifies what kind of data the query targets.
type IntentType string

// Intent type constants define the primary classification of query operations.
const (
	IntentTypeEntity       IntentType = "entity"       // Entity CRUD operations
	IntentTypeRelationship IntentType = "relationship" // Graph traversal and relationship queries
	IntentTypeSpatial      IntentType = "spatial"      // Location/geo-based queries
	IntentTypeTemporal     IntentType = "temporal"     // Time-based queries
	IntentTypeSemantic     IntentType = "semantic"     // Similarity/embedding-based queries
	IntentTypeAggregate    IntentType = "aggregate"    // Aggregation and statistics
	IntentTypeAnomaly      IntentType = "anomaly"      // Anomaly detection queries
)

// IntentStrategy classifies the query execution approach.
type IntentStrategy string

// Intent strategy constants define how a query should be executed.
const (
	StrategyDirect IntentStrategy = "direct" // Single direct lookup
	StrategyBatch  IntentStrategy = "batch"  // Multiple independent lookups
	StrategyLocal  IntentStrategy = "local"  // Community-scoped search
	StrategyGlobal IntentStrategy = "global" // Cross-community search
	StrategyPath   IntentStrategy = "path"   // Graph traversal/pathfinding
)

// IntentScope classifies the result cardinality.
type IntentScope string

// Intent scope constants define the expected result cardinality.
const (
	ScopeSingle IntentScope = "single" // Returns 0-1 result
	ScopeSet    IntentScope = "set"    // Returns bounded collection
	ScopeStream IntentScope = "stream" // Returns unbounded/continuous results
)

// QueryIntent provides typed multi-dimensional query classification.
type QueryIntent struct {
	Type     IntentType     `json:"type"`
	Strategy IntentStrategy `json:"strategy"`
	Scope    IntentScope    `json:"scope"`
}

// Legacy intent tag constants - deprecated, use QueryIntent instead.
// Kept temporarily for backward compatibility during migration.
const (
	// IntentTagSpatial indicates location/geo queries (bounds, near, within).
	IntentTagSpatial = "spatial"

	// IntentTagTemporal indicates time-based queries (range, before, after).
	IntentTagTemporal = "temporal"

	// IntentTagSemantic indicates similarity/embedding queries (search, similar).
	IntentTagSemantic = "semantic"

	// IntentTagEntity indicates entity CRUD operations (get, list, batch).
	IntentTagEntity = "entity"

	// IntentTagRelationship indicates graph traversal queries (outgoing, incoming).
	IntentTagRelationship = "relationship"

	// IntentTagAggregate indicates aggregation/stats queries (count, hierarchy).
	IntentTagAggregate = "aggregate"

	// IntentTagAnomaly indicates anomaly detection queries (outliers, k-core).
	IntentTagAnomaly = "anomaly"

	// IntentTagAlias indicates alias resolution queries (resolveAlias).
	IntentTagAlias = "alias"

	// IntentTagPrefix indicates prefix/hierarchy queries (listByPrefix, hierarchyStats).
	IntentTagPrefix = "prefix"

	// IntentTagBatch indicates batch retrieval operations (getBatch, getMultiple).
	IntentTagBatch = "batch"
)

// QueryCapabilityProvider is an optional interface for components that
// expose query capabilities. Components implement this to provide rich
// schema information for query discovery.
//
// This interface enables dynamic discovery of query endpoints at runtime,
// allowing coordinators (like graph-query) to:
// - Discover available query operations across components
// - Route queries to appropriate handlers
// - Generate documentation automatically
// - Validate request/response schemas
//
// Components implementing this interface should:
// - Declare query endpoints via NATSRequestPort in InputPorts()
// - Implement QueryCapabilities() to describe those endpoints
// - Follow the single-owner pattern (query the data you write)
type QueryCapabilityProvider interface {
	// QueryCapabilities returns the component's query capabilities
	QueryCapabilities() QueryCapabilities
}

// QueryCapabilities describes a component's query endpoints with schema information.
// This type enables runtime discovery of query capabilities across components.
type QueryCapabilities struct {
	// Component is the name of the component (e.g., "graph-ingest", "graph-index")
	Component string `json:"component"`

	// Version is the schema version for this component's queries
	Version string `json:"version"`

	// Queries is the list of query operations this component exposes
	Queries []QueryCapability `json:"queries"`

	// Definitions contains shared type definitions referenced by query schemas.
	// Uses JSON Schema $ref syntax: {"$ref": "#/definitions/EntityState"}
	// Omitted from JSON when nil or empty.
	Definitions map[string]any `json:"definitions,omitempty"`
}

// QueryCapability describes a single query operation exposed by a component.
// Each capability corresponds to a NATS request/reply subject that handles
// the operation.
type QueryCapability struct {
	// Subject is the NATS subject for this query (e.g., "graph.ingest.query.entity")
	Subject string `json:"subject"`

	// Operation is the semantic operation name (e.g., "getEntity", "pathSearch")
	Operation string `json:"operation"`

	// Description is a human-readable description of what this query does.
	// Omitted from JSON when empty.
	Description string `json:"description,omitempty"`

	// RequestSchema is the JSON Schema for the request payload.
	// Can be a map[string]any or any JSON-serializable schema structure.
	RequestSchema any `json:"request_schema"`

	// ResponseSchema is the JSON Schema for the response payload.
	// Can be a map[string]any or any JSON-serializable schema structure.
	ResponseSchema any `json:"response_schema"`

	// Intent provides typed multi-dimensional query classification.
	// Replaces IntentTags with compile-time safe type system.
	Intent QueryIntent `json:"intent"`

	// IntentTags are semantic tags for intent-based routing.
	// DEPRECATED: Use Intent field instead. Kept for backward compatibility.
	// Standard tags: spatial, temporal, semantic, entity, relationship, aggregate.
	// Components use these to declare what KIND of queries they handle.
	// Omitted from JSON when empty.
	IntentTags []string `json:"intent_tags,omitempty"`

	// EntityTypes lists entity types this query operates on.
	// Use "*" for queries that handle all entity types.
	// Omitted from JSON when empty.
	EntityTypes []string `json:"entity_types,omitempty"`
}
