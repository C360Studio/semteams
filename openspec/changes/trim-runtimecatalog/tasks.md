## 1. Approval

- [x] 1.1 Owner approves `proposal.md` (`Approved-by: Coby Leuschke (2026-11-08)`) in `#399` and Enjoy conversation `Pickup Next Milestone 1` (`quick` track, run through to cross-reviewed merge-ready PR).

## 2. Catalog trim and aggregator-free tests

- [ ] 2.1 Trim `internal/runtimecatalog/components.go` to the 9 composed factories (`graph-ingest`, `graph-query`, `rule-processor`, `objectstore`, `agentic-loop`, `agentic-model`, `agentic-tools`, `agentic-dispatch`, `graph-gateway`) and `internal/runtimecatalog/payloads.go` to the 5 required framework payload owners (`message`, `agentic`, `storage/objectstore`, `pkg/lifecycle`, `graph/inference`), updating doc comments in `cmd/semteams/main.go` and `cmd/semteams/portresolver/portresolver.go`.
- [ ] 2.2 Replace `componentregistry` and `payloadbuiltins` imports in `internal/runtimecatalog/catalog_test.go` with package-direct 9-factory / 5-owner parity, collision, and shipped-bootstrap (`configs/flow-bootstrap.json` + `configs/e2e-flow-bootstrap.json`) coverage checks; update `internal/runtimecatalog/payloads_test.go` and `cmd/semteams/payload_registry_test.go` (`// spec:` citations + `task mutate:check` proof).

## 3. Generated artifacts, crosswalk sync, and verification

- [ ] 3.1 Prune uncomposed `schemas/*.v1.json` files during `task schema:generate`, regenerate `schemas/`, `specs/openapi.v3.yaml`, and `ui/src/lib/types/api.generated.ts`, regenerate the `docs/migrations/semengine-port-ledger.md` crosswalk block, and record `go list -deps ./cmd/semteams` package closure before and after in `openspec/changes/trim-runtimecatalog/notes/closure.md`.
- [ ] 3.2 Run `task spec:check` and read-only `task validate` on a clean worktree before requesting cross-provider `go-reviewer` review.
