package chain

import (
	"context"
	"errors"
	"testing"

	"github.com/c360studio/semstreams/agentic"
)

// fakeEntityReader returns predicate maps keyed by entity_id. Used by
// both Resolver-via-fake-parent (already covered) and the new
// ResearchMilestoneStamper.
type fakeEntityReader struct {
	entities map[string]map[string]any
	err      error
	calls    int
}

func (f *fakeEntityReader) ReadEntity(_ context.Context, entityID string) (map[string]any, error) {
	f.calls++
	if f.err != nil {
		return nil, f.err
	}
	if m, ok := f.entities[entityID]; ok {
		return m, nil
	}
	return map[string]any{}, nil
}

// resolverWithChain is a small helper that wires a Resolver with a
// fakeParentReader (from resolver_test.go) so the test fixture mirrors
// production wiring shape.
func resolverWithChain(parents map[string]string) *Resolver {
	return NewResolver(&fakeParentReader{parents: parents}, testPlatform())
}

func TestResearchMilestone_NonReviewerNoOp(t *testing.T) {
	pub := &recordingPublisher{}
	resolver := resolverWithChain(nil)
	er := &fakeEntityReader{}
	s := NewResearchMilestoneStamper(pub, resolver, er, testPlatform(), nil)

	ev := &agentic.LoopCompletedEvent{
		LoopID:  "loop_x",
		Role:    "researcher",
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

func TestResearchMilestone_FailedReviewerNoOp(t *testing.T) {
	pub := &recordingPublisher{}
	resolver := resolverWithChain(nil)
	er := &fakeEntityReader{}
	s := NewResearchMilestoneStamper(pub, resolver, er, testPlatform(), nil)

	ev := &agentic.LoopCompletedEvent{
		LoopID:  "loop_x",
		Role:    "reviewer-research",
		Outcome: agentic.OutcomeFailed,
	}
	if err := s.HandleLoopCompleted(context.Background(), ev); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if er.calls != 0 || len(pub.triples) != 0 {
		t.Error("failed reviewer should skip both reads and writes")
	}
}

func TestResearchMilestone_InsufficientNoOp(t *testing.T) {
	pub := &recordingPublisher{}
	resolver := resolverWithChain(nil)

	reviewerEntityID := loopEntityID(t, "reviewer_1")
	er := &fakeEntityReader{
		entities: map[string]map[string]any{
			reviewerEntityID: {
				reviewerNextActionPredicate:        "insufficient",
				reviewerLineageResearcherPredicate: "researcher_a",
			},
		},
	}
	s := NewResearchMilestoneStamper(pub, resolver, er, testPlatform(), nil)

	ev := &agentic.LoopCompletedEvent{
		LoopID:  "reviewer_1",
		Role:    "reviewer-research",
		Outcome: agentic.OutcomeSuccess,
	}
	if err := s.HandleLoopCompleted(context.Background(), ev); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(pub.triples) != 0 {
		t.Errorf("insufficient milestone should write no triples, got %d", len(pub.triples))
	}
}

func TestResearchMilestone_ApprovedHappyPath(t *testing.T) {
	pub := &recordingPublisher{}
	// Chain: dispatch_root → researcher_a → reviewer_1
	resolver := resolverWithChain(map[string]string{
		"researcher_a": loopEntityID(t, "dispatch_root"),
		// dispatch_root has no parent → chain root
	})

	reviewerEntityID := loopEntityID(t, "reviewer_1")
	researcherEntityID := loopEntityID(t, "researcher_a")
	er := &fakeEntityReader{
		entities: map[string]map[string]any{
			reviewerEntityID: {
				reviewerNextActionPredicate:        "approved",
				reviewerLineageResearcherPredicate: "researcher_a",
			},
			researcherEntityID: {
				researcherTestHarnessPredicate: "meshtasticd-2.x",
				researcherActorsCountPredicate: float64(3), // JSON unmarshal shape
				researcherTasksCountPredicate:  float64(5),
				researcherPathPredicate:        "docs/research/2026-05-08-osh-meshtastic-driver-research.md",
			},
		},
	}
	s := NewResearchMilestoneStamper(pub, resolver, er, testPlatform(), nil)

	ev := &agentic.LoopCompletedEvent{
		LoopID:  "reviewer_1",
		Role:    "reviewer-research",
		Outcome: agentic.OutcomeSuccess,
	}
	if err := s.HandleLoopCompleted(context.Background(), ev); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	wantChainEntityID := "c360.test.agent.chain.execution.dispatch_root"
	wantPredicates := map[string]any{
		chainPredicateResearchArtifactLoop:       "researcher_a",
		chainPredicateResearchArtifactHarness:    "meshtasticd-2.x",
		chainPredicateResearchArtifactActorCount: 3,
		chainPredicateResearchArtifactTaskCount:  5,
		chainPredicateResearchArtifactPath:       "docs/research/2026-05-08-osh-meshtastic-driver-research.md",
		// chain.slug.stem rides path: basename minus -research minus .md.
		// Downstream emit-tools compose "<stem>-plan" / "<stem>-consensus"
		// / "<stem>-implementation" so the slug stays stable across the
		// chain (smoke #8 run-5 D2 fix).
		chainPredicateSlugStem: "2026-05-08-osh-meshtastic-driver",
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

func TestResearchMilestone_HarnessOmittedWhenEmpty(t *testing.T) {
	pub := &recordingPublisher{}
	resolver := resolverWithChain(map[string]string{
		"researcher_a": loopEntityID(t, "dispatch_root"),
	})
	reviewerEntityID := loopEntityID(t, "reviewer_1")
	researcherEntityID := loopEntityID(t, "researcher_a")
	er := &fakeEntityReader{
		entities: map[string]map[string]any{
			reviewerEntityID: {
				reviewerNextActionPredicate:        "approved",
				reviewerLineageResearcherPredicate: "researcher_a",
			},
			researcherEntityID: {
				// test_harness empty — researcher chose no catalog entry
				researcherTestHarnessPredicate: "",
				researcherActorsCountPredicate: float64(2),
				researcherTasksCountPredicate:  float64(1),
			},
		},
	}
	s := NewResearchMilestoneStamper(pub, resolver, er, testPlatform(), nil)

	ev := &agentic.LoopCompletedEvent{
		LoopID:  "reviewer_1",
		Role:    "reviewer-research",
		Outcome: agentic.OutcomeSuccess,
	}
	if err := s.HandleLoopCompleted(context.Background(), ev); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if _, ok := pub.byPredicate(chainPredicateResearchArtifactHarness); ok {
		t.Error("harness predicate should be omitted when test_harness is empty (signals 'no catalog match')")
	}
	if _, ok := pub.byPredicate(chainPredicateResearchArtifactLoop); !ok {
		t.Error("loop predicate must still land regardless of harness state")
	}
	// Path predicate absence is the legacy-artifact signal; with no
	// research.artifact.path on the researcher's entity, the chain
	// triple must not be fabricated.
	if _, ok := pub.byPredicate(chainPredicateResearchArtifactPath); ok {
		t.Error("path predicate should be omitted when research.artifact.path is absent on researcher loop")
	}
	// chain.slug.stem rides chain.research_artifact.path; absent path
	// → absent stem.
	if _, ok := pub.byPredicate(chainPredicateSlugStem); ok {
		t.Error("slug.stem predicate should be omitted when research.artifact.path is absent")
	}
}

func TestResearchMilestone_MissingLineageReseacher_Skipped(t *testing.T) {
	pub := &recordingPublisher{}
	resolver := resolverWithChain(nil)
	reviewerEntityID := loopEntityID(t, "reviewer_1")
	er := &fakeEntityReader{
		entities: map[string]map[string]any{
			reviewerEntityID: {
				reviewerNextActionPredicate: "approved",
				// lineage.researcher absent
			},
		},
	}
	s := NewResearchMilestoneStamper(pub, resolver, er, testPlatform(), nil)

	ev := &agentic.LoopCompletedEvent{
		LoopID:  "reviewer_1",
		Role:    "reviewer-research",
		Outcome: agentic.OutcomeSuccess,
	}
	if err := s.HandleLoopCompleted(context.Background(), ev); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(pub.triples) != 0 {
		t.Errorf("missing lineage.researcher should drop the milestone, got %d triples", len(pub.triples))
	}
}

func TestResearchMilestone_ResolverError_Skipped(t *testing.T) {
	pub := &recordingPublisher{}
	resolver := NewResolver(&fakeParentReader{err: errors.New("graph KV unavailable")}, testPlatform())
	reviewerEntityID := loopEntityID(t, "reviewer_1")
	researcherEntityID := loopEntityID(t, "researcher_a")
	er := &fakeEntityReader{
		entities: map[string]map[string]any{
			reviewerEntityID: {
				reviewerNextActionPredicate:        "approved",
				reviewerLineageResearcherPredicate: "researcher_a",
			},
			researcherEntityID: {
				researcherTestHarnessPredicate: "h",
			},
		},
	}
	s := NewResearchMilestoneStamper(pub, resolver, er, testPlatform(), nil)

	ev := &agentic.LoopCompletedEvent{
		LoopID:  "reviewer_1",
		Role:    "reviewer-research",
		Outcome: agentic.OutcomeSuccess,
	}
	if err := s.HandleLoopCompleted(context.Background(), ev); err != nil {
		t.Fatalf("expected fail-soft, got error: %v", err)
	}
	if len(pub.triples) != 0 {
		t.Errorf("resolver error should drop the milestone, got %d triples", len(pub.triples))
	}
}
