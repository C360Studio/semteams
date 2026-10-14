# Change: Implement live GitHub WorkSource with baton, review, and decision gate derivation

## Why

Following `#314` (`work-baton-overlays`) and `#315` (`work-inline-agentic-drawer`), the `/work` lens renders GitHub-owned columns alongside structured `BatonOverlay`, `ReviewOverlay`, and `DecisionGate` (`Overlay<T>` with strict `known | none | unknown` states) and an inline agentic run workbench. However, `createGithubSource()` in `ui/src/lib/server/work/source.ts` is still a stub returning `lookup: "unsupported"`, preventing operators from running `/work` against live GitHub repositories (`WORK_SOURCE=github`) across a multi-repo portfolio (`C360Studio/semteams`, `C360Studio/semengine`, `C360Studio/semstreams`).

## What Changes

- **Live GitHub `WorkSource` (`ui/src/lib/server/work/githubSource.ts`):**
  - Implements a read-only server-side GitHub REST adapter (`GET` only, bounded by `READ_TIMEOUT_MS = 5000`), configured via `GITHUB_TOKEN` (optional bearer auth) and `GITHUB_API_URL` (default `https://api.github.com`, configurable for deterministic tests).
  - Enumerates repository issues and pull requests (`listItems` and `getItem`), places issues into GitHub-owned columns (`columnsFor` / `columnFor`), links claiming pull requests (`Closes #n` / `Fixes #n` / `Resolves #n` in PR body or `(#n)` in PR title), extracts `implemented-by`, `reviewed-by`, `stop-point`, `blocked-by`, task-list dependencies, and `class:flake` repository holds, and inspects `<!-- semteams-review-request -->` / `<!-- semteams-review-record -->` PR comments to populate `deriveBatonOverlays()`.
  - Preserves `Overlay<T>` honesty: HTTP errors, rate limits, timeouts, or truncated listings degrade to `lookup: "failed"` or `"partial"` with an explicit reason (never `complete` with `[]`), and uninspected PR review comments report `review: { state: "unknown", reason: ... }` rather than `not_requested`.
- **Portfolio Configuration (`configs/portfolio-c360.json`):**
  - Ships a ready-to-use C360 portfolio configuration covering `C360Studio/semteams`, `C360Studio/semengine`, and `C360Studio/semstreams`.

## Impact

- Affected specs: `observable-work`
- Affected code:
  - `ui/src/lib/types/work.ts`
  - `ui/src/lib/server/work/baton.ts`
  - `ui/src/lib/server/work/config.ts`
  - `ui/src/lib/server/work/githubSource.ts`
  - `ui/src/lib/server/work/githubSource.test.ts`
  - `ui/src/lib/server/work/source.ts`
  - `ui/src/lib/server/work/service.ts`
  - `ui/src/lib/server/work/service.test.ts`
  - `configs/portfolio-c360.json`
- Issue: Closes #319
