// Package graphingest mutation handlers for triple operations via NATS request/reply.
package graphingest

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/c360/semstreams/graph"
)

const (
	// SubjectTripleAdd is the NATS subject for add triple requests
	SubjectTripleAdd = "graph.mutation.triple.add"
	// SubjectTripleRemove is the NATS subject for remove triple requests
	SubjectTripleRemove = "graph.mutation.triple.remove"
)

// setupMutationHandlers registers NATS request handlers for triple mutations.
// These handlers allow the rule processor (and other components) to modify
// entity triples via NATS request/reply.
func (c *Component) setupMutationHandlers(ctx context.Context) error {
	if err := c.natsClient.SubscribeForRequests(ctx, SubjectTripleAdd, c.handleTripleAdd); err != nil {
		return fmt.Errorf("subscribe triple add: %w", err)
	}

	if err := c.natsClient.SubscribeForRequests(ctx, SubjectTripleRemove, c.handleTripleRemove); err != nil {
		return fmt.Errorf("subscribe triple remove: %w", err)
	}

	c.logger.Info("mutation handlers registered",
		"subjects", []string{SubjectTripleAdd, SubjectTripleRemove})
	return nil
}

// handleTripleAdd handles add triple requests from rule processor and other components
func (c *Component) handleTripleAdd(ctx context.Context, data []byte) ([]byte, error) {
	var req graph.AddTripleRequest
	if err := json.Unmarshal(data, &req); err != nil {
		return json.Marshal(graph.AddTripleResponse{
			MutationResponse: graph.MutationResponse{
				Success:   false,
				Error:     fmt.Sprintf("invalid request: %v", err),
				Timestamp: time.Now().UnixNano(),
			},
		})
	}

	// AddTriple uses triple.Subject as entity ID
	err := c.AddTriple(ctx, req.Triple)
	if err != nil {
		return json.Marshal(graph.AddTripleResponse{
			MutationResponse: graph.MutationResponse{
				Success:   false,
				Error:     err.Error(),
				Timestamp: time.Now().UnixNano(),
			},
		})
	}

	return json.Marshal(graph.AddTripleResponse{
		MutationResponse: graph.MutationResponse{
			Success:   true,
			Timestamp: time.Now().UnixNano(),
		},
		Triple: &req.Triple,
	})
}

// handleTripleRemove handles remove triple requests from rule processor and other components
func (c *Component) handleTripleRemove(ctx context.Context, data []byte) ([]byte, error) {
	var req graph.RemoveTripleRequest
	if err := json.Unmarshal(data, &req); err != nil {
		return json.Marshal(graph.RemoveTripleResponse{
			MutationResponse: graph.MutationResponse{
				Success:   false,
				Error:     fmt.Sprintf("invalid request: %v", err),
				Timestamp: time.Now().UnixNano(),
			},
		})
	}

	// RemoveTriple takes subject (entity ID) and predicate
	err := c.RemoveTriple(ctx, req.Subject, req.Predicate)
	if err != nil {
		return json.Marshal(graph.RemoveTripleResponse{
			MutationResponse: graph.MutationResponse{
				Success:   false,
				Error:     err.Error(),
				Timestamp: time.Now().UnixNano(),
			},
		})
	}

	return json.Marshal(graph.RemoveTripleResponse{
		MutationResponse: graph.MutationResponse{
			Success:   true,
			Timestamp: time.Now().UnixNano(),
		},
		Removed: true,
	})
}
