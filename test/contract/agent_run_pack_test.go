package contract

import (
	"encoding/json"
	"os"
	"strings"
	"testing"
)

// ADR-053 Phase 4a — structural contract for the agent-run substrate rule pack.
//
// These are the design invariants the architect + Coby reviews made
// load-bearing (docs/adr/053-adoption-plan.md §Phase 4 design spike). They are
// CHEAP structural pins; the state-machine SAFETY (marker-before-executing,
// duplicate terminal, restart mid-run, publish-fail-after-mint) is proven by the
// e2e journeys + failure-injection tests, not here.

type ruleDoc struct {
	ID         string `json:"id"`
	Conditions []struct {
		Field    string `json:"field"`
		Operator string `json:"operator"`
		Value    any    `json:"value"`
	} `json:"conditions"`
	OnEnter []struct {
		Type         string            `json:"type"`
		Subject      string            `json:"subject"`
		Predicate    string            `json:"predicate"`
		Object       any               `json:"object"`
		Workflow     string            `json:"workflow"`
		Phase        string            `json:"phase"`
		When         json.RawMessage   `json:"when"`
		RelatedLoops map[string]string `json:"related_loops"`
	} `json:"on_enter"`
}

func loadRule(t *testing.T, path string) ruleDoc {
	t.Helper()
	raw, err := os.ReadFile(path) //nolint:gosec // test-controlled config path
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	var r ruleDoc
	if err := json.Unmarshal(raw, &r); err != nil {
		t.Fatalf("unmarshal %s: %v", path, err)
	}
	return r
}

func (r ruleDoc) hasCondition(field, op string, value any) bool {
	for _, c := range r.Conditions {
		if c.Field == field && c.Operator == op && c.Value == value {
			return true
		}
	}
	return false
}

// TestAgentRunPack_HandoffMarker pins rule 01: it bridges the firing-entity gap
// by firing on the coordinator loop (confirmed handoff) and stamping
// agent.run.handoff on the run entity. The rule.spawned_task gate (not bare
// mint) is Coby review P1a — publish-failure safety.
func TestAgentRunPack_HandoffMarker(t *testing.T) {
	r := loadRule(t, "../../configs/rules/agent-run/01-handoff-marker.json")

	if !r.hasCondition("agent.loop.role", "eq", "coordinator") {
		t.Error("handoff marker must fire on the coordinator loop (agent.loop.role==coordinator)")
	}
	// rule.spawned_task is the CONFIRMED-HANDOFF gate (post-publish-success),
	// NOT bare mint — the load-bearing P1a invariant.
	if !r.hasCondition("rule.spawned_task", "ne", "") {
		t.Error("handoff marker must gate on rule.spawned_task != \"\" (confirmed handoff, Coby review P1a) — NOT on agent.run.phase==dispatched (bare mint, which survives a publish failure)")
	}
	// agent.run.entity_id presence is the run-less-chat guard.
	if !r.hasCondition("agent.run.entity_id", "ne", "") {
		t.Error("handoff marker must gate on agent.run.entity_id != \"\" (run-less-chat guard)")
	}
	// Must NOT trigger on bare mint.
	if r.hasCondition("agent.run.phase", "eq", "dispatched") {
		t.Error("handoff marker must NOT gate on agent.run.phase==dispatched — that is bare-mint triggering (P1a regression)")
	}

	var stampsHandoff bool
	for _, a := range r.OnEnter {
		if a.Type == "add_triple" && a.Predicate == "agent.run.handoff" {
			stampsHandoff = true
			if a.Subject != "$entity.triple.agent.run.entity_id" {
				t.Errorf("handoff stamp subject = %q, want $entity.triple.agent.run.entity_id (the mint-stamped run anchor on the coordinator)", a.Subject)
			}
		}
	}
	if !stampsHandoff {
		t.Error("handoff marker must add_triple agent.run.handoff on the run entity")
	}
}

// TestAgentRunPack_TransitionsPhaseGuardedTopLevel pins the load-bearing Coby
// review round-2 invariant: the phase guard is a TOP-LEVEL CONDITION, never an
// action `when`. A `when`-buried phase guard would let the rule enter on the
// outcome alone, skip the action while in the wrong phase, and never re-enter.
func TestAgentRunPack_TransitionsPhaseGuardedTopLevel(t *testing.T) {
	cases := []struct {
		path      string
		fromPhase string // top-level phase guard
		toPhase   string // lifecycle_transition target
		trigger   string // the other top-level condition
	}{
		{"../../configs/rules/agent-run/02-dispatched-to-executing.json", "dispatched", "executing", "agent.run.handoff"},
		{"../../configs/rules/agent-run/03-executing-to-completed.json", "executing", "completed", "agent.run.outcome"},
	}
	for _, tc := range cases {
		r := loadRule(t, tc.path)

		// Phase guard is a TOP-LEVEL CONDITION.
		if !r.hasCondition("agent.run.phase", "eq", tc.fromPhase) {
			t.Errorf("%s: agent.run.phase==%s must be a TOP-LEVEL condition (Coby review round 2 — never an action `when`)", tc.path, tc.fromPhase)
		}
		// The driving trigger is also a top-level condition.
		var hasTrigger bool
		for _, c := range r.Conditions {
			if c.Field == tc.trigger {
				hasTrigger = true
			}
		}
		if !hasTrigger {
			t.Errorf("%s: missing top-level trigger condition on %s", tc.path, tc.trigger)
		}

		// Exactly one lifecycle_transition to the right phase, workflow=agent-run,
		// and NO action carries a `when` (the phase guard must not leak into when).
		var transitions int
		for _, a := range r.OnEnter {
			if len(a.When) > 0 {
				t.Errorf("%s: on_enter action carries a `when` — the phase guard must be a top-level condition, not a `when` (Coby review round 2)", tc.path)
			}
			if a.Type == "lifecycle_transition" {
				transitions++
				if a.Workflow != "agent-run" {
					t.Errorf("%s: lifecycle_transition workflow = %q, want agent-run", tc.path, a.Workflow)
				}
				if a.Phase != tc.toPhase {
					t.Errorf("%s: lifecycle_transition phase = %q, want %s", tc.path, a.Phase, tc.toPhase)
				}
			}
		}
		if transitions != 1 {
			t.Errorf("%s: want exactly 1 lifecycle_transition action, got %d", tc.path, transitions)
		}
	}
}

// TestAgentRunPack_SuccessOutcomeStamps pins that the three reviewer/CBG-approved
// terminal rules stamp agent.run.outcome=success on the run entity, each with the
// per-pack run-anchor subject (research uses agent.run.entity_id; autoresearch +
// dev-via-test descend through run-entity-fired rules so carry only
// lineage.run-loop-entity-id). This is the §E per-pack-anchor correction.
func TestAgentRunPack_SuccessOutcomeStamps(t *testing.T) {
	cases := []struct {
		path    string
		subject string
	}{
		{"../../configs/rules/research/07-reviewer-approved-to-coordinator.json", "$entity.triple.agent.run.entity_id"},
		{"../../configs/rules/autoresearch/08-reviewer-approved-to-coordinator.json", "$entity.triple.lineage.run-loop-entity-id"},
		{"../../configs/rules/dev-via-test/07a-cbg-approved-to-coordinator.json", "$entity.triple.lineage.run-loop-entity-id"},
	}
	for _, tc := range cases {
		r := loadRule(t, tc.path)
		var found bool
		for _, a := range r.OnEnter {
			if a.Type == "add_triple" && a.Predicate == "agent.run.outcome" {
				found = true
				if obj, _ := a.Object.(string); obj != "success" {
					t.Errorf("%s: agent.run.outcome object = %v, want \"success\"", tc.path, a.Object)
				}
				if a.Subject != tc.subject {
					t.Errorf("%s: agent.run.outcome subject = %q, want %q (per-pack run anchor — §E)", tc.path, a.Subject, tc.subject)
				}
			}
		}
		if !found {
			t.Errorf("%s: must stamp agent.run.outcome=success on the run entity (drives executing→completed)", tc.path)
		}
	}
}

// TestAgentRunPack_SuccessStampAnchorThreaded is the SEMANTIC pin that the
// per-rule subject check (TestAgentRunPack_SuccessOutcomeStamps) cannot give: a
// `$entity.triple.lineage.<key>` stamp subject only resolves if the rule that
// SPAWNED the stamping loop threads <key> in its related_loops. Without it the
// substitution leaves a literal token and the run never completes.
//
// go-reviewer C1/C2: autoresearch/08 stamped lineage.run-loop-entity-id, but its
// spawner autoresearch/07 threaded only autoresearch-run — so the subject resolved
// to a garbage literal and every autoresearch run hung in `executing`. The
// per-rule subject test passed green because it pinned the author's intent string,
// not that the anchor resolves on the firing role. This test closes that gap.
//
// research/07 uses agent.run.entity_id (framework-inherited via the loop-spawn
// chain, NOT a related_loops thread), so it is not checked here.
func TestAgentRunPack_SuccessStampAnchorThreaded(t *testing.T) {
	cases := []struct {
		pack       string
		spawnRule  string // the rule that spawns the loop the success stamp fires on
		lineageKey string // the related_loops key the stamp's lineage.<key> subject reads
	}{
		{"autoresearch", "../../configs/rules/autoresearch/07-synthesize-to-reviewer.json", "run-loop-entity-id"},
		{"dev-via-test", "../../configs/rules/dev-via-test/06-coordinator-dispatch-cbg.json", "run-loop-entity-id"},
	}
	for _, tc := range cases {
		r := loadRule(t, tc.spawnRule)
		var threaded bool
		for _, a := range r.OnEnter {
			if _, ok := a.RelatedLoops[tc.lineageKey]; ok {
				threaded = true
				break
			}
		}
		if !threaded {
			t.Errorf("%s: spawn rule %s must thread related_loops[%q] so the downstream success stamp's "+
				"$entity.triple.lineage.%s subject resolves on the terminal loop (else it writes to a literal "+
				"token and the run never completes — go-reviewer C1)", tc.pack, tc.spawnRule, tc.lineageKey, tc.lineageKey)
		}
	}
}

// TestAgentRunPack_WiredInBothConfigs pins that all three agent-run rules are
// loaded by BOTH the production and e2e bootstrap configs — a Phase-4a rule that
// ships in prod but not e2e (or vice-versa) would pass mock journeys yet break in
// production, the structural-wiring gap class.
func TestAgentRunPack_WiredInBothConfigs(t *testing.T) {
	wantFiles := []string{
		"/app/configs/rules/agent-run/01-handoff-marker.json",
		"/app/configs/rules/agent-run/02-dispatched-to-executing.json",
		"/app/configs/rules/agent-run/03-executing-to-completed.json",
	}
	for _, cfg := range []string{"../../configs/flow-bootstrap.json", "../../configs/e2e-flow-bootstrap.json"} {
		raw, err := os.ReadFile(cfg) //nolint:gosec // test-controlled config path
		if err != nil {
			t.Fatalf("read %s: %v", cfg, err)
		}
		s := string(raw)
		for _, f := range wantFiles {
			if !strings.Contains(s, f) {
				t.Errorf("%s: missing agent-run rule %q from rules_files", cfg, f)
			}
		}
	}
}
