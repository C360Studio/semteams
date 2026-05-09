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
