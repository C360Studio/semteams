# Delta for Repository Governance

## ADDED Requirements

### Requirement: Publication is image-only, tag-triggered, and gated on CI evidence

The repository SHALL publish only the container image `ghcr.io/c360studio/semteams` from one `Publish` workflow
triggered by `v*` tags, SHALL verify publication evidence before building, and SHALL fail closed when that evidence is
missing, incomplete or unsuccessful.

#### Scenario: A tag on verified main publishes the image and a release

- GIVEN `Repository CI` runs on pushes to `main`
- AND a `v*` SemVer tag points at a commit that is an ancestor of `main` and carries this `Publish` workflow
- AND every `Repository CI` run for that commit has completed
- AND the newest `CI Status Check` check-run created by GitHub Actions on that commit, by highest id, concluded
  `success`
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
- AND a `v*` tag points at a commit with a `Repository CI` run still queued or in progress, with no `CI Status Check`
  check-run, or whose newest `CI Status Check` check-run did not conclude `success`
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

## MODIFIED Requirements

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
- THEN it runs `task openspec:validate` and `task openspec:queue-test`
- AND it runs `task publish:verify-test`, which proves the publication evidence gate fails closed
- AND OpenSpec is 1.7.0, Task is 3.51.1, revive is 1.15.0, and Node is 22.20.0
- AND setup-go reads the Go version from `go.mod`
- AND official GitHub Actions use reviewed major-version tags rather than floating `latest` or a repository SHA policy

### Requirement: Repository CI cannot activate container publication

The repository SHALL keep merge validation and publication as separate workflows; publication SHALL NOT listen for
`Repository CI` completion.

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
