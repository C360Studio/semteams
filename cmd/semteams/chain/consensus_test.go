package chain

import (
	"context"
	"errors"
	"testing"

	"github.com/c360studio/semstreams/agentic"
)

func TestConsensusMilestone_NonChallengerNoOp(t *testing.T) {
	pub := &recordingPublisher{}
	resolver := resolverWithChain(nil)
	er := &fakeEntityReader{}
	s := NewConsensusMilestoneStamper(pub, resolver, er, testPlatform(), nil)

	ev := &agentic.LoopCompletedEvent{
		LoopID:  "loop-x",
		Role:    "dev-via-spec-architect",
		Outcome: agentic.OutcomeSuccess,
	}
	if err := s.HandleLoopCompleted(context.Background(), ev); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if er.calls != 0 {
		t.Errorf("non-challenger event should skip graph reads, got %d calls", er.calls)
	}
	if len(pub.triples) != 0 {
		t.Errorf("non-challenger event should write no triples, got %d", len(pub.triples))
	}
}

func TestConsensusMilestone_FailedChallengerNoOp(t *testing.T) {
	pub := &recordingPublisher{}
	resolver := resolverWithChain(nil)
	er := &fakeEntityReader{}
	s := NewConsensusMilestoneStamper(pub, resolver, er, testPlatform(), nil)

	ev := &agentic.LoopCompletedEvent{
		LoopID:  "loop-x",
		Role:    challengerRole,
		Outcome: agentic.OutcomeFailed,
	}
	if err := s.HandleLoopCompleted(context.Background(), ev); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if er.calls != 0 || len(pub.triples) != 0 {
		t.Error("failed challenger should skip both reads and writes")
	}
}

func TestConsensusMilestone_ConcernsRaisedNoOp(t *testing.T) {
	pub := &recordingPublisher{}
	resolver := resolverWithChain(nil)

	challengerEntityID := loopEntityID(t, "challenger_x")
	er := &fakeEntityReader{
		entities: map[string]map[string]any{
			challengerEntityID: {
				reviewerNextActionPredicate:     "concerns_raised",
				challengerArtifactPathPredicate: "docs/consensus/x.md",
			},
		},
	}
	s := NewConsensusMilestoneStamper(pub, resolver, er, testPlatform(), nil)

	ev := &agentic.LoopCompletedEvent{
		LoopID:  "challenger_x",
		Role:    challengerRole,
		Outcome: agentic.OutcomeSuccess,
	}
	if err := s.HandleLoopCompleted(context.Background(), ev); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(pub.triples) != 0 {
		t.Errorf("concerns_raised milestone should write no triples, got %d", len(pub.triples))
	}
}

func TestConsensusMilestone_AcceptHappyPath(t *testing.T) {
	pub := &recordingPublisher{}
	// Chain ancestry: dispatch_root → researcher_a → research_reviewer_b
	//                 → planner_c → reviewer_d → challenger_e
	resolver := resolverWithChain(map[string]string{
		"challenger_e":        loopEntityID(t, "reviewer_d"),
		"reviewer_d":          loopEntityID(t, "planner_c"),
		"planner_c":           loopEntityID(t, "research_reviewer_b"),
		"research_reviewer_b": loopEntityID(t, "researcher_a"),
		"researcher_a":        loopEntityID(t, "dispatch_root"),
	})

	challengerEntityID := loopEntityID(t, "challenger_e")
	er := &fakeEntityReader{
		entities: map[string]map[string]any{
			challengerEntityID: {
				reviewerNextActionPredicate:     consensusAcceptAction,
				challengerArtifactPathPredicate: "docs/consensus/2026-05-08-osh-meshtastic-consensus.md",
			},
		},
	}
	s := NewConsensusMilestoneStamper(pub, resolver, er, testPlatform(), nil)

	ev := &agentic.LoopCompletedEvent{
		LoopID:  "challenger_e",
		Role:    challengerRole,
		Outcome: agentic.OutcomeSuccess,
	}
	if err := s.HandleLoopCompleted(context.Background(), ev); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	wantChainEntityID := "c360.test.agent.chain.execution.dispatch_root"
	wantPredicates := map[string]any{
		chainPredicateConsensusLoop: "challenger_e",
		chainPredicateConsensusPath: "docs/consensus/2026-05-08-osh-meshtastic-consensus.md",
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

func TestConsensusMilestone_PathOmittedWhenAbsent(t *testing.T) {
	pub := &recordingPublisher{}
	resolver := resolverWithChain(map[string]string{
		"challenger_e": loopEntityID(t, "dispatch_root"),
	})
	challengerEntityID := loopEntityID(t, "challenger_e")
	er := &fakeEntityReader{
		entities: map[string]map[string]any{
			challengerEntityID: {
				reviewerNextActionPredicate: consensusAcceptAction,
				// no dev_via_spec.consensus.path — legacy challenger,
				// pre-Phase-C3 (or persona contract change deferred to
				// Phase C5).
			},
		},
	}
	s := NewConsensusMilestoneStamper(pub, resolver, er, testPlatform(), nil)

	ev := &agentic.LoopCompletedEvent{
		LoopID:  "challenger_e",
		Role:    challengerRole,
		Outcome: agentic.OutcomeSuccess,
	}
	if err := s.HandleLoopCompleted(context.Background(), ev); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if _, ok := pub.byPredicate(chainPredicateConsensusPath); ok {
		t.Error("path predicate should be omitted when dev_via_spec.consensus.path is absent")
	}
	if _, ok := pub.byPredicate(chainPredicateConsensusLoop); !ok {
		t.Error("consensus_loop predicate must still land regardless of path state")
	}
}

func TestConsensusMilestone_ResolverError_Skipped(t *testing.T) {
	pub := &recordingPublisher{}
	resolver := NewResolver(&fakeParentReader{err: errors.New("graph KV unavailable")}, testPlatform())
	challengerEntityID := loopEntityID(t, "challenger_e")
	er := &fakeEntityReader{
		entities: map[string]map[string]any{
			challengerEntityID: {
				reviewerNextActionPredicate:     consensusAcceptAction,
				challengerArtifactPathPredicate: "docs/consensus/x.md",
			},
		},
	}
	s := NewConsensusMilestoneStamper(pub, resolver, er, testPlatform(), nil)

	ev := &agentic.LoopCompletedEvent{
		LoopID:  "challenger_e",
		Role:    challengerRole,
		Outcome: agentic.OutcomeSuccess,
	}
	if err := s.HandleLoopCompleted(context.Background(), ev); err != nil {
		t.Fatalf("expected fail-soft, got error: %v", err)
	}
	if len(pub.triples) != 0 {
		t.Errorf("resolver error should drop the milestone, got %d triples", len(pub.triples))
	}
}
