package contract

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/c360studio/semteams/cmd/semteams/tools/emitchange"
)

// Create-change pack contract tests (ADR-057 §D5) — structural
// invariants the spawn → author → reviewer-gate → coordinator-wake-up
// arc must hold. Walks configs/rules/create-change/ +
// configs/personas/fragments/ at test time; no runtime rule engine
// needed. Mirrors the research / dev-via-test pack test patterns.
//
// Per CLAUDE.md "Reviewer-Pass Protocol" these are the
// structural-correctness gate; substantive review (does the persona
// prose teach the OpenSpec contract well?) is the reviewer-pass's job.

const createChangePackDir = "../../configs/rules/create-change"

// createChangeRunLoopKey is the related_loops key rule 01 pins so
// emit_change can resolve the run entity. It MUST match
// emitchange's runLoopIDRoleKey ("create-change-run") — the literal
// lives in both the rule JSON and the executor; this test is the
// drift guard (mirrors dev-via-test's plan-start literal pin). If
// emit_change ever changes the key, this test must change with it.
const createChangeRunLoopKey = "create-change-run"

type createChangeRuleJSON struct {
	ID         string                      `json:"id"`
	Type       string                      `json:"type"`
	Enabled    bool                        `json:"enabled"`
	Conditions []createChangeConditionJSON `json:"conditions"`
	OnEnter    []createChangeOnEnterJSON   `json:"on_enter"`
	Metadata   map[string]any              `json:"metadata"`
}

type createChangeConditionJSON struct {
	Field    string `json:"field"`
	Operator string `json:"operator"`
	Value    any    `json:"value"`
	Required bool   `json:"required"`
}

type createChangeOnEnterJSON struct {
	Type          string            `json:"type"`
	RunScope      string            `json:"run_scope,omitempty"`
	Role          string            `json:"role,omitempty"`
	Subject       string            `json:"subject,omitempty"`
	Predicate     string            `json:"predicate,omitempty"`
	Object        any               `json:"object,omitempty"`
	Tools         []string          `json:"tools,omitempty"`
	ActionAllowed []string          `json:"action_allowlist,omitempty"`
	ToolChoice    map[string]any    `json:"tool_choice,omitempty"`
	Properties    map[string]any    `json:"properties,omitempty"`
	RelatedLoops  map[string]string `json:"related_loops,omitempty"`
}

// TestCreateChangePack_01_Spawn pins the spawn rule. The whole pack
// hangs off this rule's terminal contract — wrong role, wrong tools,
// wrong action_allowlist, or a wrong related_loops key all silently
// dead-end the chain or leave emit_change unable to resolve the run
// entity.
//
//  1. Conditions: coordinator + decide(create_change).
//  2. run_scope=new (the coordinator loop becomes the run anchor).
//  3. Spawns role "author-create-change".
//  4. related_loops["create-change-run"] = $entity.instance — the
//     load-bearing wiring emit_change reads to resolve the run entity.
//  5. Tools include emit_change (otherwise authoring is theatre) +
//     decide + bash + scratchpad + read_loop_result.
//  6. action_allowlist == [drafted, needs_clarification].
//  7. tool_choice.mode = required (don't text-out of the strict emit).
func TestCreateChangePack_01_Spawn(t *testing.T) {
	rule := loadCreateChangeRule(t, "01-coordinator-create-change-spawn.json")

	if !ccRoleCondition(rule, "coordinator") {
		t.Error("rule 01 does not condition on agent.loop.role = coordinator")
	}
	if !ccActionCondition(rule, "create_change") {
		t.Error("rule 01 does not condition on coordinator.decision.next_action = create_change")
	}

	spawn := ccFindSpawn(rule, "author-create-change")
	if spawn == nil {
		t.Fatal("rule 01 has no publish_agent for role=author-create-change")
	}

	if spawn.RunScope != "new" {
		t.Errorf("rule 01 run_scope = %q; want %q (the coordinator loop must mint the run anchor)", spawn.RunScope, "new")
	}

	// Invariant 4: the load-bearing related_loops key. emit_change reads
	// related_loops[createChangeRunLoopKey] to derive the run entity id;
	// a wrong key here makes every emit_change call fail with "related_loops
	// missing".
	if got := spawn.RelatedLoops[createChangeRunLoopKey]; got != "$entity.instance" {
		t.Errorf("rule 01 related_loops[%q] = %q; want %q so emit_change can derive the run entity id via TryChainExecutionEntityID",
			createChangeRunLoopKey, got, "$entity.instance")
	}

	for _, want := range []string{"read_loop_result", "bash", emitchange.ToolName, "scratchpad", "decide"} {
		if !ccSliceHas(spawn.Tools, want) {
			t.Errorf("rule 01 author tools missing %q; current = %v", want, spawn.Tools)
		}
	}

	wantActions := []string{"drafted", "needs_clarification"}
	if len(spawn.ActionAllowed) != len(wantActions) {
		t.Errorf("rule 01 action_allowlist = %v; want exactly %v (other actions silently dead-end the chain)", spawn.ActionAllowed, wantActions)
	}
	for _, want := range wantActions {
		if !ccSliceHas(spawn.ActionAllowed, want) {
			t.Errorf("rule 01 action_allowlist missing %q", want)
		}
	}

	if spawn.ToolChoice == nil {
		t.Error("rule 01 spawn missing tool_choice — the author may text-out of emit_change (semstreams#158 class)")
	} else if mode, _ := spawn.ToolChoice["mode"].(string); mode != "required" {
		t.Errorf("rule 01 tool_choice.mode = %q; want %q", mode, "required")
	}
}

// TestCreateChangePack_02_AuthorToReviewer pins the forward edge.
//
//  1. Conditions: role=author-create-change, outcome=success,
//     next_action=drafted.
//  2. Spawns reviewer-create-change (NOT a coordinator).
//  3. Reviewer tools include query_entity (reads ask + change in one
//     call off the run entity) + read_loop_result + decide.
//  4. action_allowlist == [approved, rejected, needs_clarification].
func TestCreateChangePack_02_AuthorToReviewer(t *testing.T) {
	rule := loadCreateChangeRule(t, "02-author-to-reviewer.json")

	if !ccRoleCondition(rule, "author-create-change") {
		t.Error("rule 02 does not condition on role=author-create-change")
	}
	if !ccOutcomeCondition(rule, "success") {
		t.Error("rule 02 does not condition on outcome=success")
	}
	if !ccNextActionCondition(rule, "drafted") {
		t.Error("rule 02 does not condition on next_action=drafted")
	}

	spawn := ccFindSpawn(rule, "reviewer-create-change")
	if spawn == nil {
		t.Fatal("rule 02 has no publish_agent for role=reviewer-create-change")
	}

	for _, want := range []string{"read_loop_result", "query_entity", "decide"} {
		if !ccSliceHas(spawn.Tools, want) {
			t.Errorf("rule 02 reviewer tools missing %q; current = %v", want, spawn.Tools)
		}
	}

	wantActions := []string{"approved", "rejected", "needs_clarification"}
	if len(spawn.ActionAllowed) != len(wantActions) {
		t.Errorf("rule 02 action_allowlist = %v; want exactly %v", spawn.ActionAllowed, wantActions)
	}
	for _, want := range wantActions {
		if !ccSliceHas(spawn.ActionAllowed, want) {
			t.Errorf("rule 02 action_allowlist missing %q", want)
		}
	}
}

// TestCreateChangePack_03_ApprovedToCoordinator pins the chain
// terminal: approved stamps agent.run.outcome=success on the run
// entity (drives agent-run/03 → completed) and wakes a coordinator
// restricted to respond_direct/ask_user (a wake-up that could
// re-classify would loop the chain).
func TestCreateChangePack_03_ApprovedToCoordinator(t *testing.T) {
	rule := loadCreateChangeRule(t, "03-reviewer-approved-to-coordinator.json")

	if !ccRoleCondition(rule, "reviewer-create-change") {
		t.Error("rule 03 does not condition on role=reviewer-create-change")
	}
	if !ccNextActionCondition(rule, "approved") {
		t.Error("rule 03 does not condition on next_action=approved")
	}

	// Success stamp on the run entity.
	var stamped *createChangeOnEnterJSON
	for i := range rule.OnEnter {
		if rule.OnEnter[i].Type == "add_triple" && rule.OnEnter[i].Predicate == "agent.run.outcome" {
			stamped = &rule.OnEnter[i]
			break
		}
	}
	if stamped == nil {
		t.Fatal("rule 03 does not stamp agent.run.outcome — the run never completes (agent-run/03 has nothing to consume)")
	}
	if stamped.Subject != "$entity.triple.agent.run.entity_id" {
		t.Errorf("rule 03 outcome subject = %q; want the run entity ($entity.triple.agent.run.entity_id)", stamped.Subject)
	}
	if obj, _ := stamped.Object.(string); obj != "success" {
		t.Errorf("rule 03 outcome object = %v; want \"success\"", stamped.Object)
	}

	spawn := ccFindSpawn(rule, "coordinator")
	if spawn == nil {
		t.Fatal("rule 03 has no coordinator wake-up spawn")
	}
	wantActions := []string{"respond_direct", "ask_user"}
	if len(spawn.ActionAllowed) != len(wantActions) {
		t.Errorf("rule 03 wake-up allowlist = %v; want exactly %v (a re-classify token would loop the chain)", spawn.ActionAllowed, wantActions)
	}
	for _, want := range wantActions {
		if !ccSliceHas(spawn.ActionAllowed, want) {
			t.Errorf("rule 03 wake-up allowlist missing %q", want)
		}
	}
}

// TestCreateChangePack_04_RejectedToCoordinator pins the reject
// recovery: rejected routes to the coordinator with create_change in
// the allowlist (re-dispatch a fresh change — supersession, since v1
// emit_change has no remover for in-place re-author).
func TestCreateChangePack_04_RejectedToCoordinator(t *testing.T) {
	rule := loadCreateChangeRule(t, "04-reviewer-rejected-to-coordinator.json")

	if !ccRoleCondition(rule, "reviewer-create-change") {
		t.Error("rule 04 does not condition on role=reviewer-create-change")
	}
	if !ccNextActionCondition(rule, "rejected") {
		t.Error("rule 04 does not condition on next_action=rejected")
	}
	spawn := ccFindSpawn(rule, "coordinator")
	if spawn == nil {
		t.Fatal("rule 04 has no coordinator spawn")
	}
	if !ccSliceHas(spawn.ActionAllowed, "create_change") {
		t.Error("rule 04 coordinator allowlist missing create_change — cannot re-dispatch a tightened change")
	}
}

// TestCreateChangePack_05_NeedsClarification pins the ADR-039
// recovery: needs_clarification from EITHER pack role routes to the
// coordinator (ask_user / re-dispatch / respond).
func TestCreateChangePack_05_NeedsClarification(t *testing.T) {
	rule := loadCreateChangeRule(t, "05-needs-clarification-to-coordinator.json")

	if !ccNextActionCondition(rule, "needs_clarification") {
		t.Error("rule 05 does not condition on next_action=needs_clarification")
	}
	// Role condition must be an `in` over both pack roles.
	roles := ccRoleInValues(rule)
	for _, want := range []string{"author-create-change", "reviewer-create-change"} {
		if !ccSliceHas(roles, want) {
			t.Errorf("rule 05 role `in` list missing %q (got %v) — that role's needs_clarification would wedge", want, roles)
		}
	}
	spawn := ccFindSpawn(rule, "coordinator")
	if spawn == nil {
		t.Fatal("rule 05 has no coordinator spawn")
	}
	if !ccSliceHas(spawn.ActionAllowed, "ask_user") {
		t.Error("rule 05 coordinator allowlist missing ask_user — the canonical clarification route")
	}
}

// TestCreateChangePack_06_07_LoopFailed pins the two failure
// handlers: 06 stamps chain.paused.marker (operator surface) and 07
// stamps agent.run.outcome=failed on the run entity (drives
// executing→failed). Both list exactly the pack roles.
func TestCreateChangePack_06_07_LoopFailed(t *testing.T) {
	pause := loadCreateChangeRule(t, "06-loop-failed-pause.json")
	if !ccHasCondition(pause, "agent.loop.outcome", "in") {
		t.Error("rule 06 does not condition on agent.loop.outcome in [failed, truncated]")
	}
	var marker bool
	for _, a := range pause.OnEnter {
		if a.Type == "add_triple" && a.Predicate == "chain.paused.marker" {
			marker = true
		}
	}
	if !marker {
		t.Error("rule 06 does not stamp chain.paused.marker — a failed loop dies silently (no operator surface)")
	}

	outcome := loadCreateChangeRule(t, "07-loop-failed-run-outcome.json")
	if !ccHasCondition(outcome, "agent.run.entity_id", "ne") {
		t.Error("rule 07 missing agent.run.entity_id ne \"\" guard — a run-less failure would stamp a malformed subject")
	}
	var failStamp *createChangeOnEnterJSON
	for i := range outcome.OnEnter {
		if outcome.OnEnter[i].Predicate == "agent.run.outcome" {
			failStamp = &outcome.OnEnter[i]
		}
	}
	if failStamp == nil {
		t.Fatal("rule 07 does not stamp agent.run.outcome — failed loop wedges the run in executing forever")
	}
	if failStamp.Subject != "$entity.triple.agent.run.entity_id" {
		t.Errorf("rule 07 outcome subject = %q; want the run entity", failStamp.Subject)
	}
	if obj, _ := failStamp.Object.(string); obj != "failed" {
		t.Errorf("rule 07 outcome object = %v; want \"failed\"", failStamp.Object)
	}
}

// TestCreateChangePackPersonaDirsExist asserts every role token the
// pack spawns (excluding coordinator, which has its own corpus) has a
// persona fragment dir with a non-empty 00-identity.md. Mirrors
// TestDevViaTestPackPersonaDirsExist.
func TestCreateChangePackPersonaDirsExist(t *testing.T) {
	files, err := filepath.Glob(filepath.Join(createChangePackDir, "*.json"))
	if err != nil {
		t.Fatalf("glob create-change pack: %v", err)
	}
	if len(files) == 0 {
		t.Fatal("no rule files found in create-change pack — wrong working directory?")
	}
	spawned := make(map[string]bool)
	for _, path := range files {
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read %s: %v", path, err)
		}
		var rule createChangeRuleJSON
		if err := json.Unmarshal(data, &rule); err != nil {
			t.Fatalf("unmarshal %s: %v", path, err)
		}
		for _, a := range rule.OnEnter {
			if a.Type == "publish_agent" && a.Role != "" && a.Role != "coordinator" {
				spawned[a.Role] = true
			}
		}
	}
	// The pack's own roles must be present.
	for _, want := range []string{"author-create-change", "reviewer-create-change"} {
		if !spawned[want] {
			t.Errorf("pack never spawns role %q — pack/test drift", want)
		}
	}
	root := "../../configs/personas/fragments"
	for role := range spawned {
		identityPath := filepath.Join(root, role, "00-identity.md")
		info, err := os.Stat(identityPath)
		if err != nil {
			t.Errorf("role %q has no persona at %s: %v", role, identityPath, err)
			continue
		}
		if info.Size() == 0 {
			t.Errorf("role %q identity fragment at %s is empty", role, identityPath)
		}
	}
}

// TestCoordinatorDecisionContractHasCreateChange asserts the
// coordinator persona lists create_change as a valid action token.
// Without it the coordinator either invents the token (dead-ending
// the chain) or never picks it (create-change unreachable).
func TestCoordinatorDecisionContractHasCreateChange(t *testing.T) {
	path := "../../configs/personas/fragments/coordinator/10-decision-contract.md"
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	if !strings.Contains(string(data), "`create_change`") {
		t.Error("coordinator 10-decision-contract.md does not list `create_change` — token is unreachable")
	}
}

// TestCreateChangePackWiredInBothConfigs asserts every pack rule
// filename appears in BOTH flow-bootstrap.json (prod) and
// e2e-flow-bootstrap.json (mock-LLM journeys). Without this the rules
// never load at boot and the pack is dead regardless of the JSON
// quality. The e2e wiring is what the P2 mock-LLM journey gate runs
// against.
func TestCreateChangePackWiredInBothConfigs(t *testing.T) {
	files, err := filepath.Glob(filepath.Join(createChangePackDir, "*.json"))
	if err != nil {
		t.Fatalf("glob create-change pack: %v", err)
	}
	for _, cfg := range []string{"../../configs/flow-bootstrap.json", "../../configs/e2e-flow-bootstrap.json"} {
		data, err := os.ReadFile(cfg)
		if err != nil {
			t.Fatalf("read %s: %v", cfg, err)
		}
		body := string(data)
		for _, path := range files {
			want := "/app/configs/rules/create-change/" + filepath.Base(path)
			if !strings.Contains(body, want) {
				t.Errorf("%s rules_files missing %q — rule never loads at boot", filepath.Base(cfg), want)
			}
		}
	}
}

// --- helpers ---

func loadCreateChangeRule(t *testing.T, name string) *createChangeRuleJSON {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(createChangePackDir, name))
	if err != nil {
		t.Fatalf("read %s: %v", name, err)
	}
	var rule createChangeRuleJSON
	if err := json.Unmarshal(data, &rule); err != nil {
		t.Fatalf("unmarshal %s: %v", name, err)
	}
	return &rule
}

func ccFindSpawn(r *createChangeRuleJSON, role string) *createChangeOnEnterJSON {
	for i := range r.OnEnter {
		if r.OnEnter[i].Type == "publish_agent" && r.OnEnter[i].Role == role {
			return &r.OnEnter[i]
		}
	}
	return nil
}

func ccRoleCondition(r *createChangeRuleJSON, role string) bool {
	for _, c := range r.Conditions {
		if c.Field == "agent.loop.role" && c.Operator == "eq" {
			if s, ok := c.Value.(string); ok && s == role {
				return true
			}
		}
	}
	return false
}

// ccRoleInValues returns the agent.loop.role `in` condition's value list.
func ccRoleInValues(r *createChangeRuleJSON) []string {
	for _, c := range r.Conditions {
		if c.Field == "agent.loop.role" && c.Operator == "in" {
			raw, _ := c.Value.([]any)
			out := make([]string, 0, len(raw))
			for _, v := range raw {
				if s, ok := v.(string); ok {
					out = append(out, s)
				}
			}
			return out
		}
	}
	return nil
}

func ccActionCondition(r *createChangeRuleJSON, action string) bool {
	return ccFieldEq(r, "coordinator.decision.next_action", action)
}

func ccNextActionCondition(r *createChangeRuleJSON, action string) bool {
	return ccFieldEq(r, "coordinator.decision.next_action", action)
}

func ccOutcomeCondition(r *createChangeRuleJSON, outcome string) bool {
	return ccFieldEq(r, "agent.loop.outcome", outcome)
}

func ccFieldEq(r *createChangeRuleJSON, field, value string) bool {
	for _, c := range r.Conditions {
		if c.Field == field && c.Operator == "eq" {
			if s, ok := c.Value.(string); ok && s == value {
				return true
			}
		}
	}
	return false
}

func ccHasCondition(r *createChangeRuleJSON, field, operator string) bool {
	for _, c := range r.Conditions {
		if c.Field == field && c.Operator == operator {
			return true
		}
	}
	return false
}

func ccSliceHas(slice []string, want string) bool {
	for _, s := range slice {
		if s == want {
			return true
		}
	}
	return false
}
