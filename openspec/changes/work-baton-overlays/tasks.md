## 1. Claim and design

- [x] 1.1 Open the claim for issue #314 on `gemini/work-baton-overlays` with `proposal.md`, `design.md`,
      `specs/observable-work/spec.md`, and `tasks.md` as the first content commit.
- [x] 1.2 `architect` design D1–D5 recorded in `design.md`: protocol baton overlays (`claim`, `review`, `gate`) derive
      server-side on `WorkItem` with strict `known | none | unknown` semantics and zero Go runtime changes.

## 2. Wire types, server derivation, and fixtures

- [x] 2.1 Extend `ui/src/lib/types/work.ts` with `WorkClaim`, `WorkReviewBaton`, `WorkGate`, and `BatonFilter` types
      and add `claim`, `review`, and `gate` `Overlay<T>` fields to `WorkItem`.
- [x] 2.2 Implement deterministic baton, review-SHA parity (`not_requested`, `requested`, `current`, `stale`,
      `changes_requested`), and decision/hold gate derivation in `ui/src/lib/server/work/baton.ts` and wire it into
      `ui/src/lib/server/work/fixtureSource.ts`.
- [x] 2.3 Extend `ui/src/lib/server/work/fixtures/board-mvp/items.json` with representative cases for `claude` and
      `codex` PR claims, declared stop-points, current vs. stale `Review record` commit SHAs, `status:needs-decision`,
      and OpenSpec `Hold:` caveats.
- [x] 2.4 Add Vitest unit coverage in `ui/src/lib/server/work/` verifying `known | none | unknown` derivation for
      builder claims, review SHA staleness, decision/hold gates, and column non-derivation.

## 3. Work lens UI and baton filtering

- [x] 3.1 Add URL-backed `?baton=` slice filtering (D3) to `ui/src/routes/work/+page.svelte` and
      `ui/src/lib/utils/workView.ts` while preserving GitHub-owned columns.
- [x] 3.2 Render `Baton`, `Review`, and `Gate` `OverlayBadge` controls on `WorkItemCard.svelte` and `WorkTable.svelte`
      and add the structured Coordination & Baton section to `WorkItemOverview.svelte`.
- [x] 3.3 Update component and E2E tests (`ui/e2e/agentic/work-board.spec.ts`) to cover baton badges, overview review
      SHA parity details, and `?baton=` filtering across board and table views.

## 4. Review and archive

- [x] 4.1 Run `svelte-reviewer` on the UI and server-route changes and resolve any findings.
- [ ] 4.2 Archive `work-baton-overlays` (`openspec archive work-baton-overlays` and spec sync) as the landing PR's
      final content commit.
