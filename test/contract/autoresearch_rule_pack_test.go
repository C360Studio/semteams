package contract

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Autoresearch pack contract tests — structural invariants the rule
// JSON must hold for the pack to compose correctly with the
// substrate. Walks configs/rules/autoresearch/ at test time; no
// runtime rule engine needed.
//
// The pack is the second category pack on the substrate-plus-overlays
// MVP (ADR-042). Tests here are the structural-correctness gate the
// design review (2026-05-29) flagged for the mutual-exclusion claim
// between rules 05 and 06.

const autoresearchPackDir = "../../configs/rules/autoresearch"

// autoresearchRuleJSON mirrors researchRuleJSON's shape, extended
// with condition.operator and condition.value (needed to assert the
// length_lt / length_eq pairing that makes rules 05 and 06 mutually
// exclusive).
type autoresearchRuleJSON struct {
	ID         string                      `json:"id"`
	Type       string                      `json:"type"`
	Enabled    bool                        `json:"enabled"`
	Conditions []autoresearchConditionJSON `json:"conditions"`
	OnEnter    []autoresearchOnEnterJSON   `json:"on_enter"`
	Metadata   map[string]any              `json:"metadata"`
}

type autoresearchConditionJSON struct {
	Field    string `json:"field"`
	Operator string `json:"operator"`
	Value    any    `json:"value"`
	Required bool   `json:"required"`
}

type autoresearchOnEnterJSON struct {
	Type          string                      `json:"type"`
	Role          string                      `json:"role,omitempty"`
	Subject       string                      `json:"subject,omitempty"`
	Predicate     string                      `json:"predicate,omitempty"`
	Object        any                         `json:"object,omitempty"`
	ActionAllowed []string                    `json:"action_allowlist,omitempty"`
	When          []autoresearchConditionJSON `json:"when,omitempty"`
}

// TestAutoresearchPack_05_IterationDispatch_PatternA pins the
// presence-marker iteration-loop pattern (semstreams#204 / R4
// canonical form, beta.95+). The old rule 02 + 05 + 06 trio was
// retired in favor of a single rule 05 that handles iter 1 → cap
// via a presence-marker (`autoresearch.iteration.pending`)
// triggered remove-then-add cycle.
//
// Why this exists: the rule engine fires `on_enter` actions only on
// false→true transitions. Monotonic conditions (e.g. count<cap)
// stay true across iterations 2..N, so `wasMatching` stays true and
// the next iteration's marker stamp would otherwise produce
// `TransitionNone` (empty WhileTrue, action_count=0, chain stalls).
// The remove_triple in this rule's first on_enter action flips the
// trigger condition false (Exited), resetting wasMatching for the
// next Entered cycle.
//
// Without these invariants, the iteration loop can't fire past
// iter 1 (proven empirically across four reverted attempts in June
// 2026; see semteams 7c82b95 / edf2c5b / cd094727 / 2be0e9f).
//
// This test enforces:
//
//  1. Rule conditions include a presence marker on
//     `autoresearch.iteration.pending ne ""` — without it the
//     remove-then-add cycle has nothing to remove.
//  2. The first on_enter action is `remove_triple` on the marker —
//     ordering matters: clearing the marker before the spawn
//     actions is what produces the Exited transition.
//  3. A publish_agent for autoresearch-propose exists with a `when`
//     clause `$state.iteration <= $entity.triple.autoresearch.cap`.
//  4. A publish_agent for autoresearch-synthesize exists with a
//     `when` clause `$state.iteration > $entity.triple.autoresearch.cap`
//     — the cap-exhaust branch.
//  5. An update_triple (NOT add_triple) for autoresearch.run.status
//     = "stopped" with the same cap-exhaust when clause. update_triple
//     wipes the prior "active" value; add_triple would leave both,
//     and GetFieldValue's first-wins read would still find "active",
//     defeating the run.status=active gate's defense-in-depth.
func TestAutoresearchPack_05_IterationDispatch_PatternA(t *testing.T) {
	rule, err := loadAutoresearchRule(t, "05-iteration-dispatch.json")
	if err != nil {
		t.Fatalf("load rule 05: %v", err)
	}

	// Invariant 1: presence-marker trigger.
	marker := findCondition(rule, "autoresearch.iteration.pending")
	if marker == nil {
		t.Fatal("rule 05 missing autoresearch.iteration.pending condition — the marker trigger is the whole basis of Pattern A")
	}
	if marker.Operator != "ne" {
		t.Errorf("rule 05 iteration.pending operator = %q; want %q (presence check, not equality on a specific value)",
			marker.Operator, "ne")
	}
	if got, ok := marker.Value.(string); !ok || got != "" {
		t.Errorf("rule 05 iteration.pending compare value = %v; want empty string (presence test)", marker.Value)
	}

	// Invariant 2: first on_enter action must be remove_triple on the
	// marker. Ordering is load-bearing — clearing the marker must
	// happen before the spawn actions so the rule's conditions flip
	// false (Exited) and the next iteration's marker stamp produces
	// a fresh Entered. If a spawn fires first, the propose loop could
	// race against the marker still being present.
	if len(rule.OnEnter) == 0 || rule.OnEnter[0].Type != "remove_triple" ||
		rule.OnEnter[0].Predicate != "autoresearch.iteration.pending" {
		t.Errorf("rule 05 first on_enter action = %+v; want remove_triple of autoresearch.iteration.pending so the Exited transition fires before spawn actions",
			ifFirstOnEnter(rule))
	}

	// Invariant 3 + 4: propose and synthesize publish_agent actions
	// with matching when clauses on $state.iteration vs cap.
	wantCap := "$entity.triple.autoresearch.cap"
	var foundProposeWhen, foundSynthesizeWhen bool
	for _, a := range rule.OnEnter {
		if a.Type != "publish_agent" {
			continue
		}
		switch a.Role {
		case "autoresearch-propose":
			foundProposeWhen = whenIterationCap(a.When, "lte", wantCap)
			if !foundProposeWhen {
				t.Errorf("rule 05 propose spawn missing when {$state.iteration lte %s}; current when = %+v", wantCap, a.When)
			}
		case "autoresearch-synthesize":
			foundSynthesizeWhen = whenIterationCap(a.When, "gt", wantCap)
			if !foundSynthesizeWhen {
				t.Errorf("rule 05 synthesize spawn missing when {$state.iteration gt %s} (cap-exhaust branch); current when = %+v", wantCap, a.When)
			}
		}
	}
	if !foundProposeWhen {
		t.Error("rule 05 missing publish_agent for autoresearch-propose with the per-iteration when clause")
	}
	if !foundSynthesizeWhen {
		t.Error("rule 05 missing publish_agent for autoresearch-synthesize with the cap-exhaust when clause — chain would loop past cap forever")
	}

	// Invariant 5: update_triple (NOT add_triple) for the run.status
	// stop flip, gated by the same cap-exhaust when clause.
	var foundStopFlip bool
	for _, a := range rule.OnEnter {
		if a.Type != "update_triple" || a.Predicate != "autoresearch.run.status" {
			continue
		}
		if obj, ok := a.Object.(string); !ok || obj != "stopped" {
			t.Errorf("rule 05 run.status update_triple object = %v; want %q so the status condition correctly fails on belated state changes",
				a.Object, "stopped")
		}
		if !whenIterationCap(a.When, "gt", wantCap) {
			t.Errorf("rule 05 run.status flip missing when {$state.iteration gt %s}; current when = %+v", wantCap, a.When)
		}
		foundStopFlip = true
	}
	if !foundStopFlip {
		t.Error("rule 05 missing update_triple for autoresearch.run.status='stopped' at cap-exhaust — without it, a belated execute finishing after synthesize spawned would re-trigger the rule and spawn a duplicate synthesize")
	}

	// add_triple on run.status would be a regression: append-only
	// stacking means GetFieldValue's first-wins read still finds the
	// original "active" value, defeating the gate. Explicitly forbid.
	for _, a := range rule.OnEnter {
		if a.Type == "add_triple" && a.Predicate == "autoresearch.run.status" {
			t.Errorf("rule 05 uses add_triple on autoresearch.run.status; must be update_triple — add_triple appends and GetFieldValue's first-wins read would keep returning the original 'active' value")
		}
	}

	// Invariant 4 (carry-over from old test): run.status=active gate
	// stays as defense-in-depth.
	if !hasActiveStatusGate(rule) {
		t.Error("rule 05 missing autoresearch.run.status=active gate — defense-in-depth against firing after the cap-exhaust stop flip")
	}
}

func ifFirstOnEnter(r *autoresearchRuleJSON) any {
	if len(r.OnEnter) == 0 {
		return nil
	}
	return r.OnEnter[0]
}

func whenIterationCap(when []autoresearchConditionJSON, operator, capRef string) bool {
	for _, w := range when {
		if w.Field == "$state.iteration" && w.Operator == operator {
			if s, ok := w.Value.(string); ok && s == capRef {
				return true
			}
		}
	}
	return false
}

// TestAutoresearchPack_04a_04b_OutcomeCoverage pins the C1 fix from
// the design review (2026-05-29): rule 04a fires on clean execute
// completion (outcome=success + decide=measured); rule 04b fires on
// loop-failed executes (outcome=failed). Together they stamp
// experiment.completed for every distinct execute terminal, so the
// cap budget honestly bounds iterations including failures.
//
// Without this pairing, a failed execute would never increment the
// counter; the chain would wedge with the counter stuck at N-1 while
// rule 11 stamped chain.paused.marker. The fix relies on both rules
// existing AND both stamping the same predicate on the same subject.
func TestAutoresearchPack_04a_04b_OutcomeCoverage(t *testing.T) {
	rule04a, err := loadAutoresearchRule(t, "04a-execute-stamp-completion.json")
	if err != nil {
		t.Fatalf("load rule 04a: %v", err)
	}
	rule04b, err := loadAutoresearchRule(t, "04b-execute-stamp-failed.json")
	if err != nil {
		t.Fatalf("load rule 04b: %v", err)
	}

	// Both rules must condition on autoresearch-execute role.
	if !roleCondition(rule04a, "autoresearch-execute") {
		t.Error("rule 04a does not condition on agent.loop.role = autoresearch-execute")
	}
	if !roleCondition(rule04b, "autoresearch-execute") {
		t.Error("rule 04b does not condition on agent.loop.role = autoresearch-execute")
	}

	// Rule 04a must condition on outcome=success.
	if !outcomeCondition(rule04a, "success") {
		t.Error("rule 04a does not condition on agent.loop.outcome = success — clean-completion half cannot fire")
	}

	// Rule 04b must condition on outcome=failed.
	if !outcomeCondition(rule04b, "failed") {
		t.Error("rule 04b does not condition on agent.loop.outcome = failed — loop-failed half cannot fire; counter loses failed iterations")
	}

	// Both rules must stamp experiment.completed via add_triple on
	// $entity.triple.lineage.run-loop-entity-id. This is the cap-
	// budget counter; without both rules stamping it, failed
	// executes don't count.
	if !stampsExperimentCompleted(rule04a) {
		t.Error("rule 04a does not stamp autoresearch.experiment.completed on the run entity")
	}
	if !stampsExperimentCompleted(rule04b) {
		t.Error("rule 04b does not stamp autoresearch.experiment.completed on the run entity")
	}

	// Rule 04b additionally stamps experiment.loop_failed so the
	// SYNTHESIZE rollup can distinguish clean iterations from
	// loop-failed ones in the journey rendering.
	if !stampsPredicateOnRun(rule04b, "autoresearch.experiment.loop_failed") {
		t.Error("rule 04b does not stamp autoresearch.experiment.loop_failed — SYNTHESIZE cannot distinguish loop-failed iterations from cleanly-measured ones")
	}

	// Pattern A (semstreams#204 marker-based iteration loop): both
	// 04a and 04b must stamp autoresearch.iteration.pending on the
	// run entity alongside experiment.completed. The new marker
	// re-triggers rule 05's Entered cycle for the next iteration.
	// Without this stamp, rule 05 never re-fires after iter 1 (the
	// monotonic-condition trap that motivated the entire pattern).
	if !stampsPredicateOnRun(rule04a, "autoresearch.iteration.pending") {
		t.Error("rule 04a does not stamp autoresearch.iteration.pending — rule 05's iteration loop cannot advance past iter 1")
	}
	if !stampsPredicateOnRun(rule04b, "autoresearch.iteration.pending") {
		t.Error("rule 04b does not stamp autoresearch.iteration.pending — loop-failed executes cannot advance the iteration counter past iter 1")
	}
}

// TestAutoresearchPack_11_ExcludesExecute pins the second half of
// the C1 fix: rule 11 (loop-failed-pause) must NOT include
// autoresearch-execute in its role list, because execute loop-
// failures are counted by rule 04b instead of paused. Including
// autoresearch-execute here means a failed execute would BOTH
// increment the counter AND stamp chain.paused.marker, leading to
// observability confusion and a wedged chain even when the budget
// could continue.
func TestAutoresearchPack_11_ExcludesExecute(t *testing.T) {
	rule11, err := loadAutoresearchRule(t, "11-loop-failed-pause.json")
	if err != nil {
		t.Fatalf("load rule 11: %v", err)
	}

	for _, c := range rule11.Conditions {
		if c.Field != "agent.loop.role" || c.Operator != "in" {
			continue
		}
		roles, ok := c.Value.([]any)
		if !ok {
			t.Fatalf("rule 11 agent.loop.role condition value is not an array: %T", c.Value)
		}
		for _, r := range roles {
			s, ok := r.(string)
			if !ok {
				continue
			}
			if s == "autoresearch-execute" {
				t.Error("rule 11 includes autoresearch-execute in its role list — must be excluded so execute loop-failures route through rule 04b (counted + continue) rather than pausing the chain. See reviewer C1 fix 2026-05-29.")
			}
		}
		// Sanity check: the remaining four roles MUST be present so
		// failures in baseline / propose / synthesize / reviewer DO
		// still pause (those have no in-arc recovery path).
		needed := map[string]bool{
			"autoresearch-baseline":   false,
			"autoresearch-propose":    false,
			"autoresearch-synthesize": false,
			"reviewer-autoresearch":   false,
		}
		for _, r := range roles {
			s, _ := r.(string)
			if _, ok := needed[s]; ok {
				needed[s] = true
			}
		}
		for role, present := range needed {
			if !present {
				t.Errorf("rule 11 missing required role %q — loop-failures in this role need to pause the chain (no in-arc recovery)", role)
			}
		}
		return
	}
	t.Error("rule 11 has no agent.loop.role condition with `in` operator")
}

// --- helpers ---

func loadAutoresearchRule(t *testing.T, name string) (*autoresearchRuleJSON, error) {
	t.Helper()
	path := filepath.Join(autoresearchPackDir, name)
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", path, err)
	}
	var r autoresearchRuleJSON
	if err := json.Unmarshal(data, &r); err != nil {
		return nil, fmt.Errorf("unmarshal %s: %w", path, err)
	}
	return &r, nil
}

func findCondition(r *autoresearchRuleJSON, field string) *autoresearchConditionJSON {
	for i := range r.Conditions {
		if r.Conditions[i].Field == field {
			return &r.Conditions[i]
		}
	}
	return nil
}

func hasActiveStatusGate(r *autoresearchRuleJSON) bool {
	for _, c := range r.Conditions {
		if c.Field == "autoresearch.run.status" && c.Operator == "eq" {
			if s, ok := c.Value.(string); ok && s == "active" {
				return true
			}
		}
	}
	return false
}

func roleCondition(r *autoresearchRuleJSON, role string) bool {
	for _, c := range r.Conditions {
		if c.Field == "agent.loop.role" && c.Operator == "eq" {
			if s, ok := c.Value.(string); ok && s == role {
				return true
			}
		}
	}
	return false
}

func outcomeCondition(r *autoresearchRuleJSON, outcome string) bool {
	for _, c := range r.Conditions {
		if c.Field == "agent.loop.outcome" && c.Operator == "eq" {
			if s, ok := c.Value.(string); ok && s == outcome {
				return true
			}
		}
	}
	return false
}

func stampsExperimentCompleted(r *autoresearchRuleJSON) bool {
	return stampsPredicateOnRun(r, "autoresearch.experiment.completed")
}

func stampsPredicateOnRun(r *autoresearchRuleJSON, predicate string) bool {
	for _, a := range r.OnEnter {
		if a.Type == "add_triple" && a.Predicate == predicate &&
			strings.Contains(a.Subject, "lineage.run-loop-entity-id") {
			return true
		}
	}
	return false
}
