# Tasks

- [x] 1. Propose image-only tag publication with a repository-governance delta that adds the publication requirement
  and rewrites the `Repository CI cannot activate container publication` requirement; validate strictly.
- [x] 2. Replace the deleted copied `release.yml` and `container.yml` with one `Publish` workflow: `v*` tag trigger,
  dispatch dry run, main-ancestry and `CI Status Check` verification before any build, `linux/amd64` and `linux/arm64`
  image build, and a release with generated notes, image reference and digest. Lint it with actionlint and prove no
  workflow listens for `workflow_run`.
- [x] 3. Make the image identity SemTeams and the build information ldflags-settable; prove `--version` output and the
  OCI labels from a local image build; run lint, build, unit tests and schema drift locally.
- [x] 4. Document tag-only publication in `docs/release.md` and point the CI Baseline paragraph in `CLAUDE.md` and
  `AGENTS.md` at it.
- [x] 5. Move the evidence gate into `scripts/publish-verify.sh`: SemVer 2.0 tags, main ancestry, every `Repository CI`
  run for the commit completed, and the newest (highest-id) `CI Status Check` check-run concluded `success`. Cover it
  with `scripts/publish-verify_fixture_test.sh`, run as `task publish:verify-test` in the Governance CI job, and align
  the spec delta.
- [x] 6. Harden the publish job: create the release through `gh api` without a third-party release action, refuse a
  tag that already has a release, gate login, push and release on an explicit `dry_run == 'false'`, publish one run
  at a time, label the verified revision, and keep the token out of the checkout.
- [ ] 7. Replace the unread `docker/.dockerignore` with `docker/Dockerfile.dockerignore` and prove the builder context
  excludes `.git` while keeping `configs` and `.devcontainer`.
- [ ] 8. Document which commits may be tagged, the admin-only tag ruleset as the guard to enable before the first
  tag, and recovery from a tag pushed before CI finishes.
- [ ] 9. Archive this change as the final content commit after the reviewer pass.
