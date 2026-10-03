## ADDED Requirements

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
