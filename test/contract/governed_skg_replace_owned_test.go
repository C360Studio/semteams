package contract

import (
	"encoding/json"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// Governed-SKG replace_owned migration (ADR-056 #278 / semstreams beta.110).
//
// These pins guard the rule-pack projection-ownership adoption: the substrate
// rule processor declares pack_id "semteams" + a replace-owned projection
// contract, and the autoresearch single-valued scalar-replace rules
// (best.value / best.experiment_id / run.status) write via the owned
// `replace_owned` lane whose predicate set falls INSIDE that contract.
//
// Why this is a structural gate and not a comment: the framework's rule loader
// (processor/rule/rule_loader.go validateRuleReplaceOwnedActions) HARD-FAILS
// boot when a replace_owned action names a predicate outside the pack's declared
// ModeReplaceOwned groups — a misconfigured envelope is a boot-abort, not a
// degraded run. This test reproduces that envelope check over the committed
// config so the drift surfaces in CI (and in a code review) rather than at
// container start. It also reproduces the "predicate must be a literal" rule:
// a `$`-bearing replace_owned predicate is rejected at load.
//
// Owner-claim disjointness (rule-pack.semteams's autoresearch.* predicates vs
// the lifecycle agent-run owner's agent.run.* predicates on the SAME
// *.*.agent.chain.execution.* entity pattern) is verified live at boot — the
// multi-owner-by-predicate-group pattern is observe-only on this tag (an overlap
// would WARN, not brick). It is not re-derived here; this test scopes to the
// boot-fatal envelope.

// ownershipFlowConfig is the minimal slice of a flow config needed to read the
// rule processor's pack_id + projection contracts.
type ownershipFlowConfig struct {
	Components map[string]struct {
		Name   string          `json:"name"`
		Config json.RawMessage `json:"config"`
	} `json:"components"`
}

type ruleProcessorOwnershipConfig struct {
	PackID              string `json:"pack_id"`
	ProjectionContracts []struct {
		Name          string `json:"name"`
		EntityPattern string `json:"entity_pattern"`
		Groups        []struct {
			Mode       string   `json:"mode"`
			Predicates []string `json:"predicates"`
		} `json:"groups"`
	} `json:"projection_contracts"`
}

// ownershipRuleJSON captures EVERY action list a replace_owned action can appear
// in, matching the framework's validateRuleReplaceOwnedActions (semstreams
// processor/rule/processor.go), which validates all five lists uniformly:
// on_enter, on_exit, while_true, on_recovery (expression rules) + actions (cron
// rules). Missing while_true/on_recovery here would let a replace_owned action in
// those blocks slip past CI and hard-fail boot in production — the exact failure
// this test exists to prevent.
type ownershipRuleJSON struct {
	ID         string                `json:"id"`
	Type       string                `json:"type"`
	OnEnter    []ownershipActionJSON `json:"on_enter"`
	OnExit     []ownershipActionJSON `json:"on_exit"`
	WhileTrue  []ownershipActionJSON `json:"while_true"`
	OnRecovery []ownershipActionJSON `json:"on_recovery"`
	Actions    []ownershipActionJSON `json:"actions"`
}

type ownershipActionJSON struct {
	ID        string `json:"id"`
	Type      string `json:"type"`
	Predicate string `json:"predicate"`
}

func (r ownershipRuleJSON) allActions() []ownershipActionJSON {
	out := make([]ownershipActionJSON, 0,
		len(r.OnEnter)+len(r.OnExit)+len(r.WhileTrue)+len(r.OnRecovery)+len(r.Actions))
	out = append(out, r.OnEnter...)
	out = append(out, r.OnExit...)
	out = append(out, r.WhileTrue...)
	out = append(out, r.OnRecovery...)
	out = append(out, r.Actions...)
	return out
}

const (
	flowBootstrapPath    = "../../configs/flow-bootstrap.json"
	e2eFlowBootstrapPath = "../../configs/e2e-flow-bootstrap.json"
	rulesRootDir         = "../../configs/rules"
	expectedPackID       = "semteams"
	runAnchorPattern     = "*.*.agent.chain.execution.*"
)

// loadRuleProcessorOwnership reads the named flow config and returns the rule
// processor component's ownership config.
func loadRuleProcessorOwnership(t *testing.T, flowPath string) ruleProcessorOwnershipConfig {
	t.Helper()
	raw, err := os.ReadFile(flowPath) //nolint:gosec // test-controlled config path
	require.NoError(t, err, "read flow config %s", flowPath)

	var flow ownershipFlowConfig
	require.NoError(t, json.Unmarshal(raw, &flow), "unmarshal flow config %s", flowPath)

	comp, ok := flow.Components["rule"]
	require.Truef(t, ok, "%s: missing 'rule' component", flowPath)
	require.Equalf(t, "rule-processor", comp.Name, "%s: 'rule' component is not the rule-processor factory", flowPath)

	var cfg ruleProcessorOwnershipConfig
	require.NoError(t, json.Unmarshal(comp.Config, &cfg), "unmarshal rule processor config %s", flowPath)
	return cfg
}

// declaredReplaceOwnedPredicates returns the union of every ModeReplaceOwned
// group's predicates across the processor's contracts — the boot-time envelope.
func declaredReplaceOwnedPredicates(cfg ruleProcessorOwnershipConfig) map[string]struct{} {
	set := map[string]struct{}{}
	for _, c := range cfg.ProjectionContracts {
		for _, g := range c.Groups {
			if g.Mode != "replace-owned" {
				continue
			}
			for _, p := range g.Predicates {
				set[p] = struct{}{}
			}
		}
	}
	return set
}

// TestReplaceOwned_FlowConfigsDeclarePackContract pins that BOTH the production
// and e2e flow configs declare the rule-pack projection ownership identically:
// pack_id "semteams" + an autoresearch replace-owned contract on the run-anchor
// entity pattern carrying the three migrated single-valued predicates. The e2e
// config is a mock-LLM clone of production (CLAUDE.md), so the ownership shape
// must match or the mock journey would validate a different envelope than ships.
func TestReplaceOwned_FlowConfigsDeclarePackContract(t *testing.T) {
	wantPredicates := []string{
		// PR #219 — autoresearch single-valued scalar replaces.
		"autoresearch.best.value",
		"autoresearch.best.experiment_id",
		"autoresearch.run.status",
		// Fast-follow — autoresearch iteration presence marker.
		"autoresearch.iteration.pending",
		// Fast-follow — dev-via-test retry/iteration coordination gates.
		"dev_via_test.plan.retry.pending",
		"dev_via_test.plan.retry.finding",
		"dev_via_test.cbg.retry.pending",
		"dev_via_test.cbg.retry.finding",
		"dev_via_test.cbg.retry.target_task",
	}

	for _, flowPath := range []string{flowBootstrapPath, e2eFlowBootstrapPath} {
		cfg := loadRuleProcessorOwnership(t, flowPath)

		require.Equalf(t, expectedPackID, cfg.PackID,
			"%s: rule processor must declare pack_id %q so replace_owned actions resolve owner rule-pack.%s",
			flowPath, expectedPackID, expectedPackID)

		// Find the autoresearch contract.
		var found bool
		declared := declaredReplaceOwnedPredicates(cfg)
		for _, c := range cfg.ProjectionContracts {
			if c.EntityPattern == runAnchorPattern {
				found = true
			}
		}
		require.Truef(t, found,
			"%s: expected a projection contract on the run-anchor pattern %q (where autoresearch.best.* + run.status are stamped)",
			flowPath, runAnchorPattern)

		for _, p := range wantPredicates {
			_, ok := declared[p]
			require.Truef(t, ok,
				"%s: predicate %q must be in a replace-owned projection-contract group; "+
					"otherwise its replace_owned rule action hard-fails boot", flowPath, p)
		}
	}
}

// TestReplaceOwned_RuleActionsWithinDeclaredEnvelope reproduces the framework's
// boot-time envelope check across every committed rule file: every replace_owned
// action's predicate MUST be a literal (no `$` substitution) AND fall inside the
// production config's declared replace-owned predicate set. A failure here is a
// boot-abort in production — catch it in CI.
func TestReplaceOwned_RuleActionsWithinDeclaredEnvelope(t *testing.T) {
	declared := declaredReplaceOwnedPredicates(loadRuleProcessorOwnership(t, flowBootstrapPath))
	require.NotEmpty(t, declared, "production config declares no replace-owned predicates — envelope check is vacuous")

	var scanned int
	err := filepath.WalkDir(rulesRootDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || !strings.HasSuffix(path, ".json") {
			return nil
		}
		raw, readErr := os.ReadFile(path) //nolint:gosec // test-controlled config path
		require.NoError(t, readErr, "read rule file %s", path)

		var rule ownershipRuleJSON
		require.NoErrorf(t, json.Unmarshal(raw, &rule), "unmarshal rule file %s", path)

		for _, a := range rule.allActions() {
			if a.Type != "replace_owned" {
				continue
			}
			scanned++
			require.Falsef(t, strings.Contains(a.Predicate, "$"),
				"%s action %q: replace_owned predicate %q must be a literal (no $ substitution) — the loader rejects $-bearing predicates",
				path, a.ID, a.Predicate)
			_, ok := declared[a.Predicate]
			require.Truef(t, ok,
				"%s action %q: replace_owned predicate %q is OUTSIDE the declared replace-owned envelope %v — "+
					"this hard-fails boot. Add it to a projection_contract replace-owned group in flow-bootstrap.json "+
					"(and e2e-flow-bootstrap.json), or use add_triple/update_triple.",
				path, a.ID, a.Predicate, keysOf(declared))
		}
		return nil
	})
	require.NoError(t, err, "walk rules root")
	require.Positivef(t, scanned, "no replace_owned actions found under %s — the migration regressed to update_triple/add_triple", rulesRootDir)
}

// TestReplaceOwned_MigratedRulesUseOwnedLane is the regression pin against a
// silent revert: the three autoresearch scalar-replace predicates MUST be
// written by a replace_owned action (not the non-atomic update_triple they used
// 2026-06-03..2026-06-14, nor an appending add_triple). Keyed by (file,
// predicate) so a reorder of actions within a rule does not flake the test.
func TestReplaceOwned_MigratedRulesUseOwnedLane(t *testing.T) {
	type target struct {
		file      string
		predicate string
	}
	targets := []target{
		// PR #219 — autoresearch single-valued scalar replaces.
		{"../../configs/rules/autoresearch/04c-execute-promote-best-on-kept.json", "autoresearch.best.value"},
		{"../../configs/rules/autoresearch/04c-execute-promote-best-on-kept.json", "autoresearch.best.experiment_id"},
		{"../../configs/rules/autoresearch/05-iteration-dispatch.json", "autoresearch.run.status"},
		// Fast-follow — autoresearch iteration presence marker (set 04a/04b, clear 05).
		{"../../configs/rules/autoresearch/04a-execute-stamp-completion.json", "autoresearch.iteration.pending"},
		{"../../configs/rules/autoresearch/04b-execute-stamp-failed.json", "autoresearch.iteration.pending"},
		{"../../configs/rules/autoresearch/05-iteration-dispatch.json", "autoresearch.iteration.pending"},
		// Fast-follow — dev-via-test retry coordination gates.
		{"../../configs/rules/dev-via-test/02c-plan-retry-stamp.json", "dev_via_test.plan.retry.finding"},
		{"../../configs/rules/dev-via-test/02c-plan-retry-stamp.json", "dev_via_test.plan.retry.pending"},
		{"../../configs/rules/dev-via-test/07c-cbg-retry-stamp.json", "dev_via_test.cbg.retry.target_task"},
		{"../../configs/rules/dev-via-test/07c-cbg-retry-stamp.json", "dev_via_test.cbg.retry.finding"},
		{"../../configs/rules/dev-via-test/07c-cbg-retry-stamp.json", "dev_via_test.cbg.retry.pending"},
	}

	for _, tg := range targets {
		raw, err := os.ReadFile(tg.file) //nolint:gosec // test-controlled config path
		require.NoError(t, err, "read %s", tg.file)
		var rule ownershipRuleJSON
		require.NoError(t, json.Unmarshal(raw, &rule), "unmarshal %s", tg.file)

		// Assert EVERY action touching this predicate is on the owned lane —
		// not just the last one — so a future mixed-lane write (one
		// replace_owned + one stray add_triple/remove_triple on the same
		// predicate in the same file) can't slip past on last-write-wins.
		var lanes []string
		for _, a := range rule.allActions() {
			if a.Predicate == tg.predicate {
				lanes = append(lanes, a.Type)
			}
		}
		require.NotEmptyf(t, lanes, "%s: predicate %q has no triple action — migration regressed", tg.file, tg.predicate)
		for _, lane := range lanes {
			require.Equalf(t, "replace_owned", lane,
				"%s: predicate %q must be written ONLY by replace_owned actions (atomic owned replace, ADR-056 Decision 3), got %q among %v",
				tg.file, tg.predicate, lane, lanes)
		}
	}
}

func keysOf(m map[string]struct{}) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}
