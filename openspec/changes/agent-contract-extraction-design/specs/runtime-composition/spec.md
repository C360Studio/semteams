## ADDED Requirements

### Requirement: Single-authority extraction admission

An agent-contract extraction design SHALL measure production and test dependency closure, private dependencies and
concrete-type consumers at an exact frozen source revision before selecting its implementation boundary. The design
SHALL retain one authoritative concrete Go type family across payload registration and its consumers, preserve
source algorithms, meaningful tests and licensing, and distinguish engine-owned dependencies from agent-runtime
ownership. Unimplemented SemEngine APIs and generic durability SHALL remain named integration dependencies rather
than being replaced with speculative product-local primitives.

#### Scenario: Nominal type boundary crosses the proposed cut

- **GIVEN** a proposed extracted payload type and an upstream consumer of the original concrete Go type
- **WHEN** the extraction boundary is evaluated
- **THEN** the design SHALL identify the incompatible concrete-type relationship and choose a single-authority transition
- **AND** a separately defined lookalike type or type-alias facade SHALL NOT be represented as completed ownership transfer

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
