# Adopt ATeam v0.3.5: diff-scoped test-to-spec gate and the Merge check awaiting probe

Approved-by: Coby Leuschke (2026-10-30)

Track: design. This change edits `.github/workflows/`, which counts as a gate file. It also adds a `repository-governance` Requirement.

## Why

[ATeam v0.3.5](https://github.com/C360Studio/ateam/releases/tag/v0.3.5) shipped on 2026-10-30.

- It ships `ateam test-spec --base <ref>` (`C360Studio/ateam#237`). With it, a brownfield repository can require a spec citation on new and changed tests without backfilling the 4,732 uncited tests counted on #386. That was the condition the owner set when deferring test-to-spec on #374 (`.agents/protocol.md:38-41`).
- It also skips `Merge check` while a code PR is still waiting for its review record (`C360Studio/ateam#262`). Today every ready code PR shows a red `Merge check` until the reviewer posts (#387, #389).
- Under v0.3.5, `ateam doctor --check` flags our `.agents/contracts/reviewer.md` as drifted, so the pin cannot move without syncing that file.

Owner ruling on #375: match ateam unless there is a very good reason not to.

## What changes

- Pin `ateam@v0.3.5` in `ci.yml`, `merge-check.yml`, and `scripts/doctor.sh`, and sync `reviewer.md` to the v0.3.5 template.
- Add `task test:spec`, which runs `ateam test-spec --base <ref>` (default `origin/main`). `Repository CI` runs it against the PR base (design D1).
- Fix the three `// spec:` citations that `test-spec` reports as malformed (design D3).
- In `merge-check.yml`, add ateam's `Merge check probe` job. `Merge check` is skipped and renamed while the PR is still awaiting its review record (design D2).
- Update `.agents/protocol.md` to replace the deferral paragraph with the adopted rule, and add the rule row to both `AGENTS.md` and `.agents/rules-index.md`. Update the `repository-governance` spec and the contract tests to match.

## Non-goals

- Backfilling citations on existing tests.
- `ateam state prune` adoption.
- Ponytail mode config. The repository sets no `lite`, `ultra`, or `full` mode.

## Impact

- A PR that adds or changes a Go or Vitest test without a resolvable `// spec:` citation fails `Required`.
- A ready code PR shows `Merge check probe` and a skipped `Merge check` until its review lands. While it is skipped, the required `Merge check` context is missing, so the merge stays blocked.

Closes #386.
