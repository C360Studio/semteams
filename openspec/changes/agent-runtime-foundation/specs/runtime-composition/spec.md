## ADDED Requirements

### Requirement: Product-owned registration boundary

SemTeams SHALL explicitly register its supported component and framework payload families using the frozen
SemStreams package-owned registration functions. Product bootstrap and generated component interfaces SHALL use
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
