package phasevalidator

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/c360studio/semstreams/agentic"
	"github.com/c360studio/semstreams/message"
	"github.com/c360studio/semstreams/types"
	agvocab "github.com/c360studio/semstreams/vocabulary/agentic"

	"github.com/c360studio/semteams/cmd/semteams/chain"
)

const (
	// reviewerSpecRole is the role the spec-mode gate filters on.
	// Matches the ADR-041 collapsed reviewer role with "spec" mode
	// suffix (reviewer-spec / reviewer-qa / reviewer-research).
	reviewerSpecRole = "reviewer-spec"

	// approvedAction is the coordinator.next_action value that
	// triggers a builder spawn. Non-approved terminals
	// (insufficient / etc.) bypass the gate and route via the
	// existing reject paths.
	approvedAction = "approved"

	// specModeRejectMissingResearch is the only rejection reason
	// the spec-mode gate currently emits. Distinct from
	// "no_evidence_loop_ids" because the actual check is whether the
	// chain has a research-artifact stamped, not the reviewer's
	// self-reported list — the chain triple is the load-bearing audit.
	specModeRejectMissingResearch = "missing_research_artifact"

	triplesSourceSpecMode = "chain.spec_mode_gate"
)

// SpecModeGate implements chain.CompletionHandler and gates
// reviewer-spec→builder transitions per ADR-041 §"Risks mitigation".
// Concrete check: chain.research_artifact.loop is non-empty on the
// chain entity (i.e., a researcher upstream emitted an artifact, the
// reviewer-research approved it, and the chain stamped the canonical
// reference). Absence means the reviewer-spec is approving a spec
// without grounding in any research arc — the gate stalls the
// chain fail-safe.
//
// Why chain.research_artifact.loop rather than reading the
// reviewer's claimed evidence_loop_ids:
//
//   - The framework's `decide` tool only accepts (action, reason,
//     subtopics, retry_hint) — no evidence_loop_ids arg. Adding one
//     would be a framework change ADR-041 avoids.
//   - The chain.research_artifact.loop triple is the canonical chain-
//     entity audit landed by the existing research-reviewer flow.
//     Reading it from the chain entity gives the gate a structural
//     truth, not a self-report.
//   - Goodhart resistance: the reviewer-spec cannot fake the chain
//     triple from inside its loop. The triple is stamped by a
//     different code path (the research-artifact-loop stamper) on
//     research-reviewer approval. Self-attestation would be a weaker
//     guarantee.
type SpecModeGate struct {
	publisher TriplePublisher
	resolver  *chain.Resolver
	entities  chain.EntityTripleReader
	platform  types.PlatformMeta
	logger    *slog.Logger
}

// NewSpecModeGate constructs a SpecModeGate.
func NewSpecModeGate(
	pub TriplePublisher,
	resolver *chain.Resolver,
	entities chain.EntityTripleReader,
	platform types.PlatformMeta,
	logger *slog.Logger,
) *SpecModeGate {
	if logger == nil {
		logger = slog.Default()
	}
	return &SpecModeGate{
		publisher: pub,
		resolver:  resolver,
		entities:  entities,
		platform:  platform,
		logger:    logger,
	}
}

// HandleLoopCompleted is the chain.CompletionHandler entry point.
func (g *SpecModeGate) HandleLoopCompleted(ctx context.Context, ev *agentic.LoopCompletedEvent) error {
	if ev == nil {
		return nil
	}
	if ev.Role != reviewerSpecRole || ev.Outcome != agentic.OutcomeSuccess {
		return nil
	}
	if ev.LoopID == "" {
		return fmt.Errorf("phasevalidator.SpecModeGate: empty LoopID on agent.complete event")
	}

	loopEntityID := agentic.LoopExecutionEntityID(g.platform.Org, g.platform.Platform, ev.LoopID)
	loopTriples, err := g.entities.ReadEntity(ctx, loopEntityID)
	if err != nil {
		g.logger.Warn("spec-mode gate: read loop entity failed; skipping",
			slog.String("loop_id", ev.LoopID),
			slog.String("error", err.Error()))
		return nil
	}
	action, _ := loopTriples[agvocab.CoordinatorNextAction].(string)
	if action != approvedAction {
		// non-approved terminals (insufficient, needs_clarification, ...)
		// route via other rules; this gate is the approve-path filter.
		return nil
	}

	chainEntityID, err := g.resolver.ChainEntityID(ctx, ev.LoopID)
	if err != nil {
		g.logger.Warn("spec-mode gate: chain ancestry walk failed; skipping",
			slog.String("loop_id", ev.LoopID),
			slog.String("error", err.Error()))
		return nil
	}
	chainTriples, err := g.entities.ReadEntity(ctx, chainEntityID)
	if err != nil {
		g.logger.Warn("spec-mode gate: read chain entity failed; skipping",
			slog.String("loop_id", ev.LoopID),
			slog.String("chain_entity", chainEntityID),
			slog.String("error", err.Error()))
		return nil
	}

	researchLoop, _ := chainTriples[chain.PredicateResearchArtifactLoop].(string)
	now := time.Now().UTC()
	stamp := newStamper(triplesSourceSpecMode, now)

	if researchLoop == "" {
		writes := []message.Triple{
			stamp(chainEntityID, chain.PredicateSpecModeGateRejected, "true"),
			stamp(chainEntityID, chain.PredicateSpecModeGateRejectReason, specModeRejectMissingResearch),
		}
		g.publishAll(ctx, writes)
		g.logger.Info("spec-mode gate: rejected (no research artifact)",
			slog.String("loop_id", ev.LoopID),
			slog.String("chain_entity", chainEntityID))
		return nil
	}

	writes := []message.Triple{
		stamp(loopEntityID, chain.PredicateSpecModeGateProceed, "true"),
	}
	g.publishAll(ctx, writes)
	g.logger.Info("spec-mode gate: approved",
		slog.String("loop_id", ev.LoopID),
		slog.String("research_artifact_loop", researchLoop))
	return nil
}

// publishAll writes every triple in order; per-triple failures are
// logged but do not abort the loop. Mirrors PhaseValidator.publishAll.
func (g *SpecModeGate) publishAll(ctx context.Context, triples []message.Triple) {
	for _, t := range triples {
		if err := g.publisher.AddTriple(ctx, t); err != nil {
			g.logger.Warn("spec-mode gate: triple write failed",
				slog.String("predicate", t.Predicate),
				slog.String("subject", t.Subject),
				slog.String("error", err.Error()))
		}
	}
}
