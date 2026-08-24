# Tasks

- [ ] 1. Add branch-local workflow contracts for the `Repository CI` display name, unconditional pull-request trigger,
  exact `CI Status Check` context, expected jobs, semantic tool pins, and publication-name mismatch.
- [ ] 2. Replace the current Go workflow with `Repository CI`; use `task lint`, unit `-race -count=1`, and
  `task test:integration` with `-race -count=1 -p 1`; run `go build ./...` and schema generation plus drift proof.
- [ ] 3. Add the UI job on Node 22.20.0 using `npm ci` and the lint, check, test:unit, generate-types:check, and build
  package scripts.
- [ ] 4. Add the governance job using `task openspec:validate` and `task openspec:queue-test` with OpenSpec 1.7.0.
- [ ] 5. Pin Task 3.51.1 and revive 1.15.0, source Go from `go.mod`, and use reviewed official action major tags.
- [ ] 6. Add the always-running `CI Status Check` aggregate that fails closed for every required job result.
- [ ] 7. Retire `.github/workflows/ui.yml` and `.github/workflows/semspec-validation.yml`; prove the existing container
  and release workflows are unchanged under #259 and cannot be triggered by `Repository CI`.
- [ ] 8. Run the workflow contracts and every new job command locally, then reconcile documentation that describes the
  retired workflows or stale Go runtime.
- [ ] 9. After PR #257 merges and PR #262 is retargeted to `main`, observe one green hosted `CI Status Check` on
  PR #262 and record its run URL.
- [ ] 10. Reconcile task and reviewer evidence, then archive this change as the final content commit.
