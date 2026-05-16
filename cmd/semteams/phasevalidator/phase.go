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
	"github.com/c360studio/semteams/cmd/semteams/chainmode"
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

	// rejectReasonInvalidEdge / rejectReasonPhaseCap /
	// rejectReasonModeMismatch are the closed classification tokens
	// stamped on chain.phase_transition.reject_reason.
	rejectReasonInvalidEdge  = "invalid_edge"
	rejectReasonPhaseCap     = "phase_cap"
	rejectReasonModeMismatch = "mode_mismatch"

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

	tctx, ok := v.loadTransitionContext(ctx, ev)
	if !ok {
		return nil
	}

	now := time.Now().UTC()
	stamp := newStamper(triplesSourcePhase, now)

	// chain.phase_transition.target lands first regardless of approve/
	// reject outcome so operators see "what was attempted." Per-triple
	// failures inside the write loop don't roll back the assessment;
	// the per-write log surfaces partial-cluster cases.
	writes := []message.Triple{
		stamp(tctx.chainEntityID, chain.PredicatePhaseTransitionTarget, tctx.action),
	}

	if !AllowedEdge(inputPhase, tctx.action) {
		writes = append(writes,
			stamp(tctx.chainEntityID, chain.PredicatePhaseTransitionRejected, "true"),
			stamp(tctx.chainEntityID, chain.PredicatePhaseTransitionRejectReason, rejectReasonInvalidEdge),
		)
		v.publishAll(ctx, writes)
		v.logger.Info("phase validator: rejected (invalid edge)",
			slog.String("loop_id", ev.LoopID),
			slog.String("chain_entity", tctx.chainEntityID),
			slog.String("input_phase", inputPhase),
			slog.String("target", tctx.action))
		return nil
	}

	// chain.mode gate (coordinator-redesign Slice 1b). Refuses the
	// architect proceed sentinel for research_only chains; passes
	// every other (input, target) pair through. See modeGateRejects
	// for the full rationale.
	if v.modeGateRejects(ctx, ev, inputPhase, tctx, stamp, writes) {
		return nil
	}

	// action=emit is approved structurally without a proceed sentinel
	// and without a counter increment. The existing rule 01a handles
	// researcher→reviewer; the reviewer's own gate (chain.recovery.*)
	// bounds retries downstream. Stamping target=emit is sufficient
	// audit.
	if tctx.action == actionEmit {
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
	priorCount := readCount(tctx.chainTriples[phaseCountPredicate[tctx.action]])
	newCount := priorCount + 1
	capacity := PhaseCap(tctx.action)
	if capacity > 0 && newCount > capacity {
		writes = append(writes,
			stamp(tctx.chainEntityID, chain.PredicatePhaseTransitionRejected, "true"),
			stamp(tctx.chainEntityID, chain.PredicatePhaseTransitionRejectReason, rejectReasonPhaseCap),
		)
		v.publishAll(ctx, writes)
		v.logger.Info("phase validator: rejected (phase cap)",
			slog.String("loop_id", ev.LoopID),
			slog.String("chain_entity", tctx.chainEntityID),
			slog.String("input_phase", inputPhase),
			slog.String("target", tctx.action),
			slog.Int("prior_count", priorCount),
			slog.Int("cap", capacity))
		return nil
	}

	// Approved. Counter audit on chain entity, optional mode mirror on
	// loop entity (synthesize only), then proceed sentinel on loop
	// entity LAST. Counter lands first — operators inspecting an
	// "approved" chain see the bump even when downstream writes fail.
	//
	// Write order on the loop entity is load-bearing: mirror BEFORE
	// proceed. The rule engine's entity-state-watch fires on the
	// proceed sentinel; if proceed landed first and the mirror write
	// failed (publishAll log-and-continue semantics), the loop would
	// carry proceed=true with no mode triple, neither 05a nor 05b
	// would match, and the chain would wedge silently with no
	// chainstall signal. Mirror-first makes every observable
	// intermediate state safe — the rule's mode condition is satisfied
	// by the time the proceed condition can be.
	writes = append(writes,
		stamp(tctx.chainEntityID, phaseCountPredicate[tctx.action], strconv.Itoa(newCount)),
	)
	// Mirror chain.mode onto the source-loop entity when approving a
	// synthesize transition (ADR-041 addendum 2026-05-16). Rules 05a /
	// 05b condition on this mirror to fork the spawned synthesize
	// loop's action_allowlist by mode — research_only excludes
	// architect; dev_via_spec keeps it. Rule conditions only read the
	// firing entity's own triples (rule 06 metadata § "chain_mode_gate"
	// has a parallel discussion for the architect case), so the
	// mirror is the cross-entity bridge.
	//
	// Absence policy: a chain dispatched without a coordinator
	// front-door carries no chain.mode triple. Default to dev_via_spec
	// to preserve pre-Slice-1b behaviour — architect was always in the
	// synthesize allowlist before the addendum, and legacy configs
	// without a coordinator front-door run the dev arc by convention.
	// An explicit-but-unknown token (operator typo, future routing
	// class) collapses to the same default; the warn-log surfaces the
	// coercion so operators can spot misconfiguration.
	if tctx.action == PhaseSynthesize {
		mode, _ := tctx.chainTriples[chain.PredicateChainMode].(string)
		if mode != chainmode.ModeResearchOnly && mode != chainmode.ModeDevViaSpec {
			if mode != "" {
				v.logger.Warn("phase validator: unknown chain.mode token; defaulting synthesize mirror to dev_via_spec",
					slog.String("loop_id", ev.LoopID),
					slog.String("chain_entity", tctx.chainEntityID),
					slog.String("observed_mode", mode))
			}
			mode = chainmode.ModeDevViaSpec
		}
		writes = append(writes,
			stamp(tctx.loopEntityID, chain.PredicateChainMode, mode),
		)
	}
	writes = append(writes,
		stamp(tctx.loopEntityID, proceedPredicate[tctx.action], "true"),
	)
	v.publishAll(ctx, writes)
	v.logger.Info("phase validator: approved",
		slog.String("loop_id", ev.LoopID),
		slog.String("chain_entity", tctx.chainEntityID),
		slog.String("input_phase", inputPhase),
		slog.String("target", tctx.action),
		slog.Int("new_count", newCount),
		slog.Int("cap", capacity))
	return nil
}

// transitionContext groups the entity-graph reads HandleLoopCompleted
// needs before evaluating a researcher transition. A struct beats a
// 6-value multi-return at the call site — operators tracing a stall
// can see all the inputs in one log dump without unpacking returns.
type transitionContext struct {
	loopEntityID  string
	action        string
	chainEntityID string
	loopTriples   map[string]any
	chainTriples  map[string]any
}

// loadTransitionContext does the entity-graph reads HandleLoopCompleted
// needs before evaluating the transition: loop entity id, the
// coordinator.next_action terminal, the loop entity's full triple
// map (so callers can read sibling predicates like
// coordinator.decision_reason for log context), the canonical chain
// entity id, and the chain entity's triple map. Returns ok=false
// (with no triples published) for every fail-safe case: graph read
// blip, missing action triple, resolver failure.
func (v *PhaseValidator) loadTransitionContext(ctx context.Context, ev *agentic.LoopCompletedEvent) (transitionContext, bool) {
	loopEntityID := agentic.LoopExecutionEntityID(v.platform.Org, v.platform.Platform, ev.LoopID)
	loopTriples, err := v.entities.ReadEntity(ctx, loopEntityID)
	if err != nil {
		v.logger.Warn("phase validator: read loop entity failed; skipping",
			slog.String("loop_id", ev.LoopID),
			slog.String("error", err.Error()))
		return transitionContext{}, false
	}
	action, _ := loopTriples[agvocab.CoordinatorNextAction].(string)
	if action == "" {
		// No decide terminal triple → not a structural transition signal.
		// Either the loop didn't call decide (framework / persona drift)
		// or the upstream triple flush is mid-flight. Skip — fail-safe.
		return transitionContext{}, false
	}
	chainEntityID, err := v.resolver.ChainEntityID(ctx, ev.LoopID)
	if err != nil {
		v.logger.Warn("phase validator: chain ancestry walk failed; skipping",
			slog.String("loop_id", ev.LoopID),
			slog.String("error", err.Error()))
		return transitionContext{}, false
	}
	chainTriples, err := v.entities.ReadEntity(ctx, chainEntityID)
	if err != nil {
		v.logger.Warn("phase validator: read chain entity failed; skipping",
			slog.String("loop_id", ev.LoopID),
			slog.String("chain_entity", chainEntityID),
			slog.String("error", err.Error()))
		return transitionContext{}, false
	}
	return transitionContext{
		loopEntityID:  loopEntityID,
		action:        action,
		chainEntityID: chainEntityID,
		loopTriples:   loopTriples,
		chainTriples:  chainTriples,
	}, true
}

// modeGateRejects implements the coordinator-redesign Slice 1b
// structural gate on the synthesize→architect transition. Returns
// true when the gate refused the transition (caller should return
// nil immediately — rejection triples have already been published).
// Returns false in every other case (gate doesn't apply, or chain.mode
// permits the transition).
//
// Why a research_only chain must not architect: a chain dispatched
// via coordinator decide(action=delegate_research) is classified at
// the user's intent boundary as "answer the user, no build artifact."
// The synthesize loop is free to choose decide(action=architect) by
// persona judgment (Slice 1 smoke evidence: 2 of 3 synthesize loops
// chose architect regardless of the spawn prompt's framing), but
// that choice is wrong for a research_only chain — it would route
// into the dev arc the coordinator already classified as out-of-scope.
// The validator refuses the proceed sentinel; the synthesize loop's
// only remaining legal moves are decide(action=emit) (close the
// research arc; reviewer-research evaluates) or decide(action=gather)
// (back-edge re-gather, bounded by the gather cap).
//
// Read-shape: chain.mode.classification is stamped on the chain
// entity by cmd/semteams/chainmode.Stamper at coordinator-terminal
// time. The coordinator's loop completes BEFORE any researcher loop
// in the chain (the dispatch path is "coordinator decide → rule
// fires → researcher-plan spawned"), so by the time synthesize
// completes the triple is on disk and chainTriples already contains
// it.
//
// Absence policy: pre-Slice-1b chains (legacy research-iterative
// configs, any chain dispatched without a coordinator front-door)
// carry no chain.mode triple. The gate is a no-op in that case —
// the synthesize→architect path passes through to the cap check
// exactly as it did before Slice 1b. This protects backward
// compatibility for every config that doesn't run a coordinator.
//
// priorWrites carries the target-audit triple already appended in
// the caller; this helper extends it with the rejection cluster
// before publishing. The synthesizer's coordinator.decision_reason
// (when present) lands in the log line so operators tracing a stall
// see what the synthesize loop intended without manually walking to
// the loop entity.
//
// Returns true ⇒ caller MUST return nil; rejection writes have been
// published. Returns false in every other case (gate doesn't apply,
// or chain.mode permits the transition).
func (v *PhaseValidator) modeGateRejects(
	ctx context.Context,
	ev *agentic.LoopCompletedEvent,
	inputPhase string,
	tctx transitionContext,
	stamp func(subject, predicate string, object any) message.Triple,
	priorWrites []message.Triple,
) bool {
	if inputPhase != PhaseSynthesize || tctx.action != PhaseArchitect {
		return false
	}
	mode, _ := tctx.chainTriples[chain.PredicateChainMode].(string)
	if mode != chainmode.ModeResearchOnly {
		return false
	}
	reason, _ := tctx.loopTriples[agvocab.CoordinatorDecisionReason].(string)
	out := append(priorWrites,
		stamp(tctx.chainEntityID, chain.PredicatePhaseTransitionRejected, "true"),
		stamp(tctx.chainEntityID, chain.PredicatePhaseTransitionRejectReason, rejectReasonModeMismatch),
	)
	v.publishAll(ctx, out)
	v.logger.Info("phase validator: rejected (mode mismatch)",
		slog.String("loop_id", ev.LoopID),
		slog.String("chain_entity", tctx.chainEntityID),
		slog.String("input_phase", inputPhase),
		slog.String("target", tctx.action),
		slog.String("mode", mode),
		slog.String("reason", reason))
	return true
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
