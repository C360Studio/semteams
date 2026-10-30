# repository-governance Specification

## Purpose

Define the repository-wide CI contract that every pull request to `main` must satisfy before merge.
## Requirements
### Requirement: One unconditional repository CI workflow reports one aggregate

The repository SHALL run one workflow named `Repository CI` for every push to `main` and every non-draft pull request
targeting `main`, and SHALL report the stable `Required` aggregate check-run expected by `ateam review gate` and
`Publish`. On a draft pull request every job SHALL be skipped and its name SHALL NOT evaluate to `Required`, so a draft
head carries no check-run named `Required`.

#### Scenario: Any pull request reports the required context

- GIVEN a non-draft pull request targets `main`
- WHEN GitHub Actions evaluates its head commit
- THEN `Repository CI` runs without workflow-level path filters
- AND `Required` reports after all required jobs

#### Scenario: The aggregate fails closed

- GIVEN a required job is unsuccessful, cancelled, or unexpectedly skipped
- WHEN the aggregate evaluates its dependencies
- THEN `Required` fails
- AND missing evidence is not translated into success

#### Scenario: A draft head carries no Required

- GIVEN a draft pull request targets `main`
- WHEN `Repository CI` runs on it
- THEN every job is skipped, and no skipped check-run is named `Required`, so none counts as passing; GitHub does not
  evaluate a skipped job's `name:` expression, so the check-run shows the raw expression (ATeam v0.3.4,
  `C360Studio/ateam#250`)
- AND marking the pull request ready runs every job on the `ready_for_review` event

### Requirement: Repository CI runs the obvious repository checks

The repository SHALL run separate Go, UI, and governance jobs using repository commands and exact semantic tool pins.

#### Scenario: Go checks cover lint, test, build, and generated drift

- GIVEN the Go job runs
- WHEN it evaluates a pull request head
- THEN `task lint` runs Go fmt, vet, and revive, and unit tests run with `-race -count=1`
- AND `task test:integration` owns `-tags=integration -race -count=1 -p 1`
- AND `go build ./...` runs
- AND `task schema:generate` regenerates schema/OpenAPI artifacts and the job fails on drift

#### Scenario: UI checks cover the maintained package contract

- GIVEN the UI job has run `npm ci`
- WHEN it evaluates the checked-out UI
- THEN it runs the `lint`, `check`, `test:unit`, `generate-types:check`, and `build` package scripts

#### Scenario: Governance checks use reviewed validation semantics

- GIVEN the governance job runs
- WHEN it evaluates repository policy artifacts
- THEN it runs `ateam doctor --check` with session-ID environment variables unset so multi-agent setup, hooks, MCP config, skills, and review-policy parity are verified without mutating the worktree
- AND it runs `task openspec:validate`, which runs `openspec validate --all --strict` and then `ateam state spec-check`, so a missing `openspec` fails rather than skips
- AND it runs `task test:spec` against the pull request base, or the previous `main` commit on a push (`ateam test-spec --base`)
- AND it runs `task publish:verify-test`, which proves the publication evidence gate fails closed
- AND ATeam is v0.3.5, OpenSpec is 1.7.0, Task is 3.51.1, revive is 1.15.0, and Node is 22.20.0
- AND setup-go reads the Go version from `go.mod`
- AND official GitHub Actions use reviewed major-version tags rather than floating `latest` or a repository SHA policy

### Requirement: Publication is image-only, tag-triggered, and gated on CI evidence

The repository SHALL publish only the container image `ghcr.io/c360studio/semteams` from one `Publish` workflow
triggered by `v*` tags, SHALL verify publication evidence before building, and SHALL fail closed when that evidence is
missing, incomplete or unsuccessful.

#### Scenario: A tag on verified main publishes the image and a release

- GIVEN `Repository CI` runs on pushes to `main`
- AND a `v*` SemVer tag points at a commit that is an ancestor of `main` and carries this `Publish` workflow
- AND every `Repository CI` run for that commit has completed
- AND the newest `Required` check-run created by GitHub Actions on that commit, by highest id, concluded `success`
- WHEN the `Publish` workflow runs for the tag push
- THEN it builds the image for `linux/amd64` and `linux/arm64`
- AND it pushes the semver version and `<major>.<minor>` image tags to `ghcr.io/c360studio/semteams` without `latest`
- AND it creates a GitHub release with generated notes, the image reference and the pushed digest
- AND it attaches no binaries

#### Scenario: A tag off main fails before building

- GIVEN the tagged commit carries this `Publish` workflow
- AND a `v*` tag points at a commit that is not an ancestor of `main`
- WHEN the `Publish` workflow verifies publication evidence
- THEN verification fails
- AND no image is built or pushed and no release is created

#### Scenario: A tag without complete, successful CI evidence fails before building

- GIVEN the tagged commit carries this `Publish` workflow
- AND a `v*` tag points at a commit with a `Repository CI` run still queued or in progress, with no `Required`
  check-run, or whose newest `Required` check-run did not conclude `success`
- WHEN the `Publish` workflow verifies publication evidence
- THEN verification fails
- AND an older success does not outvote a newer or still-running check
- AND missing evidence is not translated into success
- AND no image is built or pushed and no release is created

#### Scenario: A tag that is not SemVer fails before building

- GIVEN the tagged commit carries this `Publish` workflow
- AND a `v*` tag that is not SemVer 2.0 `vMAJOR.MINOR.PATCH[-PRERELEASE]`, such as `v01.2.3`, `v1.2` or `v1.2.3+meta`
- WHEN the `Publish` workflow verifies publication evidence
- THEN verification fails
- AND no image is built or pushed and no release is created

#### Scenario: A tag that already has a release is not republished

- GIVEN a `v*` tag that already has a GitHub release
- WHEN the `Publish` workflow runs for it again
- THEN it fails before logging in or pushing
- AND the existing release and the image digest it records stay unchanged

#### Scenario: A manual dispatch is only a dry run

- GIVEN a maintainer starts `Publish` with `workflow_dispatch`
- WHEN `dry_run` is true, its default
- THEN the workflow verifies the same evidence and builds both platforms with push disabled
- AND it performs no registry login, image push or release
- AND a dispatch with `dry_run` false fails verification because manual publication is not a path

#### Scenario: A prerelease tag stays off the latest release

- GIVEN a `v*` tag whose version carries a hyphen suffix, such as `v0.1.0-rc.1`
- WHEN the `Publish` workflow creates the release
- THEN the GitHub release is marked prerelease
- AND it does not become the repository's latest release
- AND only the full version image tag is pushed

#### Scenario: A published image identifies itself as SemTeams

- GIVEN an image built by the `Publish` workflow for tag `vX.Y.Z`
- WHEN the image runs with `--version`
- THEN it reports `semteams version vX.Y.Z` with the built commit and build date
- AND its OCI labels name SemTeams, the SemTeams source repository, the verified revision and the MIT license

### Requirement: Merge validation and publication are separate workflows

The repository SHALL keep merge validation and publication as separate workflows; publication SHALL NOT listen for
`Repository CI` completion, and the `Publish` workflow SHALL be the only publication path.

#### Scenario: No workflow listens for workflow completion

- GIVEN the workflows under `.github/workflows/`
- WHEN their triggers are inspected
- THEN no workflow declares a `workflow_run` trigger
- AND successful `Repository CI` completion cannot start image publication

#### Scenario: The copied publication workflows are removed

- GIVEN `release.yml` and `container.yml` were copied SemStreams publication workflows
- WHEN image-only tag publication is adopted
- THEN both workflows are removed
- AND the `Publish` workflow is the only publication path

### Requirement: Merge check is its own required workflow

The repository SHALL run `Merge check` as its own workflow, `.github/workflows/merge-check.yml`, which executes
`scripts/merge-check.sh` (`ateam review gate`) from the pull request's base commit. The `main` ruleset SHALL require
both `Required` and `Merge check`, and `Required` SHALL NOT depend on `Merge check`.

#### Scenario: The gate's script and policy come from the base commit

- GIVEN a non-draft pull request targets `main`
- WHEN `Merge check` runs
- THEN it checks out the base commit, not the pull request head, without persisted credentials
- AND it installs ATeam v0.3.5 and runs `scripts/merge-check.sh` with the pull request number and
  `ATEAM_REQUIRED_CHECKS: Required,Merge check`
- AND its token permissions are read-only for contents, issues, and pull requests
- AND the workflow file itself is read from the pull request, so a change to it is caught by review only

#### Scenario: The ruleset must require both contexts

- GIVEN the `main` ruleset does not require `Merge check`, or does not require `Required`
- WHEN `Merge check` runs
- THEN the up-to-date rule fails and `Merge check` fails

#### Scenario: A description edit reruns the check

- GIVEN a non-draft code pull request whose `Merge check` was skipped while awaiting its review record
- WHEN the reviewer posts an approving record and then edits the description to add `reviewed-by:`
- THEN `Merge check` reruns on the `edited` event at the same head without rerunning `Repository CI`
- AND runs for one pull request queue rather than cancel one another

#### Scenario: A code pull request whose latest review record does not approve fails Merge check

- GIVEN a non-draft code pull request whose latest review record from the agent named on the PR body's `reviewed-by:`
  line names one of its commits
- WHEN that record has no `Verdict:` line outside code fences, more than one, or one that is not `Verdict: APPROVE`
- THEN `Merge check` fails (ATeam v0.3.3, `C360Studio/ateam#201`)
- AND whether the findings justify an approving verdict stays review only

#### Scenario: An open known flake fails only Merge check

- GIVEN an open `class:flake` issue that the pull request does not close
- WHEN `Repository CI` and `Merge check` run
- THEN `Merge check` fails and `Required` is unaffected

#### Scenario: A draft carries no Merge check

- GIVEN a draft pull request
- WHEN `merge-check.yml` runs
- THEN its job is skipped, and its check-run is not named `Merge check` (it shows the raw `name:` expression)
- AND marking the pull request ready runs the `Merge check probe`, and `Merge check` runs in full unless the pull request
  is awaiting its review record

#### Scenario: A code pull request awaiting its review record carries no Merge check

- GIVEN a non-draft code pull request with no `reviewed-by:` line in its description and no review record comment
- WHEN `merge-check.yml` runs
- THEN the `Merge check probe` job, which runs ATeam from the base commit, answers `awaiting=true`
- AND the `Merge check` job is skipped, so no check-run named `Merge check` exists and the merge stays blocked
- AND a failed probe gives no answer and the full `Merge check` runs (ATeam v0.3.5, `C360Studio/ateam#262`)

### Requirement: New and changed tests cite a spec Requirement

`Repository CI` SHALL fail when a Go or Vitest test that the pull request adds or changes has no resolvable spec citation
(`// spec: openspec/specs/<capability>/spec.md#Requirement: <name>`). Tests the pull request does not touch SHALL NOT be
required to carry one.

#### Scenario: A new uncited test fails Governance

- GIVEN a pull request that adds a Go `Test` function or a Vitest `it`/`test` with no `// spec:` citation
- WHEN the Governance job runs `task test:spec` with the pull request base
- THEN `ateam test-spec --base` reports the test as missing a citation and `Governance` fails, so `Required` fails

#### Scenario: Legacy uncited tests do not fail

- GIVEN a pull request that changes no test
- WHEN the Governance job runs `task test:spec`
- THEN no missing citation is reported for tests outside the diff

#### Scenario: A malformed citation fails anywhere

- GIVEN a test whose `// spec:` comment does not name an `openspec/specs/` path and Requirement
- WHEN `task test:spec` runs
- THEN it is reported as malformed whether or not the pull request changes it

