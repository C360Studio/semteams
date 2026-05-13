package chain

import (
	"context"
	"errors"
	"testing"

	"github.com/c360studio/semstreams/agentic"
)

// plannerLoopEntityID returns the canonical loop entity ID for the
// fake-test platform fixture. Saves repetition at every assertion site.
func plannerLoopEntityID(t *testing.T, loopID string) string {
	t.Helper()
	return loopEntityID(t, loopID)
}

func TestPlanMilestone_NonReviewerNoOp(t *testing.T) {
	pub := &recordingPublisher{}
	resolver := resolverWithChain(nil)
	er := &fakeEntityReader{}
	s := NewPlanMilestoneStamper(pub, resolver, er, testPlatform(), nil)

	ev := &agentic.LoopCompletedEvent{
		LoopID:  "loop-x",
		Role:    "dev-via-spec-planner",
		Outcome: agentic.OutcomeSuccess,
	}
	if err := s.HandleLoopCompleted(context.Background(), ev); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if er.calls != 0 {
		t.Errorf("non-reviewer event should skip graph reads, got %d calls", er.calls)
	}
	if len(pub.triples) != 0 {
		t.Errorf("non-reviewer event should write no triples, got %d", len(pub.triples))
	}
}

func TestPlanMilestone_FailedReviewerNoOp(t *testing.T) {
	pub := &recordingPublisher{}
	resolver := resolverWithChain(nil)
	er := &fakeEntityReader{}
	s := NewPlanMilestoneStamper(pub, resolver, er, testPlatform(), nil)

	ev := &agentic.LoopCompletedEvent{
		LoopID:  "loop-x",
		Role:    devViaSpecReviewerRole,
		Outcome: agentic.OutcomeFailed,
	}
	if err := s.HandleLoopCompleted(context.Background(), ev); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if er.calls != 0 || len(pub.triples) != 0 {
		t.Error("failed reviewer should skip both reads and writes")
	}
}

func TestPlanMilestone_RejectedNoOp(t *testing.T) {
	pub := &recordingPublisher{}
	resolver := resolverWithChain(nil)

	reviewerEntityID := plannerLoopEntityID(t, "reviewer_x")
	er := &fakeEntityReader{
		entities: map[string]map[string]any{
			reviewerEntityID: {
				reviewerNextActionPredicate: "rejected",
				agentLoopParentPredicate:    plannerLoopEntityID(t, "planner_a"),
			},
		},
	}
	s := NewPlanMilestoneStamper(pub, resolver, er, testPlatform(), nil)

	ev := &agentic.LoopCompletedEvent{
		LoopID:  "reviewer_x",
		Role:    devViaSpecReviewerRole,
		Outcome: agentic.OutcomeSuccess,
	}
	if err := s.HandleLoopCompleted(context.Background(), ev); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(pub.triples) != 0 {
		t.Errorf("rejected milestone should write no triples, got %d", len(pub.triples))
	}
}

func TestPlanMilestone_ApprovedHappyPath(t *testing.T) {
	pub := &recordingPublisher{}
	// Chain ancestry: dispatch_root → researcher_a → research_reviewer_b
	//                 → planner_c → reviewer_d
	resolver := resolverWithChain(map[string]string{
		"planner_c":           plannerLoopEntityID(t, "research_reviewer_b"),
		"research_reviewer_b": plannerLoopEntityID(t, "researcher_a"),
		"researcher_a":        plannerLoopEntityID(t, "dispatch_root"),
		// dispatch_root has no parent → chain root
	})

	reviewerEntityID := plannerLoopEntityID(t, "reviewer_d")
	plannerEntityID := plannerLoopEntityID(t, "planner_c")
	er := &fakeEntityReader{
		entities: map[string]map[string]any{
			reviewerEntityID: {
				reviewerNextActionPredicate: planApprovedAction,
				agentLoopParentPredicate:    plannerEntityID,
			},
			plannerEntityID: {
				plannerArtifactPathPredicate: "docs/plans/2026-05-08-osh-meshtastic-plan.md",
			},
		},
	}
	s := NewPlanMilestoneStamper(pub, resolver, er, testPlatform(), nil)

	ev := &agentic.LoopCompletedEvent{
		LoopID:  "reviewer_d",
		Role:    devViaSpecReviewerRole,
		Outcome: agentic.OutcomeSuccess,
	}
	if err := s.HandleLoopCompleted(context.Background(), ev); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	wantChainEntityID := "c360.test.agent.chain.execution.dispatch_root"
	wantPredicates := map[string]any{
		chainPredicatePlanLoop: "planner_c",
		chainPredicatePlanPath: "docs/plans/2026-05-08-osh-meshtastic-plan.md",
		// chain.plan_reviewer_loop is the reviewer loop that approved the
		// plan — taken straight from ev.LoopID at milestone time.
		// Originally read by emit_consensus (retired in ADR-041 Slice
		// 2D-5) to populate depends_on.reviewer_loop without trusting an
		// LLM guess (smoke #8 run-5 D1 fix). The predicate stays for
		// legacy-config audit replay; no live consumer under MVP.
		chainPredicatePlanReviewerLoop: "reviewer_d",
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
		if got.Object != want {
			t.Errorf("predicate %q object = %v, want %v", pred, got.Object, want)
		}
	}
}

func TestPlanMilestone_PathOmittedWhenAbsent(t *testing.T) {
	pub := &recordingPublisher{}
	resolver := resolverWithChain(map[string]string{
		"planner_c":     plannerLoopEntityID(t, "dispatch_root"),
		"dispatch_root": "",
	})
	reviewerEntityID := plannerLoopEntityID(t, "reviewer_d")
	plannerEntityID := plannerLoopEntityID(t, "planner_c")
	er := &fakeEntityReader{
		entities: map[string]map[string]any{
			reviewerEntityID: {
				reviewerNextActionPredicate: planApprovedAction,
				agentLoopParentPredicate:    plannerEntityID,
			},
			plannerEntityID: {
				// no dev_via_spec.plan.path — legacy plan emitted without
				// markdown rendering (pre-Phase-C2). Chain triple must
				// be omitted, not fabricated.
			},
		},
	}
	s := NewPlanMilestoneStamper(pub, resolver, er, testPlatform(), nil)

	ev := &agentic.LoopCompletedEvent{
		LoopID:  "reviewer_d",
		Role:    devViaSpecReviewerRole,
		Outcome: agentic.OutcomeSuccess,
	}
	if err := s.HandleLoopCompleted(context.Background(), ev); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if _, ok := pub.byPredicate(chainPredicatePlanPath); ok {
		t.Error("path predicate should be omitted when dev_via_spec.plan.path is absent on planner loop")
	}
	if _, ok := pub.byPredicate(chainPredicatePlanLoop); !ok {
		t.Error("plan_loop predicate must still land regardless of path state")
	}
	// chain.plan_reviewer_loop is independent of plan path: it identifies
	// the reviewer loop that just approved, not the renderer state. Lands
	// for every approved milestone.
	if _, ok := pub.byPredicate(chainPredicatePlanReviewerLoop); !ok {
		t.Error("plan_reviewer_loop predicate must still land when reviewer approves regardless of path state")
	}
}

func TestPlanMilestone_MissingParent_Skipped(t *testing.T) {
	pub := &recordingPublisher{}
	resolver := resolverWithChain(nil)
	reviewerEntityID := plannerLoopEntityID(t, "reviewer_d")
	er := &fakeEntityReader{
		entities: map[string]map[string]any{
			reviewerEntityID: {
				reviewerNextActionPredicate: planApprovedAction,
				// no agent.loop.parent — should not happen in practice
				// (graph_writer stamps it at completion) but defensive
				// handling keeps the milestone from fabricating.
			},
		},
	}
	s := NewPlanMilestoneStamper(pub, resolver, er, testPlatform(), nil)

	ev := &agentic.LoopCompletedEvent{
		LoopID:  "reviewer_d",
		Role:    devViaSpecReviewerRole,
		Outcome: agentic.OutcomeSuccess,
	}
	if err := s.HandleLoopCompleted(context.Background(), ev); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(pub.triples) != 0 {
		t.Errorf("missing agent.loop.parent should drop the milestone, got %d triples", len(pub.triples))
	}
}

func TestPlanMilestone_ResolverError_Skipped(t *testing.T) {
	pub := &recordingPublisher{}
	resolver := NewResolver(&fakeParentReader{err: errors.New("graph KV unavailable")}, testPlatform())
	reviewerEntityID := plannerLoopEntityID(t, "reviewer_d")
	plannerEntityID := plannerLoopEntityID(t, "planner_c")
	er := &fakeEntityReader{
		entities: map[string]map[string]any{
			reviewerEntityID: {
				reviewerNextActionPredicate: planApprovedAction,
				agentLoopParentPredicate:    plannerEntityID,
			},
			plannerEntityID: {
				plannerArtifactPathPredicate: "docs/plans/x.md",
			},
		},
	}
	s := NewPlanMilestoneStamper(pub, resolver, er, testPlatform(), nil)

	ev := &agentic.LoopCompletedEvent{
		LoopID:  "reviewer_d",
		Role:    devViaSpecReviewerRole,
		Outcome: agentic.OutcomeSuccess,
	}
	if err := s.HandleLoopCompleted(context.Background(), ev); err != nil {
		t.Fatalf("expected fail-soft, got error: %v", err)
	}
	if len(pub.triples) != 0 {
		t.Errorf("resolver error should drop the milestone, got %d triples", len(pub.triples))
	}
}
