package contract

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestModelCapabilitiesResolveInRules locks in the invariant that every
// capability name referenced by a `model` field in any rule's publish_agent
// action must appear in the config's model_registry.capabilities map.
//
// Motivating regression: Slice 1 reviewer flagged that osh-demo.json's
// research-mode-transition rules reference `"model": "research"` but the
// capabilities map had no `research` entry — the registry silently fell
// back to defaults.model (gemini-pro). Same gap for "coordinator". The
// resolution still "works" because Registry.Resolve returns defaults
// on miss, but the silent fallback means model-tier discipline is
// undefined per-role and operators can't pin a cheap model to a 1-shot
// classifier role without realizing the request is being routed elsewhere.
//
// Test scope: configs that DECLARE both a `model_registry.capabilities`
// map AND a `rule-processor.rules_files` list. Configs without rules
// (mock-LLM, hello-world, etc.) are skipped silently.
//
// For each in-scope config, the test walks every rule's
// `on_enter[].publish_agent.model` value and asserts:
//
//  1. The value is in the config's capabilities map, OR
//  2. The value matches an endpoint name in model_registry.endpoints
//     (direct endpoint selection is also valid wire-shape per upstream
//     model/registry.go Resolve fallback semantics).
//
// A miss surfaces as a structural failure with the offending rule path
// and model name so operators can either add the capability or rename
// the rule's reference.
func TestModelCapabilitiesResolveInRules(t *testing.T) {
	configs, err := filepath.Glob("../../configs/*.json")
	if err != nil {
		t.Fatalf("glob configs: %v", err)
	}
	if len(configs) == 0 {
		t.Fatal("no configs found — wrong working directory?")
	}

	for _, cfgPath := range configs {
		t.Run(filepath.Base(cfgPath), func(t *testing.T) {
			data, err := os.ReadFile(cfgPath)
			if err != nil {
				t.Fatalf("read %s: %v", cfgPath, err)
			}

			capabilities, endpoints, rulesFiles, ok := extractCapsAndRules(t, data)
			if !ok {
				t.Skip("config does not declare both capabilities + rules_files; out of scope")
			}

			for _, rulePath := range rulesFiles {
				rel := strings.TrimPrefix(rulePath, "/app/")
				hostPath := filepath.Join("../..", rel)
				ruleData, err := os.ReadFile(hostPath)
				if err != nil {
					t.Logf("skipping unreadable %s: %v (existence enforced by rules_files_paths_test.go)", hostPath, err)
					continue
				}

				var rule struct {
					ID      string `json:"id"`
					OnEnter []struct {
						Type  string `json:"type"`
						Model string `json:"model"`
					} `json:"on_enter"`
				}
				if err := json.Unmarshal(ruleData, &rule); err != nil {
					t.Errorf("%s: invalid JSON: %v", rulePath, err)
					continue
				}

				for i, action := range rule.OnEnter {
					if action.Type != "publish_agent" {
						continue
					}
					if action.Model == "" {
						continue
					}
					if _, ok := capabilities[action.Model]; ok {
						continue
					}
					if _, ok := endpoints[action.Model]; ok {
						// Direct endpoint reference — Registry.Resolve
						// returns defaults on capability miss, but
						// the endpoint name itself bypasses resolution.
						continue
					}
					t.Errorf("%s rule %q on_enter[%d].model=%q is neither a capability nor an endpoint — Registry.Resolve will silently fall back to defaults.model. Add %q to model_registry.capabilities or rename the rule's reference.",
						filepath.Base(rulePath), rule.ID, i, action.Model, action.Model)
				}
			}
		})
	}
}

// extractCapsAndRules pulls the capabilities map, endpoints map, and the
// rules_files list from a config's component map. Returns ok=false when
// the config doesn't have both pieces (out of scope for this test).
func extractCapsAndRules(t *testing.T, data []byte) (caps, endpoints map[string]any, rulesFiles []string, ok bool) {
	t.Helper()
	var cfg struct {
		ModelRegistry struct {
			Capabilities map[string]any `json:"capabilities"`
			Endpoints    map[string]any `json:"endpoints"`
		} `json:"model_registry"`
		Components map[string]struct {
			Config json.RawMessage `json:"config"`
		} `json:"components"`
	}
	if err := json.Unmarshal(data, &cfg); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(cfg.ModelRegistry.Capabilities) == 0 {
		return nil, nil, nil, false
	}
	for _, comp := range cfg.Components {
		if len(comp.Config) == 0 {
			continue
		}
		var compCfg struct {
			RulesFiles []string `json:"rules_files"`
		}
		if err := json.Unmarshal(comp.Config, &compCfg); err != nil {
			continue
		}
		if len(compCfg.RulesFiles) > 0 {
			rulesFiles = compCfg.RulesFiles
			break
		}
	}
	if len(rulesFiles) == 0 {
		return nil, nil, nil, false
	}
	return cfg.ModelRegistry.Capabilities, cfg.ModelRegistry.Endpoints, rulesFiles, true
}
