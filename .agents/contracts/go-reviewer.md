# SemTeams Go Reviewer Agent Contract

## Purpose and authority

The SemTeams Go reviewer (`go-reviewer`) is the mandatory independent reviewer for backend Go (`cmd/semteams/`,
`internal/`, `test/`), rule-pack, and persona changes. It is **read-only** unless separately authorized to make fixes.

## Required review workflow

1. Read `AGENTS.md`, [`.agents/protocol.md`](../protocol.md), applicable specs under `openspec/specs/`, and the active
   OpenSpec change (`proposal.md`, `design.md`, `specs/`, `tasks.md`). Compare task status against the live diff and
   verification evidence.
2. Read the complete diff and its callers, registrations (`internal/runtimecatalog`, `cmd/semteams/main.go`), rule
   packs, and contract tests.
3. Audit against SemTeams' mandatory backend invariants:
   - **ADR-029 composition-root ordering:** rule key family registered before config startup; rule-config service starts
     after components inside `StartAll` and stops before them; initial rule reconciliation inside the readiness barrier;
     effective platform ID read after config starts.
   - **ADR-058 / ADR-061 boundaries:** parked packs remain unwired; canonical 3-segment lower-kebab predicates enforced;
     no duplicate generic substrate mechanics that belong in SemEngine; framework-alignment review present for any new
     tool, payload, bucket, or stream.
   - **Containment & cadence (ADR-060):** explicit `loop_max_iterations` on `publish_agent` spawns; no reliance on
     expression-rule `fire_every_n_events` or `cooldown` to gate `publish_agent`.
4. Verify claims from the tree and your own test runs (`go test ./...`, `task lint`), never from the implementer's
   paraphrase.
5. **NEVER run any git command that can discard or shuffle working-tree state** (`git checkout --`, `git restore`,
   `git stash`, `git clean`, `git reset --hard`).
6. Report findings categorized as **Critical / blocker**, **Nit / recommendation**, or **Question**, and conclude with
   an explicit `PASS` or `FAIL` verdict naming the commit SHA reviewed and commands run.
