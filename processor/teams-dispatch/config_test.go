package teamsdispatch

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDefaultConfig(t *testing.T) {
	config := DefaultConfig()

	assert.Equal(t, "general", config.DefaultRole)
	assert.True(t, config.AutoContinue)
	assert.Equal(t, "TEAMS", config.StreamName)

	// Check permissions
	assert.Contains(t, config.Permissions.View, "*")
	assert.Contains(t, config.Permissions.SubmitTask, "*")
	assert.True(t, config.Permissions.CancelOwn)
	assert.Contains(t, config.Permissions.Approve, "*")

	// Check ports
	require.NotNil(t, config.Ports)
	assert.Len(t, config.Ports.Inputs, 4)  // user_messages, complete, created, failed
	assert.Len(t, config.Ports.Outputs, 3) // tasks, signals, user_response
}

func TestConfig_Validate(t *testing.T) {
	tests := []struct {
		name    string
		config  Config
		wantErr string
	}{
		{
			name:    "valid default config",
			config:  DefaultConfig(),
			wantErr: "",
		},
		{
			name: "valid minimal config",
			config: Config{
				DefaultRole: "agent",
				StreamName:  "CUSTOM",
			},
			wantErr: "",
		},
		{
			name: "missing default_role",
			config: Config{
				StreamName: "TEAMS",
			},
			wantErr: "default_role is required",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.config.Validate()
			if tt.wantErr == "" {
				assert.NoError(t, err)
			} else {
				assert.ErrorContains(t, err, tt.wantErr)
			}
		})
	}
}

func TestPermissions_Defaults(t *testing.T) {
	config := DefaultConfig()

	// Default allows anyone to view and submit
	assert.Contains(t, config.Permissions.View, "*")
	assert.Contains(t, config.Permissions.SubmitTask, "*")

	// Users can cancel their own by default
	assert.True(t, config.Permissions.CancelOwn)

	// No one can cancel others' by default
	assert.Empty(t, config.Permissions.CancelAny)

	// Everyone can approve by default
	assert.Contains(t, config.Permissions.Approve, "*")
}

func TestPortDefinitions(t *testing.T) {
	config := DefaultConfig()

	// Check input ports
	inputNames := make(map[string]bool)
	for _, p := range config.Ports.Inputs {
		inputNames[p.Name] = true
	}
	assert.True(t, inputNames["user_messages"])
	assert.True(t, inputNames["complete"])
	assert.True(t, inputNames["created"])
	assert.True(t, inputNames["failed"])

	// Check output ports
	outputNames := make(map[string]bool)
	for _, p := range config.Ports.Outputs {
		outputNames[p.Name] = true
	}
	assert.True(t, outputNames["tasks"])
	assert.True(t, outputNames["signals"])
	assert.True(t, outputNames["user_response"])
}

func TestPortDefinitions_Subjects(t *testing.T) {
	config := DefaultConfig()

	// Verify input subjects
	for _, p := range config.Ports.Inputs {
		switch p.Name {
		case "user_messages":
			assert.Equal(t, "teams.user.message.>", p.Subject)
			assert.Equal(t, "TEAMS", p.StreamName)
		case "complete":
			assert.Equal(t, "teams.complete.*", p.Subject)
			assert.Equal(t, "TEAMS", p.StreamName)
		case "created":
			assert.Equal(t, "teams.created.*", p.Subject)
			assert.Equal(t, "TEAMS", p.StreamName)
		case "failed":
			assert.Equal(t, "teams.failed.*", p.Subject)
			assert.Equal(t, "TEAMS", p.StreamName)
		}
	}

	// Verify output subjects
	for _, p := range config.Ports.Outputs {
		switch p.Name {
		case "tasks":
			assert.Equal(t, "teams.task.*", p.Subject)
			assert.Equal(t, "TEAMS", p.StreamName)
		case "signals":
			assert.Equal(t, "teams.signal.*", p.Subject)
			assert.Equal(t, "TEAMS", p.StreamName)
		case "user_response":
			assert.Equal(t, "teams.user.response.>", p.Subject)
			assert.Equal(t, "TEAMS", p.StreamName)
		}
	}
}
