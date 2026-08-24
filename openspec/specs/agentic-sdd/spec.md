# Agentic SDD Specification

## Purpose

Define the behavior SemTeams actually exposes on the beta.160 bootstrap, including explicit negative contracts for the
spec/dev surfaces that remain implemented on disk but unwired. Current live behavior is one conversational coordinator,
research and autoresearch categories, graph-backed run state, and a four-journey black-box demo evidence pack.

## Requirements

### Requirement: OpenSpec Change Artifact

The system SHALL treat graph-backed OpenSpec authoring as a parked capability, not a live beta.160 coordinator route.

#### Scenario: Spec-authoring ask does not create a change

- GIVEN a user asks SemTeams to create or ingest an OpenSpec change
- WHEN the live coordinator evaluates the request
- THEN it emits `respond_direct` explaining that spec authoring is not wired in this deployment
- AND it does not spawn `create_change` or emit new `change.<slug>.*` facts

### Requirement: Human Spec Review

The system SHALL NOT advertise the retained spec review components as a live workflow while `create-change` is parked.

#### Scenario: Parked review UI is not a live claim

- GIVEN no live create-change run can produce a proposed spec
- WHEN product capabilities are presented to the user
- THEN spec review, edit, approval, and revision are identified as parked
- AND their retained code and tests are treated as migration inventory rather than demo evidence

### Requirement: OpenSpec Artifact Export

The system SHALL NOT advertise OpenSpec export as a live front-door capability while its producing category is parked.

#### Scenario: Export command is absent from the live ChatBar inventory

- GIVEN the user opens the beta.160 ChatBar command hints
- WHEN no task is selected
- THEN `/export-spec` is not shown as a live team or run action
- AND retained renderer or command-registry code does not establish a live export claim

### Requirement: Command Shortcut Surface

The system SHALL expose only live product-team shortcuts as coordinator-routed hints under a closed
`research | autoresearch | respond_direct | ask_user` taxonomy.

#### Scenario: Live team hints use the coordinator path

- GIVEN a user enters `/research` or `/optimize`
- WHEN the ChatBar sends the prompt
- THEN the full editable prompt reaches the coordinator through the ordinary message path
- AND the prefix does not directly invoke a team or bypass validation

#### Scenario: Parked team hints are absent

- GIVEN no task is selected and the user opens ChatBar hints
- WHEN the UI lists product teams
- THEN it shows research and optimize only
- AND it does not show create-change, spec, dev-via-test, or implement-spec

#### Scenario: Parked ask receives an honest response

- GIVEN a user types a spec-authoring or implementation request, with or without a historical slash token
- WHEN the coordinator evaluates the message
- THEN it emits `respond_direct` with the deployment limitation
- AND it does not emit `create_change`, `proof_readiness`, `dev_from_task`, or `dev_via_test`

### Requirement: Proof Fact Model

The system SHALL treat the retained `proof.*` spec/dev model as parked and SHALL NOT use it as live routing authority.

#### Scenario: Retained proof tooling does not imply live proof routing

- GIVEN proof-related tools, components, fixtures, or graph vocabulary remain in the repository
- WHEN the beta.160 bootstrap loads its rule packs
- THEN no `proof-readiness` category pack is wired
- AND retained proof surfaces are not counted as current demo behavior

### Requirement: Proof Readiness Gate

The system SHALL fail closed at sandbox admission for the live autoresearch category without claiming the parked
proof-readiness workflow.

#### Scenario: Non-ready sandbox prevents autoresearch dispatch

- GIVEN a complete autoresearch request
- WHEN `request_sandbox` returns pending, denied, or terminal non-ready admission
- THEN the coordinator reports the limitation or required operator action
- AND autoresearch is not spawned until admission is ready

#### Scenario: Incomplete optimization contract asks before admission

- GIVEN an optimization ask lacks a scalar metric, measurement command, direction, bounded surface, or target
- WHEN the coordinator evaluates it
- THEN it uses `ask_user` for one missing fact or `respond_direct` with the requirements
- AND it does not dispatch autoresearch

### Requirement: Dev From Task Dispatch

The system SHALL treat spec-to-task implementation dispatch as parked and SHALL NOT spawn implementation roles.

#### Scenario: Build request does not dispatch Ralph or CBG

- GIVEN a user requests implementation from a prompt or retained OpenSpec artifact
- WHEN the live coordinator evaluates the request
- THEN it emits `respond_direct` explaining that implementation teams are not wired
- AND no `dev_from_task` or `dev_via_test` run is created

### Requirement: Definition Of Done Authority

The system SHALL NOT claim the parked spec/dev done-authority stack as a current execution contract.

#### Scenario: Retained gates are not live acceptance authority

- GIVEN Lisa, Ralph, CBG, and approved-spec projection code remain on disk
- WHEN current product behavior is documented or demonstrated
- THEN those roles are identified as parked migration inventory
- AND their retained state cannot mark a live beta.160 implementation run complete because no such run is dispatched

### Requirement: Autoresearch Metric Guardrails

The system SHALL use deterministic measurement and guardrails, rather than model prose, to decide whether an
autoresearch change is kept.

#### Scenario: Measurement decides kept work

- GIVEN autoresearch proposes and executes a bounded change in an admitted sandbox
- WHEN the measurement command completes
- THEN the system extracts one numeric metric and compares it with the current best value
- AND model judgment alone cannot keep the change

#### Scenario: Guardrail violation rejects apparent improvement

- GIVEN a change appears to improve the scalar metric
- WHEN it narrows coverage, bypasses tests, mutates the measurement command, leaves the bounded surface, or fails the pass
  gate
- THEN the change is rejected or reverted
- AND the violation is retained as run evidence

### Requirement: Governed State Instead Of Private Workflow Buckets

The system SHALL represent live routing and category progress through SemStreams graph facts, lifecycle, component
ports, rules, and registered payload contracts rather than a product-local workflow bucket.

#### Scenario: Research rules fan out and join through governed state

- GIVEN the coordinator emits `research`
- WHEN the research pack plans and gathers evidence
- THEN bounded gatherer loops may fan out and synthesis waits for their required completion facts
- AND approved synthesis returns to the coordinator with recoverable source evidence

#### Scenario: Rules and UI share run authority

- GIVEN a research or autoresearch run is active
- WHEN rules or UI status models inspect it
- THEN they read current graph and lifecycle facts
- AND no private SemTeams planning bucket becomes a second authority

### Requirement: Run Health Surface

The system SHALL describe run and trajectory evidence according to the beta.160 GraphQL surface actually available.

#### Scenario: Trajectory facts omit evidence bodies

- GIVEN the UI reads a beta.160 trajectory
- WHEN GraphQL returns its facts
- THEN the UI receives previews and `StorageReference` values rather than tool-argument evidence bodies
- AND ArtifactCard content, artifact-context handoff, rich model prose, and proof-card rendering are not claimed live

#### Scenario: Terminal lifecycle remains visible

- GIVEN a live research or autoresearch run completes or fails
- WHEN the UI and rules inspect its graph state
- THEN the run outcome and lifecycle phase remain observable
- AND absence of an evidence body is not interpreted as successful artifact rendering

### Requirement: Real LLM And Playwright Validation

The system SHALL separate optional paid model evidence from deterministic mock-LLM product wiring evidence.

#### Scenario: Production model selection remains configurable

- GIVEN the production bootstrap starts without an explicit endpoint override
- WHEN the model registry resolves its default
- THEN it selects the `gemini-flash` endpoint
- AND configured fallback endpoints remain provider-neutral product configuration

#### Scenario: Mock journeys do not prove model judgment

- GIVEN a Playwright journey uses `e2e-flow-bootstrap.json` and mock fixtures
- WHEN it passes
- THEN it proves the declared wiring and delivery assertions
- AND model routing quality still requires an explicitly identified real-LLM smoke

### Requirement: Demo MVP Evidence Pack

The system SHALL make only claims proven by the four black-box journeys in the `demo-mvp` aggregate.

#### Scenario: Aggregate contains the declared journeys

- GIVEN `task ui:test:e2e:agentic:demo-mvp` is invoked
- WHEN the aggregate expands
- THEN it runs coordinator-routing-matrix, research-mvp, coordinator-readiness-gate, and autoresearch
- AND no skipped or additional journey is represented as an aggregate member

#### Scenario: Black-box journeys avoid direct state seeding

- GIVEN a journey is used as MVP black-box evidence
- WHEN it drives SemTeams
- THEN it uses public UI, dispatch, slash-command, or documented task-runner surfaces
- AND it does not write directly to NATS, graph storage, lifecycle state, proof facts, or private KV to satisfy the claim

#### Scenario: Narrow journeys do not broaden aggregate claims

- GIVEN another journey exercises a non-aggregate behavior
- WHEN evidence is reported
- THEN that journey proves only its own assertions
- AND fixture-seeded or skipped journeys are not counted as black-box MVP evidence
