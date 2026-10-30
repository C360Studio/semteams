# Design: Adopt ATeam v0.3.5

## Context

Line references are to `origin/main` `4f45a387`. Upstream references are to `C360Studio/ateam` `v0.3.5`.

- `.agents/protocol.md:38-41` records the deferral of test-to-spec: owner ruling #374, tracked in #386, until
  `ateam#237` could scope the check to new tests. `AGENTS.md` and `rules-index.md` carry no test-to-spec row.
- `ateam test-spec --base <ref>` reports missing citations only for tests changed since `merge-base(ref, HEAD)`.
  Uncommitted and untracked files count as changed. Its Vitest scan needs `node` and `ui/node_modules/typescript`
  (seen locally: without them it exits 1).
- Measured at `4f45a387` with v0.3.5:
  - `ateam test-spec --base origin/main` reports only 3 **malformed** citations. They are at
    `cmd/semteams/rule_config_service_test.go:69`, `:121` and `cmd/semteams/rule_hot_reload_integration_test.go:120`,
    and are reported whatever the base. All three cite `component-runtime-config / …`, a SemStreams capability with no
    `openspec/specs/` entry here.
  - Unscoped `ateam test-spec` reports thousands of missing citations.
- `.github/workflows/ci.yml:101-138`, the `governance` job, has Go, Node 22.20.0 and ateam installed, but no
  `ui/node_modules` and a shallow checkout. The `ui` job (`:62-99`) has `ui/node_modules` but no Go.
- Upstream `merge-check.yml` adds an `awaiting` job named `Merge check probe`. It builds ateam from the base commit and
  runs `ateam review gate --awaiting <pr>`, which prints `awaiting=true|false`. The `merge-check` job then
  `needs: awaiting`, gets the name `Merge check (awaiting review record, skipped)` when the answer is true, and skips.
  A failed probe gives no answer, and the full check runs.
- `ateam doctor --check` under v0.3.5 reports one violation: `.agents/contracts/reviewer.md: missing or drifted review
  instructions` (`ateam#262` added the PR-description line). The `architect.md` drift check (`ateam#217`) passes.

## Goals / Non-Goals

Goals: gate new and changed tests on a spec citation, adopt the awaiting probe, and keep `task validate` in parity with
CI. Non-goals: backfilling citations, `state prune`, and Ponytail configuration (none set).

## Decisions

### D1. Where the test-to-spec gate runs, and against which base

- **A (recommended): the Governance job.** Add `npm --prefix ui ci`, `fetch-depth: 0`, and `task test:spec` with
  `TEST_SPEC_BASE` set to `github.event.pull_request.base.sha` on a PR and `github.event.before` on a push.
  `task test:spec` defaults to `origin/main` locally, and `scripts/validate.sh` runs it in the governance lane.
  Cost: a UI install in Governance, about 30 s.
- B: the UI job. It would need setup-go plus an ateam install, which mixes the lanes.
- C: run it in `Merge check`. That workflow checks out only the base commit, so it cannot see the PR's tests.

Adopter seam: a contributor who adds a test without a citation sees `Governance` fail with the test name and file:line.

### D2. Adopt the awaiting probe

- **A (recommended): copy upstream's `awaiting` job and conditional skip.** Keep our `go install @v0.3.5` and action
  tags, and add contract-test rows for the probe's base checkout, its read-only permissions, and the skip condition.
  The required `Merge check` context is missing while the probe answers `awaiting=true`, so the merge stays blocked
  (fails closed).
- B: keep today's red `Merge check` until the review lands. It is noisy, but it is one fewer job.

### D3. The three malformed citations

- **A (recommended): change the prefix to `// upstream spec: semstreams component-runtime-config / …`.** This keeps the
  provenance and leaves these legacy tests uncited, which `--base` allows.
- B: add a `runtime-composition` Requirement for rule-config service ordering (ADR-029) and cite it. That is real spec
  work, outside this change.

## Risks / Trade-offs

- **A new test with no matching Requirement forces a spec edit or a citation the reviewer has to judge.** That is what
  the default intends. A citation is resolvable only when it names an existing `### Requirement:` heading.
- **Push runs with `github.event.before` all-zero, such as a new branch, have no base.** The task falls back to
  `origin/main`, and pushes only target `main` anyway.
- **The probe is new upstream code on the merge path.** Its failure mode is "run the full check".

## Migration Plan

There is one squash merge, and no ruleset change: the context name stays `Merge check`. Open PRs pick up the
`test:spec` gate when they next merge `main`.

## Open Questions

1. **Owner ruling: lift the #374 deferral.** Adopting test-to-spec here reverses `horizon:post-mvp` on #386.
2. **Owner ruling: D1 A, D2 A, D3 A**, or name a different option.
