package chain

import (
	"context"
	"errors"
	"testing"
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
				"chain.research_artifact_loop": "researcher_xyz",
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
	if got := triples["chain.research_artifact_loop"]; got != "researcher_xyz" {
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

func TestAnchorFromMetadata_PriorLoopIDPreferred(t *testing.T) {
	got := AnchorFromMetadata(map[string]any{
		"prior_loop_id":             "completed-parent-1",
		"research_reviewer_loop_id": "should-be-ignored",
	}, "running-loop-fallback")
	if got != "completed-parent-1" {
		t.Errorf("got %q, want completed-parent-1 (prior_loop_id wins)", got)
	}
}

func TestAnchorFromMetadata_FallsBackToResearchReviewerKey(t *testing.T) {
	// Planner spawn (research-mode-transition rule_03) uses the
	// research_reviewer_loop_id key — not prior_loop_id.
	got := AnchorFromMetadata(map[string]any{
		"research_reviewer_loop_id": "research-reviewer-abc",
	}, "running-loop-fallback")
	if got != "research-reviewer-abc" {
		t.Errorf("got %q, want research-reviewer-abc (planner-spawn metadata key)", got)
	}
}

func TestAnchorFromMetadata_FallsBackWhenAllAbsent(t *testing.T) {
	got := AnchorFromMetadata(map[string]any{
		"unrelated_key": "ignored",
	}, "running-loop-fallback")
	if got != "running-loop-fallback" {
		t.Errorf("got %q, want fallback (no anchor key present)", got)
	}
}

func TestAnchorFromMetadata_FallsBackOnEmptyValue(t *testing.T) {
	// Empty string in metadata shouldn't be treated as a valid anchor —
	// fall through to the next key, then the fallback. Empty fallback
	// surfaces as empty (caller decides what to do).
	got := AnchorFromMetadata(map[string]any{
		"prior_loop_id": "",
	}, "running-loop-fallback")
	if got != "running-loop-fallback" {
		t.Errorf("got %q, want fallback (empty-string value treated as absent)", got)
	}
}

func TestAnchorFromMetadata_FallsBackOnNonStringValue(t *testing.T) {
	// Defensive: if metadata key was set to a non-string (rare, but the
	// type-assert with comma-ok handles it gracefully).
	got := AnchorFromMetadata(map[string]any{
		"prior_loop_id": 42,
	}, "running-loop-fallback")
	if got != "running-loop-fallback" {
		t.Errorf("got %q, want fallback (non-string value treated as absent)", got)
	}
}

func TestAnchorFromMetadata_NilMetadata(t *testing.T) {
	got := AnchorFromMetadata(nil, "running-loop-fallback")
	if got != "running-loop-fallback" {
		t.Errorf("got %q, want fallback (nil metadata treated as no keys present)", got)
	}
}
