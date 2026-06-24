package contract

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const devFromTaskPackDir = "../../configs/rules/dev-from-task"

type devFromTaskRuleJSON struct {
	ID         string                         `json:"id"`
	Type       string                         `json:"type"`
	Enabled    bool                           `json:"enabled"`
	Conditions []devFromTaskConditionJSON     `json:"conditions"`
	OnEnter    []devFromTaskOnEnterActionJSON `json:"on_enter"`
	Metadata   map[string]any                 `json:"metadata"`
}

type devFromTaskConditionJSON struct {
	Field    string `json:"field"`
	Operator string `json:"operator"`
	Value    any    `json:"value"`
	Required bool   `json:"required"`
}

type devFromTaskOnEnterActionJSON struct {
	ID              string            `json:"id,omitempty"`
	Type            string            `json:"type"`
	Subject         string            `json:"subject,omitempty"`
	Predicate       string            `json:"predicate,omitempty"`
	Object          any               `json:"object,omitempty"`
	Role            string            `json:"role,omitempty"`
	Tools           []string          `json:"tools,omitempty"`
	ActionAllowlist []string          `json:"action_allowlist,omitempty"`
	RelatedLoops    map[string]string `json:"related_loops,omitempty"`
	Prompt          string            `json:"prompt,omitempty"`
}

func TestDevFromTaskPack_RequestRulesStampExplicitRunMarker(t *testing.T) {
	files, err := filepath.Glob(filepath.Join(devFromTaskPackDir, "*.json"))
	if err != nil {
		t.Fatalf("glob dev-from-task pack: %v", err)
	}
	if len(files) != 3 {
		t.Fatalf("dev-from-task rule count = %d, want 3", len(files))
	}

	runAnchor := loadDevFromTaskRule(t, "01a-coordinator-request-run-anchor.json")
	assertDFTCondition(t, runAnchor, "agent.loop.role", "eq", "coordinator")
	assertDFTCondition(t, runAnchor, "coordinator.decision.next_action", "eq", "dev_from_task")
	assertDFTCondition(t, runAnchor, "agent.run.entity_id", "ne", "")
	assertDFTCondition(t, runAnchor, "lineage.run-loop-entity-id", "length_eq", float64(0))
	assertDFTTriple(t, runAnchor, "$entity.triple.agent.run.entity_id", "dev_from_task.requested", "$entity.instance")
	assertDFTTriple(t, runAnchor, "$entity.triple.agent.run.entity_id", "dev_from_task.requested_reason", "$entity.triple.coordinator.decision.reason")

	lineage := loadDevFromTaskRule(t, "01b-coordinator-request-lineage.json")
	assertDFTCondition(t, lineage, "agent.loop.role", "eq", "coordinator")
	assertDFTCondition(t, lineage, "coordinator.decision.next_action", "eq", "dev_from_task")
	assertDFTCondition(t, lineage, "lineage.run-loop-entity-id", "ne", "")
	assertDFTTriple(t, lineage, "$entity.triple.lineage.run-loop-entity-id", "dev_from_task.requested", "$entity.instance")
	assertDFTTriple(t, lineage, "$entity.triple.lineage.run-loop-entity-id", "dev_from_task.requested_reason", "$entity.triple.coordinator.decision.reason")
}

func TestDevFromTaskPack_ReadyRequestProjectsThenDispatchesOneRalph(t *testing.T) {
	rule := loadDevFromTaskRule(t, "02-ready-request-to-coordinator.json")
	assertDFTCondition(t, rule, "agent.run.outcome", "eq", "success")
	assertDFTCondition(t, rule, "proof_readiness.implementation_ready", "eq", "true")
	assertDFTCondition(t, rule, "dev_from_task.requested", "ne", "")
	assertDFTCondition(t, rule, "dev_from_task.started", "length_eq", float64(0))
	assertDFTTriple(t, rule, "$entity.id", "dev_from_task.started", "true")

	var spawn *devFromTaskOnEnterActionJSON
	for i := range rule.OnEnter {
		if rule.OnEnter[i].Type == "publish_agent" {
			spawn = &rule.OnEnter[i]
			break
		}
	}
	if spawn == nil {
		t.Fatalf("%s missing publish_agent action", rule.ID)
	}
	if spawn.Role != "coordinator" {
		t.Fatalf("spawn role = %q, want coordinator", spawn.Role)
	}
	for _, tool := range []string{
		"project_spec_tasks",
		"query_entity",
		"query_sandbox_attestation",
		"request_sandbox",
		"bash",
		"scratchpad",
		"decide",
	} {
		if !hasString(spawn.Tools, tool) {
			t.Errorf("dev-from-task coordinator tools missing %q; tools=%v", tool, spawn.Tools)
		}
	}
	for _, action := range []string{"dev_via_test", "ask_user", "respond_direct"} {
		if !hasString(spawn.ActionAllowlist, action) {
			t.Errorf("dev-from-task coordinator action_allowlist missing %q; allowlist=%v", action, spawn.ActionAllowlist)
		}
	}
	for _, forbidden := range []string{"create_change", "dev_from_task", "autoresearch"} {
		if hasString(spawn.ActionAllowlist, forbidden) {
			t.Errorf("dev-from-task coordinator action_allowlist includes %q; bridge must not recursively re-route itself", forbidden)
		}
	}
	if got := spawn.RelatedLoops["run-loop-entity-id"]; got != "$entity.id" {
		t.Fatalf("related run-loop-entity-id = %q, want $entity.id", got)
	}
	if got := spawn.RelatedLoops["dev-via-test-run"]; got != "$entity.id" {
		t.Fatalf("related dev-via-test-run = %q, want $entity.id", got)
	}
	for _, want := range []string{
		"project_spec_tasks()",
		"MUST NOT spawn Lisa",
		"proof_readiness.implementation_ready=true",
		"plan.done_authority.*",
		"git tag -f <plan.chain_start_git_tag>",
		"decide(action=\"dev_via_test\", subtopics=[\"<task-id>\"]",
		"Do not call dev_via_test without subtopics",
	} {
		if !strings.Contains(spawn.Prompt, want) {
			t.Errorf("dev-from-task coordinator prompt missing %q", want)
		}
	}
}

func TestDevFromTaskPackWiredInBothConfigs(t *testing.T) {
	files, err := filepath.Glob(filepath.Join(devFromTaskPackDir, "*.json"))
	if err != nil {
		t.Fatalf("glob dev-from-task pack: %v", err)
	}
	for _, cfg := range []string{"../../configs/flow-bootstrap.json", "../../configs/e2e-flow-bootstrap.json"} {
		data, err := os.ReadFile(cfg)
		if err != nil {
			t.Fatalf("read %s: %v", cfg, err)
		}
		body := string(data)
		for _, path := range files {
			want := "/app/configs/rules/dev-from-task/" + filepath.Base(path)
			if !strings.Contains(body, want) {
				t.Errorf("%s rules_files missing %q — dev-from-task bridge never loads at boot",
					filepath.Base(cfg), want)
			}
		}
	}
}

func TestDevFromTaskCoordinatorPersonaKnowsClosedToken(t *testing.T) {
	for _, path := range []string{
		"../../configs/personas/fragments/coordinator/10-decision-contract.md",
		"../../configs/personas/fragments/coordinator/20-delegation-rules.md",
	} {
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read %s: %v", path, err)
		}
		if !strings.Contains(string(data), "dev_from_task") {
			t.Fatalf("%s missing dev_from_task; coordinator would dead-end the new rule token", filepath.Base(path))
		}
	}
}

func loadDevFromTaskRule(t *testing.T, name string) *devFromTaskRuleJSON {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(devFromTaskPackDir, name))
	if err != nil {
		t.Fatalf("read %s: %v", name, err)
	}
	var rule devFromTaskRuleJSON
	if err := json.Unmarshal(data, &rule); err != nil {
		t.Fatalf("unmarshal %s: %v", name, err)
	}
	if rule.Type != "expression" || !rule.Enabled {
		t.Fatalf("%s type/enabled = %q/%v, want expression/true", name, rule.Type, rule.Enabled)
	}
	return &rule
}

func assertDFTCondition(t *testing.T, rule *devFromTaskRuleJSON, field, op string, value any) {
	t.Helper()
	for _, cond := range rule.Conditions {
		if cond.Field == field && cond.Operator == op && cond.Value == value && cond.Required {
			return
		}
	}
	t.Fatalf("%s missing required condition %s %s %v; conditions=%+v", rule.ID, field, op, value, rule.Conditions)
}

func assertDFTTriple(t *testing.T, rule *devFromTaskRuleJSON, subject, predicate string, object any) {
	t.Helper()
	for _, action := range rule.OnEnter {
		if action.Type == "add_triple" &&
			action.Subject == subject &&
			action.Predicate == predicate &&
			action.Object == object {
			return
		}
	}
	t.Fatalf("%s missing add_triple %s %s=%v; actions=%+v", rule.ID, subject, predicate, object, rule.OnEnter)
}

func hasString(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}
