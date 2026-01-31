package agentictools

import (
	"strings"
)

// ToolDefinition represents a tool definition for discovery responses
type ToolDefinition struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Provider    string `json:"provider"`
	Available   bool   `json:"available"`
}

// ToolListResponse represents the response to a tool.list request
type ToolListResponse struct {
	Tools []ToolDefinition `json:"tools"`
}

// ConsumerNameForTool generates a JetStream consumer name for a tool.
// Sanitizes dots and underscores to dashes, adds "tool-exec-" prefix.
//
// Examples:
//
//	file_read → tool-exec-file-read
//	graph.query → tool-exec-graph-query
func ConsumerNameForTool(toolName string) string {
	// Replace dots and underscores with dashes
	sanitized := strings.ReplaceAll(toolName, ".", "-")
	sanitized = strings.ReplaceAll(sanitized, "_", "-")

	// Add prefix
	return "tool-exec-" + sanitized
}
