package analyzeproof

import "github.com/c360studio/semstreams/agentic"

// ListTools returns the LLM-facing schema. The tool has no arguments: it always
// analyzes the proof.* facts on the current run entity.
func (e *Executor) ListTools() []agentic.ToolDefinition {
	return []agentic.ToolDefinition{{
		Name: ToolName,
		Description: "Analyze proof.* facts on the current run entity and emit formal_claims.* " +
			"findings for routing. This is deterministic graph evaluation, not an LLM judgment: " +
			"missing dependencies, stale readiness, failed readiness, and inactive waivers become " +
			"routeable findings for the coordinator, test-harness team, and UI.",
		Parameters: map[string]any{
			"type":                 "object",
			"additionalProperties": false,
			"properties":           map[string]any{},
		},
	}}
}
