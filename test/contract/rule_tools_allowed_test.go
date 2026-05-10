package contract

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestRuleToolsSubsetOfAllowedTools enforces a structural contract:
// any tool a rule's publish_agent action lists in its `tools` array
// must also appear in the agentic-tools component's `allowed_tools`
// whitelist for every config that loads that rule. The agentic-tools
// allowlist is enforced at runtime — rule says "spawn loop with these
// tools", framework boots the loop, LLM calls a tool, agentic-tools
// rejects with `tool "X" is not allowed` if X is missing from the
// component config's allowed_tools.
//
// Motivating regression: ADR-040 PR 4 first run wedged the curator at
// emit_curator_artifact with `not allowed` because three configs that
// load the new spawn-curator rule (osh-demo, e2e-dev-via-spec,
// e2e-research-mode-transition) didn't whitelist emit_curator_artifact.
// This test prevents the regression class for all future rule-tool
// additions.
//
// Iteration shape: walk configs/*.json, find any with both an
// agentic-tools component (carrying allowed_tools) AND a rule-processor
// component (carrying rules_files). For each rule loaded by that
// config, parse the rule's publish_agent on_enter actions and assert
// every tool name appears in allowed_tools.
func TestRuleToolsSubsetOfAllowedTools(t *testing.T) {
	configs, err := filepath.Glob("../../configs/*.json")
	if err != nil {
		t.Fatalf("glob configs: %v", err)
	}

	for _, cfgPath := range configs {
		t.Run(filepath.Base(cfgPath), func(t *testing.T) {
			data, err := os.ReadFile(cfgPath)
			if err != nil {
				t.Fatalf("read %s: %v", cfgPath, err)
			}

			var cfg struct {
				Components map[string]struct {
					Name   string          `json:"name"`
					Config json.RawMessage `json:"config"`
				} `json:"components"`
			}
			if err := json.Unmarshal(data, &cfg); err != nil {
				t.Fatalf("unmarshal %s: %v", cfgPath, err)
			}

			allowed, hasAllowed := extractAllowedTools(cfg.Components)
			ruleFiles, hasRules := extractRulesFiles(cfg.Components)
			if !hasAllowed || !hasRules {
				return // out of scope — config doesn't have both surfaces
			}

			for _, rulePath := range ruleFiles {
				rel := strings.TrimPrefix(rulePath, "/app/")
				if rel == rulePath {
					continue // rules_files_paths_test catches this; skip here
				}
				hostPath := filepath.Join("../..", rel)
				ruleData, err := os.ReadFile(hostPath)
				if err != nil {
					continue // rules_files_paths_test catches this; skip here
				}

				toolsInRule, err := extractRuleTools(ruleData)
				if err != nil {
					t.Errorf("%s loads %s but rule parse failed: %v",
						filepath.Base(cfgPath), filepath.Base(rulePath), err)
					continue
				}

				for _, tool := range toolsInRule {
					if !allowed[tool] {
						t.Errorf("%s loads %s which spawns loops with tool %q, but the agentic-tools component's allowed_tools does not include it. Boot succeeds; the LLM call fails at runtime with 'tool %q is not allowed'.",
							filepath.Base(cfgPath), filepath.Base(rulePath), tool, tool)
					}
				}
			}
		})
	}
}

// extractAllowedTools finds the agentic-tools component (by name or
// component map key) and returns its allowed_tools as a set. Returns
// (nil, false) if the config has no agentic-tools component or no
// allowed_tools field.
func extractAllowedTools(components map[string]struct {
	Name   string          `json:"name"`
	Config json.RawMessage `json:"config"`
}) (map[string]bool, bool) {
	for key, comp := range components {
		// Match by component map key OR name field — operators may use
		// instance names (e.g. teams-tools) that point at the
		// agentic-tools factory.
		if key != "agentic-tools" && comp.Name != "agentic-tools" {
			continue
		}
		if len(comp.Config) == 0 {
			return nil, false
		}
		var compCfg struct {
			AllowedTools []string `json:"allowed_tools"`
		}
		if err := json.Unmarshal(comp.Config, &compCfg); err != nil {
			return nil, false
		}
		if len(compCfg.AllowedTools) == 0 {
			return nil, false
		}
		set := make(map[string]bool, len(compCfg.AllowedTools))
		for _, t := range compCfg.AllowedTools {
			set[t] = true
		}
		return set, true
	}
	return nil, false
}

// extractRulesFiles finds the rule-processor component and returns its
// rules_files paths. Returns (nil, false) if the config has no
// rule-processor or no rules_files.
func extractRulesFiles(components map[string]struct {
	Name   string          `json:"name"`
	Config json.RawMessage `json:"config"`
}) ([]string, bool) {
	for key, comp := range components {
		if key != "rule-processor" && comp.Name != "rule-processor" {
			continue
		}
		if len(comp.Config) == 0 {
			return nil, false
		}
		var compCfg struct {
			RulesFiles []string `json:"rules_files"`
		}
		if err := json.Unmarshal(comp.Config, &compCfg); err != nil {
			return nil, false
		}
		if len(compCfg.RulesFiles) == 0 {
			return nil, false
		}
		return compCfg.RulesFiles, true
	}
	return nil, false
}

// extractRuleTools parses a rule JSON file and returns every tool name
// referenced by any publish_agent on_enter action's `tools` array.
// Deduplicated across actions.
func extractRuleTools(ruleData []byte) ([]string, error) {
	var rule struct {
		OnEnter []struct {
			Type  string   `json:"type"`
			Tools []string `json:"tools"`
		} `json:"on_enter"`
	}
	if err := json.Unmarshal(ruleData, &rule); err != nil {
		return nil, err
	}
	seen := make(map[string]struct{})
	for _, action := range rule.OnEnter {
		if action.Type != "publish_agent" {
			continue
		}
		for _, t := range action.Tools {
			seen[t] = struct{}{}
		}
	}
	out := make([]string, 0, len(seen))
	for t := range seen {
		out = append(out, t)
	}
	return out, nil
}
