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
