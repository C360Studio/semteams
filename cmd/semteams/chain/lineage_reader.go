package chain

import (
	"context"
	"fmt"

	"github.com/c360studio/semstreams/agentic"
)

// LineageReader composes ChainEntityID resolution + entity-triple
// reading into a single ReadChainFor method. Implements the
// ChainReader interface declared (structurally) by the emit-tool
// packages (cmd/semteams/tools/emitplan, .../emitspecartifact) —
// Go's structural interfaces let a single adapter satisfy both with
// no shared interface declaration.
//
// Wiring at boot: each emit-tool's product_tools.go registration
// constructs a LineageReader from the same NATSEntityReader +
// Resolver pair and passes it via SetChainReader. Smoke #8 run-5 D1
// + D2 fix needs this read surface so the emit-tools can override
// LLM-supplied lineage IDs and slug stems with chain-canonical
// values stamped earlier in the chain.
//
// Failure modes (both surface as a non-nil err with chainEntityID
// possibly empty):
//
//   - resolver ancestry walk fails (graph KV unreachable, malformed
//     parent chain, walk hits maxAncestryHops cap).
//   - entity read fails (graph-query timeout, decode mismatch, the
//     entity simply doesn't exist yet — chain entity is stamped on
//     dispatch, so absence implies a serious wiring bug).
//
// Callers are expected to fail soft on err — log a Warn and fall
// through to whatever LLM-supplied values they had. See each
// emit-tool's readChainTriples helper for the canonical pattern.
type LineageReader struct {
	resolver *Resolver
	entities EntityTripleReader
}

// NewLineageReader constructs a LineageReader. resolver derives the
// 6-part chain entity ID from any loop in the chain; entities reads
// the triple set off that entity. Both must be non-nil.
//
// Production wiring composes these with NATS-backed implementations
// (NewResolver(NewNATSParentReader(...)) and NewNATSEntityReader);
// tests can pass any structurally-compatible pair.
func NewLineageReader(resolver *Resolver, entities EntityTripleReader) *LineageReader {
	return &LineageReader{resolver: resolver, entities: entities}
}

// ReadChainFor walks the ancestry from fromLoopID to the chain root,
// composes the chain entity ID, and reads its triple set. Returns
// (chainEntityID, triples, nil) on success; (chainEntityID, nil, err)
// when the entity read failed AFTER the resolver succeeded; and
// ("", nil, err) when the resolver itself failed.
//
// IMPORTANT: fromLoopID MUST be a loop whose `agent.loop.parent`
// triple has been stamped by graph_writer — i.e. a COMPLETED ancestor
// in the chain. Walking from a still-running loop's ID returns that
// loop's own ID as the chain root (because the missing parent triple
// is read as "no parent → I am root"), then attempts to read a
// chain entity at that wrong ID and fails decode. Smoke #8 run-6
// surfaced this when emit-tools called with their own (running)
// loop IDs; the fix is to call from a known-completed ancestor —
// see AnchorFromMetadata for the canonical lookup against
// task-property metadata.
func (lr *LineageReader) ReadChainFor(ctx context.Context, fromLoopID string) (string, map[string]any, error) {
	chainEntityID, err := lr.resolver.ChainEntityID(ctx, fromLoopID)
	if err != nil {
		return "", nil, fmt.Errorf("chain.LineageReader.ReadChainFor: resolve chain entity for loop %q: %w", fromLoopID, err)
	}
	triples, err := lr.entities.ReadEntity(ctx, chainEntityID)
	if err != nil {
		return chainEntityID, nil, fmt.Errorf("chain.LineageReader.ReadChainFor: read chain entity %q: %w", chainEntityID, err)
	}
	return chainEntityID, triples, nil
}

// AnchorRoleKeys names the role labels in TaskMessage.Metadata's
// related_loops map (keyed by agentic.MetadataKeyRelatedLoops at the
// outer level) that AnchorFromMetadata tries in declaration order.
// Each named role IS a completed ancestor loop in the chain at the
// time the spawned loop fires its emit-tool.
//
// Why related_loops and not bare task properties: spawn rules
// expose two channels for cross-loop data —
//
//   - `properties` map: visible to the persona via task properties
//     (the LLM reads them in the spawn prompt) but NOT propagated
//     to ToolCall.Metadata (smoke #8 run-7 evidence: hotfix on
//     PR #111 assumed properties propagate; they don't).
//   - `related_loops` map: framework-supported loop-ID lineage
//     channel. Stamped onto TaskMessage.Metadata under
//     agentic.MetadataKeyRelatedLoops at spawn time; agentic-loop
//     propagates that map onto every ToolCall.Metadata
//     (processor/agentic-loop/handlers.go:898).
//
// Every dev-via-spec spawn rule (rules/dev-via-spec/*.json) and
// the research→dev-via-spec transition rule already set
// `related_loops: { "researcher": "$entity.triple.lineage.researcher" }`
// — that researcher loop is a completed-ancestor with stamped
// `agent.loop.parent` triple, so chain.Resolver can walk from it.
//
// Adding a new role label here means any spawn rule that sets
// related_loops with the new key automatically becomes a viable
// chain anchor without touching tool code.
var AnchorRoleKeys = []string{
	"researcher",
}

// AnchorFromMetadata picks a chain-walkable loop_id from a tool
// call's task-property metadata, falling back to fallbackLoopID when
// no AnchorRoleKeys entry is present in the related_loops sub-map.
// Designed for emit-tools calling LineageReader.ReadChainFor — they
// MUST anchor on a completed loop, never the running one.
//
// The lookup walks two levels:
//
//  1. metadata[agentic.MetadataKeyRelatedLoops] — the framework's
//     cross-arc loop-ID lineage channel (set by spawn rules'
//     `related_loops` map; propagated automatically into
//     ToolCall.Metadata by agentic-loop). Decoded as map[string]any
//     after JSON round-trip.
//  2. Inside that sub-map, AnchorRoleKeys names the roles to try in
//     order. First non-empty string match wins.
//
// Falls back to fallbackLoopID (typically call.LoopID) when the
// related_loops sub-map is absent OR when no AnchorRoleKeys match.
// fallbackLoopID preserves backward-compat behaviour for tests +
// deployments that don't yet set related_loops with a known role.
//
// Metadata is map[string]any to match agentic.ToolCall.Metadata; this
// helper handles every type-assert and empty-string check so callers
// stay one-line.
func AnchorFromMetadata(metadata map[string]any, fallbackLoopID string) string {
	related, _ := metadata[agentic.MetadataKeyRelatedLoops].(map[string]any)
	for _, role := range AnchorRoleKeys {
		if v, ok := related[role].(string); ok && v != "" {
			return v
		}
	}
	return fallbackLoopID
}
