package chainbash

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"testing"

	"github.com/c360studio/semstreams/agentic"
)

// fakeResolver records ChainID calls and returns the configured chain_id
// or err. Lets us drive the wrapper through every resolution outcome
// without bringing NATS up.
type fakeResolver struct {
	chainID string
	err     error
	calls   []string
}

func (f *fakeResolver) ChainID(_ context.Context, loopID string) (string, error) {
	f.calls = append(f.calls, loopID)
	return f.chainID, f.err
}

// fakeInner records the ToolCall it was handed and returns a stub
// result. The wrapper's only behavioural contract on top of upstream is
// what task_id it threads through; capturing the delegated call is what
// the tests need to assert against.
type fakeInner struct {
	got    agentic.ToolCall
	result agentic.ToolResult
	tools  []agentic.ToolDefinition
}

func (f *fakeInner) Execute(_ context.Context, call agentic.ToolCall) (agentic.ToolResult, error) {
	f.got = call
	return f.result, nil
}

func (f *fakeInner) ListTools() []agentic.ToolDefinition { return f.tools }

func quietLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func TestExecutor_RewritesTaskID_WhenChainIDDiffersFromLoopID(t *testing.T) {
	t.Parallel()
	inner := &fakeInner{result: agentic.ToolResult{Content: "ok"}}
	resolver := &fakeResolver{chainID: "chain-root-uuid"}
	exec := NewExecutor(inner, resolver, quietLogger())

	call := agentic.ToolCall{
		ID:        "call-1",
		Name:      "bash",
		LoopID:    "child-loop-uuid",
		Arguments: map[string]any{"command": "echo hi"},
		Metadata:  map[string]any{"loop_id": "child-loop-uuid"},
	}

	_, err := exec.Execute(context.Background(), call)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}

	gotTaskID, _ := inner.got.Metadata[MetadataKeyTaskID].(string)
	if gotTaskID != "chain-root-uuid" {
		t.Errorf("Metadata[task_id] = %q, want %q", gotTaskID, "chain-root-uuid")
	}
	// loop_id should remain intact — task_id is the override key.
	if gotLoop, _ := inner.got.Metadata["loop_id"].(string); gotLoop != "child-loop-uuid" {
		t.Errorf("Metadata[loop_id] = %q; should be unchanged", gotLoop)
	}
}

func TestExecutor_DoesNotMutateOriginalToolCall(t *testing.T) {
	t.Parallel()
	inner := &fakeInner{result: agentic.ToolResult{Content: "ok"}}
	resolver := &fakeResolver{chainID: "chain-root"}
	exec := NewExecutor(inner, resolver, quietLogger())

	origMeta := map[string]any{"loop_id": "loop-A"}
	call := agentic.ToolCall{ID: "call-1", Name: "bash", LoopID: "loop-A", Metadata: origMeta}

	if _, err := exec.Execute(context.Background(), call); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if _, hasTask := origMeta[MetadataKeyTaskID]; hasTask {
		t.Errorf("original Metadata mutated: task_id leaked into caller's map")
	}
}

func TestExecutor_FailsSoftOnResolverError(t *testing.T) {
	t.Parallel()
	inner := &fakeInner{result: agentic.ToolResult{Content: "ok"}}
	resolver := &fakeResolver{err: errors.New("nats timeout")}
	exec := NewExecutor(inner, resolver, quietLogger())

	call := agentic.ToolCall{
		ID:       "call-1",
		Name:     "bash",
		LoopID:   "orphan-loop",
		Metadata: map[string]any{"loop_id": "orphan-loop"},
	}

	if _, err := exec.Execute(context.Background(), call); err != nil {
		t.Fatalf("Execute returned error on resolver-failure soft-fallback: %v", err)
	}
	// No task_id rewrite happened — upstream falls back to loop_id.
	if _, has := inner.got.Metadata[MetadataKeyTaskID]; has {
		t.Errorf("task_id should NOT be set on resolver error; got %v", inner.got.Metadata[MetadataKeyTaskID])
	}
}

func TestExecutor_SkipsRewriteWhenChainIDEqualsLoopID(t *testing.T) {
	t.Parallel()
	// Loop is the chain root: ChainID returns the same loop_id back.
	// We can skip the rewrite (no allocation, same behaviour).
	inner := &fakeInner{result: agentic.ToolResult{Content: "ok"}}
	resolver := &fakeResolver{chainID: "root-loop"}
	exec := NewExecutor(inner, resolver, quietLogger())

	call := agentic.ToolCall{ID: "call-1", Name: "bash", LoopID: "root-loop"}
	if _, err := exec.Execute(context.Background(), call); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if _, has := inner.got.Metadata[MetadataKeyTaskID]; has {
		t.Errorf("unnecessary task_id rewrite when chain_id == loop_id")
	}
}

func TestExecutor_FallbackToMetadataLoopID_WhenTypedFieldEmpty(t *testing.T) {
	t.Parallel()
	inner := &fakeInner{result: agentic.ToolResult{Content: "ok"}}
	resolver := &fakeResolver{chainID: "chain-from-meta"}
	exec := NewExecutor(inner, resolver, quietLogger())

	// Typed LoopID empty; loop_id only present in Metadata. Wrapper
	// should still resolve.
	call := agentic.ToolCall{
		ID:       "call-1",
		Name:     "bash",
		Metadata: map[string]any{"loop_id": "loop-from-meta"},
	}
	if _, err := exec.Execute(context.Background(), call); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if len(resolver.calls) != 1 || resolver.calls[0] != "loop-from-meta" {
		t.Errorf("resolver.ChainID called with %v, want [loop-from-meta]", resolver.calls)
	}
	if gotTaskID, _ := inner.got.Metadata[MetadataKeyTaskID].(string); gotTaskID != "chain-from-meta" {
		t.Errorf("Metadata[task_id] = %q, want %q", gotTaskID, "chain-from-meta")
	}
}

func TestExecutor_NoLoopID_DelegatesUnchanged(t *testing.T) {
	t.Parallel()
	inner := &fakeInner{result: agentic.ToolResult{Content: "ok"}}
	resolver := &fakeResolver{} // should never be called
	exec := NewExecutor(inner, resolver, quietLogger())

	call := agentic.ToolCall{ID: "call-1", Name: "bash"}
	if _, err := exec.Execute(context.Background(), call); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if len(resolver.calls) != 0 {
		t.Errorf("resolver should not be called when loop_id absent; got %v", resolver.calls)
	}
}

func TestExecutor_ListTools_Delegates(t *testing.T) {
	t.Parallel()
	wantTools := []agentic.ToolDefinition{{Name: "bash", Description: "wrapped", Parameters: map[string]any{"type": "object"}}}
	inner := &fakeInner{tools: wantTools}
	exec := NewExecutor(inner, &fakeResolver{}, quietLogger())

	got := exec.ListTools()
	if len(got) != 1 || got[0].Name != "bash" || got[0].Description != "wrapped" {
		t.Errorf("ListTools = %+v, want %+v", got, wantTools)
	}
}

// Nil-Metadata path: withTaskID must allocate a fresh map rather than
// indexing into a nil map (which would panic). Regression guard for
// callers that thread loop_id only via the typed field.
func TestExecutor_NilMetadata_AllocatesAndRewrites(t *testing.T) {
	t.Parallel()
	inner := &fakeInner{result: agentic.ToolResult{Content: "ok"}}
	resolver := &fakeResolver{chainID: "chain-root"}
	exec := NewExecutor(inner, resolver, quietLogger())

	// LoopID set via typed field only; Metadata is nil.
	call := agentic.ToolCall{ID: "call-1", Name: "bash", LoopID: "child-loop"}
	if _, err := exec.Execute(context.Background(), call); err != nil {
		t.Fatalf("Execute panicked or errored on nil Metadata: %v", err)
	}
	if gotTaskID, _ := inner.got.Metadata[MetadataKeyTaskID].(string); gotTaskID != "chain-root" {
		t.Errorf("Metadata[task_id] = %q, want %q", gotTaskID, "chain-root")
	}
}
