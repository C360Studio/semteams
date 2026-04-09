package agenticdispatch

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSignalCommands_Registered(t *testing.T) {
	registry := NewCommandRegistry()

	// Register signal commands the same way the component does
	for _, cmd := range []struct {
		name    string
		pattern string
		help    string
	}{
		{"approve", `^/approve\s*(\S*)\s*(.*)$`, "/approve"},
		{"reject", `^/reject\s*(\S*)\s*(.*)$`, "/reject"},
		{"pause", `^/pause\s*(\S*)$`, "/pause"},
		{"resume", `^/resume\s*(\S*)$`, "/resume"},
	} {
		err := registry.Register(cmd.name, CommandConfig{
			Pattern: cmd.pattern,
			Help:    cmd.help,
		}, emptyHandler())
		require.NoError(t, err, "registering %s", cmd.name)
	}

	assert.Equal(t, 4, registry.Count())
}

func TestSignalCommands_PatternMatching(t *testing.T) {
	registry := NewCommandRegistry()

	// Register commands
	registry.Register("approve", CommandConfig{Pattern: `^/approve\s*(\S*)\s*(.*)$`}, emptyHandler())
	registry.Register("reject", CommandConfig{Pattern: `^/reject\s*(\S*)\s*(.*)$`}, emptyHandler())
	registry.Register("pause", CommandConfig{Pattern: `^/pause\s*(\S*)$`}, emptyHandler())
	registry.Register("resume", CommandConfig{Pattern: `^/resume\s*(\S*)$`}, emptyHandler())

	tests := []struct {
		input    string
		wantName string
		wantArgs []string
	}{
		{"/approve", "approve", []string{"", ""}},
		{"/approve loop_123", "approve", []string{"loop_123", ""}},
		{"/approve loop_123 looks good to me", "approve", []string{"loop_123", "looks good to me"}},
		{"/reject loop_abc needs more work", "reject", []string{"loop_abc", "needs more work"}},
		{"/pause", "pause", []string{""}},
		{"/pause loop_xyz", "pause", []string{"loop_xyz"}},
		{"/resume loop_xyz", "resume", []string{"loop_xyz"}},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			name, _, args, found := registry.Match(tt.input)
			require.True(t, found, "command should match: %s", tt.input)
			assert.Equal(t, tt.wantName, name)
			assert.Equal(t, tt.wantArgs, args)
		})
	}
}

func TestSignalCommands_NoMatchOtherCommands(t *testing.T) {
	registry := NewCommandRegistry()
	registry.Register("approve", CommandConfig{Pattern: `^/approve\s*(\S*)\s*(.*)$`}, emptyHandler())

	_, _, _, found := registry.Match("/cancel loop_123")
	assert.False(t, found, "/cancel should not match /approve")

	_, _, _, found = registry.Match("approve something")
	assert.False(t, found, "text without / prefix should not match")
}
