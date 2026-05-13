package contract

import (
	"encoding/json"
	"os"
	"strings"
	"testing"
)

// builderQARuleJSON is the parsed shape of
// dev-via-spec/04-builder-decide-to-reviewer-qa.json conditions + prompt.
// Mirrors the rule file's on_enter[0].prompt for assertion purposes.
type builderQARuleJSON struct {
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

const builderQARulePath = "../../configs/rules/dev-via-spec/04-builder-decide-to-reviewer-qa.json"

// TestBuilderQARule_EvidenceSummaryReadyCondition asserts that
// 04-builder-decide-to-reviewer-qa.json carries the evidence.summary_ready
// condition required by ADR-036 §Phase 2. The condition gates the qa-
// reviewer spawn until cmd/semteams/evidence/preprocessor.go has finished
// running checks. (Migrated from the deleted 07-builder-decide-to-qa-
// reviewer.json under ADR-041 Slice 2D-3.)
func TestBuilderQARule_EvidenceSummaryReadyCondition(t *testing.T) {
	data, err := os.ReadFile(builderQARulePath)
	if err != nil {
		t.Fatalf("read builder→qa rule: %v", err)
	}

	var rule builderQARuleJSON
	if err := json.Unmarshal(data, &rule); err != nil {
		t.Fatalf("unmarshal builder→qa rule: %v", err)
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
		t.Error("builder→qa rule conditions missing evidence.summary_ready — " +
			"ADR-036 §Phase 2 requires this for the preprocessor two-AND gate")
	}
}

// TestBuilderQARule_QAModeGateProceedCondition asserts the rule gates on
// the phasevalidator.QAModeGate sentinel (chain.qa_mode_gate.proceed).
// ADR-041 §Risks mitigation qa-mode pre-check requires the structural
// tests_run>0 AND tests_failed=0 check to run BEFORE the reviewer LLM is
// invoked — the sentinel is the load-bearing gate.
func TestBuilderQARule_QAModeGateProceedCondition(t *testing.T) {
	data, err := os.ReadFile(builderQARulePath)
	if err != nil {
		t.Fatalf("read builder→qa rule: %v", err)
	}

	var rule builderQARuleJSON
	if err := json.Unmarshal(data, &rule); err != nil {
		t.Fatalf("unmarshal builder→qa rule: %v", err)
	}

	found := false
	for _, c := range rule.Conditions {
		if c.Field != "chain.qa_mode_gate.proceed" {
			continue
		}
		found = true
		if !c.Required {
			t.Error("chain.qa_mode_gate.proceed condition must have required=true")
		}
		if c.Operator != "eq" {
			t.Errorf("chain.qa_mode_gate.proceed operator = %q, want %q", c.Operator, "eq")
		}
		val, _ := c.Value.(string)
		if val != "true" {
			t.Errorf("chain.qa_mode_gate.proceed value = %v, want %q", c.Value, "true")
		}
	}
	if !found {
		t.Error("builder→qa rule conditions missing chain.qa_mode_gate.proceed — " +
			"ADR-041 §Risks mitigation qa-mode pre-check requires this gate")
	}
}

// TestBuilderQARule_PromptNoStub asserts that the literal "(stub" string
// from the R3.7.2.k′ placeholder has been removed. Any remaining "stub"
// text would route qa-reviewer to needs_clarification prematurely rather
// than grading real evidence.
func TestBuilderQARule_PromptNoStub(t *testing.T) {
	data, err := os.ReadFile(builderQARulePath)
	if err != nil {
		t.Fatalf("read builder→qa rule: %v", err)
	}

	var rule builderQARuleJSON
	if err := json.Unmarshal(data, &rule); err != nil {
		t.Fatalf("unmarshal builder→qa rule: %v", err)
	}

	if len(rule.OnEnter) == 0 {
		t.Fatal("builder→qa rule on_enter is empty — expected at least one publish_agent action")
	}
	prompt := rule.OnEnter[0].Prompt
	if strings.Contains(prompt, "(stub") {
		t.Errorf("builder→qa rule prompt still contains '(stub' literal — retire the R3.7.2.k′ placeholder")
	}
}

// TestBuilderQARule_PromptTripleSubstitution asserts that the prompt uses
// the triple-substitution syntax for the evidence summary block. The
// preprocessor stamps evidence.summary onto the builder entity; the spawn
// prompt must reference it via $entity.triple.evidence.summary so the
// framework interpolates the rendered summary into qa-reviewer's context.
func TestBuilderQARule_PromptTripleSubstitution(t *testing.T) {
	data, err := os.ReadFile(builderQARulePath)
	if err != nil {
		t.Fatalf("read builder→qa rule: %v", err)
	}

	var rule builderQARuleJSON
	if err := json.Unmarshal(data, &rule); err != nil {
		t.Fatalf("unmarshal builder→qa rule: %v", err)
	}

	if len(rule.OnEnter) == 0 {
		t.Fatal("builder→qa rule on_enter is empty")
	}
	prompt := rule.OnEnter[0].Prompt
	const wantToken = "$entity.triple.evidence.summary"
	if !strings.Contains(prompt, wantToken) {
		t.Errorf("builder→qa rule prompt missing %q — preprocessor triple substitution not wired", wantToken)
	}
}
