# Proposal: Hydrate GitHub Projects v2 Status Options, `pm_status`, and `priority` in `githubSource.ts`

## Problem

`createGithubSource()` (`ui/src/lib/server/work/githubSource.ts`) currently reads repository issues, pull requests, and PR review comments via the GitHub REST API, but passes `statusOptions = []` and leaves `pm_status` and `priority` unset even when a repository in `SEMTEAMS_PORTFOLIO_PATH` declares `project_board` (`{ owner, number, status_field, priority_field? }`). As a result, a live repository with a configured `project_board` places every issue into `"No Status"` instead of its GitHub Projects v2 status column.

## What Changes

- **Read-only GitHub Projects v2 GraphQL hydration (`ui/src/lib/server/work/githubSource.ts`):**
  - When `repository.project_board` is configured, query `${apiBase}/graphql` (or `/api/graphql` for GitHub Enterprise `/api/v3` base URLs) with a bounded (`READ_TIMEOUT_MS = 5000`), read-only GraphQL `query` document for the configured `ProjectV2(number)`.
  - Extract the ordered option names of the configured single-select `status_field` (`status_options`) and each repository issue's `status_field` value (`pm_status`) and optional `priority_field` value (`priority`).
  - Hydrate `columnsFor(repository, statusOptions, ...)` and `columnFor({ state, assignees, pm_status }, repository)` in both `listItems()` and `getItem()`.
- **Honest degradation and fallback (`ADR-061 §3`):**
  - When the Projects v2 GraphQL read fails, times out, or cannot resolve the configured project or single-select `status_field`, record the reason so `lookup` degrades to `"partial"`, leave `pm_status` and `priority` unset, and fall back to the issue-state columns (`Todo`, `In Progress`, `Done`) rather than collapsing all cards into `"No Status"`.
  - When the Project's item connection is truncated at `GITHUB_PAGE_LIMIT` (`pageInfo.hasNextPage === true`), record the truncation reason so `lookup` reports `"partial"`.

## Capabilities

### Modified Capabilities
- `observable-work`: Adds the scenario for live GitHub Projects v2 `status_options`, `pm_status`, and `priority` hydration and honest `partial` fallback when the Projects v2 GraphQL read fails.

## Non-Goals

- No mutation of GitHub Projects v2 fields, issues, or pull requests.
- No change to `WorkItem`, `Repository`, or `ProjectBoard` wire types.
