# runtime-composition Specification

## Purpose
Keep SemTeams runtime registration, generated interfaces and contract qualification on one explicit product-owned catalog while preserving the frozen framework behavior.
## Requirements
### Requirement: Product-owned registration boundary

SemTeams SHALL explicitly register all 27 factories and all seven framework payload owner families in the
frozen `8b99efe9` catalog using their package-owned registration functions. Names, factory metadata, schemas, port
declarations, registered payload type identities, concrete factory types, metadata, and previously supported decoding
behavior SHALL be preserved, along with existing product payload families.
This foundation SHALL NOT prune the catalog or change service/vocabulary registration. Product bootstrap and generated component interfaces SHALL use
the same component catalog. Registration errors SHALL fail startup rather than leave a partial admitted catalog.

#### Scenario: Supported bootstrap admission

- **GIVEN** either the production or mock bootstrap and the explicit SemTeams catalog
- **WHEN** the composition is admitted
- **THEN** every configured factory is registered and its existing composition contract is retained
- **AND** registered availability does not claim that a component or parked pack is running

#### Scenario: Registration fails

- **GIVEN** a conflicting registration
- **WHEN** the product constructs its catalog or payload registry
- **THEN** it returns an error that identifies the failed registration
- **AND** startup does not continue with a partial catalog

#### Scenario: Generated interfaces match the product

- **GIVEN** the explicit product component catalog
- **WHEN** component schemas and OpenAPI are generated
- **THEN** they describe the same registered factories used by the product bootstrap
- **AND** regeneration is reproducible

#### Scenario: Frozen public catalog preservation

- **GIVEN** the qualified frozen component catalog and existing generated schemas
- **WHEN** the product-owned registration boundary replaces the framework aggregator
- **THEN** every previously advertised component retains its metadata, schema, and port declarations
- **AND** all seven framework payload owners and existing product payload owners retain their registrations and existing decoding contracts
- **AND** catalog membership does not imply active bootstrap wiring or SemEngine admission

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
