package agentic

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/c360studio/semstreams/message"
)

// ToolDefinition represents the definition of a tool that can be called
type ToolDefinition struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	Parameters  map[string]any `json:"parameters"`
}

// Validate checks if the ToolDefinition is valid
func (t ToolDefinition) Validate() error {
	if t.Name == "" {
		return fmt.Errorf("tool name required")
	}
	if t.Parameters == nil || len(t.Parameters) == 0 {
		return fmt.Errorf("tool parameters required")
	}
	return nil
}

// ToolCall represents a request to call a tool
type ToolCall struct {
	ID        string         `json:"id"`
	Name      string         `json:"name"`
	Arguments map[string]any `json:"arguments,omitempty"`
}

// Validate checks if the ToolCall is valid
func (t ToolCall) Validate() error {
	if t.ID == "" {
		return fmt.Errorf("tool call id required")
	}
	if t.Name == "" {
		return fmt.Errorf("tool call function name required")
	}
	return nil
}

// Schema implements message.Payload
func (t *ToolCall) Schema() message.Type {
	return message.Type{Domain: Domain, Category: CategoryToolCall, Version: SchemaVersion}
}

// MarshalJSON implements json.Marshaler
func (t *ToolCall) MarshalJSON() ([]byte, error) {
	type Alias ToolCall
	return json.Marshal((*Alias)(t))
}

// UnmarshalJSON implements json.Unmarshaler
func (t *ToolCall) UnmarshalJSON(data []byte) error {
	type Alias ToolCall
	return json.Unmarshal(data, (*Alias)(t))
}

// ToolResult represents the result of a tool call
type ToolResult struct {
	CallID   string         `json:"call_id"`
	Content  string         `json:"content,omitempty"`
	Error    string         `json:"error,omitempty"`
	Metadata map[string]any `json:"metadata,omitempty"`
}

// Validate checks if the ToolResult is valid
func (t ToolResult) Validate() error {
	if t.CallID == "" {
		return fmt.Errorf("tool result call_id required")
	}
	return nil
}

// Schema implements message.Payload
func (t *ToolResult) Schema() message.Type {
	return message.Type{Domain: Domain, Category: CategoryToolResult, Version: SchemaVersion}
}

// MarshalJSON implements json.Marshaler
func (t *ToolResult) MarshalJSON() ([]byte, error) {
	type Alias ToolResult
	return json.Marshal((*Alias)(t))
}

// UnmarshalJSON implements json.Unmarshaler
func (t *ToolResult) UnmarshalJSON(data []byte) error {
	type Alias ToolResult
	return json.Unmarshal(data, (*Alias)(t))
}

// ValidateToolsAllowed validates that all tool calls are in the allowed list
func ValidateToolsAllowed(calls []ToolCall, allowed []string) error {
	if len(calls) == 0 {
		return nil
	}

	// Build allowed set for fast lookup
	allowedSet := make(map[string]bool)
	for _, name := range allowed {
		allowedSet[name] = true
	}

	// Check each call
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
