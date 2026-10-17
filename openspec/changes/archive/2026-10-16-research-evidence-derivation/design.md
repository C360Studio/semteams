# Design: Evidence Contract and Mechanical Derivation Check (`#271`)

## D1. Claim-Level Evidence Contract (`cmd/semteams/research`)

- `research.Actor` and `research.IntegrationPoint` gain `Evidence []EvidenceRecord` (`json:"evidence"`).
- `tasks` remains `[]string` (preserving existing UI `ArtifactCard.svelte` and task-decomposition consumers), and `open_gaps []string` remains the explicit channel for surfacing unavailable or missing evidence (`needs_test_harness: ...`, `evidence_unavailable: ...`).
- `Artifact.PopulateCitationDefaults()` server-populates omitted `origin_id` (`NormalizeOriginID(channel, locator)`), `retrieved_at` (from `produced_at`), and `revision` (from `a.Revision`).
- `Artifact.Validate()` enforces that every `Actor` and `IntegrationPoint` carries at least one `EvidenceRecord`, and validates each record via `EvidenceRecord.ValidateArtifactCitation()` (requiring `MinQuoteLength = 12` after whitespace normalization, canonical `origin_id`, and valid `kind` when set) plus `ev.Revision <= a.Revision`.

## D2. Mechanical Derivation Verifier (`cmd/semteams/tools/emitartifact`)

- `emitartifact.ToolResultResolver` abstracts resolving a cited `tool_result_ref` for a synthesizer loop (`ResolveToolResult(ctx, callerLoopID, toolResultRef) (ResolvedToolResult, error)`):
  - **Runtime implementation (`NATSToolResultResolver`):**
    1. Resolves the calling synthesizer loop's `agent.run.entity-id` from `ENTITY_STATES` (using `agvocab.LoopRunEntityID` / `agvocab.LoopRun`).
    2. Discovers candidate loop IDs in the same run via graph lineage (`call.LoopID`, `agent.lineage.researcher-plan`, multi-valued `research.gather.completed-subtopic` via `MultiEntityTripleReader.ReadEntityAll`, and `agent.lineage.researcher` across revision chains) — never performing a full-bucket scan of `ENTITY_STATES` or `AGENT_TRAJECTORIES`.
    3. Queries `AGENT_TRAJECTORIES` using NATS JetStream server-side subject filter `agentic.TrajectoryFactPrefix(loopID) + ">"` (`natsclient.FilteredKeys`) for paired `TrajectoryKindToolDispatched` and `TrajectoryKindToolCompleted` facts matching `SourceCorrelation == toolResultRef`, `<loop_id>/<call_id>`, or `ExecutionID`, and fetches the content envelope from the `ObjectStore` bucket named by `TrajectoryFactV1.Evidence.StorageInstance` (via `WithStorageBuckets`, defaulting to `AGENT_CONTENT`), verifying `sha256(rawBytes) == fact.EvidenceDigest`.
    4. Classifies NATS KV/ObjectStore/`EntityReader` lookup failures as `agentic.ToolErrorNetwork`, corrupt trajectory/envelope JSON or digest mismatches as `agentic.ToolErrorInternal`, and missing/cross-run/invalid citations as `agentic.ToolErrorInvalidArgs`.
  - **Derivation rules (zero LLM):**
    1. **Channel-scoped retrieval tool allowlist & execution status:** The resolved tool must not have `Status == "failed"` or non-empty `Error`/`ErrorKind`, and `ToolName` must be in the allowlist for `ev.Channel` (`github_read`, `http_request`, `web_search`, `graph_query`/`graph_search`/`query_entity`/`query_relationships`/`semsource_read`, or `bash` only when the command invokes `curl`/`wget`/`gh`). Non-retrieval tools (`read_loop_result`, `scratchpad`, `decide`, `bash echo/cat`, etc.) are rejected.
    2. **Anti-echo check:** `ev.Quote` must not appear in the tool call's `DispatchArguments`.
    3. **Same-block / same-element locator & quote co-occurrence:** Both the URL/token-bounded `ev.Locator`/`ev.OriginID` and verbatim `ev.Quote` must co-occur in the same `[N]` result block (for `web_search`, which also stamps `ev.Kind = "snippet"` and rejects `kind == "body"`), the same structured `github_read` item (matching `OriginID` and `RawBody`), or the same JSON array element / single-resource body.
    4. **Revision attribution & continuity:** When the producing loop's revision (`research.plan.revision` / `research.artifact.revision`) is known, `ev.Revision` must equal `res.ProducerRevision`. Because all plan/gather/synthesize loops across revisions of the same run share `agent.run.entity-id` and `03b-synthesize-when-all-gathers-complete.json` threads `prior_synthesize_loop_id`, revision 2+ synthesizers can retain `revision: 1` citations from revision-1 gather loops while citing `revision: 2` tool results for updated claims.

## D3. Deterministic Mock-LLM Tool Call IDs and Research Journey Fixtures

- `test/e2e/mock/fixture.go` (`FixtureToolCall`) adds optional `ID string` (`yaml:"id,omitempty"`). When set, `OpenAIServer.buildFixtureResponse` uses `entry.ToolCall.ID` instead of a random UUID suffix.
- `test/fixtures/journeys/research-mvp.yaml`, `research-fanout.yaml`, and `ops-run-terminal.yaml` include a deterministic retrieval tool call on the `researcher-research-gather` bucket before `decide(action="synthesize")`, and cite that `tool_result_ref`, `locator`, `origin_id`, and verbatim `quote` in `emit_research_artifact` (verified by `test/contract/journey_emit_args_contract_test.go` via `emitartifact.VerifyDerivation`).

