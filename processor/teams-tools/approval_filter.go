package teamtools

import (
	"fmt"
	"strings"

	"github.com/c360studio/semteams/teams"
)

// ApprovalRequiredPrefix is prepended to rejection reasons when a tool requires
// human approval. The agentic-loop detects this prefix and transitions the loop
// to LoopStateAwaitingApproval instead of storing a normal error result.
const ApprovalRequiredPrefix = "approval_required: "

// IsApprovalRequired checks whether a rejection reason indicates the tool
// needs human approval rather than being a genuine error.
func IsApprovalRequired(reason string) bool {
	return strings.HasPrefix(reason, ApprovalRequiredPrefix)
}

// ApprovalFilter implements teams.ToolCallFilter. It checks each tool call
// against the tool registry — if the tool's definition has RequiresApproval: true,
// the call is rejected with an ApprovalRequiredPrefix reason so the loop can
// transition to awaiting_approval.
type ApprovalFilter struct {
	registry *ExecutorRegistry
}

// NewApprovalFilter creates a filter that enforces RequiresApproval gates.
func NewApprovalFilter(registry *ExecutorRegistry) *ApprovalFilter {
	return &ApprovalFilter{registry: registry}
}

var _ teams.ToolCallFilter = (*ApprovalFilter)(nil)

// FilterToolCalls checks each call against the registry for RequiresApproval.
func (f *ApprovalFilter) FilterToolCalls(_ string, calls []teams.ToolCall) (teams.ToolCallFilterResult, error) {
	var result teams.ToolCallFilterResult

	for _, call := range calls {
		if f.requiresApproval(call.Name) {
			result.Rejected = append(result.Rejected, teams.ToolCallRejection{
				Call:   call,
				Reason: fmt.Sprintf("%sTool '%s' requires human approval before execution", ApprovalRequiredPrefix, call.Name),
			})
		} else {
			result.Approved = append(result.Approved, call)
		}
	}

	return result, nil
}

// requiresApproval checks if the named tool has RequiresApproval set.
func (f *ApprovalFilter) requiresApproval(toolName string) bool {
	executor := f.registry.GetTool(toolName)
	if executor == nil {
		return false // unknown tools are handled by the dispatch layer
	}

	for _, def := range executor.ListTools() {
		if def.Name == toolName {
			return def.RequiresApproval
		}
	}
	return false
}
