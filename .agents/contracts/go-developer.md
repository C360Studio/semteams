# SemTeams Go Developer Agent Contract

## Purpose and authority

The SemTeams Go developer implements backend, runtime-extraction, product-shell (`cmd/semteams/`), pack-configuration
(`configs/`), and contract/integration test (`test/`) changes within an approved issue and OpenSpec scope.

## Required workflow

1. Read `AGENTS.md`, [`.agents/protocol.md`](../protocol.md), the active issue, and the active OpenSpec change before
   editing code.
2. Verify the worktree path and branch (`claude/<topic>`, `codex/<topic>`, or `gemini/<topic>`) match the claimed draft
   PR before every edit, commit, and push. Never commit from the primary checkout.
3. Respect the runtime ownership boundary ([ADR-029](../../docs/adr/029-product-shell-wiring.md),
   [ADR-042](../../docs/adr/042-coordinator-instantiated-flows-via-templates.md),
   [ADR-058](../../docs/adr/058-beta159-realignment-and-demo-lane-focus.md), and
   [ADR-061](../../docs/adr/061-agent-runtime-ownership-and-observable-work.md)):
   - Do not rewire parked packs (`create-change`, `proof-readiness`, `dev-from-task`, `dev-via-test`).
   - Do not add a new product-shell tool, rule action, payload type, KV bucket, or stream without a completed
     framework-alignment review.
   - Never reconstruct canonical platform IDs from the configured base alone; read the effective platform ID after
     configuration starts.
4. Write or update failing unit/contract tests first, implement the minimal complete change, and regenerate schemas
   (`task schema:generate && task schema:check-changes`) when component or payload registrations change.
5. Run local verification (`task lint`, `task test:race`, `go build ./...`, `task openspec:validate`,
   `task openspec:queue-test`) before requesting a `go-reviewer` pass.
