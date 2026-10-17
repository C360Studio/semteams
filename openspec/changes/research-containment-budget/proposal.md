# Complete research containment budget: planner fan-out ceiling, cron cooldown/inflight, and kill switch (#272)

## Why

[Issue #272](https://github.com/C360Studio/semteams/issues/272) is the final prerequisite blocking [#267](https://github.com/C360Studio/semteams/issues/267) (Program Pulse walking skeleton in milestone `v0.2.0`). [PR #311](https://github.com/C360Studio/semteams/pull/311) established explicit per-spawn `loop_max_iterations` across all 17 live `research`/`autoresearch` spawns. Three containment boundaries remain in #272:

1. **Planner fan-out ceiling (`1..4` subtopics):** `emit_plan` and `decide` currently advertise no `maxItems` bound on `epics`/`subtopics`, and `DecideExecutor` accepts an arbitrary-length `subtopics` list that `configs/rules/research/02-plan-to-gather.json` fans out via `for_each`.
2. **Scheduled cron `cooldown` + scheduler inflight guard + Pattern-B `enabled: false` kill switch:** scheduled dispatches must be protected from stacking fires when an execution is slow or wedged, and operators must be able to flip a cron rule's `enabled: false` through the Pattern-B `rule.ConfigManager` to stop future ticks without a process restart.
3. **Run-wide recovery qualification:** rule `05-reviewer-rejected-retry.json`'s `max_iterations: 3` is scoped per triggering entity (each fresh reviewer loop is a distinct entity), so run-wide recovery remains an upstream runtime/SemEngine contract ([`semstreams#383`](https://github.com/C360Studio/semstreams/issues/383), [`semstreams#1005`](https://github.com/C360Studio/semstreams/issues/1005), [`semengine#24`](https://github.com/C360Studio/semengine/issues/24)) rather than a persona counter or local ledger.

## What changes

- **Planner fan-out schema + pre-mutation admission (`1..4` subtopics):**
  - Define canonical research planner fan-out bounds (`MinSubtopics = 1`, `MaxSubtopics = 4`) in `cmd/semteams/research`.
  - Advertise `minItems: 1` and `maxItems: 4` on `emit_plan`'s `epics` array schema and enforce `1 <= len(Epics) <= 4` in `devviaspec.Plan.Validate()`.
  - Register a product-shell `decide` wrapper (`cmd/semteams/tools/researchdecide`) via `SkipBuiltins: ["bash", "decide"]` that advertises `minItems: 1, maxItems: 4` on the optional `subtopics` array property and enforces pre-mutation validation before delegating to the framework `agentictools.DecideExecutor`:
    - When `call.Metadata["agent.role"] == "researcher-research-plan"` and the resolved action is `"gather"`, require `subtopics` to be a string slice with `1 <= len(subtopics) <= 4`.
    - When `subtopics` is populated on any `decide` call, require `1 <= len(subtopics) <= 4`.
    - Reject violations with `agentic.ToolErrorInvalidArgs` and `StopLoop = false` prior to graph mutation so zero decision triples are written, zero gatherers are spawned, and the planner can self-correct within its existing loop iteration budget.
  - Add defense-in-depth `coordinator.decision.subtopics` `length_gt: 0` and `length_lte: 4` conditions on `configs/rules/research/02-plan-to-gather.json` and update `configs/personas/fragments/researcher-research-plan/` prose to point at the schema-enforced `1..4` subtopic bound.
- **Cron cooldown, inflight guard, and Pattern-B kill-switch qualification:**
  - Add contract coverage asserting any `cron` rule in live rule packs declares a valid positive `cooldown` and `loop_max_iterations` on any `publish_agent` action.
  - Add integration tests over isolated NATS 2.14.4 proving:
    - Scheduler `inflight` and `cooldown` guards skip overlapping or rapid ticks without stacking dispatches.
    - Flipping a running cron rule's `enabled: false` through Pattern-B `rule.ConfigManager.SaveRule` stops future firings without a process restart while leaving already-dispatched work untouched.
- **Per-entity vs. run-wide recovery qualification:**
  - Add a deterministic test proving that rule `05-reviewer-rejected-retry.json` evaluates `max_iterations: 3` per reviewer entity (so distinct reviewer loop entities each fire retry independently), explicitly recording run-wide retry enforcement as tracked upstream (`semstreams#383`, `semstreams#1005`, `semengine#24`).

## Non-goals

- Unfreezing or modifying `github.com/c360studio/semstreams`.
- Extracting `agentic-loop` or introducing a product-local retry ledger or persona call counter.
- Activating scheduled Program Pulse rules (owned by [#268](https://github.com/C360Studio/semteams/issues/268) after [#267](https://github.com/C360Studio/semteams/issues/267)).
