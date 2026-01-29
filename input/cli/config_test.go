package cli

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDefaultConfig(t *testing.T) {
	config := DefaultConfig()

	assert.Equal(t, "cli-user", config.UserID)
	assert.Equal(t, "cli-session", config.SessionID)
	assert.Equal(t, "> ", config.Prompt)
	assert.Equal(t, "USER", config.StreamName)

	// Check ports
	require.NotNil(t, config.Ports)
	assert.Len(t, config.Ports.Inputs, 1)
	assert.Len(t, config.Ports.Outputs, 2)
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
			name: "valid custom config",
			config: Config{
				UserID:    "my-user",
				SessionID: "my-session",
				Prompt:    ">> ",
			},
			wantErr: "",
		},
		{
			name: "missing user_id",
			config: Config{
				SessionID: "session",
			},
			wantErr: "user_id is required",
		},
		{
			name: "missing session_id",
			config: Config{
				UserID: "user",
			},
			wantErr: "session_id is required",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.config.Validate()
			if tt.wantErr == "" {
				assert.NoError(t, err)
			} else {
				assert.EqualError(t, err, tt.wantErr)
			}
		})
	}
}

func TestPortDefinitions(t *testing.T) {
	config := DefaultConfig()

	// Check input ports
	inputNames := make(map[string]bool)
	for _, p := range config.Ports.Inputs {
		inputNames[p.Name] = true
	}
	assert.True(t, inputNames["user.response"])

	// Check output ports
	outputNames := make(map[string]bool)
	for _, p := range config.Ports.Outputs {
		outputNames[p.Name] = true
	}
	assert.True(t, outputNames["user.message"])
	assert.True(t, outputNames["user.signal"])
}
