# runtime-composition Specification

## Purpose
Keep SemTeams runtime registration, generated interfaces and contract qualification on one explicit product-owned catalog while preserving the frozen framework behavior.
## Requirements
### Requirement: Product-owned registration boundary

SemTeams SHALL explicitly register only the 9 composed component factories (`graph-ingest`, `graph-query`,
`rule-processor`, `objectstore`, `agentic-loop`, `agentic-model`, `agentic-tools`, `agentic-dispatch`, and
`graph-gateway`), the five required framework payload owner families (`message`, `agentic`, `storage/objectstore`,
`pkg/lifecycle`, and `graph/inference`), and its boot predicate vocabularies (`vocabulary/agentic` plus
`cmd/semteams/vocab` via `vocabulary.Register`) using their package-owned registration functions, without importing
dropped framework aggregator or helper packages (`componentregistry`, `payloadbuiltins`, `vocabulary/builtins`,
`pkg/logging`). Names, factory metadata, schemas, port declarations, registered payload type identities, concrete
factory types, metadata, decoding behavior, declared boot predicates for all wired rules and projection contracts, and
`log/slog` stdout + NATS `logs.<LEVEL>.<source>` wire subject routing, entry keys, level filtering, and
`log-forwarder` `exclude_sources` prefix filtering SHALL be preserved (resolving `slog.LogValuer` attributes, keying
`AddSource` on parsed debug level, and bounding async NATS publish deadlines). Uncomposed framework component
factories, framework payload owners with no composed consumer (`processor/gated-dag`, `governance`), and unused
reference rule-pack vocabularies (`vocabulary/rulepacks`) SHALL NOT be registered by the product composition root.
Product bootstrap and generated component interfaces SHALL use the same 9-factory component catalog. Registration
errors SHALL fail startup rather than leave a partial admitted catalog.

#### Scenario: Supported bootstrap admission

- **GIVEN** either the production or mock bootstrap and the explicit 9-factory SemTeams catalog
- **WHEN** the composition is admitted
- **THEN** every configured factory is registered in the catalog, every catalog factory is exercised by the shipped bootstrap compositions, and its existing composition contract is retained
- **AND** registered availability does not claim that a parked pack is running

#### Scenario: Registration fails

- **GIVEN** a conflicting or nil registration
- **WHEN** the product constructs its catalog or payload registry
- **THEN** it returns an error that identifies the failed component or every colliding payload owner
- **AND** startup does not continue with a partial catalog

#### Scenario: Generated interfaces match the product

- **GIVEN** the explicit 9-factory product component catalog
- **WHEN** component schemas, OpenAPI, and frontend OpenAPI types are generated
- **THEN** they describe the exact 9 registered factories used by the product bootstrap with no orphaned uncomposed component schemas
- **AND** regeneration is reproducible

#### Scenario: Frozen public catalog preservation

- **GIVEN** the package-owned registration functions of the 9 composed factories and five framework payload owners from the frozen baseline
- **WHEN** the product-owned catalog and payload registry are constructed without `componentregistry` or `payloadbuiltins`
- **THEN** every admitted component retains its metadata, schema, and port declarations
- **AND** all five framework payload owners and existing product payload owners retain their registrations and wire round-trip decoding contracts
- **AND** uncomposed factories and payload owners with no composed consumer (`processor/gated-dag`, `governance`) remain unregistered

#### Scenario: Boot vocabulary and logger preservation without unported helpers

- **GIVEN** the SemTeams composition root without `vocabulary/builtins`, `vocabulary/rulepacks`, or `pkg/logging`
- **WHEN** boot vocabulary registration (`vocabulary/agentic.Register()` + `cmd/semteams/vocab.Register()` via `vocabulary.Register`) and product-owned `setupLogger` run
- **THEN** every predicate referenced by the shipped bootstrap-wired rules and projection contracts satisfies `vocabulary.RequireDeclaredPredicate` with preserved `PredicateMetadata` (`Description`, `DataType`) while `vocabulary/rulepacks`-only predicates remain unregistered
- **AND** `setupLogger` preserves JSON/text stdout formatting, level filtering, `source > component > service > "system"` subject routing to `logs.<LEVEL>.<source>`, and `log-forwarder` `exclude_sources` prefix filtering

### Requirement: Single-authority extraction admission

An agent-contract extraction design SHALL measure production and test dependency closure, private dependencies and
concrete-type consumers at an exact frozen source revision before selecting its implementation boundary. The design
SHALL retain one authoritative concrete Go type family and identity-bearing shared values, including error sentinels,
across payload registration and consumers, preserve
source algorithms, meaningful tests and licensing, and distinguish engine-owned dependencies from agent-runtime
ownership. Unimplemented SemEngine APIs and generic durability SHALL remain named integration dependencies rather
than being replaced with speculative product-local primitives.

#### Scenario: Nominal type boundary crosses the proposed cut

- **GIVEN** a proposed extracted payload type and an upstream consumer of the original concrete Go type
- **WHEN** the extraction boundary is evaluated
- **THEN** the design SHALL identify the incompatible concrete-type relationship and choose a single-authority transition
- **AND** a separately defined lookalike type or type-alias facade SHALL NOT be represented as completed ownership transfer

#### Scenario: An error sentinel crosses the proposed cut

- **GIVEN** retained code comparing an exported agent-contract sentinel with `errors.Is`
- **WHEN** a design relocates the contract's definitions
- **THEN** it SHALL preserve the sentinel identity at that consumer boundary or migrate the consumer in the same cut
- **AND** a fresh error with the same text SHALL NOT be treated as equivalent authority

#### Scenario: Measured evidence has a limited build context

- **GIVEN** a dependency or consumer measurement with declared platform, tags and test scope
- **WHEN** its result supports extraction admission
- **THEN** the design SHALL retain exact source identities, commands, compiler errors and scope limitations
- **AND** failed or partial contexts SHALL NOT be counted as qualified closure

#### Scenario: Engine mechanism is not yet implemented

- **GIVEN** an agent recovery or rule-action dependency on an unsettled SemEngine contract
- **WHEN** the next implementation slice is proposed
- **THEN** the dependency and required proving checks SHALL remain explicit
- **AND** this design SHALL NOT claim a shipped engine API or authorize a competing generic journal

### Requirement: Configured model retry qualification

SemTeams SHALL qualify the existing model retry boundary through the product's component and payload setup and
public component manager, using each shipped bootstrap's model configuration with an explicitly external fixture
request input, isolated real NATS and a controlled local HTTP fixture. Qualification SHALL preserve shipped files,
retry settings, all other port fields and canonical upstream payload types, correlate requests and responses,
synchronize running state and response consumption before publication, and stop/join owned work before final
provider-call assertions. This requirement qualifies an isolated composed model; it adds no runtime retry or delivery
behavior and does not replace full-bootstrap admission checks.

#### Scenario: External fixture input is declared without changing the shipped contract

- **GIVEN** the model configuration derived from either shipped bootstrap
- **WHEN** the isolated model is composed with a fixture request publisher outside the component graph
- **THEN** qualification SHALL first assert the shipped `agent.request` input is required and non-external, then set only its `External` marker in the test copy
- **AND** it SHALL preserve input subjects, `Required`, stream, retry settings and output fields without adding a fake publisher component

#### Scenario: Production retry budget reaches successful completion

- **GIVEN** the shipped production model configuration with three total attempts and a non-streaming fixture returning HTTP 503, 503, then 200
- **WHEN** the running composed model receives a canonical upstream request through its configured AGENT stream port
- **THEN** qualification SHALL observe three HTTP attempts and decode a canonical upstream `AgentResponse` with matching request identity, expected response subject and successful content
- **AND** the response SHALL report `RetryCount: 2`

#### Scenario: Mock retry budget preserves its single-attempt boundary

- **GIVEN** the shipped mock model configuration with one total attempt and the same initially failing fixture
- **WHEN** the running composed model receives a canonical upstream request through its configured port
- **THEN** qualification SHALL observe one HTTP attempt and a correlated canonical upstream typed error response on the expected response subject
- **AND** the test SHALL NOT silently increase the mock retry budget to reach success

#### Scenario: Qualification detects configuration or registration regression

- **GIVEN** the composed-model qualification tests
- **WHEN** a controlled mutation reduces the production retry budget or removes the configured model factory
- **THEN** the relevant test SHALL fail on the changed behavior or missing composition boundary
- **AND** both mutations SHALL be restored before final qualification evidence is accepted

#### Scenario: Qualification scope stays explicit

- **GIVEN** passing synchronized local HTTP and NATS fixture tests
- **WHEN** their results are recorded
- **THEN** the evidence SHALL identify exact source/dependency inputs, commands, outcomes and test limitations
- **AND** it SHALL NOT claim streaming, live-provider quality, restart recovery, retained-state migration or exactly-once delivery

### Requirement: Composed loop first-request qualification

SemTeams SHALL qualify the task-to-first-model-request boundary using the shipped graph-ingest and agentic-loop
configuration, product registration/payload helpers, public managers and real local NATS. The qualification SHALL
preserve real graph ownership and identify configuration adaptations, observed runtime boundaries and evidence limits.
It SHALL NOT represent a composed first-request result as provider, continuation, recovery or future-engine qualification.

#### Scenario: Fixture publishers enter a real two-component composition

- **GIVEN** shipped graph-ingest and teams-loop parameters, platform, streams, subjects and budgets
- **WHEN** the qualification admits the isolated composition with fixture-owned input publishers
- **THEN** only truthful input External markers SHALL differ, with exact reversal to the original component configuration proved
- **AND** typed mutation and graph queries SHALL be served by the real graph owner rather than fabricated graph state or responders
- **AND** the evidence SHALL distinguish this composition from unchanged full-bootstrap admission

#### Scenario: A new root emits its first model request

- **GIVEN** both components have reached Started state and independently expected fixture values
- **WHEN** a canonical new-root task enters the loop
- **THEN** the qualification SHALL establish typed loop birth and capture the first canonical AgentRequest with matching task/loop/request identity
- **AND** it SHALL assert tool-choice mode/function name and complete response-format values against independent expectations
- **AND** it SHALL use bounded synchronization and cleanup, without inferring broker effect uniqueness or downstream provider behavior

#### Scenario: An independent fresh root omits both knobs

- **GIVEN** an earlier root carrying tool choice or response format on the same running loop component
- **WHEN** a separate new-root task omits both fields
- **THEN** its first request SHALL carry neither field
- **AND** the qualification SHALL NOT interpret this as same-loop nil-reset, continuation or run-inheritance proof
- **AND** live-source versus supplemental fixture classifications SHALL remain explicit

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

