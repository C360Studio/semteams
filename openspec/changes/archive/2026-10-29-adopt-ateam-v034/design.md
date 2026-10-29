# Design: Adopt ATeam v0.3.4

## Context

Line references are to `origin/main` `781a57ea`. Upstream references are to `C360Studio/ateam` `v0.3.4`.

- `.github/workflows/ci.yml:19-187`: `go`, `ui`, `governance`, `merge-check` (`:135-171`), and `required`
  (`:173-187`, `needs: [go, ui, governance, merge-check]`). `ateam@v0.3.3` is pinned at `:36`, `:119`, and `:162`.
  No job has a draft guard.
- Upstream `.github/workflows/merge-check.yml` triggers on `pull_request` `[opened, synchronize, reopened,
  ready_for_review, edited]`. It uses `concurrency: merge-check-<pr>` with `cancel-in-progress: false`, a job guard
  `if: !draft` that also renames the job to `Merge check (draft, skipped)`, a base-commit checkout, and
  `ATEAM_REQUIRED_CHECKS: Required,Merge check`. Upstream `ci.yml` guards and renames `Verify` and `Required` the same
  way.
- `cmd/ateam/review.go:177` and `internal/review/gate.go:54`: the gate checks that the `main` ruleset requires every
  check named in `ATEAM_REQUIRED_CHECKS`, or only `Required` when the variable is unset. It reads the ruleset, not
  check-run results, so it does not depend on its own run (`openspec/specs/harness/spec.md:443` upstream).
- `internal/state/openspec.go:478-526`: `spec-check` looks for `openspec` in `node_modules/.bin`, then on PATH. When
  it finds neither, it prints "strict validation skipped" and **exits 0**.
- `#369` surface: `scripts/openspec-queue.sh` and `_fixture_test.sh`; `taskfiles/openspec.yml:21-39`;
  `Taskfile.yml:99-104`; `scripts/validate.sh:111`; `ci.yml` (`task openspec:queue-test`);
  `test/contract/specqueue_test.go` (`TestSpecQueueHolds`, `TestSpecQueueCheck`, `TestSpecCheckWiring`);
  `test/contract/validate_task_contract_test.go:55`; `AGENTS.md:81` and `:162`; `.agents/rules-index.md:60`;
  `.agents/skills/preflight/SKILL.md:30-40`; `.agents/contracts/technical-writer.md:34-57`; `README.md:224`; and the
  python3 comment at `scripts/doctor.sh:39`.
- `ateam doctor --check` at v0.3.4 reports drifted review instructions in `.agents/contracts/{developer,reviewer}.md`:
  "exact model" becomes "model as `.agents/review-policy.json` lists it". `review-pr` § 5 now covers the `edited`
  rerun.
- `openspec/specs/repository-governance/spec.md:7-24` ("One unconditional repository CI workflow"), `:26-54`
  (governance scenario), and `:150-191` ("Merge check runs the review gate from the base commit").

## Goals / Non-Goals

Goals: match ateam v0.3.4's merge-gate layout and draft deferral, retire the queue script, and keep `task validate`,
`Publish`, and the ruleset consistent. Non-goals: `Gate files` (#372), `ateam state prune`, and test-to-spec (#386).

## Decisions

### D1. Move Merge check into its own workflow

- **A (chosen): copy upstream `merge-check.yml`.** Keep our install-by-`go install @v0.3.4` instead of upstream's
  build from source. `required` then needs only the three lanes. The reviewer's edit reruns the check, and an open
  flake no longer turns `Required` red. Cost: one new file, and the job's push-to-`main` run goes away.
- B: keep the job in `ci.yml` and add `edited` to its triggers. That reruns Go, UI, and integration on every
  description edit, which is minutes of CI per edit, and the flake would still turn `Required` red.

Adopter seam: `Merge check` becomes a separate required context. If nobody adds it to the ruleset, D2's gate fails
every PR.

### D2. Set `ATEAM_REQUIRED_CHECKS`

- **A (chosen): `Required,Merge check`, as upstream.** The gate then fails if the ruleset ever drops `Merge check`.
  The owner adds `Merge check` to the `main` ruleset **before** this PR can pass its own gate. Adding it early is
  safe: today's `ci.yml` job already reports a context named `Merge check`.
- B: leave the variable unset. The gate checks `Required` only, so a ruleset that silently stops requiring
  `Merge check` would let unreviewed PRs merge.

### D3. Decide what `spec:check` runs after the script is gone

- **A (recommended): `openspec validate --all --strict` then `ateam state spec-check`.** This diverges from upstream,
  whose `spec:check` runs `spec-check` alone. The reason is fail-closed behavior: without `openspec`, `spec-check`
  exits 0 (`openspec.go:526`), and the explicit step fails instead (AGENTS.md § 1). Cost: strict validation runs twice
  where `openspec` is present, which takes a few seconds.
- B: `ateam state spec-check` alone, matching upstream. A host or CI image without `openspec` passes silently.

### D4. Defer draft PRs

- **A (chosen): guard and rename every `Repository CI` job and `Merge check`, as upstream does.** Each job gets
  `if: github.event_name != 'pull_request' || !github.event.pull_request.draft` and the name
  `<name> (draft, skipped)`. `required` keeps `always()` and adds the draft guard. GitHub counts a skipped check as
  passing, so the renamed check never satisfies the ruleset (`ateam#250`). Push events are unaffected, and so is
  `Publish`, which reads `Required` on tag commits.
- B: guard without renaming. A skipped `Required` on a draft head would count as passing.

## Risks / Trade-offs

- The ruleset change is an owner action outside the diff. Task 1.1 records it as an owner precondition, and the PR's
  own `Merge check` fails until it is done.
- `validate_task_contract_test` and `specqueue_test` change shape. The new ci.yml and merge-check.yml assertions carry
  sensitivity rows: `edited` dropped, draft guard dropped, `ATEAM_REQUIRED_CHECKS` entry dropped, and `required` still
  needing `merge-check`.
- The local `ateam` on PATH (`~/.local/bin`, a v0.3.3+ dev build) shadows `~/go/bin/ateam` v0.3.4. Local evidence
  names the binary it used.

## Migration Plan

Before merge, the owner adds `Merge check` to the `main` ruleset. The PR then merges as one squash commit. Already-open
PRs pick up the new workflow when they next update from `main`. Until then, their `ci.yml` still carries the old job.

## Open Questions

1. **Owner ruling: D3 A or B.** Should `spec:check` keep the explicit strict step so it fails closed, or match
   upstream?
2. **Owner confirmation: add `Merge check` to the `main` ruleset (D2 A).** This is a GitHub settings change I will
   not make without your go-ahead.
