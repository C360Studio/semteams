package chain

import (
	"context"
	"errors"
	"testing"

	"github.com/c360studio/semstreams/agentic"
)

// TestLineageReader_HappyPath drives the success case end-to-end:
// resolver picks chain entity ID, entities returns its predicate map,
// LineageReader returns both with no error.
func TestLineageReader_HappyPath(t *testing.T) {
	pr := &fakeParentReader{parents: map[string]string{
		// Two-hop chain: from_loop → parent → root (no parent).
		"from_loop": "c360.test.agent.agentic-loop.execution.parent_loop",
		// parent_loop has no entry → root signal.
	}}
	resolver := NewResolver(pr, testPlatform())
	er := &fakeEntityReader{
		entities: map[string]map[string]any{
			"c360.test.agent.chain.execution.parent_loop": {
				"chain.research_artifact.loop": "researcher_xyz",
				"chain.slug.stem":              "2026-05-09-osh-driver",
			},
		},
	}

	lr := NewLineageReader(resolver, er)
	chainEntityID, triples, err := lr.ReadChainFor(context.Background(), "from_loop")
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if chainEntityID != "c360.test.agent.chain.execution.parent_loop" {
		t.Errorf("chainEntityID = %q, want resolver-derived value", chainEntityID)
	}
	if got := triples["chain.research_artifact.loop"]; got != "researcher_xyz" {
		t.Errorf("triples missing or wrong predicate: %v", triples)
	}
}

// TestLineageReader_ResolverError returns the resolver error wrapped
// with package context. chainEntityID is empty per the doc-comment
// contract.
func TestLineageReader_ResolverError(t *testing.T) {
	resolver := NewResolver(&fakeParentReader{err: errors.New("graph KV unavailable")}, testPlatform())
	lr := NewLineageReader(resolver, &fakeEntityReader{})
	chainEntityID, triples, err := lr.ReadChainFor(context.Background(), "from_loop")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if chainEntityID != "" {
		t.Errorf("chainEntityID = %q, want empty when resolver fails", chainEntityID)
	}
	if triples != nil {
		t.Errorf("triples = %v, want nil when resolver fails", triples)
	}
}

// TestLineageReader_EntityReadError returns the chainEntityID (so
// callers can log it) plus the wrapped error; triples is nil. Lets
// callers distinguish "I know which chain you meant but couldn't
// read it" from "I couldn't even resolve which chain you meant."
func TestLineageReader_EntityReadError(t *testing.T) {
	pr := &fakeParentReader{parents: map[string]string{}}
	resolver := NewResolver(pr, testPlatform())
	er := &fakeEntityReader{err: errors.New("graph timeout")}

	lr := NewLineageReader(resolver, er)
	chainEntityID, triples, err := lr.ReadChainFor(context.Background(), "from_loop")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if chainEntityID == "" {
		t.Error("chainEntityID should still be populated when entity read fails (resolver succeeded)")
	}
	if triples != nil {
		t.Errorf("triples = %v, want nil when entity read fails", triples)
	}
}

// withRelatedLoops wraps a related-loops map in the outer
// MetadataKeyRelatedLoops envelope agentic-loop produces at runtime.
// Mirrors the JSON-decoded shape: outer is map[string]any, inner is
// map[string]any (string values), keyed by role label.
func withRelatedLoops(roles map[string]string) map[string]any {
	inner := make(map[string]any, len(roles))
	for k, v := range roles {
		inner[k] = v
	}
	return map[string]any{agentic.MetadataKeyRelatedLoops: inner}
}

func TestAnchorFromMetadata_RelatedLoopsResearcherWins(t *testing.T) {
	got := AnchorFromMetadata(
		withRelatedLoops(map[string]string{"researcher": "researcher-completed"}),
		"running-loop-fallback",
	)
	if got != "researcher-completed" {
		t.Errorf("got %q, want researcher-completed (related_loops.researcher anchors the chain walk)", got)
	}
}

func TestAnchorFromMetadata_FallsBackWhenRelatedLoopsAbsent(t *testing.T) {
	// Metadata exists but no agent.related_loops envelope — pre-spawn-rule
	// or test deployments without lineage threading.
	got := AnchorFromMetadata(map[string]any{
		"loop_id": "running-loop-id",
	}, "running-loop-fallback")
	if got != "running-loop-fallback" {
		t.Errorf("got %q, want fallback (no related_loops envelope)", got)
	}
}

func TestAnchorFromMetadata_FallsBackWhenRoleNotInRelatedLoops(t *testing.T) {
	// related_loops envelope is present but only carries unrecognised role
	// labels — operator added a label this helper doesn't yet know about.
	got := AnchorFromMetadata(
		withRelatedLoops(map[string]string{"some-future-role": "ignored"}),
		"running-loop-fallback",
	)
	if got != "running-loop-fallback" {
		t.Errorf("got %q, want fallback (no AnchorRoleKeys match)", got)
	}
}

func TestAnchorFromMetadata_FallsBackOnEmptyRoleValue(t *testing.T) {
	// Empty string in related_loops isn't a viable anchor — fall through.
	got := AnchorFromMetadata(
		withRelatedLoops(map[string]string{"researcher": ""}),
		"running-loop-fallback",
	)
	if got != "running-loop-fallback" {
		t.Errorf("got %q, want fallback (empty-string value treated as absent)", got)
	}
}

func TestAnchorFromMetadata_FallsBackOnNonStringRoleValue(t *testing.T) {
	// Defensive: if related_loops value was set to a non-string (rare, but
	// the type-assert handles it gracefully). Mock direct rather than via
	// withRelatedLoops to bypass its string-typed signature.
	got := AnchorFromMetadata(map[string]any{
		agentic.MetadataKeyRelatedLoops: map[string]any{"researcher": 42},
	}, "running-loop-fallback")
	if got != "running-loop-fallback" {
		t.Errorf("got %q, want fallback (non-string value treated as absent)", got)
	}
}

func TestAnchorFromMetadata_FallsBackOnRelatedLoopsNotMap(t *testing.T) {
	// Defensive: outer envelope value isn't a map (corruption / wire-format
	// drift). Don't panic; fall through to fallback.
	got := AnchorFromMetadata(map[string]any{
		agentic.MetadataKeyRelatedLoops: "not a map",
	}, "running-loop-fallback")
	if got != "running-loop-fallback" {
		t.Errorf("got %q, want fallback (related_loops envelope wrong type)", got)
	}
}

func TestAnchorFromMetadata_NilMetadata(t *testing.T) {
	got := AnchorFromMetadata(nil, "running-loop-fallback")
	if got != "running-loop-fallback" {
		t.Errorf("got %q, want fallback (nil metadata)", got)
	}
}
