package chainstall

import "testing"

// TestReasonToRoute_AllEightReasonsRouted pins the routing-table
// coverage. Adding a new structural-gate reject reason to any producer
// package without updating reasonToRoute fails this test — see
// test/contract/chain_stall_routing_test.go for the cross-package pin.
func TestReasonToRoute_AllEightReasonsRouted(t *testing.T) {
	expectations := map[string]Route{
		ReasonModeMismatch:               RouteCoordinator,
		ReasonPhaseCap:                   RouteHuman,
		ReasonInvalidEdge:                RouteHuman,
		ReasonMissingResearchArtifact:    RouteCoordinator,
		ReasonZeroTestsRun:               RouteHuman,
		ReasonTestsFailedNonzero:         RouteHuman,
		ReasonUnroutedNeedsClarification: RouteCoordinator,
		ReasonRecoveryExhausted:          RouteHuman,
	}
	for reason, want := range expectations {
		got, ok := RouteForReason(reason)
		if !ok {
			t.Errorf("RouteForReason(%q): not routed; expected %q", reason, want)
			continue
		}
		if got != want {
			t.Errorf("RouteForReason(%q) = %q, want %q", reason, got, want)
		}
	}
	// Pin the exact table size — a future maintainer who adds a new
	// reason MUST extend the expectations map above.
	if len(reasonToRoute) != len(expectations) {
		t.Errorf("reasonToRoute size = %d; expectations cover %d. A new reason without a test entry would slip through.",
			len(reasonToRoute), len(expectations))
	}
}

// TestRouteForReason_UnknownReturnsFalse pins the unknown-reason
// fail-safe: RouteForReason must return ok=false so the subscriber's
// detectRejection path can log-and-skip rather than route under a
// silently-incorrect default.
func TestRouteForReason_UnknownReturnsFalse(t *testing.T) {
	if _, ok := RouteForReason("not_a_real_reason"); ok {
		t.Error("RouteForReason should return ok=false for unknown reasons; got ok=true")
	}
}

// TestAllReasons_EnumeratesRoutingTable pins AllReasons against
// reasonToRoute so the contract test's enumeration helper stays in sync
// with the production routing path.
func TestAllReasons_EnumeratesRoutingTable(t *testing.T) {
	got := AllReasons()
	if len(got) != len(reasonToRoute) {
		t.Errorf("AllReasons returned %d reasons, reasonToRoute has %d", len(got), len(reasonToRoute))
	}
	for _, reason := range got {
		if _, ok := reasonToRoute[reason]; !ok {
			t.Errorf("AllReasons returned %q which is not in reasonToRoute", reason)
		}
	}
}

// TestCoordinatorVsHumanRouteCount pins the coordinator/human split.
// Today 3 coordinator-route reasons + 5 human-route reasons. A change
// to this split is a design decision that should require an explicit
// test update + a §addendum to the slice 2 design memo.
func TestCoordinatorVsHumanRouteCount(t *testing.T) {
	var coord, human int
	for _, r := range reasonToRoute {
		switch r {
		case RouteCoordinator:
			coord++
		case RouteHuman:
			human++
		default:
			t.Errorf("unexpected route %q in routing table", r)
		}
	}
	const wantCoord, wantHuman = 3, 5
	if coord != wantCoord {
		t.Errorf("coordinator routes = %d, want %d", coord, wantCoord)
	}
	if human != wantHuman {
		t.Errorf("human routes = %d, want %d", human, wantHuman)
	}
}
