package chain

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
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
// terminal write — buildLoopCompletionTriples on success and
// buildLoopFailureTriples on failure (semstreams beta.56/.57). In
// rule-fanned chains every parent has completed before its child is
// spawned (rules fire on outcome events), so by the time a child loop
// completes OR fails, the ancestry chain is fully stamped and
// one-hop-walkable.
//
// The publish-vs-write race that previously affected just-completed-
// loop walks closed in beta.57: persistHandlerResult now runs
// WriteLoopCompletion / WriteLoopFailure under a 2s budget BEFORE
// publishResults, so any subscriber consuming agent.complete.* or
// agent.failed.* sees the full triple set the moment the event lands.
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
// the chain root. Example: ChainID(ctx, "<dispatch_uuid>") returns
// "<dispatch_uuid>" when the dispatch loop is the chain root (the
// most common chain-start case).
//
// Bounded by maxAncestryHops; longer walks return an error so a malformed
// graph cannot wedge a caller.
func (r *Resolver) ChainID(ctx context.Context, loopID string) (string, error) {
	if err := validateLoopID(loopID); err != nil {
		return "", fmt.Errorf("chain.Resolver.ChainID: %w", err)
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
		// Defensive: LoopIDFromExecutionEntityID enforces the entity
		// ID shape upstream, but we still validate the recovered loop_id
		// before passing it back into upstream constructors that panic
		// on dots/empties. A schema drift or graph corruption shouldn't
		// take down a subscriber goroutine.
		if err := validateLoopID(parentLoopID); err != nil {
			return "", fmt.Errorf("chain.Resolver.ChainID: parent of %q yields invalid loop_id %q: %w", cur, parentLoopID, err)
		}
		cur = parentLoopID
	}
	return "", fmt.Errorf("chain.Resolver.ChainID: ancestry walk from %q exceeded %d hops (cycle?)", loopID, maxAncestryHops)
}

// validateLoopID rejects shapes that would panic the upstream
// agentic.LoopExecutionEntityID / ChainExecutionEntityID constructors
// (empty, contains dot). Callers convert the returned error to the
// public "chain.Resolver.X" prefix.
func validateLoopID(loopID string) error {
	if loopID == "" {
		return fmt.Errorf("loopID required")
	}
	if strings.ContainsRune(loopID, '.') {
		return fmt.Errorf("loopID %q must not contain dots (entity ID separator)", loopID)
	}
	return nil
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

// EntityTripleReader returns a flat predicate→object map for a single
// graph entity. The shape is a thin convenience over the
// graph.query.entity request/reply: callers that want one or two
// predicates avoid bespoke JSON decoding at every site.
//
// Returns an empty map (not error) when the entity has no triples;
// callers can distinguish "absent predicate" from "graph error" by
// checking the map vs the error.
type EntityTripleReader interface {
	ReadEntity(ctx context.Context, entityID string) (map[string]any, error)
}

// NATSEntityReader reads any entity's triples via the graph component's
// graph.query.entity request/reply NATS surface. Mirrors the response
// shape used by chainpause/decision_handler.go NATSPauseDataReader.
type NATSEntityReader struct {
	client *natsclient.Client
}

// NewNATSEntityReader constructs an EntityTripleReader backed by the
// given NATS client.
func NewNATSEntityReader(client *natsclient.Client) *NATSEntityReader {
	return &NATSEntityReader{client: client}
}

// ReadEntity implements EntityTripleReader against a live graph
// component.
func (r *NATSEntityReader) ReadEntity(ctx context.Context, entityID string) (map[string]any, error) {
	req := map[string]string{"id": entityID}
	reqData, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("marshal entity query: %w", err)
	}

	queryCtx, cancel := context.WithTimeout(ctx, graphQueryTimeout)
	defer cancel()

	respData, err := r.client.Request(queryCtx, graphQueryEntitySubject, reqData, graphQueryTimeout)
	if err != nil {
		return nil, fmt.Errorf("graph entity query for %q: %w", entityID, err)
	}

	var entity struct {
		ID      string `json:"id"`
		Triples []struct {
			Predicate string `json:"predicate"`
			Object    any    `json:"object"`
		} `json:"triples"`
	}
	if err := json.Unmarshal(respData, &entity); err != nil {
		return nil, fmt.Errorf("decode entity response for %q: %w", entityID, err)
	}

	out := make(map[string]any, len(entity.Triples))
	for _, t := range entity.Triples {
		// Last-wins semantics: if a predicate appears twice the most
		// recent one wins, matching graph-ingest's stamping policy.
		out[t.Predicate] = t.Object
	}
	return out, nil
}

// NATSParentReader reads agent.loop.parent via the graph component's
// graph.query.entity request/reply NATS surface — a thin wrapper over
// NATSEntityReader that selects the parent predicate.
//
// Returns ("", nil) when the entity has no agent.loop.parent triple —
// that's the chain-root signal, not an error. Network/decode failures
// propagate as errors.
type NATSParentReader struct {
	entities *NATSEntityReader
	platform types.PlatformMeta
}

// NewNATSParentReader constructs a NATSParentReader bound to the given
// NATS client + platform identity.
func NewNATSParentReader(client *natsclient.Client, platform types.PlatformMeta) *NATSParentReader {
	return &NATSParentReader{entities: NewNATSEntityReader(client), platform: platform}
}

// ReadParent implements ParentReader against a live graph component.
func (r *NATSParentReader) ReadParent(ctx context.Context, loopID string) (string, error) {
	if err := validateLoopID(loopID); err != nil {
		return "", fmt.Errorf("chain.NATSParentReader.ReadParent: %w", err)
	}
	entityID := agentic.LoopExecutionEntityID(r.platform.Org, r.platform.Platform, loopID)
	triples, err := r.entities.ReadEntity(ctx, entityID)
	if err != nil {
		return "", err
	}
	// agent.loop.parent's object is a 6-part entity ID string.
	if s, ok := triples["agent.loop.parent"].(string); ok {
		return s, nil
	}
	// No parent triple — caller treats this as chain root.
	return "", nil
}
