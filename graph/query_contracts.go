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
