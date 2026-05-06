package contract

import (
	"encoding/json"
	"os"
	"strings"
	"testing"
)

// rule07JSON is the parsed shape of rule 07's conditions and prompt.
// It mirrors the rule file's on_enter[0].prompt for assertion purposes.
type rule07JSON struct {
	Conditions []struct {
		Field    string `json:"field"`
		Operator string `json:"operator"`
		Value    any    `json:"value"`
		Required bool   `json:"required"`
	} `json:"conditions"`
	Metadata struct {
		Phase                   string `json:"phase"`
		EvidenceSummaryPlumbing string `json:"evidence_summary_plumbing"`
	} `json:"metadata"`
	OnEnter []struct {
		Prompt string `json:"prompt"`
	} `json:"on_enter"`
}

// TestRule07_EvidenceSummaryReadyCondition asserts that rule_07 contains the
// evidence.summary_ready condition required by ADR-036 §Phase 2. The two-AND
// condition (coordinator.next_action + evidence.summary_ready) is what gates
// the qa-reviewer spawn until the preprocessor has finished running checks.
func TestRule07_EvidenceSummaryReadyCondition(t *testing.T) {
	data, err := os.ReadFile("../../configs/rules/dev-via-spec/07-builder-decide-to-qa-reviewer.json")
	if err != nil {
		t.Fatalf("read rule 07: %v", err)
	}

	var rule rule07JSON
	if err := json.Unmarshal(data, &rule); err != nil {
		t.Fatalf("unmarshal rule 07: %v", err)
	}

	// Assert the evidence.summary_ready condition is present and required.
	found := false
	for _, c := range rule.Conditions {
		if c.Field != "evidence.summary_ready" {
			continue
		}
		found = true
		if !c.Required {
			t.Error("evidence.summary_ready condition must have required=true")
		}
		if c.Operator != "eq" {
			t.Errorf("evidence.summary_ready operator = %q, want %q", c.Operator, "eq")
		}
		val, _ := c.Value.(string)
		if val != "true" {
			t.Errorf("evidence.summary_ready value = %v, want %q", c.Value, "true")
		}
	}
	if !found {
		t.Error("rule_07 conditions missing evidence.summary_ready — " +
			"ADR-036 §Phase 2 requires this for the preprocessor two-AND gate")
	}
}

// TestRule07_PromptNoStub asserts that the literal "(stub" string from the
// R3.7.2.k′ placeholder has been removed. Any remaining "stub" text in the
// prompt would route qa-reviewer to needs_clarification prematurely rather
// than grading real evidence.
func TestRule07_PromptNoStub(t *testing.T) {
	data, err := os.ReadFile("../../configs/rules/dev-via-spec/07-builder-decide-to-qa-reviewer.json")
	if err != nil {
		t.Fatalf("read rule 07: %v", err)
	}

	var rule rule07JSON
	if err := json.Unmarshal(data, &rule); err != nil {
		t.Fatalf("unmarshal rule 07: %v", err)
	}

	if len(rule.OnEnter) == 0 {
		t.Fatal("rule_07 on_enter is empty — expected at least one publish_agent action")
	}
	prompt := rule.OnEnter[0].Prompt
	if strings.Contains(prompt, "(stub") {
		t.Errorf("rule_07 prompt still contains '(stub' literal — retire the R3.7.2.k′ placeholder")
	}
}

// TestRule07_PromptTripleSubstitution asserts that the prompt uses the
// triple-substitution syntax for the evidence summary block. The
// preprocessor stamps evidence.summary onto the builder entity; rule_07's
// spawn prompt must reference it via $entity.triple.evidence.summary so
// the framework interpolates the rendered summary into qa-reviewer's context.
func TestRule07_PromptTripleSubstitution(t *testing.T) {
	data, err := os.ReadFile("../../configs/rules/dev-via-spec/07-builder-decide-to-qa-reviewer.json")
	if err != nil {
		t.Fatalf("read rule 07: %v", err)
	}

	var rule rule07JSON
	if err := json.Unmarshal(data, &rule); err != nil {
		t.Fatalf("unmarshal rule 07: %v", err)
	}

	if len(rule.OnEnter) == 0 {
		t.Fatal("rule_07 on_enter is empty")
	}
	prompt := rule.OnEnter[0].Prompt
	const wantToken = "$entity.triple.evidence.summary"
	if !strings.Contains(prompt, wantToken) {
		t.Errorf("rule_07 prompt missing %q — preprocessor triple substitution not wired", wantToken)
	}
}
