package contract

import (
	"encoding/json"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// ADR-053 Phase 4a — structural contract for the agent-run substrate rule pack.
//
// These are the design invariants the architect + Coby reviews made
// load-bearing (docs/adr/053-adoption-plan.md §Phase 4 design spike). They are
// CHEAP structural pins; the state-machine SAFETY (marker-before-executing,
// duplicate terminal, restart mid-run, publish-fail-after-mint) is proven by the
// e2e journeys + failure-injection tests, not here.

type ruleDoc struct {
	ID         string `json:"id"`
	Conditions []struct {
		Field    string `json:"field"`
		Operator string `json:"operator"`
		Value    any    `json:"value"`
	} `json:"conditions"`
	OnEnter []struct {
		Type            string            `json:"type"`
		Subject         string            `json:"subject"`
		Predicate       string            `json:"predicate"`
		Object          any               `json:"object"`
		Workflow        string            `json:"workflow"`
		Phase           string            `json:"phase"`
		When            json.RawMessage   `json:"when"`
		RelatedLoops    map[string]string `json:"related_loops"`
		Role            string            `json:"role"`
		ActionAllowlist []string          `json:"action_allowlist"`
		Tools           []string          `json:"tools"`
	} `json:"on_enter"`
}

// sameStrings reports whether two string slices are element-wise equal.
func sameStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func loadRule(t *testing.T, path string) ruleDoc {
	t.Helper()
	raw, err := os.ReadFile(path) //nolint:gosec // test-controlled config path
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	var r ruleDoc
	if err := json.Unmarshal(raw, &r); err != nil {
		t.Fatalf("unmarshal %s: %v", path, err)
	}
	return r
}

func (r ruleDoc) hasCondition(field, op string, value any) bool {
	for _, c := range r.Conditions {
		if c.Field == field && c.Operator == op && c.Value == value {
			return true
		}
	}
	return false
}

// hasConditionField matches a (field, operator) pair regardless of value — used
// for the length_eq/length_gt anchor fences whose value is a JSON number (which
// hasCondition's == against an int/literal would not match).
func (r ruleDoc) hasConditionField(field, op string) bool {
	for _, c := range r.Conditions {
		if c.Field == field && c.Operator == op {
			return true
		}
	}
	return false
}

// roleValues flattens an agent.loop.role condition value into role tokens,
// handling both the eq form (a string) and the in form (a JSON array).
func roleValues(v any) []string {
	switch x := v.(type) {
	case string:
		return []string{x}
	case []any:
		out := make([]string, 0, len(x))
		for _, e := range x {
			if s, ok := e.(string); ok {
				out = append(out, s)
			}
		}
		return out
	}
	return nil
}

// TestAgentRunPack_HandoffMarker pins rule 01: it bridges the firing-entity gap
// by firing on the coordinator loop (confirmed handoff) and stamping
// agent.run.handoff on the run entity. The rule.spawned_task gate (not bare
// mint) is Coby review P1a — publish-failure safety.
func TestAgentRunPack_HandoffMarker(t *testing.T) {
	r := loadRule(t, "../../configs/rules/agent-run/01-handoff-marker.json")

	if !r.hasCondition("agent.loop.role", "eq", "coordinator") {
		t.Error("handoff marker must fire on the coordinator loop (agent.loop.role==coordinator)")
	}
	// rule.spawned_task is the CONFIRMED-HANDOFF gate (post-publish-success),
	// NOT bare mint — the load-bearing P1a invariant.
	if !r.hasCondition("rule.spawned_task", "ne", "") {
		t.Error("handoff marker must gate on rule.spawned_task != \"\" (confirmed handoff, Coby review P1a) — NOT on agent.run.phase==dispatched (bare mint, which survives a publish failure)")
	}
	// agent.run.entity_id presence is the run-less-chat guard.
	if !r.hasCondition("agent.run.entity_id", "ne", "") {
		t.Error("handoff marker must gate on agent.run.entity_id != \"\" (run-less-chat guard)")
	}
	// Must NOT trigger on bare mint.
	if r.hasCondition("agent.run.phase", "eq", "dispatched") {
		t.Error("handoff marker must NOT gate on agent.run.phase==dispatched — that is bare-mint triggering (P1a regression)")
	}

	var stampsHandoff bool
	for _, a := range r.OnEnter {
		if a.Type == "add_triple" && a.Predicate == "agent.run.handoff" {
			stampsHandoff = true
			if a.Subject != "$entity.triple.agent.run.entity_id" {
				t.Errorf("handoff stamp subject = %q, want $entity.triple.agent.run.entity_id (the mint-stamped run anchor on the coordinator)", a.Subject)
			}
		}
	}
	if !stampsHandoff {
		t.Error("handoff marker must add_triple agent.run.handoff on the run entity")
	}
}

// TestAgentRunPack_TransitionsPhaseGuardedTopLevel pins the load-bearing Coby
// review round-2 invariant: the phase guard is a TOP-LEVEL CONDITION, never an
// action `when`. A `when`-buried phase guard would let the rule enter on the
// outcome alone, skip the action while in the wrong phase, and never re-enter.
func TestAgentRunPack_TransitionsPhaseGuardedTopLevel(t *testing.T) {
	cases := []struct {
		path      string
		fromPhase string // top-level phase guard
		toPhase   string // lifecycle_transition target
		trigger   string // the other top-level condition
	}{
		{"../../configs/rules/agent-run/02-dispatched-to-executing.json", "dispatched", "executing", "agent.run.handoff"},
		{"../../configs/rules/agent-run/03-executing-to-completed.json", "executing", "completed", "agent.run.outcome"},
		{"../../configs/rules/agent-run/04-executing-to-failed.json", "executing", "failed", "agent.run.outcome"},
		{"../../configs/rules/agent-run/09-executing-to-awaiting-on-clarification.json", "executing", "awaiting_approval", "agent.run.clarification_pending"},
		{"../../configs/rules/agent-run/11-resume-awaiting-to-executing.json", "awaiting_approval", "executing", "agent.run.clarification_resumed"},
	}
	for _, tc := range cases {
		r := loadRule(t, tc.path)

		// Phase guard is a TOP-LEVEL CONDITION.
		if !r.hasCondition("agent.run.phase", "eq", tc.fromPhase) {
			t.Errorf("%s: agent.run.phase==%s must be a TOP-LEVEL condition (Coby review round 2 — never an action `when`)", tc.path, tc.fromPhase)
		}
		// The driving trigger is also a top-level condition.
		var hasTrigger bool
		for _, c := range r.Conditions {
			if c.Field == tc.trigger {
				hasTrigger = true
			}
		}
		if !hasTrigger {
			t.Errorf("%s: missing top-level trigger condition on %s", tc.path, tc.trigger)
		}

		// Exactly one lifecycle_transition to the right phase, workflow=agent-run,
		// and NO action carries a `when` (the phase guard must not leak into when).
		var transitions int
		for _, a := range r.OnEnter {
			if len(a.When) > 0 {
				t.Errorf("%s: on_enter action carries a `when` — the phase guard must be a top-level condition, not a `when` (Coby review round 2)", tc.path)
			}
			if a.Type == "lifecycle_transition" {
				transitions++
				if a.Workflow != "agent-run" {
					t.Errorf("%s: lifecycle_transition workflow = %q, want agent-run", tc.path, a.Workflow)
				}
				if a.Phase != tc.toPhase {
					t.Errorf("%s: lifecycle_transition phase = %q, want %s", tc.path, a.Phase, tc.toPhase)
				}
			}
		}
		if transitions != 1 {
			t.Errorf("%s: want exactly 1 lifecycle_transition action, got %d", tc.path, transitions)
		}
	}
}

// TestAgentRunPack_SuccessOutcomeStamps pins that the three reviewer/CBG-approved
// terminal rules stamp agent.run.outcome=success on the run entity, each with the
// per-pack run-anchor subject (research uses agent.run.entity_id; autoresearch +
// dev-via-test descend through run-entity-fired rules so carry only
// lineage.run-loop-entity-id). This is the §E per-pack-anchor correction.
func TestAgentRunPack_SuccessOutcomeStamps(t *testing.T) {
	cases := []struct {
		path    string
		subject string
	}{
		{"../../configs/rules/research/07-reviewer-approved-to-coordinator.json", "$entity.triple.agent.run.entity_id"},
		{"../../configs/rules/autoresearch/08-reviewer-approved-to-coordinator.json", "$entity.triple.lineage.run-loop-entity-id"},
		{"../../configs/rules/dev-via-test/07a-cbg-approved-to-coordinator.json", "$entity.triple.lineage.run-loop-entity-id"},
	}
	for _, tc := range cases {
		r := loadRule(t, tc.path)
		var found bool
		for _, a := range r.OnEnter {
			if a.Type == "add_triple" && a.Predicate == "agent.run.outcome" {
				found = true
				if obj, _ := a.Object.(string); obj != "success" {
					t.Errorf("%s: agent.run.outcome object = %v, want \"success\"", tc.path, a.Object)
				}
				if a.Subject != tc.subject {
					t.Errorf("%s: agent.run.outcome subject = %q, want %q (per-pack run anchor — §E)", tc.path, a.Subject, tc.subject)
				}
			}
		}
		if !found {
			t.Errorf("%s: must stamp agent.run.outcome=success on the run entity (drives executing→completed)", tc.path)
		}
	}
}

// TestAgentRunPack_SuccessStampAnchorThreaded is the SEMANTIC pin that the
// per-rule subject check (TestAgentRunPack_SuccessOutcomeStamps) cannot give: a
// `$entity.triple.lineage.<key>` stamp subject only resolves if the rule that
// SPAWNED the stamping loop threads <key> in its related_loops. Without it the
// substitution leaves a literal token and the run never completes.
//
// go-reviewer C1/C2: autoresearch/08 stamped lineage.run-loop-entity-id, but its
// spawner autoresearch/07 threaded only autoresearch-run — so the subject resolved
// to a garbage literal and every autoresearch run hung in `executing`. The
// per-rule subject test passed green because it pinned the author's intent string,
// not that the anchor resolves on the firing role. This test closes that gap.
//
// research/07 uses agent.run.entity_id (framework-inherited via the loop-spawn
// chain, NOT a related_loops thread), so it is not checked here.
func TestAgentRunPack_SuccessStampAnchorThreaded(t *testing.T) {
	cases := []struct {
		pack       string
		spawnRule  string // the rule that spawns the loop the success stamp fires on
		lineageKey string // the related_loops key the stamp's lineage.<key> subject reads
	}{
		{"autoresearch", "../../configs/rules/autoresearch/07-synthesize-to-reviewer.json", "run-loop-entity-id"},
		{"dev-via-test", "../../configs/rules/dev-via-test/06-coordinator-dispatch-cbg.json", "run-loop-entity-id"},
	}
	for _, tc := range cases {
		r := loadRule(t, tc.spawnRule)
		var threaded bool
		for _, a := range r.OnEnter {
			if _, ok := a.RelatedLoops[tc.lineageKey]; ok {
				threaded = true
				break
			}
		}
		if !threaded {
			t.Errorf("%s: spawn rule %s must thread related_loops[%q] so the downstream success stamp's "+
				"$entity.triple.lineage.%s subject resolves on the terminal loop (else it writes to a literal "+
				"token and the run never completes — go-reviewer C1)", tc.pack, tc.spawnRule, tc.lineageKey, tc.lineageKey)
		}
	}
}

// TestAgentRunPack_WiredInBothConfigs pins that all three agent-run rules are
// loaded by BOTH the production and e2e bootstrap configs — a Phase-4a rule that
// ships in prod but not e2e (or vice-versa) would pass mock journeys yet break in
// production, the structural-wiring gap class.
func TestAgentRunPack_WiredInBothConfigs(t *testing.T) {
	wantFiles := []string{
		"/app/configs/rules/agent-run/01-handoff-marker.json",
		"/app/configs/rules/agent-run/02-dispatched-to-executing.json",
		"/app/configs/rules/agent-run/03-executing-to-completed.json",
		"/app/configs/rules/agent-run/04-executing-to-failed.json",
		"/app/configs/rules/agent-run/05-coordinator-failed-run-anchor.json",
		"/app/configs/rules/agent-run/06-coordinator-failed-lineage-anchor.json",
		"/app/configs/rules/agent-run/07-ask-user-pause-run-anchor.json",
		"/app/configs/rules/agent-run/08-ask-user-pause-lineage-anchor.json",
		"/app/configs/rules/agent-run/09-executing-to-awaiting-on-clarification.json",
		"/app/configs/rules/agent-run/10-clarification-reply-resume-marker.json",
		"/app/configs/rules/agent-run/11-resume-awaiting-to-executing.json",
	}
	for _, cfg := range []string{"../../configs/flow-bootstrap.json", "../../configs/e2e-flow-bootstrap.json"} {
		raw, err := os.ReadFile(cfg) //nolint:gosec // test-controlled config path
		if err != nil {
			t.Fatalf("read %s: %v", cfg, err)
		}
		s := string(raw)
		for _, f := range wantFiles {
			if !strings.Contains(s, f) {
				t.Errorf("%s: missing agent-run rule %q from rules_files", cfg, f)
			}
		}
	}
}

// failedStampRule pairs a per-pack/coordinator loop-failed run-outcome rule with
// the run-anchor subject its agent.run.outcome=failed stamp must use. The anchor
// differs by spawn path (ADR-053 4a′ §E, the same asymmetry as the success path):
// agent.run.entity_id for loop-spawn-chain roles (research, baseline, first-pass
// Lisa, loop-fired coordinators), lineage.run-loop-entity-id for run-entity-
// descended roles (autoresearch propose/synthesize/reviewer, re-plan Lisa, CBG,
// run-entity-spawned coordinators).
var failedStampRules = []struct {
	path    string
	subject string
}{
	{"../../configs/rules/research/09-loop-failed-run-outcome.json", "$entity.triple.agent.run.entity_id"},
	{"../../configs/rules/autoresearch/12-baseline-loop-failed-run-outcome.json", "$entity.triple.agent.run.entity_id"},
	{"../../configs/rules/autoresearch/13-loop-failed-run-outcome.json", "$entity.triple.lineage.run-loop-entity-id"},
	{"../../configs/rules/dev-via-test/09-firstpass-loop-failed-run-outcome.json", "$entity.triple.agent.run.entity_id"},
	{"../../configs/rules/dev-via-test/10-loop-failed-run-outcome.json", "$entity.triple.lineage.run-loop-entity-id"},
	{"../../configs/rules/agent-run/05-coordinator-failed-run-anchor.json", "$entity.triple.agent.run.entity_id"},
	{"../../configs/rules/agent-run/06-coordinator-failed-lineage-anchor.json", "$entity.triple.lineage.run-loop-entity-id"},
}

// TestAgentRunPack_FailedOutcomeStamps is the failure-side mirror of
// TestAgentRunPack_SuccessOutcomeStamps: every loop-failed run-outcome rule must
// stamp agent.run.outcome=failed on the run entity with the correct per-pack
// run anchor (ADR-053 4a′). The marker is what drives agent-run/04 executing→failed.
func TestAgentRunPack_FailedOutcomeStamps(t *testing.T) {
	for _, tc := range failedStampRules {
		r := loadRule(t, tc.path)
		var found bool
		for _, a := range r.OnEnter {
			if a.Type == "add_triple" && a.Predicate == "agent.run.outcome" {
				found = true
				if obj, _ := a.Object.(string); obj != "failed" {
					t.Errorf("%s: agent.run.outcome object = %v, want \"failed\"", tc.path, a.Object)
				}
				if a.Subject != tc.subject {
					t.Errorf("%s: agent.run.outcome subject = %q, want %q (per-pack run anchor — ADR-053 4a′ §E)", tc.path, a.Subject, tc.subject)
				}
			}
		}
		if !found {
			t.Errorf("%s: must stamp agent.run.outcome=failed on the run entity (drives executing→failed)", tc.path)
		}
	}
}

// TestAgentRunPack_FailedStampAnchorGuarded closes the failure-side go-reviewer
// C1 class: a $entity.triple.<anchor> subject that resolves to an unset triple
// becomes a literal token, the stamp lands on garbage, and the run hangs
// `executing` forever. So every lineage-subject stamp MUST guard on
// lineage.run-loop-entity-id PRESENCE (length_gt 0) and every agent.run.entity_id
// subject stamp MUST guard on agent.run.entity_id != "". The two coordinator
// rules additionally fence on lineage presence so they are mutually exclusive.
func TestAgentRunPack_FailedStampAnchorGuarded(t *testing.T) {
	for _, tc := range failedStampRules {
		r := loadRule(t, tc.path)
		switch tc.subject {
		case "$entity.triple.lineage.run-loop-entity-id":
			if !r.hasConditionField("lineage.run-loop-entity-id", "length_gt") {
				t.Errorf("%s: a lineage.run-loop-entity-id failed-stamp must guard on `lineage.run-loop-entity-id length_gt 0` (C1 garbage-literal-subject defense)", tc.path)
			}
		case "$entity.triple.agent.run.entity_id":
			if !r.hasCondition("agent.run.entity_id", "ne", "") {
				t.Errorf("%s: an agent.run.entity_id failed-stamp must guard on `agent.run.entity_id != \"\"` (anchor-present / run-less-chat guard)", tc.path)
			}
		}
	}

	// The two coordinator-failed rules must be mutually exclusive on lineage
	// presence (length_eq 0 vs length_gt 0), so a coordinator carrying both
	// anchors routes to exactly one and is never double-stamped.
	c05 := loadRule(t, "../../configs/rules/agent-run/05-coordinator-failed-run-anchor.json")
	if !c05.hasConditionField("lineage.run-loop-entity-id", "length_eq") {
		t.Error("agent-run/05 must fence on `lineage.run-loop-entity-id length_eq 0` (mutually exclusive with rule 06)")
	}
}

// TestAgentRunPack_AskUserPauseMarkers pins the 4b-2 interactive-pause marker
// pair (07/08): each fires on a coordinator decide(ask_user) WITHIN a run and
// stamps agent.run.clarification_pending on the run entity via the correct
// per-anchor subject, fenced so the subject resolves and the pair is mutually
// exclusive (same anchor discipline as the coordinator-failed pair 05/06). The
// next_action==ask_user trigger is ALSO the autonomous-inertness gate: under
// restricted_decide_actions=["ask_user"] the framework rejects decide(ask_user)
// BEFORE that triple is stamped, so neither rule can fire in autonomous mode —
// 4b-2 is interactive-only by construction.
func TestAgentRunPack_AskUserPauseMarkers(t *testing.T) {
	cases := []struct {
		path      string
		subject   string
		lineageOp string // the lineage fence operator
	}{
		{"../../configs/rules/agent-run/07-ask-user-pause-run-anchor.json", "$entity.triple.agent.run.entity_id", "length_eq"},
		{"../../configs/rules/agent-run/08-ask-user-pause-lineage-anchor.json", "$entity.triple.lineage.run-loop-entity-id", "length_gt"},
	}
	for _, tc := range cases {
		r := loadRule(t, tc.path)
		if !r.hasCondition("agent.loop.role", "eq", "coordinator") {
			t.Errorf("%s: must fire on agent.loop.role==coordinator", tc.path)
		}
		// The ask_user trigger — ALSO the autonomous-inertness gate.
		if !r.hasCondition("coordinator.decision.next_action", "eq", "ask_user") {
			t.Errorf("%s: must trigger on coordinator.decision.next_action==ask_user — the trigger AND the autonomous-inertness "+
				"gate (decide(ask_user) is rejected pre-stamp under restricted_decide_actions, so this rule can't fire in autonomous mode)", tc.path)
		}
		var stamps bool
		for _, a := range r.OnEnter {
			if a.Type == "add_triple" && a.Predicate == "agent.run.clarification_pending" {
				stamps = true
				if a.Subject != tc.subject {
					t.Errorf("%s: clarification_pending subject = %q, want %q (per-anchor run subject)", tc.path, a.Subject, tc.subject)
				}
			}
		}
		if !stamps {
			t.Errorf("%s: must add_triple agent.run.clarification_pending on the run entity (drives agent-run/09 executing→awaiting_approval)", tc.path)
		}
		// Anchor fence so the subject resolves + the pair is mutually exclusive.
		if !r.hasConditionField("lineage.run-loop-entity-id", tc.lineageOp) {
			t.Errorf("%s: must fence on `lineage.run-loop-entity-id %s 0` (subject-resolves + mutual exclusion with the sibling — mirrors 05/06)", tc.path, tc.lineageOp)
		}
	}
	// Rule 07 (run-anchor) additionally guards agent.run.entity_id != "" — the
	// run-less-chat guard (a front-door ask_user has no run to pause).
	r07 := loadRule(t, "../../configs/rules/agent-run/07-ask-user-pause-run-anchor.json")
	if !r07.hasCondition("agent.run.entity_id", "ne", "") {
		t.Error("agent-run/07 must guard agent.run.entity_id != \"\" (run-less-chat guard — a front-door ask_user has no run to pause)")
	}
}

// TestAgentRunPack_AskUserPauseAutonomousInert is the NAMED structural pin the
// 4b-2 rule metadata + README reference: the pause markers (07/08) CANNOT fire in
// autonomous mode. The mechanism is UPSTREAM ORDERING — under
// agentic-tools.restricted_decide_actions=["ask_user"] the framework rejects
// decide(ask_user) (ToolErrorInvalidArgs) BEFORE the
// coordinator.decision.next_action=ask_user triple is published (semstreams
// beta.104 decide.go:307-322 runs before the next_action stamp at :362-370). Both
// pause markers gate on that triple, so neither can enter in autonomous mode — no
// rule-level autonomous gate is needed. This test makes the named claim concrete
// (the structural proxy: 07/08 trigger ONLY on next_action==ask_user). The
// end-to-end proof that autonomous mode stamps ZERO such triples is the shipped
// clarification-autonomous.spec.ts.
func TestAgentRunPack_AskUserPauseAutonomousInert(t *testing.T) {
	for _, path := range []string{
		"../../configs/rules/agent-run/07-ask-user-pause-run-anchor.json",
		"../../configs/rules/agent-run/08-ask-user-pause-lineage-anchor.json",
	} {
		r := loadRule(t, path)
		if !r.hasCondition("coordinator.decision.next_action", "eq", "ask_user") {
			t.Errorf("%s: must gate on coordinator.decision.next_action==ask_user — that triple is what makes the rule "+
				"autonomous-inert (restricted_decide_actions rejects decide(ask_user) before the triple is stamped). Without "+
				"this exact gate the inertness claim in the rule metadata + README would be false.", path)
		}
	}
}

// TestAgentRunPack_ClarificationResumeMarker pins the 4b-2 PR-2 resume marker
// (rule 10): a coordinator loop re-entering as a clarification REPLY stamps
// agent.run.clarification_resumed on the run entity via the run-anchor subject
// override. Single rule (no 07/08-style anchor split) because semstreams#256
// threads the run anchor onto the reply, so the reply loop always carries
// agent.run.entity_id. The reply discriminator (lineage.clarification-reply) is
// what distinguishes a reply from any other coordinator loop in the run.
func TestAgentRunPack_ClarificationResumeMarker(t *testing.T) {
	r := loadRule(t, "../../configs/rules/agent-run/10-clarification-reply-resume-marker.json")
	if !r.hasCondition("agent.loop.role", "eq", "coordinator") {
		t.Error("agent-run/10: must fire on agent.loop.role==coordinator")
	}
	// The run anchor (Thread 1) — present on the reply loop only post-#256.
	if !r.hasCondition("agent.run.entity_id", "ne", "") {
		t.Error("agent-run/10: must require agent.run.entity_id != \"\" (the run anchor threaded by semstreams#256)")
	}
	// The reply discriminator (Thread 2) — the loop-local signal that this is a
	// clarification reply, not just any run coordinator.
	if !r.hasCondition("lineage.clarification-reply", "ne", "") {
		t.Error("agent-run/10: must require lineage.clarification-reply != \"\" (the reply discriminator — without it the rule would fire on every run coordinator)")
	}
	var stamps bool
	for _, a := range r.OnEnter {
		if a.Type == "add_triple" && a.Predicate == "agent.run.clarification_resumed" {
			stamps = true
			if a.Subject != "$entity.triple.agent.run.entity_id" {
				t.Errorf("agent-run/10: clarification_resumed subject = %q, want $entity.triple.agent.run.entity_id (the run entity)", a.Subject)
			}
		}
	}
	if !stamps {
		t.Error("agent-run/10: must add_triple agent.run.clarification_resumed on the run entity (drives agent-run/11 awaiting_approval→executing)")
	}
}

// TestAgentRunPack_ClarificationResumeTransition pins the 4b-2 PR-2 resume
// transition (rule 11) AND the bounce-proof contract that depends on action
// ORDER. The on_enter must be EXACTLY [lifecycle_transition→executing,
// remove clarification_pending, remove clarification_resumed] in that order:
// combined with rule 09's clarification_resumed length_eq 0 guard, every
// intermediate KV revision keeps rule 09 blocked, so the run can never bounce
// back to awaiting_approval. clarification_pending MUST be removed before
// clarification_resumed (else the resumed-set-but-pending-gone window would
// re-arm rule 09 once resumed is cleared). A regression that reorders these or
// drops a remove re-introduces the infinite pause↔resume bounce.
func TestAgentRunPack_ClarificationResumeTransition(t *testing.T) {
	r := loadRule(t, "../../configs/rules/agent-run/11-resume-awaiting-to-executing.json")
	if !r.hasCondition("agent.run.clarification_resumed", "ne", "") {
		t.Error("agent-run/11: must trigger on agent.run.clarification_resumed != \"\"")
	}
	if len(r.OnEnter) != 3 {
		t.Fatalf("agent-run/11: want exactly 3 on_enter actions [transition, remove pending, remove resumed], got %d", len(r.OnEnter))
	}
	if r.OnEnter[0].Type != "lifecycle_transition" || r.OnEnter[0].Phase != "executing" {
		t.Errorf("agent-run/11: on_enter[0] must be lifecycle_transition→executing, got type=%q phase=%q", r.OnEnter[0].Type, r.OnEnter[0].Phase)
	}
	if r.OnEnter[1].Type != "remove_triple" || r.OnEnter[1].Predicate != "agent.run.clarification_pending" {
		t.Errorf("agent-run/11: on_enter[1] must remove agent.run.clarification_pending (BEFORE clarification_resumed — bounce-proof order), got type=%q predicate=%q", r.OnEnter[1].Type, r.OnEnter[1].Predicate)
	}
	if r.OnEnter[2].Type != "remove_triple" || r.OnEnter[2].Predicate != "agent.run.clarification_resumed" {
		t.Errorf("agent-run/11: on_enter[2] must remove agent.run.clarification_resumed (AFTER clarification_pending), got type=%q predicate=%q", r.OnEnter[2].Type, r.OnEnter[2].Predicate)
	}
	// neither remove may carry a subject override — both must default to the
	// firing entity (the run entity) so the run's own markers are cleared.
	for i := 1; i <= 2; i++ {
		if r.OnEnter[i].Subject != "" {
			t.Errorf("agent-run/11: on_enter[%d] remove_triple must NOT set a subject (defaults to the firing run entity); got %q", i, r.OnEnter[i].Subject)
		}
	}
}

// TestAgentRunPack_ClarificationResumeJourney_BlockedOnUpstream is a tracked,
// non-optional gate for the DEFERRED behavioral mock journey (go-reviewer R2).
// The 4b-2 resume rules (10/11) are inert until semstreams#256 threads the run
// anchor + reply identity onto the reply TaskMessage; the e2e harness has only a
// triple-READ helper, so a faithful seeded journey would need a net-new write
// seam. When #256 lands, the real reply path produces the trigger triples, so a
// `clarification-resume` Playwright journey can drive the resume FOR REAL (assert
// awaiting_approval→executing + both markers cleared + NO re-pause) — a strictly
// better gate than a seeded fake. This Skip keeps the obligation VISIBLE (it
// surfaces in `go test -v` output) so the journey can't silently evaporate from
// the #256 adoption PR. Delete this test when the journey lands.
func TestAgentRunPack_ClarificationResumeJourney_BlockedOnUpstream(t *testing.T) {
	t.Skip("blocked on semstreams#256: the behavioral clarification-resume mock journey lands WITH #256 (drives the real reply path, no seeding). Tracked gate — see configs/rules/agent-run/README.md §Resume + docs/adr/053-adoption-plan.md §4b-2 PR-2.")
}

// TestAgentRunPack_PauseResumeBounceGuard pins the structural mutual-exclusion
// that makes the pause↔resume cycle bounce-proof: rule 09 (pause) must carry the
// agent.run.clarification_resumed length_eq 0 guard, so the instant rule 10
// stamps clarification_resumed the pause rule is dead until rule 11 clears both
// markers. Dropping this guard re-introduces the infinite bounce (rule 11's
// transition back to executing would re-trip rule 09 while pending is still set).
func TestAgentRunPack_PauseResumeBounceGuard(t *testing.T) {
	r := loadRule(t, "../../configs/rules/agent-run/09-executing-to-awaiting-on-clarification.json")
	if !r.hasConditionField("agent.run.clarification_resumed", "length_eq") {
		t.Error("agent-run/09: must guard `agent.run.clarification_resumed length_eq 0` — the bounce-proof mutual-exclusion with the resume (rule 10/11). Without it, rule 11's awaiting_approval→executing re-trips rule 09 → infinite pause↔resume bounce.")
	}
}

// TestAgentRunPack_BudgetedRolesExcludedFromFailedStamp pins the load-bearing
// budgeted-vs-fatal invariant: autoresearch-execute (via rule 04b) and
// dev-via-test-execute/Ralph (via 04b+05→coordinator ask_user) are BUDGETED
// loop-failures that must keep the run `executing`. A regression that adds either
// to a failed-outcome-stamp role list would wrongly fail the run on a budgeted
// iteration.
func TestAgentRunPack_BudgetedRolesExcludedFromFailedStamp(t *testing.T) {
	budgeted := []string{"autoresearch-execute", "dev-via-test-execute"}
	for _, tc := range failedStampRules {
		r := loadRule(t, tc.path)
		for _, c := range r.Conditions {
			if c.Field != "agent.loop.role" {
				continue
			}
			for _, role := range roleValues(c.Value) {
				for _, b := range budgeted {
					if role == b {
						t.Errorf("%s: budgeted role %q must NOT be in a failed-outcome-stamp role list — a budgeted loop-failure keeps the run executing (rule 04b)", tc.path, b)
					}
				}
			}
		}
	}
}

// Coordinator-spawn-rule dispositions for the ADR-053 4a′ executing→failed
// boundary. A coordinator that fails involuntarily WHILE the run is `executing`
// must drive executing→failed (agent-run/04) via a marker stamped by the
// coordinator-failed rules (agent-run/05 keyed on agent.run.entity_id; 06 keyed
// on lineage.run-loop-entity-id). That only works if the woken coordinator
// carries a run anchor those rules can read. The adversarial review (ADR-053
// 4a′) showed some woken coordinators carry NEITHER → a silent executing-zombie.
// This map + TestAgentRunPack_CoordinatorSpawnCoverage make the boundary
// STRUCTURAL: every coordinator-spawn rule must be deliberately classified, and
// each class carries a checkable invariant.
const (
	// dispPostApproval: the rule ALSO stamps agent.run.outcome=success, so
	// agent-run/03 transitions the run to `completed` BEFORE the woken
	// coordinator could fail — its failure is post-terminal, no coverage needed.
	dispPostApproval = "post_approval"
	// dispAnchorThreaded: the rule threads related_loops[run-loop-entity-id], so
	// the woken coordinator carries lineage.run-loop-entity-id → failure routes
	// to agent-run/06.
	dispAnchorThreaded = "anchor_threaded"
	// dispAnchorInherit: the rule fires only on loop-inherit-chain roles (the
	// research pack has no run-entity-fired spawn), so the woken coordinator
	// inherits agent.run.entity_id from its parent's bare agent.run → failure
	// routes to agent-run/05.
	dispAnchorInherit = "anchor_inherit"
	// dispDeferred4b: the woken coordinator is a pure human-in-the-loop delivery
	// (action_allowlist ⊆ {respond_direct, ask_user}) on an
	// ask_user/needs_clarification/rejected path whose run-phase semantics
	// ADR-053 assigned to Phase 4b. ADR-053 Phase 4b-1a CLOSED every such
	// deferral: each recovery coordinator now carries a readable run anchor
	// (anchor_inherit where the firing role has a bare agent.run; anchor_threaded
	// otherwise) so an involuntary failure drives executing→failed via
	// agent-run/05 or 06. No rule maps here today; the class is retained because
	// a future pack MAY introduce a new deferred path before its anchor is wired.
	dispDeferred4b = "deferred_4b"
)

var coordinatorSpawnDisposition = map[string]string{
	"research_reviewer_approved_to_coordinator":                   dispPostApproval,   // research/07
	"autoresearch_reviewer_approved_to_coordinator":               dispPostApproval,   // autoresearch/08
	"dev_via_test_cbg_approved_to_coordinator":                    dispPostApproval,   // dev-via-test/07a
	"dev_via_test_plan_approved_to_coordinator":                   dispAnchorThreaded, // dev-via-test/02b
	"dev_via_test_plan_retry_driver":                              dispAnchorThreaded, // dev-via-test/02d
	"dev_via_test_ralph_terminal_to_coordinator":                  dispAnchorThreaded, // dev-via-test/05
	"dev_via_test_cbg_retry_driver":                               dispAnchorThreaded, // dev-via-test/07d
	"research_needs_clarification_to_coordinator":                 dispAnchorInherit,  // research/06
	"autoresearch_needs_clarification_replan":                     dispAnchorInherit,  // autoresearch/10 (baseline — 4b-1a)
	"autoresearch_descended_needs_clarification_replan":           dispAnchorThreaded, // autoresearch/10b (4b-1a)
	"dev_via_test_plan_rejected_to_coordinator":                   dispAnchorThreaded, // dev-via-test/02e (4b-1a)
	"dev_via_test_lisa_needs_clarification_to_coordinator":        dispAnchorInherit,  // dev-via-test/02f (first-pass — 4b-1a)
	"dev_via_test_replan_lisa_needs_clarification_to_coordinator": dispAnchorThreaded, // dev-via-test/02f-replan (4b-1a)
	"dev_via_test_cbg_rejected_to_coordinator":                    dispAnchorThreaded, // dev-via-test/07b (4b-1a)
	"dev_via_test_cbg_retry_missing_target":                       dispAnchorThreaded, // dev-via-test/07e (4b-1a)
}

type coordSpawnInfo struct {
	threadsRunLoopID bool
	stampsSuccess    bool
	allowlist        []string
}

// findCoordinatorSpawnRules walks the rule packs and returns every rule that
// spawns a `coordinator` loop, keyed by rule id, with the structural facts the
// coverage test pins. threadsRunLoopID is AND across all coordinator publish
// actions in the rule (every spawn must be anchored); allowlist is their union.
func findCoordinatorSpawnRules(t *testing.T) map[string]coordSpawnInfo {
	t.Helper()
	out := map[string]coordSpawnInfo{}
	root := "../../configs/rules"
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || !strings.HasSuffix(path, ".json") {
			return nil
		}
		raw, rerr := os.ReadFile(path) //nolint:gosec // test-controlled config path
		if rerr != nil {
			return rerr
		}
		var r ruleDoc
		if json.Unmarshal(raw, &r) != nil {
			return nil // not a rule doc — skip
		}
		var info coordSpawnInfo
		var isCoordSpawn, firstCoord bool
		firstCoord = true
		for _, a := range r.OnEnter {
			if a.Type == "publish_agent" && a.Role == "coordinator" {
				isCoordSpawn = true
				_, threaded := a.RelatedLoops["run-loop-entity-id"]
				if firstCoord {
					info.threadsRunLoopID = threaded
					firstCoord = false
				} else {
					info.threadsRunLoopID = info.threadsRunLoopID && threaded
				}
				info.allowlist = append(info.allowlist, a.ActionAllowlist...)
			}
			if a.Type == "add_triple" && a.Predicate == "agent.run.outcome" {
				if obj, _ := a.Object.(string); obj == "success" {
					info.stampsSuccess = true
				}
			}
		}
		if isCoordSpawn {
			out[r.ID] = info
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walk rules dir: %v", err)
	}
	return out
}

// TestAgentRunPack_CoordinatorSpawnCoverage is the structural boundary the
// adversarial 4a′ review demanded. It asserts EVERY rule that spawns a
// coordinator is deliberately classified for executing→failed coverage, and
// that each class holds its checkable invariant. A NEW coordinator-spawn rule
// (or a new pack) added later without a disposition entry FAILS here — which is
// exactly the silent-executing-zombie the slice exists to prevent.
func TestAgentRunPack_CoordinatorSpawnCoverage(t *testing.T) {
	found := findCoordinatorSpawnRules(t)

	// Completeness: every discovered coordinator-spawn rule must be classified.
	for id := range found {
		if _, ok := coordinatorSpawnDisposition[id]; !ok {
			t.Errorf("coordinator-spawn rule %q is UNCLASSIFIED — add it to coordinatorSpawnDisposition as "+
				"post_approval / anchor_threaded / anchor_inherit / deferred_4b (ADR-053 4a′). An unclassified "+
				"coordinator-spawn risks a silent executing-zombie when the woken coordinator fails.", id)
		}
	}
	// No stale entries.
	for id := range coordinatorSpawnDisposition {
		if _, ok := found[id]; !ok {
			t.Errorf("coordinatorSpawnDisposition lists %q but no coordinator-spawn rule with that id exists — remove the stale entry", id)
		}
	}

	// Per-disposition invariants.
	for id, info := range found {
		switch coordinatorSpawnDisposition[id] {
		case dispPostApproval:
			if !info.stampsSuccess {
				t.Errorf("%s: classified post_approval but does not stamp agent.run.outcome=success — a post_approval "+
					"coordinator-spawn must complete the run (agent-run/03) before the woken coordinator can fail", id)
			}
		case dispAnchorThreaded:
			if !info.threadsRunLoopID {
				t.Errorf("%s: classified anchor_threaded but does not thread related_loops[run-loop-entity-id] on every "+
					"coordinator spawn — the woken coordinator would carry no anchor and hang the run on failure (agent-run/06 misses)", id)
			}
		case dispAnchorInherit:
			// anchor_inherit relies on the firing role's bare agent.run
			// propagating agent.run.entity_id to the woken coordinator (→
			// agent-run/05, the length_eq 0 branch). If the rule ALSO threaded
			// run-loop-entity-id the coordinator would carry lineage and route to
			// agent-run/06 instead — so a threading anchor_inherit rule is
			// misclassified. (The other half — that the firing role actually
			// carries a bare agent.run — is a spawn-graph property the mock
			// journeys verify, not structurally checkable from one rule.)
			if info.threadsRunLoopID {
				t.Errorf("%s: classified anchor_inherit but threads run-loop-entity-id — a threaded coordinator carries "+
					"lineage and routes to agent-run/06, not the inherit path (agent-run/05). Reclassify anchor_threaded.", id)
			}
		case dispDeferred4b:
			// The deferral is principled ONLY if the woken coordinator is pure
			// human-in-the-loop delivery (run-phase semantics are 4b's). If a
			// deferred rule re-dispatches work it is no longer 4b-semantic and
			// must instead be anchor-covered.
			for _, tok := range info.allowlist {
				if tok != "respond_direct" && tok != "ask_user" {
					t.Errorf("%s: classified deferred_4b but its action_allowlist contains %q (not respond_direct/ask_user) "+
						"— it re-dispatches work, so its woken coordinator must be anchor-covered, not deferred. Thread "+
						"run-loop-entity-id and reclassify anchor_threaded.", id, tok)
				}
			}
			if info.threadsRunLoopID {
				t.Errorf("%s: classified deferred_4b but threads run-loop-entity-id — it IS anchor-covered; reclassify anchor_threaded", id)
			}
		}
	}
}

// TestDevViaTestReviewerSpawnsThreadRunAnchor pins the load-bearing precondition
// that the UNFENCED run-loop-entity-id threads on rules 02e/07b/07e rely on: every
// rule that SPAWNS a reviewer-dev-via-test (CBG) loop must thread
// related_loops[run-loop-entity-id] onto it. 02e/07b/07e fire ON the CBG and thread
// run-loop-entity-id from $entity.triple.lineage.run-loop-entity-id WITHOUT a
// length_gt 0 fence (they rely on the firing CBG always carrying it). A CBG-spawn
// that omitted the thread would make those threads resolve to a garbage literal
// (the go-reviewer C1 class) with no fence to no-op it. This makes the precondition
// structural instead of resting on review (ADR-053 4b-1a review, completeness #3).
func TestDevViaTestReviewerSpawnsThreadRunAnchor(t *testing.T) {
	root := "../../configs/rules/dev-via-test"
	entries, err := os.ReadDir(root)
	if err != nil {
		t.Fatalf("read %s: %v", root, err)
	}
	var checked int
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		r := loadRule(t, filepath.Join(root, e.Name()))
		for _, a := range r.OnEnter {
			if a.Type == "publish_agent" && a.Role == "reviewer-dev-via-test" {
				checked++
				if _, ok := a.RelatedLoops["run-loop-entity-id"]; !ok {
					t.Errorf("%s: spawns a reviewer-dev-via-test (CBG) loop but does NOT thread "+
						"related_loops[run-loop-entity-id] — rules 02e/07b/07e thread run-loop-entity-id off the CBG's "+
						"lineage WITHOUT a fence, so EVERY CBG spawn must carry it or those threads resolve to a garbage "+
						"literal (C1). Thread related_loops[run-loop-entity-id].", e.Name())
				}
			}
		}
	}
	if checked == 0 {
		t.Error("found no reviewer-dev-via-test spawns to check — did the CBG-spawn rule shape change? (test is no longer exercising the invariant)")
	}
}

// coordSpawnContract returns the (allowlist, tools) of a rule's single coordinator
// publish_agent — the user-facing contract the anchor-split siblings must keep in sync.
func coordSpawnContract(r ruleDoc) (allowlist, tools []string) {
	for _, a := range r.OnEnter {
		if a.Type == "publish_agent" && a.Role == "coordinator" {
			return a.ActionAllowlist, a.Tools
		}
	}
	return nil, nil
}

// TestAnchorSplitSiblingsKeepContractInSync pins that the first-pass/re-plan anchor
// split siblings — which the rule engine's lack of a coalesce operator forces to
// DUPLICATE the coordinator wake-up block — keep IDENTICAL action_allowlist + tools.
// The only intended differences are the lineage fence, the related_loops anchor
// source, and the prompt framing. Without this, a future edit to one sibling's
// tools/allowlist could silently diverge (the drift each rule's duplication_note
// warns of). ADR-053 4b-1a review, go-reviewer #2.
func TestAnchorSplitSiblingsKeepContractInSync(t *testing.T) {
	pairs := []struct{ first, sibling string }{
		{
			"../../configs/rules/dev-via-test/02f-lisa-needs-clarification-to-coordinator.json",
			"../../configs/rules/dev-via-test/02f-replan-lisa-needs-clarification-to-coordinator.json",
		},
		{
			"../../configs/rules/autoresearch/10-needs-clarification-replan.json",
			"../../configs/rules/autoresearch/10b-descended-needs-clarification-replan.json",
		},
	}
	for _, p := range pairs {
		fa, ft := coordSpawnContract(loadRule(t, p.first))
		sa, st := coordSpawnContract(loadRule(t, p.sibling))
		if !sameStrings(fa, sa) {
			t.Errorf("%s vs %s: coordinator action_allowlist diverged (%v vs %v) — anchor-split siblings must keep "+
				"the user-facing contract in sync (only the fence/anchor-source/prompt may differ)", p.first, p.sibling, fa, sa)
		}
		if !sameStrings(ft, st) {
			t.Errorf("%s vs %s: coordinator tools diverged (%v vs %v) — anchor-split siblings must keep tools in sync",
				p.first, p.sibling, ft, st)
		}
	}
}
