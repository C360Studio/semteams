package emitconsensus

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

	"github.com/c360studio/semteams/cmd/semteams/chain"
)

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
		LoopID:    "loop-challenger-abc",
		TraceID:   "trace-x",
	}
}

func defaultConsensusArgs() map[string]any {
	return map[string]any{
		"summary": "Plan delivers an OSH IDriver adapter over Meshtastic LoRa transport. No execution-blocking concerns surface; epic decomposition grounds each integration boundary the reviewer cited.",
		"chain_consensus": []any{
			"OSH driver framework: implementation against IDriver interface (epic 1).",
			"Meshtastic radio: receive() / publish() over LoRa (epic 1, integration boundary).",
			"OGC CS endpoints: surfaces observations from the IDriver (epic 2).",
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
	for _, key := range []string{"title", "summary", "chain_consensus", "considered_concerns", "depends_on"} {
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
	wantRequired := map[string]bool{"summary": true, "chain_consensus": true}
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
	call := defaultCall(defaultConsensusArgs())
	call.Name = "something-else"
	res, _ := exec.Execute(context.Background(), call)
	if res.ErrorKind != agentic.ToolErrorNotFound {
		t.Errorf("ErrorKind = %v, want %v", res.ErrorKind, agentic.ToolErrorNotFound)
	}
}

func TestExecute_MissingLoopID(t *testing.T) {
	exec, _, _, _ := newExecutor(t)
	call := defaultCall(defaultConsensusArgs())
	call.LoopID = ""
	res, _ := exec.Execute(context.Background(), call)
	if res.ErrorKind != agentic.ToolErrorInternal {
		t.Errorf("ErrorKind = %v, want %v", res.ErrorKind, agentic.ToolErrorInternal)
	}
}

func TestExecute_MissingSummary_FailsValidation(t *testing.T) {
	exec, _, _, _ := newExecutor(t)
	args := defaultConsensusArgs()
	delete(args, "summary")
	res, _ := exec.Execute(context.Background(), defaultCall(args))
	if res.ErrorKind != agentic.ToolErrorInvalidArgs {
		t.Errorf("ErrorKind = %v, want %v", res.ErrorKind, agentic.ToolErrorInvalidArgs)
	}
}

func TestExecute_EmptyChainConsensus_FailsValidation(t *testing.T) {
	exec, _, _, _ := newExecutor(t)
	args := defaultConsensusArgs()
	args["chain_consensus"] = []any{}
	res, _ := exec.Execute(context.Background(), defaultCall(args))
	if res.ErrorKind != agentic.ToolErrorInvalidArgs {
		t.Errorf("ErrorKind = %v, want %v", res.ErrorKind, agentic.ToolErrorInvalidArgs)
	}
	if !strings.Contains(res.Error, "chain_consensus") {
		t.Errorf("Result.Error = %q, want contains 'chain_consensus'", res.Error)
	}
}

func TestExecute_HappyPath_TriplesAndPayload(t *testing.T) {
	exec, tp, pub, dir := newExecutor(t)
	args := defaultConsensusArgs()
	args["title"] = "OSH Meshtastic consensus"
	args["considered_concerns"] = []any{
		"Initial planner pass missed the NODEINFO_APP packet type — resolved in revision 2 by adding it to scope.",
	}
	args["depends_on"] = map[string]any{
		"plan_loop":     "loop-planner-001",
		"reviewer_loop": "loop-reviewer-002",
	}

	res, err := exec.Execute(context.Background(), defaultCall(args))
	if err != nil {
		t.Fatalf("Execute err: %v", err)
	}
	if res.Error != "" {
		t.Fatalf("Result.Error = %q, want empty", res.Error)
	}

	triples := tp.snapshot()
	wantLoopEntityID := "c360.test.agent.agentic-loop.execution.loop-challenger-abc"
	const wantTripleCount = 3 // chain_consensus_count, generated_at, path
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

	if got[predicateChainConsensusCount] != 3 {
		t.Errorf("chain_consensus_count = %v, want 3", got[predicateChainConsensusCount])
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
	for _, want := range []string{"OSH Meshtastic consensus", "## Summary", "## Chain consensus", "## Considered concerns", "## Depends on", "loop-planner-001", "loop-reviewer-002", "OSH driver framework"} {
		if !strings.Contains(string(body), want) {
			t.Errorf("markdown body missing %q; got:\n%s", want, body)
		}
	}

	// Payload sanity.
	subject, data, calls := pub.snapshot()
	if calls != 1 {
		t.Errorf("publish calls = %d, want 1", calls)
	}
	if subject != "dev_via_spec.consensus.loop-challenger-abc" {
		t.Errorf("subject = %q, want dev_via_spec.consensus.loop-challenger-abc", subject)
	}
	var roundTrip map[string]any
	if err := json.Unmarshal(data, &roundTrip); err != nil {
		t.Fatalf("decode payload: %v", err)
	}
	if roundTrip["title"] != "OSH Meshtastic consensus" {
		t.Errorf("payload title = %v, want OSH Meshtastic consensus", roundTrip["title"])
	}
}

func TestExecute_TitleEmpty_LoopIDFallbackSlug(t *testing.T) {
	exec, tp, _, dir := newExecutor(t)
	args := defaultConsensusArgs() // no title
	res, err := exec.Execute(context.Background(), defaultCall(args))
	if err != nil {
		t.Fatalf("Execute err: %v", err)
	}
	if res.Error != "" {
		t.Fatalf("Result.Error = %q, want empty", res.Error)
	}
	got := triplesByPredicate(tp.snapshot())
	pathStr := got[predicatePath].(string)
	// loop-challenger-abc → first 8 chars "loop-cha" → fallback "consensus-loop-cha"
	if !strings.Contains(pathStr, "consensus-loop-cha") {
		t.Errorf("fallback slug missing 'consensus-loop-cha' stem; got %q", pathStr)
	}
	if !strings.HasPrefix(pathStr, dir) {
		t.Errorf("path = %q, want prefix %q", pathStr, dir)
	}
}

func TestExecute_OverwritesOnRerun(t *testing.T) {
	exec, _, _, dir := newExecutor(t)
	args := defaultConsensusArgs()
	args["title"] = "Stable Consensus"

	if _, err := exec.Execute(context.Background(), defaultCall(args)); err != nil {
		t.Fatalf("first Execute err: %v", err)
	}
	args["summary"] = "Updated rationale on second pass."
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
	if !strings.Contains(string(body), "Updated rationale on second pass.") {
		t.Errorf("overwritten file should reflect updated summary; got:\n%s", body)
	}
}

func TestExecute_TriplePublisherFails(t *testing.T) {
	exec, tp, pub, _ := newExecutor(t)
	tp.err = errors.New("graph-ingest unreachable")
	res, _ := exec.Execute(context.Background(), defaultCall(defaultConsensusArgs()))
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

// stubChainReader returns a fixed (chainEntityID, triples) pair to
// every ReadChainFor call. Mirrors emitplan's stub. Errors land via
// err.
type stubChainReader struct {
	entityID string
	triples  map[string]any
	err      error
	calls    int
	mu       sync.Mutex
}

func (s *stubChainReader) ReadChainFor(_ context.Context, _ string) (string, map[string]any, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.calls++
	if s.err != nil {
		return s.entityID, nil, s.err
	}
	return s.entityID, s.triples, nil
}

// spyChainReader records the loop ID it was invoked with — used to
// pin the smoke #8 run-6 hotfix that the chain ancestry walk anchors
// on a completed-ancestor metadata key, not the running call.LoopID.
type spyChainReader struct {
	entityID   string
	triples    map[string]any
	lastLoopID string
	mu         sync.Mutex
}

func (s *spyChainReader) ReadChainFor(_ context.Context, loopID string) (string, map[string]any, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.lastLoopID = loopID
	return s.entityID, s.triples, nil
}

// TestExecute_ChainAnchorsOnPriorLoopMetadata pins the smoke #8
// run-6 hotfix at the consensus seam: emit-tools MUST anchor the
// chain walk on a completed ancestor's loop_id from task-property
// metadata, not the running loop's own LoopID. The challenger spawn
// rule (rules/dev-via-spec/03-reviewer-approved-to-challenger.json)
// always sets prior_loop_id to the upstream completed reviewer.
func TestExecute_ChainAnchorsOnPriorLoopMetadata(t *testing.T) {
	exec, _, _, _ := newExecutor(t)
	stub := &spyChainReader{
		entityID: "c360.test.agent.chain.execution.dispatch_root",
		triples:  map[string]any{},
	}
	exec.SetChainReader(stub)

	call := defaultCall(defaultConsensusArgs())
	call.Metadata = map[string]any{
		"prior_loop_id": "reviewer-completed",
	}
	if _, err := exec.Execute(context.Background(), call); err != nil {
		t.Fatalf("Execute err: %v", err)
	}
	if stub.lastLoopID != "reviewer-completed" {
		t.Errorf("ReadChainFor lastLoopID = %q, want metadata anchor 'reviewer-completed' (running LoopID %q must NOT be used when metadata is set)", stub.lastLoopID, call.LoopID)
	}
}

// TestExecute_ChainAnchorFallsBackToCallLoopID pins fallback when no
// metadata anchor is set — preserves pre-hotfix behaviour for tests
// and deployments without the spawn convention.
func TestExecute_ChainAnchorFallsBackToCallLoopID(t *testing.T) {
	exec, _, _, _ := newExecutor(t)
	stub := &spyChainReader{
		entityID: "c360.test.agent.chain.execution.dispatch_root",
		triples:  map[string]any{},
	}
	exec.SetChainReader(stub)

	call := defaultCall(defaultConsensusArgs())
	if _, err := exec.Execute(context.Background(), call); err != nil {
		t.Fatalf("Execute err: %v", err)
	}
	if stub.lastLoopID != call.LoopID {
		t.Errorf("ReadChainFor lastLoopID = %q, want call.LoopID %q (fallback)", stub.lastLoopID, call.LoopID)
	}
}

// TestExecute_ChainSlugStemOverridesTitleSlug pins the smoke #8 run-5
// D2 fix at the consensus seam. Chain stem keeps the slug stable as
// "<stem>-consensus" even when the challenger's title drifts (e.g.
// "OSH Meshtastic IDriver consensus" — extra "i" — would otherwise
// produce a divergent slug from the planner's
// "OSH Meshtastic Driver plan").
func TestExecute_ChainSlugStemOverridesTitleSlug(t *testing.T) {
	exec, tp, _, dir := newExecutor(t)
	exec.SetChainReader(&stubChainReader{
		entityID: "c360.test.agent.chain.execution.dispatch_root",
		triples: map[string]any{
			chain.PredicateSlugStem: "2026-05-09-osh-meshtastic-driver",
		},
	})

	args := defaultConsensusArgs()
	args["title"] = "OSH Meshtastic IDriver consensus"

	res, err := exec.Execute(context.Background(), defaultCall(args))
	if err != nil {
		t.Fatalf("Execute err: %v", err)
	}
	if res.Error != "" {
		t.Fatalf("Result.Error = %q, want empty", res.Error)
	}

	got := triplesByPredicate(tp.snapshot())
	pathStr, _ := got[predicatePath].(string)
	const wantSuffix = "/2026-05-09-osh-meshtastic-driver-consensus.md"
	if !strings.HasSuffix(pathStr, wantSuffix) {
		t.Errorf("rendered path = %q, want suffix %q (chain.slug.stem must override drifting title)", pathStr, wantSuffix)
	}
	if !strings.HasPrefix(pathStr, dir) {
		t.Errorf("path = %q, want prefix %q", pathStr, dir)
	}
}

// TestExecute_ChainPlanLoopsOverrideDependsOn pins the smoke #8 run-5
// D1 fix at the consensus seam. Chain entity carries chain.plan_loop
// (planner) and chain.plan_reviewer_loop (reviewer); both must
// override the LLM-supplied depends_on.{plan_loop,reviewer_loop}.
// Smoke #8 run-5 evidence: challenger filled BOTH slots with the
// reviewer's loop ID — chain entity's distinct values fix that.
func TestExecute_ChainPlanLoopsOverrideDependsOn(t *testing.T) {
	exec, tp, _, _ := newExecutor(t)
	exec.SetChainReader(&stubChainReader{
		entityID: "c360.test.agent.chain.execution.dispatch_root",
		triples: map[string]any{
			chain.PredicatePlanLoop:         "planner_canonical",
			chain.PredicatePlanReviewerLoop: "reviewer_canonical",
		},
	})

	args := defaultConsensusArgs()
	args["depends_on"] = map[string]any{
		// Challenger's local guess — both slots got the reviewer's ID
		// in smoke #8 run-5. Chain entity's distinct values must
		// replace these.
		"plan_loop":     "reviewer_wrong",
		"reviewer_loop": "reviewer_wrong",
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
	if !strings.Contains(string(body), "planner_canonical") {
		t.Errorf("markdown depends_on must use chain-canonical plan_loop; got body:\n%s", body)
	}
	if !strings.Contains(string(body), "reviewer_canonical") {
		t.Errorf("markdown depends_on must use chain-canonical reviewer_loop; got body:\n%s", body)
	}
	if strings.Contains(string(body), "reviewer_wrong") {
		t.Errorf("LLM-supplied depends_on must NOT appear when chain entity has the canonical values; got body:\n%s", body)
	}
}

// TestExecute_ChainReadFailureFallsBackToLLMValues pins the fail-soft
// contract: chain read errors fall back to LLM-supplied values + the
// title-derived slug, logged at Warn. Same shape as emitplan's
// equivalent test.
func TestExecute_ChainReadFailureFallsBackToLLMValues(t *testing.T) {
	exec, tp, _, _ := newExecutor(t)
	stub := &stubChainReader{
		entityID: "c360.test.agent.chain.execution.dispatch_root",
		err:      errors.New("graph timeout"),
	}
	exec.SetChainReader(stub)

	args := defaultConsensusArgs()
	args["title"] = "OSH Meshtastic consensus"
	args["depends_on"] = map[string]any{
		"plan_loop":     "loop-planner-001",
		"reviewer_loop": "loop-reviewer-001",
	}

	res, err := exec.Execute(context.Background(), defaultCall(args))
	if err != nil {
		t.Fatalf("Execute err: %v", err)
	}
	if res.Error != "" {
		t.Fatalf("Result.Error = %q, want empty (chain read failure must not surface to caller)", res.Error)
	}
	if stub.calls != 1 {
		t.Errorf("stub ReadChainFor calls = %d, want 1", stub.calls)
	}

	got := triplesByPredicate(tp.snapshot())
	pathStr, _ := got[predicatePath].(string)
	if !strings.HasSuffix(pathStr, "-osh-meshtastic-consensus.md") {
		t.Errorf("path = %q, want title-derived slug fallback", pathStr)
	}
	body, _ := os.ReadFile(pathStr)
	if !strings.Contains(string(body), "loop-planner-001") || !strings.Contains(string(body), "loop-reviewer-001") {
		t.Errorf("markdown should fall back to LLM-supplied depends_on when chain read fails; got body:\n%s", body)
	}
}
