# Design: Live GitHub WorkSource with Baton, Review, and Decision Gate Derivation (`#319`)

## Context

`ui/src/lib/server/work/service.ts` already supports `WORK_SOURCE=github` with `SEMTEAMS_PORTFOLIO_PATH`, validating the portfolio JSON document before delegating to `createGithubSource()`. Until now, `createGithubSource()` returned `lookup: "unsupported"`.

To support multi-repository coordination across `C360Studio/semteams`, `C360Studio/semengine`, and `C360Studio/semstreams` without violating the read-only boundary or `Overlay<T>` honesty rules:
1. All GitHub reads must happen server-side inside `/api/work/*` (`GET` only, bounded by `READ_TIMEOUT_MS = 5000`), never exposing `GITHUB_TOKEN` to the browser.
2. GitHub-owned columns (`Todo`, `In Progress`, `Done` when no Project board is configured, or Project status options when present) must never be derived from baton, review, or gate overlays.
3. Failed, rate-limited, or truncated GitHub API reads must surface as `lookup: "failed"` or `"partial"` with a human-readable reason, never as a `complete` empty board.

## Decisions

### D1: Read-Only Server-Side GitHub REST Client (`ui/src/lib/server/work/githubSource.ts`)

`readWorkConfig()` in `ui/src/lib/server/work/config.ts` reads two optional environment variables in addition to `WORK_SOURCE` and `SEMTEAMS_PORTFOLIO_PATH`:
- `GITHUB_TOKEN`: optional bearer token sent in `Authorization: Bearer <token>` on server-side GitHub requests. Never echoed in error responses.
- `GITHUB_API_URL`: optional base URL defaulting to `https://api.github.com` so unit and route tests can bind a local mock server deterministically.

Every GitHub request sets `Accept: application/vnd.github+json`, `X-GitHub-Api-Version: 2022-11-28`, and a 5-second `AbortController` timeout (`READ_TIMEOUT_MS`).

### D2: Issue, Pull Request Claim, and Comment Derivation

For a configured `repository` (`owner/name`):
1. **`listItems(repository)`:**
   - Queries `GET /repos/{owner}/{name}/issues?state=open&per_page=100` and `GET /repos/{owner}/{name}/pulls?state=all&per_page=100` in parallel.
   - Filters `/issues` to true issues (excluding entries with a `pull_request` property).
   - Collects open `class:flake` issue references (`owner/name#number`) as repository-wide `flake_holds`.
   - Matches pull requests from `/pulls` to issues via:
     - Closing keywords in PR body (`(?:close[sd]?|fix(?:e[sd])?|resolve[sd]?)\s+(?:([a-z0-9_.-]+\/[a-z0-9_.-]+)?#(\d+))` case-insensitive), or
     - Issue reference in PR title (`#(\d+)`).
   - Extracts `implemented-by`, `reviewed-by`, and `stop-point` trailers/lines from each PR body, and `blocked-by` / task-list dependencies from each issue body.
   - For open claiming pull requests (up to `MAX_OPEN_PR_COMMENT_FETCHES = 10` per repository), fetches `GET /repos/{owner}/{name}/issues/{prNumber}/comments?per_page=100` to parse `<!-- semteams-review-request -->` and `<!-- semteams-review-record -->` comments. If a PR's comments are not fetched (or the comment fetch fails/truncates), sets `review_inspected: false` on that `PullRequestRef` so `deriveReviewOverlay()` honestly returns `{ state: "unknown", reason: ... }` instead of `not_requested`.
2. **`getItem({ repository, number })`:**
   - Queries `GET /repos/{owner}/{name}/issues/{number}` (returning `{ lookup: "complete" }` on 404 when the issue does not exist, or when the number belongs to a pull request rather than an issue), `GET /repos/{owner}/{name}/issues?state=open&labels=class:flake&per_page=100`, and `GET /repos/{owner}/{name}/pulls?state=all&per_page=100`, then fetches comments for any open claiming PRs of that issue.
3. **Short-SHA Matching in `deriveReviewOverlay()`:**
   - `deriveReviewOverlay()` treats a review record or review request SHA as matching `pr.head_sha` when `rec.sha === pr.head_sha` or (for 7+ character short SHAs written in review comments) `pr.head_sha.toLowerCase().startsWith(rec.sha.toLowerCase())`.
   - When `pr.review_inspected === false`, `deriveReviewOverlay()` returns `{ state: "unknown", reason: "PR review comments not inspected" }`.

### D3: C360 Multi-Repo Portfolio Configuration (`configs/portfolio-c360.json`)

Provide `configs/portfolio-c360.json` defining the `c360` program with `SemTeams` (`C360Studio/semteams`), `SemEngine` (`C360Studio/semengine`), and `SemStreams` (`C360Studio/semstreams`) projects so operators can launch `/work` in live GitHub mode immediately.
