package contract

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Sandbox-bootstrap pack contract tests — structural invariants the
// rule JSON must hold for the pack to compose correctly with the
// substrate AND with the autoresearch pack downstream. Walks
// configs/rules/sandbox-bootstrap/ at test time; no runtime rule
// engine needed.
//
// Per ADR-042 §addendum 2026-05-29: the sandbox-bootstrap pack is the
// first to exercise the multi-category coordinator orchestration
// pattern (chained wake-up allowlist) and the first to depend on
// cross-rule lineage propagation for chain entity stamping.

const sandboxBootstrapPackDir = "../../configs/rules/sandbox-bootstrap"

// sandboxBootstrapRuleJSON mirrors autoresearchRuleJSON's shape with
// publish_agent properties + related_loops so we can assert lineage
// propagation through the chain.
type sandboxBootstrapRuleJSON struct {
	ID         string                        `json:"id"`
	Type       string                        `json:"type"`
	Enabled    bool                          `json:"enabled"`
	Conditions []autoresearchConditionJSON   `json:"conditions"`
	OnEnter    []sandboxBootstrapOnEnterJSON `json:"on_enter"`
	Metadata   map[string]any                `json:"metadata"`
}

type sandboxBootstrapOnEnterJSON struct {
	Type          string            `json:"type"`
	Subject       string            `json:"subject,omitempty"`
	Predicate     string            `json:"predicate,omitempty"`
	Object        string            `json:"object,omitempty"`
	Role          string            `json:"role,omitempty"`
	ActionAllowed []string          `json:"action_allowlist,omitempty"`
	Properties    map[string]string `json:"properties,omitempty"`
	RelatedLoops  map[string]string `json:"related_loops,omitempty"`
	Tools         []string          `json:"tools,omitempty"`
}

// TestSandboxBootstrapPack_LineageThreading pins the M2 fix from the
// design review (2026-05-29): rule 07's `chain.arc.bootstrap.tenant_signature`
// stamp substitutes `$entity.triple.sandbox.tenant.signature` from
// reviewer-bootstrap's loop entity. For that to resolve to a real
// value, the signature triple must propagate through every loop in
// the bootstrap arc — plan stamps it, then plan → execute → verify
// → reviewer related_loops chains must carry it forward.
//
// What "carrying it forward" means structurally: each downstream
// rule's publish_agent.related_loops must include a key referencing
// the SAME lineage that subsequent rules read from. The autoresearch
// pack uses `run-loop-entity-id`; the sandbox-bootstrap pack uses
// `run-loop-entity-id` for the same purpose. This test verifies the
// chain.
//
// Without lineage propagation, rule 07's add_triple substitutes to
// empty and chain.arc.bootstrap.tenant_signature becomes blank,
// breaking chain-arc audit + breaking the downstream autoresearch
// arc's ability to read the tenant_signature from chain state.
func TestSandboxBootstrapPack_LineageThreading(t *testing.T) {
	// Rules that spawn a downstream loop must propagate the
	// bootstrap-run + run-loop-entity-id lineage keys forward.
	// Spawn rules in the bootstrap pack:
	rulesWithSpawns := []string{
		"01-coordinator-bootstrap-spawn.json",      // coordinator → plan
		"02a-plan-skip-to-verify.json",             // plan → verify (skip)
		"02b-plan-to-execute.json",                 // plan → execute
		"03-execute-to-verify.json",                // execute → verify
		"04-verify-to-reviewer.json",               // verify → reviewer
		"07-reviewer-approved-to-coordinator.json", // reviewer → coordinator wake-up
	}

	for _, name := range rulesWithSpawns {
		t.Run(name, func(t *testing.T) {
			rule, err := loadSandboxBootstrapRule(name)
			if err != nil {
				t.Fatalf("load %s: %v", name, err)
			}

			pa := findPublishAgent(rule)
			if pa == nil {
				t.Fatalf("%s has no publish_agent action", name)
			}

			// Rule 01 originates the lineage (spawns from coordinator
			// loop = run entity). Its related_loops establishes the
			// keys: bootstrap-run + run-loop-entity-id.
			if name == "01-coordinator-bootstrap-spawn.json" {
				assertRelatedLoopsKey(t, pa, "bootstrap-run", "$entity.instance",
					"rule 01 must establish bootstrap-run lineage from coordinator $entity.instance")
				assertRelatedLoopsKey(t, pa, "run-loop-entity-id", "$entity.id",
					"rule 01 must establish run-loop-entity-id lineage from coordinator $entity.id")
				return
			}

			// All other spawn rules must PROPAGATE the lineage forward.
			// They reference the prior loop's lineage triples via
			// $entity.triple.lineage.<key>.
			assertRelatedLoopsKey(t, pa, "bootstrap-run", "$entity.triple.lineage.bootstrap-run",
				name+" must propagate bootstrap-run lineage from prior loop")
			assertRelatedLoopsKey(t, pa, "run-loop-entity-id", "$entity.triple.lineage.run-loop-entity-id",
				name+" must propagate run-loop-entity-id lineage from prior loop — needed so rule 07's chain.arc.bootstrap.tenant_signature substitution resolves; needed so rule 04 (execute-stamp-completion equivalent) can target the run entity via subject override")
		})
	}
}

// TestSandboxBootstrapPack_07_ChainedAllowlist pins ADR-042
// §addendum §C: rule 07 is the canonical reference for chained
// wake-up routing. The action_allowlist must include the
// downstream-category tokens (autoresearch, research) AND the user-
// facing tokens (respond_direct, ask_user) BUT NOT the originating
// action (bootstrap_sandbox) — loop protection per §C.
func TestSandboxBootstrapPack_07_ChainedAllowlist(t *testing.T) {
	rule, err := loadSandboxBootstrapRule("07-reviewer-approved-to-coordinator.json")
	if err != nil {
		t.Fatalf("load rule 07: %v", err)
	}
	pa := findPublishAgent(rule)
	if pa == nil {
		t.Fatal("rule 07 has no publish_agent action")
	}

	required := []string{"respond_direct", "ask_user", "autoresearch", "research"}
	for _, want := range required {
		if !contains(pa.ActionAllowed, want) {
			t.Errorf("rule 07 action_allowlist missing %q — required for chained wake-up per ADR-042 §addendum §C", want)
		}
	}

	// Loop protection: bootstrap_sandbox must NOT appear.
	if contains(pa.ActionAllowed, "bootstrap_sandbox") {
		t.Error("rule 07 action_allowlist includes bootstrap_sandbox — must be excluded for loop protection (prevents bootstrap → wake-up → bootstrap cycles). See ADR-042 §addendum §C 'Loop protection'.")
	}
}

// TestSandboxBootstrapPack_07_ChainArcStamps pins the §addendum §C
// 'Multi-step state threading' contract: rule 07 stamps two triples
// on the run entity capturing the bootstrap arc's category +
// tenant_signature, so the chain entity carries multi-arc audit
// state for UI rendering + downstream-arc context.
func TestSandboxBootstrapPack_07_ChainArcStamps(t *testing.T) {
	rule, err := loadSandboxBootstrapRule("07-reviewer-approved-to-coordinator.json")
	if err != nil {
		t.Fatalf("load rule 07: %v", err)
	}

	wantStamps := map[string]string{
		"chain.arc.bootstrap.category":         "sandbox-bootstrap",
		"chain.arc.bootstrap.tenant_signature": "$entity.triple.sandbox.tenant.signature",
	}

	for pred, wantObj := range wantStamps {
		found := false
		for _, a := range rule.OnEnter {
			if a.Type != "add_triple" || a.Predicate != pred {
				continue
			}
			if !strings.Contains(a.Subject, "lineage.bootstrap-run") {
				t.Errorf("rule 07 stamps %q but subject %q is not on the run entity (expected lineage.bootstrap-run)", pred, a.Subject)
				continue
			}
			if a.Object != wantObj {
				t.Errorf("rule 07 stamps %q with object %q; want %q", pred, a.Object, wantObj)
			}
			found = true
		}
		if !found {
			t.Errorf("rule 07 missing chain-arc stamp for predicate %q — ADR-042 §addendum §C multi-step state threading", pred)
		}
	}
}

// --- helpers ---

func loadSandboxBootstrapRule(name string) (*sandboxBootstrapRuleJSON, error) {
	path := filepath.Join(sandboxBootstrapPackDir, name)
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", path, err)
	}
	var r sandboxBootstrapRuleJSON
	if err := json.Unmarshal(data, &r); err != nil {
		return nil, fmt.Errorf("unmarshal %s: %w", path, err)
	}
	return &r, nil
}

func findPublishAgent(r *sandboxBootstrapRuleJSON) *sandboxBootstrapOnEnterJSON {
	for i := range r.OnEnter {
		if r.OnEnter[i].Type == "publish_agent" {
			return &r.OnEnter[i]
		}
	}
	return nil
}

func assertRelatedLoopsKey(t *testing.T, pa *sandboxBootstrapOnEnterJSON, key, wantValue, msg string) {
	t.Helper()
	got, ok := pa.RelatedLoops[key]
	if !ok {
		t.Errorf("%s — related_loops missing key %q", msg, key)
		return
	}
	if got != wantValue {
		t.Errorf("%s — related_loops[%q] = %q; want %q", msg, key, got, wantValue)
	}
}

func contains(slice []string, want string) bool {
	for _, s := range slice {
		if s == want {
			return true
		}
	}
	return false
}
