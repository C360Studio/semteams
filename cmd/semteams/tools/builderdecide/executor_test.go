package builderdecide

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"testing"

	"github.com/c360studio/semstreams/agentic"
	"github.com/c360studio/semstreams/message"
	"github.com/c360studio/semstreams/types"
	agvocab "github.com/c360studio/semstreams/vocabulary/agentic"
)

// ---------------------------------------------------------------------
// Fakes
// ---------------------------------------------------------------------

// fakeTriplePublisher records every AddTriple call. Mirrors the pattern
// from emitspecartifact's tests so reviewers see one shape across the
// product-shell tool fleet.
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

// ---------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------

func newExecutor(t *testing.T) (*Executor, *fakeTriplePublisher) {
	t.Helper()
	tp := &fakeTriplePublisher{}
	exec := NewExecutor(tp, types.PlatformMeta{Org: "c360", Platform: "semteams"}, nil)
	return exec, tp
}

func defaultCall(args map[string]any) agentic.ToolCall {
	return agentic.ToolCall{
		ID:        "call-001",
		Name:      ToolName,
		Arguments: args,
		LoopID:    "loop-builder-abc",
		TraceID:   "trace-xyz",
	}
}

func passingArgs() map[string]any {
	return map[string]any{
		"action":           ActionTestsPassing,
		"reason":           "all OSGi bundle tests passed; surefire harness reports 5/5",
		"tests_run":        5,
		"tests_passed":     5,
		"tests_failed":     0,
		"artifact_summary": "OSGi bundle target/osh-driver-1.0.0.jar built; 5 tests, 5 passing",
	}
}

func failingArgs() map[string]any {
	return map[string]any{
		"action":          ActionTestsFailing,
		"reason":          "two unit tests failed in MeshSensorAdapterTest",
		"tests_run":       5,
		"tests_failed":    2,
		"failure_summary": "MeshSensorAdapterTest.testParse — expected MeshPacket envelope, got null",
		"retry_hint":      "thread the parser context into the adapter constructor; current call sites pass nil",
	}
}

func clarificationArgs() map[string]any {
	return map[string]any{
		"action":            ActionNeedsClarification,
		"reason":            "spec underspecifies the OGC CS endpoint contract for the radio-update path",
		"blocking_question": "Should the adapter publish to /sensor/update or /sensor/event when the radio packet is a heartbeat?",
	}
}

// ---------------------------------------------------------------------
// ListTools — schema sanity
// ---------------------------------------------------------------------

func TestListTools_SchemaShape(t *testing.T) {
	exec, _ := newExecutor(t)
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
	for _, key := range []string{"action", "reason", "tests_run", "tests_passed", "tests_failed", "artifact_summary", "failure_summary", "retry_hint", "blocking_question"} {
		if _, ok := props[key]; !ok {
			t.Errorf("missing property %q in tool schema", key)
		}
	}
	required, _ := def.Parameters["required"].([]string)
	wantRequired := map[string]bool{"action": true, "reason": true}
	if len(required) != len(wantRequired) {
		t.Errorf("required = %v, want %v (per-action evidence is enforced by the executor, not JSON schema)", required, wantRequired)
	}
	actionProp, _ := props["action"].(map[string]any)
	enum, _ := actionProp["enum"].([]string)
	wantEnum := map[string]bool{ActionTestsPassing: true, ActionTestsFailing: true, ActionNeedsClarification: true}
	if len(enum) != len(wantEnum) {
		t.Errorf("action enum = %v, want %d values", enum, len(wantEnum))
	}
	for _, v := range enum {
		if !wantEnum[v] {
			t.Errorf("unexpected enum value %q", v)
		}
	}
}

// ---------------------------------------------------------------------
// Routing / preconditions
// ---------------------------------------------------------------------

func TestExecute_WrongToolName(t *testing.T) {
	exec, _ := newExecutor(t)
	call := defaultCall(passingArgs())
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
	exec, _ := newExecutor(t)
	call := defaultCall(passingArgs())
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
// Action-shape validation — generic
// ---------------------------------------------------------------------

func TestExecute_MissingAction_FailsValidation(t *testing.T) {
	exec, _ := newExecutor(t)
	args := passingArgs()
	delete(args, "action")
	res, _ := exec.Execute(context.Background(), defaultCall(args))
	if res.ErrorKind != agentic.ToolErrorInvalidArgs {
		t.Errorf("ErrorKind = %v, want %v", res.ErrorKind, agentic.ToolErrorInvalidArgs)
	}
	if !strings.Contains(res.Error, "action") {
		t.Errorf("Result.Error = %q, want contains 'action'", res.Error)
	}
}

func TestExecute_UnknownAction_FailsValidation(t *testing.T) {
	exec, _ := newExecutor(t)
	args := passingArgs()
	args["action"] = "ship_it_yolo"
	res, _ := exec.Execute(context.Background(), defaultCall(args))
	if res.ErrorKind != agentic.ToolErrorInvalidArgs {
		t.Errorf("ErrorKind = %v, want %v", res.ErrorKind, agentic.ToolErrorInvalidArgs)
	}
	if !strings.Contains(res.Error, "action must be one of") {
		t.Errorf("Result.Error = %q, want contains 'action must be one of'", res.Error)
	}
}

func TestExecute_MissingReason_FailsValidation(t *testing.T) {
	exec, _ := newExecutor(t)
	args := passingArgs()
	delete(args, "reason")
	res, _ := exec.Execute(context.Background(), defaultCall(args))
	if res.ErrorKind != agentic.ToolErrorInvalidArgs {
		t.Errorf("ErrorKind = %v, want %v", res.ErrorKind, agentic.ToolErrorInvalidArgs)
	}
	if !strings.Contains(res.Error, "reason") {
		t.Errorf("Result.Error = %q, want contains 'reason'", res.Error)
	}
}

// ---------------------------------------------------------------------
// Action-shape validation — tests_passing
// ---------------------------------------------------------------------

func TestExecute_TestsPassing_HappyPath(t *testing.T) {
	exec, tp := newExecutor(t)
	res, err := exec.Execute(context.Background(), defaultCall(passingArgs()))
	if err != nil {
		t.Fatalf("Execute err = %v, want nil", err)
	}
	if res.Error != "" {
		t.Fatalf("tool error: %s", res.Error)
	}
	if !res.StopLoop {
		t.Errorf("expected StopLoop=true")
	}

	tr := tp.snapshot()
	if len(tr) != 6 {
		t.Fatalf("expected 6 triples (action+reason+4 evidence), got %d: %+v", len(tr), tr)
	}

	wantSubject := "c360.semteams.agent.agentic-loop.execution.loop-builder-abc"
	for _, x := range tr {
		if x.Subject != wantSubject {
			t.Errorf("triple subject = %q, want %q", x.Subject, wantSubject)
		}
	}

	byPredicate := map[string]any{}
	for _, x := range tr {
		byPredicate[x.Predicate] = x.Object
	}
	if got := byPredicate[agvocab.CoordinatorNextAction]; got != ActionTestsPassing {
		t.Errorf("coordinator.next_action = %v, want %v", got, ActionTestsPassing)
	}
	if got := byPredicate[predicateTestsRun]; got != 5 {
		t.Errorf("tests_run = %v, want 5", got)
	}
	if got := byPredicate[predicateTestsPassed]; got != 5 {
		t.Errorf("tests_passed = %v, want 5", got)
	}
	if got := byPredicate[predicateTestsFailed]; got != 0 {
		t.Errorf("tests_failed = %v, want 0", got)
	}
	if got := byPredicate[predicateArtifactSummary]; got != passingArgs()["artifact_summary"] {
		t.Errorf("artifact_summary = %v, want %v", got, passingArgs()["artifact_summary"])
	}

	// Content must be JSON-decodable back to the args.
	var roundtrip map[string]any
	if err := json.Unmarshal([]byte(res.Content), &roundtrip); err != nil {
		t.Fatalf("Content not valid JSON: %v (%q)", err, res.Content)
	}
	if roundtrip["action"] != ActionTestsPassing {
		t.Errorf("Content.action = %v, want %v", roundtrip["action"], ActionTestsPassing)
	}
}

// ADR-032 §15 closes the "exit 0 with no tests" loophole — tests_passing with
// tests_run=0 must reject before any triples are published.
func TestExecute_TestsPassing_RejectsZeroTestsRun(t *testing.T) {
	exec, tp := newExecutor(t)
	args := passingArgs()
	args["tests_run"] = 0
	res, _ := exec.Execute(context.Background(), defaultCall(args))
	if res.ErrorKind != agentic.ToolErrorInvalidArgs {
		t.Errorf("ErrorKind = %v, want %v", res.ErrorKind, agentic.ToolErrorInvalidArgs)
	}
	if !strings.Contains(res.Error, "tests_run") || !strings.Contains(res.Error, "> 0") {
		t.Errorf("Result.Error = %q, want contains 'tests_run' and '> 0'", res.Error)
	}
	if got := tp.snapshot(); len(got) != 0 {
		t.Errorf("expected 0 triples published on rejection, got %d", len(got))
	}
}

func TestExecute_TestsPassing_MissingFields(t *testing.T) {
	cases := []string{"tests_run", "tests_passed", "tests_failed", "artifact_summary"}
	for _, missing := range cases {
		t.Run(missing, func(t *testing.T) {
			exec, tp := newExecutor(t)
			args := passingArgs()
			delete(args, missing)
			res, _ := exec.Execute(context.Background(), defaultCall(args))
			if res.ErrorKind != agentic.ToolErrorInvalidArgs {
				t.Errorf("ErrorKind = %v, want %v", res.ErrorKind, agentic.ToolErrorInvalidArgs)
			}
			if !strings.Contains(res.Error, missing) {
				t.Errorf("Result.Error = %q, want contains %q", res.Error, missing)
			}
			if got := tp.snapshot(); len(got) != 0 {
				t.Errorf("expected 0 triples published on rejection, got %d", len(got))
			}
		})
	}
}

func TestExecute_TestsPassing_NonIntegerTestsRun(t *testing.T) {
	exec, _ := newExecutor(t)
	args := passingArgs()
	args["tests_run"] = "five"
	res, _ := exec.Execute(context.Background(), defaultCall(args))
	if res.ErrorKind != agentic.ToolErrorInvalidArgs {
		t.Errorf("ErrorKind = %v, want %v", res.ErrorKind, agentic.ToolErrorInvalidArgs)
	}
	if !strings.Contains(res.Error, "tests_run") {
		t.Errorf("Result.Error = %q, want contains 'tests_run'", res.Error)
	}
}

// JSON numbers decode to float64 from map[string]any — the executor must
// accept that shape (the LLM's tool-call args land as JSON-decoded values).
func TestExecute_TestsPassing_AcceptsFloat64FromJSON(t *testing.T) {
	exec, tp := newExecutor(t)
	args := passingArgs()
	args["tests_run"] = float64(5)
	args["tests_passed"] = float64(5)
	args["tests_failed"] = float64(0)
	res, err := exec.Execute(context.Background(), defaultCall(args))
	if err != nil {
		t.Fatalf("Execute err = %v, want nil", err)
	}
	if res.Error != "" {
		t.Fatalf("tool error: %s", res.Error)
	}
	tr := tp.snapshot()
	byPredicate := map[string]any{}
	for _, x := range tr {
		byPredicate[x.Predicate] = x.Object
	}
	if got := byPredicate[predicateTestsRun]; got != 5 {
		t.Errorf("tests_run = %v (%T), want 5 (int)", got, got)
	}
}

// A non-integer JSON number (e.g. "tests_run": 5.5) must reject — we don't
// silently truncate.
func TestExecute_TestsPassing_RejectsFractionalNumber(t *testing.T) {
	exec, _ := newExecutor(t)
	args := passingArgs()
	args["tests_run"] = float64(5.5)
	res, _ := exec.Execute(context.Background(), defaultCall(args))
	if res.ErrorKind != agentic.ToolErrorInvalidArgs {
		t.Errorf("ErrorKind = %v, want %v", res.ErrorKind, agentic.ToolErrorInvalidArgs)
	}
}

func TestExecute_TestsPassing_NegativeCount(t *testing.T) {
	exec, _ := newExecutor(t)
	args := passingArgs()
	args["tests_failed"] = -1
	res, _ := exec.Execute(context.Background(), defaultCall(args))
	if res.ErrorKind != agentic.ToolErrorInvalidArgs {
		t.Errorf("ErrorKind = %v, want %v", res.ErrorKind, agentic.ToolErrorInvalidArgs)
	}
	if !strings.Contains(res.Error, "non-negative") {
		t.Errorf("Result.Error = %q, want contains 'non-negative'", res.Error)
	}
}

func TestExecute_TestsPassing_EmptyArtifactSummary(t *testing.T) {
	exec, _ := newExecutor(t)
	args := passingArgs()
	args["artifact_summary"] = ""
	res, _ := exec.Execute(context.Background(), defaultCall(args))
	if res.ErrorKind != agentic.ToolErrorInvalidArgs {
		t.Errorf("ErrorKind = %v, want %v", res.ErrorKind, agentic.ToolErrorInvalidArgs)
	}
}

// ---------------------------------------------------------------------
// Action-shape validation — tests_failing
// ---------------------------------------------------------------------

func TestExecute_TestsFailing_HappyPath(t *testing.T) {
	exec, tp := newExecutor(t)
	res, err := exec.Execute(context.Background(), defaultCall(failingArgs()))
	if err != nil {
		t.Fatalf("Execute err = %v, want nil", err)
	}
	if res.Error != "" {
		t.Fatalf("tool error: %s", res.Error)
	}
	if !res.StopLoop {
		t.Errorf("expected StopLoop=true")
	}
	tr := tp.snapshot()
	if len(tr) != 6 {
		t.Fatalf("expected 6 triples (action+reason+4 evidence), got %d", len(tr))
	}
	byPredicate := map[string]any{}
	for _, x := range tr {
		byPredicate[x.Predicate] = x.Object
	}
	if got := byPredicate[agvocab.CoordinatorNextAction]; got != ActionTestsFailing {
		t.Errorf("coordinator.next_action = %v, want %v", got, ActionTestsFailing)
	}
	if got := byPredicate[predicateRetryHint]; got == nil {
		t.Errorf("missing retry_hint triple")
	}
	if got := byPredicate[predicateFailureSummary]; got == nil {
		t.Errorf("missing failure_summary triple")
	}
	// tests_passed is *not* required for tests_failing and *not* in the
	// default fixture — must not produce a triple.
	if _, ok := byPredicate[predicateTestsPassed]; ok {
		t.Errorf("did not expect tests_passed triple for tests_failing without that field")
	}
}

func TestExecute_TestsFailing_MissingFields(t *testing.T) {
	cases := []string{"tests_run", "tests_failed", "failure_summary", "retry_hint"}
	for _, missing := range cases {
		t.Run(missing, func(t *testing.T) {
			exec, tp := newExecutor(t)
			args := failingArgs()
			delete(args, missing)
			res, _ := exec.Execute(context.Background(), defaultCall(args))
			if res.ErrorKind != agentic.ToolErrorInvalidArgs {
				t.Errorf("ErrorKind = %v, want %v", res.ErrorKind, agentic.ToolErrorInvalidArgs)
			}
			if !strings.Contains(res.Error, missing) {
				t.Errorf("Result.Error = %q, want contains %q", res.Error, missing)
			}
			if got := tp.snapshot(); len(got) != 0 {
				t.Errorf("expected 0 triples published on rejection, got %d", len(got))
			}
		})
	}
}

// tests_passed is optional for tests_failing — but if the LLM supplies it,
// it must be a valid integer. A typed but wrong-shaped value (e.g. the LLM
// emits "five" by accident) must reject loudly so the model can self-correct,
// not silently pass through.
func TestExecute_TestsFailing_OptionalTestsPassedTypeChecked(t *testing.T) {
	exec, tp := newExecutor(t)
	args := failingArgs()
	args["tests_passed"] = "five"
	res, _ := exec.Execute(context.Background(), defaultCall(args))
	if res.ErrorKind != agentic.ToolErrorInvalidArgs {
		t.Errorf("ErrorKind = %v, want %v", res.ErrorKind, agentic.ToolErrorInvalidArgs)
	}
	if !strings.Contains(res.Error, "tests_passed") {
		t.Errorf("Result.Error = %q, want contains 'tests_passed'", res.Error)
	}
	if got := tp.snapshot(); len(got) != 0 {
		t.Errorf("expected 0 triples published on rejection, got %d", len(got))
	}
}

// tests_failing with tests_run=0 is *not* invalid — a build that fails to
// start any tests is a legitimate failing terminal. The only positive-test
// guard fires for tests_passing.
func TestExecute_TestsFailing_AllowsZeroTestsRun(t *testing.T) {
	exec, tp := newExecutor(t)
	args := failingArgs()
	args["tests_run"] = 0
	res, err := exec.Execute(context.Background(), defaultCall(args))
	if err != nil {
		t.Fatalf("Execute err = %v, want nil", err)
	}
	if res.Error != "" {
		t.Fatalf("tool error: %s", res.Error)
	}
	tr := tp.snapshot()
	if len(tr) != 6 {
		t.Errorf("expected 6 triples, got %d", len(tr))
	}
}

// ---------------------------------------------------------------------
// Action-shape validation — needs_clarification
// ---------------------------------------------------------------------

func TestExecute_NeedsClarification_HappyPath(t *testing.T) {
	exec, tp := newExecutor(t)
	res, err := exec.Execute(context.Background(), defaultCall(clarificationArgs()))
	if err != nil {
		t.Fatalf("Execute err = %v, want nil", err)
	}
	if res.Error != "" {
		t.Fatalf("tool error: %s", res.Error)
	}
	if !res.StopLoop {
		t.Errorf("expected StopLoop=true")
	}
	tr := tp.snapshot()
	if len(tr) != 3 {
		t.Fatalf("expected 3 triples (action+reason+blocking_question), got %d", len(tr))
	}
	byPredicate := map[string]any{}
	for _, x := range tr {
		byPredicate[x.Predicate] = x.Object
	}
	if got := byPredicate[predicateBlockingQuestion]; got != clarificationArgs()["blocking_question"] {
		t.Errorf("blocking_question = %v, want %v", got, clarificationArgs()["blocking_question"])
	}
	// No tests_* triples on a needs_clarification terminal.
	for _, p := range []string{predicateTestsRun, predicateTestsPassed, predicateTestsFailed, predicateArtifactSummary, predicateFailureSummary, predicateRetryHint} {
		if _, ok := byPredicate[p]; ok {
			t.Errorf("did not expect %q triple on needs_clarification terminal", p)
		}
	}
}

func TestExecute_NeedsClarification_MissingBlockingQuestion(t *testing.T) {
	exec, tp := newExecutor(t)
	args := clarificationArgs()
	delete(args, "blocking_question")
	res, _ := exec.Execute(context.Background(), defaultCall(args))
	if res.ErrorKind != agentic.ToolErrorInvalidArgs {
		t.Errorf("ErrorKind = %v, want %v", res.ErrorKind, agentic.ToolErrorInvalidArgs)
	}
	if !strings.Contains(res.Error, "blocking_question") {
		t.Errorf("Result.Error = %q, want contains 'blocking_question'", res.Error)
	}
	if got := tp.snapshot(); len(got) != 0 {
		t.Errorf("expected 0 triples published on rejection, got %d", len(got))
	}
}

// ---------------------------------------------------------------------
// Triple publish failure
// ---------------------------------------------------------------------

// Half-fired triples produce ghost rule firings that are hard to debug;
// the executor short-circuits on the first publish error and leaves the
// caller to retry. Same posture as decide / emit_dev_via_spec_artifact.
func TestExecute_TriplePublishError_SurfaceAsNetwork(t *testing.T) {
	tp := &fakeTriplePublisher{err: errors.New("nats: timeout")}
	exec := NewExecutor(tp, types.PlatformMeta{Org: "c360", Platform: "semteams"}, nil)
	res, err := exec.Execute(context.Background(), defaultCall(passingArgs()))
	if err != nil {
		t.Fatalf("Execute err = %v, want nil (publish errors land on Result, not the err return)", err)
	}
	if res.ErrorKind != agentic.ToolErrorNetwork {
		t.Errorf("ErrorKind = %v, want %v", res.ErrorKind, agentic.ToolErrorNetwork)
	}
	if !strings.Contains(res.Error, "nats: timeout") {
		t.Errorf("Result.Error = %q, want contains transport error message", res.Error)
	}
}
