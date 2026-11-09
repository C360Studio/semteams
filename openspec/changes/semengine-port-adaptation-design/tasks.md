# Tasks: SemEngine port manifest and adaptation design (`#402`)

## 1. Design Blueprint and Owner Gate

- [x] 1.1 Record owner rulings (`Approved-by: Coby Leuschke (2026-11-09)`) across `proposal.md`, `design.md`, and `specs/runtime-composition/spec.md` for `internal/` package layout (D1), `ExecutorRegistry` closure DI (`cmd/semteams/main.go:190,207-214`, `internal/runtimecatalog/components.go:26-49`, D2), substrate/transitive helper adaptations (`semstreams:processor/agentic-loop/summarizer.go:20-55`, `semstreams:processor/agentic-loop/config.go:429`, `semstreams:service/milestone_service.go:22-43`, D3), and trimmed `POST /graphql` contract (`configs/flow-bootstrap.json:746,768`, `ui/src/lib/services/agentApi.ts:161-215`, `ui/src/lib/services/graphApi.ts:169-354`, D4); verify line budgets (`proposal.md` <= 60 lines, `design.md` <= 120 lines) and `task openspec:validate`.

## 2. Provenance Manifest Generator, Blueprint Reference, and Ledger Sync

- [ ] 2.1 Implement stdlib-only `scripts/generate-semengine-port-manifest.go` (`//go:build ignore`) reading `github.com/c360studio/semstreams@v1.0.0-beta.162.0.20260930150212-8b99efe9c66a` (`go.mod:7`), verifying module version/`h1:` sum/no-replace and MIT `LICENSE`, walking all 21 absorbed units into deterministic JSON + Markdown summary output, asserting zero imports of `agentic/{identity,research}` or `pkg/fusion`, classifying all external `semstreams/...` imports fail-closed, and recording `internal/agentic/looptoken/token_test.go`.
- [ ] 2.2 Generate `docs/migrations/semengine-port-manifest/manifest.json` and author `docs/migrations/semengine-port-manifest/README.md` with the generated summary/import-disposition block plus concrete Milestone 2 reference tables (`internal/` package map, `ExecutorRegistry` registration signatures, substrate/transitive adaptation recipes, and UI `/graphql` + `TrajectoryPage` query/type contract for `#406`), and sync Group 3 destination notes in `scripts/measure-semengine-port-ledger.go` + `docs/migrations/semengine-port-ledger.md`.

## 3. Contract Tests and Verification

- [ ] 3.1 Add `test/contract/semengine_port_manifest_test.go` (citing `openspec/specs/runtime-composition/spec.md#Requirement: Absorbed agent-runtime and gateway port blueprint`) asserting stdlib-only generator AST imports, live module-cache parity with `manifest.json` and `README.md`, independent SHA-256/MIT oracle verification, fail-closed rejection of missing MIT notice / forbidden `agentic/research` import / unclassified SemStreams import, and mutation sensitivity checks.
- [ ] 3.2 Run `task test:spec`, `task openspec:validate`, and full `task validate` cleanly on the branch with zero runtime, schema, or OpenAPI drift.
