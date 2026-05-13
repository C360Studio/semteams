package contract

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/c360studio/semstreams/agentic"
	"github.com/c360studio/semstreams/message"
	"github.com/c360studio/semstreams/types"
	"github.com/c360studio/semteams/cmd/semteams/chain"
)

// recordingTriplePublisher captures every triple write across both the
// dispatched and research milestone stampers so the coverage assertion
// can scope to a single subject (the chain entity).
type recordingTriplePublisher struct {
	mu      sync.Mutex
	triples []message.Triple
}

func (r *recordingTriplePublisher) AddTriple(_ context.Context, t message.Triple) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.triples = append(r.triples, t)
	return nil
}

func (r *recordingTriplePublisher) AddTriplesBatch(ctx context.Context, triples []message.Triple) error {
	for _, t := range triples {
		if err := r.AddTriple(ctx, t); err != nil {
			return err
		}
	}
	return nil
}

func (r *recordingTriplePublisher) bySubject(subject string) map[string]any {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := map[string]any{}
	for _, t := range r.triples {
		if t.Subject == subject {
			out[t.Predicate] = t.Object
		}
	}
	return out
}

// staticEntityReader returns pre-populated triples per entity, plus
// satisfies chain.ParentReader so a Resolver wires cleanly.
type staticEntityReader struct {
	entities map[string]map[string]any
}

func (s *staticEntityReader) ReadEntity(_ context.Context, entityID string) (map[string]any, error) {
	if m, ok := s.entities[entityID]; ok {
		return m, nil
	}
	return map[string]any{}, nil
}

func (s *staticEntityReader) ReadParent(_ context.Context, loopID string) (string, error) {
	entityID := agentic.LoopExecutionEntityID("c360", "test", loopID)
	if m, ok := s.entities[entityID]; ok {
		if parent, _ := m["agent.loop.parent"].(string); parent != "" {
			return parent, nil
		}
	}
	return "", nil
}

// TestChainEntityCoverage_PR_B_Pipeline drives a full successful chain
// pass through every PR B chain-milestone CompletionHandler and asserts
// the chain entity ends up with the predicate union ADR-038 §D2
// documents for the milestones currently shipping.
//
// Why this lives in test/contract: drift between handler logic and the
// documented chain schema is the failure mode this PR exists to catch.
// A unit test inside chain/ asserts each handler in isolation; this
// contract test asserts the cross-handler shape of what a healthy chain
// produces, which is what downstream consumers (ops agent, future
// cross-arc rules) read.
//
// Phases NOT yet covered:
//   - Phase 3 (chainpause re-point): SHIPPED post-semstreams beta.56/.57.
//     chain.paused.* and chain.decision.* now land on the canonical
//     chain entity (resolved from the failed loop's ancestry).
//     chainpause uses a SEPARATE subscriber on agent.failed.* — its
//     own package tests assert the chain entity Subject directly, so
//     this contract test still excludes chain.paused.* from the
//     positive assertion (the CompletionSubscriber here only fires
//     on agent.complete.*, never agent.failed.*).
//   - Phase 4 / Phase 5 milestones (chain.spec_artifact.*,
//     chain.evidence.*) — those are tested per-package with the same
//     fail-soft behaviour; this test focuses on the CompletionSubscriber
//     pipeline (dispatched + research) which is the only multi-handler
//     path through the chain package itself.
func TestChainEntityCoverage_PR_B_Pipeline(t *testing.T) {
	platform := types.PlatformMeta{Org: "c360", Platform: "test"}
	pub := &recordingTriplePublisher{}

	// Build a chain ancestry that mirrors the OSH-Meshtastic happy path:
	//   dispatch_root → researcher_a → research_reviewer_b
	//                 → planner_c → dev_via_spec_reviewer_d
	dispatchEntityID := agentic.LoopExecutionEntityID("c360", "test", "dispatch_root")
	researcherEntityID := agentic.LoopExecutionEntityID("c360", "test", "researcher_a")
	researchReviewerEntityID := agentic.LoopExecutionEntityID("c360", "test", "research_reviewer_b")
	plannerEntityID := agentic.LoopExecutionEntityID("c360", "test", "planner_c")
	devReviewerEntityID := agentic.LoopExecutionEntityID("c360", "test", "dev_via_spec_reviewer_d")
	er := &staticEntityReader{
		entities: map[string]map[string]any{
			dispatchEntityID: {
				// dispatch is the chain root — no agent.loop.parent.
				"agent.loop.role": "dispatch",
			},
			researcherEntityID: {
				"agent.loop.parent":              dispatchEntityID,
				"research.artifact.test_harness": "meshtasticd-2.x",
				"research.artifact.actors_count": float64(3),
				"research.artifact.tasks_count":  float64(5),
				// emit_research_artifact's renderMarkdown writes to
				// docs/research/<slug>.md where <slug> is title-derived
				// and ends with "-research". The chain milestone strips
				// the trailing "-research" + ".md" to derive the
				// chain.slug.stem (smoke #8 run-5 D2 fix).
				"research.artifact.path": "docs/research/2026-05-08-osh-meshtastic-driver-research.md",
			},
			researchReviewerEntityID: {
				"agent.loop.parent":       researcherEntityID,
				"coordinator.next_action": "approved",
				"lineage.researcher":      "researcher_a",
			},
			plannerEntityID: {
				"agent.loop.parent":      researchReviewerEntityID,
				"dev_via_spec.plan.path": "docs/plans/2026-05-08-osh-meshtastic-plan.md",
			},
			devReviewerEntityID: {
				"agent.loop.parent":       plannerEntityID,
				"coordinator.next_action": "approved",
			},
		},
	}

	resolver := chain.NewResolver(er, platform)

	dispatched := chain.NewDispatchedStamper(pub, platform, nil)
	research := chain.NewResearchMilestoneStamper(pub, resolver, er, platform, nil)
	planMilestone := chain.NewPlanMilestoneStamper(pub, resolver, er, platform, nil)
	needsReview := chain.NewNeedsReviewStamper(pub, resolver, er, platform, nil)

	// We invoke handlers directly. CompletionSubscriber's NATS plumbing
	// is covered by chain/subscriber_test.go; this contract test focuses
	// on the per-handler chain-entity output across the pipeline.
	now := time.Now().UTC()

	// Drive five events in chain order. We invoke the handlers directly
	// rather than going through NATS — the subscriber's NATS plumbing is
	// covered by chain/subscriber_test.go.
	events := []*agentic.LoopCompletedEvent{
		{
			LoopID:       "dispatch_root",
			Role:         "dispatch",
			Outcome:      agentic.OutcomeSuccess,
			ParentLoopID: "",
			CompletedAt:  now,
		},
		{
			LoopID:       "researcher_a",
			Role:         "researcher",
			Outcome:      agentic.OutcomeSuccess,
			ParentLoopID: "dispatch_root",
			CompletedAt:  now.Add(time.Second),
		},
		{
			LoopID:       "research_reviewer_b",
			Role:         "research-reviewer",
			Outcome:      agentic.OutcomeSuccess,
			ParentLoopID: "researcher_a",
			CompletedAt:  now.Add(2 * time.Second),
		},
		{
			LoopID:       "planner_c",
			Role:         "dev-via-spec-planner",
			Outcome:      agentic.OutcomeSuccess,
			ParentLoopID: "research_reviewer_b",
			CompletedAt:  now.Add(3 * time.Second),
		},
		{
			LoopID:       "dev_via_spec_reviewer_d",
			Role:         "dev-via-spec-reviewer",
			Outcome:      agentic.OutcomeSuccess,
			ParentLoopID: "planner_c",
			CompletedAt:  now.Add(4 * time.Second),
		},
	}
	for _, ev := range events {
		if err := dispatched.HandleLoopCompleted(context.Background(), ev); err != nil {
			t.Fatalf("DispatchedStamper.HandleLoopCompleted(%s) error: %v", ev.LoopID, err)
		}
		if err := research.HandleLoopCompleted(context.Background(), ev); err != nil {
			t.Fatalf("ResearchMilestoneStamper.HandleLoopCompleted(%s) error: %v", ev.LoopID, err)
		}
		if err := planMilestone.HandleLoopCompleted(context.Background(), ev); err != nil {
			t.Fatalf("PlanMilestoneStamper.HandleLoopCompleted(%s) error: %v", ev.LoopID, err)
		}
		if err := needsReview.HandleLoopCompleted(context.Background(), ev); err != nil {
			t.Fatalf("NeedsReviewStamper.HandleLoopCompleted(%s) error: %v", ev.LoopID, err)
		}
	}

	chainEntityID := agentic.ChainExecutionEntityID("c360", "test", "dispatch_root")
	got := pub.bySubject(chainEntityID)

	// Every predicate ADR-038 §D2 documents AND PR B currently lands.
	// Comments name which phase each comes from so the next reviewer can
	// trace.
	want := map[string]any{
		"chain.dispatched.at":                 nil,            // Phase 1b — RFC3339 string; presence-only check
		"chain.research_artifact.loop":        "researcher_a", // Phase 2 (3-part rename 2026-05-11)
		"chain.research_artifact.harness":     "meshtasticd-2.x",
		"chain.research_artifact.actor_count": 3,
		"chain.research_artifact.task_count":  5,
		"chain.research_artifact.path":        "docs/research/2026-05-08-osh-meshtastic-driver-research.md", // PR C Phase C1
		"chain.slug.stem":                     "2026-05-08-osh-meshtastic-driver",                           // smoke #8 run-5 D2 fix — stem derived from research path; downstream tools compose <stem>-{plan,consensus,implementation}
		"chain.plan.loop":                     "planner_c",                                                  // PR C Phase C2 (3-part rename 2026-05-11)
		"chain.plan.reviewer_loop":            "dev_via_spec_reviewer_d",                                    // smoke #8 run-5 D1 fix — reviewer loop ID for emit_consensus depends_on.reviewer_loop (3-part rename 2026-05-11)
		"chain.plan.path":                     "docs/plans/2026-05-08-osh-meshtastic-plan.md",               // PR C Phase C2
	}
	for pred, wantObj := range want {
		gotObj, ok := got[pred]
		if !ok {
			t.Errorf("predicate %q missing on chain entity (PR B coverage gap)", pred)
			continue
		}
		if wantObj == nil {
			continue // presence-only assertion
		}
		if gotObj != wantObj {
			t.Errorf("predicate %q object = %v (%T), want %v (%T)", pred, gotObj, gotObj, wantObj, wantObj)
		}
	}

	// Negative assertion: predicates from milestones not yet in PR B
	// scope should NOT appear (catches accidental cross-handler bleed).
	notYet := []string{
		"chain.paused.cause",                  // Phase 3 SHIPPED — but writes from chainpause's agent.failed.* subscriber, not the agent.complete.* path this test drives
		"chain.decision.verb",                 // Phase 3 SHIPPED — same reason; written by DecisionHandler, not a CompletionHandler
		"chain.spec_artifact.loop",            // Phase 4 (writes from emit_dev_via_spec, not the chain package)
		"chain.spec_artifact.path",            // Phase 4
		"chain.spec_artifact.check_count",     // Phase 4
		"chain.evidence.summary_ready",        // Phase 5 (writes from evidence preprocessor, not the chain package)
		"chain.evidence.summary.path",         // PR C Phase C4 (writes from evidence preprocessor, not the chain package)
		"chain.dispatched.observed_fallback",  // Phase 1b — only on zero-CompletedAt path; happy path must not emit
		"chain.needs_review.classification",   // ADR-039 Phase 1 Slice B — only on builder needs_clarification; happy path's tests_passing builder must not emit
		"chain.needs_review.producer_loop_id", // same
		"chain.needs_review.producer_role",    // same
		"chain.needs_review.reason",           // same
		"chain.needs_review.observed_at",      // same
	}
	for _, pred := range notYet {
		if _, ok := got[pred]; ok {
			t.Errorf("predicate %q must NOT be written by PR B chain-package handlers (look for accidental bleed)", pred)
		}
	}
}

// TestChainEntityCoverage_NoBleedOntoLoopEntity confirms the chain
// stampers in PR B don't write onto loop entities (they own the chain
// entity exclusively). Loop-entity predicate writes happen in
// emitspecartifact, evidence preprocessor, etc. — not in the chain
// package. A handler that accidentally targets a loop subject would
// silently double-write and confuse downstream graph queries.
func TestChainEntityCoverage_NoBleedOntoLoopEntity(t *testing.T) {
	platform := types.PlatformMeta{Org: "c360", Platform: "test"}
	pub := &recordingTriplePublisher{}

	er := &staticEntityReader{
		entities: map[string]map[string]any{
			agentic.LoopExecutionEntityID("c360", "test", "researcher_a"): {
				"research.artifact.test_harness": "h",
				"research.artifact.actors_count": float64(1),
				"research.artifact.tasks_count":  float64(1),
			},
			agentic.LoopExecutionEntityID("c360", "test", "reviewer_b"): {
				"agent.loop.parent":       agentic.LoopExecutionEntityID("c360", "test", "researcher_a"),
				"coordinator.next_action": "approved",
				"lineage.researcher":      "researcher_a",
			},
		},
	}
	resolver := chain.NewResolver(er, platform)

	dispatched := chain.NewDispatchedStamper(pub, platform, nil)
	research := chain.NewResearchMilestoneStamper(pub, resolver, er, platform, nil)

	rootEv := &agentic.LoopCompletedEvent{
		LoopID:       "dispatch_root",
		Role:         "dispatch",
		Outcome:      agentic.OutcomeSuccess,
		ParentLoopID: "",
		CompletedAt:  time.Now().UTC(),
	}
	reviewerEv := &agentic.LoopCompletedEvent{
		LoopID:       "reviewer_b",
		Role:         "research-reviewer",
		Outcome:      agentic.OutcomeSuccess,
		ParentLoopID: "researcher_a",
		CompletedAt:  time.Now().UTC(),
	}
	if err := dispatched.HandleLoopCompleted(context.Background(), rootEv); err != nil {
		t.Fatalf("dispatched: %v", err)
	}
	if err := research.HandleLoopCompleted(context.Background(), reviewerEv); err != nil {
		t.Fatalf("research: %v", err)
	}

	pub.mu.Lock()
	defer pub.mu.Unlock()
	for _, t2 := range pub.triples {
		// Every chain-package triple must target a chain entity ID
		// (containing ".agent.chain.execution.").
		if !containsSubstr(t2.Subject, ".agent.chain.execution.") {
			t.Errorf("chain handler wrote triple to non-chain subject: subject=%q predicate=%q (a chain stamper must only target chain entities)", t2.Subject, t2.Predicate)
		}
	}
}

func containsSubstr(s, substr string) bool {
	if len(substr) > len(s) {
		return false
	}
	for i := 0; i+len(substr) <= len(s); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
