package phasevalidator

import (
	"context"
	"fmt"
	"log/slog"
	"strconv"
	"strings"
	"time"

	"github.com/c360studio/semstreams/agentic"
	"github.com/c360studio/semstreams/message"
	"github.com/c360studio/semstreams/types"
	agvocab "github.com/c360studio/semstreams/vocabulary/agentic"

	"github.com/c360studio/semteams/cmd/semteams/chain"
)

const (
	// researcherRolePrefix matches the ADR-041 phase-as-sub-role
	// naming convention. The PhaseValidator fires on any role with
	// this prefix and parses the phase from the suffix.
	researcherRolePrefix = "researcher-"

	// PhasePlan is the "plan" researcher phase. Token shared between
	// the role suffix (researcher-plan) and the decide action value.
	PhasePlan = "plan"
	// PhaseGather is the "gather" researcher phase.
	PhaseGather = "gather"
	// PhaseSynthesize is the "synthesize" researcher phase.
	PhaseSynthesize = "synthesize"
	// PhaseArchitect is the "architect" researcher phase.
	PhaseArchitect = "architect"

	// actionEmit terminates the researcher arc. Approved structurally
	// (the existing rule 01a handles researcher→reviewer); no proceed
	// sentinel is written, but the target stamp lands for audit.
	actionEmit = "emit"

	// rejectReasonInvalidEdge / rejectReasonPhaseCap are the closed
	// classification tokens stamped on chain.phase_transition.reject_reason.
	rejectReasonInvalidEdge = "invalid_edge"
	rejectReasonPhaseCap    = "phase_cap"

	// triplesSourcePhase tags every triple this handler writes.
	triplesSourcePhase = "chain.phase_transition"
)

// allowedEdges encodes the ADR-041 §"Allowed transitions" set. Key is
// (input_phase, target_phase). Membership = allowed; absence = rejected.
// The map is exported via AllowedEdge() so contract tests can iterate
// without duplicating the table.
var allowedEdges = map[edge]struct{}{
	{PhasePlan, PhaseGather}:          {},
	{PhasePlan, actionEmit}:           {}, // premature emit; reviewer rejects
	{PhaseGather, PhaseSynthesize}:    {},
	{PhaseSynthesize, PhaseGather}:    {}, // back-edge re-gather
	{PhaseSynthesize, PhaseArchitect}: {},
	{PhaseSynthesize, actionEmit}:     {}, // pure-research arc terminal (addendum 2026-05-12)
	{PhaseArchitect, PhaseGather}:     {}, // back-edge re-gather
	{PhaseArchitect, actionEmit}:      {},
}

// phaseCaps holds the per-phase cap per ADR-041 §"Per-phase cap".
// Each value is the maximum number of times that phase may fire in a
// single chain. action=emit is not capped (researcher arc terminates
// on emit; the existing rule 01a + ADR-039 chain.recovery cap on the
// reviewer side bound retries).
var phaseCaps = map[string]int{
	PhasePlan:       1,
	PhaseGather:     3,
	PhaseSynthesize: 2,
	PhaseArchitect:  2,
}

// phaseCountPredicate maps a phase token to its chain-entity counter
// predicate. Sourced from cmd/semteams/chain/predicates.go so renames
// remain compiler-checked.
var phaseCountPredicate = map[string]string{
	PhasePlan:       chain.PredicatePhaseCountPlan,
	PhaseGather:     chain.PredicatePhaseCountGather,
	PhaseSynthesize: chain.PredicatePhaseCountSynthesize,
	PhaseArchitect:  chain.PredicatePhaseCountArchitect,
}

// proceedPredicate maps a target phase token to its loop-entity proceed
// sentinel predicate. action=emit has no proceed entry — emit is
// approved without a sentinel write (see HandleLoopCompleted).
var proceedPredicate = map[string]string{
	PhaseGather:     chain.PredicatePhaseTransitionProceedGather,
	PhaseSynthesize: chain.PredicatePhaseTransitionProceedSynthesize,
	PhaseArchitect:  chain.PredicatePhaseTransitionProceedArchitect,
}

type edge struct{ from, to string }

// AllowedEdge reports whether the (input_phase, target_phase) edge is
// in the allowed-set. Exported for contract tests; the runtime uses
// the unexported map directly.
func AllowedEdge(from, to string) bool {
	_, ok := allowedEdges[edge{from, to}]
	return ok
}

// PhaseCap reports the per-chain cap for the named phase. Returns 0
// when the phase token is unknown (defensive; emit is not capped here).
func PhaseCap(phase string) int {
	return phaseCaps[phase]
}

// TriplePublisher is the narrow write surface the handler needs.
// agentictools.NATSTriplePublisher satisfies it structurally.
type TriplePublisher interface {
	AddTriple(ctx context.Context, triple message.Triple) error
}

// PhaseValidator implements chain.CompletionHandler and gates
// researcher phase transitions per ADR-041.
//
// Filter contract: fires on roles prefixed researcher- (the ADR-041
// phase-as-sub-role naming) with outcome=success. Returns nil for
// every other event so sibling handlers see the event.
//
// See doc.go for the design rationale + fail-safe semantics.
type PhaseValidator struct {
	publisher TriplePublisher
	resolver  *chain.Resolver
	entities  chain.EntityTripleReader
	platform  types.PlatformMeta
	logger    *slog.Logger
}

// NewPhaseValidator constructs a PhaseValidator.
func NewPhaseValidator(
	pub TriplePublisher,
	resolver *chain.Resolver,
	entities chain.EntityTripleReader,
	platform types.PlatformMeta,
	logger *slog.Logger,
) *PhaseValidator {
	if logger == nil {
		logger = slog.Default()
	}
	return &PhaseValidator{
		publisher: pub,
		resolver:  resolver,
		entities:  entities,
		platform:  platform,
		logger:    logger,
	}
}

// HandleLoopCompleted is the chain.CompletionHandler entry point.
func (v *PhaseValidator) HandleLoopCompleted(ctx context.Context, ev *agentic.LoopCompletedEvent) error {
	if ev == nil {
		return nil
	}
	if ev.Outcome != agentic.OutcomeSuccess {
		return nil
	}
	inputPhase, ok := parseResearcherPhase(ev.Role)
	if !ok {
		return nil
	}
	if ev.LoopID == "" {
		// Defensive: LoopCompletedEvent.Validate enforces non-empty
		// LoopID at publish time, but the subscriber path reaches us
		// via JSON unmarshal which doesn't re-run Validate. Empty
		// LoopID would propagate into LoopExecutionEntityID's panic.
		return fmt.Errorf("phasevalidator.PhaseValidator: empty LoopID on agent.complete event")
	}

	loopEntityID := agentic.LoopExecutionEntityID(v.platform.Org, v.platform.Platform, ev.LoopID)
	loopTriples, err := v.entities.ReadEntity(ctx, loopEntityID)
	if err != nil {
		v.logger.Warn("phase validator: read loop entity failed; skipping",
			slog.String("loop_id", ev.LoopID),
			slog.String("error", err.Error()))
		return nil
	}
	action, _ := loopTriples[agvocab.CoordinatorNextAction].(string)
	if action == "" {
		// No decide terminal triple → not a structural transition signal.
		// Either the loop didn't call decide (framework / persona drift)
		// or the upstream triple flush is mid-flight. Skip — fail-safe.
		return nil
	}

	chainEntityID, err := v.resolver.ChainEntityID(ctx, ev.LoopID)
	if err != nil {
		v.logger.Warn("phase validator: chain ancestry walk failed; skipping",
			slog.String("loop_id", ev.LoopID),
			slog.String("error", err.Error()))
		return nil
	}
	chainTriples, err := v.entities.ReadEntity(ctx, chainEntityID)
	if err != nil {
		v.logger.Warn("phase validator: read chain entity failed; skipping",
			slog.String("loop_id", ev.LoopID),
			slog.String("chain_entity", chainEntityID),
			slog.String("error", err.Error()))
		return nil
	}

	now := time.Now().UTC()
	stamp := newStamper(triplesSourcePhase, now)

	// chain.phase_transition.target lands first regardless of approve/
	// reject outcome so operators see "what was attempted." Per-triple
	// failures inside the write loop don't roll back the assessment;
	// the per-write log surfaces partial-cluster cases.
	writes := []message.Triple{
		stamp(chainEntityID, chain.PredicatePhaseTransitionTarget, action),
	}

	if !AllowedEdge(inputPhase, action) {
		writes = append(writes,
			stamp(chainEntityID, chain.PredicatePhaseTransitionRejected, "true"),
			stamp(chainEntityID, chain.PredicatePhaseTransitionRejectReason, rejectReasonInvalidEdge),
		)
		v.publishAll(ctx, writes)
		v.logger.Info("phase validator: rejected (invalid edge)",
			slog.String("loop_id", ev.LoopID),
			slog.String("chain_entity", chainEntityID),
			slog.String("input_phase", inputPhase),
			slog.String("target", action))
		return nil
	}

	// action=emit is approved structurally without a proceed sentinel
	// and without a counter increment. The existing rule 01a handles
	// researcher→reviewer; the reviewer's own gate (chain.recovery.*)
	// bounds retries downstream. Stamping target=emit is sufficient
	// audit.
	if action == actionEmit {
		v.publishAll(ctx, writes)
		v.logger.Info("phase validator: emit approved (terminal)",
			slog.String("loop_id", ev.LoopID),
			slog.String("input_phase", inputPhase))
		return nil
	}

	// Phase cap check on the TARGET phase. priorCount is the chain's
	// running count for that target; newCount is the count after this
	// fire would land (semantically: spawning the target with this
	// transition increments the target's counter, so the cap test asks
	// "would the new count exceed the cap?").
	priorCount := readCount(chainTriples[phaseCountPredicate[action]])
	newCount := priorCount + 1
	capacity := PhaseCap(action)
	if capacity > 0 && newCount > capacity {
		writes = append(writes,
			stamp(chainEntityID, chain.PredicatePhaseTransitionRejected, "true"),
			stamp(chainEntityID, chain.PredicatePhaseTransitionRejectReason, rejectReasonPhaseCap),
		)
		v.publishAll(ctx, writes)
		v.logger.Info("phase validator: rejected (phase cap)",
			slog.String("loop_id", ev.LoopID),
			slog.String("chain_entity", chainEntityID),
			slog.String("input_phase", inputPhase),
			slog.String("target", action),
			slog.Int("prior_count", priorCount),
			slog.Int("cap", capacity))
		return nil
	}

	// Approved. Three writes: target audit (already in writes), counter
	// audit on chain entity, proceed sentinel on loop entity. Counter
	// lands BEFORE proceed — operators inspecting an "approved" chain
	// see the bump even when the proceed write fails (fail-soft on
	// loop-entity write blocks the rule fire and stalls fail-safe).
	writes = append(writes,
		stamp(chainEntityID, phaseCountPredicate[action], strconv.Itoa(newCount)),
		stamp(loopEntityID, proceedPredicate[action], "true"),
	)
	v.publishAll(ctx, writes)
	v.logger.Info("phase validator: approved",
		slog.String("loop_id", ev.LoopID),
		slog.String("chain_entity", chainEntityID),
		slog.String("input_phase", inputPhase),
		slog.String("target", action),
		slog.Int("new_count", newCount),
		slog.Int("cap", capacity))
	return nil
}

// parseResearcherPhase pulls the phase suffix from a researcher-<phase>
// role. Returns ("", false) for non-researcher roles, the plain
// "researcher" role (no suffix means no phase metadata), and roles
// where the suffix is empty. The caller treats !ok as a non-target
// event and returns nil from the handler.
func parseResearcherPhase(role string) (string, bool) {
	if !strings.HasPrefix(role, researcherRolePrefix) {
		return "", false
	}
	suffix := strings.TrimPrefix(role, researcherRolePrefix)
	if suffix == "" {
		return "", false
	}
	return suffix, true
}

// newStamper returns a closure that produces message.Triple values with
// a shared Source + Timestamp + Confidence. Saves the per-write
// boilerplate at each call site without coupling the handler to a
// struct-builder.
func newStamper(source string, now time.Time) func(subject, predicate string, object any) message.Triple {
	return func(subject, predicate string, object any) message.Triple {
		return message.Triple{
			Subject:    subject,
			Predicate:  predicate,
			Object:     object,
			Source:     source,
			Timestamp:  now,
			Confidence: 1.0,
		}
	}
}

// publishAll writes every triple in order; per-triple failures are
// logged but do not abort the loop. Mirrors recoverycounter's "partial
// cluster is queryable; no triples is invisible" stance.
func (v *PhaseValidator) publishAll(ctx context.Context, triples []message.Triple) {
	for _, t := range triples {
		if err := v.publisher.AddTriple(ctx, t); err != nil {
			v.logger.Warn("phase validator: triple write failed",
				slog.String("predicate", t.Predicate),
				slog.String("subject", t.Subject),
				slog.String("error", err.Error()))
		}
	}
}

// readCount parses a chain.researcher.phase_count.<phase> triple object.
// Mirrors recoverycounter.readCount — wire shape is a string-formatted
// integer ("0", "1", …) but defensive coercion accepts float64 / int /
// int64 so test-injected values don't need stringification.
func readCount(obj any) int {
	switch v := obj.(type) {
	case string:
		n, err := strconv.Atoi(v)
		if err != nil {
			return 0
		}
		return n
	case float64:
		return int(v)
	case int:
		return v
	case int64:
		return int(v)
	default:
		return 0
	}
}
