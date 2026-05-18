package contract

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// researchRuleJSON is the minimal parsed shape we need for pack-level
// invariants — conditions (for the forbidden-field scan), on_enter
// (for spawned-role discovery, the write-side forbidden-predicate
// scan, and add_triple-predicate inspection), and id (for the
// unique-IDs check).
type researchRuleJSON struct {
	ID         string `json:"id"`
	Type       string `json:"type"`
	Enabled    bool   `json:"enabled"`
	Conditions []struct {
		Field string `json:"field"`
	} `json:"conditions"`
	OnEnter []struct {
		Type      string `json:"type"`
		Role      string `json:"role,omitempty"`
		Predicate string `json:"predicate,omitempty"`
	} `json:"on_enter"`
}

const researchPackDir = "../../configs/rules/research"

// researchPackForbiddenConditionFields lists the chain.mode / phasevalidator /
// chainstall / recoverycounter / evidence-preprocessor sentinel fields that
// the ADR-042 §Phase 2 redesign retires. The research pack must reach the
// same recovery + terminal behaviour without any of these gates — pack
// identity replaces chain-entity classification; rule max_iterations
// replaces chain-ancestry recovery counting.
//
// Listed as exact prefixes; any condition.field that starts with one of
// these is a violation.
var researchPackForbiddenConditionFields = []string{
	"chain.mode.",                     // phasevalidator mirror writes
	"chain.phase_transition.proceed.", // phasevalidator forward edges
	"chain.spec_mode_gate.",           // phasevalidator.SpecModeGate
	"chain.qa_mode_gate.",             // phasevalidator.QAModeGate
	"chain.recovery.proceed",          // recoverycounter sentinel
	"chain.stall.",                    // chainstall recovery
	"evidence.summary_ready",          // evidence preprocessor (dev-via-spec only)
}

// TestResearchRulePack_NoChainMachinery scans both the condition.field
// (read side) and on_enter[].predicate (write side) in every research
// pack rule file and asserts none reference the retired chain.mode /
// phasevalidator / chainstall / recoverycounter machinery.
//
// ADR-042 §Phase 2 redesign requires the category-keyed pack to encode
// its forward + recovery edges via pack-local matches (role + decide
// action), NOT via Go-component-stamped sentinels. The write-side scan
// is defense-in-depth: if a future maintainer adds an `add_triple`
// writing chain.mode.classification (e.g. attempting to interoperate
// with a still-loaded legacy rule during MVP-7 migration), this test
// catches it before any condition starts gating on the new write.
//
// Metadata-level mentions are allowed (the migration-posture notes
// reference the legacy machinery by name); only condition.field and
// on_enter[].predicate violations fail the test.
func TestResearchRulePack_NoChainMachinery(t *testing.T) {
	files, err := filepath.Glob(filepath.Join(researchPackDir, "*.json"))
	if err != nil {
		t.Fatalf("glob research pack: %v", err)
	}
	if len(files) == 0 {
		t.Fatal("no rule files found in research pack — wrong working directory?")
	}

	for _, path := range files {
		t.Run(filepath.Base(path), func(t *testing.T) {
			data, err := os.ReadFile(path)
			if err != nil {
				t.Fatalf("read %s: %v", path, err)
			}
			var rule researchRuleJSON
			if err := json.Unmarshal(data, &rule); err != nil {
				t.Fatalf("unmarshal %s: %v", path, err)
			}

			for _, c := range rule.Conditions {
				for _, forbidden := range researchPackForbiddenConditionFields {
					if strings.HasPrefix(c.Field, forbidden) {
						t.Errorf("condition field %q starts with forbidden prefix %q — ADR-042 §Phase 2 redesign retires this gate; rewrite the rule to match on role + coordinator.decision.next_action instead",
							c.Field, forbidden)
					}
				}
			}

			for _, a := range rule.OnEnter {
				if a.Type != "add_triple" || a.Predicate == "" {
					continue
				}
				for _, forbidden := range researchPackForbiddenConditionFields {
					if strings.HasPrefix(a.Predicate, forbidden) {
						t.Errorf("on_enter add_triple predicate %q starts with forbidden prefix %q — ADR-042 §Phase 2 redesign retires the chain.mode / phasevalidator / chainstall write surface; remove the add_triple",
							a.Predicate, forbidden)
					}
				}
			}
		})
	}
}

// TestResearchRulePack_ExpectedRolesSpawned asserts the pack's
// publish_agent actions spawn exactly the role tokens documented in
// configs/rules/research/README.md. This is the structural pin that
// keeps role-naming consistent with the future MVP-3 persona dirs and
// MVP-4 coordinator action allowlist. Adding a role here without
// updating the personas (or vice versa) is the regression class this
// catches.
func TestResearchRulePack_ExpectedRolesSpawned(t *testing.T) {
	files, err := filepath.Glob(filepath.Join(researchPackDir, "*.json"))
	if err != nil {
		t.Fatalf("glob research pack: %v", err)
	}

	spawnedRoles := make(map[string]bool)
	for _, path := range files {
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read %s: %v", path, err)
		}
		var rule researchRuleJSON
		if err := json.Unmarshal(data, &rule); err != nil {
			t.Fatalf("unmarshal %s: %v", path, err)
		}
		for _, a := range rule.OnEnter {
			if a.Type == "publish_agent" && a.Role != "" {
				spawnedRoles[a.Role] = true
			}
		}
	}

	want := []string{
		"researcher-research-plan",
		"researcher-research-gather",
		"researcher-research-synthesize",
		"reviewer-research",
		"coordinator", // wake-up on terminal approval
	}
	for _, r := range want {
		if !spawnedRoles[r] {
			t.Errorf("research pack does not spawn role %q — check publish_agent actions in configs/rules/research/*.json", r)
		}
	}

	allowed := make(map[string]bool, len(want))
	for _, r := range want {
		allowed[r] = true
	}
	for r := range spawnedRoles {
		if !allowed[r] {
			t.Errorf("research pack spawns unexpected role %q — either add it to the README's naming table + this test's want list, or fix the rule",
				r)
		}
	}
}

// TestResearchRulePack_RuleIDsUnique asserts every rule file carries a
// unique id. Two rules with the same id collide in the rule-engine's
// stateful evaluator (the second silently overwrites the first); this
// test catches accidental duplication at commit time.
func TestResearchRulePack_RuleIDsUnique(t *testing.T) {
	files, err := filepath.Glob(filepath.Join(researchPackDir, "*.json"))
	if err != nil {
		t.Fatalf("glob research pack: %v", err)
	}

	seen := make(map[string]string)
	for _, path := range files {
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read %s: %v", path, err)
		}
		var rule researchRuleJSON
		if err := json.Unmarshal(data, &rule); err != nil {
			t.Fatalf("unmarshal %s: %v", path, err)
		}
		if rule.ID == "" {
			t.Errorf("%s: rule has no id", filepath.Base(path))
			continue
		}
		if prior, dup := seen[rule.ID]; dup {
			t.Errorf("duplicate rule id %q in %s — also in %s",
				rule.ID, filepath.Base(path), filepath.Base(prior))
			continue
		}
		seen[rule.ID] = path
	}
}

// TestResearchRulePack_AllEnabled asserts every rule in the pack is
// enabled: true. A disabled rule in a curated pack is almost always a
// commit-in-progress artifact — fail-loud catches it.
func TestResearchRulePack_AllEnabled(t *testing.T) {
	files, err := filepath.Glob(filepath.Join(researchPackDir, "*.json"))
	if err != nil {
		t.Fatalf("glob research pack: %v", err)
	}
	for _, path := range files {
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read %s: %v", path, err)
		}
		var rule researchRuleJSON
		if err := json.Unmarshal(data, &rule); err != nil {
			t.Fatalf("unmarshal %s: %v", path, err)
		}
		if !rule.Enabled {
			t.Errorf("%s: rule enabled=false — research pack rules must be enabled (commit-in-progress?)",
				filepath.Base(path))
		}
	}
}

// TestResearchRulePack_WiredIntoBootstrap asserts every rule file in
// the pack is referenced from configs/flow-bootstrap.json's
// rule.config.rules_files. A new rule file that is never wired is a
// silent no-op; this test catches it on commit.
func TestResearchRulePack_WiredIntoBootstrap(t *testing.T) {
	files, err := filepath.Glob(filepath.Join(researchPackDir, "*.json"))
	if err != nil {
		t.Fatalf("glob research pack: %v", err)
	}

	bootstrapData, err := os.ReadFile("../../configs/flow-bootstrap.json")
	if err != nil {
		t.Fatalf("read flow-bootstrap.json: %v", err)
	}
	var bootstrap struct {
		Components map[string]struct {
			Config struct {
				RulesFiles []string `json:"rules_files"`
			} `json:"config"`
		} `json:"components"`
	}
	if err := json.Unmarshal(bootstrapData, &bootstrap); err != nil {
		t.Fatalf("unmarshal flow-bootstrap.json: %v", err)
	}

	ruleProcessor, ok := bootstrap.Components["rule"]
	if !ok {
		t.Fatal("rule component not found in flow-bootstrap.json")
	}
	wired := make(map[string]bool, len(ruleProcessor.Config.RulesFiles))
	for _, f := range ruleProcessor.Config.RulesFiles {
		wired[filepath.Base(f)] = true
	}

	for _, path := range files {
		base := filepath.Base(path)
		if base == "README.md" {
			continue
		}
		if !wired[base] {
			t.Errorf("research pack rule %q is not referenced in flow-bootstrap.json rules_files — add the /app/configs/rules/research/%s entry",
				base, base)
		}
	}
}
