# Add structured baton, review, and decision overlays to observable work

## Why

[Issue #314](https://github.com/C360Studio/semteams/issues/314) extends the read-only observable-work slice
([#296](https://github.com/C360Studio/semteams/issues/296) / [PR #304](https://github.com/C360Studio/semteams/pull/304))
for multi-vendor (`claude`, `codex`, `gemini`) and multi-repository (`semengine`, `semteams`, `semdev`, `semsource`,
`semmem`) coordination. When a small team coordinates humans and multiple agent vendors across N repositories,
traditional Kanban columns (`Todo`, `In Progress`, `In Review`, `Done`) show where an issue sits in GitHub Projects,
but hide who currently holds the active baton, whether a cross-agent review record matches the PR's current `HEAD` SHA
or has gone stale after follow-up commits, and whether progress is gated on an owner decision (`status:needs-decision`),
a dependency blocker (`status:blocked`), a repository merge hold (`class:flake`), or an OpenSpec `tasks.md` caveat.

Per the repository's shared work protocol (`.agents/protocol.md`) and
[ADR-061](../../../docs/adr/061-agent-runtime-ownership-and-observable-work.md), SemTeams does not introduce a freeform
agent-to-agent chat bus or a shadow state store. Instead, it projects the authoritative protocol artifacts already
recorded in GitHub and OpenSpec onto `WorkItem` as explicit, provenance-backed `known | none | unknown` overlays.

This change starts from `main` at `f7c39e6a` (frozen SemStreams `8b99efe9c66a4faa4fa509f9f62cc6bad8392128`) on branch
`gemini/work-baton-overlays`.

## What changes

- Extend `WorkItem` and the `GET /api/work/*` server derivation with three structured protocol overlays, each carrying
  an explicit `Overlay<T>` (`known | none | unknown`) state:
  - **Builder / Claim Baton (`claim`):** identifies whether the work item is claimed by an agent draft/open PR
    (`claude/...`, `codex/...`, `gemini/...`), a human PR, or an issue assignee, along with the claiming PR reference,
    draft state, `implemented-by:` attribution, and declared stop-point when present.
  - **Review Baton (`review`):** compares the claiming PR's `HEAD` commit SHA against its latest structured
    `Review request` and `Review record` comments (`not_requested`, `requested`, `current`, `stale`,
    `changes_requested`), surfacing stale reviews when commits land after a review record and showing `reviewed-by:`
    and reviewer-lane attribution.
  - **Decision & Hold Gate (`gate`):** aggregates owner-docket holds (`status:needs-decision`), explicit issue/dependency
    blockers (`status:blocked`, `Blocked by #n`), repository-wide required-job flake holds (`class:flake`), and
    unticked OpenSpec `tasks.md` caveats (`HALT`, `RED`, `BLOCKED`, `WONTDO`, `OPEN-Q`).
- Add a **baton filter lens** (`?baton=all|needs-decision|claimed|review|blocked|ready-to-land`) on `/work` so operators
  can slice portfolio work items across both board and table views without ever mutating or overriding GitHub-owned
  column placement.
- Render the `claim`, `review`, and `gate` overlays as separate badges on `WorkItemCard` and `WorkTable` rows and as
  structured provenance sections in `WorkItemOverview`.
- Extend the `board-mvp` fixture set and contract/unit tests to cover agent branch claims, current vs. stale
  `Review record` SHA comparisons, `status:needs-decision`, `class:flake`, and OpenSpec `Hold:` caveats.

## Non-goals

- No freeform agent-to-agent chat channel or shadow coordination database outside GitHub, OpenSpec, and the runtime
  graph.
- No write actions from `/work`: no claiming issues, posting review comments, changing labels, or merging PRs (#299
  owns governed write authority).
- No derivation of Kanban column placement (`column` or `pm_status`) from baton, review, or gate overlays.
- No live GitHub GraphQL/REST transport implementation in this slice (#273 owns the live GitHub read adapter; this
  change defines the normalized source contract that #273 and #267 Program Pulse share).
- No inline agentic drawer or scoped chat reorganization on `/work` (#315 owns the unified drawer and scoped chat UI).

## Impact

- Updates `openspec/specs/observable-work/spec.md` with structured baton, review-parity, decision/hold gate, and baton
  filtering requirements.
- Extends `ui/src/lib/types/work.ts`, `ui/src/lib/server/work/fixtureSource.ts`, `ui/src/lib/server/work/fixtures/board-mvp/items.json`,
  and `/work` UI components (`WorkItemCard.svelte`, `WorkTable.svelte`, `WorkItemOverview.svelte`, `+page.svelte`).
- Zero Go runtime changes; no new component, rule action, payload type, KV bucket, or stream.
