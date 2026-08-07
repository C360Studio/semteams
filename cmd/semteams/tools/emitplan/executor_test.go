package emitplan

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"strings"
	"sync"
	"testing"

	"github.com/c360studio/semstreams/agentic"
	"github.com/c360studio/semstreams/message"
	"github.com/c360studio/semstreams/types"
)

// fakeTriplePublisher records every AddTriple call. Mirrors the same
// shape as emitartifact / emitspecartifact test fakes — kept duplicated
// because each tool package sets its own narrow interface.
type fakeTriplePublisher struct {
	mu      sync.Mutex
	triples []message.Triple
	err     error
}

func (f *fakeTriplePublisher) AddTriple(_ context.Context, t message.Triple) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.err != nil {
		return f.err
	}
	f.triples = append(f.triples, t)
	return nil
}

func (f *fakeTriplePublisher) AddTriplesBatch(ctx context.Context, triples []message.Triple) error {
	for _, t := range triples {
		if err := f.AddTriple(ctx, t); err != nil {
			return err
		}
	}
	return nil
}

func (f *fakeTriplePublisher) snapshot() []message.Triple {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]message.Triple, len(f.triples))
	copy(out, f.triples)
	return out
}

type fakePublisher struct {
	mu      sync.Mutex
	subject string
	data    []byte
	calls   int
	err     error
}

func (f *fakePublisher) Publish(_ context.Context, subject string, data []byte) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls++
	if f.err != nil {
		return f.err
	}
	f.subject = subject
	f.data = append([]byte(nil), data...)
	return nil
}

func (f *fakePublisher) snapshot() (string, []byte, int) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.subject, append([]byte(nil), f.data...), f.calls
}

func newExecutor(t *testing.T) (*Executor, *fakeTriplePublisher, *fakePublisher, string) {
	t.Helper()
	tmpDir := t.TempDir()
	tp := &fakeTriplePublisher{}
	pub := &fakePublisher{}
	exec := NewExecutor(tp, pub, types.PlatformMeta{Org: "c360", Platform: "test"}, nil, tmpDir)
	return exec, tp, pub, tmpDir
}

func defaultCall(args map[string]any) agentic.ToolCall {
	return agentic.ToolCall{
		ID:        "call-001",
		Name:      ToolName,
		Arguments: args,
		LoopID:    "loop-planner-abc",
		TraceID:   "trace-x",
	}
}

func defaultPlanArgs() map[string]any {
	return map[string]any{
		"revision": 1,
		"goal":     "Implement OSH IDriver backed by Meshtastic LoRa transport.",
		"context":  "OSH driver framework expects observation events on its IDriver interface; Meshtastic radio supplies the physical-layer transport. The boundary is the IDriver implementation that adapts MeshPacket payloads into OSH observations.",
		"epics": []any{
			"Implement IDriver adapter exposing receive() / publish() against Meshtastic radio events.",
			"Wire OGC CS endpoints to surface observations from the IDriver.",
		},
	}
}

func TestListTools_SchemaShape(t *testing.T) {
	exec, _, _, _ := newExecutor(t)
	defs := exec.ListTools()
	if len(defs) != 1 {
		t.Fatalf("ListTools length = %d, want 1", len(defs))
	}
	def := defs[0]
	if def.Name != ToolName {
		t.Errorf("tool name = %q, want %q", def.Name, ToolName)
	}
	props, ok := def.Parameters["properties"].(map[string]any)
	if !ok {
		t.Fatalf("schema missing properties: %#v", def.Parameters)
	}
	for _, key := range []string{"revision", "title", "goal", "context", "scope_in", "scope_out", "epics", "depends_on"} {
		if _, ok := props[key]; !ok {
			t.Errorf("missing property %q in tool schema", key)
		}
	}
	for _, forbidden := range []string{"loop_id", "produced_at", "slug"} {
		if _, ok := props[forbidden]; ok {
			t.Errorf("schema must not advertise %q (server-supplied)", forbidden)
		}
	}
	required, _ := def.Parameters["required"].([]string)
	wantRequired := map[string]bool{"revision": true, "goal": true, "context": true, "epics": true}
	got := map[string]bool{}
	for _, r := range required {
		got[r] = true
	}
	for k := range wantRequired {
		if !got[k] {
			t.Errorf("required field missing: %q (have %v)", k, required)
		}
	}
}

func TestExecute_WrongToolName(t *testing.T) {
	exec, _, _, _ := newExecutor(t)
	call := defaultCall(defaultPlanArgs())
	call.Name = "something-else"
	res, err := exec.Execute(context.Background(), call)
	if err != nil {
		t.Fatalf("Execute err = %v, want nil", err)
	}
	if res.ErrorKind != agentic.ToolErrorNotFound {
		t.Errorf("ErrorKind = %v, want %v", res.ErrorKind, agentic.ToolErrorNotFound)
	}
}

func TestExecute_MissingLoopID(t *testing.T) {
	exec, _, _, _ := newExecutor(t)
	call := defaultCall(defaultPlanArgs())
	call.LoopID = ""
	res, err := exec.Execute(context.Background(), call)
	if err != nil {
		t.Fatalf("Execute err = %v, want nil", err)
	}
	if res.ErrorKind != agentic.ToolErrorInternal {
		t.Errorf("ErrorKind = %v, want %v", res.ErrorKind, agentic.ToolErrorInternal)
	}
}

func TestExecute_NoEpics_FailsValidation(t *testing.T) {
	exec, _, _, _ := newExecutor(t)
	args := defaultPlanArgs()
	args["epics"] = []any{}
	res, _ := exec.Execute(context.Background(), defaultCall(args))
	if res.ErrorKind != agentic.ToolErrorInvalidArgs {
		t.Errorf("ErrorKind = %v, want %v", res.ErrorKind, agentic.ToolErrorInvalidArgs)
	}
	if !strings.Contains(res.Error, "epics") {
		t.Errorf("Result.Error = %q, want contains 'epics'", res.Error)
	}
}

func TestExecute_MissingGoal_FailsValidation(t *testing.T) {
	exec, _, _, _ := newExecutor(t)
	args := defaultPlanArgs()
	delete(args, "goal")
	res, _ := exec.Execute(context.Background(), defaultCall(args))
	if res.ErrorKind != agentic.ToolErrorInvalidArgs {
		t.Errorf("ErrorKind = %v, want %v", res.ErrorKind, agentic.ToolErrorInvalidArgs)
	}
}

func TestExecute_MissingContext_FailsValidation(t *testing.T) {
	exec, _, _, _ := newExecutor(t)
	args := defaultPlanArgs()
	delete(args, "context")
	res, _ := exec.Execute(context.Background(), defaultCall(args))
	if res.ErrorKind != agentic.ToolErrorInvalidArgs {
		t.Errorf("ErrorKind = %v, want %v", res.ErrorKind, agentic.ToolErrorInvalidArgs)
	}
}

func TestExecute_HappyPath_TriplesAndPayload(t *testing.T) {
	exec, tp, pub, dir := newExecutor(t)
	args := defaultPlanArgs()
	args["title"] = "OSH Meshtastic plan"
	args["scope_in"] = []any{"IDriver adapter", "OGC CS endpoint wiring"}
	args["scope_out"] = []any{"Production deployment automation — out of arc."}
	args["depends_on"] = map[string]any{
		"research_artifact_loop": "loop-research-001",
	}

	res, err := exec.Execute(context.Background(), defaultCall(args))
	if err != nil {
		t.Fatalf("Execute err: %v", err)
	}
	if res.Error != "" {
		t.Fatalf("Result.Error = %q, want empty", res.Error)
	}

	triples := tp.snapshot()
	wantLoopEntityID := "c360.test.agent.agentic-loop.execution.loop-planner-abc"
	const wantTripleCount = 4
	if len(triples) != wantTripleCount {
		t.Errorf("triple count = %d, want %d", len(triples), wantTripleCount)
	}
	got := map[string]any{}
	for _, tr := range triples {
		if tr.Subject != wantLoopEntityID {
			t.Errorf("triple subject = %q, want %q", tr.Subject, wantLoopEntityID)
		}
		if tr.Source != toolSource {
			t.Errorf("triple source = %q, want %q", tr.Source, toolSource)
		}
		got[tr.Predicate] = tr.Object
	}

	if got[predicateRevision] != 1 {
		t.Errorf("revision triple = %v, want 1", got[predicateRevision])
	}
	if got[predicateEpicCount] != 2 {
		t.Errorf("epic_count triple = %v, want 2", got[predicateEpicCount])
	}
	pathStr, ok := got[predicatePath].(string)
	if !ok {
		t.Fatalf("path Object should be string, got %T", got[predicatePath])
	}
	if !strings.HasPrefix(pathStr, dir) || !strings.HasSuffix(pathStr, ".md") {
		t.Errorf("path = %q, want under %q with .md suffix", pathStr, dir)
	}

	// Markdown body sanity.
	body, err := os.ReadFile(pathStr)
	if err != nil {
		t.Fatalf("read rendered markdown: %v", err)
	}
	for _, want := range []string{"OSH Meshtastic plan", "## Goal", "## Context", "## Scope — In", "## Scope — Out", "## Epics", "## Depends on", "loop-research-001"} {
		if !strings.Contains(string(body), want) {
			t.Errorf("markdown body missing %q; got:\n%s", want, body)
		}
	}

	// Payload sanity.
	subject, data, calls := pub.snapshot()
	if calls != 1 {
		t.Errorf("publish calls = %d, want 1", calls)
	}
	if subject != "dev_via_spec.plan.loop-planner-abc" {
		t.Errorf("subject = %q, want dev_via_spec.plan.loop-planner-abc", subject)
	}
	var roundTrip map[string]any
	if err := json.Unmarshal(data, &roundTrip); err != nil {
		t.Fatalf("decode payload: %v", err)
	}
	if roundTrip["title"] != "OSH Meshtastic plan" {
		t.Errorf("payload title = %v, want OSH Meshtastic plan", roundTrip["title"])
	}
}

func TestExecute_TitleEmpty_LoopIDFallbackSlug(t *testing.T) {
	exec, tp, _, dir := newExecutor(t)
	args := defaultPlanArgs() // no title
	res, err := exec.Execute(context.Background(), defaultCall(args))
	if err != nil {
		t.Fatalf("Execute err: %v", err)
	}
	if res.Error != "" {
		t.Fatalf("Result.Error = %q, want empty", res.Error)
	}
	got := triplesByPredicate(tp.snapshot())
	pathStr := got[predicatePath].(string)
	if !strings.Contains(pathStr, "plan-loop-pla") {
		t.Errorf("fallback slug missing 'plan-loop-pla' stem (loop-planner-abc → first 8 chars 'loop-pla'); got %q", pathStr)
	}
	if !strings.HasPrefix(pathStr, dir) {
		t.Errorf("path = %q, want prefix %q", pathStr, dir)
	}
}

func TestExecute_OverwritesOnRerun(t *testing.T) {
	exec, _, _, dir := newExecutor(t)
	args := defaultPlanArgs()
	args["title"] = "Stable Plan"

	if _, err := exec.Execute(context.Background(), defaultCall(args)); err != nil {
		t.Fatalf("first Execute err: %v", err)
	}
	args["revision"] = 2
	if _, err := exec.Execute(context.Background(), defaultCall(args)); err != nil {
		t.Fatalf("second Execute err: %v", err)
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("read dir: %v", err)
	}
	mdCount := 0
	var mdName string
	for _, e := range entries {
		if strings.HasSuffix(e.Name(), ".md") {
			mdCount++
			mdName = e.Name()
		}
	}
	if mdCount != 1 {
		t.Errorf("expected 1 .md after two emissions (overwrite); got %d", mdCount)
	}
	body, err := os.ReadFile(dir + "/" + mdName)
	if err != nil {
		t.Fatalf("read overwritten file: %v", err)
	}
	if !strings.Contains(string(body), "Revision: **2**") {
		t.Errorf("overwritten file should reflect revision=2; got:\n%s", body)
	}
}

func TestExecute_TriplePublisherFails(t *testing.T) {
	exec, tp, pub, _ := newExecutor(t)
	tp.err = errors.New("graph-ingest unreachable")

	res, _ := exec.Execute(context.Background(), defaultCall(defaultPlanArgs()))
	if res.ErrorKind != agentic.ToolErrorNetwork {
		t.Errorf("ErrorKind = %v, want %v", res.ErrorKind, agentic.ToolErrorNetwork)
	}
	if _, _, calls := pub.snapshot(); calls != 0 {
		t.Errorf("payload publish should not fire after triple failure; got %d", calls)
	}
}

func triplesByPredicate(triples []message.Triple) map[string]any {
	out := make(map[string]any, len(triples))
	for _, tr := range triples {
		out[tr.Predicate] = tr.Object
	}
	return out
}

// TestExecute_DependsOnRendersFromLLMArgs pins the ADR-053 Phase 3c
// posture: emit_plan no longer reads the chain entity, so the rendered
// "Depends on" section reflects the LLM-supplied research_artifact_loop
// verbatim (the research pack's planner persona instructs leaving it
// unset, in which case the section is omitted — covered by the happy
// path's title-only variant). Replaces the retired chain-override tests
// (TestExecute_Chain* — smoke #8 run-5 D1/D2), which exercised the
// dev-via-spec arc's chain.slug.stem / chain.research_artifact.loop
// reads that never fired in the plan-first research pack.
func TestExecute_DependsOnRendersFromLLMArgs(t *testing.T) {
	exec, tp, _, _ := newExecutor(t)
	args := defaultPlanArgs()
	args["depends_on"] = map[string]any{
		"research_artifact_loop": "loop-research-001",
	}

	res, err := exec.Execute(context.Background(), defaultCall(args))
	if err != nil {
		t.Fatalf("Execute err: %v", err)
	}
	if res.Error != "" {
		t.Fatalf("Result.Error = %q, want empty", res.Error)
	}

	got := triplesByPredicate(tp.snapshot())
	pathStr, _ := got[predicatePath].(string)
	body, err := os.ReadFile(pathStr)
	if err != nil {
		t.Fatalf("read rendered markdown: %v", err)
	}
	if !strings.Contains(string(body), "loop-research-001") {
		t.Errorf("markdown depends_on must render the LLM-supplied loop ID; got body:\n%s", body)
	}
}

// CreateEntityWithTriples satisfies beta.159's widened TriplePublisher;
// the fake delegates to AddTriplesBatch so recording semantics are identical.
func (f *fakeTriplePublisher) CreateEntityWithTriples(ctx context.Context, _ string, _ message.Type, triples []message.Triple) error {
	return f.AddTriplesBatch(ctx, triples)
}
