# repository-governance Specification

## Purpose

Define the repository-wide CI contract that every pull request to `main` must satisfy before merge.

## Requirements
### Requirement: One unconditional repository CI workflow reports one aggregate

The repository SHALL run one workflow named `Repository CI` for every pull request targeting `main` and SHALL report
the stable `CI Status Check` aggregate.

#### Scenario: Any pull request reports the required context

- GIVEN a pull request targets `main`
- WHEN GitHub Actions evaluates its head commit
- THEN `Repository CI` runs without workflow-level path filters
- AND `CI Status Check` reports after all required jobs

#### Scenario: The aggregate fails closed

- GIVEN a required job is unsuccessful, cancelled, or unexpectedly skipped
- WHEN the aggregate evaluates its dependencies
- THEN `CI Status Check` fails
- AND missing evidence is not translated into success

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
- AND OpenSpec is 1.7.0, Task is 3.51.1, revive is 1.15.0, and Node is 22.20.0
- AND setup-go reads the Go version from `go.mod`
- AND official GitHub Actions use reviewed major-version tags rather than floating `latest` or a repository SHA policy

### Requirement: Repository CI cannot activate container publication

The repository SHALL keep the new validation workflow distinct from existing publication triggers and SHALL retire
obsolete validation workflows.

#### Scenario: Workflow name cannot match the publisher listener

- GIVEN the existing container workflow listens for a completed workflow named `CI`
- WHEN the new workflow is named `Repository CI`
- THEN the names do not match
- AND successful `Repository CI` completion cannot start container publication

#### Scenario: Obsolete workflows retire without publication redesign

- GIVEN `ui.yml` is path-filtered and `semspec-validation.yml` is copied broken validation
- WHEN `Repository CI` becomes the repository merge check
- THEN both obsolete workflows are removed
- AND the container and release workflows remain unchanged under issue #259
