// Package rule provides triple mutation support via NATS request/response
package rule

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	gtypes "github.com/c360/semstreams/graph"
	"github.com/c360/semstreams/message"
	"github.com/c360/semstreams/natsclient"
)

// NATS subjects for graph mutations (must match processor/graph/mutations.go)
const (
	SubjectTripleAdd    = "graph.mutation.triple.add"
	SubjectTripleRemove = "graph.mutation.triple.remove"
	MutationTimeout     = 5 * time.Second
)

// tripleMutator implements TripleMutator using NATS request/response.
// It calls the graph processor's mutation handlers and tracks KV revisions
// to prevent feedback loops in rule evaluation.
type tripleMutator struct {
	natsClient      *natsclient.Client
	revisionTracker revisionTracker
}

// revisionTracker is the interface for tracking KV revisions we generate.
// This is implemented by the Processor to break feedback loops.
type revisionTracker interface {
	trackOwnRevision(entityID string, revision uint64)
}

// newTripleMutator creates a new TripleMutator that uses NATS request/response.
func newTripleMutator(natsClient *natsclient.Client, tracker revisionTracker) TripleMutator {
	return &tripleMutator{
		natsClient:      natsClient,
		revisionTracker: tracker,
	}
}

// AddTriple adds a triple via NATS request/response and returns the KV revision.
// It tracks the revision to prevent re-evaluation of our own writes.
func (m *tripleMutator) AddTriple(ctx context.Context, triple message.Triple) (uint64, error) {
	if m.natsClient == nil {
		return 0, fmt.Errorf("NATS client not available")
	}

	// Build request
	req := gtypes.AddTripleRequest{
		Triple: triple,
	}
	reqData, err := json.Marshal(req)
	if err != nil {
		return 0, fmt.Errorf("marshal request: %w", err)
	}

	// Make NATS request
	respData, err := m.natsClient.Request(ctx, SubjectTripleAdd, reqData, MutationTimeout)
	if err != nil {
		return 0, fmt.Errorf("NATS request failed: %w", err)
	}

	// Parse response
	var resp gtypes.AddTripleResponse
	if err := json.Unmarshal(respData, &resp); err != nil {
		return 0, fmt.Errorf("unmarshal response: %w", err)
	}

	if !resp.Success {
		return 0, fmt.Errorf("mutation failed: %s", resp.Error)
	}

	// Track the revision to prevent feedback loop
	if m.revisionTracker != nil && resp.KVRevision > 0 {
		m.revisionTracker.trackOwnRevision(triple.Subject, resp.KVRevision)
	}

	return resp.KVRevision, nil
}

// RemoveTriple removes a triple via NATS request/response and returns the KV revision.
// It tracks the revision to prevent re-evaluation of our own writes.
func (m *tripleMutator) RemoveTriple(ctx context.Context, subject, predicate string) (uint64, error) {
	if m.natsClient == nil {
		return 0, fmt.Errorf("NATS client not available")
	}

	// Build request
	req := gtypes.RemoveTripleRequest{
		Subject:   subject,
		Predicate: predicate,
	}
	reqData, err := json.Marshal(req)
	if err != nil {
		return 0, fmt.Errorf("marshal request: %w", err)
	}

	// Make NATS request
	respData, err := m.natsClient.Request(ctx, SubjectTripleRemove, reqData, MutationTimeout)
	if err != nil {
		return 0, fmt.Errorf("NATS request failed: %w", err)
	}

	// Parse response
	var resp gtypes.RemoveTripleResponse
	if err := json.Unmarshal(respData, &resp); err != nil {
		return 0, fmt.Errorf("unmarshal response: %w", err)
	}

	if !resp.Success {
		return 0, fmt.Errorf("mutation failed: %s", resp.Error)
	}

	// Track the revision to prevent feedback loop
	if m.revisionTracker != nil && resp.KVRevision > 0 {
		m.revisionTracker.trackOwnRevision(subject, resp.KVRevision)
	}

	return resp.KVRevision, nil
}
