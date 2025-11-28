// Package graph provides types for NATS mutation API
package graph

import (
	"time"

	"github.com/c360/semstreams/message"
)

// Mutation Response Types

// MutationResponse is the base response for all mutations
type MutationResponse struct {
	Success   bool   `json:"success"`
	Error     string `json:"error,omitempty"`
	TraceID   string `json:"trace_id,omitempty"`
	RequestID string `json:"request_id,omitempty"`
	Timestamp int64  `json:"timestamp"` // Unix nano timestamp
}

// CreateEntityResponse response for entity creation
type CreateEntityResponse struct {
	MutationResponse
	Entity *EntityState `json:"entity,omitempty"`
}

// UpdateEntityResponse response for entity update
type UpdateEntityResponse struct {
	MutationResponse
	Entity  *EntityState `json:"entity,omitempty"`
	Version int64        `json:"version,omitempty"`
}

// DeleteEntityResponse response for entity deletion
type DeleteEntityResponse struct {
	MutationResponse
	Deleted bool `json:"deleted"`
}

// CreateEntityWithTriplesResponse response for atomic entity+triples creation
type CreateEntityWithTriplesResponse struct {
	MutationResponse
	Entity       *EntityState `json:"entity,omitempty"`
	TriplesAdded int          `json:"triples_added"`
}

// UpdateEntityWithTriplesResponse response for atomic entity+triples update
type UpdateEntityWithTriplesResponse struct {
	MutationResponse
	Entity         *EntityState `json:"entity,omitempty"`
	TriplesAdded   int          `json:"triples_added"`
	TriplesRemoved int          `json:"triples_removed"`
	Version        int64        `json:"version,omitempty"`
}

// AddTripleResponse response for triple addition
type AddTripleResponse struct {
	MutationResponse
	Triple *message.Triple `json:"triple,omitempty"`
}

// RemoveTripleResponse response for triple removal
type RemoveTripleResponse struct {
	MutationResponse
	Removed bool `json:"removed"`
}

// Helper functions

// NewMutationResponse creates a base mutation response
func NewMutationResponse(success bool, err error, traceID, requestID string) MutationResponse {
	resp := MutationResponse{
		Success:   success,
		TraceID:   traceID,
		RequestID: requestID,
		Timestamp: time.Now().UnixNano(),
	}
	if err != nil {
		resp.Error = err.Error()
	}
	return resp
}
