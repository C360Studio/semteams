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
	Type          string   `json:"type"`
	Role          string   `json:"role,omitempty"`
	Subject       string   `json:"subject,omitempty"`
	Predicate     string   `json:"predicate,omitempty"`
	ActionAllowed []string `json:"action_allowlist,omitempty"`
}

// TestAutoresearchPack_05_06_MutualExclusion pins the structural
// claim from the autoresearch pack README §"Iteration counter
// accounting": rule 05 (continue) and rule 06 (stop-cap) must fire
// mutually exclusively at any single entity-state snapshot. The
// design-review pass (2026-05-29 H3) called this out: the metadata
// claims mutual exclusion; the structure must enforce it.
//
// The mathematical basis: rule 05 conditions on
// `autoresearch.experiment.completed length_lt cap` and rule 06 on
// `autoresearch.experiment.completed length_eq cap`. For any
// length value L, exactly one of (L < cap) or (L == cap) is true
// (assuming L <= cap, which the chain structurally guarantees: rule
// 04a/04b are the only stamps, and rule 06's status=stopped flip
// blocks further stamps).
//
// This test enforces:
//
//  1. Both rules condition on the same predicate
//     (`autoresearch.experiment.completed`).
//  2. The operator pair is (length_lt, length_eq) — not (length_lt,
//     length_lte) or any other combination that would overlap.
//  3. Both reference the same cap value
//     ($entity.triple.autoresearch.cap) — divergent cap references
//     would silently break mutual exclusion.
//  4. Both gate on autoresearch.run.status=active — defense in
//     depth against belated counter changes after rule 06 has
//     flipped the status to stopped.
//
// What this test does NOT cover: runtime evaluation-order races
// in the rule engine itself (e.g. whether the engine could fire
// rule 05 against a stale snapshot showing length < cap while
// the live snapshot shows length == cap). That belongs to the
// §A foundation PR's integration suite. This test only verifies
// the rule JSON is structurally capable of mutual exclusion;
// runtime correctness is the engine's job and is independently
// covered.
func TestAutoresearchPack_05_06_MutualExclusion(t *testing.T) {
	rule05, err := loadAutoresearchRule(t, "05-experiment-continue.json")
	if err != nil {
		t.Fatalf("load rule 05: %v", err)
	}
	rule06, err := loadAutoresearchRule(t, "06-experiment-stop-cap.json")
	if err != nil {
		t.Fatalf("load rule 06: %v", err)
	}

	c05 := findCondition(rule05, "autoresearch.experiment.completed")
	if c05 == nil {
		t.Fatal("rule 05 has no condition on autoresearch.experiment.completed — mutual exclusion broken at the predicate layer")
	}
	c06 := findCondition(rule06, "autoresearch.experiment.completed")
	if c06 == nil {
		t.Fatal("rule 06 has no condition on autoresearch.experiment.completed — mutual exclusion broken at the predicate layer")
	}

	// Invariant 2: operator pair must be (length_lt, length_eq).
	if c05.Operator != "length_lt" {
		t.Errorf("rule 05 operator on experiment.completed = %q; want %q so the continue path fires strictly when L < cap",
			c05.Operator, "length_lt")
	}
	if c06.Operator != "length_eq" {
		t.Errorf("rule 06 operator on experiment.completed = %q; want %q so the stop path fires strictly when L == cap",
			c06.Operator, "length_eq")
	}

	// Invariant 3: both must reference the same cap substitution.
	wantCap := "$entity.triple.autoresearch.cap"
	if got, ok := c05.Value.(string); !ok || got != wantCap {
		t.Errorf("rule 05 cap reference = %v; want %q so cap is data-driven and aligned with rule 06",
			c05.Value, wantCap)
	}
	if got, ok := c06.Value.(string); !ok || got != wantCap {
		t.Errorf("rule 06 cap reference = %v; want %q so cap is data-driven and aligned with rule 05",
			c06.Value, wantCap)
	}

	// Invariant 4: both must gate on autoresearch.run.status=active.
	if !hasActiveStatusGate(rule05) {
		t.Error("rule 05 missing autoresearch.run.status=active gate — defense-in-depth against rule 06 flipping status to stopped")
	}
	if !hasActiveStatusGate(rule06) {
		t.Error("rule 06 missing autoresearch.run.status=active gate — defense-in-depth against re-firing after status flip")
	}
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
