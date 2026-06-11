package chainbash

import (
	"context"
	"io"
	"log/slog"
	"testing"

	"github.com/c360studio/semstreams/agentic"
	"github.com/c360studio/semstreams/types"
)

// fakeInner records the ToolCall it was handed and returns a stub result. The
// wrapper's only behavioural contract on top of upstream is what task_id it
// threads through; capturing the delegated call is what the tests assert against.
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

func testPlatform() types.PlatformMeta {
	return types.PlatformMeta{Org: "org", Platform: "plat"}
}

// withRunID seeds the dispatch-stamped run anchor (semstreams#250) onto a call's
// metadata — the bare run loop-id agentic-loop puts under MetadataKeyRunID.
func withRunID(meta map[string]any, runID string) map[string]any {
	if meta == nil {
		meta = map[string]any{}
	}
	meta[agentic.MetadataKeyRunID] = runID
	return meta
}

func TestExecutor_RewritesTaskID_WhenRunIDDiffersFromLoopID(t *testing.T) {
	t.Parallel()
	inner := &fakeInner{result: agentic.ToolResult{Content: "ok"}}
	exec := NewExecutor(inner, testPlatform(), quietLogger())

	call := agentic.ToolCall{
		ID:        "call-1",
		Name:      "bash",
		LoopID:    "child-loop-uuid",
		Arguments: map[string]any{"command": "echo hi"},
		Metadata:  withRunID(map[string]any{"loop_id": "child-loop-uuid"}, "chain-root-uuid"),
	}

	if _, err := exec.Execute(context.Background(), call); err != nil {
		t.Fatalf("Execute: %v", err)
	}

	// task_id is the BARE run id (NOT a 6-part entity id) — the sandbox uses it
	// as a worktree dir name and the AttestationRunner prepends the prefix.
	if gotTaskID, _ := inner.got.Metadata[MetadataKeyTaskID].(string); gotTaskID != "chain-root-uuid" {
		t.Errorf("Metadata[task_id] = %q, want %q (bare run id)", gotTaskID, "chain-root-uuid")
	}
	if gotLoop, _ := inner.got.Metadata["loop_id"].(string); gotLoop != "child-loop-uuid" {
		t.Errorf("Metadata[loop_id] = %q; should be unchanged", gotLoop)
	}
}

func TestExecutor_DoesNotMutateOriginalToolCall(t *testing.T) {
	t.Parallel()
	inner := &fakeInner{result: agentic.ToolResult{Content: "ok"}}
	exec := NewExecutor(inner, testPlatform(), quietLogger())

	origMeta := withRunID(map[string]any{"loop_id": "loop-A"}, "chain-root")
	call := agentic.ToolCall{ID: "call-1", Name: "bash", LoopID: "loop-A", Metadata: origMeta}

	if _, err := exec.Execute(context.Background(), call); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if _, hasTask := origMeta[MetadataKeyTaskID]; hasTask {
		t.Errorf("original Metadata mutated: task_id leaked into caller's map")
	}
}

func TestExecutor_FailsSoftWhenNoRunAnchor(t *testing.T) {
	t.Parallel()
	// No run anchor on the call (standalone loop / pre-#250). The wrapper must
	// not rewrite task_id — upstream falls back to loop_id.
	inner := &fakeInner{result: agentic.ToolResult{Content: "ok"}}
	exec := NewExecutor(inner, testPlatform(), quietLogger())

	call := agentic.ToolCall{
		ID:       "call-1",
		Name:     "bash",
		LoopID:   "orphan-loop",
		Metadata: map[string]any{"loop_id": "orphan-loop"}, // no MetadataKeyRunID
	}

	if _, err := exec.Execute(context.Background(), call); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if _, has := inner.got.Metadata[MetadataKeyTaskID]; has {
		t.Errorf("task_id should NOT be set with no run anchor; got %v", inner.got.Metadata[MetadataKeyTaskID])
	}
}

func TestExecutor_SkipsRewriteWhenRunIDEqualsLoopID(t *testing.T) {
	t.Parallel()
	// The call's loop IS the chain/run root: run id == loop id → no rewrite.
	inner := &fakeInner{result: agentic.ToolResult{Content: "ok"}}
	exec := NewExecutor(inner, testPlatform(), quietLogger())

	call := agentic.ToolCall{
		ID:       "call-1",
		Name:     "bash",
		LoopID:   "root-loop",
		Metadata: withRunID(map[string]any{"loop_id": "root-loop"}, "root-loop"),
	}
	if _, err := exec.Execute(context.Background(), call); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if _, has := inner.got.Metadata[MetadataKeyTaskID]; has {
		t.Errorf("unnecessary task_id rewrite when run_id == loop_id")
	}
}

func TestExecutor_UsesMetadataLoopID_ForRootSkipCheck(t *testing.T) {
	t.Parallel()
	// Typed LoopID empty; loop_id only in Metadata. The wrapper reads the run
	// anchor from metadata and uses the metadata loop_id for the root-skip check;
	// since run_id != loop_id, it rewrites task_id to the bare run id.
	inner := &fakeInner{result: agentic.ToolResult{Content: "ok"}}
	exec := NewExecutor(inner, testPlatform(), quietLogger())

	call := agentic.ToolCall{
		ID:       "call-1",
		Name:     "bash",
		Metadata: withRunID(map[string]any{"loop_id": "loop-from-meta"}, "chain-from-meta"),
	}
	if _, err := exec.Execute(context.Background(), call); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if gotTaskID, _ := inner.got.Metadata[MetadataKeyTaskID].(string); gotTaskID != "chain-from-meta" {
		t.Errorf("Metadata[task_id] = %q, want %q", gotTaskID, "chain-from-meta")
	}
}

func TestExecutor_NilMetadata_DelegatesUnchanged(t *testing.T) {
	t.Parallel()
	// Nil metadata → no run anchor → delegate unchanged, no panic, no task_id.
	inner := &fakeInner{result: agentic.ToolResult{Content: "ok"}}
	exec := NewExecutor(inner, testPlatform(), quietLogger())

	call := agentic.ToolCall{ID: "call-1", Name: "bash", LoopID: "child-loop"}
	if _, err := exec.Execute(context.Background(), call); err != nil {
		t.Fatalf("Execute panicked or errored on nil Metadata: %v", err)
	}
	if _, has := inner.got.Metadata[MetadataKeyTaskID]; has {
		t.Errorf("nil-metadata call must delegate unchanged (no run anchor)")
	}
}

// TestExecutor_MisroutedCall_ReturnsNotFound pins the defence-in-depth guard:
// a call whose Name isn't the wrapper's registered ToolName must return a clean
// ToolErrorNotFound and must NOT delegate to the inner executor (which would
// silently rewrite metadata on the wrong tool).
func TestExecutor_MisroutedCall_ReturnsNotFound(t *testing.T) {
	t.Parallel()
	inner := &fakeInner{result: agentic.ToolResult{Content: "should not run"}}
	exec := NewExecutor(inner, testPlatform(), quietLogger())

	call := agentic.ToolCall{
		ID:       "call-misroute",
		Name:     "not-bash",
		LoopID:   "loop-A",
		Metadata: withRunID(map[string]any{}, "chain-root-uuid"),
	}
	res, err := exec.Execute(context.Background(), call)
	if err != nil {
		t.Fatalf("Execute returned a transport error; want a tool-result error: %v", err)
	}
	if res.ErrorKind != agentic.ToolErrorNotFound {
		t.Errorf("ErrorKind = %q, want %q", res.ErrorKind, agentic.ToolErrorNotFound)
	}
	if res.Error == "" {
		t.Error("expected a descriptive Error string on the misrouted result")
	}
	if res.CallID != "call-misroute" || res.Name != "not-bash" {
		t.Errorf("result identity = (CallID %q, Name %q), want (call-misroute, not-bash)", res.CallID, res.Name)
	}
	// The inner executor must never have run (its got-call stays zero-valued).
	if inner.got.ID != "" {
		t.Errorf("inner executor was delegated to on a misrouted call (got ID %q)", inner.got.ID)
	}
}

func TestExecutor_ListTools_Delegates(t *testing.T) {
	t.Parallel()
	wantTools := []agentic.ToolDefinition{{Name: "bash", Description: "wrapped", Parameters: map[string]any{"type": "object"}}}
	inner := &fakeInner{tools: wantTools}
	exec := NewExecutor(inner, testPlatform(), quietLogger())

	got := exec.ListTools()
	if len(got) != 1 || got[0].Name != "bash" || got[0].Description != "wrapped" {
		t.Errorf("ListTools = %+v, want %+v", got, wantTools)
	}
}
