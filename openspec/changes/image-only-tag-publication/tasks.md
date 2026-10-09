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
- [ ] 5. Archive this change as the final content commit after the reviewer pass.
