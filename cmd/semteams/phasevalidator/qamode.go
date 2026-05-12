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
	// builderRole is the role the qa-mode gate filters on. Matches the
	// ADR-041 collapsed builder role name (was dev-via-spec-builder).
	builderRole = "builder"

	// testsPassingAction is the coordinator.next_action value the
	// builder_decide tool stamps when the builder claims green
	// terminal. The qa-mode gate engages only on this action — other
	// terminals (tests_failing, needs_clarification) route via the
	// existing dev-via-spec rules and chain.recovery / chain.needs_review
	// paths.
	testsPassingAction = "tests_passing"

	// predicateBuilderTestsRun / TestsFailed mirror the builder_decide
	// executor's predicate strings. Local copies (not chain.Predicate*)
	// because these are produced by the dev_via_spec.builder.*
	// vocabulary that builder_decide owns, not the chain.* vocabulary
	// chain owns. Renames on either side must keep both in sync — the
	// builder_decide_executor_test exercises the wire shape so drift
	// shows up there first.
	predicateBuilderTestsRun    = "dev_via_spec.builder.tests_run"
	predicateBuilderTestsFailed = "dev_via_spec.builder.tests_failed"

	// qaModeRejectZeroTests / NonzeroFailed are the two classification
	// tokens the qa-mode gate emits. Distinct so ops-agent can surface
	// "no tests run" stalls separately from "tests ran but some failed"
	// stalls — different remediation surfaces.
	qaModeRejectZeroTests     = "zero_tests_run"
	qaModeRejectNonzeroFailed = "tests_failed_nonzero"

	triplesSourceQAMode = "chain.qa_mode_gate"
)

// QAModeGate implements chain.CompletionHandler and gates
// builder→reviewer-qa transitions per ADR-041 §"Risks mitigation".
// Two concrete checks against the builder's own decide triples:
//
//  1. dev_via_spec.builder.tests_run > 0 — builder cannot ship green
//     without having run any tests.
//  2. dev_via_spec.builder.tests_failed == 0 — builder cannot ship
//     green if any test failed; "tests_passing with failures" is a
//     structural contradiction the gate catches even if the LLM
//     persona missed it.
//
// Why a separate gate when builder_decide already validates inputs:
// builder_decide enforces that tests_run > 0 for action=tests_passing
// (see cmd/semteams/tools/builderdecide/executor.go line 233). But
// builder_decide does NOT enforce tests_failed == 0 — the builder may
// claim tests_passing with tests_failed > 0 and the tool accepts it.
// The qa-mode gate is the chain-level catch for that structural
// contradiction, plus a redundancy layer for the tests_run > 0
// guarantee in case builder_decide is bypassed (future tool changes,
// alternate emit paths, e2e fixture drift).
type QAModeGate struct {
	publisher TriplePublisher
	resolver  *chain.Resolver
	entities  chain.EntityTripleReader
	platform  types.PlatformMeta
	logger    *slog.Logger
}

// NewQAModeGate constructs a QAModeGate.
func NewQAModeGate(
	pub TriplePublisher,
	resolver *chain.Resolver,
	entities chain.EntityTripleReader,
	platform types.PlatformMeta,
	logger *slog.Logger,
) *QAModeGate {
	if logger == nil {
		logger = slog.Default()
	}
	return &QAModeGate{
		publisher: pub,
		resolver:  resolver,
		entities:  entities,
		platform:  platform,
		logger:    logger,
	}
}

// HandleLoopCompleted is the chain.CompletionHandler entry point.
func (g *QAModeGate) HandleLoopCompleted(ctx context.Context, ev *agentic.LoopCompletedEvent) error {
	if ev == nil {
		return nil
	}
	if ev.Role != builderRole || ev.Outcome != agentic.OutcomeSuccess {
		return nil
	}
	if ev.LoopID == "" {
		return fmt.Errorf("phasevalidator.QAModeGate: empty LoopID on agent.complete event")
	}

	loopEntityID := agentic.LoopExecutionEntityID(g.platform.Org, g.platform.Platform, ev.LoopID)
	loopTriples, err := g.entities.ReadEntity(ctx, loopEntityID)
	if err != nil {
		g.logger.Warn("qa-mode gate: read loop entity failed; skipping",
			slog.String("loop_id", ev.LoopID),
			slog.String("error", err.Error()))
		return nil
	}
	action, _ := loopTriples[agvocab.CoordinatorNextAction].(string)
	if action != testsPassingAction {
		// tests_failing / needs_clarification route via the existing
		// dev-via-spec rules; this gate is the green-path filter.
		return nil
	}

	testsRun := readCount(loopTriples[predicateBuilderTestsRun])
	testsFailed := readCount(loopTriples[predicateBuilderTestsFailed])

	now := time.Now().UTC()
	stamp := newStamper(triplesSourceQAMode, now)

	// Resolve the chain entity ID lazily — only when we need to write
	// rejection triples. The happy path (approval) writes only the
	// loop-entity proceed sentinel and skips the chain-walk.
	rejectOn := func(reason string) {
		chainEntityID, walkErr := g.resolver.ChainEntityID(ctx, ev.LoopID)
		if walkErr != nil {
			g.logger.Warn("qa-mode gate: chain ancestry walk failed during rejection; rejection marker not stamped",
				slog.String("loop_id", ev.LoopID),
				slog.String("reason", reason),
				slog.String("error", walkErr.Error()))
			return
		}
		writes := []message.Triple{
			stamp(chainEntityID, chain.PredicateQAModeGateRejected, "true"),
			stamp(chainEntityID, chain.PredicateQAModeGateRejectReason, reason),
		}
		g.publishAll(ctx, writes)
		g.logger.Info("qa-mode gate: rejected",
			slog.String("loop_id", ev.LoopID),
			slog.String("chain_entity", chainEntityID),
			slog.String("reason", reason),
			slog.Int("tests_run", testsRun),
			slog.Int("tests_failed", testsFailed))
	}

	if testsRun == 0 {
		rejectOn(qaModeRejectZeroTests)
		return nil
	}
	if testsFailed > 0 {
		rejectOn(qaModeRejectNonzeroFailed)
		return nil
	}

	writes := []message.Triple{
		stamp(loopEntityID, chain.PredicateQAModeGateProceed, "true"),
	}
	g.publishAll(ctx, writes)
	g.logger.Info("qa-mode gate: approved",
		slog.String("loop_id", ev.LoopID),
		slog.Int("tests_run", testsRun),
		slog.Int("tests_failed", testsFailed))
	return nil
}

// publishAll writes every triple in order; per-triple failures are
// logged but do not abort the loop. Mirrors PhaseValidator.publishAll.
func (g *QAModeGate) publishAll(ctx context.Context, triples []message.Triple) {
	for _, t := range triples {
		if err := g.publisher.AddTriple(ctx, t); err != nil {
			g.logger.Warn("qa-mode gate: triple write failed",
				slog.String("predicate", t.Predicate),
				slog.String("subject", t.Subject),
				slog.String("error", err.Error()))
		}
	}
}
