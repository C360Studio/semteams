package contract

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
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

// TestDevViaTestChainStartTagLiteralConsistency pins the
// `plan-start` literal across the executor constant + Lisa's spawn
// prompt + the persona's git-tag instruction. Per reviewer N4
// (Slice 1 review): if any one of these three drifts, CBG (Slice
// 4) cannot diff the chain-end state against the chain-start
// snapshot — silent failure mode that would surface only when
// Slice 4 runs.
//
// The literal lives in three places by design (no string substitution
// across rule/persona boundaries today); this test ensures all three
// stay in sync.
func TestDevViaTestChainStartTagLiteralConsistency(t *testing.T) {
	const literal = "plan-start"

	// Source 1: spawn rule prompt (rule 01's instruction to Lisa).
	ruleData, err := os.ReadFile(filepath.Join(devViaTestPackDir, "01-coordinator-dev-via-test-spawn.json"))
	if err != nil {
		t.Fatalf("read rule 01: %v", err)
	}
	if !strings.Contains(string(ruleData), "git tag "+literal) {
		t.Errorf("rule 01 spawn prompt does not contain %q — Lisa won't create the tag CBG diffs against", "git tag "+literal)
	}

	// Source 2: Lisa's persona 00-identity.md (the redundant instruction
	// so the persona reads coherently when read standalone).
	personaData, err := os.ReadFile("../../configs/personas/fragments/dev-via-test-plan/00-identity.md")
	if err != nil {
		t.Fatalf("read 00-identity.md: %v", err)
	}
	if !strings.Contains(string(personaData), "git tag "+literal) {
		t.Errorf("dev-via-test-plan 00-identity.md does not contain %q", "git tag "+literal)
	}

	// Source 3: executor's defaultChainStartGitTag constant (via the
	// triple it stamps). We check the README's documented value since
	// the executor's constant is unexported; pack README is authoritative
	// for what downstream consumers (Slice 4 CBG) will read.
	readmeData, err := os.ReadFile(filepath.Join(devViaTestPackDir, "README.md"))
	if err != nil {
		t.Fatalf("read pack README: %v", err)
	}
	if !strings.Contains(string(readmeData), `plan.chain_start_git_tag        = "`+literal+`"`) {
		t.Errorf("pack README does not document plan.chain_start_git_tag = %q — consumer drift risk", literal)
	}
}

// TestDevViaTestPackWiredInFlowBootstrap asserts the Slice 1-4
// rule filenames appear in flow-bootstrap.json's rules_files list.
// Without this, the rules never load at boot and the pack is dead
// regardless of how good the rule JSON itself is. Mirrors the
// rules_files_paths_test posture.
func TestDevViaTestPackWiredInFlowBootstrap(t *testing.T) {
	data, err := os.ReadFile("../../configs/flow-bootstrap.json")
	if err != nil {
		t.Fatalf("read flow-bootstrap.json: %v", err)
	}
	body := string(data)
	for _, want := range []string{
		"/app/configs/rules/dev-via-test/01-coordinator-dev-via-test-spawn.json",
		"/app/configs/rules/dev-via-test/02-lisa-terminal-to-plan-review.json",
		"/app/configs/rules/dev-via-test/02b-plan-approved-to-walker.json",
		"/app/configs/rules/dev-via-test/02c-plan-retry-stamp.json",
		"/app/configs/rules/dev-via-test/02d-plan-retry-driver.json",
		"/app/configs/rules/dev-via-test/02e-plan-rejected-to-coordinator.json",
		"/app/configs/rules/dev-via-test/02f-lisa-needs-clarification-to-coordinator.json",
		"/app/configs/rules/dev-via-test/03-coordinator-dispatch-ralph.json",
		"/app/configs/rules/dev-via-test/04a-execute-stamp-converged.json",
		"/app/configs/rules/dev-via-test/04b-execute-stamp-failed.json",
		"/app/configs/rules/dev-via-test/05-ralph-terminal-to-walker.json",
		"/app/configs/rules/dev-via-test/06-coordinator-dispatch-cbg.json",
		"/app/configs/rules/dev-via-test/07a-cbg-approved-to-coordinator.json",
		"/app/configs/rules/dev-via-test/07b-cbg-rejected-to-coordinator.json",
		"/app/configs/rules/dev-via-test/07c-cbg-retry-stamp.json",
		"/app/configs/rules/dev-via-test/07d-cbg-retry-driver.json",
		"/app/configs/rules/dev-via-test/07e-cbg-retry-missing-target.json",
		"/app/configs/rules/dev-via-test/08-loop-failed-pause.json",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("flow-bootstrap.json rules_files missing %q — rule never loads at boot", want)
		}
	}
}

// TestCoordinatorDispatchHasQueryEntityTool asserts the
// teams-dispatch default_tools list includes query_entity. The
// Slice 3 walker reads the run entity's plan + execution-state
// triples via query_entity in EVERY wake-up — without it in the
// coordinator's default toolset, the walker can't read state and
// the chain wedges silently.
//
// Per Slice 3 design: query_entity is added to default_tools (not
// per-wake-up-rule tools) so the front-door coordinator also has
// it. Wake-up rules 02 + 05 explicitly include it in their spawn
// tools as defense-in-depth.
func TestCoordinatorDispatchHasQueryEntityTool(t *testing.T) {
	data, err := os.ReadFile("../../configs/flow-bootstrap.json")
	if err != nil {
		t.Fatalf("read flow-bootstrap.json: %v", err)
	}
	body := string(data)
	// Locate teams-dispatch.config.default_tools and assert
	// query_entity is in the list. Substring match is sufficient —
	// the dispatch block's default_tools is the only line with
	// that exact key.
	if !strings.Contains(body, `"query_entity"`) {
		t.Error("flow-bootstrap.json teams-dispatch.default_tools does not include \"query_entity\" — Slice 3 walker cannot read run-entity state and chain wedges silently")
	}
}

// TestDevViaTestPack_01_SubtopicsLengthZero pins the Slice 3
// addition to rule 01: the spawn rule now requires
// coordinator.decision.subtopics.length=0 to differentiate from
// the walker-dispatch path (rule 03). Without this, walker
// dispatches (decide with subtopics non-empty) would double-fire
// rule 01 + rule 03, spawning both Lisa AND Ralph in parallel.
func TestDevViaTestPack_01_SubtopicsLengthZero(t *testing.T) {
	rule := loadDevViaTestRule(t, "01-coordinator-dev-via-test-spawn.json")
	if !devViaTestLengthCondition(rule, "coordinator.decision.subtopics", "length_eq", float64(0)) {
		t.Error("rule 01 missing coordinator.decision.subtopics length_eq 0 — walker dispatches will double-fire rule 01 + rule 03")
	}
}

// TestDevViaTestPack_02_LisaTerminalToPlanReview pins the Slice 6
// redirect: Lisa-terminal(planned) now spawns CBG in plan_review
// mode (a fidelity gate BEFORE the walker), not the walker directly.
//
//  1. Conditions: role=dev-via-test-plan, outcome=success,
//     next_action=planned.
//  2. Spawns reviewer-dev-via-test (CBG), NOT a coordinator.
//  3. Plan-review tools: query_entity (reads ask + plan in one
//     call) + read_loop_result + scratchpad + decide; NO bash (no
//     tests run at the plan gate).
//  4. action_allowlist = the distinct plan-gate tokens
//     [plan_approved, plan_rejected_retry, plan_rejected] — disjoint
//     from the chain-end gate so routing never collides.
//  5. related_loops carries plan-gate + run-loop-entity-id from
//     Lisa's lineage.
func TestDevViaTestPack_02_LisaTerminalToPlanReview(t *testing.T) {
	rule := loadDevViaTestRule(t, "02-lisa-terminal-to-plan-review.json")

	if !devViaTestRoleCondition(rule, "dev-via-test-plan") {
		t.Error("rule 02 does not condition on role=dev-via-test-plan")
	}
	if !devViaTestOutcomeCondition(rule, "success") {
		t.Error("rule 02 does not condition on outcome=success")
	}
	if !devViaTestNextActionCondition(rule, "planned") {
		t.Error("rule 02 does not condition on next_action=planned")
	}

	var spawn *devViaTestOnEnterJSON
	for i := range rule.OnEnter {
		if rule.OnEnter[i].Type == "publish_agent" && rule.OnEnter[i].Role == "reviewer-dev-via-test" {
			spawn = &rule.OnEnter[i]
			break
		}
	}
	if spawn == nil {
		t.Fatal("rule 02 has no publish_agent for role=reviewer-dev-via-test (CBG plan-review)")
	}

	for _, want := range []string{"read_loop_result", "query_entity", "scratchpad", "decide"} {
		if !devViaTestSliceHas(spawn.Tools, want) {
			t.Errorf("rule 02 plan-review tools missing %q; current = %v", want, spawn.Tools)
		}
	}
	if devViaTestSliceHas(spawn.Tools, "bash") {
		t.Error("rule 02 plan-review must NOT grant bash — no tests run at the plan gate (fidelity review only)")
	}

	for _, want := range []string{"plan_approved", "plan_rejected_retry", "plan_rejected"} {
		if !devViaTestSliceHas(spawn.ActionAllowed, want) {
			t.Errorf("rule 02 plan-review allowlist missing %q", want)
		}
	}
	if len(spawn.ActionAllowed) != 3 {
		t.Errorf("rule 02 allowlist = %v; want exactly the 3 plan-gate tokens (disjoint from chain-end)", spawn.ActionAllowed)
	}
	// Distinct tokens: the chain-end verdicts must NOT appear here, or
	// rule 07a/07b/07c would double-fire on a plan verdict.
	for _, forbidden := range []string{"approved", "rejected", "rejected_retry"} {
		if devViaTestSliceHas(spawn.ActionAllowed, forbidden) {
			t.Errorf("rule 02 allowlist includes chain-end token %q — would collide with the work gate", forbidden)
		}
	}

	if want, got := "$entity.triple.lineage.run-loop-entity-id", spawn.RelatedLoops["run-loop-entity-id"]; got != want {
		t.Errorf("rule 02 related_loops[run-loop-entity-id] = %q; want %q (threads the run entity from Lisa's lineage so CBG reads ask+plan there)", got, want)
	}
	if spawn.ToolChoice == nil {
		t.Error("rule 02 spawn missing tool_choice")
	}
}

// TestDevViaTestPack_02b_PlanApprovedToWalker pins that the walker is
// now gated behind CBG plan-approval, on the distinct plan_approved
// token (never colliding with the chain-end approved → rule 07a).
func TestDevViaTestPack_02b_PlanApprovedToWalker(t *testing.T) {
	rule := loadDevViaTestRule(t, "02b-plan-approved-to-walker.json")
	if !devViaTestRoleCondition(rule, "reviewer-dev-via-test") {
		t.Error("rule 02b does not condition on role=reviewer-dev-via-test")
	}
	if !devViaTestNextActionCondition(rule, "plan_approved") {
		t.Error("rule 02b does not condition on next_action=plan_approved")
	}
	var spawn *devViaTestOnEnterJSON
	for i := range rule.OnEnter {
		if rule.OnEnter[i].Type == "publish_agent" && rule.OnEnter[i].Role == "coordinator" {
			spawn = &rule.OnEnter[i]
			break
		}
	}
	if spawn == nil {
		t.Fatal("rule 02b has no coordinator walker spawn")
	}
	if !devViaTestSliceHas(spawn.ActionAllowed, "dev_via_test") {
		t.Error("rule 02b walker allowlist missing dev_via_test (cannot dispatch Ralph)")
	}
}

// TestDevViaTestPack_02c_PlanRetryStamp pins the plan-retry stamp
// (plan-phase analog of 07c): on plan_rejected_retry, upsert the
// finding + add the pending trigger on the RUN entity. No target_task
// (the whole plan is the unit).
func TestDevViaTestPack_02c_PlanRetryStamp(t *testing.T) {
	rule := loadDevViaTestRule(t, "02c-plan-retry-stamp.json")
	if !devViaTestRoleCondition(rule, "reviewer-dev-via-test") {
		t.Error("rule 02c does not condition on role=reviewer-dev-via-test")
	}
	if !devViaTestNextActionCondition(rule, "plan_rejected_retry") {
		t.Error("rule 02c does not condition on next_action=plan_rejected_retry")
	}
	const runSubject = "$entity.triple.lineage.run-loop-entity-id"
	var finding, pending *devViaTestOnEnterJSON
	for i := range rule.OnEnter {
		switch rule.OnEnter[i].Predicate {
		case "dev_via_test.plan.retry.finding":
			finding = &rule.OnEnter[i]
		case "dev_via_test.plan.retry.pending":
			pending = &rule.OnEnter[i]
		}
	}
	if finding == nil || finding.Type != "update_triple" || finding.Subject != runSubject {
		t.Errorf("rule 02c finding stamp wrong: %+v (want update_triple on run entity)", finding)
	}
	if obj, _ := finding.Object.(string); obj != "$entity.triple.coordinator.decision.reason" {
		t.Errorf("rule 02c finding object = %v; want coordinator.decision.reason (CBG's fix-spec)", finding.Object)
	}
	if pending == nil || pending.Type != "add_triple" || pending.Subject != runSubject {
		t.Errorf("rule 02c pending stamp wrong: %+v (want add_triple on run entity)", pending)
	}
}

// TestDevViaTestPack_02d_PlanRetryDriver pins the plan-retry driver
// (plan-phase analog of 07d): run-entity anchored, $state.iteration
// vs plan.lisa_retry_budget gates re-dispatch of LISA vs escalate.
func TestDevViaTestPack_02d_PlanRetryDriver(t *testing.T) {
	rule := loadDevViaTestRule(t, "02d-plan-retry-driver.json")
	var hasPending bool
	for _, c := range rule.Conditions {
		if c.Field == "dev_via_test.plan.retry.pending" && c.Operator == "ne" {
			hasPending = true
		}
	}
	if !hasPending {
		t.Error("rule 02d does not condition on dev_via_test.plan.retry.pending (ne) — driver never fires")
	}
	var removed bool
	var replan, escalate *devViaTestOnEnterJSON
	for i := range rule.OnEnter {
		a := &rule.OnEnter[i]
		if a.Type == "remove_triple" && a.Predicate == "dev_via_test.plan.retry.pending" {
			removed = true
		}
		if a.Type == "publish_agent" && a.Role == "dev-via-test-plan" {
			replan = a
		}
		if a.Type == "publish_agent" && a.Role == "coordinator" {
			escalate = a
		}
	}
	if !removed {
		t.Error("rule 02d does not remove the pending marker (presence-marker re-entry broken)")
	}
	if replan == nil {
		t.Fatal("rule 02d has no Lisa re-dispatch (role=dev-via-test-plan)")
	}
	if !devViaTestWhenHas(replan, "$state.iteration", "lte", "$entity.triple.plan.lisa_retry_budget") {
		t.Error("rule 02d re-plan When must gate $state.iteration <= plan.lisa_retry_budget")
	}
	if !devViaTestSliceHas(replan.Tools, "emit_dev_via_test_plan") {
		t.Error("rule 02d re-planned Lisa missing emit_dev_via_test_plan tool")
	}
	// max_iterations:0 (unlimited) is load-bearing — the default
	// per-action cap of 3 would silently cut re-dispatch off before the
	// budget (go-reviewer N3 / Slice 5 07d rationale).
	if replan.MaxIterations == nil || *replan.MaxIterations != 0 {
		t.Error("rule 02d re-plan action must set max_iterations:0; the When+clamp is the bound, the default cap 3 would cut off before plan.lisa_retry_budget")
	}
	if escalate == nil {
		t.Fatal("rule 02d has no escalate (coordinator) spawn")
	}
	if !devViaTestWhenHas(escalate, "$state.iteration", "gt", "$entity.triple.plan.lisa_retry_budget") {
		t.Error("rule 02d escalate When must gate $state.iteration > plan.lisa_retry_budget")
	}
}

// TestDevViaTestPack_02e_PlanRejectedToCoordinator pins the plan-gate
// escalate (plan-phase analog of 07b): plan_rejected → ask_user.
func TestDevViaTestPack_02e_PlanRejectedToCoordinator(t *testing.T) {
	rule := loadDevViaTestRule(t, "02e-plan-rejected-to-coordinator.json")
	if !devViaTestRoleCondition(rule, "reviewer-dev-via-test") {
		t.Error("rule 02e does not condition on role=reviewer-dev-via-test")
	}
	if !devViaTestNextActionCondition(rule, "plan_rejected") {
		t.Error("rule 02e does not condition on next_action=plan_rejected")
	}
	var spawn *devViaTestOnEnterJSON
	for i := range rule.OnEnter {
		if rule.OnEnter[i].Type == "publish_agent" && rule.OnEnter[i].Role == "coordinator" {
			spawn = &rule.OnEnter[i]
			break
		}
	}
	if spawn == nil {
		t.Fatal("rule 02e has no coordinator spawn")
	}
	if !devViaTestSliceHas(spawn.ActionAllowed, "ask_user") {
		t.Error("rule 02e allowlist missing ask_user")
	}
}

// TestDevViaTestPack_03_DispatchRalphForEach pins the Slice 3
// walker-dispatch rule's shape. The pack's whole sequential
// walking pattern hangs off this rule's for_each over subtopics.
//
// Invariants:
//
//  1. Conditions match coordinator + decide(dev_via_test) +
//     subtopics.length > 0 (the walker-dispatch path, mutually
//     exclusive with rule 01).
//  2. Spawns dev-via-test-execute (Ralph).
//  3. for_each iterates over $entity.triple.coordinator.decision.subtopics
//     with for_each_var = subtopic (per ADR-046 Phase 1).
//  4. properties.task_id substitutes $subtopic — Ralph reads
//     this to know which task he owns.
//  5. related_loops pins run-loop-entity-id from walker's lineage
//     (NOT $entity.id — the walker isn't the run entity).
//  6. action_allowlist constrains to [measured, needs_clarification].
//  7. Ralph's tools include bash + emit_dev_via_test_measurement.
func TestDevViaTestPack_03_DispatchRalphForEach(t *testing.T) {
	rule := loadDevViaTestRule(t, "03-coordinator-dispatch-ralph.json")

	if !devViaTestRoleCondition(rule, "coordinator") {
		t.Error("rule 03 does not condition on role=coordinator")
	}
	if !devViaTestActionCondition(rule, "dev_via_test") {
		t.Error("rule 03 does not condition on next_action=dev_via_test")
	}
	if !devViaTestLengthCondition(rule, "coordinator.decision.subtopics", "length_gt", float64(0)) {
		t.Error("rule 03 missing coordinator.decision.subtopics length_gt 0 — rule 01 (Lisa spawn) would double-fire with rule 03 on every walker dispatch")
	}

	var spawn *devViaTestOnEnterJSON
	for i := range rule.OnEnter {
		if rule.OnEnter[i].Type == "publish_agent" && rule.OnEnter[i].Role == "dev-via-test-execute" {
			spawn = &rule.OnEnter[i]
			break
		}
	}
	if spawn == nil {
		t.Fatal("rule 03 has no publish_agent for role=dev-via-test-execute")
	}

	if spawn.ForEach != "$entity.triple.coordinator.decision.subtopics" {
		t.Errorf("rule 03 for_each = %q; want %q (subtopics-driven fan-out)",
			spawn.ForEach, "$entity.triple.coordinator.decision.subtopics")
	}
	if spawn.ForEachVar != "subtopic" {
		t.Errorf("rule 03 for_each_var = %q; want %q (matches $subtopic substitution in prompt + properties)",
			spawn.ForEachVar, "subtopic")
	}

	// Ralph's tools must include bash + emit_dev_via_test_measurement.
	for _, want := range []string{"bash", "emit_dev_via_test_measurement", "decide", "scratchpad"} {
		if !devViaTestSliceHas(spawn.Tools, want) {
			t.Errorf("rule 03 Ralph tools missing %q", want)
		}
	}

	if want, got := "$entity.triple.lineage.run-loop-entity-id", spawn.RelatedLoops["run-loop-entity-id"]; got != want {
		t.Errorf("rule 03 related_loops[run-loop-entity-id] = %q; want %q (walker is NOT the run entity; thread from lineage)", got, want)
	}

	for _, want := range []string{"measured", "needs_clarification"} {
		if !devViaTestSliceHas(spawn.ActionAllowed, want) {
			t.Errorf("rule 03 action_allowlist missing %q", want)
		}
	}
}

// TestDevViaTestPack_05_RalphTerminalToWalker pins the Slice 3
// post-Ralph wake-up rule's shape. Mirror of rule 02 but for the
// Ralph-terminal path. Critical: outcome condition must cover all
// four terminal classes (success, failed, truncated, cancelled)
// so the walker wakes on ANY Ralph termination — failure modes
// route through walker's ask_user per ADR-044 §Stuck-task recovery.
func TestDevViaTestPack_05_RalphTerminalToWalker(t *testing.T) {
	rule := loadDevViaTestRule(t, "05-ralph-terminal-to-walker.json")

	if !devViaTestRoleCondition(rule, "dev-via-test-execute") {
		t.Error("rule 05 does not condition on role=dev-via-test-execute")
	}
	for _, outcome := range []string{"success", "failed", "truncated", "cancelled"} {
		if !devViaTestOutcomeCondition(rule, outcome) {
			t.Errorf("rule 05 does not match outcome=%q — walker silently misses %s terminations", outcome, outcome)
		}
	}

	var spawn *devViaTestOnEnterJSON
	for i := range rule.OnEnter {
		if rule.OnEnter[i].Type == "publish_agent" && rule.OnEnter[i].Role == "coordinator" {
			spawn = &rule.OnEnter[i]
			break
		}
	}
	if spawn == nil {
		t.Fatal("rule 05 has no publish_agent for role=coordinator")
	}

	for _, want := range []string{"read_loop_result", "query_entity", "scratchpad", "decide"} {
		if !devViaTestSliceHas(spawn.Tools, want) {
			t.Errorf("rule 05 walker tools missing %q", want)
		}
	}
	for _, want := range []string{"dev_via_test", "respond_direct", "ask_user"} {
		if !devViaTestSliceHas(spawn.ActionAllowed, want) {
			t.Errorf("rule 05 action_allowlist missing %q", want)
		}
	}
	if want, got := "$entity.triple.lineage.run-loop-entity-id", spawn.RelatedLoops["run-loop-entity-id"]; got != want {
		t.Errorf("rule 05 related_loops[run-loop-entity-id] = %q; want %q", got, want)
	}

	// Per Slice 3 reviewer R4: rule 02 pins tool_choice=required;
	// rule 05 must too — same load-bearing reason (walker must use
	// the tool path or text-only completion class wedges per
	// [[failed-loops-wedge-chain]] / semstreams#158).
	if spawn.ToolChoice == nil {
		t.Error("rule 05 spawn missing tool_choice — walker may text-out of query_entity / decide")
	} else if mode, _ := spawn.ToolChoice["mode"].(string); mode != "required" {
		t.Errorf("rule 05 tool_choice.mode = %q; want %q", mode, "required")
	}
}

// TestDevViaTestPack_06_DispatchCBG pins the Slice 4 walker-
// finalize dispatch rule. The chain-end gate hangs off this rule
// firing on the new dev_via_test_finalize action token.
//
// Invariants:
//
//  1. Conditions match coordinator + decide(dev_via_test_finalize).
//     NO subtopics-length condition — finalize never carries
//     subtopics (CBG dispatch is not parametrized).
//  2. Spawns reviewer-dev-via-test (CBG).
//  3. CBG's tools: query_entity (read run-entity state), bash (run
//     integration test + git diff), read_loop_result (read walker's
//     pre-CBG rollup), decide, scratchpad.
//  4. action_allowlist: [approved, rejected_retry, rejected] — the
//     ADR-044 §Slice 5 three-way verdict. CBG cannot start a fresh
//     chain or escalate via needs_clarification; the only escape
//     hatches are the two structured reject tokens (rejected_retry
//     = dev-fixable bounded bounce, rejected = escalate to user).
//  5. related_loops pins run-loop-entity-id from walker's lineage
//     (CBG is NOT the run entity; thread from walker).
//  6. tool_choice=required (CBG must use the tool path).
func TestDevViaTestPack_06_DispatchCBG(t *testing.T) {
	rule := loadDevViaTestRule(t, "06-coordinator-dispatch-cbg.json")

	if !devViaTestRoleCondition(rule, "coordinator") {
		t.Error("rule 06 does not condition on role=coordinator")
	}
	if !devViaTestActionCondition(rule, "dev_via_test_finalize") {
		t.Error("rule 06 does not condition on next_action=dev_via_test_finalize — CBG dispatch token missing")
	}

	var spawn *devViaTestOnEnterJSON
	for i := range rule.OnEnter {
		if rule.OnEnter[i].Type == "publish_agent" && rule.OnEnter[i].Role == "reviewer-dev-via-test" {
			spawn = &rule.OnEnter[i]
			break
		}
	}
	if spawn == nil {
		t.Fatal("rule 06 has no publish_agent for role=reviewer-dev-via-test (CBG)")
	}

	for _, want := range []string{"query_entity", "bash", "read_loop_result", "decide", "scratchpad"} {
		if !devViaTestSliceHas(spawn.Tools, want) {
			t.Errorf("rule 06 CBG tools missing %q (need to read run entity + run integration test + read walker terminal)", want)
		}
	}
	for _, want := range []string{"approved", "rejected_retry", "rejected"} {
		if !devViaTestSliceHas(spawn.ActionAllowed, want) {
			t.Errorf("rule 06 action_allowlist missing %q", want)
		}
	}
	if len(spawn.ActionAllowed) != 3 {
		t.Errorf("rule 06 action_allowlist = %v; want exactly [approved, rejected_retry, rejected] — the Slice 5 three-way verdict; CBG cannot start a fresh chain or escalate via needs_clarification by design", spawn.ActionAllowed)
	}
	if want, got := "$entity.triple.lineage.run-loop-entity-id", spawn.RelatedLoops["run-loop-entity-id"]; got != want {
		t.Errorf("rule 06 related_loops[run-loop-entity-id] = %q; want %q (CBG threads from walker's lineage)", got, want)
	}
	if spawn.ToolChoice == nil {
		t.Error("rule 06 spawn missing tool_choice — CBG may text-out of bash / decide")
	} else if mode, _ := spawn.ToolChoice["mode"].(string); mode != "required" {
		t.Errorf("rule 06 tool_choice.mode = %q; want %q", mode, "required")
	}
}

// TestDevViaTestPack_07a_CBGApprovedToCoordinator pins the chain-
// terminal approved-path wake-up. Mirrors autoresearch rule 08
// shape — coordinator wakes scoped to respond_direct (deliver
// CBG's rollup to user).
func TestDevViaTestPack_07a_CBGApprovedToCoordinator(t *testing.T) {
	rule := loadDevViaTestRule(t, "07a-cbg-approved-to-coordinator.json")

	if !devViaTestRoleCondition(rule, "reviewer-dev-via-test") {
		t.Error("rule 07a does not condition on role=reviewer-dev-via-test")
	}
	if !devViaTestActionCondition(rule, "approved") {
		t.Error("rule 07a does not condition on next_action=approved")
	}

	var spawn *devViaTestOnEnterJSON
	for i := range rule.OnEnter {
		if rule.OnEnter[i].Type == "publish_agent" && rule.OnEnter[i].Role == "coordinator" {
			spawn = &rule.OnEnter[i]
			break
		}
	}
	if spawn == nil {
		t.Fatal("rule 07a has no publish_agent for role=coordinator")
	}
	if !devViaTestSliceHas(spawn.ActionAllowed, "respond_direct") {
		t.Error("rule 07a action_allowlist missing respond_direct — final wake-up must deliver to user")
	}
	if !devViaTestSliceHas(spawn.Tools, "read_loop_result") {
		t.Error("rule 07a coordinator tools missing read_loop_result — needed to read CBG's verdict")
	}
}

// TestDevViaTestPack_07a_WakeupModeProperty pins the per-Slice-4
// reviewer R2 telemetry differentiator. The ADR justifies the
// 07a/07b split partly via "operator-facing telemetry shouldn't
// have to grep through verdict strings." This test prevents a
// future refactor that "simplifies" both rules to a shared
// template from silently dropping the differentiator.
func TestDevViaTestPack_07a_WakeupModeProperty(t *testing.T) {
	rule := loadDevViaTestRule(t, "07a-cbg-approved-to-coordinator.json")
	for _, a := range rule.OnEnter {
		if a.Type != "publish_agent" {
			continue
		}
		if got := a.Properties["wakeup_mode"]; got != "chain_terminal_dev_via_test_approved" {
			t.Errorf("rule 07a properties.wakeup_mode = %q; want %q (operator telemetry differentiator per ADR-044 §addendum Slice 4)",
				got, "chain_terminal_dev_via_test_approved")
		}
		return
	}
	t.Fatal("rule 07a has no publish_agent action")
}

func TestDevViaTestPack_07b_WakeupModeProperty(t *testing.T) {
	rule := loadDevViaTestRule(t, "07b-cbg-rejected-to-coordinator.json")
	for _, a := range rule.OnEnter {
		if a.Type != "publish_agent" {
			continue
		}
		if got := a.Properties["wakeup_mode"]; got != "chain_terminal_dev_via_test_rejected" {
			t.Errorf("rule 07b properties.wakeup_mode = %q; want %q (operator telemetry differentiator per ADR-044 §addendum Slice 4)",
				got, "chain_terminal_dev_via_test_rejected")
		}
		return
	}
	t.Fatal("rule 07b has no publish_agent action")
}

// TestDevViaTestPack_07c_CBGRetryStamp pins the ADR-044 §Slice 5
// stamp half: when CBG decides rejected_retry, the rule stamps the
// retry markers on the RUN entity (via lineage subject override),
// NOT on CBG's own loop entity. target_task + finding use
// update_triple (upsert — latest verdict wins); pending uses
// add_triple (presence trigger that 07d removes each cycle).
func TestDevViaTestPack_07c_CBGRetryStamp(t *testing.T) {
	rule := loadDevViaTestRule(t, "07c-cbg-retry-stamp.json")

	if !devViaTestRoleCondition(rule, "reviewer-dev-via-test") {
		t.Error("rule 07c does not condition on role=reviewer-dev-via-test")
	}
	if !devViaTestActionCondition(rule, "rejected_retry") {
		t.Error("rule 07c does not condition on next_action=rejected_retry")
	}
	// go-reviewer C1: 07c MUST be fenced on subtopics presence so it
	// can never stamp a literal-garbage target. The complementary
	// case (rejected_retry + no subtopics) is rule 07e's job.
	if !devViaTestLengthCondition(rule, "coordinator.decision.subtopics", "length_gt", float64(0)) {
		t.Error("rule 07c does not fence on coordinator.decision.subtopics length_gt 0 — a subtopics-less rejected_retry would stamp a garbage target_task (go-reviewer C1)")
	}

	const runSubject = "$entity.triple.lineage.run-loop-entity-id"
	// go-reviewer R3: pin the object SOURCES, not just the subject +
	// type — these are the substitution tokens that thread CBG's
	// verdict onto the run entity; a typo silently breaks the retry.
	wantObject := map[string]string{
		"dev_via_test.cbg.retry.target_task": "$entity.triple.coordinator.decision.subtopics",
		"dev_via_test.cbg.retry.finding":     "$entity.triple.coordinator.decision.reason",
	}
	seen := map[string]devViaTestOnEnterJSON{}
	for _, a := range rule.OnEnter {
		switch a.Predicate {
		case "dev_via_test.cbg.retry.target_task", "dev_via_test.cbg.retry.finding", "dev_via_test.cbg.retry.pending":
			seen[a.Predicate] = a
		}
	}

	for _, pred := range []string{"dev_via_test.cbg.retry.target_task", "dev_via_test.cbg.retry.finding"} {
		a, ok := seen[pred]
		if !ok {
			t.Errorf("rule 07c does not stamp %q", pred)
			continue
		}
		if a.Type != "update_triple" {
			t.Errorf("rule 07c %q type = %q; want update_triple (upsert so the latest CBG verdict overwrites the prior)", pred, a.Type)
		}
		if a.Subject != runSubject {
			t.Errorf("rule 07c %q subject = %q; want %q (markers live on the RUN entity, not CBG's loop)", pred, a.Subject, runSubject)
		}
		if obj, _ := a.Object.(string); obj != wantObject[pred] {
			t.Errorf("rule 07c %q object = %v; want %q (threads CBG's verdict from coordinator.decision.*)", pred, a.Object, wantObject[pred])
		}
	}

	pending, ok := seen["dev_via_test.cbg.retry.pending"]
	if !ok {
		t.Fatal("rule 07c does not stamp dev_via_test.cbg.retry.pending — 07d's trigger never fires")
	}
	if pending.Type != "add_triple" {
		t.Errorf("rule 07c pending type = %q; want add_triple (presence trigger removed by 07d each cycle, per semstreams#204)", pending.Type)
	}
	if pending.Subject != runSubject {
		t.Errorf("rule 07c pending subject = %q; want %q", pending.Subject, runSubject)
	}
}

// TestDevViaTestPack_07d_CBGRetryDriver pins the ADR-044 §Slice 5
// driver half: anchored to the RUN entity (so $state.iteration is a
// stable per-run retry counter), it clears the pending marker and
// routes — re-dispatch under budget, escalate over budget. The
// When-clauses gate on $state.iteration vs plan.cbg_retry_budget.
func TestDevViaTestPack_07d_CBGRetryDriver(t *testing.T) {
	rule := loadDevViaTestRule(t, "07d-cbg-retry-driver.json")

	// Trigger: presence of the pending marker on the run entity.
	var hasPendingCond bool
	for _, c := range rule.Conditions {
		if c.Field == "dev_via_test.cbg.retry.pending" && c.Operator == "ne" {
			hasPendingCond = true
		}
	}
	if !hasPendingCond {
		t.Error("rule 07d does not condition on dev_via_test.cbg.retry.pending (ne) — driver never fires")
	}

	var removedPending bool
	var redispatch, escalate *devViaTestOnEnterJSON
	for i := range rule.OnEnter {
		a := &rule.OnEnter[i]
		if a.Type == "remove_triple" && a.Predicate == "dev_via_test.cbg.retry.pending" {
			removedPending = true
		}
		if a.Type == "publish_agent" && a.Role == "coordinator" {
			if devViaTestSliceHas(a.ActionAllowed, "dev_via_test") {
				redispatch = a
			} else {
				escalate = a
			}
		}
	}

	if !removedPending {
		t.Error("rule 07d does not remove_triple the pending marker — without the clear, the rule can't re-Enter on the next retry (semstreams#204 presence-marker discipline)")
	}

	if redispatch == nil {
		t.Fatal("rule 07d has no re-dispatch coordinator spawn (allowlist with dev_via_test)")
	}
	if !devViaTestSliceHas(redispatch.ActionAllowed, "ask_user") {
		t.Error("rule 07d re-dispatch allowlist missing ask_user — coordinator must be able to override CBG's dev-fixable judgment")
	}
	if !devViaTestWhenHas(redispatch, "$state.iteration", "lte", "$entity.triple.plan.cbg_retry_budget") {
		t.Error("rule 07d re-dispatch When clause must gate $state.iteration <= plan.cbg_retry_budget (the bounded-retry budget)")
	}

	if escalate == nil {
		t.Fatal("rule 07d has no escalate coordinator spawn (ask_user-only)")
	}
	if !devViaTestSliceHas(escalate.ActionAllowed, "ask_user") {
		t.Error("rule 07d escalate allowlist missing ask_user")
	}
	if devViaTestSliceHas(escalate.ActionAllowed, "dev_via_test") {
		t.Error("rule 07d escalate branch must NOT allow dev_via_test — budget is exhausted, no more auto re-dispatch")
	}
	if !devViaTestWhenHas(escalate, "$state.iteration", "gt", "$entity.triple.plan.cbg_retry_budget") {
		t.Error("rule 07d escalate When clause must gate $state.iteration > plan.cbg_retry_budget")
	}
}

// TestDevViaTestPack_07e_CBGRetryMissingTarget pins the go-reviewer
// C1 fence: a rejected_retry WITHOUT subtopics (07c can't fire,
// nothing would wake the chain) must escalate to ask_user instead
// of wedging or stamping a garbage target. 07c (subtopics>0) and
// 07e (subtopics=0) are a mutually-exclusive split — every
// rejected_retry routes exactly one way.
func TestDevViaTestPack_07e_CBGRetryMissingTarget(t *testing.T) {
	rule := loadDevViaTestRule(t, "07e-cbg-retry-missing-target.json")

	if !devViaTestRoleCondition(rule, "reviewer-dev-via-test") {
		t.Error("rule 07e does not condition on role=reviewer-dev-via-test")
	}
	if !devViaTestActionCondition(rule, "rejected_retry") {
		t.Error("rule 07e does not condition on next_action=rejected_retry")
	}
	if !devViaTestLengthCondition(rule, "coordinator.decision.subtopics", "length_eq", float64(0)) {
		t.Error("rule 07e does not fence on subtopics length_eq 0 — it must catch ONLY the subtopics-less rejected_retry (the complement of 07c's length_gt 0)")
	}

	var spawn *devViaTestOnEnterJSON
	for i := range rule.OnEnter {
		if rule.OnEnter[i].Type == "publish_agent" && rule.OnEnter[i].Role == "coordinator" {
			spawn = &rule.OnEnter[i]
			break
		}
	}
	if spawn == nil {
		t.Fatal("rule 07e has no publish_agent for role=coordinator — a subtopics-less rejected_retry would wedge (07c won't fire)")
	}
	if !devViaTestSliceHas(spawn.ActionAllowed, "ask_user") {
		t.Error("rule 07e allowlist missing ask_user — the fail-safe escalation for an unpinnable retry")
	}
	if devViaTestSliceHas(spawn.ActionAllowed, "dev_via_test") {
		t.Error("rule 07e must NOT allow dev_via_test — there is no target to re-dispatch")
	}
}

// TestDevViaTestFinalizeTokenLiteralConsistency pins the
// `dev_via_test_finalize` action token across all surfaces that
// must agree on the literal: rule 06's condition, rules 02/05's
// walker action_allowlist, coordinator persona decision-contract
// + plan-walking fragment, walker prompt step instructions. Per
// Slice 4 reviewer R8: if a future refactor renames the token,
// every dependent surface fails simultaneously at PR time, not
// five smoke-time failures.
//
// Mirrors TestDevViaTestChainStartTagLiteralConsistency for the
// `plan-start` literal (Slice 1 reviewer N4 pattern).
func TestDevViaTestFinalizeTokenLiteralConsistency(t *testing.T) {
	const token = "dev_via_test_finalize"

	// Source 1: rule 06 condition.
	rule06 := loadDevViaTestRule(t, "06-coordinator-dispatch-cbg.json")
	if !devViaTestActionCondition(rule06, token) {
		t.Errorf("rule 06 does not condition on next_action=%q — CBG dispatch dead", token)
	}

	// Source 2 + 3: the walker rules' action_allowlist must include
	// token (walker emits the action; allowlist gates). Since Slice 6,
	// the post-Lisa walker is spawned by 02b (plan_approved), not 02
	// (which now spawns the CBG plan-review gate).
	for _, name := range []string{"02b-plan-approved-to-walker.json", "05-ralph-terminal-to-walker.json"} {
		rule := loadDevViaTestRule(t, name)
		for _, a := range rule.OnEnter {
			if a.Type != "publish_agent" {
				continue
			}
			if !devViaTestSliceHas(a.ActionAllowed, token) {
				t.Errorf("%s walker action_allowlist missing %q — walker cannot reach CBG finalize path", name, token)
			}
			break
		}
	}

	// Source 4: coordinator persona decision-contract.md.
	decisionContract, err := os.ReadFile("../../configs/personas/fragments/coordinator/10-decision-contract.md")
	if err != nil {
		t.Fatalf("read 10-decision-contract.md: %v", err)
	}
	if !strings.Contains(string(decisionContract), "`"+token+"`") {
		t.Errorf("coordinator 10-decision-contract.md missing %q in closed-taxonomy table", token)
	}

	// Source 5: coordinator persona plan-walking.md (walker contract).
	planWalking, err := os.ReadFile("../../configs/personas/fragments/coordinator/30-plan-walking.md")
	if err != nil {
		t.Fatalf("read 30-plan-walking.md: %v", err)
	}
	if !strings.Contains(string(planWalking), token) {
		t.Errorf("coordinator 30-plan-walking.md does not reference %q — walker doesn't know to emit it when all tasks done", token)
	}

	// Source 6: rule 05's walker prompt (post-Ralph wake-up tells
	// walker explicitly to use this token on "all done").
	rule05Data, err := os.ReadFile("../../configs/rules/dev-via-test/05-ralph-terminal-to-walker.json")
	if err != nil {
		t.Fatalf("read rule 05: %v", err)
	}
	if !strings.Contains(string(rule05Data), token) {
		t.Errorf("rule 05's prompt does not mention %q — walker wake-up doesn't instruct the CBG dispatch path", token)
	}
}

// TestDevViaTestPack_07b_CBGRejectedToCoordinator pins the chain-
// terminal rejected-path wake-up. Routes through ask_user (no
// auto-recover per ADR §CBG's gate).
func TestDevViaTestPack_07b_CBGRejectedToCoordinator(t *testing.T) {
	rule := loadDevViaTestRule(t, "07b-cbg-rejected-to-coordinator.json")

	if !devViaTestRoleCondition(rule, "reviewer-dev-via-test") {
		t.Error("rule 07b does not condition on role=reviewer-dev-via-test")
	}
	if !devViaTestActionCondition(rule, "rejected") {
		t.Error("rule 07b does not condition on next_action=rejected")
	}

	var spawn *devViaTestOnEnterJSON
	for i := range rule.OnEnter {
		if rule.OnEnter[i].Type == "publish_agent" && rule.OnEnter[i].Role == "coordinator" {
			spawn = &rule.OnEnter[i]
			break
		}
	}
	if spawn == nil {
		t.Fatal("rule 07b has no publish_agent for role=coordinator")
	}
	if !devViaTestSliceHas(spawn.ActionAllowed, "ask_user") {
		t.Error("rule 07b action_allowlist missing ask_user — rejected path must surface to user (no auto-recover)")
	}
}

// TestReviewerDevViaTestPersonaTeachesGate asserts CBG's persona
// corpus carries the load-bearing concepts: the integration_test_
// command gate, the diff sanity-check, the approved/rejected
// terminal, and the explicit no-recovery posture.
func TestReviewerDevViaTestPersonaTeachesGate(t *testing.T) {
	root := "../../configs/personas/fragments/reviewer-dev-via-test"
	got, err := concatFragmentsDevViaTest(root)
	if err != nil {
		t.Fatalf("read reviewer-dev-via-test fragments: %v", err)
	}
	for _, want := range []string{
		"integration_test_command", // the gate
		"chain_start_git_tag",      // the diff anchor
		"git diff",                 // the diff sanity-check
		"approved",                 // terminal action
		"rejected",                 // terminal action
		"do not iterate",           // no-recovery posture (or similar phrasing)
	} {
		// Case-insensitive contains — persona authors phrase naturally
		if !strings.Contains(strings.ToLower(got), strings.ToLower(want)) {
			t.Errorf("reviewer-dev-via-test persona missing %q — gate contract incomplete", want)
		}
	}
}

// TestCoordinatorPersonaTeachesFinalize asserts the coordinator
// decision-contract documents the dev_via_test_finalize action
// (per Slice 4 — new closed-taxonomy entry).
func TestCoordinatorPersonaTeachesFinalize(t *testing.T) {
	path := "../../configs/personas/fragments/coordinator/10-decision-contract.md"
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	text := string(data)
	if !strings.Contains(text, "`dev_via_test_finalize`") {
		t.Error("coordinator 10-decision-contract.md does not list `dev_via_test_finalize` as a valid action — walker cannot reach CBG dispatch path")
	}
	if !strings.Contains(text, "CBG") || !strings.Contains(text, "integration_test_command") {
		t.Error("coordinator 10-decision-contract.md dev_via_test_finalize entry should mention CBG + the integration_test_command — wire-shape context")
	}
}

// TestDevViaTestPack_01_LineageLengthZero pins the Slice 3
// reviewer B1 fence: rule 01 requires lineage.run-loop-entity-id
// to be absent (length_eq 0). Front-door coordinators have no
// lineage; walkers always do (pinned by rules 02 + 05). Without
// this condition, a walker emitting decide(action=dev_via_test)
// without subtopics (LLM degeneration, token truncation mid-arg)
// would re-fire rule 01 and spawn a second Lisa — corrupting the
// run-entity plan with duplicate stamps. With this fence, the
// spurious dispatch silently no-ops (walker decision goes
// nowhere; persona "what NOT to do" warns against this directly).
//
// The "silent no-op > corruption" trade-off is the explicit
// disposition per CLAUDE.md [[structural-over-llm-judgment]].
func TestDevViaTestPack_01_LineageLengthZero(t *testing.T) {
	rule := loadDevViaTestRule(t, "01-coordinator-dev-via-test-spawn.json")
	if !devViaTestLengthCondition(rule, "lineage.run-loop-entity-id", "length_eq", float64(0)) {
		t.Error("rule 01 missing lineage.run-loop-entity-id length_eq 0 — walker re-dispatch of dev_via_test without subtopics can spawn a duplicate Lisa and corrupt run-entity plan triples")
	}
}

// TestCoordinatorPersonaTeachesPlanWalking asserts the coordinator
// persona corpus carries the walker contract — the dual-mode
// dev_via_test token, the query_entity-based state read pattern,
// and the no-retry posture from ADR-044 §Stuck-task recovery.
func TestCoordinatorPersonaTeachesPlanWalking(t *testing.T) {
	root := "../../configs/personas/fragments/coordinator"
	got, err := concatFragmentsDevViaTest(root)
	if err != nil {
		t.Fatalf("read coordinator fragments: %v", err)
	}
	for _, want := range []string{
		"walker",          // role name
		"query_entity",    // load-bearing tool
		"plan.task.",      // plan triple predicate prefix
		"task_completed",  // execution marker
		"task_failed",     // execution marker
		"subtopics",       // walker dispatch arg
		"30-plan-walking", // fragment exists as a referenced unit
	} {
		if !strings.Contains(got, want) {
			t.Errorf("coordinator persona missing %q — walker contract incomplete", want)
		}
	}
}

// --- helpers added in Slice 3 ---

func devViaTestNextActionCondition(r *devViaTestRuleJSON, action string) bool {
	for _, c := range r.Conditions {
		if c.Field == "coordinator.decision.next_action" && c.Operator == "eq" {
			if s, ok := c.Value.(string); ok && s == action {
				return true
			}
		}
	}
	return false
}

func devViaTestLengthCondition(r *devViaTestRuleJSON, field, operator string, want any) bool {
	for _, c := range r.Conditions {
		if c.Field == field && c.Operator == operator {
			// JSON-decoded numbers come back as float64; compare loosely.
			if cv, ok := c.Value.(float64); ok {
				if wv, ok := want.(float64); ok && cv == wv {
					return true
				}
			}
			if c.Value == want {
				return true
			}
		}
	}
	return false
}

// devViaTestWhenHas reports whether a publish_agent action carries a
// When clause matching field/operator/value (Slice 5 — the
// $state.iteration budget gate on rules 07d's branches).
func devViaTestWhenHas(a *devViaTestOnEnterJSON, field, operator, value string) bool {
	for _, c := range a.When {
		if c.Field == field && c.Operator == operator {
			if s, ok := c.Value.(string); ok && s == value {
				return true
			}
		}
	}
	return false
}

// TestDevViaTestPack_04a_ConvergedRule pins the Slice 2 Ralph-success
// terminal rule. The pack's plan-walker hangs off this rule's stamp
// of dev_via_test.execute.task_completed on the run entity — Slice 3
// reads it to know which Ralph just finished and pick the next task.
//
// Invariants:
//
//  1. Conditions match dev-via-test-execute role + outcome=success +
//     dev_via_test.measurement.pass=true. All three are load-bearing
//     (role differentiates from Lisa/CBG; outcome=success ensures
//     decide() ran; measurement.pass=true is the convergence signal).
//  2. Stamps dev_via_test.execute.outcome=converged on Ralph's own
//     entity (audit + slice 3 read convenience).
//  3. Stamps dev_via_test.execute.task_completed=<ralph-loop-id> on
//     the RUN entity via $entity.triple.lineage.run-loop-entity-id
//     subject substitution. THIS is the walker's pickup signal.
func TestDevViaTestPack_04a_ConvergedRule(t *testing.T) {
	rule := loadDevViaTestRule(t, "04a-execute-stamp-converged.json")

	if !devViaTestRoleCondition(rule, "dev-via-test-execute") {
		t.Error("rule 04a does not condition on agent.loop.role = dev-via-test-execute")
	}
	if !devViaTestOutcomeCondition(rule, "success") {
		t.Error("rule 04a does not condition on agent.loop.outcome = success")
	}
	if !devViaTestMeasurementPassCondition(rule, true) {
		t.Error("rule 04a does not condition on dev_via_test.measurement.pass = true — converged signal missing")
	}

	// Invariant 2 + 3: dual-target stamps.
	var foundOwnOutcome, foundRunCompleted bool
	for _, a := range rule.OnEnter {
		if a.Type != "add_triple" {
			continue
		}
		switch a.Predicate {
		case "dev_via_test.execute.outcome":
			if obj, ok := a.Object.(string); !ok || obj != "converged" {
				t.Errorf("rule 04a outcome stamp object = %v; want %q", a.Object, "converged")
			}
			if a.Subject != "" {
				t.Errorf("rule 04a outcome stamp subject = %q; want empty (defaults to Ralph's own entity)", a.Subject)
			}
			foundOwnOutcome = true
		case "dev_via_test.execute.task_completed":
			if a.Subject != "$entity.triple.lineage.run-loop-entity-id" {
				t.Errorf("rule 04a task_completed subject = %q; want %q — walker pickup needs run-entity stamp via lineage", a.Subject, "$entity.triple.lineage.run-loop-entity-id")
			}
			if obj, ok := a.Object.(string); !ok || obj != "$entity.instance" {
				t.Errorf("rule 04a task_completed object = %v; want %q (Ralph's loop ID for walker to read measurement triples)", a.Object, "$entity.instance")
			}
			foundRunCompleted = true
		}
	}
	if !foundOwnOutcome {
		t.Error("rule 04a missing add_triple dev_via_test.execute.outcome=converged on Ralph entity")
	}
	if !foundRunCompleted {
		t.Error("rule 04a missing add_triple dev_via_test.execute.task_completed on run entity — Slice 3 walker has no pickup signal")
	}
}

// TestDevViaTestPack_04b_FailedRule pins the Slice 2 Ralph-failed
// terminal rule. Mirror of 04a for the loop-failed path.
//
// Invariants:
//
//  1. Conditions match dev-via-test-execute role + outcome=failed.
//     NO measurement.pass condition (because if the loop failed,
//     measurement may have never been called; the FRAMEWORK termination
//     is the signal).
//  2. Stamps dev_via_test.execute.outcome=failed on Ralph's entity.
//  3. Stamps dev_via_test.execute.task_failed=<ralph-loop-id> on the
//     run entity for walker pickup.
//  4. Does NOT stamp chain.paused.marker — that's rule 08's job for
//     non-execute roles. Ralph failures go through coordinator
//     wake-up → ask_user (per ADR §Stuck-task recovery).
func TestDevViaTestPack_04b_FailedRule(t *testing.T) {
	rule := loadDevViaTestRule(t, "04b-execute-stamp-failed.json")

	if !devViaTestRoleCondition(rule, "dev-via-test-execute") {
		t.Error("rule 04b does not condition on agent.loop.role = dev-via-test-execute")
	}
	// Per Slice 2 reviewer R1: rule 04b matches all three terminal
	// non-success outcomes so context-length truncation and chain
	// cancellation don't silently wedge the walker.
	for _, outcome := range []string{"failed", "truncated", "cancelled"} {
		if !devViaTestOutcomeCondition(rule, outcome) {
			t.Errorf("rule 04b does not match agent.loop.outcome = %q — walker will silently wedge on this terminal class", outcome)
		}
	}

	var foundOwnOutcome, foundRunFailed bool
	for _, a := range rule.OnEnter {
		if a.Type != "add_triple" {
			continue
		}
		switch a.Predicate {
		case "dev_via_test.execute.outcome":
			if obj, ok := a.Object.(string); !ok || obj != "failed" {
				t.Errorf("rule 04b outcome stamp object = %v; want %q", a.Object, "failed")
			}
			foundOwnOutcome = true
		case "dev_via_test.execute.task_failed":
			if a.Subject != "$entity.triple.lineage.run-loop-entity-id" {
				t.Errorf("rule 04b task_failed subject = %q; want %q", a.Subject, "$entity.triple.lineage.run-loop-entity-id")
			}
			if obj, ok := a.Object.(string); !ok || obj != "$entity.instance" {
				t.Errorf("rule 04b task_failed object = %v; want %q", a.Object, "$entity.instance")
			}
			foundRunFailed = true
		case "chain.paused.marker":
			t.Error("rule 04b stamps chain.paused.marker — must NOT; Ralph failures go through coordinator wake-up + ask_user, not chain pause (per ADR-044 §Stuck-task recovery). Pause posture is for non-execute roles only (rule 08).")
		}
	}
	if !foundOwnOutcome {
		t.Error("rule 04b missing add_triple dev_via_test.execute.outcome=failed")
	}
	if !foundRunFailed {
		t.Error("rule 04b missing add_triple dev_via_test.execute.task_failed on run entity")
	}
}

// TestDevViaTestPack_08_LoopFailedPauseExcludesRalph pins the Slice 2
// chain-pause rule's role list. Ralph (dev-via-test-execute) must be
// EXCLUDED — its failures route through 04b → coordinator wake-up
// → ask_user, NOT chain pause. Mirrors autoresearch rule 11's
// exclusion of autoresearch-execute.
func TestDevViaTestPack_08_LoopFailedPauseExcludesRalph(t *testing.T) {
	rule := loadDevViaTestRule(t, "08-loop-failed-pause.json")
	for _, c := range rule.Conditions {
		if c.Field != "agent.loop.role" || c.Operator != "in" {
			continue
		}
		roles, ok := c.Value.([]any)
		if !ok {
			t.Fatalf("rule 08 agent.loop.role condition value is not an array: %T", c.Value)
		}
		for _, r := range roles {
			s, _ := r.(string)
			if s == "dev-via-test-execute" {
				t.Error("rule 08 includes dev-via-test-execute in its role list — must be excluded so Ralph failures route through 04b (coordinator wake-up + ask_user), not chain pause")
			}
		}
		// Lisa must be present (no in-arc recovery for Lisa).
		var foundLisa bool
		for _, r := range roles {
			if s, _ := r.(string); s == "dev-via-test-plan" {
				foundLisa = true
				break
			}
		}
		if !foundLisa {
			t.Error("rule 08 missing dev-via-test-plan in its role list — Lisa failures need to pause the chain (no auto-retry path)")
		}
		return
	}
	t.Error("rule 08 has no agent.loop.role condition with `in` operator")
}

// TestDevViaTestExecutePersonaTeachesIterationDiscipline asserts
// Ralph's persona corpus carries the load-bearing concepts: the
// target_files constraint (Karpathy Rule 3), the test_command
// convergence signal (Karpathy Rule 4), the emit_dev_via_test_measurement
// terminal contract, and the no-numeric-caps posture (per
// [[coordinator-first-not-persona-patches]] and ADR-044 §Stuck-task
// recovery).
func TestDevViaTestExecutePersonaTeachesIterationDiscipline(t *testing.T) {
	root := "../../configs/personas/fragments/dev-via-test-execute"
	got, err := concatFragmentsDevViaTest(root)
	if err != nil {
		t.Fatalf("read dev-via-test-execute fragments: %v", err)
	}
	for _, want := range []string{
		"target_files",                  // Karpathy Rule 3 constraint
		"test_command",                  // Karpathy Rule 4 convergence signal
		"emit_dev_via_test_measurement", // terminal commit tool
		"needs_clarification",           // escalation path
		"max_iterations",                // framework cap (not persona-level)
	} {
		if !strings.Contains(got, want) {
			t.Errorf("dev-via-test-execute persona missing %q — Ralph's iteration discipline depends on this concept being taught", want)
		}
	}
	// Negative check: persona MUST NOT carry numeric anchors that
	// would prime Ralph to reason about budget — per ADR-044
	// §Stuck-task recovery + [[coordinator-first-not-persona-patches]].
	// Per Slice 2 reviewer B1: tighter than a literal-string blocklist
	// because "typically 50" sailed through the original check.
	// Pattern matches 1-3 digits adjacent to cap-anchoring nouns.
	numericCap := regexp.MustCompile(`(?i)\b\d{1,3}\b[^.\n]{0,30}\b(iter|attempt|cap|ceiling|tries|retries|max_iterations)\b|\b(iter|attempt|cap|ceiling|tries|retries|max_iterations)\b[^.\n]{0,30}\b\d{1,3}\b`)
	if loc := numericCap.FindStringIndex(got); loc != nil {
		start := loc[0] - 30
		if start < 0 {
			start = 0
		}
		end := loc[1] + 30
		if end > len(got) {
			end = len(got)
		}
		t.Errorf("dev-via-test-execute persona contains a numeric-cap anchor near %q — per ADR-044, persona has no caps; framework safety ceiling exists for runaway protection, not as a budget Ralph reasons about", got[start:end])
	}
}

// TestDevViaTestSpawnRulesPinRunLoopEntityID is the structural
// fence for the cross-entity stamping pattern. Rules 04a/04b stamp
// triples on the run entity via $entity.triple.lineage.run-loop-
// entity-id substitution. If a spawn rule for any dev-via-test-*
// role forgets to pin "run-loop-entity-id" in related_loops, those
// stamp rules will error at fire time and the chain wedges silently.
//
// Per Slice 2 reviewer R2: pin the contract structurally so Slice 3
// (walker spawning Ralph) and Slice 4 (coordinator dispatching CBG)
// cannot ship without the wire-key in place. Today only rule 01
// (Lisa) spawns; this test ensures any future spawner stays
// consistent.
func TestDevViaTestSpawnRulesPinRunLoopEntityID(t *testing.T) {
	files, err := filepath.Glob(filepath.Join(devViaTestPackDir, "*.json"))
	if err != nil {
		t.Fatalf("glob dev-via-test pack: %v", err)
	}
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
			if a.Type != "publish_agent" {
				continue
			}
			// Pack-scope spawns: dev-via-test-* + reviewer-dev-via-test.
			if !strings.HasPrefix(a.Role, "dev-via-test-") && a.Role != "reviewer-dev-via-test" {
				continue
			}
			if v, ok := a.RelatedLoops["run-loop-entity-id"]; !ok || v == "" {
				t.Errorf("%s spawn of role %q missing related_loops[run-loop-entity-id] — rules 04a/04b/(future 06/07) stamp via $entity.triple.lineage.run-loop-entity-id and will error at fire time without this key (silent chain wedge)",
					filepath.Base(path), a.Role)
			}
		}
	}
}

// TestDevViaTestPack_08_RoleListInventory pins the full set of
// non-execute roles whose loop-failed outcomes must trigger chain
// pause. Mirrors the autoresearch rule 11 inventory check (per
// Slice 2 reviewer N8). Slice 4 extended to include CBG
// (reviewer-dev-via-test). Future packs that add non-execute
// roles MUST grow this set + rule 08's `in` list together — the
// test fails at PR time, not at smoke time.
func TestDevViaTestPack_08_RoleListInventory(t *testing.T) {
	wantRoles := map[string]struct{}{
		"dev-via-test-plan":     {}, // Lisa (Slice 1)
		"reviewer-dev-via-test": {}, // CBG (Slice 4)
		// dev-via-test-execute (Ralph) is DELIBERATELY excluded —
		// Ralph failures route through rule 04b (walker wake-up +
		// ask_user, no chain pause).
	}
	rule := loadDevViaTestRule(t, "08-loop-failed-pause.json")
	var gotRoles map[string]struct{}
	for _, c := range rule.Conditions {
		if c.Field != "agent.loop.role" || c.Operator != "in" {
			continue
		}
		roles, ok := c.Value.([]any)
		if !ok {
			t.Fatalf("rule 08 agent.loop.role condition value is not an array: %T", c.Value)
		}
		gotRoles = make(map[string]struct{}, len(roles))
		for _, r := range roles {
			if s, ok := r.(string); ok {
				gotRoles[s] = struct{}{}
			}
		}
		break
	}
	if gotRoles == nil {
		t.Fatal("rule 08 has no agent.loop.role condition with `in` operator — chain pause coverage absent")
	}
	if !reflect.DeepEqual(gotRoles, wantRoles) {
		t.Errorf("rule 08 role list = %v; want exactly %v — any non-Ralph dev-via-test role spawned in the pack must appear so its failures pause the chain (and only such roles; Ralph's failures route through rule 04b instead)",
			gotRoles, wantRoles)
	}
}

// --- helpers added in Slice 2 ---

// devViaTestOutcomeCondition accepts either operator=eq with
// matching string, OR operator=in with the outcome present in the
// list. Per Slice 2 reviewer R1: rule 04b widened from eq=failed
// to in=[failed,truncated,cancelled] to cover all three terminal
// non-success classes.
func devViaTestOutcomeCondition(r *devViaTestRuleJSON, outcome string) bool {
	for _, c := range r.Conditions {
		if c.Field != "agent.loop.outcome" {
			continue
		}
		switch c.Operator {
		case "eq":
			if s, ok := c.Value.(string); ok && s == outcome {
				return true
			}
		case "in":
			if list, ok := c.Value.([]any); ok {
				for _, v := range list {
					if s, ok := v.(string); ok && s == outcome {
						return true
					}
				}
			}
		}
	}
	return false
}

func devViaTestMeasurementPassCondition(r *devViaTestRuleJSON, want bool) bool {
	for _, c := range r.Conditions {
		if c.Field == "dev_via_test.measurement.pass" && c.Operator == "eq" {
			if b, ok := c.Value.(bool); ok && b == want {
				return true
			}
		}
	}
	return false
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
	Properties    map[string]string         `json:"properties,omitempty"`
	ForEach       string                    `json:"for_each,omitempty"`
	ForEachVar    string                    `json:"for_each_var,omitempty"`
	When          []devViaTestConditionJSON `json:"when,omitempty"`
	// MaxIterations is a pointer so an explicit 0 (unlimited per-action
	// firing — load-bearing for budget-gated re-dispatch, Slice 5/6) is
	// distinguishable from an absent field (default cap 3).
	MaxIterations *int `json:"max_iterations,omitempty"`
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
