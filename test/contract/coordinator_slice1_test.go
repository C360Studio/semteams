package contract

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestCoordinatorSlice1ActionTaxonomy locks in the four-value action
// taxonomy that coordinator-redesign Slice 1 introduces. The persona
// (10-decision-contract.md) and the rule set
// (configs/rules/coordinator/01-03) must agree on the action values:
// every action with a rule downstream must appear in the persona, and
// every persona action that spawns must have a matching rule.
//
// respond_direct is the persona-listed terminal that has NO rule —
// coordinator just stops on that action. ask_user does have a rule
// (no spawn; publishes to user.response.* + audit triple), so it
// counts as wired.
func TestCoordinatorSlice1ActionTaxonomy(t *testing.T) {
	personaPath := "../../configs/personas/fragments/coordinator/10-decision-contract.md"
	body, err := os.ReadFile(personaPath)
	if err != nil {
		t.Fatalf("read %s: %v", personaPath, err)
	}
	personaText := string(body)

	personaActions := []string{
		"delegate_research",
		"delegate_dev_chain",
		"respond_direct",
		"ask_user",
	}
	for _, action := range personaActions {
		if !strings.Contains(personaText, "`"+action+"`") {
			t.Errorf("persona 10-decision-contract.md missing action value `%s`", action)
		}
	}

	ruleDir := "../../configs/rules/coordinator"
	entries, err := os.ReadDir(ruleDir)
	if err != nil {
		t.Fatalf("read %s: %v", ruleDir, err)
	}

	ruleActions := map[string]string{}
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		rulePath := filepath.Join(ruleDir, e.Name())
		data, err := os.ReadFile(rulePath)
		if err != nil {
			t.Fatalf("read %s: %v", rulePath, err)
		}
		var rule struct {
			ID         string `json:"id"`
			Conditions []struct {
				Field    string      `json:"field"`
				Operator string      `json:"operator"`
				Value    interface{} `json:"value"`
			} `json:"conditions"`
		}
		if err := json.Unmarshal(data, &rule); err != nil {
			t.Errorf("%s: invalid JSON: %v", e.Name(), err)
			continue
		}
		for _, cond := range rule.Conditions {
			if cond.Field != "coordinator.decision.next_action" || cond.Operator != "eq" {
				continue
			}
			action, ok := cond.Value.(string)
			if !ok {
				t.Errorf("%s: next_action condition value is not a string: %v", e.Name(), cond.Value)
				continue
			}
			ruleActions[action] = e.Name()
		}
	}

	// Every rule-listed action MUST appear in the persona — orphan
	// rule = silent dead code.
	for action, ruleFile := range ruleActions {
		found := false
		for _, p := range personaActions {
			if p == action {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("rule %s targets action %q which is not in the persona taxonomy",
				ruleFile, action)
		}
	}

	// Every spawning persona action MUST have a rule. respond_direct
	// is intentionally rule-less (stop semantics); the rest must wire.
	wantWired := []string{"delegate_research", "delegate_dev_chain", "ask_user"}
	for _, action := range wantWired {
		if _, ok := ruleActions[action]; !ok {
			t.Errorf("persona action %q has no rule in configs/rules/coordinator/", action)
		}
	}
}

// TestOshDemoUsesCoordinatorDispatch locks in the Slice 1 dispatch
// flip: osh-demo.json must route through coordinator (not directly
// to researcher-plan) and must load the three Slice 1 coordinator
// rules.
func TestOshDemoUsesCoordinatorDispatch(t *testing.T) {
	data, err := os.ReadFile("../../configs/osh-demo.json")
	if err != nil {
		t.Fatalf("read osh-demo.json: %v", err)
	}
	var cfg struct {
		Components map[string]struct {
			Config json.RawMessage `json:"config"`
		} `json:"components"`
	}
	if err := json.Unmarshal(data, &cfg); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	dispatchCfg := cfg.Components["teams-dispatch"].Config
	if len(dispatchCfg) == 0 {
		t.Fatal("teams-dispatch component missing")
	}
	var dispatch struct {
		DefaultRole  string   `json:"default_role"`
		DefaultTools []string `json:"default_tools"`
	}
	if err := json.Unmarshal(dispatchCfg, &dispatch); err != nil {
		t.Fatalf("unmarshal dispatch: %v", err)
	}
	if dispatch.DefaultRole != "coordinator" {
		t.Errorf("osh-demo dispatch default_role = %q, want %q (Slice 1 dispatch flip)",
			dispatch.DefaultRole, "coordinator")
	}
	hasDecide := false
	for _, tool := range dispatch.DefaultTools {
		if tool == "decide" {
			hasDecide = true
			break
		}
	}
	if !hasDecide {
		t.Errorf("osh-demo dispatch default_tools missing `decide` — coordinator persona requires it: %v",
			dispatch.DefaultTools)
	}

	ruleCfg := cfg.Components["rule-processor"].Config
	var ruleProc struct {
		RulesFiles []string `json:"rules_files"`
	}
	if err := json.Unmarshal(ruleCfg, &ruleProc); err != nil {
		t.Fatalf("unmarshal rule-processor: %v", err)
	}
	wantRules := []string{
		"/app/configs/rules/coordinator/01-delegate-research.json",
		"/app/configs/rules/coordinator/02-delegate-dev-chain.json",
		"/app/configs/rules/coordinator/03-ask-user.json",
	}
	loaded := make(map[string]bool, len(ruleProc.RulesFiles))
	for _, r := range ruleProc.RulesFiles {
		loaded[r] = true
	}
	for _, want := range wantRules {
		if !loaded[want] {
			t.Errorf("osh-demo rules_files missing %q (Slice 1 coordinator wiring)", want)
		}
	}
}
