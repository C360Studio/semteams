# Change: Inline Agentic Work Drawer and Work-Scoped Prompt Bar on `/work`

## Why

Today SemTeams splits the operator experience across two disconnected views: `/` renders raw `AgentLoopState` columns for ephemeral loops, while `/work` renders the trained `Program -> Project -> Repository` Kanban/Table view but forces operators to click "Open this run in the runs lens" (`/?task=<loopId>`) and leave `/work` to read a run's story, inspect evidence, or resolve a pause/approval. Meanwhile, `ChatBar` has no awareness of the selected `/work` item or project scope. Unifying the `/work` item drawer with an inline Enjoy-style agentic workbench and work-scoped prompt bar lets operators inspect run stories, review evidence, resolve blocking approvals/clarifications, and direct the coordinator in context without losing their place on the project board.

## What Changes

- **Inline Agentic Workbench in `WorkItemOverview.svelte` (`/work`):**
  - When linked runs are loaded for a selected work item and at least one linked run resolves a `coordinator_loop_id`, render an inline run workbench (`data-testid="work-inline-run-workbench"`) directly inside the `/work` drawer alongside the existing `work-drill-in` link.
  - Support switching between multiple resolved linked runs via a run selector bar (`data-testid="work-inline-run-selector"`).
  - Surface live `RunHealthPanel`, `PendingApprovalSection`, `RunWaitingSection`, sub-tasks, and rule-fired `Controls` (when the loop is present in `taskStore`), plus an in-drawer `Activity | Evidence` tab bar rendering `TaskStory` and `RunEvidencePanel` inline without leaving `/work`.
- **Work-Scoped Prompt & Context Chip (`workContext.svelte.ts` + `ChatBar.svelte`):**
  - Introduce `workContext` store tracking the active `/work` item and its program/project scope.
  - Display a work-item context chip (`data-testid="work-chat-context"`) in `ChatBar.svelte` when a work item is selected on `/work`, and prefix outbound coordinator messages with the work item's structured reference (`[Work item: owner/repo#n — <title>]`).
- **Explicit Lens Navigation (`TopNav.svelte`):**
  - Render both `Work` (`/work`) and `Runs` (`/`) links in `TopNav.svelte` with `aria-current="page"`.

## Impact

- **Affected specs:** `observable-work`
- **Affected code:**
  - `ui/src/lib/stores/workContext.svelte.ts` (new)
  - `ui/src/lib/components/work/WorkItemOverview.svelte` and `WorkItemOverview.test.ts`
  - `ui/src/lib/components/layout/ChatBar.svelte` and `ChatBar.test.ts`
  - `ui/src/lib/components/layout/TopNav.svelte`
  - `ui/src/routes/work/+page.svelte` and `ui/src/routes/work/page.fixture.test.ts`
  - `ui/e2e/agentic/work-board.spec.ts`
- **No Go backend or `/api/work/*` write endpoints added:** `/api/work/*` and GitHub board columns remain strictly read-only.
