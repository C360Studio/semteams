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

			dispatch, hasDispatch := config.Components["agentic-dispatch"]
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
