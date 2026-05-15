package phasevalidator

import (
	"context"
	"testing"

	"github.com/c360studio/semstreams/agentic"
	agvocab "github.com/c360studio/semstreams/vocabulary/agentic"

	"github.com/c360studio/semteams/cmd/semteams/chain"
)

func buildSpecModeGate(_ *testing.T, pub TriplePublisher, entities chain.EntityTripleReader) *SpecModeGate {
	resolver := resolverWithChain(map[string]string{
		testLoopID: testRootID,
	})
	return NewSpecModeGate(pub, resolver, entities, testPlatformMeta(), nil)
}

func reviewerSpecEvent() *agentic.LoopCompletedEvent {
	return &agentic.LoopCompletedEvent{
		LoopID:  testLoopID,
		Role:    reviewerSpecRole,
		Outcome: agentic.OutcomeSuccess,
	}
}

func TestSpecModeGate_NonReviewerSpecIgnored(t *testing.T) {
	pub := &recordingPublisher{}
	g := buildSpecModeGate(t, pub, &fakeEntityReader{})
	for _, role := range []string{"builder", "reviewer-qa", "researcher-plan", "coordinator", ""} {
		ev := reviewerSpecEvent()
		ev.Role = role
		if err := g.HandleLoopCompleted(context.Background(), ev); err != nil {
			t.Errorf("role %q: %v", role, err)
		}
	}
	if len(pub.triples) != 0 {
		t.Errorf("non-reviewer-spec roles triggered %d writes", len(pub.triples))
	}
}

func TestSpecModeGate_NilEvent(t *testing.T) {
	pub := &recordingPublisher{}
	g := buildSpecModeGate(t, pub, &fakeEntityReader{})
	if err := g.HandleLoopCompleted(context.Background(), nil); err != nil {
		t.Errorf("nil event: %v", err)
	}
}

func TestSpecModeGate_NonApprovedSkips(t *testing.T) {
	pub := &recordingPublisher{}
	entities := &fakeEntityReader{
		entities: map[string]map[string]any{
			loopEntityID(testLoopID): {
				agvocab.CoordinatorNextAction: "insufficient",
			},
		},
	}
	g := buildSpecModeGate(t, pub, entities)
	if err := g.HandleLoopCompleted(context.Background(), reviewerSpecEvent()); err != nil {
		t.Fatalf("HandleLoopCompleted: %v", err)
	}
	if len(pub.triples) != 0 {
		t.Errorf("insufficient terminal should bypass gate; got %d writes", len(pub.triples))
	}
}

func TestSpecModeGate_ApprovedWithResearchArtifact(t *testing.T) {
	pub := &recordingPublisher{}
	entities := &fakeEntityReader{
		entities: map[string]map[string]any{
			loopEntityID(testLoopID): {
				agvocab.CoordinatorNextAction: approvedAction,
			},
			testChainEntityID: {
				chain.PredicateResearchArtifactLoop: "researcher_loop_id_xyz",
			},
		},
	}
	g := buildSpecModeGate(t, pub, entities)
	if err := g.HandleLoopCompleted(context.Background(), reviewerSpecEvent()); err != nil {
		t.Fatalf("HandleLoopCompleted: %v", err)
	}
	proceed, ok := pub.byPredicate(chain.PredicateSpecModeGateProceed)
	if !ok {
		t.Fatal("expected proceed sentinel")
	}
	if proceed.Subject != loopEntityID(testLoopID) {
		t.Errorf("proceed subject = %q, want loop entity", proceed.Subject)
	}
	if s, _ := proceed.Object.(string); s != "true" {
		t.Errorf("proceed object = %v, want \"true\"", proceed.Object)
	}
	if pub.hasPredicate(chain.PredicateSpecModeGateRejected) {
		t.Error("approval path should not stamp rejection")
	}
}

func TestSpecModeGate_RejectsMissingResearchArtifact(t *testing.T) {
	pub := &recordingPublisher{}
	entities := &fakeEntityReader{
		entities: map[string]map[string]any{
			loopEntityID(testLoopID): {
				agvocab.CoordinatorNextAction: approvedAction,
			},
			// chain entity intentionally has no research artifact loop.
		},
	}
	g := buildSpecModeGate(t, pub, entities)
	if err := g.HandleLoopCompleted(context.Background(), reviewerSpecEvent()); err != nil {
		t.Fatalf("HandleLoopCompleted: %v", err)
	}
	rejected, ok := pub.byPredicate(chain.PredicateSpecModeGateRejected)
	if !ok {
		t.Fatal("expected rejection marker")
	}
	if rejected.Subject != testChainEntityID {
		t.Errorf("rejection subject = %q, want chain entity", rejected.Subject)
	}
	reason, _ := pub.byPredicate(chain.PredicateSpecModeGateRejectReason)
	if s, _ := reason.Object.(string); s != specModeRejectMissingResearch {
		t.Errorf("reject reason = %v, want %q", reason.Object, specModeRejectMissingResearch)
	}
	if pub.hasPredicate(chain.PredicateSpecModeGateProceed) {
		t.Error("rejection should not stamp proceed sentinel")
	}
}

func TestSpecModeGate_ChainEntityReadErrorFailsClosed(t *testing.T) {
	// Loop-entity read succeeds (approved); resolver succeeds; chain-
	// entity read fails. Gate must fail-closed with no writes.
	base := &fakeEntityReader{
		entities: map[string]map[string]any{
			loopEntityID(testLoopID): {
				agvocab.CoordinatorNextAction: approvedAction,
			},
		},
	}
	pub := &recordingPublisher{}
	g := buildSpecModeGate(t, pub, &selectiveErrorReader{base: base, errorOn: testChainEntityID})
	if err := g.HandleLoopCompleted(context.Background(), reviewerSpecEvent()); err != nil {
		t.Fatalf("HandleLoopCompleted: %v", err)
	}
	if len(pub.triples) != 0 {
		t.Errorf("chain-entity read failure should write no triples; got %d", len(pub.triples))
	}
}

func TestSpecModeGate_EmptyResearchArtifactRejects(t *testing.T) {
	// Defensive: empty string for the predicate value counts as missing.
	pub := &recordingPublisher{}
	entities := &fakeEntityReader{
		entities: map[string]map[string]any{
			loopEntityID(testLoopID): {
				agvocab.CoordinatorNextAction: approvedAction,
			},
			testChainEntityID: {
				chain.PredicateResearchArtifactLoop: "",
			},
		},
	}
	g := buildSpecModeGate(t, pub, entities)
	if err := g.HandleLoopCompleted(context.Background(), reviewerSpecEvent()); err != nil {
		t.Fatalf("HandleLoopCompleted: %v", err)
	}
	if !pub.hasPredicate(chain.PredicateSpecModeGateRejected) {
		t.Error("empty research_artifact.loop should reject")
	}
}

// TestSpecModeGate_ForwardsSpecArtifactTriples asserts the gate
// propagates dev_via_spec.artifact.{path,slug} from the architect parent
// to the reviewer-spec loop entity on the approved path. The forward is
// required by rule 02-reviewer-approved-to-builder.json's
// $entity.triple.dev_via_spec.artifact.path substitution (the rule fires
// on reviewer-spec, not on architect — the substitution engine resolves
// triples against the triggering entity).
func TestSpecModeGate_ForwardsSpecArtifactTriples(t *testing.T) {
	const architectLoopID = "researcher_architect_x"
	architectEntityID := loopEntityID(architectLoopID)
	pub := &recordingPublisher{}
	entities := &fakeEntityReader{
		entities: map[string]map[string]any{
			loopEntityID(testLoopID): {
				agvocab.CoordinatorNextAction: approvedAction,
				agentLoopParentPredicate:      architectEntityID,
			},
			architectEntityID: {
				predicateDevViaSpecArtifactPath: "docs/specs/abc.md",
				predicateDevViaSpecArtifactSlug: "abc",
			},
			testChainEntityID: {
				chain.PredicateResearchArtifactLoop: architectLoopID,
			},
		},
	}
	g := buildSpecModeGate(t, pub, entities)
	if err := g.HandleLoopCompleted(context.Background(), reviewerSpecEvent()); err != nil {
		t.Fatalf("HandleLoopCompleted: %v", err)
	}

	path, ok := pub.byPredicate(predicateDevViaSpecArtifactPath)
	if !ok {
		t.Fatal("expected dev_via_spec.artifact.path write on reviewer-spec entity")
	}
	if path.Subject != loopEntityID(testLoopID) {
		t.Errorf("path subject = %q, want reviewer-spec entity %q", path.Subject, loopEntityID(testLoopID))
	}
	if s, _ := path.Object.(string); s != "docs/specs/abc.md" {
		t.Errorf("path object = %v, want \"docs/specs/abc.md\"", path.Object)
	}

	slug, ok := pub.byPredicate(predicateDevViaSpecArtifactSlug)
	if !ok {
		t.Fatal("expected dev_via_spec.artifact.slug write on reviewer-spec entity")
	}
	if slug.Subject != loopEntityID(testLoopID) {
		t.Errorf("slug subject = %q, want reviewer-spec entity", slug.Subject)
	}
	if s, _ := slug.Object.(string); s != "abc" {
		t.Errorf("slug object = %v, want \"abc\"", slug.Object)
	}

	if !pub.hasPredicate(chain.PredicateSpecModeGateProceed) {
		t.Error("approved path must still write proceed sentinel alongside forwarded triples")
	}
}

// TestSpecModeGate_MissingParentSkipsForward asserts the gate fails-soft
// on a missing agent.loop.parent: proceed sentinel still lands so the
// other gate behaviours stay testable independently, but no forwarded
// triples are written (operator logs surface the warning). Rule 02's
// substitution will return the literal token at runtime; builder bootstrap
// then fails visibly rather than the gate silently masking the issue.
func TestSpecModeGate_MissingParentSkipsForward(t *testing.T) {
	pub := &recordingPublisher{}
	entities := &fakeEntityReader{
		entities: map[string]map[string]any{
			loopEntityID(testLoopID): {
				agvocab.CoordinatorNextAction: approvedAction,
				// no agent.loop.parent
			},
			testChainEntityID: {
				chain.PredicateResearchArtifactLoop: "researcher_xyz",
			},
		},
	}
	g := buildSpecModeGate(t, pub, entities)
	if err := g.HandleLoopCompleted(context.Background(), reviewerSpecEvent()); err != nil {
		t.Fatalf("HandleLoopCompleted: %v", err)
	}
	if pub.hasPredicate(predicateDevViaSpecArtifactPath) {
		t.Error("missing parent should not stamp spec.path on reviewer entity")
	}
	if !pub.hasPredicate(chain.PredicateSpecModeGateProceed) {
		t.Error("proceed sentinel should still land regardless of forward outcome")
	}
}

// TestSpecModeGate_MissingArchitectTriplesSkipsForward asserts the
// defensive log path when the architect parent exists but doesn't carry
// the artifact triples. Same soft-fail semantics as missing parent.
func TestSpecModeGate_MissingArchitectTriplesSkipsForward(t *testing.T) {
	const architectLoopID = "researcher_architect_x"
	architectEntityID := loopEntityID(architectLoopID)
	pub := &recordingPublisher{}
	entities := &fakeEntityReader{
		entities: map[string]map[string]any{
			loopEntityID(testLoopID): {
				agvocab.CoordinatorNextAction: approvedAction,
				agentLoopParentPredicate:      architectEntityID,
			},
			architectEntityID: {
				// no path/slug — architect didn't emit (or triples haven't landed yet)
			},
			testChainEntityID: {
				chain.PredicateResearchArtifactLoop: architectLoopID,
			},
		},
	}
	g := buildSpecModeGate(t, pub, entities)
	if err := g.HandleLoopCompleted(context.Background(), reviewerSpecEvent()); err != nil {
		t.Fatalf("HandleLoopCompleted: %v", err)
	}
	if pub.hasPredicate(predicateDevViaSpecArtifactPath) {
		t.Error("absent architect triples should not stamp spec.path")
	}
	if pub.hasPredicate(predicateDevViaSpecArtifactSlug) {
		t.Error("absent architect triples should not stamp spec.slug")
	}
	if !pub.hasPredicate(chain.PredicateSpecModeGateProceed) {
		t.Error("proceed sentinel should still land regardless of forward outcome")
	}
}
