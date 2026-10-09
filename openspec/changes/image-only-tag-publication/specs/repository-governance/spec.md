# Delta for Repository Governance

## ADDED Requirements

### Requirement: Publication is image-only, tag-triggered, and gated on CI evidence

The repository SHALL publish only the container image `ghcr.io/c360studio/semteams` from one `Publish` workflow
triggered by `v*` tags, SHALL verify publication evidence before building, and SHALL fail closed when that evidence is
missing or unsuccessful.

#### Scenario: A tag on verified main publishes the image and a release

- GIVEN a `v*` tag points at a commit that is an ancestor of `main`
- AND the most recent `CI Status Check` check-run on that commit concluded `success`
- WHEN the `Publish` workflow runs for the tag push
- THEN it builds the image for `linux/amd64` and `linux/arm64`
- AND it pushes the semver version and `<major>.<minor>` image tags to `ghcr.io/c360studio/semteams` without `latest`
- AND it creates a GitHub release with generated notes, the image reference and the pushed digest
- AND it attaches no binaries

#### Scenario: A tag off main fails before building

- GIVEN a `v*` tag points at a commit that is not an ancestor of `main`
- WHEN the `Publish` workflow verifies publication evidence
- THEN verification fails
- AND no image is built or pushed and no release is created

#### Scenario: A tag without successful CI evidence fails before building

- GIVEN a `v*` tag points at a commit with no `CI Status Check` check-run, or whose most recent one did not conclude
  `success`
- WHEN the `Publish` workflow verifies publication evidence
- THEN verification fails
- AND missing evidence is not translated into success
- AND no image is built or pushed and no release is created

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
- AND its OCI labels name SemTeams, the SemTeams source repository and the MIT license

## MODIFIED Requirements

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
