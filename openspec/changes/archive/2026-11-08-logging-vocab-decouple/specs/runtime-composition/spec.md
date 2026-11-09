## MODIFIED Requirements

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
