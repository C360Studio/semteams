package contract

import (
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// flowBootstrapAllowedRulePackPrefixes lists the category-pack
// directory prefixes that flow-bootstrap.json's rule-processor
// rules_files may reference. Pack-internal additions are routine
// (a new spawn rule, a new recovery handler) so a directory prefix
// is the right shape — adding a new category pack adds one entry.
//
// Future category packs (web-research/, dev-via-spec/) extend this
// list as they land.
var flowBootstrapAllowedRulePackPrefixes = []string{
	"/app/configs/rules/research/",          // ADR-042 MVP-2 research category pack.
	"/app/configs/rules/sandbox-bootstrap/", // ADR-042 §addendum 2026-05-29 sandbox-bootstrap pack.
	"/app/configs/rules/autoresearch/",      // ADR-042 §addendum 2026-05-29 autoresearch pack.
}

// flowBootstrapAllowedSharedRuleFiles lists individual cross-category
// rule files that flow-bootstrap.json's rule-processor rules_files
// may reference. File-level (not prefix-level) because the parent
// directories carry chain.mode-keyed legacy rules retiring in MVP-7;
// a directory prefix would silently permit them.
//
//   - configs/rules/coordinator/03b-respond-direct.json: shared
//     user-reply publisher. Research pack's terminal handler delegates
//     here for the actual user.response.* publish.
//   - configs/rules/coordinator/03-ask-user.json: shared clarification-
//     question publisher. Coordinator may emit ask_user during the
//     wake-up case (see research/07's action_allowlist).
//
// Legacy files explicitly NOT allowed: 01-delegate-research.json,
// 02-delegate-dev-chain.json, 04a-research-terminal-to-coordinator.json,
// 04b-qa-terminal-to-coordinator.json, 05-chain-stall-to-coordinator.json.
// All five retire alongside chain.mode in MVP-7; adding any of them
// to flow-bootstrap.json would double-fire the rule chain (research/07
// already owns the reviewer-approved → coordinator wake-up).
var flowBootstrapAllowedSharedRuleFiles = []string{
	"/app/configs/rules/coordinator/03-ask-user.json",
	"/app/configs/rules/coordinator/03b-respond-direct.json",
}

// TestFlowBootstrapShape pins the structural invariants of
// configs/flow-bootstrap.json per ADR-042 §Phase 2 redesign:
// the bootstrap config is the substrate-singleton boot config that
// task-category rule packs and named persona bundles run through.
//
// The test enforces what makes the config a *bootstrap*:
//
//   - Platform identity flags it as bootstrap (id + type).
//   - The default_role on agentic-dispatch is coordinator
//     (NOT a chain-specific role like researcher / planner).
//   - rules_files MAY be non-empty (MVP-2 picked file-based pack
//     loading), but every entry MUST live under a known
//     category-pack prefix — no chain.mode-keyed legacy packs
//     (research-mode-transition/, research-iterative/) and no
//     ad-hoc paths.
//   - inline_rules starts empty (no chain-specific inline rules
//     baked in; the chain-pause / chainstall pieces retire under
//     ADR-042 §Phase 6 anyway).
//   - The component set excludes the IoT-demo cruft (file_sensors,
//     iot_sensor) and the onboarding-only workflow-processor that
//     dev-research.json carried as accreted legacy.
//
// A future MVP-N PR that adds a chain-specific knob here should
// either (a) demonstrate it belongs in substrate, or (b) put it in
// a category rule pack / persona dir instead. The test exists to
// hold that line.
func TestFlowBootstrapShape(t *testing.T) {
	t.Parallel()

	data, err := os.ReadFile("../../configs/flow-bootstrap.json")
	require.NoError(t, err, "read bootstrap config")

	var cfg struct {
		Platform struct {
			ID   string `json:"id"`
			Type string `json:"type"`
		} `json:"platform"`
		Components map[string]struct {
			Name   string          `json:"name"`
			Config json.RawMessage `json:"config"`
		} `json:"components"`
	}
	require.NoError(t, json.Unmarshal(data, &cfg))

	// Platform identity.
	require.Equal(t, "semteams-bootstrap", cfg.Platform.ID,
		"platform.id must mark this as the bootstrap config")
	require.Equal(t, "bootstrap", cfg.Platform.Type,
		"platform.type must be 'bootstrap' (not chain-specific)")

	// Excluded components — IoT demo cruft + onboarding-only.
	for _, excluded := range []string{"file_sensors", "iot_sensor", "workflow-processor"} {
		_, present := cfg.Components[excluded]
		require.False(t, present,
			"component %q must NOT live in the bootstrap (chain-specific/legacy)", excluded)
	}

	// Required substrate — these MUST be present for the agentic
	// stack to function at all.
	for _, required := range []string{"teams-dispatch", "teams-loop", "agentic-model", "agentic-tools", "graph-ingest", "graph-query", "rule"} {
		_, present := cfg.Components[required]
		require.True(t, present,
			"substrate component %q missing from bootstrap", required)
	}

	// teams-dispatch.default_role must be coordinator. Any other
	// value implies chain-specific routing, which violates the
	// substrate-singleton design.
	dispatch, ok := cfg.Components["teams-dispatch"]
	require.True(t, ok)
	var dispatchCfg struct {
		DefaultRole string `json:"default_role"`
	}
	require.NoError(t, json.Unmarshal(dispatch.Config, &dispatchCfg))
	require.Equal(t, "coordinator", dispatchCfg.DefaultRole,
		"agentic-dispatch.default_role must be 'coordinator' — chain-specific roles belong to category rule packs")

	// rule-processor must ship with empty rules_files + inline_rules.
	// Category rule packs land in follow-up phases via the rule KV
	// or via additional rules_files entries (TBD per MVP-2 design).
	// entity_watch_patterns stays broad ("c360.>") — that's substrate
	// (the framework's pattern-driven rule firing); chain-specific
	// patterns belong inside individual rule definitions, not on the
	// processor.
	rule, ok := cfg.Components["rule"]
	require.True(t, ok)
	var ruleCfg struct {
		RulesFiles          []string          `json:"rules_files"`
		InlineRules         []json.RawMessage `json:"inline_rules"`
		EntityWatchPatterns []string          `json:"entity_watch_patterns"`
	}
	require.NoError(t, json.Unmarshal(rule.Config, &ruleCfg))
	allowedFiles := make(map[string]bool, len(flowBootstrapAllowedSharedRuleFiles))
	for _, f := range flowBootstrapAllowedSharedRuleFiles {
		allowedFiles[f] = true
	}
	for _, rf := range ruleCfg.RulesFiles {
		var matched bool
		for _, allowed := range flowBootstrapAllowedRulePackPrefixes {
			if strings.HasPrefix(rf, allowed) {
				matched = true
				break
			}
		}
		if !matched && allowedFiles[rf] {
			matched = true
		}
		require.True(t, matched,
			"rule-processor.rules_files entry %q does not match an allowed category-pack prefix %v nor a shared-rule allowlist entry %v — chain.mode-keyed legacy packs (research-mode-transition/, research-iterative/, dev-via-spec/) and chain.mode-keyed coordinator rules (01-delegate-research, 02-delegate-dev-chain, 04a/b, 05-chain-stall) belong in legacy configs, not the bootstrap",
			rf, flowBootstrapAllowedRulePackPrefixes, flowBootstrapAllowedSharedRuleFiles)
	}
	require.Empty(t, ruleCfg.InlineRules,
		"rule-processor.inline_rules must be empty — no chain-specific rules baked into substrate")
	require.Equal(t, []string{"c360.>"}, ruleCfg.EntityWatchPatterns,
		"rule-processor.entity_watch_patterns must stay broad — chain-specific patterns belong inside individual rule definitions")

	// The coordinator persona dir MUST exist on disk. agentic-dispatch's
	// default_role of "coordinator" is load-bearing — if the persona
	// dir is missing, every initial task spawns into an empty prompt.
	personaDir := "../../configs/personas/fragments/coordinator"
	info, err := os.Stat(personaDir)
	require.NoError(t, err, "coordinator persona dir must exist: %s", personaDir)
	require.True(t, info.IsDir(), "coordinator persona path must be a directory: %s", personaDir)
}
