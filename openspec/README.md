# SemTeams OpenSpec

SemTeams uses [OpenSpec](https://github.com/Fission-AI/OpenSpec) for governed, spec-driven changes. Living specs state
the accepted behavior of product capabilities; a claimed pull request carries any proposed delta and the tasks needed
to implement it. OpenSpec is not the work backlog.

Read [`project.md`](project.md) before scoping a change. It defines SemTeams' purpose, its boundary with SemStreams, and
the beta.160 live/parked product posture.

## Authority Split

| Question | Authority |
|---|---|
| What work is wanted, decided, blocked, or held | GitHub issue, structured labels, and owner ruling comments |
| What gates a release | GitHub milestone membership |
| Who claimed the work and where it stopped | Draft pull request with `Closes #n` and a current stop-point |
| What behavior and tasks this pull request targets | OpenSpec change in that pull request |
| What the product does now | `specs/<capability>/spec.md` after archive and merge |
| Why an architectural choice exists | ADR or owner ruling on the issue |

An active change without a claimed pull request is ambiguous state. Discovery, sequencing, holds, and future campaigns
belong in issues; do not keep them as 0/N-task changes. Issue
[#258](https://github.com/C360Studio/semteams/issues/258) owns this PR's retirement of the unimplemented
`repo-readiness-init` proposal without promoting its delta into current truth. Issue
[#260](https://github.com/C360Studio/semteams/issues/260) owns any future reintroduction and freshly reconciled change.

## Layout

- `project.md` — standing project context and product boundary.
- `config.yaml` — concise machine context and artifact rules. `project.md` remains the human-readable source.
- `specs/<capability>/spec.md` — living accepted behavior: `Requirement` plus `GIVEN`/`WHEN`/`THEN` scenarios.
- `changes/<id>/proposal.md` — why and what changes, including explicit non-goals.
- `changes/<id>/design.md` — decisions and tradeoffs when the change needs a design artifact.
- `changes/<id>/tasks.md` — branch-checkable implementation tasks in dependency order.
- `changes/<id>/specs/<capability>/spec.md` — target-state deltas added, modified, or removed by the change.
- `changes/archive/` — completed changes moved here by `openspec archive`.

## Lifecycle

1. Claim an issue with an agent-prefixed branch, dedicated worktree, push, and draft pull request.
2. Create the OpenSpec proposal as the first content commit for nontrivial or cross-cutting work.
3. Review the proposal, design, task list, and delta before implementation.
4. Keep tasks conservative and checkable on the branch. Do not add post-merge assertions such as "CI green",
   "merged", or "merge-ready"; the merge gate owns those facts.
5. Implement and reconcile the change whenever reality changes its target or scope.
6. After implementation and reviewer approval, archive in the landing pull request's **final content commit**. The
   archive moves the change and promotes its durable requirements into the living specs for review with the code.
7. Run a mandatory **read-only reviewer pass after archive**. The reviewer checks the archive move, promoted living
   specs, task truth, and strict validation without editing. If it finds a defect, fold the correction into the archive
   commit and repeat the read-only pass. No content commit may follow the accepted archive commit.

When a change is abandoned or deliberately parked, remove it from the active queue without running `openspec archive`.
Git history preserves its artifacts; a GitHub issue owns any resume decision and requires a freshly reconciled change.

## Commands

```bash
openspec list
openspec validate --all --strict
openspec show <change-id>
openspec archive <change-id>
```

Use `task openspec:queue` for the repository's hold-aware queue. CI validation
and the future main-branch ruleset are owned by
[#254](https://github.com/C360Studio/semteams/issues/254); workflow presence alone is not proof of a trustworthy
required-check contract.

Do not archive `conversational-front-door` early. Its archive and living-spec sync are the landing pull request's final
content commit before the mandatory read-only reviewer pass.

The completed pre-beta.160 `artifact-context-handoff` change was removed from the active queue without archive or spec
promotion because ADR-059 removed the evidence bodies its UI requires. Git history preserves it. Resumption requires a
freshly reconciled OpenSpec change after the UI can dereference trajectory `StorageReference` values; issue
[#261](https://github.com/C360Studio/semteams/issues/261) owns that authorized evidence-fetch follow-up.

## Relationship to Documentation

- `docs/adr/` records durable architectural decisions and cross-repository contracts.
- Operational, tutorial, and product-orientation material stays in `docs/`.
- Living behavior belongs in `openspec/specs/`, not in an ADR addendum.
- Work state belongs in GitHub, not in `docs/`, `/tickets`, or an OpenSpec change held without a claimed pull request.
