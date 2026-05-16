package chainstall

import "sort"

// Route classifies the recovery path for a structural-gate rejection.
//
// RouteCoordinator → wake a fresh coordinator loop with stall context;
// the coordinator re-frames the spawn prompt and retries. Reserved for
// reject reasons that name a framing problem (persona drift, weak spawn
// prompt) — an LLM call can plausibly fix.
//
// RouteHuman → stamp chain.stall.awaiting_human; the operator decides via
// /teams-loop/chain-stall/decide. Reserved for reject reasons that name
// budget exhaustion or structural impossibility — more LLM calls won't
// fix, they just cost more.
type Route string

const (
	// RouteCoordinator wakes a coordinator loop via rule 05.
	RouteCoordinator Route = "coordinator"

	// RouteHuman publishes a chain.stall.awaiting_human audit and waits
	// for the operator's verdict at the HTTP endpoint.
	RouteHuman Route = "human"
)

// Reject-reason tokens. Single source of truth duplicated here from
// cmd/semteams/phasevalidator/ + cmd/semteams/chain/needs_review.go +
// cmd/semteams/recoverycounter/ so chainstall's routing table doesn't
// import every producer package. A contract test
// (test/contract/chain_stall_routing_test.go) pins these spellings
// against the producer constants — divergence fails build.
const (
	// ReasonModeMismatch — phasevalidator (synthesize→architect under
	// research_only). Producer: researcher-synthesize. Route:
	// coordinator (persona drift; better framing on retry can fix).
	ReasonModeMismatch = "mode_mismatch"

	// ReasonPhaseCap — phasevalidator (chain.researcher.phase_count.<phase>
	// exceeded ADR-041 cap). Producer: any researcher role. Route: human
	// (budget exhausted; retry hits the cap again).
	ReasonPhaseCap = "phase_cap"

	// ReasonInvalidEdge — phasevalidator (persona attempted illegal
	// phase transition). Producer: any researcher role. Route: human
	// (allowed_actions list is wrong OR persona fundamentally confused;
	// LLM retry unlikely to help).
	ReasonInvalidEdge = "invalid_edge"

	// ReasonMissingResearchArtifact — specModeGate (reviewer-spec
	// approved without a chain.research_artifact.loop ancestor).
	// Producer: reviewer-spec. Route: coordinator (research arc may be
	// salvageable with a stronger researcher spawn).
	ReasonMissingResearchArtifact = "missing_research_artifact"

	// ReasonZeroTestsRun — qaModeGate (builder claimed tests_passing
	// with tests_run=0). Producer: builder. Route: human (persona drift
	// OR test infra didn't execute; both want human inspection).
	ReasonZeroTestsRun = "zero_tests_run"

	// ReasonTestsFailedNonzero — qaModeGate (builder claimed
	// tests_passing with tests_failed>0). Producer: builder. Route:
	// human (persona drift OR test infra defect; both want human
	// inspection).
	ReasonTestsFailedNonzero = "tests_failed_nonzero"

	// ReasonUnroutedNeedsClarification — NeedsReviewStamper (ADR-039
	// Phase 1 Tier 3 catch-all). Producer: builder (Phase 1 scope).
	// Route: coordinator (producer asked a structured clarification
	// question; coordinator is the right router — already designed this
	// way in ADR-039 Phase 2 sketch).
	ReasonUnroutedNeedsClarification = "unrouted_needs_clarification"

	// ReasonRecoveryExhausted — recoverycounter (research-reviewer /
	// reviewer-spec rejected 3 times in a row, ADR-040 cap). Producer:
	// the final rejecting reviewer. Route: human (ADR-040's cap already
	// gave 3 retries; piling more isn't recovery, it's persistence).
	ReasonRecoveryExhausted = "chain.recovery.exhausted"
)

// reasonToRoute is the single source of truth for chainstall's per-reason
// routing. Adding a new structural-gate reject reason to any producer
// package requires updating this table — the contract test
// (test/contract/chain_stall_routing_test.go) enumerates the producer
// constants and fails the build if any aren't routed here.
var reasonToRoute = map[string]Route{
	ReasonModeMismatch:               RouteCoordinator,
	ReasonPhaseCap:                   RouteHuman,
	ReasonInvalidEdge:                RouteHuman,
	ReasonMissingResearchArtifact:    RouteCoordinator,
	ReasonZeroTestsRun:               RouteHuman,
	ReasonTestsFailedNonzero:         RouteHuman,
	ReasonUnroutedNeedsClarification: RouteCoordinator,
	ReasonRecoveryExhausted:          RouteHuman,
}

// RouteForReason returns the default route for a reject reason. ok=false
// for unrecognised tokens — callers should treat unknown reasons as
// "skip" (no audit cluster, no rule fire) to fail-safe; an unrouted
// reason is by definition a producer that landed a new token without
// updating chainstall's table, and the contract test catches that at
// build time.
func RouteForReason(reason string) (Route, bool) {
	r, ok := reasonToRoute[reason]
	return r, ok
}

// AllReasons returns every reason in the routing table in deterministic
// sorted order. Exported for the contract test's enumeration — callers
// SHOULD NOT iterate this outside of test code; production routing goes
// through RouteForReason. Sorted to keep debug logs and contract-test
// failure messages stable across runs.
func AllReasons() []string {
	out := make([]string, 0, len(reasonToRoute))
	for r := range reasonToRoute {
		out = append(out, r)
	}
	sort.Strings(out)
	return out
}
