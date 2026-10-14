# Design: GitHub Projects v2 Read-Only Hydration in `githubSource.ts`

## Context

In `ui/src/lib/types/work.ts`, a portfolio repository may declare:
```ts
export interface ProjectBoard {
  owner: string;
  number: number;
  status_field: string;
  priority_field?: string;
}
```
GitHub Projects v2 stores single-select status options (`Todo`, `In Progress`, `In Review`, `Done`, etc.) and per-item field values in GraphQL v4 (`ProjectV2`) rather than on the REST v3 `/repos/{owner}/{repo}/issues` resource.

## Decisions

### D1: Bounded, Read-Only GraphQL Query in `githubSource.ts`

- When `repository.project_board` is present, `createGithubSource()` issues a `POST` request to the GraphQL endpoint (`${apiBase}/graphql`, or `/api/graphql` when `apiBase` ends with `/api/v3`) carrying a read-only `query` document (`query SemTeamsProjectBoard`) bounded by `READ_TIMEOUT_MS` (`5000 ms`).
- The query inspects `organization(login: $owner).projectV2(number: $number)` and `user(login: $owner).projectV2(number: $number)`:
  - If either resolves a `ProjectV2` node, any `NOT_FOUND` GraphQL error from the other owner kind is ignored.
  - From `fields(first: 50)`, the single-select field matching `project_board.status_field` supplies `statusOptions` in option order.
  - From `items(first: 100)`, each item whose `content` is an `Issue` belonging to `${repository.owner}/${repository.name}` supplies `pm_status` (from `status_field`) and `priority` (from `priority_field`, when configured on `project_board`).

### D2: Honest `partial` Degradation and Issue-State Column Fallback

- If the GraphQL request fails, times out, returns no `ProjectV2` for `owner#number`, or lacks the configured single-select `status_field`:
  - The failure reason is appended to `reasons`, degrading the enclosing `listItems` or `getItem` `lookup` to `"partial"`.
  - Column assignment falls back to the issue-state mapping (`Todo`, `In Progress`, `Done`) with `pm_status` and `priority` left unset (`undefined`), preventing a failed project-board read from misplacing every repository issue into `"No Status"`.
- If the GraphQL read succeeds and an individual issue in the repository is not on the project board (or has an empty status field on the board), that issue receives `column: "No Status"` (`pm_status` unset) and `columnsFor` appends `"No Status"` after `statusOptions`, matching `fixtureSource.ts`.
