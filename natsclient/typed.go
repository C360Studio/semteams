// Package natsclient provides typed subject patterns for compile-time type safety.
package natsclient

import (
	"context"
	"encoding/json"

	"github.com/nats-io/nats.go"
)

// Codec defines the serialization interface for typed subjects.
// Implementations handle marshaling and unmarshaling of typed payloads.
type Codec[T any] interface {
	Marshal(T) ([]byte, error)
	Unmarshal([]byte, *T) error
}

// JSONCodec provides JSON serialization for typed subjects.
// This is the default codec for most use cases.
type JSONCodec[T any] struct{}

// Marshal serializes a value to JSON bytes.
func (c JSONCodec[T]) Marshal(v T) ([]byte, error) {
	return json.Marshal(v)
}

// Unmarshal deserializes JSON bytes into a value.
func (c JSONCodec[T]) Unmarshal(data []byte, v *T) error {
	return json.Unmarshal(data, v)
}

// Subject represents a typed NATS subject with compile-time type safety.
// It binds a subject pattern to a specific payload type and codec.
//
// Example:
//
//	var WorkflowStarted = Subject[WorkflowStartedEvent]{
//	    Pattern: "workflow.events.started",
//	    Codec:   JSONCodec[WorkflowStartedEvent]{},
//	}
//
//	// Type-safe publish
//	err := WorkflowStarted.Publish(ctx, client, event)
//
//	// Type-safe subscribe
//	sub, err := WorkflowStarted.Subscribe(ctx, client, func(ctx context.Context, event WorkflowStartedEvent) error {
//	    // event is already typed - no assertions needed
//	    return nil
//	})
type Subject[T any] struct {
	// Pattern is the NATS subject pattern (may include wildcards for subscribe)
	Pattern string

	// Codec handles serialization/deserialization
	Codec Codec[T]
}

// Publish sends a typed payload to the subject.
// The payload is serialized using the subject's codec before publishing.
func (s Subject[T]) Publish(ctx context.Context, client *Client, payload T) error {
	data, err := s.Codec.Marshal(payload)
	if err != nil {
		return err
	}
	return client.Publish(ctx, s.Pattern, data)
}

// PublishToStream sends a typed payload to a JetStream subject.
// The payload is serialized using the subject's codec before publishing.
func (s Subject[T]) PublishToStream(ctx context.Context, client *Client, payload T) error {
	data, err := s.Codec.Marshal(payload)
	if err != nil {
		return err
	}
	return client.PublishToStream(ctx, s.Pattern, data)
}

// Subscribe creates a subscription to the subject with type-safe message handling.
// The handler receives deserialized payloads directly.
func (s Subject[T]) Subscribe(ctx context.Context, client *Client, handler func(context.Context, T) error) (*Subscription, error) {
	return client.Subscribe(ctx, s.Pattern, func(ctx context.Context, msg *nats.Msg) {
		var payload T
		if err := s.Codec.Unmarshal(msg.Data, &payload); err != nil {
			// Log error but continue processing other messages
			return
		}
		_ = handler(ctx, payload)
	})
}

// SubscribeWithMsg creates a subscription that provides both the typed payload and raw message.
// Use this when you need access to message metadata (subject, headers, etc.).
func (s Subject[T]) SubscribeWithMsg(ctx context.Context, client *Client, handler func(context.Context, *nats.Msg, T) error) (*Subscription, error) {
	return client.Subscribe(ctx, s.Pattern, func(ctx context.Context, msg *nats.Msg) {
		var payload T
		if err := s.Codec.Unmarshal(msg.Data, &payload); err != nil {
			return
		}
		_ = handler(ctx, msg, payload)
	})
}

// NewSubject creates a typed subject with a JSON codec (most common case).
func NewSubject[T any](pattern string) Subject[T] {
	return Subject[T]{
		Pattern: pattern,
		Codec:   JSONCodec[T]{},
	}
}

// NewSubjectWithCodec creates a typed subject with a custom codec.
func NewSubjectWithCodec[T any](pattern string, codec Codec[T]) Subject[T] {
	return Subject[T]{
		Pattern: pattern,
		Codec:   codec,
	}
}
