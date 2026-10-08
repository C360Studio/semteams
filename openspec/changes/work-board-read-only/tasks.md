## 1. Claim and design

- [x] 1.1 Open the claim for issue #296 on `claude/gh296-work-board` with this proposal as the first content commit.
- [ ] 1.2 HOLD on the `architect` read of the product read-API shape; tasks 2.x and 3.x wait on it. The read names
      which existing scoped reads (GraphQL entity queries, `/teams-dispatch/loops`, the trajectory endpoint,
      `/components/flowgraph`) supply the work-item overlays, the portfolio configuration's minimal read-side
      shape, and whether any runtime type is missing on `main`; record the result in `design.md`.
- [ ] 1.3 If the read requires runtime types absent on `main`, stop at the design, post the gap on #296, and leave
      the remaining tasks unchecked.

## 2. Fixtures and read boundary

- [ ] 2.1 Add the portfolio configuration's minimal read-side shape (program → project → repository) shared with #267.
- [ ] 2.2 Add GitHub enumeration fixtures (issues across two repositories, one with a Project status field, one
      without) and run-fact fixtures, including one ops-observer firing on `agent.run.phase`.
- [ ] 2.3 Expose the fixtures behind the product read API with repository and run scoping; the browser never touches
      NATS or admin credentials.

## 3. Work lens

- [ ] 3.1 Board and table over work items, grouped by project, columns from Project status or the Todo / In Progress /
      Done fallback, PM status visibly separate from execution stage and from verification.
- [ ] 3.2 Work-item overview: purpose, owner, priority, milestone, dependencies, delivery context; linked specs, runs
      and evidence on request.
- [ ] 3.3 Overlays with explicit unknown states: linked runs, execution stage, needs-you; "no linked run" only after a
      complete scoped lookup; attention absent until #267.
- [ ] 3.4 Drill-in to the existing runs lens; rule-fired controls render as explained events with the firing fact
      visible.
- [ ] 3.5 No card action mutates run state; drag does not change a column.

## 4. Verification

- [ ] 4.1 Playwright journey on the `chain-drill-in.spec.ts` template: render from fixtures, drill-in reaches the run
      story, unknown renders as unknown.
- [ ] 4.2 Existing journeys (`research-mvp`, `chain-drill-in`, approval, ask-user) still pass; no new ESLint,
      svelte-check or revive warnings.
- [ ] 4.3 `svelte-reviewer` pass on the work lens; `go-reviewer` pass if the read API adds Go.
- [ ] 4.4 Archive this change and synchronize the `observable-work` specification as the landing PR's final content
      commit, followed by the read-only reviewer pass.
