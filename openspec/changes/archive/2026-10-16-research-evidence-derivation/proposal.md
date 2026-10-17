# Evidence contract and mechanical derivation check for `emit_research_artifact`

## Why

[Issue #271](https://github.com/C360Studio/semteams/issues/271) and [ADR-060](../../../../docs/adr/060-program-manager-direction-and-research-design.md) §2–4 require research and program-pulse synthesis (`#267`) to attach verifiable source provenance to every claim-bearing artifact entry and mechanically verify that cited claims derive from tool results produced during the active run. Today `cmd/semteams/research/artifact.go` and `emit_research_artifact` validate only structural field presence (`actors`, `integration_points`, `tasks`, `open_gaps`), allowing a synthesizer to emit ungrounded prose without citing a recorded tool result or verbatim passage.

## What changes

- Extend `research.Actor` and `research.IntegrationPoint` in `cmd/semteams/research/artifact.go` with required `evidence []research.EvidenceRecord` (`channel`, `locator`, `origin_id`, `source_class`, `retrieved_at`, `quote`, `tool_result_ref`, `revision`, and optional `kind`), keeping `tasks` as `[]string` and preserving `open_gaps` for missing or unavailable evidence.
- Server-populate omitted `origin_id` (`NormalizeOriginID(channel, locator)`), `retrieved_at` (from `produced_at` / trajectory `ObservedAt`), and `revision` (from `artifact.Revision`) via `Artifact.PopulateCitationDefaults()`, and validate citation structure on `research.Artifact.Validate()` via `EvidenceRecord.ValidateArtifactCitation()` (including `MinQuoteLength = 12`).
- Render evidence provenance on actors and integration points in `cmd/semteams/tools/emitartifact/executor.go` (`researchTemplateText`), including `kind`, `revision`, `tool_result_ref`, `retrieved_at`, and JSON-escaped `quote`.
- Implement the zero-LLM mechanical derivation verifier in `cmd/semteams/tools/emitartifact/` (`ToolResultResolver` + NATS `AGENT_TRAJECTORIES` KV / `AGENT_CONTENT` ObjectStore resolver wired in `cmd/semteams/product_tools.go`):
  - discover run loop IDs via `ENTITY_STATES` lineage (`agent.lineage.researcher-plan`, `research.gather.completed-subtopic` via `ReadEntityAll`, and `agent.lineage.researcher`) without full-bucket KV scans, and look up trajectory facts by `agentic.TrajectoryFactPrefix(loopID)` prefix filter (`natsclient.FilteredKeys`);
  - verify every cited `tool_result_ref` resolves to a successful (`status != "failed"` and empty `error`), channel-allowlisted retrieval tool result whose producing loop shares the calling synthesizer loop's `agent.run.entity-id` and whose producing revision matches `ev.Revision` (rejecting unknown or cross-run `tool_result_ref`s, non-retrieval tools like `read_loop_result`/`scratchpad`/`decide`, and revision spoofing);
  - verify the cited `quote` is not echoed from the tool call's dispatch arguments (`res.DispatchArguments`);
  - verify the cited `locator` or canonical `origin_id` (with URL/token boundary matching) and verbatim `quote` co-occur in the same result block (`[N]` for `web_search`) or same JSON element of the referenced tool result, and stamp `ev.Kind = "snippet"` on `web_search` citations and `ev.RetrievedAt` from trajectory `ObservedAt`.
- Update the `emit_research_artifact` tool schema (`cmd/semteams/tools/emitartifact/executor.go`) and regenerate `schemas/` and `specs/openapi.v3.yaml`.
- Update `configs/rules/research/03b-synthesize-when-all-gathers-complete.json` (threading `prior_synthesize_loop_id`) and `configs/personas/fragments/researcher-research-gather/`, `researcher-research-synthesize/`, and `reviewer-research/` for structured evidence handoff, citation synthesis, and reviewer freshness/diversity/gap checks.
- Allow optional deterministic `tool_call.id` in `test/e2e/mock/` and update the mock-LLM research journey fixtures (`research-mvp.yaml`, `research-fanout.yaml`, `ops-run-terminal.yaml`) and `test/contract/journey_emit_args_contract_test.go` so gatherers execute a real retrieval tool call and synthesizers cite its `tool_result_ref`, `locator`, and verbatim `quote` verified through `emitartifact.VerifyDerivation`.

## Non-goals

- LLM-based claim-entailment judging inside `emit_research_artifact` (semantic evaluation remains `reviewer-research`'s responsibility).
- Replacing `decide(action="synthesize", reason=...)` with a separate typed gatherer terminal tool: `decide` is registered as a frozen built-in terminal tool in `github.com/c360studio/semstreams` (`8b99efe`) and rule `02-gather-complete-record.json` keys on `agent.loop.decision == "synthesize"`. Because `emit_research_artifact` verifies citations directly against `AGENT_TRAJECTORIES` + `AGENT_CONTENT` produced by the gather loops in the run (never trusting the gatherer's `decide.reason` summary as ground truth), gatherer handoff enforces structured JSON inside `decide.reason` at the persona layer while mechanical verification inspects the underlying tool execution records directly.
- New retrieval adapters (`semsource` adapter is tracked in `#272`; `github_read` landed in `#273`).
- Wiring the `program-report` rule pack (`#267`).

## Acceptance and risks

- `emit_research_artifact` deterministically rejects missing `evidence`, invalid `channel`/`source_class`, non-canonical `origin_id`, unknown or cross-run `tool_result_ref`, non-retrieval or failed tool calls, quotes echoed from dispatch arguments, invented `locator`s or cross-block quote/locator mismatches, revision spoofing, and fabricated `quote` spans not present verbatim in the retrieved body.
- Revision 2+ artifacts pass mechanical derivation when retaining revision-1 citations for unchanged claims and citing revision-2 gather tool results for updated claims.
- Mock-LLM research fixtures and contract tests pass schema validation and mechanical derivation end-to-end.

