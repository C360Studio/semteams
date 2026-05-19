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
// Pins the dispatch component's default_tools field on the wire
// AND the post-parse struct shape, with a per-config expectation
// table. A failure pinpoints exactly which config drifted from its
// authored allowlist (e.g. JSON tag drift, Config-struct refactor,
// or accidental schema rename).
//
// Per-config expectations: the file lists the wire-shape default_tools
// the operator authored. Each entry is independently validated. A
// config without the field at all declares `absent:true`; a config
// with `default_tools: []` declares `expected: []string{}`.
//
// Under ADR-042 MVP-7 only two configs survive (flow-bootstrap
// production + e2e-flow-bootstrap mock-LLM). The pre-MVP-7
// expectation table covered ~9 deleted configs; that history lives
// in git log if needed.
func TestConfigDispatchDefaultToolsParse(t *testing.T) {
	expectations := map[string]struct {
		// absent: the config has no default_tools key at all (legitimate
		// for flows whose initial role uses global discovery — the
		// surviving bootstrap configs both default to coordinator role
		// and let agentic-tools' allow-list policy take over).
		absent bool
		// expected: when absent=false, the exact tool-name list the
		// config author put in the file (order-sensitive).
		expected []string
	}{
		"flow-bootstrap.json":     {absent: true},
		"e2e-flow-bootstrap.json": {absent: true},
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
