// Package graph provides types for NATS mutation API
package graph

import "github.com/c360/semstreams/message"

// Mutation Request Types

// CreateEntityRequest creates a new entity
type CreateEntityRequest struct {
	Entity    *EntityState `json:"entity"`
	TraceID   string       `json:"trace_id,omitempty"`
	RequestID string       `json:"request_id,omitempty"`
}

// UpdateEntityRequest updates an existing entity
type UpdateEntityRequest struct {
	Entity    *EntityState `json:"entity"`
	TraceID   string       `json:"trace_id,omitempty"`
	RequestID string       `json:"request_id,omitempty"`
}

// DeleteEntityRequest deletes an entity
type DeleteEntityRequest struct {
	EntityID  string `json:"entity_id"`
	TraceID   string `json:"trace_id,omitempty"`
	RequestID string `json:"request_id,omitempty"`
}

// CreateEntityWithTriplesRequest creates entity with triples atomically
type CreateEntityWithTriplesRequest struct {
	Entity    *EntityState     `json:"entity"`
	Triples   []message.Triple `json:"triples"`
	TraceID   string           `json:"trace_id,omitempty"`
	RequestID string           `json:"request_id,omitempty"`
}

// UpdateEntityWithTriplesRequest updates entity and modifies triples atomically
type UpdateEntityWithTriplesRequest struct {
	Entity        *EntityState     `json:"entity"`
	AddTriples    []message.Triple `json:"add_triples,omitempty"`
	RemoveTriples []string         `json:"remove_triples,omitempty"` // Triple predicates to remove
	TraceID       string           `json:"trace_id,omitempty"`
	RequestID     string           `json:"request_id,omitempty"`
}

// AddTripleRequest adds a triple to an existing entity
type AddTripleRequest struct {
	Triple    message.Triple `json:"triple"`
	TraceID   string         `json:"trace_id,omitempty"`
	RequestID string         `json:"request_id,omitempty"`
}

// RemoveTripleRequest removes a triple from an entity
type RemoveTripleRequest struct {
	Subject   string `json:"subject"`
	Predicate string `json:"predicate"`
	TraceID   string `json:"trace_id,omitempty"`
	RequestID string `json:"request_id,omitempty"`
}
