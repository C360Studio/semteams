package teamsdispatch

import (
	"context"
	"testing"

	"github.com/c360studio/semteams/teams"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// stubHandler creates a test handler that returns a fixed response.
// It validates that context is not nil and captures call metadata for test assertions.
func stubHandler(response string) CommandHandler {
	return func(ctx context.Context, msg teams.UserMessage, args []string, loopID string) (teams.UserResponse, error) {
		// Validate context is passed correctly
		if ctx == nil {
			panic("context should not be nil")
		}
		// Use all parameters to satisfy linter and enable future test assertions
		resp := teams.UserResponse{
			Content:     response,
			ChannelType: msg.ChannelType,
			ChannelID:   msg.ChannelID,
			UserID:      msg.UserID,
		}
		if loopID != "" {
			resp.InReplyTo = loopID
		}
		if len(args) > 0 && args[0] != "" {
			resp.Content = response + " (arg: " + args[0] + ")"
		}
		return resp, nil
	}
}

// emptyHandler creates a minimal test handler for registration tests.
func emptyHandler() CommandHandler {
	return func(ctx context.Context, msg teams.UserMessage, args []string, loopID string) (teams.UserResponse, error) {
		// Ensure parameters are used to satisfy linter
		_ = ctx.Done()
		_ = msg.UserID
		_ = len(args)
		_ = loopID
		return teams.UserResponse{}, nil
	}
}

func TestNewCommandRegistry(t *testing.T) {
	registry := NewCommandRegistry()
	require.NotNil(t, registry)
	assert.Equal(t, 0, registry.Count())
}

func TestCommandRegistry_Register(t *testing.T) {
	registry := NewCommandRegistry()

	config := CommandConfig{
		Pattern: `^/test(?:\s+(.*))?$`,
		Help:    "A test command",
	}

	err := registry.Register("test", config, stubHandler("test response"))
	assert.NoError(t, err)
	assert.Equal(t, 1, registry.Count())
}

func TestCommandRegistry_Register_InvalidPattern(t *testing.T) {
	registry := NewCommandRegistry()

	config := CommandConfig{
		Pattern: `^/test[invalid`,
	}

	err := registry.Register("test", config, emptyHandler())
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "compile pattern")
}

func TestCommandRegistry_Register_Duplicate(t *testing.T) {
	registry := NewCommandRegistry()

	config := CommandConfig{
		Pattern: `^/test$`,
	}

	err := registry.Register("test", config, emptyHandler())
	assert.NoError(t, err)

	// Register again with same name
	err = registry.Register("test", config, emptyHandler())
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "already registered")
}

func TestCommandRegistry_Match(t *testing.T) {
	registry := NewCommandRegistry()

	config := CommandConfig{
		Pattern: `^/cancel(?:\s+(\S+))?$`,
	}

	err := registry.Register("cancel", config, stubHandler("cancelled"))
	require.NoError(t, err)

	tests := []struct {
		input    string
		wantName string
		wantArgs []string
		wantOk   bool
	}{
		{"/cancel", "cancel", []string{""}, true},
		{"/cancel loop-123", "cancel", []string{"loop-123"}, true},
		{"/other", "", nil, false},
		{"not a command", "", nil, false},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			name, _, args, ok := registry.Match(tt.input)
			assert.Equal(t, tt.wantOk, ok)
			if ok {
				assert.Equal(t, tt.wantName, name)
				assert.Equal(t, tt.wantArgs, args)
			}
		})
	}
}

func TestCommandRegistry_Match_MultipleGroups(t *testing.T) {
	registry := NewCommandRegistry()

	config := CommandConfig{
		Pattern: `^/set\s+(\S+)\s+(.+)$`,
	}

	err := registry.Register("set", config, emptyHandler())
	require.NoError(t, err)

	name, _, args, ok := registry.Match("/set key value with spaces")
	assert.True(t, ok)
	assert.Equal(t, "set", name)
	assert.Equal(t, []string{"key", "value with spaces"}, args)
}

func TestCommandRegistry_Get(t *testing.T) {
	registry := NewCommandRegistry()

	config := CommandConfig{
		Pattern: `^/test$`,
		Help:    "Test command",
	}

	registry.Register("test", config, emptyHandler())

	retrieved, ok := registry.Get("test")
	assert.True(t, ok)
	assert.Equal(t, "test", retrieved.Name)
	assert.Equal(t, "Test command", retrieved.Config.Help)

	_, ok = registry.Get("nonexistent")
	assert.False(t, ok)
}

func TestCommandRegistry_All(t *testing.T) {
	registry := NewCommandRegistry()

	// Register multiple commands
	for _, name := range []string{"cancel", "status", "help"} {
		config := CommandConfig{
			Pattern: `^/` + name + `$`,
		}
		registry.Register(name, config, emptyHandler())
	}

	all := registry.All()
	assert.Len(t, all, 3)
	assert.Contains(t, all, "cancel")
	assert.Contains(t, all, "status")
	assert.Contains(t, all, "help")
}

func TestCommandRegistry_Match_NoArgs(t *testing.T) {
	registry := NewCommandRegistry()

	config := CommandConfig{
		Pattern: `^/help$`,
	}

	err := registry.Register("help", config, stubHandler("help text"))
	require.NoError(t, err)

	name, cmd, args, ok := registry.Match("/help")
	assert.True(t, ok)
	assert.Equal(t, "help", name)
	assert.NotNil(t, cmd)
	assert.Empty(t, args) // No capture groups, so empty args
}

func TestCommandRegistry_Permission(t *testing.T) {
	registry := NewCommandRegistry()

	config := CommandConfig{
		Pattern:    `^/admin$`,
		Permission: "admin",
	}

	err := registry.Register("admin", config, emptyHandler())
	require.NoError(t, err)

	_, cmd, _, ok := registry.Match("/admin")
	assert.True(t, ok)
	assert.Equal(t, "admin", cmd.Config.Permission)
}

func TestCommandRegistry_RequireLoop(t *testing.T) {
	registry := NewCommandRegistry()

	config := CommandConfig{
		Pattern:     `^/cancel$`,
		RequireLoop: true,
	}

	err := registry.Register("cancel", config, emptyHandler())
	require.NoError(t, err)

	_, cmd, _, ok := registry.Match("/cancel")
	assert.True(t, ok)
	assert.True(t, cmd.Config.RequireLoop)
}

func TestCommandRegistry_HandlerExecution(t *testing.T) {
	registry := NewCommandRegistry()

	config := CommandConfig{
		Pattern: `^/echo(?:\s+(.*))?$`,
	}

	err := registry.Register("echo", config, stubHandler("echoed"))
	require.NoError(t, err)

	_, cmd, args, ok := registry.Match("/echo hello")
	require.True(t, ok)

	// Execute the handler and verify it uses all parameters
	ctx := context.Background()
	msg := teams.UserMessage{
		ChannelType: "cli",
		ChannelID:   "test-session",
		UserID:      "test-user",
	}

	resp, err := cmd.Handler(ctx, msg, args, "loop-123")
	require.NoError(t, err)

	assert.Equal(t, "cli", resp.ChannelType)
	assert.Equal(t, "test-session", resp.ChannelID)
	assert.Equal(t, "test-user", resp.UserID)
	assert.Equal(t, "loop-123", resp.InReplyTo)
	assert.Contains(t, resp.Content, "hello")
}
