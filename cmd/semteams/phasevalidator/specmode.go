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

	// agentLoopParentPredicate is the upstream-stamped triple naming
	// the spawning loop's parent execution entity. Mirrors the constant
	// in cmd/semteams/chain/plan.go — local copy to avoid an import
	// cycle and to make the SpecModeGate self-contained.
	agentLoopParentPredicate = "agent.loop.parent"

	// predicateDevViaSpecArtifactPath / Slug mirror the constants
	// produced by cmd/semteams/tools/emitspecartifact/executor.go
	// (predicatePath / predicateSlug there). When researcher-architect
	// emits the spec artifact these triples land on the architect's
	// loop entity. The spec-mode gate copies them onto the reviewer-spec
	// loop entity so rule 02-reviewer-approved-to-builder.json can
	// substitute `$entity.triple.dev_via_spec.artifact.{path,slug}` at
	// the builder spawn site (the substitution engine resolves against
	// the rule's triggering entity, which is the reviewer-spec loop,
	// not the architect that originally stamped the triples). Renames
	// on either side break rule 02's substitution before a real-LLM
	// smoke catches it.
	predicateDevViaSpecArtifactPath = "dev_via_spec.artifact.path"
	predicateDevViaSpecArtifactSlug = "dev_via_spec.artifact.slug"
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

	// Forward-propagate dev_via_spec.artifact.{path,slug} from the
	// architect researcher (the reviewer-spec's parent loop) onto the
	// reviewer-spec loop entity. Rule 02-reviewer-approved-to-builder.json
	// substitutes `$entity.triple.dev_via_spec.artifact.path` against the
	// reviewer-spec entity (the rule's triggering entity); without this
	// hop the triples remain only on the architect's entity and the
	// substitution returns the literal token at builder spawn. Per-write
	// failure is logged but does not block the proceed sentinel — partial
	// propagation leaves the chain stalled at builder bootstrap, which is
	// the conservative fail-safe (same posture as PhaseValidator's
	// partial-cluster behaviour). Failures here are not fatal to the gate
	// decision itself.
	writes := g.forwardSpecArtifactTriples(ctx, ev.LoopID, loopEntityID, loopTriples, stamp)

	writes = append(writes, stamp(loopEntityID, chain.PredicateSpecModeGateProceed, "true"))
	g.publishAll(ctx, writes)
	g.logger.Info("spec-mode gate: approved",
		slog.String("loop_id", ev.LoopID),
		slog.String("research_artifact_loop", researchLoop))
	return nil
}

// forwardSpecArtifactTriples walks one hop from the reviewer-spec loop's
// agent.loop.parent triple to the architect researcher loop entity and
// returns triple writes copying the architect's
// dev_via_spec.artifact.{path,slug} onto the reviewer-spec entity. Empty
// slice returned when the architect entity cannot be located, doesn't
// carry the artifact triples, or any read fails — caller's proceed
// sentinel still lands and the chain stalls at builder bootstrap (the
// substitution will be visible as an unresolved literal in logs).
//
// Architectural note: the chain-walk lives in the gate (not in the
// emitting tool) because the canonical wire shape is "artifact triples
// on the emitter's entity"; downstream consumers that need different
// entities to carry the triples take responsibility for replication.
// Each new consumer is a new walk, which surfaces the design choice
// rather than burying it inside the emitter.
func (g *SpecModeGate) forwardSpecArtifactTriples(ctx context.Context, reviewerLoopID, reviewerLoopEntityID string, reviewerTriples map[string]any, stamp func(string, string, any) message.Triple) []message.Triple {
	parentEntityID, _ := reviewerTriples[agentLoopParentPredicate].(string)
	if parentEntityID == "" {
		g.logger.Warn("spec-mode gate: reviewer entity missing agent.loop.parent; skipping spec artifact forwarding",
			slog.String("reviewer_loop_id", reviewerLoopID))
		return nil
	}
	architectLoopID, ok := agentic.LoopIDFromExecutionEntityID(parentEntityID)
	if !ok {
		g.logger.Warn("spec-mode gate: agent.loop.parent malformed; skipping spec artifact forwarding",
			slog.String("reviewer_loop_id", reviewerLoopID),
			slog.String("parent_entity_id", parentEntityID))
		return nil
	}
	architectEntityID := agentic.LoopExecutionEntityID(g.platform.Org, g.platform.Platform, architectLoopID)
	architectTriples, err := g.entities.ReadEntity(ctx, architectEntityID)
	if err != nil {
		g.logger.Warn("spec-mode gate: read architect entity failed; skipping spec artifact forwarding",
			slog.String("reviewer_loop_id", reviewerLoopID),
			slog.String("architect_loop_id", architectLoopID),
			slog.String("error", err.Error()))
		return nil
	}

	path, _ := architectTriples[predicateDevViaSpecArtifactPath].(string)
	slug, _ := architectTriples[predicateDevViaSpecArtifactSlug].(string)
	if path == "" && slug == "" {
		// Architect didn't emit the spec artifact — the reviewer approved
		// without one upstream. Goodhart concern, but the existing
		// chain.research_artifact.loop check already gates against that.
		// Defensive log; no writes.
		g.logger.Warn("spec-mode gate: architect missing dev_via_spec.artifact triples; substitution will fail at builder spawn",
			slog.String("reviewer_loop_id", reviewerLoopID),
			slog.String("architect_loop_id", architectLoopID))
		return nil
	}

	writes := make([]message.Triple, 0, 2)
	if path != "" {
		writes = append(writes, stamp(reviewerLoopEntityID, predicateDevViaSpecArtifactPath, path))
	}
	if slug != "" {
		writes = append(writes, stamp(reviewerLoopEntityID, predicateDevViaSpecArtifactSlug, slug))
	}
	g.logger.Info("spec-mode gate: forwarded dev_via_spec.artifact triples",
		slog.String("reviewer_loop_id", reviewerLoopID),
		slog.String("architect_loop_id", architectLoopID),
		slog.String("path", path),
		slog.String("slug", slug))
	return writes
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
