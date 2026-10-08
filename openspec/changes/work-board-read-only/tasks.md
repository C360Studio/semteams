## 1. Claim and design

- [x] 1.1 Open the claim for issue #296 as draft PR #304 on `claude/gh296-work-board` with this proposal as the first
      content commit.
- [x] 1.2 `architect` read on the product read-API shape, recorded in `design.md` D1–D5: no runtime type is missing on
      `main`; linkage and rule identity are handled as explicit unknown and a pack property.
- [x] 1.3 Task 1.3 stop condition not triggered (no missing runtime type); no gap posted on #296.

## 2. Fixtures and read boundary

- [x] 2.1 Add the portfolio document shape (D4): `configs/portfolio/portfolio.example.json`, its hand-authored schema,
      `types/work.ts`, and server-side validation with a visible error state.
- [x] 2.2 Add fixture set `board-mvp` (D5): two repositories, one Project status field, every column, one item with
      linked PRs, and the three linkage cases (bound by coordinator prompt, `none`, dangling run entity id).
- [x] 2.3 Implement the `GET /api/work/*` routes (D1) with `lookup` status, `GET`-only, repository scoping, and
      server-side overlay reads from `/teams-dispatch/loops/{id}` and `/graph/triples`.
- [x] 2.4 Measured on the e2e stack 2026-10-08: `GET /teams-dispatch/loops` and `/loops/{id}` expose no `metadata`,
      `prompt` or `parent_loop_id`; loop-entity triples do (`agent.loop.description`, `agent.loop.role`,
      `agent.run.entity-id`, `agent.lineage.root`, `agent.loop.task`). Design D1/D3/D5 reconciled.

## 3. Work lens and explained events

- [x] 3.1 `/work` route with board and table views, `TopNav` link, `workStore`, `workApi`, and the components in D5;
      PM status, execution stage and verification rendered as separate values.
- [x] 3.2 Work-item overview panel (`?item=`): purpose, owner, priority, milestone, dependencies, delivery context;
      linked runs on request with the D2 unknown rendering.
- [x] 3.3 Drill-in to `/?task=<coordinator loop id>`; cards carry no drag affordance.
- [x] 3.4 Posted the measured provenance gap on #298 (comment 6065775221): rule identity and the firing fact are not
      readable on any surveyed
      path (task metadata is dropped by the loops REST and lives only in the evidence store). No ops rule change.
- [x] 3.5 Runs lens: read rule-spawned loop provenance from graph triples (`agent.loop.task` prefix `rule-<firing
      entity>-`, `agent.lineage.root`), attach those loops to their coordinator card as controls instead of top-level
      cards, and render each control in `TaskStory` as one explained row: control identity, firing entity, the firing
      entity's lifecycle transition preceding the spawn (fact and value), outcome with the terminal decide reason,
      and `rule: unknown` with its reason.

## 4. Verification

- [ ] 4.1 Journey `work-board.spec.ts` per D5, with a `test:e2e:agentic:work-board` task in `ui/Taskfile.yml`.
- [ ] 4.2 Existing journeys (`research-mvp`, `chain-drill-in`, `ops-run-terminal`, approval, ask-user) still pass;
      no new ESLint, svelte-check or revive warnings; `task ui:test` green for new component tests.
- [ ] 4.3 `svelte-reviewer` pass on the work lens and runs-lens change; `go-reviewer` not required unless Go changes.
- [ ] 4.4 Archive this change and synchronize the `observable-work` specification as the landing PR's final content
      commit, followed by the read-only reviewer pass.
