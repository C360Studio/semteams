package renderopenspec

import "github.com/c360studio/semstreams/agentic"

// ListTools returns the LLM-facing schema. slug is optional — omit it to render
// the single change present on the run entity (the create_change happy path).
func (e *Executor) ListTools() []agentic.ToolDefinition {
	return []agentic.ToolDefinition{{
		Name: ToolName,
		Description: "Render the OpenSpec change stamped on the current run entity to a single " +
			"OpenSpec-markdown document (proposal + delta specs + tasks, each under its " +
			"openspec/changes/<slug>/ file marker). Reads change.<slug>.* from the graph — call it " +
			"after a create_change arc has emitted a change, to deliver the actual reviewed spec " +
			"to the user. Returns the rendered markdown as the tool result.",
		Parameters: map[string]any{
			"type":                 "object",
			"additionalProperties": false,
			"properties": map[string]any{
				"slug": map[string]any{
					"type": "string",
					"description": "Optional change slug (e.g. 'add-mfa'). Omit to render the single change " +
						"on the run entity; required only when more than one change is present.",
				},
			},
		},
	}}
}
