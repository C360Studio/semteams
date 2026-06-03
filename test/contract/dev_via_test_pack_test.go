package contract

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Dev-via-test pack contract tests — structural invariants the
// Slice 1 spawn rule + persona dir must hold. Walks
// configs/rules/dev-via-test/ + configs/personas/fragments/ at test
// time; no runtime rule engine needed. Mirrors the
// autoresearch_rule_pack_test + research_pack_persona_dirs_test
// patterns.
//
// Per ADR-044 + CLAUDE.md "Reviewer-Pass Protocol" — these tests are
// the structural-correctness gate the Slice 1 design relies on.
// Substantive review (does Lisa's persona prose teach the Karpathy
// contract well?) is the reviewer-pass's job, not this test's.

const devViaTestPackDir = "../../configs/rules/dev-via-test"

// TestDevViaTestPack_01_CoordinatorSpawn pins the Slice 1 spawn
// rule shape. The pack's whole iteration loop (Slices 2-4) hangs
// off this one rule's terminal contract — wrong role, wrong tools,
// wrong action_allowlist all silently dead-end the chain.
//
// Invariants:
//
//  1. Conditions match coordinator + decide(dev_via_test).
//  2. Spawns role "dev-via-test-plan" (Lisa).
//  3. Pins related_loops["run-loop-entity-id"] = $entity.id so
//     emit_dev_via_test_plan can stamp triples on the coordinator's
//     loop entity (mirrors autoresearch pattern).
//  4. Lisa's tools include the Karpathy-emit tool (otherwise
//     planning is theatre); decide; bash (for git tag); scratchpad
//     (for thinking); read_loop_result (for reading user ask).
//  5. action_allowlist constrains to planned + needs_clarification
//     so Lisa can't free-form into autoresearch / research / etc.
//  6. tool_choice.mode = required so the planner doesn't text-out
//     of the strict-schema emit step (semstreams#158 class).
//  7. Stamps dev_via_test.run.status = active so future "is this
//     chain still running dev_via_test" gates have something to
//     match on (Slice 3 walker uses this).
func TestDevViaTestPack_01_CoordinatorSpawn(t *testing.T) {
	rule := loadDevViaTestRule(t, "01-coordinator-dev-via-test-spawn.json")

	// Invariant 1: coordinator + decide(dev_via_test) conditions.
	if !devViaTestRoleCondition(rule, "coordinator") {
		t.Error("rule 01 does not condition on agent.loop.role = coordinator")
	}
	if !devViaTestActionCondition(rule, "dev_via_test") {
		t.Error("rule 01 does not condition on coordinator.decision.next_action = dev_via_test")
	}

	// Find the publish_agent action for Lisa.
	var spawn *devViaTestOnEnterJSON
	for i := range rule.OnEnter {
		if rule.OnEnter[i].Type == "publish_agent" && rule.OnEnter[i].Role == "dev-via-test-plan" {
			spawn = &rule.OnEnter[i]
			break
		}
	}
	if spawn == nil {
		t.Fatal("rule 01 has no publish_agent for dev-via-test-plan (Lisa)")
	}

	// Invariant 3: related_loops pins run-loop-entity-id.
	related, _ := spawn.RelatedLoops["run-loop-entity-id"]
	if related != "$entity.id" {
		t.Errorf("rule 01 related_loops[run-loop-entity-id] = %q; want %q so emit_dev_via_test_plan can stamp on the coordinator's loop entity",
			related, "$entity.id")
	}

	// Invariant 4: Lisa's tools.
	wantTools := []string{"read_loop_result", "bash", "emit_dev_via_test_plan", "scratchpad", "decide"}
	for _, want := range wantTools {
		if !devViaTestSliceHas(spawn.Tools, want) {
			t.Errorf("rule 01 dev-via-test-plan tools missing %q; current = %v", want, spawn.Tools)
		}
	}

	// Invariant 5: action_allowlist.
	wantActions := []string{"planned", "needs_clarification"}
	if len(spawn.ActionAllowed) != len(wantActions) {
		t.Errorf("rule 01 action_allowlist = %v; want exactly %v (other actions silently dead-end the chain)",
			spawn.ActionAllowed, wantActions)
	}
	for _, want := range wantActions {
		if !devViaTestSliceHas(spawn.ActionAllowed, want) {
			t.Errorf("rule 01 action_allowlist missing %q", want)
		}
	}

	// Invariant 6: tool_choice required.
	if spawn.ToolChoice == nil {
		t.Error("rule 01 spawn missing tool_choice — gemini-flash may text-out of emit_dev_via_test_plan (semstreams#158 class)")
	} else if mode, _ := spawn.ToolChoice["mode"].(string); mode != "required" {
		t.Errorf("rule 01 tool_choice.mode = %q; want %q", mode, "required")
	}

	// Invariant 7: run.status active stamp.
	var foundRunStatus bool
	for _, a := range rule.OnEnter {
		if a.Type != "add_triple" || a.Predicate != "dev_via_test.run.status" {
			continue
		}
		if obj, ok := a.Object.(string); !ok || obj != "active" {
			t.Errorf("rule 01 dev_via_test.run.status add_triple object = %v; want %q", a.Object, "active")
		}
		foundRunStatus = true
	}
	if !foundRunStatus {
		t.Error("rule 01 missing add_triple for dev_via_test.run.status='active' — Slice 3 walker has no gate to match on")
	}
}

// TestDevViaTestPackPersonaDirsExist asserts that every role token
// the dev-via-test pack spawns has a corresponding persona fragment
// directory. Slice 1 only spawns dev-via-test-plan (Lisa); Slices
// 2-4 will add dev-via-test-execute (Ralph) and reviewer-dev-via-test
// (CBG).
//
// Mirrors TestResearchPackPersonaDirsExist's pattern: enumerate
// publish_agent targets, assert each has a directory with at least
// 00-identity.md.
func TestDevViaTestPackPersonaDirsExist(t *testing.T) {
	files, err := filepath.Glob(filepath.Join(devViaTestPackDir, "*.json"))
	if err != nil {
		t.Fatalf("glob dev-via-test pack: %v", err)
	}
	if len(files) == 0 {
		t.Fatal("no rule files found in dev-via-test pack — wrong working directory?")
	}
	spawned := make(map[string]bool)
	for _, path := range files {
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read %s: %v", path, err)
		}
		var rule devViaTestRuleJSON
		if err := json.Unmarshal(data, &rule); err != nil {
			t.Fatalf("unmarshal %s: %v", path, err)
		}
		for _, a := range rule.OnEnter {
			if a.Type == "publish_agent" && a.Role != "" {
				spawned[a.Role] = true
			}
		}
	}

	root := "../../configs/personas/fragments"
	for role := range spawned {
		identityPath := filepath.Join(root, role, "00-identity.md")
		info, err := os.Stat(identityPath)
		if err != nil {
			t.Errorf("role %q has no persona at %s: %v — add the directory + 00-identity.md, or remove the rule that spawns it",
				role, identityPath, err)
			continue
		}
		if info.Size() == 0 {
			t.Errorf("role %q identity fragment at %s is empty", role, identityPath)
		}
	}
}

// TestDevViaTestPackPlanFragmentCarriesKarpathy asserts the Lisa
// persona fragment(s) reference the Karpathy guidelines structurally.
// Drift signal: if a future refactor strips the Karpathy framing
// from the persona, this test fails and surfaces the regression
// before it ships.
//
// The schema enforcement lives in emit_dev_via_test_plan (Go
// validator), but the persona has to TEACH the LLM what the schema
// means or the LLM will retry-loop on validation errors. Both are
// load-bearing.
func TestDevViaTestPackPlanFragmentCarriesKarpathy(t *testing.T) {
	root := "../../configs/personas/fragments/dev-via-test-plan"
	got, err := concatFragmentsDevViaTest(root)
	if err != nil {
		t.Fatalf("read dev-via-test-plan fragments: %v", err)
	}
	// Each Karpathy rule should show up by number or by named field.
	// Match loosely so persona authors can phrase things naturally.
	for _, want := range []string{
		"Karpathy Rule 1", "Karpathy Rule 2", "Karpathy Rule 3", "Karpathy Rule 4",
		"assumptions", "non_goals", "target_files", "test_command",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("dev-via-test-plan persona missing %q — the persona must teach the schema's required fields explicitly", want)
		}
	}
}

// TestCoordinatorDecisionContractHasDevViaTest asserts the
// coordinator persona's decision-contract fragment lists
// `dev_via_test` as a valid action token. Without this, the
// coordinator either invents the token (silently dead-ending the
// chain when the rule layer doesn't match) or never picks it
// (dev-via-test is never reachable).
func TestCoordinatorDecisionContractHasDevViaTest(t *testing.T) {
	path := "../../configs/personas/fragments/coordinator/10-decision-contract.md"
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	text := string(data)
	if !strings.Contains(text, "`dev_via_test`") {
		t.Error("coordinator 10-decision-contract.md does not list `dev_via_test` as a valid action — token is unreachable")
	}
	if !strings.Contains(text, "Lisa") || !strings.Contains(text, "Ralph") || !strings.Contains(text, "CBG") {
		t.Error("coordinator 10-decision-contract.md dev_via_test entry should reference the role names Lisa/Ralph/CBG so operators reading the persona can connect taxonomy → chain shape")
	}
}

// TestDevViaTestPackWiredInFlowBootstrap asserts the Slice 1 spawn
// rule's filename appears in flow-bootstrap.json's rules_files list.
// Without this, the rule never loads at boot and Lisa never spawns
// regardless of how good the rule JSON itself is. Mirrors the
// rules_files_paths_test posture.
func TestDevViaTestPackWiredInFlowBootstrap(t *testing.T) {
	data, err := os.ReadFile("../../configs/flow-bootstrap.json")
	if err != nil {
		t.Fatalf("read flow-bootstrap.json: %v", err)
	}
	want := "/app/configs/rules/dev-via-test/01-coordinator-dev-via-test-spawn.json"
	if !strings.Contains(string(data), want) {
		t.Errorf("flow-bootstrap.json rules_files missing %q — Slice 1 spawn rule never loads at boot", want)
	}
}

// --- types + helpers (mirror autoresearchRuleJSON shape) ---

type devViaTestRuleJSON struct {
	ID         string                    `json:"id"`
	Type       string                    `json:"type"`
	Enabled    bool                      `json:"enabled"`
	Conditions []devViaTestConditionJSON `json:"conditions"`
	OnEnter    []devViaTestOnEnterJSON   `json:"on_enter"`
	Metadata   map[string]any            `json:"metadata"`
}

type devViaTestConditionJSON struct {
	Field    string `json:"field"`
	Operator string `json:"operator"`
	Value    any    `json:"value"`
	Required bool   `json:"required"`
}

type devViaTestOnEnterJSON struct {
	Type          string                    `json:"type"`
	Role          string                    `json:"role,omitempty"`
	Subject       string                    `json:"subject,omitempty"`
	Predicate     string                    `json:"predicate,omitempty"`
	Object        any                       `json:"object,omitempty"`
	Tools         []string                  `json:"tools,omitempty"`
	ToolChoice    map[string]any            `json:"tool_choice,omitempty"`
	ActionAllowed []string                  `json:"action_allowlist,omitempty"`
	RelatedLoops  map[string]string         `json:"related_loops,omitempty"`
	When          []devViaTestConditionJSON `json:"when,omitempty"`
}

func loadDevViaTestRule(t *testing.T, name string) *devViaTestRuleJSON {
	t.Helper()
	path := filepath.Join(devViaTestPackDir, name)
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	var r devViaTestRuleJSON
	if err := json.Unmarshal(data, &r); err != nil {
		t.Fatalf("unmarshal %s: %v", path, err)
	}
	return &r
}

func devViaTestRoleCondition(r *devViaTestRuleJSON, role string) bool {
	for _, c := range r.Conditions {
		if c.Field == "agent.loop.role" && c.Operator == "eq" {
			if s, ok := c.Value.(string); ok && s == role {
				return true
			}
		}
	}
	return false
}

func devViaTestActionCondition(r *devViaTestRuleJSON, action string) bool {
	for _, c := range r.Conditions {
		if c.Field == "coordinator.decision.next_action" && c.Operator == "eq" {
			if s, ok := c.Value.(string); ok && s == action {
				return true
			}
		}
	}
	return false
}

func devViaTestSliceHas(slice []string, want string) bool {
	for _, s := range slice {
		if s == want {
			return true
		}
	}
	return false
}

// concatFragmentsDevViaTest reads all .md files in dir and concatenates
// them. Used for "persona corpus contains X" tests where the exact
// fragment file matters less than the corpus carrying the concept.
func concatFragmentsDevViaTest(dir string) (string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return "", err
	}
	var b strings.Builder
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".md") {
			continue
		}
		data, err := os.ReadFile(filepath.Join(dir, e.Name()))
		if err != nil {
			return "", err
		}
		b.Write(data)
		b.WriteString("\n")
	}
	return b.String(), nil
}
