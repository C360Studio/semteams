package chain

import (
	"context"
	"fmt"
)

// LineageReader composes ChainEntityID resolution + entity-triple
// reading into a single ReadChainFor method. Implements the
// ChainReader interface declared (structurally) by the emit-tool
// packages (cmd/semteams/tools/emitplan, .../emitconsensus,
// .../emitspecartifact) — Go's structural interfaces let a single
// adapter satisfy all three with no shared interface declaration.
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

// AnchorMetadataKeys names the task-property metadata keys product
// rules use to pass a completed ancestor's loop_id to the spawned
// loop. AnchorFromMetadata tries them in declaration order.
//
// Source: every dev-via-spec spawn rule (rules/dev-via-spec/*.json)
// sets `prior_loop_id` in spawn properties; research-mode-transition's
// planner spawn (rules/research-mode-transition/03-*.json) historically
// uses `research_reviewer_loop_id` instead. Both keys are completed
// ancestor loop_ids. Adding a new key here means a new spawn rule's
// metadata convention is now resolvable to a chain anchor without
// touching any tool code.
var AnchorMetadataKeys = []string{
	"prior_loop_id",
	"research_reviewer_loop_id",
}

// AnchorFromMetadata picks a chain-walkable loop_id from a tool
// call's task-property metadata, falling back to fallbackLoopID when
// no AnchorMetadataKeys entry is present. Designed for emit-tools
// calling LineageReader.ReadChainFor — they MUST anchor on a
// completed loop, never the running one.
//
// Returns the first non-empty string-typed value found across
// AnchorMetadataKeys (in order); falls back to fallbackLoopID if none
// match. fallbackLoopID is typically call.LoopID — preserves
// pre-fix behaviour as a last-resort safety net (works when the
// caller's own parent triple happens to be stamped, fails the same
// way it did pre-fix when not). The "preferred path → safety net"
// shape mirrors portresolver.SubjectOrDefault.
//
// Metadata is map[string]any to match agentic.ToolCall.Metadata; this
// helper handles the type-assert and empty-string check so callers
// stay one-line.
func AnchorFromMetadata(metadata map[string]any, fallbackLoopID string) string {
	for _, key := range AnchorMetadataKeys {
		if v, ok := metadata[key].(string); ok && v != "" {
			return v
		}
	}
	return fallbackLoopID
}
