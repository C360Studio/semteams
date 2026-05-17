package contract

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestConfigDispatchPermissions verifies that every config file in configs/
// that includes an agentic-dispatch component has permissions that allow
// task submission. This catches the bug where adding a dispatch config
// without a permissions block silently blocks all message submission
// (Go zero-value for []string is nil → empty list → nobody can submit).
func TestConfigDispatchPermissions(t *testing.T) {
	configs, err := filepath.Glob("../../configs/*.json")
	require.NoError(t, err, "failed to glob configs")
	require.NotEmpty(t, configs, "no config files found — wrong working directory?")

	for _, cfgPath := range configs {
		name := filepath.Base(cfgPath)
		t.Run(name, func(t *testing.T) {
			data, err := os.ReadFile(cfgPath)
			require.NoError(t, err)

			var config struct {
				Components map[string]struct {
					Config json.RawMessage `json:"config"`
				} `json:"components"`
			}
			require.NoError(t, json.Unmarshal(data, &config))

			dispatch, hasDispatch := config.Components["teams-dispatch"]
			if !hasDispatch {
				t.Skipf("%s has no explicit agentic-dispatch component (uses defaults — OK)", name)
				return
			}

			// Parse the dispatch config
			var dispatchConfig struct {
				DefaultRole string `json:"default_role"`
				Permissions struct {
					SubmitTask []string `json:"submit_task"`
					View       []string `json:"view"`
					Approve    []string `json:"approve"`
				} `json:"permissions"`
			}
			require.NoError(t, json.Unmarshal(dispatch.Config, &dispatchConfig),
				"failed to parse dispatch config in %s", name)

			// submit_task must be non-empty — otherwise nobody can submit messages
			assert.NotEmpty(t, dispatchConfig.Permissions.SubmitTask,
				"%s: agentic-dispatch.permissions.submit_task is empty — "+
					"this blocks ALL message submission. Add [\"*\"] to allow all users.", name)

			// default_role must be set
			assert.NotEmpty(t, dispatchConfig.DefaultRole,
				"%s: agentic-dispatch.default_role is empty", name)
		})
	}
}

// TestConfigDispatchDefaultToolsParse is the F4b regression guard.
// Smoke #7 (R3.7.2.l′, 2026-05-04) showed Loop A — the dispatch
// researcher loop — had access to ~16 tools when osh-demo.json's
// default_tools listed only 4. The dispatch source path
// (agentic-dispatch/component.go:710-720) DOES gate task.Tools on
// c.config.DefaultTools != nil, and the loop source path
// (agentic-loop/handlers.go:584-595) DOES respect non-nil task.Tools.
// So the runtime gap is between osh-demo.json on disk and
// c.config.DefaultTools at runtime — the most likely culprit is
// JSON unmarshal silently dropping the field (e.g., a future schema
// rename, json tag drift, or Config-struct refactor).
//
// This test pins the field name on the wire AND the post-parse
// struct shape, with a per-config-file expectation table. A failure
// pinpoints exactly which config drifted.
//
// Per-config expectations: the file lists the wire-shape default_tools
// the operator authored. Each entry is independently validated. A
// future config that legitimately uses default_tools=[] (empty
// allowlist — "no tools for the initial researcher") declares
// nilOrEmpty:true; a config without the field at all declares
// nilOrEmpty:true with absent:true.
func TestConfigDispatchDefaultToolsParse(t *testing.T) {
	expectations := map[string]struct {
		// absent: the config has no default_tools key at all (legitimate
		// for flows whose initial role uses global discovery).
		absent bool
		// expected: when absent=false, the exact tool-name list the
		// config author put in the file (order-sensitive — we want the
		// JSON wire to match the source-of-truth verbatim).
		expected []string
	}{
		// coordinator-redesign Slice 1 (2026-05-15): osh-demo dispatch
		// flipped from researcher-plan to coordinator entry. The seeded
		// tools shrink to the coordinator's minimum (decide + read).
		// submit_work intentionally omitted — the coordinator persona
		// makes `decide` the single terminal, and offering submit_work
		// as a fallback contradicts that contract and risks LLM drift
		// into a terminal that fires no rules. researcher-plan is now
		// spawned by the coordinator rules
		// (configs/rules/coordinator/01-02-*) with its own tool list
		// set on the publish_agent action, so emit_plan no longer
		// belongs in dispatch defaults.
		"osh-demo.json": {
			expected: []string{"decide", "read_loop_result"},
		},
		// ADR-041 MVP: dispatch enters at researcher-plan, so emit_plan
		// (researcher-plan's owned emit tool per
		// TestADR041_EmitToolPhaseOwnership) is the seeded default —
		// pre-rewrite seeded emit_research_artifact which belongs to
		// the researcher-synthesize phase. Rule_04 spawns gather
		// downstream and inherits its own tool list from the rule's
		// `tools` field.
		"e2e-dev-via-spec.json": {
			expected: []string{"read_loop_result", "query_entity", "query_entities", "emit_plan"},
		},
		"e2e-research-mode-transition.json": {
			expected: []string{"read_loop_result", "query_entity", "query_entities", "emit_plan"},
		},
		"e2e-research-iterative.json": {
			expected: []string{"read_loop_result", "query_entity", "query_entities"},
		},
		"e2e-research-with-source.json": {
			expected: []string{"add_source_repo"},
		},
		"e2e-research-harness-hit.json": {
			expected: []string{"read_loop_result", "query_entity", "query_entities", "emit_research_artifact"},
		},
		"e2e-coordinator.json": {
			expected: []string{"decide", "read_loop_result", "submit_work"},
		},
		// Empty arrays — explicit "no tools for initial role." The wire
		// distinguishes nil (field absent) from [] (explicit empty).
		"e2e-agentic.json":      {expected: []string{}},
		"e2e-dev-research.json": {expected: []string{}},
	}

	configs, err := filepath.Glob("../../configs/*.json")
	require.NoError(t, err)

	for _, cfgPath := range configs {
		name := filepath.Base(cfgPath)
		exp, hasExpectation := expectations[name]
		if !hasExpectation {
			continue // configs without explicit expectations are out of scope
		}

		t.Run(name, func(t *testing.T) {
			data, err := os.ReadFile(cfgPath)
			require.NoError(t, err)

			var config struct {
				Components map[string]struct {
					Config json.RawMessage `json:"config"`
				} `json:"components"`
			}
			require.NoError(t, json.Unmarshal(data, &config))

			dispatch, ok := config.Components["teams-dispatch"]
			require.True(t, ok, "%s: teams-dispatch component missing", name)

			// Parse with a struct shape that mirrors agentic-dispatch's
			// Config — same json tag we expect the framework to read.
			var dispatchConfig struct {
				DefaultTools []string `json:"default_tools,omitempty"`
			}
			require.NoError(t, json.Unmarshal(dispatch.Config, &dispatchConfig),
				"%s: dispatch config did not parse", name)

			if exp.absent {
				assert.Nil(t, dispatchConfig.DefaultTools,
					"%s: expected default_tools field absent (nil after unmarshal); got %v",
					name, dispatchConfig.DefaultTools)
				return
			}

			// Non-nil expected — check exact match including order.
			// nil-vs-empty distinction matters per
			// agentic-dispatch/component.go:710 ("if c.config.DefaultTools != nil").
			require.NotNil(t, dispatchConfig.DefaultTools,
				"%s: expected default_tools to parse to non-nil slice (even if empty); "+
					"got nil — likely json tag drift or schema refactor", name)
			assert.Equal(t, exp.expected, dispatchConfig.DefaultTools,
				"%s: default_tools wire shape drifted from expected", name)
		})
	}
}
