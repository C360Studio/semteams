# Tasks: Fold Program Pulse (#267) into the research pack with `github_read` and `/pulse`

- [x] 1. Author OpenSpec proposal, design, and `program-pulse-github-read` spec delta (`openspec/changes/program-pulse-research-profile/`).
- [x] 2. Register the `/pulse` team-hint command in `cmd/semteams/product_commands.go` and add `/pulse` + the `Pulse` action chip in `ui/src/lib/services/slashCommands.ts` and `ui/src/lib/components/layout/ChatBar.svelte` (with unit tests).
- [x] 3. Add `"github_read"` to `configs/rules/research/02-plan-to-gather.json` and update `coordinator`, `researcher-research-plan`, `researcher-research-gather`, and `researcher-research-synthesize` persona fragments + `configs/rules/research/README.md` for portfolio/project Program Pulse synthesis.
- [x] 4. Add deterministic read-only GitHub REST endpoints to `test/e2e/mock/openai_server.go`, wire `SEMTEAMS_PORTFOLIO_PATH` and `SEMTEAMS_GITHUB_API_URL` in `ui/docker-compose.agentic-e2e.yml`, and add `test/fixtures/journeys/program-pulse.yaml` + `github_read` validation in `test/contract/journey_emit_args_contract_test.go`.
- [x] 5. Update `docs/adr/060-program-manager-direction-and-research-design.md`, `docs/product/program-manager.md`, `AGENTS.md`, and `CLAUDE.md` to record `/work` (structural state) + `research`/`/pulse` with `github_read` (semantic Program Pulse).
- [x] 6. Run `go-reviewer` and `svelte-reviewer`, address findings, and verify all Go, UI, schema, and OpenSpec CI gates.
