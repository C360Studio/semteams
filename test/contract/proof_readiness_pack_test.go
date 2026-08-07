//go:build parked_packs

// The pack this file pins is parked (unwired from flow-bootstrap) pending the
// canonical-predicate migration — see ADR-058. Drop this build tag when the
// pack is re-authored and re-wired.

package contract

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const proofReadinessPackDir = "../../configs/rules/proof-readiness"

type proofReadinessRuleJSON struct {
	ID         string                        `json:"id"`
	Type       string                        `json:"type"`
	Enabled    bool                          `json:"enabled"`
	Conditions []proofReadinessConditionJSON `json:"conditions"`
	OnEnter    []proofReadinessOnEnterJSON   `json:"on_enter"`
	Metadata   map[string]any                `json:"metadata"`
}

type proofReadinessConditionJSON struct {
	Field    string `json:"field"`
	Operator string `json:"operator"`
	Value    any    `json:"value"`
	Required bool   `json:"required"`
}

type proofReadinessOnEnterJSON struct {
	Type      string `json:"type"`
	Subject   string `json:"subject"`
	Predicate string `json:"predicate"`
	Object    any    `json:"object"`
	Role      string `json:"role,omitempty"`
}

func TestProofReadinessPack_RoutesAreExactGraphPredicates(t *testing.T) {
	files, err := filepath.Glob(filepath.Join(proofReadinessPackDir, "*.json"))
	if err != nil {
		t.Fatalf("glob proof-readiness pack: %v", err)
	}
	if len(files) != 4 {
		t.Fatalf("proof-readiness rule count = %d, want 4", len(files))
	}
	for _, path := range files {
		rule := loadProofReadinessRule(t, filepath.Base(path))
		if rule.Type != "expression" || !rule.Enabled {
			t.Errorf("%s type/enabled = %q/%v, want expression/true", filepath.Base(path), rule.Type, rule.Enabled)
		}
		for _, cond := range rule.Conditions {
			if strings.Contains(cond.Field, "formal_claims.finding.") {
				t.Errorf("%s condition uses finding-family predicate %q; rules must consume formal_claims.route.* summaries",
					filepath.Base(path), cond.Field)
			}
			if strings.Contains(cond.Field, "*") {
				t.Errorf("%s condition uses wildcard predicate %q; rule engine matches literal graph predicates",
					filepath.Base(path), cond.Field)
			}
		}
		for _, action := range rule.OnEnter {
			if action.Type != "add_triple" {
				t.Errorf("%s action type = %q, want add_triple only (no raw NATS publish or pretend agent)",
					filepath.Base(path), action.Type)
			}
			if action.Subject != "$entity.id" {
				t.Errorf("%s add_triple subject = %q, want $entity.id run entity", filepath.Base(path), action.Subject)
			}
		}
	}
}

func TestProofReadinessPack_PassedMarksImplementationReady(t *testing.T) {
	rule := loadProofReadinessRule(t, "01-passed-to-implementation-ready.json")
	assertPRCondition(t, rule, "formal_claims.status", "eq", "passed")
	assertPRCondition(t, rule, "formal_claims.route.implementation", "eq", "present")
	assertPRTriple(t, rule, "proof_readiness.route", "implementation")
	assertPRTriple(t, rule, "proof_readiness.implementation_ready", "true")
	assertPRTriple(t, rule, "proof_readiness.routed", "true")
}

func TestProofReadinessPack_TestHarnessHandoff(t *testing.T) {
	rule := loadProofReadinessRule(t, "02-test-harness-handoff.json")
	assertPRCondition(t, rule, "formal_claims.status", "eq", "failed")
	assertPRCondition(t, rule, "formal_claims.route.test_harness", "eq", "present")
	assertPRTriple(t, rule, "proof_readiness.route", "test_harness")
	assertPRTriple(t, rule, "proof_readiness.test_harness_required", "true")
	assertPRTriple(t, rule, "proof_readiness.routed", "true")
}

func TestProofReadinessPack_AmbiguousReturnsToCoordinator(t *testing.T) {
	rule := loadProofReadinessRule(t, "03-ambiguous-to-coordinator.json")
	assertPRCondition(t, rule, "formal_claims.status", "eq", "ambiguous")
	assertPRCondition(t, rule, "formal_claims.route.coordinator", "eq", "present")
	assertPRTriple(t, rule, "proof_readiness.route", "coordinator")
	assertPRTriple(t, rule, "proof_readiness.requires_clarification", "true")
	assertPRTriple(t, rule, "proof_readiness.routed", "true")
}

func TestProofReadinessPack_FailedNoRoutePauses(t *testing.T) {
	rule := loadProofReadinessRule(t, "04-failed-no-route-pause.json")
	assertPRCondition(t, rule, "formal_claims.status", "eq", "failed")
	assertPRCondition(t, rule, "formal_claims.route.test_harness", "length_eq", float64(0))
	assertPRCondition(t, rule, "formal_claims.route.coordinator", "length_eq", float64(0))
	assertPRCondition(t, rule, "formal_claims.route.implementation", "length_eq", float64(0))
	assertPRTriple(t, rule, "proof_readiness.route", "pause")
	assertPRTriple(t, rule, "proof_readiness.pause_required", "true")
	assertPRTriple(t, rule, "proof_readiness.routed", "true")
}

func TestProofReadinessPackWiredInBothConfigs(t *testing.T) {
	files, err := filepath.Glob(filepath.Join(proofReadinessPackDir, "*.json"))
	if err != nil {
		t.Fatalf("glob proof-readiness pack: %v", err)
	}
	for _, cfg := range []string{"../../configs/flow-bootstrap.json", "../../configs/e2e-flow-bootstrap.json"} {
		data, err := os.ReadFile(cfg)
		if err != nil {
			t.Fatalf("read %s: %v", cfg, err)
		}
		body := string(data)
		for _, path := range files {
			want := "/app/configs/rules/proof-readiness/" + filepath.Base(path)
			if !strings.Contains(body, want) {
				t.Errorf("%s rules_files missing %q — proof-readiness route never loads at boot",
					filepath.Base(cfg), want)
			}
		}
	}
}

func loadProofReadinessRule(t *testing.T, name string) *proofReadinessRuleJSON {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(proofReadinessPackDir, name))
	if err != nil {
		t.Fatalf("read %s: %v", name, err)
	}
	var rule proofReadinessRuleJSON
	if err := json.Unmarshal(data, &rule); err != nil {
		t.Fatalf("unmarshal %s: %v", name, err)
	}
	return &rule
}

func assertPRCondition(t *testing.T, rule *proofReadinessRuleJSON, field, op string, value any) {
	t.Helper()
	for _, cond := range rule.Conditions {
		if cond.Field == field && cond.Operator == op && cond.Value == value && cond.Required {
			return
		}
	}
	t.Fatalf("%s missing required condition %s %s %v; conditions=%+v", rule.ID, field, op, value, rule.Conditions)
}

func assertPRTriple(t *testing.T, rule *proofReadinessRuleJSON, predicate, object string) {
	t.Helper()
	for _, action := range rule.OnEnter {
		obj, _ := action.Object.(string)
		if action.Type == "add_triple" && action.Predicate == predicate && obj == object {
			return
		}
	}
	t.Fatalf("%s missing add_triple %s=%s; actions=%+v", rule.ID, predicate, object, rule.OnEnter)
}
