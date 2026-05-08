package emitartifact

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/c360studio/semstreams/agentic"
	"github.com/c360studio/semstreams/message"
	"github.com/c360studio/semstreams/types"
)

// ---------------------------------------------------------------------
// Fakes
// ---------------------------------------------------------------------

// fakeTriplePublisher records every AddTriple call. By default, returns
// nil; tests that exercise the failure path set err.
type fakeTriplePublisher struct {
	mu      sync.Mutex
	triples []message.Triple
	err     error
}

func (f *fakeTriplePublisher) AddTriple(_ context.Context, triple message.Triple) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.err != nil {
		return f.err
	}
	f.triples = append(f.triples, triple)
	return nil
}

func (f *fakeTriplePublisher) snapshot() []message.Triple {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]message.Triple, len(f.triples))
	copy(out, f.triples)
	return out
}

// fakePublisher records every Publish call. Mirrors fakeTriplePublisher's
// concurrency posture (sync.Mutex + snapshot accessor) so a future
// test that exercises the executor concurrently doesn't get caught by
// asymmetric race-protection on the two fakes.
type fakePublisher struct {
	mu      sync.Mutex
	subject string
	data    []byte
	err     error
	calls   int
}

func (f *fakePublisher) Publish(_ context.Context, subject string, data []byte) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls++
	if f.err != nil {
		return f.err
	}
	f.subject = subject
	// Copy so caller mutation doesn't race with test assertions.
	f.data = append([]byte(nil), data...)
	return nil
}

// snapshot returns the most recent published subject + a copy of the
// data plus the call count, all read under the same mutex the writer
// uses. Use this rather than reading f.subject / f.data directly.
func (f *fakePublisher) snapshot() (subject string, data []byte, calls int) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.subject, append([]byte(nil), f.data...), f.calls
}

// ---------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------

func newExecutor(t *testing.T) (*Executor, *fakeTriplePublisher, *fakePublisher, string) {
	t.Helper()
	tmpDir := t.TempDir()
	tp := &fakeTriplePublisher{}
	pub := &fakePublisher{}
	exec := NewExecutor(tp, pub, types.PlatformMeta{Org: "c360", Platform: "semteams"}, nil, tmpDir)
	return exec, tp, pub, tmpDir
}

func defaultCall(args map[string]any) agentic.ToolCall {
	return agentic.ToolCall{
		ID:        "call-001",
		Name:      ToolName,
		Arguments: args,
		LoopID:    "loop-abc",
		TraceID:   "trace-xyz",
	}
}

func defaultArtifactArgs() map[string]any {
	return map[string]any{
		"revision": 1,
		"actors": []any{
			map[string]any{"name": "OSH driver framework", "role": "host of the IDriver interface"},
			map[string]any{"name": "OGC CS endpoints", "role": "exposes observation data"},
		},
		"integration_points": []any{
			map[string]any{"from": "OSH driver", "to": "OGC CS endpoints", "data": "observation messages", "direction": "write"},
		},
		"tasks": []any{
			"implement OSH IDriver backed by Meshtastic events",
			"expose observations on OGC CS endpoints",
		},
		"addressed_gaps": []any{},
		"open_gaps":      []any{},
	}
}

// ---------------------------------------------------------------------
// ListTools — schema sanity
// ---------------------------------------------------------------------

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
	for _, key := range []string{"revision", "actors", "integration_points", "tasks", "addressed_gaps", "open_gaps", "substrate_mutations", "test_harness"} {
		if _, ok := props[key]; !ok {
			t.Errorf("missing property %q in tool schema", key)
		}
	}
	// loop_id and produced_at are server-supplied — the LLM must not see
	// them as fields it can drive.
	for _, forbidden := range []string{"loop_id", "produced_at"} {
		if _, ok := props[forbidden]; ok {
			t.Errorf("schema must not advertise %q (server-supplied)", forbidden)
		}
	}
	required, _ := def.Parameters["required"].([]string)
	wantRequired := map[string]bool{"revision": true, "actors": true, "integration_points": true, "tasks": true}
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

// ---------------------------------------------------------------------
// Routing / preconditions
// ---------------------------------------------------------------------

func TestExecute_WrongToolName(t *testing.T) {
	exec, _, _, _ := newExecutor(t)
	call := defaultCall(defaultArtifactArgs())
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
	call := defaultCall(defaultArtifactArgs())
	call.LoopID = ""
	res, err := exec.Execute(context.Background(), call)
	if err != nil {
		t.Fatalf("Execute err = %v, want nil", err)
	}
	if res.ErrorKind != agentic.ToolErrorInternal {
		t.Errorf("ErrorKind = %v, want %v", res.ErrorKind, agentic.ToolErrorInternal)
	}
	if !strings.Contains(res.Error, "loop_id") {
		t.Errorf("Result.Error = %q, want contains 'loop_id'", res.Error)
	}
}

// ---------------------------------------------------------------------
// Argument validation
// ---------------------------------------------------------------------

func TestExecute_MissingRevision_FailsValidation(t *testing.T) {
	exec, _, _, _ := newExecutor(t)
	args := defaultArtifactArgs()
	delete(args, "revision")
	res, _ := exec.Execute(context.Background(), defaultCall(args))
	if res.ErrorKind != agentic.ToolErrorInvalidArgs {
		t.Errorf("ErrorKind = %v, want %v", res.ErrorKind, agentic.ToolErrorInvalidArgs)
	}
	if !strings.Contains(res.Error, "revision") {
		t.Errorf("Result.Error = %q, want contains 'revision'", res.Error)
	}
}

func TestExecute_RevisionZero_FailsValidation(t *testing.T) {
	exec, _, _, _ := newExecutor(t)
	args := defaultArtifactArgs()
	args["revision"] = 0
	res, _ := exec.Execute(context.Background(), defaultCall(args))
	if res.ErrorKind != agentic.ToolErrorInvalidArgs {
		t.Errorf("ErrorKind = %v, want %v", res.ErrorKind, agentic.ToolErrorInvalidArgs)
	}
}

func TestExecute_BadIntegrationPointDirection_FailsValidation(t *testing.T) {
	exec, _, _, _ := newExecutor(t)
	args := defaultArtifactArgs()
	args["integration_points"] = []any{
		map[string]any{"from": "A", "to": "B", "data": "x", "direction": "sideways"},
	}
	res, _ := exec.Execute(context.Background(), defaultCall(args))
	if res.ErrorKind != agentic.ToolErrorInvalidArgs {
		t.Errorf("ErrorKind = %v, want %v", res.ErrorKind, agentic.ToolErrorInvalidArgs)
	}
	if !strings.Contains(res.Error, "direction") {
		t.Errorf("Result.Error = %q, want contains 'direction'", res.Error)
	}
}

// LLM-supplied loop_id and produced_at are stripped — the tool always
// uses the call's loop_id (so the LLM cannot lie about which loop is
// emitting) and the wall-clock now (so audit traces match real time).
func TestExecute_LLMSuppliedLoopIDAndTimestampIgnored(t *testing.T) {
	exec, _, pub, _ := newExecutor(t)
	args := defaultArtifactArgs()
	args["loop_id"] = "loop-evil-spoof"
	args["produced_at"] = "1970-01-01T00:00:00Z"

	beforeCall := time.Now().UTC().Add(-1 * time.Second)
	res, err := exec.Execute(context.Background(), defaultCall(args))
	if err != nil {
		t.Fatalf("Execute err = %v", err)
	}
	if res.Error != "" {
		t.Fatalf("Result.Error = %q, want empty", res.Error)
	}

	if pub.subject != "research.artifact.loop-abc" {
		t.Errorf("subject = %q, want research.artifact.loop-abc (call.LoopID, not args)", pub.subject)
	}

	// Decode the published payload — produced_at should be ~now, not 1970.
	var sent map[string]any
	if err := json.Unmarshal(pub.data, &sent); err != nil {
		t.Fatalf("decode published payload: %v", err)
	}
	if got := sent["loop_id"]; got != "loop-abc" {
		t.Errorf("payload.loop_id = %v, want loop-abc", got)
	}
	tsStr, _ := sent["produced_at"].(string)
	ts, err := time.Parse(time.RFC3339Nano, tsStr)
	if err != nil {
		t.Fatalf("parse produced_at %q: %v", tsStr, err)
	}
	if ts.Before(beforeCall) {
		t.Errorf("produced_at = %v, must be ≥ wall-clock start of test (%v) — LLM-supplied 1970 timestamp leaked through", ts, beforeCall)
	}
}

// ---------------------------------------------------------------------
// Happy path — triples + payload + result
// ---------------------------------------------------------------------

func TestExecute_HappyPath_TripleSet(t *testing.T) {
	exec, tp, _, _ := newExecutor(t)
	res, err := exec.Execute(context.Background(), defaultCall(defaultArtifactArgs()))
	if err != nil {
		t.Fatalf("Execute err: %v", err)
	}
	if res.Error != "" {
		t.Fatalf("Result.Error = %q, want empty", res.Error)
	}

	triples := tp.snapshot()
	wantLoopEntityID := "c360.semteams.agent.agentic-loop.execution.loop-abc"

	gotPredicates := map[string]any{}
	for _, tr := range triples {
		if tr.Subject != wantLoopEntityID {
			t.Errorf("triple subject = %q, want %q", tr.Subject, wantLoopEntityID)
		}
		if tr.Source != toolSource {
			t.Errorf("triple source = %q, want %q", tr.Source, toolSource)
		}
		gotPredicates[tr.Predicate] = tr.Object
	}

	wantValues := map[string]any{
		predicateRevision:                  1,
		predicateActorsCount:               2,
		predicateIntegrationPointsCount:    1,
		predicateTasksCount:                2,
		predicateAddressedGapsCount:        0,
		predicateOpenGapsCount:             0,
		predicateLastRevisionMutationCount: 0,
	}
	for pred, want := range wantValues {
		got, ok := gotPredicates[pred]
		if !ok {
			t.Errorf("predicate %q missing from emitted triples", pred)
			continue
		}
		if got != want {
			t.Errorf("triple %q object = %v (%T), want %v (%T)", pred, got, got, want, want)
		}
	}

	if _, ok := gotPredicates[predicateProducedAt]; !ok {
		t.Errorf("predicate %q missing from emitted triples", predicateProducedAt)
	}
}

func TestExecute_HappyPath_PayloadPublishedToStableSubject(t *testing.T) {
	exec, _, pub, _ := newExecutor(t)
	_, err := exec.Execute(context.Background(), defaultCall(defaultArtifactArgs()))
	if err != nil {
		t.Fatalf("Execute err: %v", err)
	}
	subject, data, calls := pub.snapshot()
	if calls != 1 {
		t.Errorf("payload publish calls = %d, want 1", calls)
	}
	if subject != "research.artifact.loop-abc" {
		t.Errorf("subject = %q, want research.artifact.loop-abc", subject)
	}

	var roundTrip map[string]any
	if err := json.Unmarshal(data, &roundTrip); err != nil {
		t.Fatalf("decode payload: %v", err)
	}
	if got := roundTrip["loop_id"]; got != "loop-abc" {
		t.Errorf("payload.loop_id = %v, want loop-abc", got)
	}
	rev, _ := roundTrip["revision"].(float64) // JSON numbers → float64
	if int(rev) != 1 {
		t.Errorf("payload.revision = %v, want 1", rev)
	}
}

func TestExecute_ResultContent_ShapeAndCounts(t *testing.T) {
	exec, _, _, _ := newExecutor(t)
	res, err := exec.Execute(context.Background(), defaultCall(defaultArtifactArgs()))
	if err != nil {
		t.Fatalf("Execute err: %v", err)
	}
	if res.Error != "" {
		t.Fatalf("Result.Error = %q, want empty", res.Error)
	}

	var content struct {
		LoopID                    string `json:"loop_id"`
		Revision                  int    `json:"revision"`
		ActorsCount               int    `json:"actors_count"`
		IntegrationPointsCount    int    `json:"integration_points_count"`
		TasksCount                int    `json:"tasks_count"`
		LastRevisionMutationCount int    `json:"last_revision_mutation_count"`
		PayloadSubject            string `json:"payload_subject"`
		LoopEntityID              string `json:"loop_entity_id"`
	}
	if err := json.Unmarshal([]byte(res.Content), &content); err != nil {
		t.Fatalf("decode tool result: %v\nraw=%s", err, res.Content)
	}
	if content.LoopID != "loop-abc" {
		t.Errorf("loop_id = %q", content.LoopID)
	}
	if content.Revision != 1 {
		t.Errorf("revision = %d, want 1", content.Revision)
	}
	if content.ActorsCount != 2 {
		t.Errorf("actors_count = %d, want 2", content.ActorsCount)
	}
	if content.PayloadSubject != "research.artifact.loop-abc" {
		t.Errorf("payload_subject = %q", content.PayloadSubject)
	}
	if content.LoopEntityID != "c360.semteams.agent.agentic-loop.execution.loop-abc" {
		t.Errorf("loop_entity_id = %q", content.LoopEntityID)
	}
}

// ---------------------------------------------------------------------
// Mutation accounting
// ---------------------------------------------------------------------

// LatestRevisionMutationCount counts only mutations whose revision matches
// the artifact revision. Older-revision entries (carried forward in the
// append-only log) must not contribute. This is the predicate the
// stabilisation rule will gate on.
func TestExecute_MutationCount_OnlyCountsThisRevision(t *testing.T) {
	exec, tp, _, _ := newExecutor(t)

	args := defaultArtifactArgs()
	args["revision"] = 3
	args["substrate_mutations"] = []any{
		map[string]any{
			"tool":      "add_source_repo",
			"loop_id":   "loop-abc",
			"revision":  1,
			"status":    "executed",
			"timestamp": time.Now().UTC().Format(time.RFC3339),
		},
		map[string]any{
			"tool":      "add_source_repo",
			"loop_id":   "loop-abc",
			"revision":  2,
			"status":    "executed",
			"timestamp": time.Now().UTC().Format(time.RFC3339),
		},
		// Revision 3 has no new mutations — this is the "stabilised" shape.
	}

	res, err := exec.Execute(context.Background(), defaultCall(args))
	if err != nil {
		t.Fatalf("Execute err: %v", err)
	}
	if res.Error != "" {
		t.Fatalf("Result.Error = %q, want empty", res.Error)
	}

	gotPredicates := map[string]any{}
	for _, tr := range tp.snapshot() {
		gotPredicates[tr.Predicate] = tr.Object
	}
	if got := gotPredicates[predicateLastRevisionMutationCount]; got != 0 {
		t.Errorf("last_revision_mutation_count = %v, want 0 (revision 3 has no new mutations)", got)
	}
	if got := gotPredicates[predicateRevision]; got != 3 {
		t.Errorf("revision = %v, want 3", got)
	}
}

func TestExecute_MutationCount_WithCurrentRevisionEntry(t *testing.T) {
	exec, tp, _, _ := newExecutor(t)
	args := defaultArtifactArgs()
	args["revision"] = 2
	args["substrate_mutations"] = []any{
		map[string]any{
			"tool":      "add_source_repo",
			"loop_id":   "loop-abc",
			"revision":  1,
			"status":    "executed",
			"timestamp": time.Now().UTC().Format(time.RFC3339),
		},
		map[string]any{
			"tool":      "add_source_repo",
			"loop_id":   "loop-abc",
			"revision":  2,
			"status":    "executed",
			"timestamp": time.Now().UTC().Format(time.RFC3339),
		},
	}
	if _, err := exec.Execute(context.Background(), defaultCall(args)); err != nil {
		t.Fatalf("Execute err: %v", err)
	}

	gotPredicates := map[string]any{}
	for _, tr := range tp.snapshot() {
		gotPredicates[tr.Predicate] = tr.Object
	}
	if got := gotPredicates[predicateLastRevisionMutationCount]; got != 1 {
		t.Errorf("last_revision_mutation_count = %v, want 1 (one mutation at revision 2)", got)
	}
}

// ---------------------------------------------------------------------
// R3.7.1.b — test_harness field handling
// ---------------------------------------------------------------------

func TestExecute_TestHarnessSet_EmitsTestHarnessTriple(t *testing.T) {
	exec, tp, pub, _ := newExecutor(t)
	args := defaultArtifactArgs()
	args["test_harness"] = "meshtasticd-3.x"

	res, err := exec.Execute(context.Background(), defaultCall(args))
	if err != nil {
		t.Fatalf("Execute err: %v", err)
	}
	if res.Error != "" {
		t.Fatalf("Result.Error = %q, want empty", res.Error)
	}

	gotPredicates := map[string]any{}
	for _, tr := range tp.snapshot() {
		gotPredicates[tr.Predicate] = tr.Object
	}
	got, ok := gotPredicates[predicateTestHarness]
	if !ok {
		t.Fatalf("test_harness triple not emitted; predicates: %v", gotPredicates)
	}
	if got != "meshtasticd-3.x" {
		t.Errorf("test_harness object = %v, want meshtasticd-3.x", got)
	}

	// Payload also carries the field round-trip.
	_, data, _ := pub.snapshot()
	var roundTrip map[string]any
	if err := json.Unmarshal(data, &roundTrip); err != nil {
		t.Fatalf("decode payload: %v", err)
	}
	if roundTrip["test_harness"] != "meshtasticd-3.x" {
		t.Errorf("payload.test_harness = %v, want meshtasticd-3.x", roundTrip["test_harness"])
	}

	// Result content (next-turn LLM-visible) carries the test_harness key.
	var resultMap map[string]any
	if err := json.Unmarshal([]byte(res.Content), &resultMap); err != nil {
		t.Fatalf("decode result content: %v", err)
	}
	if resultMap["test_harness"] != "meshtasticd-3.x" {
		t.Errorf("result.test_harness = %v, want meshtasticd-3.x", resultMap["test_harness"])
	}
}

func TestExecute_TestHarnessUnset_OmitsTestHarnessTriple(t *testing.T) {
	exec, tp, pub, _ := newExecutor(t)
	args := defaultArtifactArgs()
	// Explicit catalog-miss path — researcher flags the gap in open_gaps.
	args["open_gaps"] = []any{"needs_test_harness: real Meshtastic radio over LoRa"}

	res, err := exec.Execute(context.Background(), defaultCall(args))
	if err != nil {
		t.Fatalf("Execute err: %v", err)
	}
	if res.Error != "" {
		t.Fatalf("Result.Error = %q, want empty", res.Error)
	}

	for _, tr := range tp.snapshot() {
		if tr.Predicate == predicateTestHarness {
			t.Errorf("test_harness triple emitted with empty test_harness field; object=%v", tr.Object)
		}
	}

	// Payload omits the field via omitempty so older v1 consumers see no
	// drift.
	_, data, _ := pub.snapshot()
	if got := string(data); strings.Contains(got, `"test_harness"`) {
		t.Errorf("expected test_harness omitted from payload, got %s", got)
	}

	// Result content (next-turn LLM-visible) also omits the key so the
	// next-turn LLM doesn't see "test_harness":"" as an explicit empty.
	if got := res.Content; strings.Contains(got, `"test_harness"`) {
		t.Errorf("expected test_harness omitted from result content, got %s", got)
	}
}

// ---------------------------------------------------------------------
// Failure paths
// ---------------------------------------------------------------------

func TestExecute_TriplePublisherFails_NoPayloadPublished(t *testing.T) {
	exec, tp, pub, _ := newExecutor(t)
	tp.err = errors.New("graph-ingest unreachable")

	res, err := exec.Execute(context.Background(), defaultCall(defaultArtifactArgs()))
	if err != nil {
		t.Fatalf("Execute err = %v, want nil", err)
	}
	if res.ErrorKind != agentic.ToolErrorNetwork {
		t.Errorf("ErrorKind = %v, want %v", res.ErrorKind, agentic.ToolErrorNetwork)
	}
	if !strings.Contains(res.Error, "graph-ingest unreachable") {
		t.Errorf("Result.Error = %q, want contains 'graph-ingest unreachable'", res.Error)
	}
	if pub.calls != 0 {
		t.Errorf("payload publish calls = %d, want 0 (must short-circuit on triple failure)", pub.calls)
	}
}

func TestExecute_PayloadPublishFails_ReportsNetworkError(t *testing.T) {
	exec, _, pub, _ := newExecutor(t)
	pub.err = errors.New("nats not connected")

	res, err := exec.Execute(context.Background(), defaultCall(defaultArtifactArgs()))
	if err != nil {
		t.Fatalf("Execute err = %v, want nil", err)
	}
	if res.ErrorKind != agentic.ToolErrorNetwork {
		t.Errorf("ErrorKind = %v, want %v", res.ErrorKind, agentic.ToolErrorNetwork)
	}
	if !strings.Contains(res.Error, "nats not connected") {
		t.Errorf("Result.Error = %q, want contains 'nats not connected'", res.Error)
	}
	if !strings.Contains(res.Error, "research.artifact.loop-abc") {
		t.Errorf("Result.Error = %q, want subject in error message", res.Error)
	}
}

// ---------------------------------------------------------------------
// PR C Phase C1 — markdown render + research.artifact.path triple
// ---------------------------------------------------------------------

func TestExecute_MarkdownRender_PathTripleStamped(t *testing.T) {
	exec, tp, _, dir := newExecutor(t)

	args := defaultArtifactArgs()
	args["title"] = "OSH Meshtastic Driver"

	res, err := exec.Execute(context.Background(), defaultCall(args))
	if err != nil {
		t.Fatalf("Execute err: %v", err)
	}
	if res.Error != "" {
		t.Fatalf("Result.Error = %q, want empty", res.Error)
	}

	// Path triple stamped on the loop entity (the research milestone
	// stamper reads it at reviewer-approves time and mirrors to chain).
	pathTriple, ok := triplesByPredicate(tp.snapshot())[predicateArtifactPath]
	if !ok {
		t.Fatal("research.artifact.path triple not stamped")
	}
	relPath, ok := pathTriple.Object.(string)
	if !ok {
		t.Fatalf("path triple Object should be string, got %T", pathTriple.Object)
	}
	if !strings.HasPrefix(relPath, dir) {
		t.Errorf("path triple object = %q, expected to start with outputDir %q", relPath, dir)
	}
	if !strings.HasSuffix(relPath, ".md") {
		t.Errorf("path triple object = %q, expected .md extension", relPath)
	}

	// File on disk.
	body, err := readFileContent(t, relPath)
	if err != nil {
		t.Fatalf("read rendered markdown: %v", err)
	}
	if !strings.Contains(body, "OSH Meshtastic Driver") {
		t.Errorf("markdown body missing title heading; got:\n%s", body)
	}
	if !strings.Contains(body, "Revision: **1**") {
		t.Errorf("markdown body missing revision metadata; got:\n%s", body)
	}
	if !strings.Contains(body, "OSH driver framework") {
		t.Errorf("markdown body missing actor; got:\n%s", body)
	}
}

func TestExecute_MarkdownRender_TitleEmpty_LoopIDFallbackSlug(t *testing.T) {
	exec, tp, _, dir := newExecutor(t)

	args := defaultArtifactArgs() // no title
	res, err := exec.Execute(context.Background(), defaultCall(args))
	if err != nil {
		t.Fatalf("Execute err: %v", err)
	}
	if res.Error != "" {
		t.Fatalf("Result.Error = %q, want empty", res.Error)
	}

	pathTriple, ok := triplesByPredicate(tp.snapshot())[predicateArtifactPath]
	if !ok {
		t.Fatal("research.artifact.path triple not stamped")
	}
	relPath, _ := pathTriple.Object.(string)
	// Slug should embed the loop_id fallback ("loop-abc" → first 8 chars).
	if !strings.Contains(relPath, "research-loop-abc") {
		t.Errorf("fallback slug missing loop-id stem; got %q", relPath)
	}
	if !strings.HasPrefix(relPath, dir) {
		t.Errorf("path = %q, want prefix %q", relPath, dir)
	}
}

func TestExecute_MarkdownRender_OverwritesOnRerun(t *testing.T) {
	exec, _, _, _ := newExecutor(t)
	args := defaultArtifactArgs()
	args["title"] = "Stable Title"

	// First emission.
	if _, err := exec.Execute(context.Background(), defaultCall(args)); err != nil {
		t.Fatalf("first Execute err: %v", err)
	}
	// Second emission with same title (and same date) → same slug → overwrite.
	args["revision"] = 2
	if _, err := exec.Execute(context.Background(), defaultCall(args)); err != nil {
		t.Fatalf("second Execute err: %v", err)
	}
	// No assertion on file count here — overwrite means one .md, not two.
	// Fail-mode would be "create a different filename"; the slug derivation
	// is deterministic so this should be a no-op on disk.
}

func triplesByPredicate(triples []message.Triple) map[string]message.Triple {
	out := make(map[string]message.Triple, len(triples))
	for _, t := range triples {
		out[t.Predicate] = t
	}
	return out
}

func readFileContent(t *testing.T, path string) (string, error) {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	return string(data), nil
}
