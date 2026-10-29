## MODIFIED Requirements

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
- AND it runs `task publish:verify-test`, which proves the publication evidence gate fails closed
- AND ATeam is v0.3.4, OpenSpec is 1.7.0, Task is 3.51.1, revive is 1.15.0, and Node is 22.20.0
- AND setup-go reads the Go version from `go.mod`
- AND official GitHub Actions use reviewed major-version tags rather than floating `latest` or a repository SHA policy

## REMOVED Requirements

### Requirement: Merge check runs the review gate from the base commit

**Reason**: ATeam v0.3.4 moves `Merge check` out of CI into its own workflow and required check (`C360Studio/ateam#255`).
**Migration**: Replaced by "Merge check is its own required workflow"; the `main` ruleset requires `Merge check` beside `Required`.

## ADDED Requirements

### Requirement: Merge check is its own required workflow

The repository SHALL run `Merge check` as its own workflow, `.github/workflows/merge-check.yml`, which executes
`scripts/merge-check.sh` (`ateam review gate`) from the pull request's base commit. The `main` ruleset SHALL require
both `Required` and `Merge check`, and `Required` SHALL NOT depend on `Merge check`.

#### Scenario: The gate's script and policy come from the base commit

- GIVEN a non-draft pull request targets `main`
- WHEN `Merge check` runs
- THEN it checks out the base commit, not the pull request head, without persisted credentials
- AND it installs ATeam v0.3.4 and runs `scripts/merge-check.sh` with the pull request number and
  `ATEAM_REQUIRED_CHECKS: Required,Merge check`
- AND its token permissions are read-only for contents, issues, and pull requests
- AND the workflow file itself is read from the pull request, so a change to it is caught by review only

#### Scenario: The ruleset must require both contexts

- GIVEN the `main` ruleset does not require `Merge check`, or does not require `Required`
- WHEN `Merge check` runs
- THEN the up-to-date rule fails and `Merge check` fails

#### Scenario: A description edit reruns the check

- GIVEN a non-draft code pull request whose `Merge check` failed for want of a review record
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
- AND marking the pull request ready runs `Merge check` with the review check applied
