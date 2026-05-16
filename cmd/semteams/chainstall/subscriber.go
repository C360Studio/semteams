package chainstall

import (
	"context"
	"fmt"
	"log/slog"
	"strconv"
	"time"

	"github.com/c360studio/semstreams/agentic"
	"github.com/c360studio/semstreams/message"
	"github.com/c360studio/semstreams/types"
	agvocab "github.com/c360studio/semstreams/vocabulary/agentic"

	"github.com/c360studio/semteams/cmd/semteams/chain"
)

const (
	// triplesSource tags the Source field on every triple this subscriber
	// writes so operators inspecting the graph can attribute the write
	// without parsing predicate names. Matches the
	// chain.<predicate-stem> convention sibling stampers use.
	triplesSource = "chain.stall"

	// DefaultThreshold is the per-chain stall-recovery cap when none is
	// configured. Three matches ADR-040's recoverycounter.DefaultThreshold —
	// the recovery cap and the stall cap stay aligned so operators
	// don't have to reason about two different ceilings.
	DefaultThreshold = 3

	// AuthorityPerReason routes each stall by reasonToRoute's per-reason
	// default. Standard for osh-demo / e2e-coordinator / dev flows.
	AuthorityPerReason = "per_reason"

	// AuthorityAllHuman forces every stall to the human-approval path
	// regardless of per-reason default. Reserved for high-stakes
	// production flows where the operator wants veto on every recovery
	// cycle. Per-flow config field on agentic-dispatch's component
	// config; default falls back to per_reason.
	AuthorityAllHuman = "all_human"
)

// roleAcceptingStall is the closed set of producer roles chainstall
// inspects. A loop completion with a role outside this set is a no-op
// (no gate produces a rejection on that role's completion).
//
// This set is exposed as the cheap pre-filter before any chain walk:
// most agent.complete events are coordinator / researcher-plan /
// researcher-gather / dispatch loops that no gate rejects, so skipping
// them up front avoids the graph round-trips. Adding a new producer
// role with rejection coverage requires extending this set AND the
// detection logic in detectRejection.
var roleAcceptingStall = map[string]struct{}{
	"researcher-synthesize": {},
	// Reviewer-spec / builder / reviewer-research producers land in
	// follow-up pieces per Slice 1c precedent. The routing table covers
	// their reasons today; the detection logic lands when the case
	// surfaces in smoke evidence.
}

// TriplePublisher is the narrow write surface chainstall needs.
// agentictools.NATSTriplePublisher satisfies it structurally — same
// per-package interface convention every sibling chain-handler uses.
type TriplePublisher interface {
	AddTriple(ctx context.Context, triple message.Triple) error
}

// Subscriber implements chain.CompletionHandler and routes structural-gate
// rejections to either the coordinator wake-up path (framing-fixable
// reasons) or the human-approval path (budget exhaustion / structural
// impossibility / cap-hit escalation).
//
// Filter contract: only fires on producer roles in roleAcceptingStall
// with OutcomeSuccess and a detectable rejection. Every other event is
// a structural no-op so sibling CompletionHandlers (registered before
// chainstall in the handler slice) see the event.
//
// Write contract per cycle:
//
//   - chain.rejection.recovery_count → chain entity. Monotonic per-cycle
//     audit. Stored as string-formatted int for rule-engine comparisons.
//   - chain.rejection.exhausted → chain entity. Cap-hit marker stamped
//     when count > threshold. Always routes human regardless of
//     per-reason default; operator-visible escalation signal.
//   - chain.stall.routed_to → chain entity. "coordinator" | "human".
//   - chain.stall.reason → chain entity. Copy of the structural gate's
//     reject_reason token.
//   - chain.stall.producer_loop_id → chain entity. ev.LoopID.
//   - chain.stall.producer_role → chain entity. ev.Role.
//   - chain.stall.observed_at → chain entity. RFC3339 timestamp.
//   - chain.stall.awaiting_human → chain entity. "true" when routed
//     human. Cleared by the HTTP endpoint (deferred follow-up) on
//     operator verdict.
//   - chain.stall.proceed → producer loop entity. "true" when routed
//     coordinator. Rule 05 fires only when this predicate is "true" on
//     the producer loop. Same fail-safe semantics as ADR-040's
//     PredicateRecoveryProceed: chainstall failure to stamp leaves no
//     proceed → rule never fires → chain stalls until human inspection.
//
// Threshold semantics: count > threshold (not >=) means the threshold
// itself is reachable. With default 3, attempts 1+2+3 route per
// reasonToRoute; attempt 4 onwards routes human + stamps exhausted.
// Same convention as recoverycounter.
type Subscriber struct {
	publisher TriplePublisher
	resolver  *chain.Resolver
	entities  chain.EntityTripleReader
	platform  types.PlatformMeta
	threshold int
	authority string
	logger    *slog.Logger
}

// NewSubscriber constructs a Subscriber. threshold ≤ 0 falls back to
// DefaultThreshold so callers can pass 0 for "use the default." authority
// outside {AuthorityPerReason, AuthorityAllHuman} falls back to
// AuthorityPerReason (operator config typos fail-safe to the
// less-permissive default).
func NewSubscriber(
	pub TriplePublisher,
	resolver *chain.Resolver,
	entities chain.EntityTripleReader,
	platform types.PlatformMeta,
	threshold int,
	authority string,
	logger *slog.Logger,
) *Subscriber {
	if logger == nil {
		logger = slog.Default()
	}
	if threshold <= 0 {
		threshold = DefaultThreshold
	}
	if authority != AuthorityAllHuman {
		authority = AuthorityPerReason
	}
	return &Subscriber{
		publisher: pub,
		resolver:  resolver,
		entities:  entities,
		platform:  platform,
		threshold: threshold,
		authority: authority,
		logger:    logger,
	}
}

// HandleLoopCompleted is the chain.CompletionHandler entry point.
// Returns nil on no-op paths so sibling handlers see the event.
func (s *Subscriber) HandleLoopCompleted(ctx context.Context, ev *agentic.LoopCompletedEvent) error {
	if ev == nil {
		return nil
	}
	if ev.Outcome != agentic.OutcomeSuccess {
		return nil
	}
	if _, ok := roleAcceptingStall[ev.Role]; !ok {
		return nil
	}
	if err := chain.ValidateLoopID(ev.LoopID); err != nil {
		return fmt.Errorf("chainstall.Subscriber: %w", err)
	}

	producerEntityID := agentic.LoopExecutionEntityID(s.platform.Org, s.platform.Platform, ev.LoopID)
	producerTriples, err := s.entities.ReadEntity(ctx, producerEntityID)
	if err != nil {
		s.logger.Warn("chainstall: read producer entity failed; skipping",
			slog.String("producer_loop_id", ev.LoopID),
			slog.String("role", ev.Role),
			slog.String("error", err.Error()))
		return nil
	}

	chainEntityID, err := s.resolver.ChainEntityID(ctx, ev.LoopID)
	if err != nil {
		s.logger.Warn("chainstall: chain ancestry walk failed; skipping",
			slog.String("producer_loop_id", ev.LoopID),
			slog.String("role", ev.Role),
			slog.String("error", err.Error()))
		return nil
	}
	chainTriples, err := s.entities.ReadEntity(ctx, chainEntityID)
	if err != nil {
		s.logger.Warn("chainstall: read chain entity failed; skipping",
			slog.String("producer_loop_id", ev.LoopID),
			slog.String("chain_entity", chainEntityID),
			slog.String("error", err.Error()))
		return nil
	}

	// Slice 1c short-circuit: if the chain has already terminated, the
	// happy-path coordinator-return rules have already fired. Anything we
	// observe at this point is stale per-prior-rejection signal that
	// chainstall must not re-process — the operator's user reply is
	// already in flight.
	if terminal, _ := chainTriples[chain.PredicateChainTerminalReached].(string); terminal == "true" {
		return nil
	}

	// At-least-once redelivery guard. NATS JetStream may redeliver the
	// same agent.complete event on consumer ack-pending timeout or
	// reconnect. Without this guard a redelivered event would
	// re-increment chain.rejection.recovery_count, re-stamp the audit
	// cluster, and double-fire chain.stall.proceed → rule 05 would
	// publish a duplicate coordinator. Comparing the chain entity's
	// recorded producer_loop_id against ev.LoopID catches the
	// same-producer redelivery cleanly: legitimate retries spawn a NEW
	// producer loop with a new loop_id (coordinator re-dispatch creates
	// a fresh chain root → researcher-synthesize gets a new LoopID).
	if prior, _ := chainTriples[chain.PredicateChainStallProducerLoopID].(string); prior == ev.LoopID {
		s.logger.Debug("chainstall: skipping redelivery for already-processed producer",
			slog.String("producer_loop_id", ev.LoopID))
		return nil
	}

	reason, ok := s.detectRejection(ev, producerTriples, chainTriples)
	if !ok {
		return nil
	}

	route, routeOK := RouteForReason(reason)
	if !routeOK {
		// reasonToRoute is the routing-table source of truth. An unknown
		// reason means a producer landed a new token without updating
		// chainstall — the contract test catches that at build time, so
		// reaching this branch at runtime is a defensive log-and-skip
		// (no audit cluster, no rule fire).
		s.logger.Warn("chainstall: rejection reason not in routing table; skipping",
			slog.String("reason", reason),
			slog.String("producer_loop_id", ev.LoopID))
		return nil
	}

	priorCount := readCount(chainTriples[chain.PredicateChainRejectionRecoveryCount])
	newCount := priorCount + 1
	capHit := newCount > s.threshold
	if capHit || s.authority == AuthorityAllHuman {
		route = RouteHuman
	}

	now := time.Now().UTC()
	s.stampAuditCluster(ctx, chainEntityID, ev, reason, route, newCount, capHit, now)
	s.stampPerLoopSignals(ctx, producerEntityID, route, reason, now)

	s.logger.Info("chainstall: rejection routed",
		slog.String("producer_loop_id", ev.LoopID),
		slog.String("role", ev.Role),
		slog.String("chain_entity", chainEntityID),
		slog.String("reason", reason),
		slog.String("route", string(route)),
		slog.Int("recovery_count", newCount),
		slog.Bool("cap_hit", capHit))
	return nil
}

// detectRejection inspects the producer loop's terminal action together
// with cross-event chain-entity state and returns the reject reason that
// would have stalled the chain. Returns ("", false) when the loop's
// terminal is structurally clean (the gate's proceed sentinel was
// stamped) so no rejection has occurred.
//
// Slice 2 Piece 1 scope: mode_mismatch detection only. Detection for the
// other seven routing-table reasons (phase_cap, invalid_edge,
// missing_research_artifact, zero_tests_run, tests_failed_nonzero,
// unrouted_needs_clarification, chain.recovery.exhausted) lands in
// follow-up pieces per the Slice 1c piece-split precedent. The routing
// table pins the future targets; detection wiring catches up arc-by-arc
// as smoke evidence motivates each one.
//
// Mode-mismatch detection is cross-event safe: chain.mode.classification
// is stamped by chainmode.Stamper at coordinator-completion time (a
// prior event), and the producer loop's coordinator.decision.next_action
// is stamped by the upstream agentic.decide tool BEFORE the loop's
// agent.complete fires. Neither read depends on a sibling
// CompletionHandler's same-event write.
func (s *Subscriber) detectRejection(ev *agentic.LoopCompletedEvent, producerTriples, chainTriples map[string]any) (string, bool) {
	if ev.Role != "researcher-synthesize" {
		return "", false
	}
	action, _ := producerTriples[agvocab.CoordinatorNextAction].(string)
	if action != "architect" {
		// emit / gather / synthesize / approved — phase validator allows
		// or routes elsewhere. No mode_mismatch.
		return "", false
	}
	mode, _ := chainTriples[chain.PredicateChainMode].(string)
	if mode != "research_only" {
		// dev_via_spec or unclassified — architect is allowed (or no
		// classification yet, which is its own chain config bug).
		return "", false
	}
	return ReasonModeMismatch, true
}

// stampAuditCluster writes the chain.rejection.* + chain.stall.* audit
// triples on the chain entity. Per-triple write errors are logged and
// remaining triples proceed (partial cluster is queryable; a zero-triple
// record is invisible).
func (s *Subscriber) stampAuditCluster(ctx context.Context, chainEntityID string, ev *agentic.LoopCompletedEvent, reason string, route Route, newCount int, capHit bool, now time.Time) {
	stamp := func(predicate string, object any) message.Triple {
		return message.Triple{
			Subject:    chainEntityID,
			Predicate:  predicate,
			Object:     object,
			Source:     triplesSource,
			Timestamp:  now,
			Confidence: 1.0,
		}
	}
	triples := []message.Triple{
		stamp(chain.PredicateChainRejectionRecoveryCount, strconv.Itoa(newCount)),
		stamp(chain.PredicateChainStallRoutedTo, string(route)),
		stamp(chain.PredicateChainStallReason, reason),
		stamp(chain.PredicateChainStallProducerLoopID, ev.LoopID),
		stamp(chain.PredicateChainStallProducerRole, ev.Role),
		stamp(chain.PredicateChainStallObservedAt, now.Format(time.RFC3339)),
	}
	if capHit {
		triples = append(triples, stamp(chain.PredicateChainRejectionExhausted, "true"))
	}
	if route == RouteHuman {
		triples = append(triples, stamp(chain.PredicateChainStallAwaitingHuman, "true"))
	}
	for _, t := range triples {
		if err := s.publisher.AddTriple(ctx, t); err != nil {
			s.logger.Warn("chainstall: audit triple write failed",
				slog.String("predicate", t.Predicate),
				slog.String("chain_entity", chainEntityID),
				slog.String("error", err.Error()))
		}
	}
}

// stampPerLoopSignals writes the per-loop chain.stall.proceed +
// chain.stall.reason sentinels on the producer loop entity when routed
// coordinator. Rule 05 fires on chain.stall.proceed=="true"; the reason
// triple is read via $entity.triple.chain.stall.reason in the rule's
// spawn-prompt template. Absence of proceed (write failure or human
// route) leaves no rule fire → chain stalls fail-safe.
func (s *Subscriber) stampPerLoopSignals(ctx context.Context, producerEntityID string, route Route, reason string, now time.Time) {
	if route != RouteCoordinator {
		return
	}
	stamp := func(predicate string, object any) message.Triple {
		return message.Triple{
			Subject:    producerEntityID,
			Predicate:  predicate,
			Object:     object,
			Source:     triplesSource,
			Timestamp:  now,
			Confidence: 1.0,
		}
	}
	triples := []message.Triple{
		stamp(chain.PredicateChainStallReason, reason),
		stamp(chain.PredicateChainStallProceed, "true"),
	}
	for _, t := range triples {
		if err := s.publisher.AddTriple(ctx, t); err != nil {
			s.logger.Warn("chainstall: per-loop signal write failed",
				slog.String("predicate", t.Predicate),
				slog.String("producer_entity", producerEntityID),
				slog.String("error", err.Error()))
		}
	}
}

// readCount parses chain.rejection.recovery_count's object. Mirrors
// recoverycounter.readCount's coercion table — the wire shape is a
// string-formatted integer ("0", "1", …); JSON unmarshal of the
// graph-query response surfaces it as `any`, so we coerce defensively.
// Missing predicate returns 0 (no prior cycles); malformed returns 0.
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
