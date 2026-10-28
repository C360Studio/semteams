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
- THEN it runs `ateam doctor --check` with session-ID environment variables unset so multi-agent setup, hooks, MCP config, skills, and review-policy parity are verified without mutating the worktree
- AND it runs `task openspec:validate` and `task openspec:queue-test`
- AND it runs `task publish:verify-test`, which proves the publication evidence gate fails closed
- AND ATeam is v0.3.3, OpenSpec is 1.7.0, Task is 3.51.1, revive is 1.15.0, and Node is 22.20.0
- AND setup-go reads the Go version from `go.mod`
- AND official GitHub Actions use reviewed major-version tags rather than floating `latest` or a repository SHA policy

### Requirement: Merge check runs the review gate from the base commit

`Repository CI` SHALL run a `Merge check` job that executes `scripts/merge-check.sh` (`ateam review gate`) from the
pull request's base commit, and `Required` SHALL depend on it.

#### Scenario: The gate's script and policy come from the base commit

- GIVEN a pull request targets `main`
- WHEN the `Merge check` job runs
- THEN it checks out the base commit, not the pull request head, without persisted credentials
- AND it installs ATeam v0.3.3 and runs `scripts/merge-check.sh` with the pull request number
- AND its token permissions are read-only for contents, issues, and pull requests
- AND the workflow file itself is read from the pull request, so a change to it is caught by review only

#### Scenario: A code pull request without a review record fails Required

- GIVEN a non-draft code pull request has no `reviewed-by:` line in its description, or no review record naming one of
  its commits with no later content commit
- WHEN `Merge check` runs
- THEN `Merge check` fails and `Required` fails
- AND after an approving record is posted, rerunning the failed job lets `Required` pass

#### Scenario: A code pull request whose latest review record does not approve fails Required

- GIVEN a non-draft code pull request whose latest review record names one of its commits
- WHEN that record has no `Verdict:` line outside code fences, more than one, or one that is not `Verdict: APPROVE`
- THEN `Merge check` fails and `Required` fails (ATeam v0.3.3, `C360Studio/ateam#201`)
- AND whether the findings justify an approving verdict stays review only

#### Scenario: Marking a draft ready starts the gated run

- GIVEN a draft pull request, whose `Merge check` does not fail on a review finding
- WHEN it is marked ready for review
- THEN `Repository CI` runs on the `ready_for_review` event and `Merge check` applies the review check

#### Scenario: A push to main checks only the up-to-date rule

- GIVEN a push to `main`
- WHEN `Merge check` runs
- THEN `ateam review gate` reads the push event and applies neither the review check nor the known-flake check
