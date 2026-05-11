package recoverycounter

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
	// reviewerRole is the role this counter monitors. Research-reviewer
	// is the rule_02 trigger; only its "insufficient" terminals consume
	// chain recovery budget. Other roles (researcher, curator, planner,
	// etc.) are intentionally ignored — coupling the counter to roles
	// other than the actual recovery trigger would mis-attribute budget
	// to non-recovery cycles.
	reviewerRole = "research-reviewer"

	// insufficientAction is the coordinator.next_action value the
	// research-reviewer's decide tool stamps when terminating with a
	// rejection that triggers the source-curator recovery path. Mirrors
	// rule_02's condition exactly — divergence here would silently
	// decouple counter increments from rule evaluation.
	insufficientAction = "insufficient"

	// triplesSource is the audit-trail Source field on every triple
	// this package writes. Matches the chain.<predicate-stem> convention
	// the sibling stampers use (chain.dispatched, chain.needs_review,
	// chain.paused), so RDF / ops-query consumers can group all
	// recovery-cap writes under one Source value.
	triplesSource = "chain.recovery"

	// DefaultThreshold is the per-chain recovery-cycle cap when none is
	// configured. Three matches the per-rule max_iterations across
	// rules 02/02b/02c — the rule engine bound and the chain-level
	// bound stay aligned so operators don't have to reason about two
	// different ceilings. Configurable via NewCounter.
	DefaultThreshold = 3
)

// TriplePublisher is the narrow write surface the counter needs.
// agentictools.NATSTriplePublisher satisfies it structurally.
type TriplePublisher interface {
	AddTriple(ctx context.Context, triple message.Triple) error
}

// Counter implements chain.CompletionHandler and gates curator
// recovery cycles at the chain level (ADR-040 §addendum 2026-05-11).
//
// Filter contract: only fires on research-reviewer success events
// whose loop entity carries coordinator.next_action="insufficient".
// Returns nil (no error) for every other event so the
// CompletionSubscriber can dispatch to siblings.
//
// Write contract — three distinct predicates, three distinct
// subjects, three distinct semantic roles:
//
//   - chain.recovery.count → chain entity. Per-cycle audit;
//     incremented monotonically. Operator observable.
//   - chain.recovery.proceed → reviewer loop entity. The per-cycle
//     gate sentinel. Stamped "true" ONLY when within budget
//     (newCount <= threshold). Rule_02 fires only when this lands.
//     Absence → rule never fires → chain stalls fail-safe.
//   - chain.recovery.exhausted → chain entity. Cap-hit marker
//     stamped when newCount > threshold. Operator escalation
//     surface; rule does NOT gate on it.
//
// See doc.go for the design rationale (Counter-owned gating vs
// rule-only), the two-step rule fire sequence, and fail-safe
// semantics.
//
// Failure mode: per-triple AddTriple errors are logged and the
// remaining writes proceed (partial cluster is queryable; a
// zero-triple record is invisible). Resolver / ReadEntity failures
// log Warn and skip the entire cycle — the agent.complete event is
// on the wire regardless, so the absent stamp is observable.
type Counter struct {
	publisher TriplePublisher
	resolver  *chain.Resolver
	entities  chain.EntityTripleReader
	platform  types.PlatformMeta
	threshold int
	logger    *slog.Logger
}

// NewCounter constructs a Counter. threshold ≤ 0 falls back to
// DefaultThreshold so callers can pass 0 for "use the default."
func NewCounter(
	pub TriplePublisher,
	resolver *chain.Resolver,
	entities chain.EntityTripleReader,
	platform types.PlatformMeta,
	threshold int,
	logger *slog.Logger,
) *Counter {
	if logger == nil {
		logger = slog.Default()
	}
	if threshold <= 0 {
		threshold = DefaultThreshold
	}
	return &Counter{
		publisher: pub,
		resolver:  resolver,
		entities:  entities,
		platform:  platform,
		threshold: threshold,
		logger:    logger,
	}
}

// HandleLoopCompleted is the chain.CompletionHandler entry point.
// Filters to research-reviewer success with
// coordinator.next_action="insufficient"; non-matching events
// return nil so other handlers see the event.
func (c *Counter) HandleLoopCompleted(ctx context.Context, ev *agentic.LoopCompletedEvent) error {
	if ev == nil {
		return nil
	}
	if ev.Role != reviewerRole || ev.Outcome != agentic.OutcomeSuccess {
		return nil
	}
	if ev.LoopID == "" {
		// Defensive: the upstream LoopCompletedEvent.Validate enforces
		// non-empty LoopID at publish time, but the subscriber path
		// reaches us via JSON unmarshal which doesn't re-run Validate.
		// Empty LoopID would propagate into LoopExecutionEntityID's
		// panic; surface it as an error instead.
		return fmt.Errorf("recoverycounter.Counter: empty LoopID on agent.complete event")
	}

	reviewerEntityID := agentic.LoopExecutionEntityID(c.platform.Org, c.platform.Platform, ev.LoopID)
	reviewerTriples, err := c.entities.ReadEntity(ctx, reviewerEntityID)
	if err != nil {
		c.logger.Warn("recovery counter: read reviewer entity failed; skipping increment",
			slog.String("reviewer_loop_id", ev.LoopID),
			slog.String("error", err.Error()))
		return nil
	}
	action, _ := reviewerTriples[agvocab.CoordinatorNextAction].(string)
	if action != insufficientAction {
		// approved / other terminal — not a recovery trigger.
		return nil
	}

	chainEntityID, err := c.resolver.ChainEntityID(ctx, ev.LoopID)
	if err != nil {
		c.logger.Warn("recovery counter: chain ancestry walk failed; skipping increment",
			slog.String("reviewer_loop_id", ev.LoopID),
			slog.String("error", err.Error()))
		return nil
	}

	chainTriples, err := c.entities.ReadEntity(ctx, chainEntityID)
	if err != nil {
		c.logger.Warn("recovery counter: read chain entity failed; skipping increment",
			slog.String("reviewer_loop_id", ev.LoopID),
			slog.String("chain_entity", chainEntityID),
			slog.String("error", err.Error()))
		return nil
	}
	priorCount := readCount(chainTriples[chain.PredicateRecoveryCount])
	newCount := priorCount + 1
	withinBudget := newCount <= c.threshold

	now := time.Now().UTC()
	stamp := func(subject, predicate string, object any) message.Triple {
		return message.Triple{
			Subject:    subject,
			Predicate:  predicate,
			Object:     object,
			Source:     triplesSource,
			Timestamp:  now,
			Confidence: 1.0,
		}
	}

	// chain.recovery.count is the audit trail; lands on the chain entity
	// per cycle. String-formatted int so the rule engine's expression
	// operators (eq/lte/gt) compare cleanly without coercion.
	//
	// chain.recovery.proceed is the per-cycle gate sentinel; lands on the
	// reviewer loop entity ONLY when within budget. Rule_02 fires only
	// when this predicate is "true" — Counter failure to write it (e.g.
	// graph blip mid-walk above) leaves no proceed → rule never fires →
	// chain stalls fail-safe, never runs unbounded.
	//
	// chain.recovery.exhausted is the cap-hit marker; lands on the chain
	// entity when newCount > threshold. Operators + ops-agent consume it
	// to surface stalled chains; rule_02 does NOT gate on it directly
	// (the absence of proceed is what blocks the fire).
	countObj := strconv.Itoa(newCount)
	// Audit always lands BEFORE the budget branch — operators see
	// recovery_count climbing whether the cap blocks or not.
	triples := []message.Triple{
		stamp(chainEntityID, chain.PredicateRecoveryCount, countObj),
	}
	if withinBudget {
		triples = append(triples,
			stamp(reviewerEntityID, chain.PredicateRecoveryProceed, "true"),
		)
	} else {
		triples = append(triples,
			stamp(chainEntityID, chain.PredicateRecoveryExhausted, "true"),
		)
	}

	for _, t := range triples {
		if err := c.publisher.AddTriple(ctx, t); err != nil {
			c.logger.Warn("recovery counter: triple write failed",
				slog.String("predicate", t.Predicate),
				slog.String("subject", t.Subject),
				slog.String("error", err.Error()))
		}
	}

	c.logger.Info("recovery counter: cycle stamped",
		slog.String("reviewer_loop_id", ev.LoopID),
		slog.String("chain_entity", chainEntityID),
		slog.Int("prior_count", priorCount),
		slog.Int("new_count", newCount),
		slog.Int("threshold", c.threshold),
		slog.Bool("within_budget", withinBudget))
	return nil
}

// readCount parses the chain.recovery_count triple object. The wire
// shape is a string-formatted integer ("0", "1", …); JSON unmarshal
// of the graph-query response surfaces it as `any`, so we coerce
// from string-or-number defensively.
//
// Missing predicate (the chain has not yet had a recovery cycle)
// returns 0 — the natural "no prior attempts" prior. Malformed
// objects also return 0; the misclassification is logged at the
// caller's increment site via the new_count / prior_count log
// fields, so an operator inspecting a chain that "skipped" cycles
// can correlate via the logs.
//
// Type branches:
//   - string: the production wire shape (this package writes
//     strconv.Itoa(n)).
//   - float64: the json.Unmarshal default for any number that ever
//     reaches the graph-query response via a non-stringly-typed
//     code path.
//   - int / int64: only fire from test-injection (fakeEntityReader
//     in counter_test.go); harmless to keep so tests don't have to
//     stringify before injecting.
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
