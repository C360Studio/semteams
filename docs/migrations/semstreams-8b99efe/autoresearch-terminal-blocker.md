# Autoresearch terminal route failure and qualified repair

**Resolved in the scoped mock qualification.** The independently reviewed SemTeams origin-return repair passes all
five focused browser cases: approved result, rejected metric win, descended `ask_user`, descended `respond_direct`
and non-budgeted loop failure. The accepted full expanded matrix passes all 24 active scenarios with five explicit
skips and no source changes;
see the [browser report](browser-baseline.md). The frozen SemStreams module is unchanged.

The historical frozen journey completed its optimization chain and final coordinator decision but published no typed
terminal result to the user's channel. This was a SemTeams lineage defect exposed by the strengthened delivery gate,
not evidence that the closed upstream #1094 change regressed. The original red evidence below remains intact.

## Historical reproduced evidence

The submission root is `64158ebb-2f1c-434c-b88d-eb8f487b5529`, user `ui-anonymous`, channel type `http`, channel ID
`http-1790856197506307379`. Its `autoresearch` decision is correctly settled as a handoff, not shown as the answer.
The baseline child has this root as both typed parent and run anchor.

Rule `autoresearch_iteration_dispatch` fires on the run entity. Its synthesize task has no `parent_loop_id`,
`run_id`, or channel fields, although `agent.related_loops.autoresearch-run` retains the root as product metadata.
The final durable ancestry is:

- Coordinator `c5795807-6b92-4c64-bf90-134942ac49e2`, decision `respond_direct`.
- Reviewer `ec8bb9c9-e6b0-4cfd-9b39-85e9e4066ceb`.
- Synthesize `511ab881-bc70-4726-b8c4-4aabf355c9c3`; no typed parent or run anchor.

The captured message logger contains the root submission `status` response and no terminal result.
[Exact selected task/completion/response envelopes](autoresearch-route-evidence.json) retain correlation and
ancestry fields; absent fields were absent on the wire, not inferred from UI display.
Full local capture: `/tmp/semteams-migration-8b99efe/evidence/target-browser/autoresearch/`.

## Historical contract attribution

At frozen SHA `8b99efe9c66a`, `processor/rule/actions.go:1799-1886` sets the parent only for a loop-entity trigger
and inherits `RunID` only from the firing entity's `agent.loop.run` triple. The authored run-triggered rule carries
product metadata instead. There is no supported action field for an explicit parent/run override in that source.

`processor/agentic-dispatch/terminal_settlement.go:393-397` explicitly classifies an ancestry chain severed by a
non-loop trigger as `route_less_settled`. Its resolver at lines 427-452 follows durable typed `RunID` and
`ParentLoopID` records in `AGENT_LOOPS`; it does not interpret category-specific `related_loops` metadata.

The repair adds no response publisher shim, invented loop predicate on the run entity or runtime primitive. It
changes only authored product facts and rules using the frozen framework's supported loop-triggered publication.
The old failure remains in the [26-scenario historical matrix](browser-evidence/final/results.json); it must not be
relabeled as a passing run. The new [focused results](browser-evidence/repair-focused/results.json) qualify the repair.

## Rejected iteration-driver rewrite

The architect's pre-repair check identified the failure as authored SemTeams wiring, not an unavoidable upstream
blocker. Frozen `processor/rule/actions.go:731-753` already stamps the persistent root coordinator's typed run anchor;
a loop-triggered dispatch can therefore preserve ancestry using existing primitives. The implemented repair uses that
path
independently of SemEngine and SemSource.

Simply changing rule 05 to fire on each baseline/execute loop is not equivalent:

- `processor/rule/stateful_evaluator.go:110-115,183-186` keys rule state and its iteration counter by firing entity.
  Every newly spawned loop would restart the counter instead of preserving the run's cap.
- `emitautoresearchbaseline/executor.go:131-138,237-257` seeds cap, best, status and pending state on the run.
  Frozen `processor/rule/expression_factory.go:288-303` substitutes from the firing entity/lifecycle context;
  `related_loops` metadata does not make the run snapshot available to those conditions and prompts.
- Rules 04a/04b account for clean and failed executions separately; 04c independently promotes the kept best result.
  A replacement must preserve that accounting, the stop latch, and the ordering of state used for the next dispatch.

## Qualified origin-return contract

[ADR-053](../../adr/053-adoption-plan.md#addendum-2026-10-01--autoresearch-returns-through-its-run-origin)
records the architect and independent review decision before implementation. Rule 05 and its run-local state remain
in place. Successful reviewer approval, descended clarification and non-budgeted loop failure append separate facts:
`autoresearch.reply.approved`, `autoresearch.reply.clarification` and `autoresearch.reply.failed`. Each value is the
full source-loop entity ID, declared with the existing entity-ID vocabulary type.

The run bridge requires exactly one source and origin, the same authority and instance in the canonical origin ID,
and the corresponding phase: completed, executing or failed. It forwards the fact to the actual existing origin
coordinator. That loop publishes the return coordinator with `run_scope: new`: the existing framework Mint path
idempotently retrieves the same origin-rooted run and preserves its lifecycle phase. The child receives native
`ParentLoopID` and `RunID` equal to the root, even when an earlier inherited anchor is first in the origin's graph.
It can therefore resolve the user's route through supported ancestry. No subscriber, queue, store, tool, response
shim, forged framework predicate or future upstream change is required.

The repeating clear/propose actions in rule 05 explicitly use `max_iterations: 0`. The new cap-five behavioral test
first reproduced the frozen default-three per-action limit at iteration four. Disabling that separate limit leaves
the existing run cap authoritative; synthesis/stop remain their existing terminal branch. Execute accounting,
measurement comparison, best promotion and the stop latch are unchanged.

[Go red/green evidence](go-validation.md#autoresearch-terminal-repair) exercises actual frozen match/action/Mint/
HandleTask paths and durable rule-state replay. The [independent Go review](autoresearch-repair-review.md) approves
origin guards, unambiguous source anchors, run reuse, terminal-child admission and the narrow registration fence.
The [independent frontend review](autoresearch-repair-frontend-review.md) approves the assertions without authoring
those tests. Focused browser evidence uses the actual graph owner and loop runtime, confirms original channel/user
correlation, and inspects the typed envelope through the existing UI.

## Preserved limits

- `ask_user` puts the run into awaiting approval. A limitation `respond_direct` intentionally leaves it executing,
  as before. That response does not claim completed optimization or qualify a later continuation.
- Direct Mint refuses a stored-origin mismatch. The frozen rule action's error fallback still publishes a task
  with its parent and no `RunID`; this is not a stronger fail-closed publication guarantee. Authored run guards
  reject malformed/foreign/ambiguous origins before forwarding, and native parent ancestry remains supported.
- Persisted match state suppresses observed duplicate/stale delivery and recreation of the evaluator. Publication
  and match-state persistence are not a cross-entity transaction, so crash-window exactly-once delivery is not claimed.
- The run-phase guard suppresses cancellation before clarification admission. Cancellation after action selection
  retains the existing rule-engine ordering boundary.
- Mock fixtures prove routing and tool/graph outcomes, not live-model judgment. Existing rule03 best-value and rule05
  scalar-journal-length prompt defects remain documented in the
  [paired comparison](browser-evidence/repair-focused/prompt-warning-comparison.json).
- Raw typed-envelope inspection does not implement evidence-body rendering or close
  [#261](https://github.com/C360Studio/semteams/issues/261). SemSource dogfooding and a later SemEngine consumer
  contract remain independent; this repair does not expand SemEngine's first release.
