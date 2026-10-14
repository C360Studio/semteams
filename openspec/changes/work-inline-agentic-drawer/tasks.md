## 1. Claim and design

- [x] 1.1 Open the claim for issue #315 on `gemini/work-inline-agentic-drawer` with `proposal.md`, `design.md`,
      `specs/observable-work/spec.md`, and `tasks.md` as the first content commit.
- [x] 1.2 `architect` design D1–D3 recorded in `design.md`: inline agentic run workbench inside `WorkItemOverview`,
      work-scoped prompt context via `workContext.svelte.ts` + `ChatBar.svelte`, and explicit `Work` / `Runs` lens links
      in `TopNav.svelte`.

## 2. Work-scoped context store and navigation

- [ ] 2.1 Create `ui/src/lib/stores/workContext.svelte.ts` tracking the active `/work` item scope (`itemRef`, `title`,
      `repository`, `programName`, `projectName`) and sync it from `ui/src/routes/work/+page.svelte`.
- [ ] 2.2 Update `ui/src/lib/components/layout/ChatBar.svelte` to render the `work-chat-context` chip when a work item
      is active on `/work` and prefix outbound messages with `[Work item: <ref> — <title>]`.
- [ ] 2.3 Update `ui/src/lib/components/layout/TopNav.svelte` to render both `Work` (`/work`) and `Runs` (`/`) lens
      navigation links with `aria-current="page"`.

## 3. Inline agentic run workbench in `WorkItemOverview.svelte`

- [ ] 3.1 Extend `ui/src/lib/components/work/WorkItemOverview.svelte` to render the inline run workbench
      (`data-testid="work-inline-run-workbench"`) when loaded linked runs resolve a `coordinator_loop_id`, including
      multi-run selection, live `RunHealthPanel` / `PendingApprovalSection` / `RunWaitingSection` integration, child/control
      loop focus, and `Activity` (`TaskStory`) / `Evidence` (`RunEvidencePanel`) tabs.
- [ ] 3.2 Update Vitest component and route tests (`WorkItemOverview.test.ts`, `ChatBar.test.ts`,
      `page.fixture.test.ts`) and Playwright E2E coverage (`work-board.spec.ts`) for inline run story/evidence tabs and
      work-scoped chat context.

## 4. Review and archive

- [ ] 4.1 Run `svelte-reviewer` on the UI changes and resolve any findings.
- [ ] 4.2 Archive `work-inline-agentic-drawer` (`openspec archive work-inline-agentic-drawer` and spec sync) as the
      landing PR's final content commit.
