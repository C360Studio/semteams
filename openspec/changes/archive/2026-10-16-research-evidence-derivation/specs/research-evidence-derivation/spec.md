# research-evidence-derivation Specification

## ADDED Requirements

### Requirement: Claim-level evidence citations on research artifacts

Every `Actor` and `IntegrationPoint` in `research.Artifact` and the `emit_research_artifact` tool schema SHALL carry a non-empty `evidence` list of `research.EvidenceRecord` entries (`channel`, `locator`, `origin_id`, `source_class`, `retrieved_at`, `quote`, `tool_result_ref`, `revision`, and optional `kind`). `OriginID` SHALL equal `NormalizeOriginID(channel, locator)`, `quote` SHALL be at least 12 characters after whitespace normalization, and `revision` SHALL satisfy `1 <= revision <= artifact.Revision`. Missing or unavailable evidence SHALL remain representable via `open_gaps` without fabricating citations.

#### Scenario: Well-formed citations across github, web, and semsource channels

- **GIVEN** an artifact whose actors and integration points cite valid `github`, `web`, and `semsource` evidence records with canonical `origin_id`, non-empty `quote` (>= 12 chars), non-empty `tool_result_ref`, and `revision <= artifact.Revision`
- **WHEN** `research.Artifact.Validate()` executes
- **THEN** validation SHALL succeed
- **AND** `emit_research_artifact` markdown rendering SHALL include the citation provenance (`channel`/`source_class`, `kind`, `revision`, `tool_result_ref`, `retrieved_at`, and JSON-escaped `quote`) in the rendered markdown

#### Scenario: Missing evidence or non-canonical origin_id is rejected

- **GIVEN** an artifact with an actor or integration point whose `evidence` slice is empty, or whose `EvidenceRecord` has an invalid `channel`/`source_class`, a non-canonical `origin_id`, a `quote` shorter than 12 characters, an empty `tool_result_ref`, or `revision > artifact.Revision`
- **WHEN** `research.Artifact.Validate()` executes
- **THEN** validation SHALL fail with a descriptive error naming the offending field

### Requirement: Mechanical derivation verification at emit boundary

When a `ToolResultResolver` is configured on `emit_research_artifact`, the executor SHALL mechanically verify every cited `EvidenceRecord` without invoking an LLM: the referenced `tool_result_ref` MUST resolve to a successful channel-authorized retrieval tool result from a loop in the same run (`agent.run.entity-id`) whose producing revision matches `ev.Revision` (when known), the cited `quote` MUST NOT be echoed from the tool call's dispatch arguments, and the cited `locator`/`origin_id` (with URL/token boundaries) and verbatim `quote` MUST co-occur in the same result block or item of that tool result.

#### Scenario: Invented locator, fabricated quote, non-retrieval tool, or cross-run tool_result_ref is rejected

- **GIVEN** an `emit_research_artifact` call citing an unknown `tool_result_ref`, a `tool_result_ref` from another run, a non-retrieval or failed tool call, a `quote` echoed from dispatch arguments, a `locator` absent from the referenced tool result block, or a `quote` that is not a verbatim substring of the retrieved body in the same block/element as the locator
- **WHEN** `emit_research_artifact.Execute` runs
- **THEN** the executor SHALL reject the call with `agentic.ToolErrorInvalidArgs` (while propagating infrastructure/storage errors as `agentic.ToolErrorNetwork` or `agentic.ToolErrorInternal`)
- **AND** it SHALL NOT write the markdown artifact or publish graph triples

#### Scenario: Revision 2+ artifact retains revision-1 citations and validates revision-2 updates

- **GIVEN** a revision-2 `emit_research_artifact` call in a run that executed revision-1 and revision-2 gather loops sharing the same `agent.run.entity-id`
- **WHEN** unchanged claims cite revision-1 `tool_result_ref`s (`revision: 1`) and updated claims cite revision-2 `tool_result_ref`s (`revision: 2`)
- **THEN** `emit_research_artifact.Execute` SHALL verify both sets of citations against their respective recorded tool results and producer revisions in the run and succeed

