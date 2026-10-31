## ADDED Requirements

### Requirement: task validate is the read-only local mirror of Repository CI

`task validate` (`scripts/validate.sh`, aliased as `task verify`) SHALL run the commands of the `Repository CI` Go, UI,
and Governance lanes in order, SHALL fail closed when a prerequisite is missing or generated artifacts drift, and
SHALL leave tracked, staged, unstaged, and untracked worktree state unchanged.

#### Scenario: A passing run executes every lane and preserves a dirty worktree

- GIVEN a worktree with staged, unstaged, and untracked changes and every prerequisite present
- WHEN `scripts/validate.sh` runs
- THEN it executes the Go, UI, and Governance lane commands in their fixed order
- AND the worktree snapshot afterwards equals the snapshot before, and transient directories it created are removed

#### Scenario: A missing prerequisite fails closed

- GIVEN a required tool is absent from `PATH`, `ui/node_modules` is missing, or the Docker daemon is unreachable
- WHEN `scripts/validate.sh` runs
- THEN it fails with a message naming the missing prerequisite and leaves the worktree unchanged
- AND for a missing tool it runs no lane command

#### Scenario: Drift or a lane failure stops the run without writes

- GIVEN regenerated schemas or OpenAPI differ from the committed ones, or a lane command fails
- WHEN `scripts/validate.sh` runs
- THEN it fails, and the worktree is unchanged

#### Scenario: task verify and the pre-push instruction point at the same gate

- GIVEN `Taskfile.yml`, `scripts/verify.sh`, and `AGENTS.md`
- WHEN they are read
- THEN `task verify` delegates to `scripts/validate.sh`, and `AGENTS.md` names `task validate` as the single command
  to run before pushing
