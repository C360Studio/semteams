package contract

import (
	"encoding/json"
	"os"
	"testing"
)

// ruleWithConditions is the minimum shape we need to inspect rule_02's
// conditions block. The full rule schema is more permissive; we look
// only at the fields the chain-recovery cap depends on.
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
// rule_02 ("Research Reviewer Rejected — Spawn Source-Curator") gates
// curator spawns on the chain.recovery.proceed sentinel that
// cmd/semteams/recoverycounter writes onto the triggering reviewer
// entity when the chain has remaining recovery budget. Absence of
// proceed → rule never fires → chain stalls fail-safe.
// ADR-040 §addendum 2026-05-11.
//
// The Counter and the rule fire on the same agent.complete event;
// Counter writes the proceed sentinel after its chain-walk and
// cap check; the KV update triggers rule re-eval; rule fires.
// Renames on either side break this test before a silently-
// decoupled cap reaches a real-LLM smoke.
func TestRecoveryCapRule_GatesOnProceedSentinel(t *testing.T) {
	const rulePath = "../../configs/rules/research-mode-transition/02-reviewer-rejected-spawn-curator.json"
	data, err := os.ReadFile(rulePath)
	if err != nil {
		t.Fatalf("read %s: %v", rulePath, err)
	}
	var rule ruleWithConditions
	if err := json.Unmarshal(data, &rule); err != nil {
		t.Fatalf("decode %s: %v", rulePath, err)
	}
	if rule.ID != "reviewer_rejected_spawn_curator" {
		t.Fatalf("rule id = %q, expected reviewer_rejected_spawn_curator (path/file mismatch?)", rule.ID)
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
		t.Fatalf("rule_02 missing chain.recovery.proceed gate — recovery cap is not wired (ADR-040 §addendum 2026-05-11). Found %d conditions: %+v", len(rule.Conditions), rule.Conditions)
	}
}
