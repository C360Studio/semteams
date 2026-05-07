package chain

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/c360studio/semstreams/agentic"
	"github.com/c360studio/semstreams/types"
)

// fakeParentReader maps loop_id → parent entity ID for deterministic
// ancestry tests. An empty parent (or absent key) means chain root.
type fakeParentReader struct {
	parents map[string]string // loop_id -> parent entity id ("" if root)
	err     error             // if non-nil, returned on every call
	calls   int               // observed read count for assertion
}

func (f *fakeParentReader) ReadParent(_ context.Context, loopID string) (string, error) {
	f.calls++
	if f.err != nil {
		return "", f.err
	}
	return f.parents[loopID], nil
}

func testPlatform() types.PlatformMeta {
	return types.PlatformMeta{Org: "c360", Platform: "test"}
}

// loopEntityID composes the 6-part loop entity ID for a given loop_id under
// testPlatform. Saves test verbosity.
func loopEntityID(t *testing.T, loopID string) string {
	t.Helper()
	return agentic.LoopExecutionEntityID("c360", "test", loopID)
}

// TestResolver_ChainID_Root: a loop with no parent triple resolves to itself.
// This is the chain-root case (e.g. dispatch loop).
func TestResolver_ChainID_Root(t *testing.T) {
	r := NewResolver(&fakeParentReader{parents: map[string]string{}}, testPlatform())

	chainID, err := r.ChainID(context.Background(), "dispatch_abc")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if chainID != "dispatch_abc" {
		t.Errorf("ChainID: got %q, want %q", chainID, "dispatch_abc")
	}
}

// TestResolver_ChainID_OneHop: a loop one hop from the root.
func TestResolver_ChainID_OneHop(t *testing.T) {
	parents := map[string]string{
		"researcher_1": loopEntityID(t, "dispatch_abc"),
		// "dispatch_abc" not present in map → reads as "" (root)
	}
	reader := &fakeParentReader{parents: parents}
	r := NewResolver(reader, testPlatform())

	chainID, err := r.ChainID(context.Background(), "researcher_1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if chainID != "dispatch_abc" {
		t.Errorf("ChainID: got %q, want %q", chainID, "dispatch_abc")
	}
	if reader.calls != 2 {
		t.Errorf("expected 2 reads (one per hop), got %d", reader.calls)
	}
}

// TestResolver_ChainID_DeepWalk: realistic 7-hop dev-via-spec arc.
// dispatch → researcher → research-reviewer → planner → reviewer →
// challenger → architect.
func TestResolver_ChainID_DeepWalk(t *testing.T) {
	parents := map[string]string{
		"architect_g":         loopEntityID(t, "challenger_f"),
		"challenger_f":        loopEntityID(t, "reviewer_e"),
		"reviewer_e":          loopEntityID(t, "planner_d"),
		"planner_d":           loopEntityID(t, "research_reviewer_c"),
		"research_reviewer_c": loopEntityID(t, "researcher_b"),
		"researcher_b":        loopEntityID(t, "dispatch_a"),
		// "dispatch_a" absent → chain root
	}
	r := NewResolver(&fakeParentReader{parents: parents}, testPlatform())

	chainID, err := r.ChainID(context.Background(), "architect_g")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if chainID != "dispatch_a" {
		t.Errorf("ChainID: got %q, want dispatch_a", chainID)
	}
}

// TestResolver_ChainID_RejectsEmpty: empty loopID is a programmer error;
// don't silently walk nothing.
func TestResolver_ChainID_RejectsEmpty(t *testing.T) {
	r := NewResolver(&fakeParentReader{}, testPlatform())
	if _, err := r.ChainID(context.Background(), ""); err == nil {
		t.Fatal("expected error on empty loopID, got nil")
	}
}

// TestResolver_ChainID_PropagatesReadError: a graph read failure surfaces
// to the caller with hop context so logs stay diagnosable.
func TestResolver_ChainID_PropagatesReadError(t *testing.T) {
	r := NewResolver(&fakeParentReader{err: errors.New("graph timeout")}, testPlatform())
	_, err := r.ChainID(context.Background(), "loop_x")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "graph timeout") || !strings.Contains(err.Error(), "loop_x") {
		t.Errorf("error should name the underlying read failure and the loop_id; got %v", err)
	}
}

// TestResolver_ChainID_RejectsMalformedParentEntity: if the parent triple
// holds a non-loop-shaped entity ID, surface it rather than silently
// walking past it.
func TestResolver_ChainID_RejectsMalformedParentEntity(t *testing.T) {
	parents := map[string]string{
		"loop_x": "c360.test.agent.chain.execution.something", // chain entity, not loop entity
	}
	r := NewResolver(&fakeParentReader{parents: parents}, testPlatform())
	_, err := r.ChainID(context.Background(), "loop_x")
	if err == nil {
		t.Fatal("expected error on malformed parent entity, got nil")
	}
}

// TestResolver_ChainID_HopBudget: a cycle (or runaway chain) is bounded.
// Don't loop forever on a malformed graph.
func TestResolver_ChainID_HopBudget(t *testing.T) {
	// Build a cycle: A → B → A.
	parents := map[string]string{
		"loop_a": loopEntityID(t, "loop_b"),
		"loop_b": loopEntityID(t, "loop_a"),
	}
	r := NewResolver(&fakeParentReader{parents: parents}, testPlatform())
	_, err := r.ChainID(context.Background(), "loop_a")
	if err == nil {
		t.Fatal("expected error on cycle, got nil")
	}
	if !strings.Contains(err.Error(), "exceeded") {
		t.Errorf("error should mention hop ceiling exceeded; got %v", err)
	}
}

// TestResolver_ChainEntityID composes ChainID with the upstream
// agentic.ChainExecutionEntityID constructor.
func TestResolver_ChainEntityID(t *testing.T) {
	parents := map[string]string{
		"researcher_1": loopEntityID(t, "dispatch_abc"),
	}
	r := NewResolver(&fakeParentReader{parents: parents}, testPlatform())

	got, err := r.ChainEntityID(context.Background(), "researcher_1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := "c360.test.agent.chain.execution.dispatch_abc"
	if got != want {
		t.Errorf("ChainEntityID: got %q, want %q", got, want)
	}
}
