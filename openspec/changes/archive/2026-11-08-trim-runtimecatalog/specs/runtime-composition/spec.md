## MODIFIED Requirements

### Requirement: Product-owned registration boundary

SemTeams SHALL explicitly register only the 9 composed component factories (`graph-ingest`, `graph-query`,
`rule-processor`, `objectstore`, `agentic-loop`, `agentic-model`, `agentic-tools`, `agentic-dispatch`, and
`graph-gateway`) and the five required framework payload owner families (`message`, `agentic`, `storage/objectstore`,
`pkg/lifecycle`, and `graph/inference`) using their package-owned registration functions, without importing dropped
framework aggregator packages (`componentregistry`, `payloadbuiltins`). Names, factory metadata, schemas, port
declarations, registered payload type identities, concrete factory types, metadata, and decoding behavior for all
admitted factories, framework payload owners (including internal lifecycle and hierarchy births), and existing product
payload families SHALL be preserved. Uncomposed framework component factories and framework payload owners with no
composed consumer (`processor/gated-dag`, `governance`) SHALL NOT be registered in the product catalog. Product
bootstrap and generated component interfaces SHALL use the same 9-factory component catalog. Registration errors SHALL
fail startup rather than leave a partial admitted catalog.

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

