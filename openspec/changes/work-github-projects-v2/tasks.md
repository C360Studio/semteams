# Tasks: `work-github-projects-v2`

- [x] 1. Implement read-only GitHub Projects v2 GraphQL hydration (`readProjectBoard`) in `ui/src/lib/server/work/githubSource.ts` for repositories declaring `project_board`, populating `status_options`, `pm_status`, and `priority` in `listItems()` and `getItem()` with `lookup: "partial"` degradation and issue-state column fallback when the GraphQL read fails.
- [x] 2. Add unit and service tests in `ui/src/lib/server/work/githubSource.test.ts` and `ui/src/lib/server/work/service.test.ts` covering Projects v2 column ordering, `pm_status` and `priority` hydration, `"No Status"` placement for unboarded issues, truncation reporting, and honest `lookup: "partial"` fallback when the GraphQL read fails.
- [x] 3. Run UI unit tests, linter, type-check, and production build.
