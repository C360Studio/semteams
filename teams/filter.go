package teams

import "github.com/c360studio/semstreams/agentic"

// ToolCallFilter intercepts tool calls before execution.
// Implementations can approve, reject, or modify calls for authorization,
// rate limiting, or domain-scoped access control.
type ToolCallFilter interface {
	FilterToolCalls(loopID string, calls []agentic.ToolCall) (ToolCallFilterResult, error)
}

// ToolCallFilterResult contains the outcome of filtering tool calls.
type ToolCallFilterResult struct {
	Approved []agentic.ToolCall
	Rejected []ToolCallRejection
}

// ToolCallRejection pairs a rejected tool call with the reason it was denied.
type ToolCallRejection struct {
	Call   agentic.ToolCall
	Reason string
}
