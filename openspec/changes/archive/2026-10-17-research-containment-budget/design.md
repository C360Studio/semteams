# Design: Research Containment Budget Completion (#272)

## D1. Shared Fan-Out Policy (`1..4` Subtopics)

The research pack bounds each planning pass to `1..4` subtopics (`MinSubtopics = 1`, `MaxSubtopics = 4` in `cmd/semteams/research`):

- `N = 1` handles non-decomposable questions with a single gatherer.
- `N <= 4` bounds concurrent `researcher-research-gather` dispatches per planner pass so downstream `researcher-research-synthesize` can read all sibling gather results and verify citations within its 24-iteration budget.

Both `emit_plan` (`epics`) and `decide` (`subtopics`) consume the same `research.MinSubtopics` / `research.MaxSubtopics` constants so the schema and runtime validators cannot drift.

## D2. Pre-Mutation `decide` Guard via `SkipBuiltins`

Frozen SemStreams (`v1.0.0-beta.162.0.20260930150212-8b99efe9c66a`) supports `SkipBuiltins: []string{"bash", "decide"}` in `executors.RegisterBuiltins`. SemTeams already uses `SkipBuiltins` to wrap `bash` (`cmd/semteams/tools/chainbash`).

We add `cmd/semteams/tools/researchdecide`, wrapping `agentictools.NewDecideExecutor(triplePublisher, platform, restrictedActions)`:

1. **Schema (`ListTools()`):** Clones the inner `DecideExecutor.ListTools()` definition and sets `"minItems": 1, "maxItems": 4` on `Parameters.properties.subtopics`. Because `subtopics` is not in `required`, non-fan-out `decide` calls (coordinator, synthesizer, reviewer, autoresearch, ops) remain unaffected by the array cardinality constraint, while any role populating `subtopics` sees the structural `1..4` bound in `task.Tools`.
2. **Pre-mutation admission (`Execute()`):**
   - Before delegating to `inner.Execute(ctx, call)` (and therefore before any Prometheus SAP counter increment or `publisher.Append` graph mutation), `researchdecide.Executor` inspects `call`:
   - If `call.Arguments["subtopics"]` is present and non-nil on any call, it must be a slice of non-blank strings with `1 <= len <= 4`.
   - When the resolved canonical action normalizes to `"gather"` (using the same trim/lowercase/hyphen-to-underscore normalization against the per-spawn allowlist, after verifying the action is allowed and not deployment-restricted), `subtopics` is **required** and must contain `1..4` non-blank string entries.
   - On violation, `Execute` returns `agentic.ToolResult{CallID: call.ID, Name: call.Name, Error: ..., ErrorKind: agentic.ToolErrorInvalidArgs, StopLoop: false}` with `nil` error. No triples are written to the planner loop entity, rule 02 does not fire, and the planner receives the diagnostic tool result in the same loop so it can re-emit a compliant `decide(action="gather", subtopics=[...])` within its `loop_max_iterations` budget.
3. **Rule 02 defense-in-depth:** `configs/rules/research/02-plan-to-gather.json` adds a `coordinator.decision.subtopics` `regex` condition matching a 1..4 element JSON string array (since `decide` stores `coordinator.decision.subtopics` as a single JSON-encoded `[]string` triple) alongside `03b-synthesize-when-all-gathers-complete.json`'s existing `length_gt: 0` guard.

## D3. Cron Cooldown, Inflight Guard, and Pattern-B Kill Switch

- **Contract invariant:** Any `type: "cron"` rule in live rule packs must set a valid positive `cooldown` duration and explicit `loop_max_iterations` on any `publish_agent` action.
- **Runtime verification over isolated NATS 2.14.4:**
  - **Inflight & cooldown:** When a scheduled rule's action is in-flight across a tick boundary (or within its `cooldown` window), `CronScheduler` skips the tick (`inflight_skipped` / `cooldown_skipped`) and does not stack concurrent dispatches.
  - **Pattern-B kill switch:** When a running cron rule is updated to `enabled: false` via `ruleManager.SaveRule(ctx, ruleID, disabledDef)`, the `rules.*` KV watch reconciles into `Processor.ApplyConfigUpdate`, deregisters the cron entry without restarting the process, and prevents any subsequent tick from firing. Already-dispatched work continues under its own lifecycle/iteration budget; schedule disablement is a future-tick gate, not retroactive cancellation.

## D4. Per-Entity Rule Counter vs. Run-Wide Recovery

`configs/rules/research/05-reviewer-rejected-retry.json` sets top-level `"max_iterations": 3`. Because `StatefulEvaluator` tracks state per `(ruleID, entityID)` and each `reviewer-research` loop is a distinct entity (`*.*.agentic-loop.agent.execution.<uuid>`), repeated reviewer rejections across fresh reviewer loop entities each evaluate as `iteration = 1` for that entity. A contract/behavioral test pins this exact distinction and documents that whole-run retry/cost ceilings belong to upstream `semstreams#383`, `semstreams#1005`, and `semengine#24`.
