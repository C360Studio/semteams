## 1. Claim and design

- [x] 1.1 Open the claim for issue #320 on `gemini/work-item-run-linkage` with `proposal.md`, `design.md`,
      `specs/observable-work/spec.md`, and `tasks.md` as the first content commit.
- [x] 1.2 `architect` design D1–D2 recorded in `design.md`: `{ by: "work_item_ref", ref: string }` binding in
      `WorkSource` and dual-path graph resolution (`run.issue.ref` + `[Work item: ...]` prompt scope) in `overlays.ts`.

## 2. Work-item run linkage implementation

- [ ] 2.1 Extend `LinkedRunBinding` in `ui/src/lib/server/work/source.ts` with `{ by: "work_item_ref"; ref: string }`,
      update `createGithubSource().linkedRunBindings()` in `ui/src/lib/server/work/githubSource.ts`, and accept
      `work_item_ref` in `ui/src/lib/server/work/fixtureSource.ts`.
- [ ] 2.2 Implement `extractScopedWorkItemRef()` and `resolveWorkItemRefBinding()` in `ui/src/lib/server/work/overlays.ts`
      to resolve runs via `run.issue.ref` triples and `[Work item: owner/repo#number ...]` coordinator prompt headers
      with deduplication and `Overlay<T>` honesty (`WORK_ITEM_REF_UNMATCHED_REASON`).
- [ ] 2.3 Add unit and service tests in `ui/src/lib/server/work/overlays.test.ts`,
      `ui/src/lib/server/work/githubSource.test.ts`, and `ui/src/lib/server/work/service.test.ts`.

## 3. Review and archive

- [ ] 3.1 Run `svelte-reviewer` on the changes and resolve any findings.
- [ ] 3.2 Archive `work-item-run-linkage` (`openspec archive work-item-run-linkage` and spec sync) as the landing PR's
      final content commit.
