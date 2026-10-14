# Design: Work-Item Run Linkage (`work-item-run-linkage`)

## Context

When an operator selects a work item on `/work` and sends a prompt in `ChatBar`, `formatWorkScopeHeader()` (#315)
prepends `[Work item: owner/repo#number (Program / Project) — title]` (or `[Work item: owner/repo#number — title]`) to
the outbound coordinator message. The runtime records that message as the coordinator loop entity's
`agent.loop.description` fact. In addition, ADR-061 §3 and the work lens design reserve the `run.issue.ref` predicate
for explicit issue-to-run graph linkage.

## Decisions

### D1: `{ by: "work_item_ref", ref: string }` binding in `WorkSource`

- `LinkedRunBinding` in `ui/src/lib/server/work/source.ts` gains a third variant:
  `{ by: "work_item_ref"; ref: string }`.
- `createGithubSource().linkedRunBindings(ref)` returns:
  `{ lookup: "complete", bindings: [{ by: "work_item_ref", ref: "${ref.repository.owner}/${ref.repository.name}#${ref.number}" }] }`.
- `parseBinding()` in `ui/src/lib/server/work/fixtureSource.ts` also accepts `{ by: "work_item_ref", ref }` so fixture
  sets can exercise the same binding path.

### D2: Dual-path graph resolution (`run.issue.ref` + `[Work item: ...]` prompt scope)

- `resolveWorkItemRefBinding(reader, itemRef)` in `ui/src/lib/server/work/overlays.ts` runs two bounded `/graph/triples`
  queries in parallel (`limit = PROMPT_LISTING_LIMIT = 50`):
  1. `GET /graph/triples?predicate=run.issue.ref&object=${itemRef}&limit=50`
     - Subjects containing `.chain.agent.execution.` (`RUN_INFIX`) are direct run entity IDs.
     - Subjects containing `.agentic-loop.agent.execution.` (`LOOP_INFIX`) are candidate coordinator loop entities.
  2. `GET /graph/triples?predicate=agent.loop.description&limit=50`
     - Triples whose subject contains `LOOP_INFIX` and whose `object` text starts with a `[Work item: <ref>...]`
       header (`extractScopedWorkItemRef(description)`) matching `itemRef` case-insensitively are candidate coordinator
       loop entities.
- Candidate loop entities (deduplicated across both listings) are inspected via
  `GET /graph/triples?subject=${loopEntity}&limit=200` using the same coordinator role filter
  (`agent.loop.role === "coordinator"`) and own-run anchor selection (`agent.loop.run` / `agent.run.entity-id`) as
  `resolvePromptBinding()`.
- Direct run entity IDs from `run.issue.ref` that were not already resolved through a candidate coordinator loop are
  resolved via `resolveRun(reader, { runEntityId, coordinatorLoopId: null })`.
- **Honest degradation (`Overlay<T>`):**
  - If both listing reads fail, `resolveWorkItemRefBinding` returns a `failed` part.
  - If one listing read fails or either listing hits `PROMPT_LISTING_LIMIT` (`>= 50`), it records a `partial` part while
    still resolving any matched runs from the readable listing.
  - If both listings succeed under the limit but match no run (`runs.length === 0 && parts.length === 0`), it records
    `{ lookup: "partial", reason: WORK_ITEM_REF_UNMATCHED_REASON }` (`"no run matched run.issue.ref or work-scoped prompt"`),
    preserving the invariant that only an empty binding list produces a `complete` `"no linked run"` (`none`) overlay.
