package emitchange

import "github.com/c360studio/semstreams/agentic"

// ListTools returns the LLM-facing schema. Required fields encode the ADR-057
// §D3/§D6 discipline structurally so a non-compliant change cannot be emitted:
// every added/modified requirement needs a SHALL-class statement + ≥1 scenario;
// every task needs the execution-rich superset. The validator (payload.go)
// enforces the cross-field rules a JSON Schema cannot (SHALL keyword present, a
// scenario has WHEN+THEN, section contiguity).
func (e *Executor) ListTools() []agentic.ToolDefinition {
	stringArray := func(desc string) map[string]any {
		return map[string]any{"type": "array", "items": map[string]any{"type": "string"}, "description": desc}
	}
	obj := func(props map[string]any, required ...string) map[string]any {
		return map[string]any{"type": "object", "additionalProperties": false, "properties": props, "required": required}
	}

	stepSchema := obj(map[string]any{
		"kw":   map[string]any{"type": "string", "description": "Step keyword: GIVEN, WHEN, THEN, or AND (case-insensitive)."},
		"text": map[string]any{"type": "string", "description": "The step text after the keyword."},
	}, "kw", "text")
	scenarioSchema := obj(map[string]any{
		"name": map[string]any{"type": "string", "description": "Short scenario name."},
		"steps": map[string]any{"type": "array", "minItems": 2, "items": stepSchema,
			"description": "Given/When/Then steps. MUST include at least a WHEN and a THEN (the testable acceptance criteria — ADR-057 §D3)."},
	}, "name", "steps")
	requirementSchema := obj(map[string]any{
		"name":      map[string]any{"type": "string", "description": "Requirement name (becomes the slugified <rid> key; the name is preserved verbatim)."},
		"statement": map[string]any{"type": "string", "description": "RFC-2119 statement: 'The system SHALL/MUST/SHOULD/MAY <behavior>'. MUST contain a SHALL-class keyword."},
		"scenarios": map[string]any{"type": "array", "minItems": 1, "items": scenarioSchema,
			"description": "≥1 Given/When/Then scenario — the acceptance criteria for this requirement."},
	}, "name", "statement", "scenarios")
	modifiedSchema := obj(map[string]any{
		"name":       requirementSchema["properties"].(map[string]any)["name"],
		"statement":  requirementSchema["properties"].(map[string]any)["statement"],
		"scenarios":  requirementSchema["properties"].(map[string]any)["scenarios"],
		"previously": map[string]any{"type": "string", "description": "The PRIOR statement this revises — OpenSpec records it as '(Previously: …)'."},
	}, "name", "statement", "scenarios", "previously")
	removedSchema := obj(map[string]any{
		"name":      map[string]any{"type": "string", "description": "Name of the requirement being removed."},
		"rationale": map[string]any{"type": "string", "description": "Why it is deprecated — OpenSpec records it as '(Rationale: …)'."},
	}, "name", "rationale")
	deltaSchema := obj(map[string]any{
		"capability": map[string]any{"type": "string", "description": "Capability folder name (a single path segment, e.g. 'auth'). One delta per capability."},
		"added":      map[string]any{"type": "array", "items": requirementSchema, "description": "Brand-new requirements."},
		"modified":   map[string]any{"type": "array", "items": modifiedSchema, "description": "Revised requirements (each carries its prior statement)."},
		"removed":    map[string]any{"type": "array", "items": removedSchema, "description": "Removed requirements (name + rationale)."},
	}, "capability")

	taskSchema := obj(map[string]any{
		"section":          map[string]any{"type": "string", "description": "OpenSpec '## <section>' grouping (e.g. '1. TOTP'). Group all tasks of a section together (sections must be contiguous)."},
		"number":           map[string]any{"type": "string", "description": "OpenSpec dotted label (e.g. '1.1'). Optional; cosmetic — the graph keys tasks by position."},
		"text":             map[string]any{"type": "string", "description": "OpenSpec checkbox prose — renders '- [ ] <number> <text>'."},
		"goal":             map[string]any{"type": "string", "description": "§D6 execution goal Ralph drives to. Concrete and verifiable."},
		"target_files":     stringArray("§D6 file globs the executor may modify (≥1 — surgical scope)."),
		"test_command":     map[string]any{"type": "string", "description": "§D6 executable acceptance command Ralph iterates until it exits 0."},
		"assumptions":      stringArray("§D6 task assumptions. May be empty; emit [] explicitly."),
		"non_goals":        stringArray("§D6 task anti-scope. May be empty; emit [] explicitly."),
		"expected_outcome": map[string]any{"type": "string", "description": "Optional 'done looks like' description (CBG diff review + logging)."},
		"requirement_ref":  map[string]any{"type": "string", "description": "Optional '<capability>/<requirement-name>' the task implements — links the task to a delta requirement."},
	}, "section", "text", "goal", "target_files", "test_command", "assumptions", "non_goals")

	proposalSchema := obj(map[string]any{
		"intent":    map[string]any{"type": "string", "description": "Why the change matters (the proposal '## Intent')."},
		"scope_in":  stringArray("In-scope bullets. May be empty; emit [] explicitly."),
		"scope_out": stringArray("Out-of-scope bullets. May be empty; emit [] explicitly."),
		"approach":  map[string]any{"type": "string", "description": "High-level direction (the proposal '## Approach')."},
	}, "intent", "scope_in", "scope_out", "approach")
	designSchema := obj(map[string]any{
		"technical_approach": map[string]any{"type": "string", "description": "Design '## Technical Approach' prose."},
		"decisions": map[string]any{"type": "array", "description": "Architecture decisions.", "items": obj(map[string]any{
			"name": map[string]any{"type": "string"}, "body": map[string]any{"type": "string"},
		}, "name")},
		"data_flow": map[string]any{"type": "string", "description": "Design '## Data Flow' prose."},
		"file_changes": map[string]any{"type": "array", "description": "Anticipated file changes.", "items": obj(map[string]any{
			"path": map[string]any{"type": "string"}, "kind": map[string]any{"type": "string", "description": "new | modified | deleted"},
		}, "path")},
	})

	return []agentic.ToolDefinition{{
		Name: ToolName,
		Description: "Emit an OpenSpec change. Stamps change.<slug>.* triples on the run entity (resolved for you). " +
			"Per ADR-057 §D3/§D6 + [[encode-principles-structurally]], the schema + validator reject a change whose requirements lack a SHALL-class statement or a WHEN/THEN scenario, or whose tasks lack the execution-rich fields. Call once per create_change arc before the terminal decide.",
		Parameters: obj(map[string]any{
			"slug":               map[string]any{"type": "string", "description": "Change folder name, lower-kebab-case (e.g. 'add-mfa'). Descriptive of the change."},
			"proposal":           proposalSchema,
			"design":             designSchema,
			"deltas":             map[string]any{"type": "array", "minItems": 1, "items": deltaSchema, "description": "Per-capability requirement deltas. At least one requirement must be added, modified, or removed."},
			"tasks":              map[string]any{"type": "array", "minItems": 1, "items": taskSchema, "description": "Ordered task list in the §D6 superset shape (OpenSpec-visible text/section + execution-rich goal/target_files/test_command)."},
			"acceptance_command": map[string]any{"type": "string", "description": "The chain-level full-suite command the chain-end reviewer (CBG) re-runs. Single shell command."},
		}, "slug", "proposal", "deltas", "tasks", "acceptance_command"),
	}}
}
