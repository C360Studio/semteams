package contract

import (
	"encoding/json"
	"os"
	"testing"
)

// ruleWithConditions is the minimum shape we need to inspect the
// reviewer-rejected-retry rule's conditions block. The full rule schema
// is more permissive; we look only at the fields the chain-recovery cap
// depends on.
type ruleWithConditions struct {
	ID         string `json:"id"`
	Conditions []struct {
		Field    string `json:"field"`
		Operator string `json:"operator"`
		Value    any    `json:"value"`
		Required bool   `json:"required"`
	} `json:"conditions"`
}

// TestRecoveryCapRule_GatesOnProceedSentinel pins the contract that
// research-mode-transition/02-reviewer-rejected-retry-research.json
// gates researcher respawns on the chain.recovery.proceed sentinel
// written by cmd/semteams/recoverycounter onto the triggering reviewer
// entity when the chain has remaining recovery budget. Absence of
// proceed → rule never fires → chain stalls fail-safe.
// ADR-040 §addendum 2026-05-11 + ADR-041 Slice 2D-3 (refit to MVP
// reviewer-{research,spec} roles after curator role was retired).
//
// The Counter and the rule fire on the same agent.complete event;
// Counter writes the proceed sentinel after its chain-walk and cap
// check; the KV update triggers rule re-eval; rule fires. Renames on
// either side break this test before a silently-decoupled cap reaches
// a real-LLM smoke.
func TestRecoveryCapRule_GatesOnProceedSentinel(t *testing.T) {
	const rulePath = "../../configs/rules/research-mode-transition/02-reviewer-rejected-retry-research.json"
	data, err := os.ReadFile(rulePath)
	if err != nil {
		t.Fatalf("read %s: %v", rulePath, err)
	}
	var rule ruleWithConditions
	if err := json.Unmarshal(data, &rule); err != nil {
		t.Fatalf("decode %s: %v", rulePath, err)
	}
	if rule.ID != "reviewer_rejected_retry_research" {
		t.Fatalf("rule id = %q, expected reviewer_rejected_retry_research (path/file mismatch?)", rule.ID)
	}

	var found bool
	for _, c := range rule.Conditions {
		if c.Field != "chain.recovery.proceed" {
			continue
		}
		found = true
		if c.Operator != "eq" {
			t.Errorf("chain.recovery.proceed operator = %q, want %q (Counter writes the positive sentinel; rule fires only on presence)", c.Operator, "eq")
		}
		if v, _ := c.Value.(string); v != "true" {
			t.Errorf("chain.recovery.proceed value = %v, want \"true\" (matches the Counter's stamp; any other value silently disables the cap)", c.Value)
		}
		if !c.Required {
			t.Errorf("chain.recovery.proceed required = false; the gate must be hard or it doesn't actually cap recovery cycles")
		}
	}
	if !found {
		t.Fatalf("retry-research rule missing chain.recovery.proceed gate — recovery cap is not wired (ADR-040 §addendum 2026-05-11). Found %d conditions: %+v", len(rule.Conditions), rule.Conditions)
	}
}

// TestRecoveryCapRule_GatesOnReviewerRoles pins the contract that the
// rule fires on BOTH reviewer-research AND reviewer-spec terminals. The
// MVP roster's "reviewer" role splits into three modes (research/spec/qa);
// the retry-research rule must cover the two modes whose insufficient
// terminal is recoverable (qa-mode rejections route through a different
// path because failing tests are not "insufficient research" — they're a
// build problem).
func TestRecoveryCapRule_GatesOnReviewerRoles(t *testing.T) {
	const rulePath = "../../configs/rules/research-mode-transition/02-reviewer-rejected-retry-research.json"
	data, err := os.ReadFile(rulePath)
	if err != nil {
		t.Fatalf("read %s: %v", rulePath, err)
	}
	var rule ruleWithConditions
	if err := json.Unmarshal(data, &rule); err != nil {
		t.Fatalf("decode %s: %v", rulePath, err)
	}

	for _, c := range rule.Conditions {
		if c.Field != "agent.loop.role" {
			continue
		}
		if c.Operator != "in" {
			t.Errorf("agent.loop.role operator = %q, want %q (rule fires on multiple reviewer modes; eq cannot match a slice value)", c.Operator, "in")
			continue
		}
		roles, ok := c.Value.([]any)
		if !ok {
			t.Fatalf("agent.loop.role value = %#v, want []any (got type %T)", c.Value, c.Value)
		}
		want := map[string]bool{
			"reviewer-research": false,
			"reviewer-spec":     false,
		}
		for _, r := range roles {
			s, _ := r.(string)
			if _, ok := want[s]; ok {
				want[s] = true
			}
		}
		for role, seen := range want {
			if !seen {
				t.Errorf("agent.loop.role 'in' list missing %q — both reviewer-research and reviewer-spec must be covered (ADR-041 MVP roster)", role)
			}
		}
		return
	}
	t.Fatal("rule missing agent.loop.role condition entirely")
}
