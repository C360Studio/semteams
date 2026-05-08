package chainpause

import (
	"context"
	"errors"
	"testing"

	"github.com/c360studio/semstreams/agentic"
)

// recordingTaskPublisher records PublishTask calls.
type recordingTaskPublisher struct {
	calls []publishCall
}

type publishCall struct {
	subject string
	task    *agentic.TaskMessage
}

func (r *recordingTaskPublisher) PublishTask(_ context.Context, subject string, task *agentic.TaskMessage) error {
	r.calls = append(r.calls, publishCall{subject: subject, task: task})
	return nil
}

// staticPauseDataReader returns fixed role and model for testing.
type staticPauseDataReader struct {
	role  string
	model string
	err   error
}

func (s *staticPauseDataReader) ReadPauseData(_ context.Context, _ string) (string, string, error) {
	return s.role, s.model, s.err
}

func testDecisionHandler(t *testing.T) (*DecisionHandler, *recordingPublisher, *recordingTaskPublisher) {
	t.Helper()
	return testDecisionHandlerWithPauseData(t, &staticPauseDataReader{role: "dev-via-spec-reviewer", model: "claude-haiku"})
}

func testDecisionHandlerWithPauseData(t *testing.T, pauseData PauseDataReader) (*DecisionHandler, *recordingPublisher, *recordingTaskPublisher) {
	t.Helper()
	pub := &recordingPublisher{}
	tasks := &recordingTaskPublisher{}
	h := NewDecisionHandler(pub, tasks, pauseData, &fakeChainResolver{}, nil)
	return h, pub, tasks
}

func TestDecisionHandler_Retry_PublishesTask(t *testing.T) {
	h, _, tasks := testDecisionHandler(t)

	req := DecisionRequest{
		FailedLoopID: "loop-abc",
		Verb:         "retry",
		Reason:       "transient API overload",
	}
	if err := h.HandleDecision(context.Background(), req, "operator@example.com"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(tasks.calls) != 1 {
		t.Fatalf("expected 1 task publish call, got %d", len(tasks.calls))
	}
	call := tasks.calls[0]
	if call.task == nil {
		t.Fatal("task is nil")
	}
	if call.task.Metadata["prior_loop_id"] != "loop-abc" {
		t.Errorf("prior_loop_id: got %v, want loop-abc", call.task.Metadata["prior_loop_id"])
	}
	if call.task.Metadata["decision_verb"] != "retry" {
		t.Errorf("decision_verb: got %v, want retry", call.task.Metadata["decision_verb"])
	}
}

// TestDecisionHandler_Retry_PreservesRoleAndModel verifies that retry reads the
// failed loop's role and model from the PauseDataReader and stamps them onto the
// re-published TaskMessage (ADR-037 §D7).
func TestDecisionHandler_Retry_PreservesRoleAndModel(t *testing.T) {
	pauseData := &staticPauseDataReader{role: "dev-via-spec-challenger", model: "claude-opus-4-5"}
	h, _, tasks := testDecisionHandlerWithPauseData(t, pauseData)

	req := DecisionRequest{FailedLoopID: "loop-preserve-rm", Verb: "retry"}
	if err := h.HandleDecision(context.Background(), req, "op"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(tasks.calls) != 1 {
		t.Fatalf("expected 1 publish call, got %d", len(tasks.calls))
	}
	call := tasks.calls[0]
	if call.task.Role != "dev-via-spec-challenger" {
		t.Errorf("task.Role = %q, want dev-via-spec-challenger", call.task.Role)
	}
	if call.task.Model != "claude-opus-4-5" {
		t.Errorf("task.Model = %q, want claude-opus-4-5", call.task.Model)
	}
}

// TestDecisionHandler_Retry_SubjectMatchesRole verifies that the publish subject
// is agent.task.<role> so the agentic-loop's agent.task.* consumer picks it up.
func TestDecisionHandler_Retry_SubjectMatchesRole(t *testing.T) {
	pauseData := &staticPauseDataReader{role: "dev-via-spec-reviewer", model: "claude-haiku"}
	h, _, tasks := testDecisionHandlerWithPauseData(t, pauseData)

	req := DecisionRequest{FailedLoopID: "loop-subject", Verb: "retry"}
	if err := h.HandleDecision(context.Background(), req, "op"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(tasks.calls) != 1 {
		t.Fatalf("expected 1 publish call, got %d", len(tasks.calls))
	}
	wantSubject := "agent.task.dev-via-spec-reviewer"
	if tasks.calls[0].subject != wantSubject {
		t.Errorf("publish subject = %q, want %q", tasks.calls[0].subject, wantSubject)
	}
}

// TestDecisionHandler_Retry_FallbackRoleModelOnReadError verifies that when
// PauseDataReader returns an error, retry still fires with safe defaults
// ("dispatch", "claude-haiku") rather than failing the whole decision.
func TestDecisionHandler_Retry_FallbackRoleModelOnReadError(t *testing.T) {
	pauseData := &staticPauseDataReader{err: errors.New("graph unavailable")}
	h, _, tasks := testDecisionHandlerWithPauseData(t, pauseData)

	req := DecisionRequest{FailedLoopID: "loop-fallback", Verb: "retry"}
	if err := h.HandleDecision(context.Background(), req, "op"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(tasks.calls) != 1 {
		t.Fatalf("expected 1 publish call, got %d", len(tasks.calls))
	}
	call := tasks.calls[0]
	if call.task.Role != "dispatch" {
		t.Errorf("fallback role = %q, want dispatch", call.task.Role)
	}
	if call.task.Model != "claude-haiku" {
		t.Errorf("fallback model = %q, want claude-haiku", call.task.Model)
	}
}

func TestDecisionHandler_Retry_WritesChainResumedTriple(t *testing.T) {
	h, pub, _ := testDecisionHandler(t)

	req := DecisionRequest{FailedLoopID: "loop-resume", Verb: "retry"}
	if err := h.HandleDecision(context.Background(), req, "op"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if _, ok := pub.byPredicate("chain.resumed"); !ok {
		t.Error("expected chain.resumed triple to be written after retry")
	}
}

func TestDecisionHandler_Kill_WritesChainKilledTriple(t *testing.T) {
	h, pub, tasks := testDecisionHandler(t)

	req := DecisionRequest{FailedLoopID: "loop-kill", Verb: "kill"}
	if err := h.HandleDecision(context.Background(), req, "op"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if _, ok := pub.byPredicate("chain.killed"); !ok {
		t.Error("expected chain.killed triple to be written after kill")
	}
	if len(tasks.calls) != 0 {
		t.Errorf("kill should not publish any task, got %d publishes", len(tasks.calls))
	}
}

func TestDecisionHandler_Defer_WritesChainDeferredTriple(t *testing.T) {
	h, pub, tasks := testDecisionHandler(t)

	req := DecisionRequest{FailedLoopID: "loop-defer", Verb: "defer"}
	if err := h.HandleDecision(context.Background(), req, "op"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if _, ok := pub.byPredicate("chain.deferred"); !ok {
		t.Error("expected chain.deferred triple to be written after defer")
	}
	if len(tasks.calls) != 0 {
		t.Errorf("defer should not publish any task, got %d publishes", len(tasks.calls))
	}
}

func TestDecisionHandler_DecisionTriples_WrittenForAllVerbs(t *testing.T) {
	for _, verb := range []string{"retry", "kill", "defer"} {
		t.Run(verb, func(t *testing.T) {
			h, pub, _ := testDecisionHandler(t)
			req := DecisionRequest{FailedLoopID: "loop-dt-" + verb, Verb: verb}
			_ = h.HandleDecision(context.Background(), req, "testactor")

			preds := []string{
				"chain.decision.verb",
				"chain.decision.authority",
				"chain.decision.actor",
				"chain.decision.decided_at",
			}
			for _, pred := range preds {
				if _, ok := pub.byPredicate(pred); !ok {
					t.Errorf("expected %q triple to be written for verb %q", pred, verb)
				}
			}

			if t2, ok := pub.byPredicate("chain.decision.authority"); ok {
				if t2.Object != "operator" {
					t.Errorf("authority: got %v, want operator", t2.Object)
				}
			}
			if t2, ok := pub.byPredicate("chain.decision.actor"); ok {
				if t2.Object != "testactor" {
					t.Errorf("actor: got %v, want testactor", t2.Object)
				}
			}
		})
	}
}

// TestDecisionHandler_AuditTriplesSubjectIsChainEntity pins the
// ADR-038 PR B Phase 3 re-point on the decision side. Every
// chain.decision.* / chain.resumed / chain.killed / chain.deferred
// triple must target the canonical chain entity, not the failed
// loop's entity. A regression here would silently fragment audit
// state from chainpause's pause-time writes; the operator HTTP flow
// would still succeed but cross-arc consumers reading the chain
// entity would see no decision history.
func TestDecisionHandler_AuditTriplesSubjectIsChainEntity(t *testing.T) {
	for _, verb := range []string{"retry", "kill", "defer"} {
		t.Run(verb, func(t *testing.T) {
			pub := &recordingPublisher{}
			tasks := &recordingTaskPublisher{}
			pauseData := &staticPauseDataReader{role: "dev-via-spec-reviewer", model: "claude-haiku"}
			resolver := &fakeChainResolver{chainRoot: "dispatch_root"}
			h := NewDecisionHandler(pub, tasks, pauseData, resolver, nil)

			req := DecisionRequest{FailedLoopID: "researcher_with_source_8", Verb: verb}
			if err := h.HandleDecision(context.Background(), req, "op"); err != nil {
				t.Fatalf("HandleDecision: %v", err)
			}

			if resolver.calls == 0 {
				t.Fatal("resolver was not consulted")
			}
			if resolver.lastArg != "researcher_with_source_8" {
				t.Errorf("resolver called with %q, want failed loop id", resolver.lastArg)
			}

			wantSubject := testChainEntityID("dispatch_root")
			pub.mu.Lock()
			defer pub.mu.Unlock()
			for _, tr := range pub.triples {
				if tr.Subject != wantSubject {
					t.Errorf("decision audit triple %q wrote to %q, want %q (chain entity)", tr.Predicate, tr.Subject, wantSubject)
				}
			}
		})
	}
}

// TestDecisionHandler_ResolverError_ReturnsError verifies that a
// resolver failure surfaces to the HTTP boundary so the operator gets
// a real error back instead of a silently-incomplete decision.
func TestDecisionHandler_ResolverError_ReturnsError(t *testing.T) {
	pub := &recordingPublisher{}
	tasks := &recordingTaskPublisher{}
	pauseData := &staticPauseDataReader{}
	resolver := &fakeChainResolver{err: errors.New("graph KV unavailable")}
	h := NewDecisionHandler(pub, tasks, pauseData, resolver, nil)

	req := DecisionRequest{FailedLoopID: "loop-x", Verb: "retry"}
	if err := h.HandleDecision(context.Background(), req, "op"); err == nil {
		t.Fatal("expected resolver error to surface; got nil")
	}
	if len(pub.triples) != 0 {
		t.Errorf("no triples should be written when resolver fails; got %d", len(pub.triples))
	}
	if len(tasks.calls) != 0 {
		t.Errorf("no retry task should be published when resolver fails; got %d", len(tasks.calls))
	}
}

// v2-reserved verb must be rejected with ErrReservedVerb in v1.
func TestDecisionHandler_ReservedV2Verb_Rejected(t *testing.T) {
	h, _, _ := testDecisionHandler(t)

	req := DecisionRequest{FailedLoopID: "loop-v2", Verb: "apply_fix_and_retry"}
	err := h.HandleDecision(context.Background(), req, "op")
	if err == nil {
		t.Fatal("expected error for reserved v2 verb, got nil")
	}
	if !errors.Is(err, ErrReservedVerb) {
		t.Errorf("expected errors.Is(err, ErrReservedVerb); got: %v", err)
	}
}

func TestDecisionHandler_UnknownVerb_Rejected(t *testing.T) {
	h, _, _ := testDecisionHandler(t)

	req := DecisionRequest{FailedLoopID: "loop-bad-verb", Verb: "auto_fix"}
	err := h.HandleDecision(context.Background(), req, "op")
	if err == nil {
		t.Fatal("expected error for unknown verb, got nil")
	}
	if !errors.Is(err, ErrInvalidVerb) {
		t.Errorf("expected errors.Is(err, ErrInvalidVerb); got: %v", err)
	}
}

// TestDecisionHandler_VerbErrors_AreVerbErrors verifies that both sentinel
// error types satisfy isVerbError (the HTTP status-picker used in http.go).
func TestDecisionHandler_VerbErrors_AreVerbErrors(t *testing.T) {
	h, _, _ := testDecisionHandler(t)

	for _, tc := range []struct {
		verb string
		want error
	}{
		{"apply_fix_and_retry", ErrReservedVerb},
		{"not_a_verb", ErrInvalidVerb},
	} {
		t.Run(tc.verb, func(t *testing.T) {
			err := h.HandleDecision(context.Background(), DecisionRequest{FailedLoopID: "loop-x", Verb: tc.verb}, "op")
			if err == nil {
				t.Fatalf("expected error, got nil")
			}
			if !isVerbError(err) {
				t.Errorf("isVerbError(%v) = false, want true", err)
			}
			if !errors.Is(err, tc.want) {
				t.Errorf("errors.Is(err, %v) = false, want true", tc.want)
			}
		})
	}
}

// Empty verb must be rejected, not silently default.
func TestDecisionHandler_EmptyVerb_Rejected(t *testing.T) {
	h, _, _ := testDecisionHandler(t)

	req := DecisionRequest{FailedLoopID: "loop-no-verb", Verb: ""}
	err := h.HandleDecision(context.Background(), req, "op")
	if err == nil {
		t.Fatal("expected error for empty verb, got nil")
	}
}
