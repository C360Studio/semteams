package agentic

// ToolCallFilter intercepts tool calls before execution.
// Implementations can approve, reject, or modify calls for authorization,
// rate limiting, or domain-scoped access control.
type ToolCallFilter interface {
	FilterToolCalls(loopID string, calls []ToolCall) (ToolCallFilterResult, error)
}

// ToolCallFilterResult contains the outcome of filtering tool calls.
type ToolCallFilterResult struct {
	Approved []ToolCall
	Rejected []ToolCallRejection
}

// ToolCallRejection pairs a rejected tool call with the reason it was denied.
type ToolCallRejection struct {
	Call   ToolCall
	Reason string
}
