package projectspecplan

import "github.com/c360studio/semstreams/agentic"

// ListTools returns the LLM-facing schema. The tool is intentionally tiny:
// the task content is read from the graph, not supplied by the caller.
func (e *Executor) ListTools() []agentic.ToolDefinition {
	return []agentic.ToolDefinition{{
		Name: ToolName,
		Description: "Project an approved OpenSpec change's execution-rich task facts into " +
			"the plan.* triples consumed by the existing dev-via-test Ralph + CBG execution " +
			"primitive. Reads change.<slug>.* from the current run entity, verifies the " +
			"create_change run was reviewer-approved, and stamps plan.task.* plus " +
			"plan.integration_test_command and plan.done_authority.*. This is an adapter, not a planner.",
		Parameters: map[string]any{
			"type":                 "object",
			"additionalProperties": false,
			"properties": map[string]any{
				"slug": map[string]any{
					"type":        "string",
					"description": "Optional change slug (e.g. 'add-mfa'). Omit when the run entity has exactly one change.",
				},
			},
		},
	}}
}
