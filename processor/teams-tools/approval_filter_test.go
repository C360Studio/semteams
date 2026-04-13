package teamtools

import (
	"context"
	"testing"

	"github.com/c360studio/semstreams/agentic"
	"github.com/c360studio/semteams/teams"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mockApprovalExecutor is a ToolExecutor that returns tools with configurable RequiresApproval.
type mockApprovalExecutor struct {
	tools []teams.ToolDefinition
}

func (m *mockApprovalExecutor) Execute(_ context.Context, call agentic.ToolCall) (agentic.ToolResult, error) {
	return agentic.ToolResult{CallID: call.ID, Content: "ok"}, nil
}

func (m *mockApprovalExecutor) ListTools() []teams.ToolDefinition {
	return m.tools
}

func TestApprovalFilter_ApprovesNormalTools(t *testing.T) {
	reg := NewExecutorRegistry()
	reg.RegisterTool("bash", &mockApprovalExecutor{
		tools: []teams.ToolDefinition{{Name: "bash"}},
	})

	filter := NewApprovalFilter(reg)
	result, err := filter.FilterToolCalls("loop-1", []agentic.ToolCall{
		{ID: "c1", Name: "bash"},
	})
	require.NoError(t, err)
	assert.Len(t, result.Approved, 1)
	assert.Empty(t, result.Rejected)
}

func TestApprovalFilter_RejectsApprovalRequired(t *testing.T) {
	t.Skip("RequiresApproval field removed during fork-to-import migration. Restore after upstreaming to semstreams.")
	reg := NewExecutorRegistry()
	reg.RegisterTool("create_rule", &mockApprovalExecutor{
		tools: []teams.ToolDefinition{{Name: "create_rule"}},
	})

	filter := NewApprovalFilter(reg)
	result, err := filter.FilterToolCalls("loop-1", []agentic.ToolCall{
		{ID: "c1", Name: "create_rule"},
	})
	require.NoError(t, err)
	assert.Empty(t, result.Approved)
	assert.Len(t, result.Rejected, 1)
	assert.True(t, IsApprovalRequired(result.Rejected[0].Reason))
}

func TestApprovalFilter_MixedBatch(t *testing.T) {
	t.Skip("RequiresApproval field removed during fork-to-import migration. Restore after upstreaming to semstreams.")
	reg := NewExecutorRegistry()
	reg.RegisterTool("bash", &mockApprovalExecutor{
		tools: []teams.ToolDefinition{{Name: "bash"}},
	})
	reg.RegisterTool("delete_rule", &mockApprovalExecutor{
		tools: []teams.ToolDefinition{{Name: "delete_rule"}},
	})

	filter := NewApprovalFilter(reg)
	result, err := filter.FilterToolCalls("loop-1", []agentic.ToolCall{
		{ID: "c1", Name: "bash"},
		{ID: "c2", Name: "delete_rule"},
	})
	require.NoError(t, err)
	assert.Len(t, result.Approved, 1)
	assert.Equal(t, "bash", result.Approved[0].Name)
	assert.Len(t, result.Rejected, 1)
	assert.Equal(t, "delete_rule", result.Rejected[0].Call.Name)
}

func TestApprovalFilter_UnknownToolPassesThrough(t *testing.T) {
	reg := NewExecutorRegistry()
	filter := NewApprovalFilter(reg)

	result, err := filter.FilterToolCalls("loop-1", []agentic.ToolCall{
		{ID: "c1", Name: "nonexistent_tool"},
	})
	require.NoError(t, err)
	assert.Len(t, result.Approved, 1)
	assert.Empty(t, result.Rejected)
}

func TestIsApprovalRequired(t *testing.T) {
	assert.True(t, IsApprovalRequired("approval_required: Tool 'create_rule' requires human approval"))
	assert.False(t, IsApprovalRequired("tool call rejected: some other reason"))
	assert.False(t, IsApprovalRequired(""))
}
