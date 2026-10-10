# Frozen per-spawn loop containment

## Boundary and plan review

Base: SemTeams `d19c3014394935997cbbe59448ab451f2ef32dc4`; frozen SemStreams
`8b99efe9c66a4faa4fa509f9f62cc6bad8392128`. ADR-060 requires spawn-level ceilings; ADR-061 assigns
pack policy here and generic durability to SemEngine. Independent architecture review approved this bounded slice.

The existing `rule.Action.LoopMaxIterations` is a string resolving to a positive integer. The action executor writes
`TaskMessage.MaxIterations`; loop intake takes the smaller positive task limit and component ceiling. This is distinct
from the rule evaluator's per-entity `max_iterations`. No new public contract or storage is introduced.

## Qualification

First record a failing regression for currently omitted budgets. Decode live rules through frozen public types,
execute their spawn actions and inspect the canonical task. Qualify public loop intake and component-ceiling narrowing.
Exercise iteration exhaustion using a supported public boundary or real component fixture; do not copy upstream-only
carrier test hooks. Any direct guard fixture must be labeled separately from a full multi-turn journey.

The research entry rule is hashed in `test/fixtures/rule-family-consumer/provenance.json`. Preserve the hash check,
refresh provenance and explicitly classify the new field in that fixture's projection.

## Delivery limits

Conservative role budgets need room for reads, artifact emission and a terminal action. Fan-out is not yet schema-bounded,
so per-loop budgets cannot promise all widths complete. Rule-driven fresh loops do not share a proved total-run counter.
Clarification remains coordinator recovery. Existing routing, tools, actions and run inheritance remain unchanged.
Evidence/typed handoffs (#271), fan-out/run-wide/scheduling work (#272) and deep research (#274) remain separate.

## Implemented checks and observed semantics

`TestResearchSpawnLoopBudgets` first failed on all 17 existing spawns because their explicit cap was absent.
After adding the declared role ceilings, it passes decoded, unchanged live actions through the public action executor,
including three actual fan-out children, then checks canonical task values and public handler intake at component
ceilings 50 and 5. Its non-loop trigger intentionally excludes rule condition evaluation, run minting/inheritance,
transport and tool resolution. The existing E2 tool-choice fixture explicitly omits the budget; its source hash remains
strict, while this new test owns the cap assertion.

`TestResearchLoopBudgetTerminalBoundary` composes real graph-ingest and loop components over isolated NATS 2.14.4,
reusing the existing product fixture configuration helper and truthful external-publisher markers. It changes only
the loop ceiling to 2 in that fixture, sends a canonical task cap of 12 and observes an effective birth budget of 2.
A fixture terminal tool result succeeds below the limit. Two nonterminal tool rounds lead to request 3, whose
completion-looking response produces `LoopFailedEvent` with reason `max_iterations` and iteration count 2.
This is the frozen response-side guard: it is **not an exact provider-call or dollar-cost ceiling**. No additional
counter or workaround is introduced here. Model/tool responses are deterministic fixtures; actual tools, model
providers, rule matching, run-level outcomes and user delivery are not exercised by this integration test.
It checks the first terminal event; post-failure replay and duplicate-terminal absence remain unqualified.

The red command was `go test ./test/contract -run '^TestResearchSpawnLoopBudgets$' -count=1` before adding fields.
The focused green command includes `TestRuleFamily` to retain the source/projection guard. Integration runs with
`go test -tags=integration ./cmd/semteams -run '^TestResearchLoopBudgetTerminalBoundary$' -count=1`.
The test source preserves the reproduction; temporary logs are not a substitute for passing branch checks.

## Local validation and independent review

All nine required local gates passed: `task lint`, `task test:race`, `task test:integration`, `go build ./...`,
`task schema:generate`, `task schema:check-changes`, `task openspec:validate`, `task openspec:queue-test` and
`task publish:verify-test`. Schema regeneration produced no generated changes. Independent Go review found no
blockers; its evidence-wording correction is applied above and in the terminal assertion. These are local results,
not post-merge or paid-provider qualification. Source commit provenance and archived-spec review follow as delivery
checks.
