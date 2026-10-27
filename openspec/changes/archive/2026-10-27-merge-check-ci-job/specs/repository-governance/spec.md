## ADDED Requirements

### Requirement: Merge check runs the review gate from the base commit

`Repository CI` SHALL run a `Merge check` job that executes `scripts/merge-check.sh` (`ateam review gate`) from the
pull request's base commit, and `Required` SHALL depend on it.

#### Scenario: The gate's script and policy come from the base commit

- GIVEN a pull request targets `main`
- WHEN the `Merge check` job runs
- THEN it checks out the base commit, not the pull request head, without persisted credentials
- AND it installs ATeam v0.3.2 and runs `scripts/merge-check.sh` with the pull request number
- AND its token permissions are read-only for contents, issues, and pull requests
- AND the workflow file itself is read from the pull request, so a change to it is caught by review only

#### Scenario: A code pull request without a review record fails Required

- GIVEN a non-draft code pull request has no `reviewed-by:` line in its description, or no review record naming one of
  its commits with no later content commit
- WHEN `Merge check` runs
- THEN `Merge check` fails and `Required` fails
- AND after the record is posted, rerunning the failed job lets `Required` pass
- AND the gate does not read `Verdict:` at ATeam v0.3.2 (`C360Studio/ateam#201`), so the verdict stays review only

#### Scenario: Marking a draft ready starts the gated run

- GIVEN a draft pull request, whose `Merge check` does not fail on a review finding
- WHEN it is marked ready for review
- THEN `Repository CI` runs on the `ready_for_review` event and `Merge check` applies the review check

#### Scenario: A push to main checks only the up-to-date rule

- GIVEN a push to `main`
- WHEN `Merge check` runs
- THEN `ateam review gate` reads the push event and applies neither the review check nor the known-flake check
