package chain

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/c360studio/semstreams/agentic"
	"github.com/c360studio/semstreams/message"
	"github.com/c360studio/semstreams/types"
)

// Predicate names for the plan-milestone cluster on the chain entity.
// Schema-stable per ADR-038 D2:
//
//	chain.plan_loop                  <planner_loop_id_at_approval>
//	chain.plan.path                  <docs/plans/<slug>.md>
//
// Path absence is the legacy-plan signal (planners that ran before
// emit_plan landed don't stamp it); the chain triple is omitted, not
// fabricated. Same shape as the research milestone's path handling.
//
// Path shape: relative to process working directory under the default
// outputDir ("docs/plans"); absolute when operators set
// SEMTEAMS_PLAN_DIR to an absolute path. Consumers must tolerate both.
const (
	chainPredicatePlanLoop = "chain.plan_loop"
	chainPredicatePlanPath = "chain.plan.path"

	chainPlanSource = "chain.plan"
)

// Predicate names on the planner's loop entity (read side).
const (
	plannerArtifactPathPredicate = "dev_via_spec.plan.path"

	// devViaSpecReviewerRole is the role we milestone on. The reviewer
	// is one hop above the planner in the dev-via-spec arc; on
	// outcome=success + coordinator.next_action=approved, the planner
	// the reviewer just signed off on is the planner whose plan we
	// promote to chain.plan_loop.
	devViaSpecReviewerRole = "dev-via-spec-reviewer"

	// planApprovedAction is the coordinator.next_action value the
	// reviewer's persona stamps via decide(action="approved"). Same
	// token the research-reviewer uses; sourced from the dev-via-spec
	// rules' action_allowlist on rule_03 (the
	// reviewer-approved-to-challenger rule).
	planApprovedAction = "approved"

	// agentLoopParentPredicate is the upstream-stamped triple naming
	// the parent loop's full 6-part entity ID. Stamped by graph_writer
	// at completion (and at failure since semstreams beta.56). The plan
	// milestone walks ONE hop via this predicate to find the planner
	// whose plan the reviewer just approved.
	agentLoopParentPredicate = "agent.loop.parent"
)

// PlanMilestoneStamper writes the chain.plan.* triple cluster on the
// chain entity when a dev-via-spec-reviewer loop completes with
// coordinator.next_action="approved".
//
// Read pattern (graph reads on completed loops):
//  1. Reviewer's loop entity → coordinator.next_action (= "approved"
//     gates the milestone) and agent.loop.parent (= planner's full
//     entity ID).
//  2. Planner's loop entity → dev_via_spec.plan.path (set by emit_plan
//     when markdown rendering succeeds; absent on legacy plans).
//  3. Resolver walks ancestry from the planner's loop_id. The
//     planner's agent.loop.parent is stamped at completion regardless
//     of which rule spawned it (initial: research-mode-transition
//     rule_03; retries: dev-via-spec rule_02 reviewer-rejected or
//     rule_04 challenger-concerns). graph_writer stamps the triple on
//     any loop with a non-empty ParentLoopID, so the walk is reliable
//     for every planner pass.
//
// Write pattern: chain.plan_loop always (when reviewer approves);
// chain.plan.path only when the planner stamped dev_via_spec.plan.path
// (i.e., emit_plan was called with markdown rendering enabled).
//
// Fail-soft on every step (read failures, malformed parent, resolver
// errors): log Warn and skip. The dev-via-spec arc continues even if
// the chain milestone can't land — the chain is observability for
// cross-arc consumers; rule firing depends on per-loop triples that
// landed earlier.
type PlanMilestoneStamper struct {
	publisher TriplePublisher
	resolver  *Resolver
	entities  EntityTripleReader
	platform  types.PlatformMeta
	logger    *slog.Logger
}

// NewPlanMilestoneStamper constructs a stamper.
func NewPlanMilestoneStamper(
	pub TriplePublisher,
	resolver *Resolver,
	entities EntityTripleReader,
	platform types.PlatformMeta,
	logger *slog.Logger,
) *PlanMilestoneStamper {
	if logger == nil {
		logger = slog.Default()
	}
	return &PlanMilestoneStamper{
		publisher: pub,
		resolver:  resolver,
		entities:  entities,
		platform:  platform,
		logger:    logger,
	}
}

// HandleLoopCompleted is the subscription entry point. Filters to
// dev-via-spec-reviewer success and the approved coordinator action;
// for non-matching events returns nil (other handlers may fire).
func (s *PlanMilestoneStamper) HandleLoopCompleted(ctx context.Context, ev *agentic.LoopCompletedEvent) error {
	if ev == nil {
		return nil
	}
	if ev.Role != devViaSpecReviewerRole || ev.Outcome != agentic.OutcomeSuccess {
		return nil
	}
	if err := validateLoopID(ev.LoopID); err != nil {
		return fmt.Errorf("chain.PlanMilestoneStamper: %w", err)
	}

	reviewerEntityID := agentic.LoopExecutionEntityID(s.platform.Org, s.platform.Platform, ev.LoopID)
	reviewerTriples, err := s.entities.ReadEntity(ctx, reviewerEntityID)
	if err != nil {
		s.logger.Warn("plan milestone: read reviewer entity failed",
			slog.String("reviewer_loop_id", ev.LoopID),
			slog.String("error", err.Error()))
		return nil
	}
	if action, _ := reviewerTriples[reviewerNextActionPredicate].(string); action != planApprovedAction {
		// rejected / insufficient / etc. — not a milestone.
		return nil
	}

	// Walk one hop to the planner via agent.loop.parent. The
	// reviewer's parent IS the planner the reviewer just approved.
	parentEntityID, _ := reviewerTriples[agentLoopParentPredicate].(string)
	if parentEntityID == "" {
		s.logger.Warn("plan milestone: reviewer entity missing agent.loop.parent; skipping chain triples",
			slog.String("reviewer_loop_id", ev.LoopID))
		return nil
	}
	plannerLoopID, ok := agentic.LoopIDFromExecutionEntityID(parentEntityID)
	if !ok {
		s.logger.Warn("plan milestone: agent.loop.parent value malformed; skipping chain triples",
			slog.String("reviewer_loop_id", ev.LoopID),
			slog.String("parent_entity_id", parentEntityID))
		return nil
	}
	if err := validateLoopID(plannerLoopID); err != nil {
		s.logger.Warn("plan milestone: planner loop_id failed validation; skipping chain triples",
			slog.String("reviewer_loop_id", ev.LoopID),
			slog.String("planner_loop_id", plannerLoopID),
			slog.String("error", err.Error()))
		return nil
	}

	plannerEntityID := agentic.LoopExecutionEntityID(s.platform.Org, s.platform.Platform, plannerLoopID)
	plannerTriples, err := s.entities.ReadEntity(ctx, plannerEntityID)
	if err != nil {
		s.logger.Warn("plan milestone: read planner entity failed",
			slog.String("planner_loop_id", plannerLoopID),
			slog.String("error", err.Error()))
		return nil
	}

	chainEntityID, err := s.resolver.ChainEntityID(ctx, plannerLoopID)
	if err != nil {
		s.logger.Warn("plan milestone: chain ancestry walk failed; skipping chain triples",
			slog.String("planner_loop_id", plannerLoopID),
			slog.String("error", err.Error()))
		return nil
	}

	now := time.Now().UTC()
	base := func(pred string, obj any) message.Triple {
		return message.Triple{
			Subject:    chainEntityID,
			Predicate:  pred,
			Object:     obj,
			Source:     chainPlanSource,
			Timestamp:  now,
			Confidence: 1.0,
		}
	}
	triples := []message.Triple{
		base(chainPredicatePlanLoop, plannerLoopID),
	}
	if path, _ := plannerTriples[plannerArtifactPathPredicate].(string); path != "" {
		triples = append(triples, base(chainPredicatePlanPath, path))
	}

	for _, t := range triples {
		if err := s.publisher.AddTriple(ctx, t); err != nil {
			s.logger.Warn("plan milestone: chain triple write failed",
				slog.String("predicate", t.Predicate),
				slog.String("chain_entity", chainEntityID),
				slog.String("error", err.Error()))
		}
	}

	s.logger.Debug("plan milestone: chain triples stamped",
		slog.String("reviewer_loop_id", ev.LoopID),
		slog.String("planner_loop_id", plannerLoopID),
		slog.String("chain_entity", chainEntityID),
		slog.Int("triples_attempted", len(triples)))
	return nil
}
