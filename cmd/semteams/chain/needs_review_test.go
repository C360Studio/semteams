package chain

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/c360studio/semstreams/agentic"
	"github.com/c360studio/semstreams/message"
	agvocab "github.com/c360studio/semstreams/vocabulary/agentic"
)

func TestNeedsReviewMilestone_NonBuilderNoOp(t *testing.T) {
	pub := &recordingPublisher{}
	resolver := resolverWithChain(nil)
	er := &fakeEntityReader{}
	s := NewNeedsReviewStamper(pub, resolver, er, testPlatform(), nil)

	ev := &agentic.LoopCompletedEvent{
		LoopID:  "loop-x",
		Role:    "dev-via-spec-architect",
		Outcome: agentic.OutcomeSuccess,
	}
	if err := s.HandleLoopCompleted(context.Background(), ev); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if er.calls != 0 {
		t.Errorf("non-builder event should skip graph reads, got %d calls", er.calls)
	}
	if len(pub.triples) != 0 {
		t.Errorf("non-builder event should write no triples, got %d", len(pub.triples))
	}
}

func TestNeedsReviewMilestone_FailedBuilderNoOp(t *testing.T) {
	pub := &recordingPublisher{}
	resolver := resolverWithChain(nil)
	er := &fakeEntityReader{}
	s := NewNeedsReviewStamper(pub, resolver, er, testPlatform(), nil)

	ev := &agentic.LoopCompletedEvent{
		LoopID:  "loop-x",
		Role:    builderRole,
		Outcome: agentic.OutcomeFailed,
	}
	if err := s.HandleLoopCompleted(context.Background(), ev); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if er.calls != 0 || len(pub.triples) != 0 {
		t.Error("failed builder should skip both reads and writes (chainpause owns FAILED loops)")
	}
}

func TestNeedsReviewMilestone_TestsPassingNoOp(t *testing.T) {
	pub := &recordingPublisher{}
	resolver := resolverWithChain(nil)

	builderEntityID := loopEntityID(t, "builder_x")
	er := &fakeEntityReader{
		entities: map[string]map[string]any{
			builderEntityID: {
				agvocab.CoordinatorNextAction:     "tests_passing",
				agvocab.CoordinatorDecisionReason: "all 3/3 tests passing",
			},
		},
	}
	s := NewNeedsReviewStamper(pub, resolver, er, testPlatform(), nil)

	ev := &agentic.LoopCompletedEvent{
		LoopID:  "builder_x",
		Role:    builderRole,
		Outcome: agentic.OutcomeSuccess,
	}
	if err := s.HandleLoopCompleted(context.Background(), ev); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(pub.triples) != 0 {
		t.Errorf("tests_passing terminal should write no needs_review triples (handled by rule 07), got %d", len(pub.triples))
	}
}

func TestNeedsReviewMilestone_TestsFailingNoOp(t *testing.T) {
	pub := &recordingPublisher{}
	resolver := resolverWithChain(nil)

	builderEntityID := loopEntityID(t, "builder_x")
	er := &fakeEntityReader{
		entities: map[string]map[string]any{
			builderEntityID: {
				agvocab.CoordinatorNextAction:     "tests_failing",
				agvocab.CoordinatorDecisionReason: "2/3 tests passing; protobuf decoder rejected POSITION_APP",
			},
		},
	}
	s := NewNeedsReviewStamper(pub, resolver, er, testPlatform(), nil)

	ev := &agentic.LoopCompletedEvent{
		LoopID:  "builder_x",
		Role:    builderRole,
		Outcome: agentic.OutcomeSuccess,
	}
	if err := s.HandleLoopCompleted(context.Background(), ev); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(pub.triples) != 0 {
		t.Errorf("tests_failing terminal should write no needs_review triples (handled by rule 07), got %d", len(pub.triples))
	}
}

func TestNeedsReviewMilestone_NeedsClarificationHappyPath(t *testing.T) {
	pub := &recordingPublisher{}
	// Chain ancestry: dispatch_root → researcher_a → ... → architect_d → builder_e
	resolver := resolverWithChain(map[string]string{
		"builder_e":    loopEntityID(t, "architect_d"),
		"architect_d":  loopEntityID(t, "challenger_c"),
		"challenger_c": loopEntityID(t, "researcher_a"),
		"researcher_a": loopEntityID(t, "dispatch_root"),
	})

	const wantReason = "image meshtastic/meshtasticd:3.5.0 returned 404 from Docker Hub; cannot bootstrap container"
	builderEntityID := loopEntityID(t, "builder_e")
	er := &fakeEntityReader{
		entities: map[string]map[string]any{
			builderEntityID: {
				agvocab.CoordinatorNextAction:     needsClarificationAction,
				agvocab.CoordinatorDecisionReason: wantReason,
			},
		},
	}
	s := NewNeedsReviewStamper(pub, resolver, er, testPlatform(), nil)

	before := time.Now().UTC()
	ev := &agentic.LoopCompletedEvent{
		LoopID:  "builder_e",
		Role:    builderRole,
		Outcome: agentic.OutcomeSuccess,
	}
	if err := s.HandleLoopCompleted(context.Background(), ev); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	after := time.Now().UTC()

	wantChainEntityID := "c360.test.agent.chain.execution.dispatch_root"
	wantPredicates := map[string]string{
		PredicateNeedsReviewClassification: needsReviewClassificationDefault,
		PredicateNeedsReviewProducerLoopID: "builder_e",
		PredicateNeedsReviewProducerRole:   builderRole,
		PredicateNeedsReviewReason:         wantReason,
	}
	for pred, want := range wantPredicates {
		got, ok := pub.byPredicate(pred)
		if !ok {
			t.Errorf("predicate %q missing on chain entity", pred)
			continue
		}
		if got.Subject != wantChainEntityID {
			t.Errorf("predicate %q subject = %q, want %q", pred, got.Subject, wantChainEntityID)
		}
		if obj, _ := got.Object.(string); obj != want {
			t.Errorf("predicate %q object = %v, want %q", pred, got.Object, want)
		}
	}

	observed, ok := pub.byPredicate(PredicateNeedsReviewObservedAt)
	if !ok {
		t.Fatal("observed_at predicate missing")
	}
	tsStr, _ := observed.Object.(string)
	ts, err := time.Parse(time.RFC3339, tsStr)
	if err != nil {
		t.Fatalf("observed_at not RFC3339: %q (%v)", tsStr, err)
	}
	// Allow second-level granularity — RFC3339 truncates sub-second precision.
	if ts.Before(before.Truncate(time.Second)) || ts.After(after.Add(time.Second)) {
		t.Errorf("observed_at %v outside [%v, %v]", ts, before, after)
	}
}

func TestNeedsReviewMilestone_ResolverError_Skipped(t *testing.T) {
	pub := &recordingPublisher{}
	resolver := NewResolver(&fakeParentReader{err: errors.New("graph KV unavailable")}, testPlatform())
	builderEntityID := loopEntityID(t, "builder_x")
	er := &fakeEntityReader{
		entities: map[string]map[string]any{
			builderEntityID: {
				agvocab.CoordinatorNextAction:     needsClarificationAction,
				agvocab.CoordinatorDecisionReason: "harness selection ambiguous",
			},
		},
	}
	s := NewNeedsReviewStamper(pub, resolver, er, testPlatform(), nil)

	ev := &agentic.LoopCompletedEvent{
		LoopID:  "builder_x",
		Role:    builderRole,
		Outcome: agentic.OutcomeSuccess,
	}
	if err := s.HandleLoopCompleted(context.Background(), ev); err != nil {
		t.Fatalf("expected fail-soft, got error: %v", err)
	}
	if len(pub.triples) != 0 {
		t.Errorf("resolver error should drop the milestone, got %d triples", len(pub.triples))
	}
}

// flakyPublisher fails AddTriple on a configured predicate; succeeds for
// the rest. Lets a test assert that NeedsReviewStamper's per-triple loop
// keeps writing siblings after one fails (the "partial cluster is
// queryable; a zero-triple record is invisible" contract documented on
// the type).
type flakyPublisher struct {
	failOn  string
	written []string
}

func (f *flakyPublisher) AddTriple(_ context.Context, t message.Triple) error {
	if t.Predicate == f.failOn {
		return errors.New("simulated publisher failure")
	}
	f.written = append(f.written, t.Predicate)
	return nil
}

func TestNeedsReviewMilestone_PartialClusterFailure_OtherWritesProceed(t *testing.T) {
	pub := &flakyPublisher{failOn: PredicateNeedsReviewProducerLoopID}
	resolver := resolverWithChain(map[string]string{
		"builder_x": loopEntityID(t, "dispatch_root"),
	})
	builderEntityID := loopEntityID(t, "builder_x")
	er := &fakeEntityReader{
		entities: map[string]map[string]any{
			builderEntityID: {
				agvocab.CoordinatorNextAction:     needsClarificationAction,
				agvocab.CoordinatorDecisionReason: "harness selection ambiguous",
			},
		},
	}
	s := NewNeedsReviewStamper(pub, resolver, er, testPlatform(), nil)

	ev := &agentic.LoopCompletedEvent{
		LoopID:  "builder_x",
		Role:    builderRole,
		Outcome: agentic.OutcomeSuccess,
	}
	if err := s.HandleLoopCompleted(context.Background(), ev); err != nil {
		t.Fatalf("partial-cluster failure should be fail-soft, got error: %v", err)
	}

	// 5 predicates total; one fails; remaining four must still land. The
	// predicate ordering in needs_review.go is classification, producer_loop_id,
	// producer_role, reason, observed_at — so failing on producer_loop_id
	// leaves classification + producer_role + reason + observed_at.
	wantWritten := []string{
		PredicateNeedsReviewClassification,
		PredicateNeedsReviewProducerRole,
		PredicateNeedsReviewReason,
		PredicateNeedsReviewObservedAt,
	}
	if len(pub.written) != len(wantWritten) {
		t.Fatalf("expected %d siblings to land after one failure, got %d (%v)", len(wantWritten), len(pub.written), pub.written)
	}
	wantSet := map[string]struct{}{}
	for _, p := range wantWritten {
		wantSet[p] = struct{}{}
	}
	for _, p := range pub.written {
		if _, ok := wantSet[p]; !ok {
			t.Errorf("unexpected predicate written: %q", p)
		}
	}
}

func TestNeedsReviewMilestone_NilEvent_NoOp(t *testing.T) {
	pub := &recordingPublisher{}
	resolver := resolverWithChain(nil)
	er := &fakeEntityReader{}
	s := NewNeedsReviewStamper(pub, resolver, er, testPlatform(), nil)

	if err := s.HandleLoopCompleted(context.Background(), nil); err != nil {
		t.Fatalf("nil event should fail-soft, got error: %v", err)
	}
	if len(pub.triples) != 0 {
		t.Errorf("nil event should write no triples")
	}
}
