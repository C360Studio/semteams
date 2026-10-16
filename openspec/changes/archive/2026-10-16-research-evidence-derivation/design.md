# Design: Evidence Contract and Mechanical Derivation Check (`#271`)

## D1. Claim-Level Evidence Contract (`cmd/semteams/research`)

- `research.Actor` and `research.IntegrationPoint` gain `Evidence []EvidenceRecord` (`json:"evidence"`).
- `tasks` remains `[]string` (preserving existing UI `ArtifactCard.svelte` and task-decomposition consumers), and `open_gaps []string` remains the explicit channel for surfacing unavailable or missing evidence (`needs_test_harness: ...`, `evidence_unavailable: ...`).
- `Artifact.Validate()` enforces that every `Actor` and `IntegrationPoint` carries at least one `EvidenceRecord`, and validates each record via `EvidenceRecord.ValidateArtifactCitation()` plus `ev.Revision <= a.Revision`.

## D2. Mechanical Derivation Verifier (`cmd/semteams/tools/emitartifact`)

- `emitartifact.ToolResultResolver` abstracts resolving a cited `tool_result_ref` for a synthesizer loop (`ResolveToolResult(ctx, callerLoopID, toolResultRef) (ResolvedToolResult, error)`):
  - **Runtime implementation (`NATSToolResultResolver`):**
    1. Resolves the calling synthesizer loop's `agent.run.entity-id` from `ENTITY_STATES` (or `call.Metadata["agent.run.entity-id"]`).
    2. Enumerates loop entities in the same run (matching `agent.run.entity-id` in `ENTITY_STATES`) and inspects their recorded tool completions in `AGENT_TRAJECTORIES` (`TrajectoryKindToolCompleted`, matching `SourceCorrelation == toolResultRef` or `<loop_id>/<call_id>` or `ExecutionID`) + `AGENT_CONTENT` objectstore (with fallback to `TOOL_CALL_OUTCOMES` KV bucket).
    3. If a `tool_result_ref` belongs to a loop from a different `agent.run.entity-id` or does not exist in the current run, returns an invalid-args derivation error.
  - **Derivation rules (zero LLM):**
    1. **Locator / origin match:** Either `ev.Locator` or `ev.OriginID` (`NormalizeOriginID(ev.Channel, ev.Locator)`) must appear in the resolved tool result's `Content` or serialized `DispatchArguments`.
    2. **Verbatim quote match:** `ev.Quote` must be a non-empty verbatim substring (`strings.Contains`) of the retrieved body (`RawBody` when the tool result is a structured `github_read` record matching `ev.OriginID`, or the raw tool result `Content` for `web` / `semsource` / generic retrieval tools).
    3. **Revision-2+ continuity:** Because all plan/gather/synthesize loops across revisions of the same run share `agent.run.entity-id`, unchanged claims retaining `revision: 1` citations resolve against revision-1 gather tool results in the same run, while claims with `revision: a.Revision` must resolve against tool results from the current run.

## D3. Deterministic Mock-LLM Tool Call IDs and Research Journey Fixtures

- `test/e2e/mock/fixture.go` (`FixtureToolCall`) adds optional `ID string` (`yaml:"id,omitempty"`). When set, `OpenAIServer.buildFixtureResponse` uses `entry.ToolCall.ID` instead of a random UUID suffix.
- `test/fixtures/journeys/research-mvp.yaml`, `research-fanout.yaml`, and `ops-run-terminal.yaml` include a deterministic retrieval tool call on the `researcher-research-gather` bucket before `decide(action="synthesize")`, and cite that `tool_result_ref`, `locator`, `origin_id`, and verbatim `quote` in `emit_research_artifact`.
