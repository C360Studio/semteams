## ADDED Requirements

### Requirement: Merge check runs the review gate from the base commit

`Repository CI` SHALL run a `Merge check` job that executes `scripts/merge-check.sh` (`ateam review gate`) from the
pull request's base commit, and `Required` SHALL depend on it.

#### Scenario: A pull request cannot loosen its own gate

- GIVEN a pull request targets `main`
- WHEN the `Merge check` job runs
- THEN it checks out the base commit, not the pull request head, without persisted credentials
- AND it installs ATeam v0.3.2 and runs `scripts/merge-check.sh` with the pull request number
- AND its token permissions are read-only for contents, issues, and pull requests

#### Scenario: A code pull request without an approving record fails Required

- GIVEN a code pull request has no approving `reviewed-by:` record that names its head
- WHEN `Merge check` runs
- THEN `Merge check` fails and `Required` fails
- AND after an approving record is posted, rerunning the failed job lets `Required` pass

#### Scenario: A push to main checks only the up-to-date rule

- GIVEN a push to `main`
- WHEN `Merge check` runs
- THEN `ateam review gate` reads the push event and applies neither the review check nor the known-flake check
