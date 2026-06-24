# Delta for Agentic SDD

## ADDED Requirements

### Requirement: OpenSpec Change Artifact
The system SHALL create and persist reviewable OpenSpec changes as graph-backed artifacts before spec-driven
implementation begins.

#### Scenario: Create change from prompt
- GIVEN a user asks SemTeams to plan a spec-driven development change
- WHEN the coordinator routes the request to `create_change`
- THEN the system emits `change.<slug>.*` graph facts
- AND renders `proposal.md`, capability delta specs, and `tasks.md` as an OpenSpec change

#### Scenario: Ingest brownfield OpenSpec
- GIVEN a target repository already contains an `openspec/` tree
- WHEN SemTeams inventories the repository for spec context
- THEN the system maps existing OpenSpec specs and changes into graph facts
- AND keeps the graph as the authority for subsequent routing and execution

### Requirement: Human Spec Review
The system SHALL allow an operator to review, edit, approve, reject, or request revision of a proposed OpenSpec change
before implementation release.

#### Scenario: Review and edit proposed spec
- GIVEN `create_change` has produced a proposed OpenSpec change
- WHEN an operator opens the review surface
- THEN the UI shows the proposal, delta requirements, tasks, proof implications, and acceptance command
- AND the operator can edit the artifact or request a revision before approval

#### Scenario: Approval gates implementation
- GIVEN a proposed OpenSpec change has not been approved
- WHEN implementation routing is evaluated
- THEN `dev_from_task` is not released
- AND the UI shows the run as waiting on spec approval

### Requirement: OpenSpec Artifact Export
The system SHALL export generated OpenSpec artifacts so a user can implement the approved spec outside SemTeams.

#### Scenario: Export reviewed change folder
- GIVEN an OpenSpec change has been created or approved
- WHEN an operator requests export
- THEN the system provides the standard `openspec/changes/<slug>/` files
- AND the export includes `proposal.md`, `tasks.md`, and every capability delta `spec.md`

#### Scenario: Export rendered handoff document
- GIVEN an OpenSpec change exists as graph facts
- WHEN an operator requests a single-file handoff
- THEN the system renders a complete OpenSpec document with file markers
- AND the document can be handed to Codex, Claude Code, or a human implementer without running SemTeams implementation

#### Scenario: MCP handoff remains an adapter
- GIVEN an MCP-based external tool handoff is available
- WHEN an operator chooses that handoff
- THEN the handoff uses the same exported OpenSpec artifact contract
- AND SemTeams does not require MCP to produce or download the spec artifact

### Requirement: Command Shortcut Surface
The system SHALL provide contextual slash-command shortcuts for spec export, implementation dispatch, run status,
evidence review, and approval control without creating a separate workflow control plane.

#### Scenario: Command mirrors visible action
- GIVEN a visible UI action exists for exporting a spec, approving a gate, or showing run status
- WHEN an operator invokes the equivalent slash command
- THEN the system routes through the same UI action handler, coordinator intent, or governed API
- AND the command result is indistinguishable from using the visible control

#### Scenario: Implementation command preserves concept boundaries
- GIVEN an approved OpenSpec change is ready for implementation routing
- WHEN an operator invokes `/implement-spec <slug>`
- THEN the system starts proof-readiness and task dispatch for that approved spec
- AND `/dev-via-spec` is treated only as a compatibility alias or rejected with guidance to use `/implement-spec`

#### Scenario: Unknown or unsafe command does not bypass governance
- GIVEN an operator enters an unknown command or a mutating command that requires approval
- WHEN the command is parsed
- THEN the system either rejects the command with a clear message or opens the normal approval path
- AND no slash command can directly mutate workflow state outside the coordinator, rule, or approval surfaces

### Requirement: Proof Fact Model
The system SHALL model claims, proof dependencies, harness profiles, readiness records, evidence, and waivers as typed
graph facts before implementation routing is evaluated.

#### Scenario: Claims declare proof dependencies
- GIVEN an approved OpenSpec change contains testable scenarios
- WHEN proof readiness is modeled for the run
- THEN the run graph contains `proof.claim.<id>.*` facts for each verifiable claim
- AND each claim records its source requirement, statement, proof dependencies, conflicts, and routeable status

#### Scenario: Harness profile defines reusable proof capability
- GIVEN a proof dependency requires an external service, simulator, or integration environment
- WHEN a harness profile is available or produced by the test-harness team
- THEN the graph contains `proof.harness_profile.<id>.*` facts for profile ID, version, supported claims, probes,
  smoke command, artifacts, renderer, and TTL
- AND the profile is reusable across runs instead of being stored only as a one-off log or script

#### Scenario: Readiness and evidence prove current usability
- GIVEN a harness profile has been instantiated for a run
- WHEN probes and smoke commands complete
- THEN the graph contains `proof.readiness.<id>.*` facts for profile version, status, timestamps, expiry,
  probe results, smoke result, attestation reference, and evidence IDs
- AND each attached `proof.evidence.<id>.*` record includes kind, URI or object reference, digest when available,
  producer, command, timestamps, and covered claims

#### Scenario: Waivers are explicit and bounded
- GIVEN an operator waives a missing dependency or stale readiness record
- WHEN the implementation gate is evaluated
- THEN the graph contains `proof.waiver.<id>.*` facts for reason, approver, approved time, expiry, covered claims,
  covered dependencies, and residual risk
- AND the affected claim remains visible as waived rather than fully proved

### Requirement: Proof Readiness Gate
The system SHALL determine required proof dependencies and readiness evidence before releasing service-heavy
implementation tasks.

#### Scenario: Missing proof dependency routes to test harness
- GIVEN an approved change requires PX4 SITL and MAVSDK readiness
- WHEN no fresh readiness record proves those dependencies
- THEN the system routes to the test-harness team
- AND blocks implementation until readiness evidence or a waiver exists

#### Scenario: Analyzer emits routeable formal-claims envelope
- GIVEN `proof.*` facts exist on the run entity
- WHEN the deterministic analyzer evaluates the run
- THEN the graph contains `formal_claims.status`, `formal_claims.analyzer.version`, and finding count facts
- AND each blocker finding includes kind, route, severity, reason, and affected claim or dependency when available

#### Scenario: Passed readiness releases implementation marker
- GIVEN the analyzer emits `formal_claims.status=passed`
- WHEN the proof-readiness rule pack evaluates the run entity
- THEN the graph contains `proof_readiness.route=implementation`
- AND the graph contains `proof_readiness.implementation_ready=true`

#### Scenario: Test-harness findings produce a governed handoff
- GIVEN the analyzer emits `formal_claims.status=failed`
- AND the graph contains `formal_claims.route.test_harness=present`
- WHEN the proof-readiness rule pack evaluates the run entity
- THEN the graph contains `proof_readiness.route=test_harness`
- AND the graph contains `proof_readiness.test_harness_required=true`

#### Scenario: Waiver preserves overclaim visibility
- GIVEN an operator waives a missing proof dependency
- WHEN the implementation gate is evaluated
- THEN the system records the waiver reason, expiry, affected claims, and remaining unproved surface
- AND the UI shows the run as proceeding with a waiver rather than fully proved

### Requirement: Dev From Task Dispatch
The system SHALL execute approved OpenSpec tasks through the existing dev-via-test execution primitive without
re-deriving the spec.

#### Scenario: Project approved task into execution state
- GIVEN an approved OpenSpec change contains execution-rich task facts
- WHEN proof readiness has passed or been waived
- THEN the system projects ready tasks into `plan.task.*`
- AND dispatches the selected task through Ralph

#### Scenario: CBG remains the final work gate
- GIVEN all projected tasks have converged
- WHEN the coordinator finalizes the run
- THEN CBG runs the chain-level acceptance command
- AND rejected gates return to the coordinator and UI with evidence

### Requirement: Definition Of Done Authority
The system SHALL preserve a single authority stack for what "done" means in spec-driven implementation runs.

#### Scenario: Approved spec owns done
- GIVEN an OpenSpec change has been approved by reviewer and operator gates
- WHEN implementation routing starts
- THEN no planner or executor redefines the approved requirements, scenarios, task goals, test commands, target files, or
  chain acceptance command
- AND `project_spec_tasks` either projects those approved facts losslessly into `plan.*` or refuses the run

#### Scenario: Ralph converges but does not redefine done
- GIVEN a task has been projected from an approved OpenSpec change
- WHEN Ralph executes the task
- THEN Ralph may iterate until the task test command passes or request clarification
- AND Ralph cannot replace task goals, broaden target files, weaken the test command, or mark the task done by prose
  judgment alone

#### Scenario: CBG judges final done
- GIVEN all projected tasks have converged
- WHEN CBG evaluates the run
- THEN CBG runs the chain-level acceptance command and reviews cumulative diff scope against the approved spec
- AND CBG's approved, rejected-retry, or rejected verdict is the final implementation acceptance gate surfaced to the
  coordinator and UI

### Requirement: Autoresearch Metric Guardrails
The system SHALL route to autoresearch only when the objective has a specific scalar metric, a repeatable measurement
command, and anti-Goodhart constraints that prevent the model from judging its own success.

#### Scenario: Non-scalar objective is not autoresearch
- GIVEN a user asks to "make this better" or optimize a vague outcome
- WHEN the coordinator cannot identify a scalar metric, measurement command, metric parser, direction, cap, and bounded
  mutation surface
- THEN the coordinator does not route to autoresearch
- AND the run asks for clarification, routes to create-change, or selects a different implementation method

#### Scenario: Measurement, not prose, decides kept work
- GIVEN autoresearch proposes a bounded change
- WHEN the measurement command runs
- THEN the system extracts one numeric metric and compares it against the current best value deterministically
- AND the LLM cannot keep a change because it claims the change is better without numeric evidence

#### Scenario: Guardrails constrain optimization pressure
- GIVEN autoresearch has an active scalar objective
- WHEN a proposed change improves the metric by narrowing coverage, bypassing tests, mutating the measurement command,
  leaving the declared surface, or invalidating the pass gate
- THEN the change is rejected or reverted even if the scalar metric appears improved
- AND the run records the guardrail violation as evidence for the coordinator, reviewer, and UI

### Requirement: Governed State Instead Of Private Workflow Buckets
The system SHALL represent spec-driven planning and execution progress as SemStreams flow-native graph facts, component
contracts, payload schemas, ports, category rule packs, and lifecycle rules rather than product-local private workflow
buckets or ad hoc NATS-only subscriptions.

#### Scenario: State is queryable by rules and UI
- GIVEN a spec-driven development run is in progress
- WHEN rules, graph queries, or UI status models inspect the run
- THEN they read current state from graph facts and lifecycle phase
- AND no SemSpec-style private planning or execution bucket is required for routing

#### Scenario: NATS transport does not bypass flow contracts
- GIVEN a spec-driven feature needs a new reactive behavior
- WHEN the implementation publishes or subscribes to NATS subjects
- THEN the design also declares the owning SemStreams flow, component or tool boundary, payload schema, port or subject,
  and rule transition that consumes the behavior
- AND no product slice treats a raw NATS subscription as a replacement for SemStreams routing, governance, or lifecycle
  facts

### Requirement: Run Health Surface
The system SHALL present run health in the UI as working, waiting, blocked, failing, or complete with current evidence
and next action.

#### Scenario: Operator asks whether the run is working
- GIVEN a run has active loops, proof findings, approval gates, or terminal evidence
- WHEN the operator opens the run view
- THEN the UI shows the current health label, current gate, last material event, evidence freshness, and next action
- AND raw logs and trajectories are available as drill-down evidence

#### Scenario: Blocked run explains the blocker
- GIVEN a run cannot proceed because proof dependencies are missing
- WHEN the UI renders the run summary
- THEN it names the missing dependencies and responsible next team
- AND it does not present the run as merely idle or still working

#### Scenario: Prometheus metrics supplement run health
- GIVEN Prometheus metrics are available for SemStreams components, NATS, model calls, tools, or rule execution
- WHEN the UI renders run health
- THEN it shows metric freshness and relevant operational signals such as scrape age, queue depth, error rate, latency,
  and component saturation
- AND it treats metrics as observability evidence rather than the authority for routing, task completion, or CBG
  acceptance

#### Scenario: Metric gaps are visible
- GIVEN Prometheus metrics are stale, unavailable, or missing required labels for run or component correlation
- WHEN the UI renders run health
- THEN it shows the metrics signal as unavailable or stale
- AND it does not infer that the run is healthy simply because no metric has fired

### Requirement: Real LLM And Playwright Validation
The system SHALL validate model-dependent behavior with Gemini first and full product behavior with Playwright e2e
journeys.

#### Scenario: Gemini starts paid smoke validation
- GIVEN a feature depends on real model routing, planning, or prompt-following behavior
- WHEN a paid real-LLM smoke is selected
- THEN the default starter provider is Gemini through the model registry
- AND the smoke records the provider, model ID, prompt class, and observed result

#### Scenario: Provider choice remains configurable
- GIVEN an operator configures a different model provider
- WHEN the same journey is executed
- THEN the model registry selects the configured provider
- AND no implementation code assumes Gemini-specific request or response shapes

#### Scenario: Playwright validates full e2e behavior
- GIVEN a spec-driven workflow includes UI review, artifact export, approvals, waits, or run health
- WHEN the full e2e gate runs
- THEN Playwright drives the journey through the UI and backend
- AND the report includes artifact output and evidence for the current run state
