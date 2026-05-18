package contract

import (
	"encoding/json"
	"os"
	"testing"

	"github.com/stretchr/testify/require"
)

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
//   - rules_files starts empty (categories load their packs at
//     runtime via hot-reload or are added in follow-up phases).
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
	require.Empty(t, ruleCfg.RulesFiles,
		"rule-processor.rules_files must start empty in the bootstrap — categories load their packs")
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
