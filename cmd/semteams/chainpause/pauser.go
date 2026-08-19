package chainpause

import (
	"context"
	"slices"
	"strings"
	"time"
	"unicode"

	"github.com/c360studio/semstreams/agentic"
	"github.com/c360studio/semstreams/message"
)

// TriplePublisher is the narrow write surface the pauser needs.
// agentictools.NATSTriplePublisher satisfies it structurally.
type TriplePublisher interface {
	Append(ctx context.Context, triples []message.Triple) error
}

// PauseResult holds the classification output of a pause-write.
// Returned from HandleFailed to let callers inspect the written cause for
// logging or test assertions.
type PauseResult struct {
	FailedLoopID   string
	Role           string
	Cause          string // closed token: api_overloaded | api_timeout | config_load_failure | tool_executor_panic | max_iterations | unknown
	Classification string // finer-grained label for ops-agent aggregation
	ObservedAt     time.Time
}

// Pauser subscribes to agent.failed.* events and writes the ADR-037 §D5
// audit triple set onto the canonical 6-part chain entity (ADR-038 PR B
// Phase 3 re-point — pre-Phase-3 the writes targeted the failed loop's
// entity).
//
// Fail-soft: any individual triple write error is logged (by callers via the
// returned error) but does not abort the remaining writes. Triple write errors
// on the pause path are worse than partial writes — a partial §D5 record is
// queryable; a zero-triple record is invisible. A failed loop with no run anchor
// (ev.RunEntityID == "", a loop outside any run) has no chain entity to stamp, so
// the §D5 writes are skipped; the agent.failed.* event is on the wire regardless.
type Pauser struct {
	publisher TriplePublisher
}

// NewPauser constructs a Pauser backed by the given publisher. The chain entity
// ID is read off the LoopFailedEvent's RunEntityID, populated by the framework at
// dispatch (semstreams#250, beta.105) — the ancestry-walk Resolver is retired
// (ADR-053 Phase 5, ADR-053 D8: subscribers read the resolved run off the wire).
func NewPauser(pub TriplePublisher) *Pauser {
	return &Pauser{publisher: pub}
}

// HandleFailed is the subscription entry point for agent.failed.* events.
// It filters for roles in the configured arc prefix list, classifies the
// error string into a cause token, and writes the §D5 triple set onto
// the canonical chain entity.
//
// Returns the PauseResult for caller logging. Resolver errors return a
// wrapped error and skip writes (no chain entity ID → no Subject for
// the §D5 cluster). Per-triple errors surface as a non-nil first error
// while the remaining writes proceed.
//
// Roles not in the managed arc list are silently skipped.
func (p *Pauser) HandleFailed(ctx context.Context, ev *agentic.LoopFailedEvent) (PauseResult, error) {
	if !isManagedRole(ev.Role) {
		return PauseResult{}, nil
	}

	// The framework resolves the run/chain entity at dispatch and carries it on
	// the failure event (semstreams#250). No run anchor → the loop is outside a
	// run; there is no chain entity to stamp §D5 on, so skip.
	entityID := ev.RunEntityID
	if entityID == "" {
		return PauseResult{}, nil
	}
	now := time.Now().UTC()
	cause, classification := classifyError(ev.Error)

	res := PauseResult{
		FailedLoopID:   ev.LoopID,
		Role:           ev.Role,
		Cause:          cause,
		Classification: classification,
		ObservedAt:     now,
	}

	// Write §D5 triple set — errors are collected but do not abort.
	// Callers log the multi-error if any writes fail.
	//
	// chain.paused.original-model captures the model name from the failed loop
	// so the retry path can re-publish with the same model without needing a
	// live graph query. ADR-037 §D7 "preserve role and model".
	//
	// chain.paused.prior-attempts is hardcoded to "1" in v1. Counting prior
	// pauses per (chain, role) pair earns its slot when a real cycle surfaces
	// in smoke evidence — until then, looking-implemented-but-fixed is worse
	// than obviously-deferred. (ADR-037 Open Question OQ2.)
	var firstErr error
	triples := []message.Triple{
		{Subject: entityID, Predicate: "chain.paused.cause", Object: cause},
		{Subject: entityID, Predicate: "chain.paused.classification", Object: classification},
		{Subject: entityID, Predicate: "chain.paused.role", Object: ev.Role},
		{Subject: entityID, Predicate: "chain.paused.original-model", Object: ev.Model},
		{Subject: entityID, Predicate: "chain.paused.error-shape", Object: sanitiseErrorShape(ev.Error)},
		{Subject: entityID, Predicate: "chain.paused.prior-attempts", Object: "1"},
		{Subject: entityID, Predicate: "chain.paused.failed-loop-id", Object: ev.LoopID},
		{Subject: entityID, Predicate: "chain.paused.spawn-loop-id", Object: ""}, // v1: derived from prior_loop_id at query time (ADR-037 OQ1)
		{Subject: entityID, Predicate: "chain.paused.observed-at", Object: now.Format(time.RFC3339)},
	}
	for i := range triples {
		triples[i].Source = "chainpause"
		triples[i].Timestamp = now
		triples[i].Confidence = 1.0
		if err := p.publisher.Append(ctx, triples[i:i+1]); err != nil && firstErr == nil {
			firstErr = err
		}
	}

	return res, firstErr
}

// managedRoles is the closed set of role names this package monitors.
// It mirrors the rule's "in" condition list exactly. The contract test
// TestChainPauseRule_ManagedRoleParity_Bidirectional asserts that both
// lists stay in sync.
var managedRoles = []string{
	// research pack — mirrors configs/rules/research/08-loop-failed-pause.json
	"researcher-research-plan",
	"researcher-research-gather",
	"researcher-research-synthesize",
	"reviewer-research",
	// autoresearch pack — mirrors configs/rules/autoresearch/11-loop-failed-pause.json
	"autoresearch-baseline",
	"autoresearch-propose",
	"autoresearch-synthesize",
	"reviewer-autoresearch",
	"dispatch",
}

// ManagedRoles returns a copy of the closed role-name set this package monitors.
// Exported so the contract test can assert bidirectional parity with the rule's
// "in" condition list (R2).
func ManagedRoles() []string {
	out := make([]string, len(managedRoles))
	copy(out, managedRoles)
	return out
}

// isManagedRole returns true when the role is in a chain arc this package
// is responsible for. The list mirrors the rule's "in" condition list exactly.
// Kept as a closed set rather than prefix matching — explicit is auditable.
func isManagedRole(role string) bool {
	return slices.Contains(managedRoles, role)
}

// classifyError maps the raw error string from LoopFailedEvent.Error to a
// (cause, classification) pair. Both are closed tokens per ADR-037 §D5.
//
// cause: high-level token for decision routing.
// classification: finer-grained for ops-agent pattern aggregation.
//
// Matching is case-insensitive contains, not prefix — error strings from the
// Anthropic SDK and from the framework's timeout path carry the token anywhere
// in the string.
func classifyError(errStr string) (cause, classification string) {
	lower := strings.ToLower(errStr)

	switch {
	case strings.Contains(lower, "overloaded"):
		return "api_overloaded", "anthropic_overloaded_529"
	case strings.Contains(lower, "timeout") || strings.Contains(lower, "deadline exceeded"):
		return "api_timeout", "llm_call_timeout"
	case strings.Contains(lower, "persona") && strings.Contains(lower, "load"):
		return "config_load_failure", "persona_load_error"
	case strings.Contains(lower, "config") && (strings.Contains(lower, "load") || strings.Contains(lower, "invalid")):
		return "config_load_failure", "config_load_error"
	case strings.Contains(lower, "panic") || strings.Contains(lower, "executor panic"):
		return "tool_executor_panic", "tool_executor_panic"
	case strings.Contains(lower, "max iter") || strings.Contains(lower, "maximum iterations"):
		return "max_iterations", "max_iterations_exhausted"
	default:
		return "unknown", "unknown"
	}
}

// sanitiseErrorShape returns a length-bounded, control-char-stripped
// representative of the error string for triple storage. PII may appear in
// error strings (model outputs, prompt excerpts) so this is deliberately
// truncated. ADR-030 sanitisation discipline applies here.
func sanitiseErrorShape(errStr string) string {
	const maxBytes = 256
	// Strip control characters (ADR-030 § sanitisation: no control chars).
	clean := strings.Map(func(r rune) rune {
		if unicode.IsControl(r) {
			return -1 // drop
		}
		return r
	}, errStr)
	if len(clean) > maxBytes {
		clean = clean[:maxBytes] + "…"
	}
	return clean
}
