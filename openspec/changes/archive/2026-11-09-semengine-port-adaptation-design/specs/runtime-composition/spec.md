## ADDED Requirements

### Requirement: Absorbed agent-runtime and gateway port blueprint

SemTeams SHALL maintain a machine-verified source-to-destination provenance manifest and adaptation blueprint at frozen
SemStreams `v1.0.0-beta.162.0.20260930150212-8b99efe9c66a` (`8b99efe9c66a4faa4fa509f9f62cc6bad8392128`,
`h1:WAlGLMjE/3EnYyeoqounCfYnVyIB3igMlG+FLP1Iq+o=`) for every package absorbed during the SemEngine port, mapping all
absorbed packages under `internal/` (`internal/agentic/...`, `internal/persona`, and `internal/gateway/graphgateway`),
verifying upstream MIT copyright preservation, classifying every external `github.com/c360studio/semstreams/...` import
across the absorbed source set fail-closed, and specifying explicit `ExecutorRegistry` injection, substrate seam
adaptations, and the trimmed `POST /graphql` + `TrajectoryPage` contract before Milestone 2 swaps `go.mod`.

#### Scenario: Machine-verified frozen provenance and MIT notice manifest

- **GIVEN** the Go module cache entry for `github.com/c360studio/semstreams@v1.0.0-beta.162.0.20260930150212-8b99efe9c66a` without a `go.mod` `replace` directive
- **WHEN** `scripts/generate-semengine-port-manifest.go` inspects the 21 absorbed package/slice units and upstream `LICENSE` and `TestSemEnginePortManifest` verifies the committed manifest artifacts
- **THEN** the generator verifies the module version and `h1:` sum, confirms `LICENSE` carries the C360 MIT copyright and permission notice, and emits deterministic JSON and Markdown recording every source file's byte size, SHA-256 digest, `internal/` destination path, and adaptation class (including `internal/agentic/looptoken/token_test.go` as a required new test target alongside `internal/looptoken/token.go`)
- **AND** the generator fails closed with a non-zero exit code if a `go.mod` `replace` directive is active, the MIT notice is missing, an unaccounted donor file or subdirectory is present, or any absorbed `.go` file imports excluded donor subpackages (`agentic/identity`, `agentic/research`, `pkg/fusion`) or an unclassified `github.com/c360studio/semstreams/...` package
- **AND** `TestSemEnginePortManifest` fails on any byte-for-byte drift between live generator output and committed `docs/migrations/semengine-port-manifest/{manifest.json,README.md}` or any mismatch against independent Go module-cache byte-size and SHA-256 checks

#### Scenario: Explicit ExecutorRegistry injection and substrate seam blueprint

- **GIVEN** SemEngine's removal of `component.Dependencies.ToolRegistry` (`semengine#29`), publication of `pkg/lifecyclecleanup.RollbackFailedStart` (`semengine#77`), Tier-0 exclusion of `graph/llm` (`semengine#32`, `#484`), internal `semengine.graph.mutation` protocol (`semengine#69`, `#137`), ADR-113 lifecycle contract, and `E1`–`E4` rule-action extension seam (`semengine#25`–`#28`)
- **WHEN** the Milestone 2 adaptation blueprint is specified
- **THEN** `internal/runtimecatalog` injects `*tools.ExecutorRegistry` explicitly into `agentic-tools`, `agentic-loop`, and `agentic-dispatch` factory closures (while keeping nil-registry schema/OpenAPI registration offline-safe)
- **AND** failed-start rollback maps to `semengine/pkg/lifecyclecleanup.RollbackFailedStart` with ADR-113 component-owned run contexts cancelled by `Stop(ctx)`, `agentic-loop` summarization maps from `graph/llm` to `internal/agentic/modelwire` + `semengine/model.NewHTTPClient`, graph-mutation callers map to `"semengine.graph.mutation"` with ADR-110 `Source` and `Timestamp` provenance, and `publish_agent` maps to `internal/agentic/rulefamily` without inventing a competing generic execution journal ahead of `semengine#24`

#### Scenario: Trimmed GraphQL and trajectory gateway contract

- **GIVEN** the SvelteKit UI's observed GraphQL calls across `ui/src/**` and E2E journeys (`trajectory`, `entitiesByPrefix`, `pathSearch`, and unit-tested `entity` with `kvRevision`)
- **WHEN** the SemTeams `internal/gateway/graphgateway` replacement contract is specified for `#406`
- **THEN** the gateway preserves component name `"graph-gateway"`, mounts `POST /graphql` via `RegisterHTTPHandlers`, declares only `graph_queries` (`graph.query.*`) and `agentic_queries` (`agentic.query.*`), unwraps SemEngine's 4-key `graph.QueryResponse` envelope, maps `ExactEntity` `kv_revision` to GraphQL `kvRevision`, and preserves `agentic.TrajectoryPage` pagination and `trajectory not found` handling
- **AND** uncomposed `graph_index_queries` (`graph.index.query.*`), `globalSearch`/`searchGraph`, `/mcp` (`mcp_path`), `/inference/*`, and `GRAPH_STATUS` readiness watchers are excluded from the trimmed gateway and scheduled for removal with dead UI `globalSearch`/SSR-AI GraphQL client code in `#406`
