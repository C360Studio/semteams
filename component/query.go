// Package component defines the QueryCapabilityProvider interface and related types
package component

// Standard intent tags for query capability classification.
// Use these constants when declaring IntentTags on QueryCapability.
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

	// IntentTags are semantic tags for intent-based routing.
	// Standard tags: spatial, temporal, semantic, entity, relationship, aggregate.
	// Components use these to declare what KIND of queries they handle.
	// Omitted from JSON when empty.
	IntentTags []string `json:"intent_tags,omitempty"`

	// EntityTypes lists entity types this query operates on.
	// Use "*" for queries that handle all entity types.
	// Omitted from JSON when empty.
	EntityTypes []string `json:"entity_types,omitempty"`
}
