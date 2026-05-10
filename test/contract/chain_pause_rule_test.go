package contract

import (
	"encoding/json"
	"os"
	"testing"

	"github.com/c360studio/semteams/cmd/semteams/chainpause"
)

// chainPauseRuleJSON is the parsed shape of the chain-pause rule file.
type chainPauseRuleJSON struct {
	ID         string `json:"id"`
	Type       string `json:"type"`
	Conditions []struct {
		Field    string `json:"field"`
		Operator string `json:"operator"`
		Value    any    `json:"value"`
		Required bool   `json:"required"`
	} `json:"conditions"`
	Logic   string `json:"logic"`
	OnEnter []struct {
		Type      string `json:"type"`
		Predicate string `json:"predicate,omitempty"`
		Object    string `json:"object,omitempty"`
	} `json:"on_enter"`
	Metadata struct {
		ADR       string `json:"adr"`
		Phase     string `json:"phase"`
		Authority string `json:"authority"`
	} `json:"metadata"`
}

// TestChainPauseRule_Shape pins the conditions array and action shape so
// rule-engine drift or editor errors are caught before deploy.
// ADR-037 §D1 specifies: outcome=failed AND role in managed-arc list,
// logic=and, on_enter writes a chain.paused add_triple.
func TestChainPauseRule_Shape(t *testing.T) {
	data, err := os.ReadFile("../../configs/rules/chain/00-loop-failed-pause.json")
	if err != nil {
		t.Fatalf("read chain-pause rule: %v", err)
	}

	var rule chainPauseRuleJSON
	if err := json.Unmarshal(data, &rule); err != nil {
		t.Fatalf("unmarshal chain-pause rule: %v", err)
	}

	// ID must be stable — downstream ops-agent queries reference it.
	if rule.ID != "chain_loop_failed_pause" {
		t.Errorf("rule ID = %q, want chain_loop_failed_pause", rule.ID)
	}
	if rule.Type != "expression" {
		t.Errorf("rule type = %q, want expression", rule.Type)
	}
	if rule.Logic != "and" {
		t.Errorf("rule logic = %q, want and", rule.Logic)
	}
	if len(rule.Conditions) != 2 {
		t.Errorf("conditions count = %d, want 2 (outcome + role)", len(rule.Conditions))
	}

	assertChainPauseConditions(t, rule.Conditions)
	assertChainPauseOnEnter(t, rule.OnEnter)

	if rule.Metadata.Phase != "v1" {
		t.Errorf("metadata.phase = %q, want v1", rule.Metadata.Phase)
	}
	if rule.Metadata.Authority != "operator" {
		t.Errorf("metadata.authority = %q, want operator", rule.Metadata.Authority)
	}
}

// assertChainPauseConditions checks that both the outcome and role conditions
// are present and shaped correctly per ADR-037 §D1.
func assertChainPauseConditions(t *testing.T, conditions []struct {
	Field    string `json:"field"`
	Operator string `json:"operator"`
	Value    any    `json:"value"`
	Required bool   `json:"required"`
}) {
	t.Helper()
	var outcomeFound, roleFound bool
	for _, c := range conditions {
		switch c.Field {
		case "agent.loop.outcome":
			outcomeFound = true
			if c.Operator != "eq" {
				t.Errorf("outcome operator = %q, want eq", c.Operator)
			}
			val, _ := c.Value.(string)
			if val != "failed" {
				t.Errorf("outcome value = %v, want failed", c.Value)
			}
			if !c.Required {
				t.Error("outcome condition must be required=true")
			}
		case "agent.loop.role":
			roleFound = true
			assertRoleCondition(t, c.Operator, c.Value, c.Required)
		}
	}
	if !outcomeFound {
		t.Error("agent.loop.outcome condition missing from chain-pause rule")
	}
	if !roleFound {
		t.Error("agent.loop.role condition missing from chain-pause rule")
	}
}

// assertRoleCondition validates the role "in" condition carries all managed arc roles.
func assertRoleCondition(t *testing.T, operator string, value any, required bool) {
	t.Helper()
	if operator != "in" {
		t.Errorf("role operator = %q, want in", operator)
	}
	if !required {
		t.Error("role condition must be required=true")
	}
	roles := toStringSlice(value)
	wantRoles := []string{
		"dev-via-spec-planner",
		"dev-via-spec-reviewer",
		"dev-via-spec-challenger",
		"dev-via-spec-architect",
		"dev-via-spec-builder",
		"dev-via-spec-qa-reviewer",
		"researcher",
		"source-curator",
		"research-reviewer",
		"dispatch",
	}
	roleSet := make(map[string]bool, len(roles))
	for _, r := range roles {
		roleSet[r] = true
	}
	for _, want := range wantRoles {
		if !roleSet[want] {
			t.Errorf("role list missing %q — update rule to include new arc role", want)
		}
	}
}

// assertChainPauseOnEnter verifies the on_enter block contains exactly one
// add_triple action stamping chain.paused.marker. Using chain.paused.marker
// (not chain.paused) keeps the dot-hierarchy uniform so UI/ops-agent queries
// for chain.paused.* find both the rule's marker and the Pauser's §D5 set
// (R6 fix).
func assertChainPauseOnEnter(t *testing.T, onEnter []struct {
	Type      string `json:"type"`
	Predicate string `json:"predicate,omitempty"`
	Object    string `json:"object,omitempty"`
}) {
	t.Helper()
	if len(onEnter) != 1 {
		t.Errorf("on_enter count = %d, want 1", len(onEnter))
		return
	}
	action := onEnter[0]
	if action.Type != "add_triple" {
		t.Errorf("on_enter[0].type = %q, want add_triple", action.Type)
	}
	if action.Predicate != "chain.paused.marker" {
		t.Errorf("on_enter[0].predicate = %q, want chain.paused.marker", action.Predicate)
	}
}

// TestChainPauseRule_PresentInOshDemoConfig asserts that the chain-pause rule
// is wired into osh-demo.json's rules_files, since that is the primary
// production config for dev-via-spec smoke runs.
func TestChainPauseRule_PresentInOshDemoConfig(t *testing.T) {
	data, err := os.ReadFile("../../configs/osh-demo.json")
	if err != nil {
		t.Fatalf("read osh-demo.json: %v", err)
	}

	var cfg struct {
		Components map[string]struct {
			Config struct {
				RulesFiles []string `json:"rules_files"`
			} `json:"config"`
		} `json:"components"`
	}
	if err := json.Unmarshal(data, &cfg); err != nil {
		t.Fatalf("unmarshal osh-demo.json: %v", err)
	}

	ruleProcessor, ok := cfg.Components["rule-processor"]
	if !ok {
		t.Fatal("rule-processor component not found in osh-demo.json")
	}

	wantSuffix := "rules/chain/00-loop-failed-pause.json"
	found := false
	for _, f := range ruleProcessor.Config.RulesFiles {
		if hasSuffix(f, wantSuffix) {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("chain-pause rule not in osh-demo.json rules_files; want entry ending in %q", wantSuffix)
	}
}

// TestChainPauseRule_ManagedRoleParity_Bidirectional asserts that the rule's
// "in" condition list and chainpause.ManagedRoles() are exact mirrors of each
// other. A role in one but not the other causes a silent mismatch: the rule
// fires but the subscriber skips (or vice-versa). R2 fix: parity was
// previously one-directional (rule→Go only).
func TestChainPauseRule_ManagedRoleParity_Bidirectional(t *testing.T) {
	data, err := os.ReadFile("../../configs/rules/chain/00-loop-failed-pause.json")
	if err != nil {
		t.Fatalf("read chain-pause rule: %v", err)
	}
	var rule chainPauseRuleJSON
	if err := json.Unmarshal(data, &rule); err != nil {
		t.Fatalf("unmarshal chain-pause rule: %v", err)
	}

	// Extract the role list from the rule's conditions.
	var ruleRoles []string
	for _, c := range rule.Conditions {
		if c.Field == "agent.loop.role" {
			ruleRoles = toStringSlice(c.Value)
			break
		}
	}
	if len(ruleRoles) == 0 {
		t.Fatal("agent.loop.role condition not found or has empty role list in rule")
	}

	goRoles := chainpause.ManagedRoles()

	// Build sets for O(1) lookup in both directions.
	ruleSet := make(map[string]bool, len(ruleRoles))
	for _, r := range ruleRoles {
		ruleSet[r] = true
	}
	goSet := make(map[string]bool, len(goRoles))
	for _, r := range goRoles {
		goSet[r] = true
	}

	// Direction 1: every Go managed role must be in the rule's "in" list.
	for _, r := range goRoles {
		if !ruleSet[r] {
			t.Errorf("chainpause.ManagedRoles() contains %q but rule's role list does not — rule will NOT fire for this role", r)
		}
	}

	// Direction 2: every rule role must be in chainpause.ManagedRoles().
	for _, r := range ruleRoles {
		if !goSet[r] {
			t.Errorf("rule's role list contains %q but chainpause.ManagedRoles() does not — subscriber will skip this role after rule fires", r)
		}
	}
}

// toStringSlice converts an interface{} that is expected to be a []interface{}
// of strings into a []string. Returns nil for unexpected types.
func toStringSlice(v any) []string {
	arr, ok := v.([]interface{})
	if !ok {
		return nil
	}
	out := make([]string, 0, len(arr))
	for _, item := range arr {
		if s, ok := item.(string); ok {
			out = append(out, s)
		}
	}
	return out
}

// hasSuffix checks whether s ends with suffix.
func hasSuffix(s, suffix string) bool {
	return len(s) >= len(suffix) && s[len(s)-len(suffix):] == suffix
}
