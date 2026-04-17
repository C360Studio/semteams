package teams

// Type aliases for tool types from semstreams/agentic. These are the
// SAME Go types — methods (Validate, Schema, Marshal/Unmarshal) are
// inherited from the framework.
//
// NOTE: ToolDefinition.RequiresApproval was a semteams-specific field
// that doesn't exist in semstreams. It's temporarily removed pending
// an upstream PR. The approval gate feature needs reworking to use a
// config-based approach instead of a struct field. See memory:
// project_fork_to_import_migration.md.

import (
	"fmt"
	"strings"

	"github.com/c360studio/semstreams/agentic"
)

// ToolDefinition is an alias for the framework's tool definition type.
type ToolDefinition = agentic.ToolDefinition

// ToolCall is an alias for the framework's tool call type.
type ToolCall = agentic.ToolCall

// ToolResult is an alias for the framework's tool result type.
type ToolResult = agentic.ToolResult

// ValidateToolsAllowed validates that all tool calls are in the allowed list.
// This is a semteams-specific helper (not in the framework).
func ValidateToolsAllowed(calls []ToolCall, allowed []string) error {
	if len(calls) == 0 {
		return nil
	}

	allowedSet := make(map[string]bool)
	for _, name := range allowed {
		allowedSet[name] = true
	}

	var disallowed []string
	for _, call := range calls {
		if !allowedSet[call.Name] {
			disallowed = append(disallowed, call.Name)
		}
	}

	if len(disallowed) > 0 {
		return fmt.Errorf("disallowed tools: %s", strings.Join(disallowed, ", "))
	}

	return nil
}
