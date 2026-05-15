package chain

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/c360studio/semstreams/agentic"
	"github.com/c360studio/semstreams/message"
	"github.com/c360studio/semstreams/types"

	"github.com/c360studio/semteams/cmd/semteams/slug"
)

// Predicate names for the research-milestone cluster on the chain
// entity. Schema-stable — chain.research_artifact.loop is the
// canonical anchor; .harness, .actor_count, .task_count, .path are
// projections from research.artifact.* on the researcher's loop entity.
// The .path predicate is propagated only when emit_research_artifact's
// markdown rendering succeeded (PR C Phase C1); absent on legacy
// research artifacts emitted before the markdown landing.
//
// Path shape: relative to the process working directory under the
// default outputDir ("docs/research"); absolute when operators set
// SEMTEAMS_RESEARCH_ARTIFACT_DIR to an absolute path (or in tests via
// t.TempDir()). Consumers of chain.research_artifact.path must tolerate
// both — emit_research_artifact's renderMarkdown does NOT normalise.
const (
	// Read-side wire format lives in predicates.go (Predicate*) so
	// downstream consumers reference the same constant. Local aliases
	// kept short for stamper-side readability.
	chainPredicateResearchArtifactLoop       = PredicateResearchArtifactLoop
	chainPredicateResearchArtifactHarness    = "chain.research_artifact.harness"
	chainPredicateResearchArtifactActorCount = "chain.research_artifact.actor_count"
	chainPredicateResearchArtifactTaskCount  = "chain.research_artifact.task_count"
	chainPredicateResearchArtifactPath       = "chain.research_artifact.path"
	chainPredicateSlugStem                   = PredicateSlugStem

	// chainResearchSource tags chain entity triples this stamper writes
	// so a graph query can scope to the emitter (parallel to
	// "chain.dispatched" and "chain.spec_artifact").
	chainResearchSource = "chain.research"

	// researchApprovedAction is the coordinator.next_action value the
	// reviewer's persona stamps via decide(action="approved"). Sourced
	// from configs/personas/fragments/research-reviewer + reviewer-research
	// (both use the same approved-token vocabulary).
	researchApprovedAction = "approved"
)

// researchReviewerRoles is the role-name set whose approve-action stamps the
// research-artifact milestone. Includes legacy `research-reviewer`
// (configs/rules/research-iterative/*) and ADR-041 MVP `reviewer-research`
// (configs/rules/research-mode-transition/01a-researcher-synthesize-to-reviewer-research.json).
// Stamper fires on either; downstream chain.research_artifact.* predicates
// don't care which reviewer flavour produced the approval.
var researchReviewerRoles = map[string]struct{}{
	"research-reviewer": {}, // legacy research-iterative
	"reviewer-research": {}, // ADR-041 MVP reviewer-research mode
}

// Predicate names on the researcher's loop entity (read side).
const (
	researcherTestHarnessPredicate = "research.artifact.test_harness"
	researcherActorsCountPredicate = "research.artifact.actors_count"
	researcherTasksCountPredicate  = "research.artifact.tasks_count"
	researcherPathPredicate        = "research.artifact.path"

	// reviewerNextActionPredicate matches the upstream framework's
	// canonical name (vocabulary/agentic/predicates.go:616 —
	// CoordinatorNextAction = "coordinator.decision.next_action").
	// Renamed from the pre-beta.64 "coordinator.next_action" form in
	// ADR-041 Phase 3 mock-LLM smoke validation when the decide tool
	// proved to stamp the new name only.
	reviewerNextActionPredicate        = "coordinator.decision.next_action"
	reviewerLineageResearcherPredicate = "lineage.researcher"
)

// ResearchMilestoneStamper writes the chain.research_artifact.* triple
// cluster on the chain entity when a research-reviewer loop completes
// with coordinator.next_action="approved".
//
// Read pattern (graph reads on a completed loop):
//  1. Reviewer's loop entity → coordinator.next_action (= "approved"
//     gates this milestone) and lineage.researcher (= the researcher's
//     loop_id whose artifact was approved).
//  2. Researcher's loop entity → research.artifact.test_harness,
//     research.artifact.actors_count, research.artifact.tasks_count.
//  3. Resolver walks ancestry from the researcher's loop_id (a
//     completed loop with full agent.loop.parent stamped) to find
//     chain_id → chain entity ID.
//
// Write pattern: 4 triples on the chain entity, all under the
// chain.research_artifact[_loop|.harness|.actor_count|.task_count]
// namespace. ADR-038 D2 lists chain.research_artifact.path in the same
// cluster — that lands in PR C when emit_research_artifact gains
// markdown rendering.
//
// Fail-soft: every step that can fail logs Warn and returns. The
// loop-entity research.artifact.* triples are unaffected; rule_03 is
// unaffected. Phase 6's contract test surfaces drift where chain
// triples should have landed but didn't.
type ResearchMilestoneStamper struct {
	publisher TriplePublisher
	resolver  *Resolver
	entities  EntityTripleReader
	platform  types.PlatformMeta
	logger    *slog.Logger
}

// NewResearchMilestoneStamper constructs a stamper.
func NewResearchMilestoneStamper(
	pub TriplePublisher,
	resolver *Resolver,
	entities EntityTripleReader,
	platform types.PlatformMeta,
	logger *slog.Logger,
) *ResearchMilestoneStamper {
	if logger == nil {
		logger = slog.Default()
	}
	return &ResearchMilestoneStamper{
		publisher: pub,
		resolver:  resolver,
		entities:  entities,
		platform:  platform,
		logger:    logger,
	}
}

// HandleLoopCompleted is the subscription entry point. Filters to
// research-reviewer / reviewer-research success and the approved coordinator
// action; for non-matching events returns nil (other handlers may fire).
func (s *ResearchMilestoneStamper) HandleLoopCompleted(ctx context.Context, ev *agentic.LoopCompletedEvent) error {
	if ev == nil {
		return nil
	}
	if _, ok := researchReviewerRoles[ev.Role]; !ok || ev.Outcome != agentic.OutcomeSuccess {
		return nil
	}
	if err := validateLoopID(ev.LoopID); err != nil {
		return fmt.Errorf("chain.ResearchMilestoneStamper: %w", err)
	}

	reviewerEntityID := agentic.LoopExecutionEntityID(s.platform.Org, s.platform.Platform, ev.LoopID)
	reviewerTriples, err := s.entities.ReadEntity(ctx, reviewerEntityID)
	if err != nil {
		s.logger.Warn("research milestone: read reviewer entity failed",
			slog.String("reviewer_loop_id", ev.LoopID),
			slog.String("error", err.Error()))
		return nil
	}

	if action, _ := reviewerTriples[reviewerNextActionPredicate].(string); action != researchApprovedAction {
		// insufficient or other terminal action — not a milestone.
		return nil
	}

	researcherLoopID, _ := reviewerTriples[reviewerLineageResearcherPredicate].(string)
	if researcherLoopID == "" {
		// Slice 4c lineage threading should populate this; if it's
		// missing the chain ancestry can't be tied to the artifact.
		// Drop the milestone rather than guess.
		s.logger.Warn("research milestone: reviewer entity missing lineage.researcher; skipping chain triples",
			slog.String("reviewer_loop_id", ev.LoopID))
		return nil
	}
	if err := validateLoopID(researcherLoopID); err != nil {
		s.logger.Warn("research milestone: lineage.researcher value malformed; skipping chain triples",
			slog.String("reviewer_loop_id", ev.LoopID),
			slog.String("researcher_loop_id", researcherLoopID),
			slog.String("error", err.Error()))
		return nil
	}

	researcherEntityID := agentic.LoopExecutionEntityID(s.platform.Org, s.platform.Platform, researcherLoopID)
	researcherTriples, err := s.entities.ReadEntity(ctx, researcherEntityID)
	if err != nil {
		s.logger.Warn("research milestone: read researcher entity failed",
			slog.String("researcher_loop_id", researcherLoopID),
			slog.String("error", err.Error()))
		return nil
	}

	chainEntityID, err := s.resolver.ChainEntityID(ctx, researcherLoopID)
	if err != nil {
		s.logger.Warn("research milestone: chain ancestry walk failed; skipping chain triples",
			slog.String("researcher_loop_id", researcherLoopID),
			slog.String("error", err.Error()))
		return nil
	}

	now := time.Now().UTC()
	base := func(pred string, obj any) message.Triple {
		return message.Triple{
			Subject:    chainEntityID,
			Predicate:  pred,
			Object:     obj,
			Source:     chainResearchSource,
			Timestamp:  now,
			Confidence: 1.0,
		}
	}

	triples := []message.Triple{
		base(chainPredicateResearchArtifactLoop, researcherLoopID),
	}
	// .harness is optional — researcher may have left it empty (no
	// catalog match; persona writes a needs_test_harness gap to
	// open_gaps instead). Stamp only when present so a graph query
	// can distinguish "harness not selected" from "harness empty
	// string" cleanly.
	if harness, _ := researcherTriples[researcherTestHarnessPredicate].(string); harness != "" {
		triples = append(triples, base(chainPredicateResearchArtifactHarness, harness))
	}
	if actorCount := tripleAsInt(researcherTriples[researcherActorsCountPredicate]); actorCount >= 0 {
		triples = append(triples, base(chainPredicateResearchArtifactActorCount, actorCount))
	}
	if taskCount := tripleAsInt(researcherTriples[researcherTasksCountPredicate]); taskCount >= 0 {
		triples = append(triples, base(chainPredicateResearchArtifactTaskCount, taskCount))
	}
	// .path absence is the legacy-artifact signal (researchers that ran
	// before ADR-038 PR C Phase C1 don't stamp it). Stamp only when
	// present so a graph query can distinguish "no markdown rendered"
	// from "markdown at empty path."
	//
	// chain.slug.stem rides the same condition: it can only be derived
	// when the researcher rendered a markdown path. Same emit-time
	// invariant — present together or absent together.
	if path, _ := researcherTriples[researcherPathPredicate].(string); path != "" {
		triples = append(triples, base(chainPredicateResearchArtifactPath, path))
		if stem := slug.StemFromPath(path, "research"); stem != "" {
			triples = append(triples, base(chainPredicateSlugStem, stem))
		}
	}

	for _, t := range triples {
		if err := s.publisher.AddTriple(ctx, t); err != nil {
			s.logger.Warn("research milestone: chain triple write failed",
				slog.String("predicate", t.Predicate),
				slog.String("chain_entity", chainEntityID),
				slog.String("error", err.Error()))
		}
	}

	s.logger.Debug("research milestone: chain triples stamped",
		slog.String("reviewer_loop_id", ev.LoopID),
		slog.String("researcher_loop_id", researcherLoopID),
		slog.String("chain_entity", chainEntityID),
		slog.Int("triples_attempted", len(triples)))
	return nil
}

// tripleAsInt extracts an int from a graph-triple Object that the JSON
// decoder may have unmarshalled as float64 (numeric literals) or
// string. Returns -1 on type miss so callers can skip absent counts.
func tripleAsInt(v any) int {
	switch n := v.(type) {
	case int:
		return n
	case int64:
		return int(n)
	case float64:
		return int(n)
	default:
		return -1
	}
}
