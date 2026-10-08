## ADDED Requirements

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
