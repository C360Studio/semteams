package phasevalidator

import (
	"context"
	"testing"

	"github.com/c360studio/semstreams/agentic"
	agvocab "github.com/c360studio/semstreams/vocabulary/agentic"

	"github.com/c360studio/semteams/cmd/semteams/chain"
)

func buildQAModeGate(_ *testing.T, pub TriplePublisher, entities chain.EntityTripleReader) *QAModeGate {
	resolver := resolverWithChain(map[string]string{
		testLoopID: testRootID,
	})
	return NewQAModeGate(pub, resolver, entities, testPlatformMeta(), nil)
}

func builderEvent() *agentic.LoopCompletedEvent {
	return &agentic.LoopCompletedEvent{
		LoopID:  testLoopID,
		Role:    builderRole,
		Outcome: agentic.OutcomeSuccess,
	}
}

func TestQAModeGate_NonBuilderIgnored(t *testing.T) {
	pub := &recordingPublisher{}
	g := buildQAModeGate(t, pub, &fakeEntityReader{})
	for _, role := range []string{"reviewer-qa", "reviewer-spec", "researcher-plan", "coordinator", ""} {
		ev := builderEvent()
		ev.Role = role
		if err := g.HandleLoopCompleted(context.Background(), ev); err != nil {
			t.Errorf("role %q: %v", role, err)
		}
	}
	if len(pub.triples) != 0 {
		t.Errorf("non-builder roles triggered %d writes", len(pub.triples))
	}
}

func TestQAModeGate_NonTestsPassingSkips(t *testing.T) {
	pub := &recordingPublisher{}
	entities := &fakeEntityReader{
		entities: map[string]map[string]any{
			loopEntityID(testLoopID): {
				agvocab.CoordinatorNextAction: "tests_failing",
			},
		},
	}
	g := buildQAModeGate(t, pub, entities)
	if err := g.HandleLoopCompleted(context.Background(), builderEvent()); err != nil {
		t.Fatalf("HandleLoopCompleted: %v", err)
	}
	if len(pub.triples) != 0 {
		t.Errorf("non-tests_passing terminal should bypass gate; got %d writes", len(pub.triples))
	}
}

func TestQAModeGate_ApprovedHappyPath(t *testing.T) {
	pub := &recordingPublisher{}
	entities := &fakeEntityReader{
		entities: map[string]map[string]any{
			loopEntityID(testLoopID): {
				agvocab.CoordinatorNextAction: testsPassingAction,
				predicateBuilderTestsRun:      "5",
				predicateBuilderTestsFailed:   "0",
			},
		},
	}
	g := buildQAModeGate(t, pub, entities)
	if err := g.HandleLoopCompleted(context.Background(), builderEvent()); err != nil {
		t.Fatalf("HandleLoopCompleted: %v", err)
	}
	proceed, ok := pub.byPredicate(chain.PredicateQAModeGateProceed)
	if !ok {
		t.Fatal("expected proceed sentinel")
	}
	if proceed.Subject != loopEntityID(testLoopID) {
		t.Errorf("proceed subject = %q, want loop entity", proceed.Subject)
	}
	if pub.hasPredicate(chain.PredicateQAModeGateRejected) {
		t.Error("approval should not stamp rejection")
	}
}

func TestQAModeGate_RejectsZeroTestsRun(t *testing.T) {
	// Defense in depth: builder_decide validates this at the call
	// site too, but the gate is the chain-level catch.
	pub := &recordingPublisher{}
	entities := &fakeEntityReader{
		entities: map[string]map[string]any{
			loopEntityID(testLoopID): {
				agvocab.CoordinatorNextAction: testsPassingAction,
				predicateBuilderTestsRun:      "0",
				predicateBuilderTestsFailed:   "0",
			},
		},
	}
	g := buildQAModeGate(t, pub, entities)
	if err := g.HandleLoopCompleted(context.Background(), builderEvent()); err != nil {
		t.Fatalf("HandleLoopCompleted: %v", err)
	}
	rejected, ok := pub.byPredicate(chain.PredicateQAModeGateRejected)
	if !ok {
		t.Fatal("expected rejection marker")
	}
	if rejected.Subject != testChainEntityID {
		t.Errorf("rejection subject = %q, want chain entity", rejected.Subject)
	}
	reason, _ := pub.byPredicate(chain.PredicateQAModeGateRejectReason)
	if s, _ := reason.Object.(string); s != qaModeRejectZeroTests {
		t.Errorf("reject reason = %v, want %q", reason.Object, qaModeRejectZeroTests)
	}
	if pub.hasPredicate(chain.PredicateQAModeGateProceed) {
		t.Error("rejection should not stamp proceed sentinel")
	}
}

func TestQAModeGate_RejectsNonzeroTestsFailed(t *testing.T) {
	// The structural contradiction builder_decide does NOT catch:
	// action=tests_passing claimed with tests_failed > 0.
	pub := &recordingPublisher{}
	entities := &fakeEntityReader{
		entities: map[string]map[string]any{
			loopEntityID(testLoopID): {
				agvocab.CoordinatorNextAction: testsPassingAction,
				predicateBuilderTestsRun:      "5",
				predicateBuilderTestsFailed:   "2",
			},
		},
	}
	g := buildQAModeGate(t, pub, entities)
	if err := g.HandleLoopCompleted(context.Background(), builderEvent()); err != nil {
		t.Fatalf("HandleLoopCompleted: %v", err)
	}
	reason, ok := pub.byPredicate(chain.PredicateQAModeGateRejectReason)
	if !ok {
		t.Fatal("expected reject reason")
	}
	if s, _ := reason.Object.(string); s != qaModeRejectNonzeroFailed {
		t.Errorf("reject reason = %v, want %q", reason.Object, qaModeRejectNonzeroFailed)
	}
}

func TestQAModeGate_MissingTriplesCountAsZero(t *testing.T) {
	// Defensive: a builder loop that stamped action=tests_passing but
	// lacks the dev_via_spec.builder.tests_* triples (framework bug,
	// triple-flush race, etc.) is treated as tests_run=0 and rejected
	// with zero_tests_run. Fail-safe.
	pub := &recordingPublisher{}
	entities := &fakeEntityReader{
		entities: map[string]map[string]any{
			loopEntityID(testLoopID): {
				agvocab.CoordinatorNextAction: testsPassingAction,
			},
		},
	}
	g := buildQAModeGate(t, pub, entities)
	if err := g.HandleLoopCompleted(context.Background(), builderEvent()); err != nil {
		t.Fatalf("HandleLoopCompleted: %v", err)
	}
	reason, _ := pub.byPredicate(chain.PredicateQAModeGateRejectReason)
	if s, _ := reason.Object.(string); s != qaModeRejectZeroTests {
		t.Errorf("missing-triples reject reason = %v, want %q", reason.Object, qaModeRejectZeroTests)
	}
}

func TestQAModeGate_NumericTypesRejectionPath(t *testing.T) {
	// Symmetry with TestQAModeGate_NumericTypesAccepted: float64 typed
	// values must also coerce on the rejection path. tests_failed=2.0
	// triggers the tests_failed_nonzero rejection identically to the
	// string-typed case.
	pub := &recordingPublisher{}
	entities := &fakeEntityReader{
		entities: map[string]map[string]any{
			loopEntityID(testLoopID): {
				agvocab.CoordinatorNextAction: testsPassingAction,
				predicateBuilderTestsRun:      float64(7),
				predicateBuilderTestsFailed:   float64(2),
			},
		},
	}
	g := buildQAModeGate(t, pub, entities)
	if err := g.HandleLoopCompleted(context.Background(), builderEvent()); err != nil {
		t.Fatalf("HandleLoopCompleted: %v", err)
	}
	reason, _ := pub.byPredicate(chain.PredicateQAModeGateRejectReason)
	if s, _ := reason.Object.(string); s != qaModeRejectNonzeroFailed {
		t.Errorf("float-typed rejection reason = %v, want %q", reason.Object, qaModeRejectNonzeroFailed)
	}
}

func TestQAModeGate_NumericTypesAccepted(t *testing.T) {
	// float64 (json.Unmarshal default for numbers) should coerce same as
	// strings — readCount handles both.
	pub := &recordingPublisher{}
	entities := &fakeEntityReader{
		entities: map[string]map[string]any{
			loopEntityID(testLoopID): {
				agvocab.CoordinatorNextAction: testsPassingAction,
				predicateBuilderTestsRun:      float64(7),
				predicateBuilderTestsFailed:   float64(0),
			},
		},
	}
	g := buildQAModeGate(t, pub, entities)
	if err := g.HandleLoopCompleted(context.Background(), builderEvent()); err != nil {
		t.Fatalf("HandleLoopCompleted: %v", err)
	}
	if !pub.hasPredicate(chain.PredicateQAModeGateProceed) {
		t.Error("expected proceed sentinel for float-typed counts")
	}
}
