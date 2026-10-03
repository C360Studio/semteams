## ADDED Requirements

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
