## Context

Issue #315 (`ui(work): unify Project Work Board with inline Enjoy-style agentic drawer and scoped chat`) completes the three-part multi-agent coordination and UX arc (#313 -> #314 -> #315). With #296 (read-only `/work` board) and #314 (structured `claim`, `review`, and `gate` baton overlays), `/work` already shows portfolio hierarchy, GitHub-owned columns, and protocol baton state. However, inspecting a linked run's narrative (`TaskStory`), raw evidence (`RunEvidencePanel`), or resolving an active pause/approval (`RunWaitingSection`, `PendingApprovalSection`) still requires leaving `/work` via `/?task=<loopId>`, and `ChatBar` does not scope prompts to the open work item.

## Goals / Non-Goals

- **Goals:**
  - Evolve `WorkItemOverview.svelte` into an inline agentic work drawer that renders `TaskStory` (`Activity` tab) and `RunEvidencePanel` (`Evidence` tab) directly inside `/work` whenever a loaded linked run has a resolved `coordinator_loop_id`.
  - Surface live `RunHealthPanel`, `PendingApprovalSection`, `RunWaitingSection`, child sub-tasks, and rule-fired `Controls` inline when the coordinator loop is present in `taskStore`, while preserving the existing `work-drill-in` link (`/?task=<loopId>`) for full-screen runs-lens inspection.
  - Scope `ChatBar.svelte` to the selected `/work` item (and its Program / Project metadata) via a shared `workContext` store so prompts dispatched from `/work` carry structured work-item context.
  - Expose both `Work` (`/work`) and `Runs` (`/`) in `TopNav.svelte` so the portfolio work board and runtime loop board are clearly navigable lenses.
- **Non-Goals:**
  - Mutating GitHub issues, pull requests, labels, or Project board columns from the UI (`/api/work/*` remains strictly `GET`-only and board cards remain non-draggable).
  - Auto-polling linked runs for every card on the board (linked runs remain per-item on demand).

## Decisions

### D1: Inline Run Workbench Inside `WorkItemOverview.svelte`

When `linkedRuns.status === "loaded"` and one or more `LinkedRun` entries carry a non-null `coordinator_loop_id`:
- `WorkItemOverview.svelte` renders an inline workbench section (`data-testid="work-inline-run-workbench"`) below the linked-run summary list.
- If more than one resolved linked run exists, a run selector (`data-testid="work-inline-run-selector"`) lets the operator switch which resolved run is inspected inline (defaulting to the first resolved run).
- For the selected resolved run (`activeRun`):
  - Look up `liveTask = taskStore.tasks.find((t) => t.id === activeRun.coordinator_loop_id)`.
  - If `liveTask` is available, render:
    - `RunHealthPanel` (when `liveTask.runHealth` is present)
    - `PendingApprovalSection` (when `liveTask.state === "awaiting_approval"` and `liveTask.primaryLoop.pending_approval` is present)
    - `RunWaitingSection` (when `liveTask.runPause` is present)
    - Collapsible/focusable sub-tasks (`liveTask.childLoops`) and rule-fired `Controls` (`liveTask.controls`) with a focus breadcrumb (`data-testid="work-inline-focus-breadcrumb"`) matching `TaskDetailPanel`.
  - Render an in-drawer `Activity | Evidence` tablist (`data-testid="work-inline-run-tabs"`, `data-testid="work-run-tab-activity"`, `data-testid="work-run-tab-evidence"`):
    - `Activity` renders `<TaskStory loopId={focusedLoopId} ... />`
    - `Evidence` renders `<RunEvidencePanel loopId={focusedLoopId} runEntityId={activeRun.run_entity_id} />`
- Keep the existing `linked-run-row` badges and `work-drill-in` anchor intact so operators can still jump to the standalone Runs lens (`/?task=<loopId>`) and existing journey assertions remain valid.

### D2: Work-Scoped Prompt Bar via `workContext.svelte.ts`

- Create `ui/src/lib/stores/workContext.svelte.ts` holding the active `/work` item context:
  ```ts
  export interface WorkChatScope {
    itemRef: string;
    title: string;
    repository: string;
    programName?: string;
    projectName?: string;
  }
  ```
- `/work/+page.svelte` updates `workContext` via `$effect` whenever `selection` resolves to `{ kind: "item", item }` (looking up the item's `programName` and `projectName` from `store.programs`), and clears `workContext` when `selection` is `"none"` or when `/work` unmounts.
- In `ChatBar.svelte`:
  - When `taskStore.selectedTask` is null and `workContext.scope` is non-null, render a context chip (`data-testid="work-chat-context"`) showing `{programName} / {projectName} · {itemRef}` and `{title}`, with a clear button (`data-testid="work-chat-context-clear"`) that calls `workContext.clear()`.
  - Update the input placeholder to `"Ask or direct the team on ${workContext.scope.itemRef}…"`.
  - When dispatching a message while `workContext.scope` is active, prepend a structured context header:
    ```
    [Work item: <itemRef> — <title>]
    <user prompt>
    ```
    so the coordinator loop's `agent.loop.description` records the work item reference and title deterministically.

### D3: Primary Navigation Lens Links in `TopNav.svelte`

- Add a `Runs` link (`href={resolve("/")}`, `data-testid="nav-runs"`, `aria-current={page.route.id === "/" ? "page" : undefined}`) alongside `Work` (`href={resolve("/work")}`, `data-testid="nav-work"`) in `TopNav.svelte`.

## Risks / Trade-offs

- **Risk:** Mounting `TaskStory` inside `WorkItemOverview` triggers `agentApi.getLoopTrajectory(loopId)` in tests where `/work` opens a linked run.
  - **Mitigation:** Ensure `page.fixture.test.ts` (and `WorkItemOverview.test.ts`) either handle the GraphQL trajectory request in their fake backend or gracefully render the trajectory state without breaking existing linked-run assertions.
