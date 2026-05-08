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
//   - Phase 3 (chainpause re-point): blocked on upstream failed-loop
//     ancestry stamping; chain.paused.* still lands on the failed loop
//     entity. When the upstream fix lands and Phase 3 re-points the
//     chainpause writer, add chain.paused.* assertions here.
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
	dispatchEntityID := agentic.LoopExecutionEntityID("c360", "test", "dispatch_root")
	researcherEntityID := agentic.LoopExecutionEntityID("c360", "test", "researcher_a")
	reviewerEntityID := agentic.LoopExecutionEntityID("c360", "test", "research_reviewer_b")
	er := &staticEntityReader{
		entities: map[string]map[string]any{
			dispatchEntityID: {
				// dispatch is the chain root — no agent.loop.parent.
				"agent.loop.role": "dispatch",
			},
			researcherEntityID: {
				"agent.loop.parent":              dispatchEntityID,
				"research.artifact.test_harness": "meshtasticd-3.x",
				"research.artifact.actors_count": float64(3),
				"research.artifact.tasks_count":  float64(5),
			},
			reviewerEntityID: {
				"agent.loop.parent":       researcherEntityID,
				"coordinator.next_action": "approved",
				"lineage.researcher":      "researcher_a",
			},
		},
	}

	resolver := chain.NewResolver(er, platform)

	dispatched := chain.NewDispatchedStamper(pub, platform, nil)
	research := chain.NewResearchMilestoneStamper(pub, resolver, er, platform, nil)

	// We invoke handlers directly. CompletionSubscriber's NATS plumbing
	// is covered by chain/subscriber_test.go; this contract test focuses
	// on the per-handler chain-entity output across the pipeline.
	now := time.Now().UTC()

	// Drive three events in chain order. We invoke the handlers directly
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
	}
	for _, ev := range events {
		if err := dispatched.HandleLoopCompleted(context.Background(), ev); err != nil {
			t.Fatalf("DispatchedStamper.HandleLoopCompleted(%s) error: %v", ev.LoopID, err)
		}
		if err := research.HandleLoopCompleted(context.Background(), ev); err != nil {
			t.Fatalf("ResearchMilestoneStamper.HandleLoopCompleted(%s) error: %v", ev.LoopID, err)
		}
	}

	chainEntityID := agentic.ChainExecutionEntityID("c360", "test", "dispatch_root")
	got := pub.bySubject(chainEntityID)

	// Every predicate ADR-038 §D2 documents AND PR B currently lands.
	// Comments name which phase each comes from so the next reviewer can
	// trace.
	want := map[string]any{
		"chain.dispatched_at":                 nil,            // Phase 1b — RFC3339 string; presence-only check
		"chain.research_artifact_loop":        "researcher_a", // Phase 2
		"chain.research_artifact.harness":     "meshtasticd-3.x",
		"chain.research_artifact.actor_count": 3,
		"chain.research_artifact.task_count":  5,
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
		"chain.paused.cause",                    // Phase 3 (upstream blocked)
		"chain.spec_artifact_loop",              // Phase 4 (writes from emit_dev_via_spec, not the chain package)
		"chain.spec_artifact.path",              // Phase 4
		"chain.spec_artifact.check_count",       // Phase 4
		"chain.evidence.summary",                // Phase 5 (writes from evidence preprocessor, not the chain package)
		"chain.evidence.summary_ready",          // Phase 5
		"chain.research_artifact.path",          // PR C (markdown rendering)
		"chain.dispatched_at.observed_fallback", // Phase 1b — only on zero-CompletedAt path; happy path must not emit
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
