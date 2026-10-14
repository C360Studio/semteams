# Change: Link Live Agentic Runs to Work Items via Prompt Scope and `run.issue.ref`

## Why

The `/work` lens (`#313`, `#314`, `#315`, `#319`) now renders live GitHub issues, pull requests, review batons, and
inline agentic run workbenches, and `ChatBar` prepends `[Work item: owner/repo#number — title]` headers to prompts
dispatched while viewing a work item. However, `createGithubSource().linkedRunBindings()` still returns
`lookup: "unsupported"` (`GITHUB_RUNS_UNSUPPORTED`), and `resolveLinkedRuns()` in `ui/src/lib/server/work/overlays.ts`
only resolves fixture-specific `run_entity_id` and exact `coordinator_prompt` bindings.

Issue #320 connects live work items to their agentic runs so that runs dispatched from `/work` via `ChatBar` (carrying
`[Work item: owner/repo#number ...]`) and runs carrying `run.issue.ref` graph triples resolve directly in `/work`
through `GET /api/work/items/{owner}/{repo}/{number}/runs`.

## What Changes

- **Work-item reference run binding (`ui/src/lib/server/work/source.ts`, `githubSource.ts`, `fixtureSource.ts`):**
  - Extend `LinkedRunBinding` with `{ by: "work_item_ref"; ref: string }`.
  - Update `createGithubSource().linkedRunBindings(ref)` to return `{ lookup: "complete", bindings: [{ by: "work_item_ref", ref: "${owner}/${name}#${number}" }] }`.
  - Allow fixture items in `fixtureSource.ts` to declare `{ by: "work_item_ref", ref: string }` bindings as well.
- **Graph resolution for `work_item_ref` (`ui/src/lib/server/work/overlays.ts`):**
  - Resolve `{ by: "work_item_ref", ref }` by querying `/graph/triples` for:
    1. `predicate=run.issue.ref&object=${ref}&limit=50` (matching run entities containing `.chain.agent.execution.` or
       coordinator loop entities containing `.agentic-loop.agent.execution.`), and
    2. `predicate=agent.loop.description&limit=50` (matching coordinator loop entities whose prompt starts with
       `[Work item: <ref>]` or `[Work item: <ref> — ...]`, case-insensitively on `<ref>`).
  - Deduplicate candidate coordinator loops and run entities so a run carrying both `run.issue.ref` and a work-scoped
    prompt is resolved once.
  - Preserve `Overlay<T>` (`known | none | unknown`) honesty: an unmatched `work_item_ref` binding degrades to
    `lookup: "partial"` with `WORK_ITEM_REF_UNMATCHED_REASON` (`"no run matched run.issue.ref or work-scoped prompt"`)
    rather than fabricating `"no linked run"`, because `/graph/triples` returns `[]` when unreadable and the frozen
    runtime has no mandatory `run.issue.ref` writer.

## Impact

- Affected specs: `observable-work`
- Affected code: `ui/src/lib/server/work/{source.ts,githubSource.ts,fixtureSource.ts,overlays.ts}` and tests
- Non-goals: No new runtime graph writer in frozen SemStreams, no write endpoints on `/api/work/*`.
