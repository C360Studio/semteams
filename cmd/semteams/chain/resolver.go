package chain

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/c360studio/semstreams/agentic"
	"github.com/c360studio/semstreams/natsclient"
	"github.com/c360studio/semstreams/types"
)

// maxAncestryHops bounds the chain ancestry walk so a malformed parent graph
// (cycle, runaway chain) cannot loop forever. In practice chains are
// 5–15 deep; 64 is a generous ceiling that still surfaces obvious bugs.
const maxAncestryHops = 64

// graphQueryTimeout caps each parent-read NATS request. Mirrors
// chainpause.NATSPauseDataReader's 3s budget.
const graphQueryTimeout = 3 * time.Second

// graphQueryEntitySubject is the request/reply NATS subject the graph
// component answers entity reads on. Source: graph processor's query
// surface, mirrored by chainpause/decision_handler.go's NATSPauseDataReader.
const graphQueryEntitySubject = "graph.query.entity"

// ParentReader reads the agent.loop.parent triple's object for a given
// loop ID. Returns the parent loop's full 6-part entity ID when present,
// or empty string when the loop has no parent (i.e. it is the chain
// root).
//
// The interface keeps Resolver testable without a live NATS client.
type ParentReader interface {
	ReadParent(ctx context.Context, loopID string) (parentEntityID string, err error)
}

// Resolver derives chain identity for a given loop by walking
// agent.loop.parent triples back to the chain root.
//
// agent.loop.parent is stamped by upstream graph_writer at every loop's
// completion (processor/agentic-loop/graph_writer.go buildLoopCompletionTriples
// — predicate agent.loop.parent, object = parent's 6-part entity ID).
// In rule-fanned chains every parent has completed before its child is
// spawned (rules fire on outcome events), so by the time a child loop
// completes the ancestry chain is fully stamped and one-hop-walkable.
//
// For loops that fail (LoopFailedEvent path), graph_writer's
// buildLoopFailureTriples ALSO stamps agent.loop.parent (semstreams
// beta.54), so chainpause-side resolves work the same way.
type Resolver struct {
	parents  ParentReader
	platform types.PlatformMeta
}

// NewResolver constructs a Resolver backed by the given ParentReader.
func NewResolver(parents ParentReader, platform types.PlatformMeta) *Resolver {
	return &Resolver{parents: parents, platform: platform}
}

// ChainID walks agent.loop.parent triples back to the chain root and
// returns the root loop's loop_id (= chain_id by ADR-038 D1). For a
// loop with no parent triple, returns the loop's own ID — that loop is
// the chain root.
//
// Bounded by maxAncestryHops; longer walks return an error so a malformed
// graph cannot wedge a caller.
func (r *Resolver) ChainID(ctx context.Context, loopID string) (string, error) {
	if loopID == "" {
		return "", fmt.Errorf("chain.Resolver.ChainID: loopID required")
	}
	cur := loopID
	for hops := 0; hops < maxAncestryHops; hops++ {
		parentEntityID, err := r.parents.ReadParent(ctx, cur)
		if err != nil {
			return "", fmt.Errorf("chain.Resolver.ChainID: read parent of %q at hop %d: %w", cur, hops, err)
		}
		if parentEntityID == "" {
			// cur has no parent — it is the chain root.
			return cur, nil
		}
		parentLoopID, ok := agentic.LoopIDFromExecutionEntityID(parentEntityID)
		if !ok {
			return "", fmt.Errorf("chain.Resolver.ChainID: parent of %q is malformed entity id %q", cur, parentEntityID)
		}
		cur = parentLoopID
	}
	return "", fmt.Errorf("chain.Resolver.ChainID: ancestry walk from %q exceeded %d hops (cycle?)", loopID, maxAncestryHops)
}

// ChainEntityID returns the canonical 6-part chain entity ID for the
// chain that contains loopID. Composes ChainID with
// agentic.ChainExecutionEntityID.
func (r *Resolver) ChainEntityID(ctx context.Context, loopID string) (string, error) {
	chainID, err := r.ChainID(ctx, loopID)
	if err != nil {
		return "", err
	}
	return agentic.ChainExecutionEntityID(r.platform.Org, r.platform.Platform, chainID), nil
}

// NATSParentReader reads agent.loop.parent via the graph component's
// graph.query.entity request/reply NATS surface.
//
// Returns ("", nil) when the entity has no agent.loop.parent triple —
// that's the chain-root signal, not an error. Network/decode failures
// propagate as errors.
type NATSParentReader struct {
	client   *natsclient.Client
	platform types.PlatformMeta
}

// NewNATSParentReader constructs a NATSParentReader bound to the given
// NATS client + platform identity.
func NewNATSParentReader(client *natsclient.Client, platform types.PlatformMeta) *NATSParentReader {
	return &NATSParentReader{client: client, platform: platform}
}

// ReadParent implements ParentReader against a live graph component.
func (r *NATSParentReader) ReadParent(ctx context.Context, loopID string) (string, error) {
	entityID := agentic.LoopExecutionEntityID(r.platform.Org, r.platform.Platform, loopID)

	req := map[string]string{"id": entityID}
	reqData, err := json.Marshal(req)
	if err != nil {
		return "", fmt.Errorf("marshal entity query: %w", err)
	}

	queryCtx, cancel := context.WithTimeout(ctx, graphQueryTimeout)
	defer cancel()

	respData, err := r.client.Request(queryCtx, graphQueryEntitySubject, reqData, graphQueryTimeout)
	if err != nil {
		return "", fmt.Errorf("graph entity query for %q: %w", entityID, err)
	}

	// Mirror chainpause/decision_handler.go NATSPauseDataReader's response
	// shape: a JSON object with id + triples[]. Only agent.loop.parent
	// matters for ancestry walking.
	var entity struct {
		ID      string `json:"id"`
		Triples []struct {
			Predicate string `json:"predicate"`
			Object    any    `json:"object"`
		} `json:"triples"`
	}
	if err := json.Unmarshal(respData, &entity); err != nil {
		return "", fmt.Errorf("decode entity response for %q: %w", entityID, err)
	}

	for _, t := range entity.Triples {
		if t.Predicate != "agent.loop.parent" {
			continue
		}
		// agent.loop.parent's object is a 6-part entity ID string.
		if s, ok := t.Object.(string); ok {
			return s, nil
		}
	}
	// No parent triple — caller treats this as chain root.
	return "", nil
}
