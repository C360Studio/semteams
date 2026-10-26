## ADDED Requirements

### Requirement: Coordination design admission

The SemTeams coordination design SHALL identify authoritative repository records, source observation, rule-pack policy,
agent-runtime ownership, and generic substrate ownership separately. It SHALL distinguish current behavior from target
contracts and state which durability and claim semantics need later substrate qualification. The design SHALL define
the first separately qualified proof as one configured GitHub repository and one existing issue: a fixture title or body
update with a changed `updated_at` is discovered and staged as one logical, explicitly scoped observation; after
downstream persistence through a proven boundary and cursor commit, an unchanged repeat yields none. This requirement admits a design; it does not assert that
the proof or delegated-manager runtime is implemented.

#### Scenario: Outbound-only repository observation is planned

- **GIVEN** a deployment with no public inbound endpoint and one configured GitHub repository
- **WHEN** the design describes how an existing issue change could be observed
- **THEN** it SHALL use service-side outbound polling as a supported target path and treat webhooks as optional hints that require reconciliation
- **AND** it SHALL describe stable source identity, revision, scope, duplicate suppression, and unavailable or partial freshness without claiming that polling reconstructs every intermediate forge event
- **AND** it SHALL identify existing `github_read` behavior to reuse or qualify before a new watcher is proposed
- **AND** it SHALL describe how run-scoped `github_read` retries replay the pre-run delta until the research artifact commits its cursor, without claiming that this is already a durable source-to-task boundary

#### Scenario: Inter-manager work is admitted for later implementation

- **GIVEN** a proposed Program-to-Project-Manager request or an external repository text body
- **WHEN** the design assigns control and evidence boundaries
- **THEN** it SHALL require explicit validated program/project coordinates, bounded task and pending-work policy, and a single claimant with inspectable outcomes
- **AND** it SHALL require metadata-first notice, fenced retrieval of untrusted text with verified provenance outside the fence, and typed secret-redaction labels before persistence
- **AND** it SHALL leave generic transport, rule, graph, and durability mechanics with SemEngine while SemTeams owns the agent-specific request and governance contract
- **AND** it SHALL identify a run-wide admission policy for work, retries, time and expenditure without treating a reactive threshold as a strict concurrent cap

#### Scenario: First proof stays narrow

- **GIVEN** the first GitHub polling proof is planned
- **WHEN** its acceptance criteria are recorded
- **THEN** a baseline followed by one fixture title/body and `updated_at` update SHALL discover and stage one scoped logical observation with stable source identity
- **AND** after the downstream observation/result is persisted through a proven boundary and the cursor commits, an unchanged repeat SHALL yield none
- **AND** a pre-commit retry SHALL be identified as able to replay the same source identity without implying once-only execution
- **AND** an out-of-scope repository or project SHALL be refused, and a failed or incomplete fetch SHALL retain the last successful cursor and expose unavailable/partial state
- **AND** prompt-like issue prose SHALL remain untrusted data and synthetic secret text SHALL be redacted before any persisted observation/result
- **AND** the proof SHALL NOT be represented as Gitea support, model execution, task response delivery, exactly-once external effects, or generic durable recovery
