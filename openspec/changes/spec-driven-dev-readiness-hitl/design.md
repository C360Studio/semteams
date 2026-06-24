# Design: Spec-Driven Development Readiness and HITL Review

## Technical Approach
Use the SemStreams governed SKG and rule engine as the coordination substrate. SemTeams owns product-level schema,
category packs, personas, tools, and UI surfaces. OpenSpec remains an interchange projection; graph facts remain
canonical for routing, ownership, evidence, and execution state.

## Architecture Decisions

### Decision: Graph facts are authoritative
Spec, task, proof, readiness, waiver, and run-health state live as governed graph facts. OpenSpec markdown is rendered
from those facts for review, ingest, and brownfield interoperability.

### Decision: Flow contracts remain the control surface
SemTeams must stay SemStreams-flow-native. NATS is the reactive transport for facts, commands, and events, but it is
not the product control plane by itself. New behavior is introduced through configured flows, category rule packs,
component contracts, payload schemas, named ports/subjects, tool governance, and lifecycle facts. A design that only
publishes or subscribes on NATS without declaring the SemStreams component/rule/payload boundary is incomplete.

### Decision: Proof readiness gates implementation
Implementation tasks are not released until required proof dependencies have readiness evidence or an explicit human
waiver. Service-heavy targets such as MAVLink/PX4 SITL route to the test-harness team before feature code when proof
infrastructure is missing.

### Decision: `dev_from_task` reuses the existing execution primitive
An approved change projects execution-rich task facts into `plan.task.*` and dispatches one ready task at a time through
Ralph. CBG remains the chain-end work gate and runs the integration acceptance command. The bridge starts only after an
explicit `dev_from_task` request marker and `proof_readiness.implementation_ready=true`; it also verifies the sandbox and
creates the chain-start git tag before Ralph mutates files.

### Decision: CBG verdicts are visible final gates
The chain-end `reviewer-dev-via-test` loop remains the final implementation gate for spec-driven work. Its approved,
rejected-retry, and rejected decisions stay on the existing dev-via-test rule rail, and the UI marks those decisions as
the CBG final gate so rejected evidence is visible without expanding raw trajectory JSON.

### Decision: The definition of done has one authority stack
For spec-driven implementation, "done" is not negotiated by every loop. The approved OpenSpec change owns the desired
behavior and task constraints; proof-readiness owns whether that behavior is currently provable; `project_spec_tasks`
only reprojects approved facts; Ralph converges task tests without redefining scope; CBG owns final implementation
acceptance by running the chain command and checking cumulative diff scope. If any layer cannot honor the layer above it,
the run blocks or asks the operator rather than silently redefining done.
The projected `plan.done_authority.*` facts make that stack visible to downstream loops, and the coordinator, Ralph, and
CBG persona contracts each state their boundary explicitly.

### Decision: Autoresearch needs a scalar objective and guardrails
Autoresearch is valid only when the coordinator can identify a repeatable measurement command, parser, scalar metric,
direction, cap, pass gate, and bounded mutation surface. The empirical compare in `emit_autoresearch_measurement`, not
the LLM, decides whether work is kept. Improvements that optimize the metric by corrupting the measurement, reducing
coverage, escaping the declared surface, or failing the pass gate are rejected or reverted even if the scalar value looks
better.

### Decision: Human review is a first-class product surface
The product must support operator review, edit, approve, reject, request revision, and waive actions for the proposed
spec and proof gates. Tool approval alone is not enough because users need to understand and shape the planning artifact
before expensive or risky work starts.

### Decision: Export is MVP, MCP handoff is optional
Users may use SemTeams only to produce a reviewed spec and then implement it elsewhere. The MVP export surface writes
or downloads the standard OpenSpec change folder and rendered single-document projection. MCP handoff can be a later
adapter over the same export contract; it is not required for the first usable slice.

### Decision: Slash commands are shortcuts, not a control plane
Slash commands give power users keyboard access to contextual actions such as `/export-spec`, `/implement-spec`,
`/run-status`, `/evidence`, `/approve`, and `/reject`. They route through the same coordinator intents, approval APIs,
and governed rule transitions as visible UI buttons. Command naming should keep concepts separate: implementation from
an approved spec is `/implement-spec`; `dev-via-spec` can exist only as a compatibility alias, not as a distinct
workflow mode.

### Decision: Gemini starts real-LLM validation
Mock journeys remain useful for deterministic CI, but model-dependent routing and prompt behavior need real-LLM smoke.
The first paid smoke target is Gemini through the existing model registry because it is cheaper to exercise repeatedly
and maps well to the registry surface. Provider choice stays configurable; Gemini is the starter pack, not a hard-coded
runtime dependency.

### Decision: Playwright is the full e2e gate
Full product validation uses Playwright journeys that drive the UI, backend, artifact review, export, and run-status
surfaces. Lower-level Go and component tests can prove contracts, but they do not replace a Playwright pass for HITL
and artifact workflows.

### Decision: UI answers "is this thing working?"
The UI presents a run-level answer with health, current gate, evidence freshness, blocked dependencies, active loops,
last material event, and next action. Raw trajectories and logs remain drill-down evidence.
Run health is a derived view, not another workflow state machine: the board composes run graph facts, lifecycle phase,
active loop states, approval/clarification markers, proof-readiness findings, CBG retry facts, and evidence freshness
into the five visible states `working`, `waiting`, `blocked`, `failing`, and `complete`.
The detail surface keeps the raw trajectory, message log, and run graph triples as drill-down receipts behind that
summary so operators can audit why the UI believes a run is healthy, blocked, or waiting.

### Decision: Prometheus metrics are operational evidence
Prometheus metrics supplement run health with component freshness, queue depth, latency, error rate, and resource
signals. They do not decide workflow routing or definition-of-done. If graph facts and Prometheus disagree, the UI
shows the disagreement explicitly: graph/lifecycle facts remain the workflow truth, while Prometheus explains runtime
pressure, stalled scrapes, or degraded infrastructure that may make the run unhealthy.

## Proof Readiness Fact Model

The proof model is stamped on the run entity under a `proof.*` namespace. It is the analyzer input contract.
`formal_claims.*` remains the analyzer output envelope that rules consume for routing.

### Claims
Claims are the modeled behaviors SemTeams may later implement or prove.

```text
proof.claim.<id>.statement          = "The system SHALL ..."
proof.claim.<id>.source_requirement = "agentic-sdd/Proof Readiness Gate"
proof.claim.<id>.source_scenario    = "Missing proof dependency routes to test harness"
proof.claim.<id>.requires           = ["px4_sitl.boots_headlessly", "..."]
proof.claim.<id>.conflicts_with     = []
proof.claim.<id>.status             = "accepted" | "draft" | "rejected"
proof.claim.<id>.task_refs          = ["change.mavlink-hard.task.2"]
```

Claim IDs are stable semantic IDs such as `mavlink.mission_upload.verifiable`. If an implementation needs predicate-safe
keys, it may slug or index the predicate segment, but the fact value must preserve the canonical claim ID.

### Proof Dependencies
Proof dependencies are the concrete preconditions needed before a claim can be treated as provable.

```text
proof.dependency.<id>.kind           = "service" | "toolchain" | "data" | "smoke" | "policy"
proof.dependency.<id>.description    = "PX4 SITL boots headlessly"
proof.dependency.<id>.required_for   = ["mavlink.mission_upload.verifiable"]
proof.dependency.<id>.status         = "unknown" | "missing" | "ready" | "failed" | "waived"
proof.dependency.<id>.profile_ref    = "mavlink.px4-sitl.mavsdk@v1"
proof.dependency.<id>.next_route     = "test_harness" | "coordinator" | "implementation"
```

### Harness Profiles
Harness profiles are reusable definitions owned by the test-harness team.

```text
proof.harness_profile.<id>.version          = "v1"
proof.harness_profile.<id>.team             = "test-harness"
proof.harness_profile.<id>.claims_supported = ["mavlink.mission_upload.verifiable"]
proof.harness_profile.<id>.dependencies     = ["px4_sitl.boots_headlessly"]
proof.harness_profile.<id>.readiness_probes = ["mavsdk_server_reachable"]
proof.harness_profile.<id>.smoke_command    = "task harness:mavlink:smoke"
proof.harness_profile.<id>.artifacts        = ["px4.log", "smoke-results.json"]
proof.harness_profile.<id>.renderer         = "compose" | "github_actions" | "act"
proof.harness_profile.<id>.ttl_seconds      = 86400
```

### Readiness Records
Readiness records are run-scoped evidence that a harness profile is currently usable.

```text
proof.readiness.<id>.profile_ref       = "mavlink.px4-sitl.mavsdk@v1"
proof.readiness.<id>.status            = "passed" | "failed" | "stale" | "blocked"
proof.readiness.<id>.started_at        = "2026-06-24T..."
proof.readiness.<id>.completed_at      = "2026-06-24T..."
proof.readiness.<id>.expires_at        = "2026-06-25T..."
proof.readiness.<id>.probe_results     = [{"name":"mavsdk_server_reachable","status":"passed"}]
proof.readiness.<id>.smoke_command     = "task harness:mavlink:smoke"
proof.readiness.<id>.smoke_status      = "passed" | "failed" | "not_run"
proof.readiness.<id>.attestation_ref   = "sandbox.attestation.signature:<signature>"
proof.readiness.<id>.evidence          = ["proof.evidence.mavlink-smoke.001"]
proof.readiness.<id>.failure_signature = "px4_boot_timeout"
```

Readiness may wrap or reference existing `sandbox.attestation.*` facts. It must not duplicate container internals when a
stable attestation reference is enough.

### Evidence
Evidence records point to artifacts, logs, command output, or object-store entries used by readiness and analyzer gates.

```text
proof.evidence.<id>.kind        = "log" | "artifact" | "command_result" | "attestation"
proof.evidence.<id>.uri         = "object://..."
proof.evidence.<id>.digest      = "sha256:..."
proof.evidence.<id>.producer    = "test-harness"
proof.evidence.<id>.command     = "task harness:mavlink:smoke"
proof.evidence.<id>.exit_code   = 0
proof.evidence.<id>.created_at  = "2026-06-24T..."
proof.evidence.<id>.covers      = ["mavlink.mission_upload.verifiable"]
```

### Waivers
Waivers are human decisions to proceed with bounded unproved surface.

```text
proof.waiver.<id>.reason        = "PX4 SITL readiness is unavailable in this environment"
proof.waiver.<id>.approved_by   = "operator-id"
proof.waiver.<id>.approved_at   = "2026-06-24T..."
proof.waiver.<id>.expires_at    = "2026-06-25T..."
proof.waiver.<id>.claims        = ["mavlink.mission_upload.verifiable"]
proof.waiver.<id>.dependencies  = ["vehicle_health.ready_detectable"]
proof.waiver.<id>.residual_risk = "Mission upload remains unproved against PX4 SITL"
proof.waiver.<id>.status        = "active" | "expired" | "revoked"
```

The analyzer may treat an active waiver as routeable, but it must leave the affected claim visible as waived rather than
passed.

### Formal Claims Analyzer
The MVP analyzer is `analyze_proof_readiness`, a deterministic product-shell tool that composes existing graph
primitives rather than adding another workflow store. It reads the current run entity with the same entity-reader shape
as `render_openspec`, evaluates `proof.*` facts in process, and emits one atomic `formal_claims.*` batch back to the run
entity.

```text
formal_claims.status                         = "passed" | "failed" | "ambiguous"
formal_claims.analyzer.version               = "go-native-v1"
formal_claims.analyzed_at                    = "2026-06-24T..."
formal_claims.finding_count                  = 2
formal_claims.route.test_harness             = "present"
formal_claims.route.implementation           = "present"
formal_claims.route.coordinator              = "present"
formal_claims.finding.<id>.kind              = "missing_proof_dependency"
formal_claims.finding.<id>.severity          = "blocker" | "warning"
formal_claims.finding.<id>.route             = "test_harness" | "coordinator" | "implementation"
formal_claims.finding.<id>.reason            = "required proof dependency is not ready"
formal_claims.finding.<id>.claim             = "mavlink.mission_upload.verifiable"
formal_claims.finding.<id>.dependency        = "px4_sitl.boots_headlessly"
formal_claims.finding.<id>.profile           = "mavlink.px4-sitl.mavsdk@v1"
formal_claims.finding.<id>.readiness         = "smoke-001"
formal_claims.finding.<id>.waiver            = "operator-001"
```

Rules consume the envelope, not the analyzer implementation. `formal_claims.status=passed` can release implementation
routing. `failed` with blocker findings routes to the finding route. `ambiguous` means the run lacks accepted proof
claims and must return to coordinator/spec modeling before implementation.

The proof-readiness rule pack consumes only exact summary predicates, not wildcard finding predicates:

```text
formal_claims.route.implementation = "present" -> proof_readiness.route = "implementation"
formal_claims.route.test_harness   = "present" -> proof_readiness.route = "test_harness"
formal_claims.route.coordinator    = "present" -> proof_readiness.route = "coordinator"
failed with no route summary                   -> proof_readiness.route = "pause"
```

The implementation route currently stamps `proof_readiness.implementation_ready=true` for the future `dev-from-task`
pack. The test-harness route stamps `proof_readiness.test_harness_required=true` for the future test-harness category
pack. Both are governed graph facts loaded through `configs/rules/proof-readiness/`, not raw NATS subscribers.

## Future Roadmap

### Skill Import Adapter
Claude/Codex-style skills can become a useful intake format after the spec-driven workflow is stable. The adapter should
translate a trusted markdown skill into a proposed SemTeams capability pack by extracting persona guidance, workflow
steps, tool needs, artifact contracts, approval boundaries, and evidence expectations. The output is an OpenSpec change
for review, not a live rule mutation.

This preserves the product distinction: SemTeams capabilities are governed operating modes composed of rule packs,
persona fragments, schemas, tests, tool governance, and UI surfaces. Markdown may seed that structure, but it does not
execute directly and cannot grant tools outside the normal approval and governance paths.

## Data Flow
User prompt or existing OpenSpec -> `create_change` -> reviewed OpenSpec change and `change.<slug>.*` facts ->
human review -> export, proof-readiness analyzer, or both. If SemTeams implements it, the run continues through
test-harness or `dev_from_task` -> Ralph task execution -> CBG integration gate -> final response or PR-ready artifact.

## File Changes
- `openspec/changes/spec-driven-dev-readiness-hitl/` (new)
- `configs/rules/create-change/` (extend review and render flow as needed)
- `configs/rules/dev-from-task/` (new category pack)
- `configs/rules/test-harness/` (new readiness category pack)
- `cmd/semteams/tools/` (add proof analyzer, readiness, and dev-from-task entry tools as needed)
- `ui/src/lib/components/board/` and run-status surfaces (spec review, export, and health panels)
