# Trim runtimecatalog to the 9 composed factories and 5 payload owners

Approved-by: Coby Leuschke (2026-11-08)

Track: quick. Updates one existing Requirement (`Product-owned registration boundary`) in `openspec/specs/runtime-composition/spec.md` to match the owner-ruled 9-factory / 5-owner catalog cut (`#399`, `C360Studio/semsource#225`); adds no new Requirement, exported surface, durability change, or gate-file edit.

## Why

`internal/runtimecatalog` registers all 27 SemStreams factories and 7 payload owners from `8b99efe9c66a`, while `configs/flow-bootstrap.json` and `configs/e2e-flow-bootstrap.json` compose only 9 factories (`graph-ingest`, `graph-query`, `rule-processor`, `objectstore`, `agentic-loop`, `agentic-model`, `agentic-tools`, `agentic-dispatch`, `graph-gateway`). `payloads.go` also registers `processor/gated-dag` and `governance` with no composed consumer, and `catalog_test.go` imports `componentregistry` and `payloadbuiltins`, both dropped in SemEngine (`C360Studio/semengine#3`).

Trimming the catalog now on `main` (substrate-neutral) eliminates 21 Group 4 SemStreams import paths (`18` uncomposed component packages + `componentregistry` + `payloadbuiltins` + `governance`) ahead of the `#403` acceptance floor and Milestone 2 port.

## What changes

- Trim `internal/runtimecatalog/components.go` from 27 factories to the 9 composed factories (`graph-ingest`, `graph-query`, `rule-processor`, `objectstore`, `agentic-loop`, `agentic-model`, `agentic-tools`, `agentic-dispatch`, `graph-gateway`).
- Trim `internal/runtimecatalog/payloads.go` from 7 to 5 framework payload owners (`message`, `agentic`, `storage/objectstore`, `pkg/lifecycle`, `graph/inference`), dropping `processor/gated-dag` and `governance`.
- Replace `componentregistry` and `payloadbuiltins` comparisons in `internal/runtimecatalog/catalog_test.go` with package-direct 9-factory / 5-owner and shipped-bootstrap parity tests; update `internal/runtimecatalog/payloads_test.go`, `cmd/semteams/payload_registry_test.go`, `cmd/semteams/main.go`, and `cmd/semteams/portresolver/portresolver.go` comments/assertions.
- Remove the 18 uncomposed `schemas/*.v1.json` files (`cmd/openapi-generator` pruning stale `*.v1.json` outputs), regenerate `schemas/`, `specs/openapi.v3.yaml`, `ui/src/lib/types/api.generated.ts`, and `docs/migrations/semengine-port-ledger.md`, and record `go list -deps ./cmd/semteams` closure before and after.

## Non-goals

- Changing live runtime wiring in `cmd/semteams/main.go`, `configs/flow-bootstrap.json`, `configs/e2e-flow-bootstrap.json`, live category packs (`research`, `autoresearch`), or parked dev packs (`create-change`, `proof-readiness`, `dev-from-task`, `dev-via-test`).
- Replacing `natsclient.NewTestClient` (`#400`), decoupling `pkg/logging` or `vocabulary/builtins` (`#401`), or absorbing agent-runtime packages (`#402`, `#404`–`#407`).

## Impact

- Shrinks `internal/runtimecatalog`, `schemas/`, `specs/openapi.v3.yaml`, `ui/src/lib/types/api.generated.ts`, and `go list -deps ./cmd/semteams` closure to SemTeams's real composed surface, unblocking `#403`.

Closes #399; addresses #412.
