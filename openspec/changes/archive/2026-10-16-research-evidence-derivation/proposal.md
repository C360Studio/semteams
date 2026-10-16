# Evidence contract and mechanical derivation check for `emit_research_artifact`

## Why

[Issue #271](https://github.com/C360Studio/semteams/issues/271) and [ADR-060](../../../docs/adr/060-program-manager-direction-and-research-design.md) §2–4 require research and program-pulse synthesis (`#267`) to attach verifiable source provenance to every claim-bearing artifact entry and mechanically verify that cited claims derive from tool results produced during the active run. Today `cmd/semteams/research/artifact.go` and `emit_research_artifact` validate only structural field presence (`actors`, `integration_points`, `tasks`, `open_gaps`), allowing a synthesizer to emit ungrounded prose without citing a recorded tool result or verbatim passage.

## What changes

- Extend `research.Actor` and `research.IntegrationPoint` in `cmd/semteams/research/artifact.go` with required `evidence []research.EvidenceRecord` (`channel`, `locator`, `origin_id`, `source_class`, `retrieved_at`, `quote`, `tool_result_ref`, `revision`), keeping `tasks` as `[]string` and preserving `open_gaps` for missing or unavailable evidence.
- Validate citation structure on `research.Artifact.Validate()` via `EvidenceRecord.ValidateArtifactCitation()`, enforcing canonical `origin_id` (`NormalizeOriginID(channel, locator)`), non-empty verbatim `quote` and `tool_result_ref`, and `1 <= revision <= artifact.Revision`.
- Render evidence provenance on actors and integration points in `cmd/semteams/tools/emitartifact/executor.go` (`researchTemplateText`).
- Implement the zero-LLM mechanical derivation verifier in `cmd/semteams/tools/emitartifact/` (`ToolResultResolver` + NATS KV/ObjectStore-backed resolver wired in `cmd/semteams/product_tools.go`):
  - verify every cited `tool_result_ref` resolves to a completed tool result whose producing loop shares the calling synthesizer loop's `agent.run.entity-id` (rejecting unknown or cross-run `tool_result_ref`s while permitting revision 2+ to retain revision-1 citations from the same run);
  - verify the cited `locator` or canonical `origin_id` appears in the referenced tool result content or dispatch arguments;
  - verify the cited `quote` is a verbatim substring (`strings.Contains`) of the retrieved body in that tool result.
- Update the `emit_research_artifact` tool schema (`cmd/semteams/tools/emitartifact/executor.go`) and regenerate `schemas/` and `specs/openapi.v3.yaml`.
- Update `configs/personas/fragments/researcher-research-gather/`, `researcher-research-synthesize/`, and `reviewer-research/` for structured evidence handoff, citation synthesis, and reviewer freshness/diversity/gap checks.
- Allow optional deterministic `tool_call.id` in `test/e2e/mock/` and update the mock-LLM research journey fixtures (`research-mvp.yaml`, `research-fanout.yaml`, `ops-run-terminal.yaml`) so gatherers execute a real retrieval tool call and synthesizers cite its `tool_result_ref`, `locator`, and verbatim `quote`.

## Non-goals

- LLM-based claim-entailment judging inside `emit_research_artifact` (semantic evaluation remains `reviewer-research`'s responsibility).
- New retrieval adapters (`semsource` adapter is tracked in `#272`; `github_read` landed in `#273`).
- Wiring the `program-report` rule pack (`#267`).

## Acceptance and risks

- `emit_research_artifact` deterministically rejects missing `evidence`, invalid `channel`/`source_class`, non-canonical `origin_id`, unknown or cross-run `tool_result_ref`, invented `locator` not present in the referenced tool result, and fabricated `quote` spans not present verbatim in the retrieved body.
- Revision 2+ artifacts pass mechanical derivation when retaining revision-1 citations for unchanged claims and citing revision-2 gather tool results for updated claims.
- Mock-LLM research fixtures and contract tests pass schema validation and mechanical derivation end-to-end.
