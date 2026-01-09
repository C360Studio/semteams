// Package graph provides query request/response contracts for the graph query system.
// These types are shared between handlers (producers) and clients (consumers)
// to ensure type safety and consistent API contracts.
package graph

import "time"

// QueryResponse is the standard envelope for all query responses.
// Uses generics for compile-time type safety.
type QueryResponse[T any] struct {
	Data      T         `json:"data"`
	Error     *string   `json:"error,omitempty"`
	RequestID string    `json:"request_id,omitempty"`
	Timestamp time.Time `json:"timestamp"`
}

// NewQueryResponse creates a successful response with the given data.
func NewQueryResponse[T any](data T) QueryResponse[T] {
	return QueryResponse[T]{
		Data:      data,
		Timestamp: time.Now(),
	}
}

// NewQueryError creates an error response with the given message.
func NewQueryError[T any](msg string) QueryResponse[T] {
	return QueryResponse[T]{
		Error:     &msg,
		Timestamp: time.Now(),
	}
}

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

// --- Type Aliases for Common Response Types ---

// OutgoingQueryResponse is the response type for outgoing relationship queries.
type OutgoingQueryResponse = QueryResponse[OutgoingRelationshipsData]

// IncomingQueryResponse is the response type for incoming relationship queries.
type IncomingQueryResponse = QueryResponse[IncomingRelationshipsData]

// AliasQueryResponse is the response type for alias resolution queries.
type AliasQueryResponse = QueryResponse[AliasData]

// PredicateQueryResponse is the response type for predicate queries.
type PredicateQueryResponse = QueryResponse[PredicateData]
