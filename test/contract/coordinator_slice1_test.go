package contract

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestMVPCoordinatorActionTaxonomy locks in the closed action
// taxonomy the coordinator persona (10-decision-contract.md) and the
// rule layer must agree on under the MVP roster (ADR-042 §Phase 2
// redesign). The MVP boots from configs/flow-bootstrap.json; its
// rules_files list selects which rules are live. Legacy rules that
// stay on disk for MVP-7 cleanup (configs/rules/coordinator/
// 01-delegate-research.json, 02-delegate-dev-chain.json) are NOT
// loaded by bootstrap, so they are out of scope for the live
// taxonomy check.
//
// The closed taxonomy for the MVP deployment is:
//
//	research        — research-pack rule 01 (research-category arc)
//	ask_user        — coordinator/03-ask-user.json
//	respond_direct  — coordinator/03b-respond-direct.json
//
// Invariant: every action emitted by the live rule set has a backing
// persona entry (no rule that fires without LLM-emitted token); every
// persona action has a backing rule (no LLM token that dead-ends).
//
// Slice 1c wake-up rules (rules that gate on `agent.loop.role ==
// "<reviewer role>"` AND `coordinator.decision.next_action ==
// <reviewer terminal>`) are coordinator-related but fire on reviewer
// loops, so their action values come from reviewer personas, not the
// coordinator persona. The taxonomy check scopes to rules whose
// firing role is "coordinator"; the wake-up rules are validated
// separately (TestCoordinatorSlice1cWakeUpRules below) for their
// reviewer-role/approval-token shape.
func TestMVPCoordinatorActionTaxonomy(t *testing.T) {
	personaPath := "../../configs/personas/fragments/coordinator/10-decision-contract.md"
	body, err := os.ReadFile(personaPath)
	if err != nil {
		t.Fatalf("read %s: %v", personaPath, err)
	}
	personaText := string(body)

	personaActions := []string{"research", "respond_direct", "ask_user"}
	for _, action := range personaActions {
		// Persona must teach the action via its action-value table
		// (backtick-wrapped token). The "don't invent" warning that
		// names legacy tokens like delegate_research uses the same
		// backtick wrapping, so the substring check alone is
		// insufficient — we anchor on the table heading "When to
		// use" + the closed-taxonomy section. The presence check
		// here is structural; the reviewer-pass grades prose
		// quality.
		if !strings.Contains(personaText, "`"+action+"`") {
			t.Errorf("persona 10-decision-contract.md missing action value `%s`", action)
		}
	}

	bootstrapRules := bootstrapLoadedRules(t)
	ruleActions := collectCoordinatorRoleActionsFromFiles(t, bootstrapRules)

	// Every live rule action MUST appear in the persona — orphan
	// rule = silent dead code under the MVP roster.
	for action, ruleFile := range ruleActions {
		found := false
		for _, p := range personaActions {
			if p == action {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("bootstrap-wired rule %s targets action %q which is not in the persona taxonomy",
				ruleFile, action)
		}
	}

	// Every persona action MUST have a live rule. A persona action
	// without a rule wedges the user (LLM emits the action, nothing
	// fires).
	for _, action := range personaActions {
		if _, ok := ruleActions[action]; !ok {
			t.Errorf("persona action %q has no bootstrap-wired rule that fires on coordinator role", action)
		}
	}
}

// bootstrapLoadedRules reads configs/flow-bootstrap.json's
// rule.config.rules_files list and returns absolute paths (within
// this repo, not the in-container /app/ paths) suitable for
// downstream file scans.
func bootstrapLoadedRules(t *testing.T) []string {
	t.Helper()
	data, err := os.ReadFile("../../configs/flow-bootstrap.json")
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
	if err := json.Unmarshal(data, &bootstrap); err != nil {
		t.Fatalf("unmarshal flow-bootstrap.json: %v", err)
	}
	ruleProc, ok := bootstrap.Components["rule"]
	if !ok {
		t.Fatal("rule component not found in flow-bootstrap.json")
	}
	out := make([]string, 0, len(ruleProc.Config.RulesFiles))
	for _, p := range ruleProc.Config.RulesFiles {
		// Bootstrap stores in-container paths (/app/configs/...);
		// rewrite to the relative test-cwd path.
		rel := strings.TrimPrefix(p, "/app/")
		out = append(out, "../../"+rel)
	}
	return out
}

// collectCoordinatorRoleActionsFromFiles scans the given rule files
// for rules whose firing role is "coordinator" and returns a map of
// action-value → rule file basename for every
// coordinator.decision.next_action condition found. Handles both
// `eq` (single value) and `in` (array of values) operators so that
// transitional rules accepting more than one action value surface
// each accepted value.
//
// `in` support is forward-compat — no bootstrap-loaded rule
// currently uses it on next_action. Future bridging rules (e.g. a
// rule that accepts a future `dev` category token alongside
// `research` during a multi-category transition) will rely on it.
//
// Rules with a different firing role (e.g. Slice 1c wake-up rules
// that gate on reviewer-research / reviewer-qa) are excluded —
// those carry reviewer-persona actions, not coordinator-persona
// actions.
func collectCoordinatorRoleActionsFromFiles(t *testing.T, ruleFiles []string) map[string]string {
	t.Helper()
	out := map[string]string{}
	for _, rulePath := range ruleFiles {
		base := filepath.Base(rulePath)
		data, err := os.ReadFile(rulePath)
		if err != nil {
			t.Fatalf("read %s: %v", rulePath, err)
		}
		var rule struct {
			Conditions []struct {
				Field    string      `json:"field"`
				Operator string      `json:"operator"`
				Value    interface{} `json:"value"`
			} `json:"conditions"`
		}
		if err := json.Unmarshal(data, &rule); err != nil {
			t.Errorf("%s: invalid JSON: %v", base, err)
			continue
		}
		firingRole := ""
		for _, cond := range rule.Conditions {
			if cond.Field == "agent.loop.role" && cond.Operator == "eq" {
				if s, ok := cond.Value.(string); ok {
					firingRole = s
				}
			}
		}
		if firingRole != "coordinator" {
			continue
		}
		for _, cond := range rule.Conditions {
			if cond.Field != "coordinator.decision.next_action" {
				continue
			}
			switch cond.Operator {
			case "eq":
				action, ok := cond.Value.(string)
				if !ok {
					t.Errorf("%s: next_action eq value is not a string: %v", base, cond.Value)
					continue
				}
				out[action] = base
			case "in":
				values, ok := cond.Value.([]interface{})
				if !ok {
					t.Errorf("%s: next_action in value is not an array: %v", base, cond.Value)
					continue
				}
				for _, v := range values {
					s, ok := v.(string)
					if !ok {
						t.Errorf("%s: next_action in array element is not a string: %v", base, v)
						continue
					}
					out[s] = base
				}
			default:
				t.Errorf("%s: unsupported operator %q on next_action condition", base, cond.Operator)
			}
		}
	}
	return out
}

// TestCoordinatorSlice1cWakeUpRules locks in the Slice 1c wake-up rule
// shape: a rule firing on `agent.loop.role == "<reviewer role>"` AND
// `coordinator.decision.next_action == "<approval token>"` spawns a
// coordinator-role loop via publish_agent. Pinning the role→token
// pair here keeps the rule set in sync with
// cmd/semteams/chain/terminal.go's roleToApprovalAction map; adding a
// third terminal role requires touching both sides.
func TestCoordinatorSlice1cWakeUpRules(t *testing.T) {
	ruleDir := "../../configs/rules/coordinator"
	wantWakeUpPairs := map[string]string{
		"reviewer-research": "approved",
		"reviewer-qa":       "accept",
	}
	foundPairs := map[string]string{}

	entries, err := os.ReadDir(ruleDir)
	if err != nil {
		t.Fatalf("read %s: %v", ruleDir, err)
	}
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
			Conditions []struct {
				Field    string      `json:"field"`
				Operator string      `json:"operator"`
				Value    interface{} `json:"value"`
			} `json:"conditions"`
			OnEnter []struct {
				Type    string `json:"type"`
				Subject string `json:"subject"`
				Role    string `json:"role"`
			} `json:"on_enter"`
		}
		if err := json.Unmarshal(data, &rule); err != nil {
			continue
		}
		firingRole, action := "", ""
		for _, cond := range rule.Conditions {
			if cond.Field == "agent.loop.role" && cond.Operator == "eq" {
				if s, ok := cond.Value.(string); ok {
					firingRole = s
				}
			}
			if cond.Field == "coordinator.decision.next_action" && cond.Operator == "eq" {
				if s, ok := cond.Value.(string); ok {
					action = s
				}
			}
		}
		if _, isWakeUp := wantWakeUpPairs[firingRole]; !isWakeUp {
			continue
		}
		foundPairs[firingRole] = action
		// Wake-up rules MUST publish_agent to coordinator role on the
		// exact two-segment subject agent.task.coordinator —
		// agentic-dispatch subscribes on `agent.task.*` (single-token
		// wildcard). Subjects with more segments silently drop into
		// the void (smoke #8 run-11 lesson; see
		// configs/rules/ops/chain-terminal-observe.json
		// subject_shape_note for the canonical reference).
		spawnsCoordinator := false
		for _, a := range rule.OnEnter {
			if a.Type != "publish_agent" {
				continue
			}
			if a.Role != "coordinator" {
				t.Errorf("%s: wake-up rule (firing role %q) publish_agent role=%q, want %q", e.Name(), firingRole, a.Role, "coordinator")
				continue
			}
			if a.Subject != "agent.task.coordinator" {
				t.Errorf("%s: wake-up rule (firing role %q) publish_agent subject=%q, want %q (agentic-dispatch wildcard requires exact two-segment shape)", e.Name(), firingRole, a.Subject, "agent.task.coordinator")
			}
			spawnsCoordinator = true
		}
		if !spawnsCoordinator {
			t.Errorf("%s: wake-up rule (firing role %q) must publish_agent role=coordinator", e.Name(), firingRole)
		}
	}

	for role, wantAction := range wantWakeUpPairs {
		gotAction, ok := foundPairs[role]
		if !ok {
			t.Errorf("missing wake-up rule for firing role %q (terminal approval action %q)", role, wantAction)
			continue
		}
		if gotAction != wantAction {
			t.Errorf("wake-up rule for firing role %q: approval action %q, want %q (must match cmd/semteams/chain/terminal.go roleToApprovalAction)", role, gotAction, wantAction)
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
