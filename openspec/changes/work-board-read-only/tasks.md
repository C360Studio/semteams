## 1. Claim and design

- [x] 1.1 Open the claim for issue #296 as draft PR #304 on `claude/gh296-work-board` with this proposal as the first content commit.
- [x] 1.2 `architect` read on the product read-API shape, recorded in `design.md` D1–D5: no runtime type is missing on
      `main`; linkage and rule identity are handled as explicit unknown and a pack property.
- [x] 1.3 Task 1.3 stop condition not triggered (no missing runtime type); no gap posted on #296.

## 2. Fixtures and read boundary

- [ ] 2.1 Add the portfolio document shape (D4): `configs/portfolio/portfolio.example.json`, its hand-authored schema,
      `types/work.ts`, and server-side validation with a visible error state.
- [ ] 2.2 Add fixture set `board-mvp` (D5): two repositories, one Project status field, every column, one item with
      linked PRs, and the three linkage cases (bound by coordinator prompt, `none`, dangling run entity id).
- [ ] 2.3 Implement the `GET /api/work/*` routes (D1) with `lookup` status, `GET`-only, repository scoping, and
      server-side overlay reads from `/teams-dispatch/loops/{id}` and `/graph/triples`.
- [ ] 2.4 Verify on the e2e stack that `GET /teams-dispatch/loops` returns `metadata` for the ops observer
      (`run_phase`, `run_entity_id`, `coordinator_loop_id`); if it does not, record the gap in `design.md` D3 and
      read the same keys from `GET /loops/{id}` or the trajectory instead.

## 3. Work lens and explained events

- [ ] 3.1 `/work` route with board and table views, `TopNav` link, `workStore`, `workApi`, and the components in D5;
      PM status, execution stage and verification rendered as separate values.
- [ ] 3.2 Work-item overview panel (`?item=`): purpose, owner, priority, milestone, dependencies, delivery context;
      linked runs on request with the D2 unknown rendering.
- [ ] 3.3 Drill-in to `/?task=<coordinator loop id>`; cards carry no drag affordance.
- [ ] 3.4 Add `fired_by_rule`, `firing_fact`, `firing_value` to the ops rule's `properties` (D3, pack config only).
- [ ] 3.5 Runs lens: `AgentLoop.metadata`, attach rule-spawned loops to their coordinator card as controls, and render
      each control in `TaskStory` as one explained row (rule, firing fact and value, control identity, outcome).

## 4. Verification

- [ ] 4.1 Journey `work-board.spec.ts` per D5, with a `test:e2e:agentic:work-board` task in `ui/Taskfile.yml`.
- [ ] 4.2 Existing journeys (`research-mvp`, `chain-drill-in`, `ops-run-terminal`, approval, ask-user) still pass;
      no new ESLint, svelte-check or revive warnings; `task ui:test` green for new component tests.
- [ ] 4.3 `svelte-reviewer` pass on the work lens and runs-lens change; `go-reviewer` not required unless Go changes.
- [ ] 4.4 Archive this change and synchronize the `observable-work` specification as the landing PR's final content
      commit, followed by the read-only reviewer pass.
