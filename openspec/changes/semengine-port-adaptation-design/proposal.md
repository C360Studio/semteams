# Source-to-destination manifest and adaptation design for absorbed agent runtime and graph-gateway

Approved-by: Coby Leuschke (2026-11-09)

Track: design. Adds `Requirement: Absorbed agent-runtime and gateway port blueprint` to `openspec/specs/runtime-composition/spec.md` and defines the Milestone 2 package layout, dependency-injection seams, and trimmed `/graphql` contract for `#402` (claimed by draft PR `#419`).

## Why

Under ADR-061 §4 and ADR-062 §3, SemTeams owns the agent runtime and HTTP/GraphQL product gateway while SemEngine owns the shared substrate, and no running binary may import both `semstreams` and `semengine`. Before Milestone 2 (`#404`–`#407`) swaps `go.mod` on the qualification branch, SemTeams needs a machine-verified SHA-256 + MIT copyright provenance manifest from frozen SemStreams `8b99efe9c66a4faa4fa509f9f62cc6bad8392128` plus settled decisions for target package paths, `ExecutorRegistry` wiring (`semengine#29`), substrate/transitive helper seams (`semengine#77`, `#25`–`#28`, `#32`, `#69`), and the trimmed `/graphql` + `TrajectoryPage` surface (`#406`).

## What changes

- Add `scripts/generate-semengine-port-manifest.go` (`//go:build ignore`, Go stdlib only) and `docs/migrations/semengine-port-manifest/` (`manifest.json` + `README.md`) recording SHA-256 digests, byte sizes, MIT copyright verification, `internal/...` destinations, adaptation classes, and fail-closed external-import dispositions for all 21 absorbed frozen-SemStreams units (while proving zero imports of excluded `agentic/identity` and `agentic/research` and recording the new `internal/agentic/looptoken/token_test.go` target).
- Lock in the `internal/` package layout (`internal/agentic/...`, `internal/persona`, `internal/gateway/graphgateway`), explicit `*tools.ExecutorRegistry` injection on `runtimecatalog.RegisterComponents`, substrate seam adaptations (`pkg/lifecyclecleanup.RollbackFailedStart`, `modelwire` summarizer replacing `graph/llm`, `"semengine.graph.mutation"` client seam, `publish_agent` `E1`–`E4` registration), and the trimmed `POST /graphql` schema (`trajectory`, `entitiesByPrefix`, `pathSearch`, `entity` with `kvRevision` over `kv_revision`; dropping `globalSearch`, `/mcp`, `/inference/*`, `GRAPH_STATUS`, `graph_index_queries`, and `mcp_path`).
- Enforce manifest/documentation parity and fail-closed mutation sensitivity via `test/contract/semengine_port_manifest_test.go`, and update Group 3 destination notes in `scripts/measure-semengine-port-ledger.go` + `docs/migrations/semengine-port-ledger.md`.

## Non-goals

- Copying runtime packages, switching `go.mod` off frozen SemStreams `v1.0.0-beta.162.0.20260930150212-8b99efe9c66a`, or changing live runtime behavior on `main` (`research` and `autoresearch` stay live on frozen SemStreams; `create-change`, `proof-readiness`, `dev-from-task`, and `dev-via-test` stay parked per ADR-058).
- Inventing local generic execution durability ahead of `C360Studio/semengine#24` or modifying `configs/flow-bootstrap.json`, `configs/e2e-flow-bootstrap.json`, `schemas/`, `specs/openapi.v3.yaml`, or `ui/` before `#406`/`#407`.

## Impact

- Gives `#404`–`#407` and `#287` a deterministic, contract-tested manifest and exact seam blueprint so Milestone 2 executes mechanically once `setup-04a-06` pins.

Closes #402; addresses #412.
