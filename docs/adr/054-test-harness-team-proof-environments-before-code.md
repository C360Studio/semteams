# ADR-054: Test-Harness Team for Proof Environments Before Code

## Status

**Proposed (2026-06-08).** Product-level ADR for the next hard-scenario
slice after ADR-044. This ADR does not supersede ADR-044; it adds the
missing pre-implementation team that hard driver scenarios need before
the dev-via-test pack can honestly start.

## Context

SemSpec's MAVLink and OSH Connected Systems API hard e2e scenarios have
made the real blocker visible. The hard part is not only writing a
driver. The hard part is often making the world in which the driver can
be proved exist:

- PX4 SITL must be provisionable, bootable, and reachable through MAVSDK.
- Readiness must be detectable without sleeps or operator folklore.
- Logs, artifacts, versions, and failure signatures must be captured.
- OSH/CS API driver work may require finding Gradle/Maven dependency
  boundaries across `osh-core`, `osh-addons`, `ogc-cs`, generated code,
  and sibling source trees before a useful implementation task exists.
- A successful local unit test is not evidence that the hard scenario is
  verifiable.

ADR-044 proved the product value of a lean implementation loop: plan,
execute against tests in a sandbox, and review the integrated result.
That is necessary, but not sufficient for the hard scenarios. If the
proof environment is missing, the implementation loop can only thrash,
stub, or overclaim.

The product thesis changes from "write the spec, then code" to:

> Before implementation begins, SemTeams assembles the team needed to
> make the target claim verifiable.

OpenSpec, BMAD-style workflows, and SemSpec artifacts remain useful
inputs and projections. They are not the canonical operating truth for
this slice. The canonical truth is graph-backed claims, proof
dependencies, harness profiles, readiness evidence, and explicit gates.

## Decision

SemTeams will introduce a **test-harness team** as a category pack that
owns proof-environment readiness before feature implementation begins.

The coordinator routes to this team when a requested feature claim
depends on a missing or unverified test harness. Feature implementation
is not released until the needed readiness record exists or the operator
explicitly waives the gate.

### D1. Model proof dependencies before implementation tasks

The planner for a hard feature emits verifiable claims and proof
dependencies before emitting implementation work packets.

Example MAVLink/PX4 proof dependencies:

- `px4_sitl.image_available`
- `px4_sitl.boots_headlessly`
- `mavsdk_server.reachable`
- `vehicle_health.ready_detectable`
- `harness.logs_collectable`
- `smoke_test.repeatable`
- `ci_runner.supports_required_services`

Example OSH/CS API proof dependencies:

- `dependency.gradle_graph_mapped`
- `dependency.maven_artifacts_resolved`
- `osh.module_builds_headlessly`
- `csapi.gateway_contract_known`
- `driver.adapter_smoke_verifiable`
- `e2e.observation_or_control_visible_via_csapi`

If any required proof dependency is unknown or false, the coordinator
routes to the test-harness team instead of the implementation team.

### D2. Persist reusable harness profiles

The output of the test-harness team is a versioned **harness profile**,
not a note, script pile, or one-off compose file.

Minimum profile shape:

```yaml
id: mavlink.px4-sitl.mavsdk
version: v1
team: test-harness
claims_supported:
  - mavlink.telemetry.visible
  - mavlink.command.roundtrip
  - mavlink.mission_upload.verifiable
services:
  - name: px4_sitl
    image: px4io/px4-sitl:latest
  - name: mavsdk_server
    image: mavsdk/mavsdk-server:latest
readiness:
  probes:
    - mavsdk_server_reachable
    - vehicle_health_ready
smoke:
  command: task harness:mavlink:smoke
artifacts:
  - px4.log
  - mavsdk.log
  - smoke-results.json
timeouts:
  startup: 120s
  smoke: 60s
failure_signatures:
  - id: px4_boot_timeout
    match: "PX4 SITL did not reach ready state before startup timeout"
```

The profile is reusable product memory. Future feature work asks:

1. Does a matching harness profile already exist?
2. If yes, instantiate it and verify readiness.
3. If no, route to the test-harness team.
4. Once ready, attach the readiness evidence to the graph and unblock
   implementation.

### D3. Store readiness records as evidence

A **readiness record** is the current proof that a harness profile can
be used for this run. It captures:

- profile ID and version
- provision command or rendered runner target
- readiness probe results
- smoke command and result
- service image digests or pinned versions where available
- logs and artifact references
- wallclock duration and timeout budget
- failure signatures, if any
- evidence IDs for the claims the profile supports

Readiness records are run-scoped evidence. Harness profiles are reusable
definitions. Both are graph-addressable.

### D4. Use boring runners; do not build mini-k8s

The harness profile is the source of truth. Runners are renderers.

Initial renderers:

- Docker Compose for local repeatability.
- GitHub Actions / `act` compatibility for CI parity and SemSpec
  qa-runner lineage.

The test-harness team may author runner-specific files, but SemTeams
does not become a scheduler, operator framework, or general DevOps
platform.

In scope:

- service images
- ports and env vars
- readiness probes
- smoke commands
- artifact capture
- teardown behavior
- CI-compatible execution

Out of scope:

- dynamic scheduling
- autoscaling
- service mesh
- persistent cluster state
- arbitrary deployment management
- tenant-wide infrastructure orchestration

If a future deployment needs Kubernetes, the profile may gain a
Kubernetes renderer by ADR. The profile schema must not assume it.

### D5. Gate implementation on readiness

Implementation work packets require one of:

- a passing readiness record for each required harness profile; or
- an explicit waiver stamped by the coordinator/operator with reason,
  expiry, and the claims it leaves unproved.

The default behavior is fail-closed:

> Cannot verify `mavlink.mission_upload` yet. Missing proof dependency:
> `px4_sitl.ready_detectable`. Next action: route to test-harness team
> to create or repair `mavlink.px4-sitl.mavsdk`.

The gate is not a moral stance against progress. It prevents the product
from misclassifying proof-environment work as feature implementation
failure.

### D6. Keep old loop names as implementation history, not product nouns

Existing category packs and experiments are useful proof of concept for
planner/execute/review loops and for SemStreams substrate improvements.
This ADR intentionally uses product nouns:

- coordinator
- test-harness team
- implementation team
- harness profile
- readiness record
- proof dependency

The test-harness team may reuse existing substrate patterns, rule
shapes, and persona lessons. It should not expose prior experimental
agent names as the product language for this slice.

## MVP

The MVP target is **one hard scenario**, preferably MAVLink/PX4 SITL.

MVP success means:

1. The coordinator identifies that the target feature claim is blocked
   by missing proof-environment dependencies.
2. The coordinator routes to the test-harness team before implementation.
3. The test-harness team produces a reusable harness profile.
4. The profile renders locally via Compose.
5. The profile renders or runs through GitHub Actions / `act`
   compatibility.
6. A golden smoke produces a readiness record with artifacts.
7. Only after the readiness record exists does the implementation team
   receive the feature packet.

The comparison against SemSpec/BMAD/OpenSpec is not "who writes code
faster." It is:

- Did SemTeams surface the missing proof environment earlier?
- Did SemTeams turn the missing environment into a bounded work packet?
- Did the resulting harness profile make the next related scenario
  cheaper?
- Did the implementation loop avoid patching nondeterminism once the
  real blocker was known?

## Consequences

### Positive

- Hard-scenario proof-environment work becomes first-class product work
  instead of a footnote in a failed implementation run.
- Harness setup compounds across scenarios because profiles are reusable.
- The coordinator has a concrete pre-code gate, not vague judgment.
- The implementation team receives a verified target instead of an
  aspirational test environment.
- SemTeams can prove value even when it does not yet solve the feature:
  a blocked run becomes diagnosable, bounded, and cheaper to resume.

### Negative

- Upfront planning cost increases for hard scenarios.
- The first profile for a new domain may be as hard as the feature work.
- Runner parity is now a product correctness surface.
- Harness profiles can become stale if image versions, upstream build
  systems, or protocol simulators drift.

### Neutral

- This is a SemTeams product/team decision first. Reusable runner,
  lifecycle, or evidence primitives should move to SemStreams only after
  at least two product scenarios prove the shape.
- Existing dev-via-test work remains valid for scenarios whose proof
  environment already exists.
- Specs and prose artifacts remain projections of graph truth, not the
  authority.

## Alternatives Considered

### Continue with dev-via-test only

Rejected for hard scenarios. It works when a test command is already a
meaningful scalar. It does not solve the case where the test command
itself cannot exist until PX4 SITL, MAVSDK, OSH dependencies, or CS API
service topology are made real.

### Treat this as generic DevOps

Rejected. "DevOps team" is too broad and would invite platform sprawl.
The useful product unit is narrower: make this proof environment real,
repeatable, ready, and evidence-producing.

### Use only Docker Compose

Rejected as the canonical model. Compose is the right first local
renderer, but the reusable object is the harness profile. CI parity
requires GitHub Actions / `act` or equivalent runner compatibility.

### Build a full orchestration platform

Rejected. That is mini-k8s. The product needs proof environments for
bounded claims, not a scheduler.

### Keep harness setup as operator notes

Rejected. Notes do not compound. They are hard to gate on, hard to
compare across runs, and easy for implementation loops to ignore.

## Open Questions

1. Where should harness profiles live initially: `configs/harnesses/`,
   `configs/test-harness/`, or a new product artifact store?
2. Should the first profile schema be YAML for operator readability or
   JSON for tighter payload/schema validation?
3. How should readiness records map onto the current evidence
   predicates and the agent-run substrate adoption plan?
4. What is the minimal waiver shape that avoids turning waivers into
   quiet overclaims?
5. Which profile fields are SemTeams-only product vocabulary versus
   SemStreams substrate candidates after reuse?
6. Does the MVP need a UI surface, or is graph + artifact evidence
   enough for the first proof?

## Addendum 2026-06-21 — Folded under the ADR-056 umbrella; brownfield extension

[ADR-056](056-openspec-spec-driven-development-umbrella.md) (OpenSpec-
compatible, environment-gated spec-driven development) is the integrating
umbrella; this ADR is its **env-readiness layer** (ADR-056 §D4/§D6, P3).
Three clarifications the umbrella fixes:

1. **The foundation is already shipped.** ADR-043's devcontainer sandbox
   + attestation (`sandboxmanager`, `sandbox.attestation.*`) provisions a
   container and proves it is up — used by autoresearch and dev-via-test
   today. **This ADR is the *unbuilt extension*** (harness profiles,
   readiness records, proof dependencies, the gate) needed for
   **service-heavy** targets (PX4 SITL, OSH). For a typical brownfield
   Go/Node repo, ADR-043 attestation + *"the repo's own test suite runs
   green in the sandbox"* may be a sufficient v1 readiness check.
2. **Brownfield extension (P3).** The fixed 3-profile catalog
   (`go-backend`/`svelte-ui`/`full-stack-e2e`) is fine for our own repo
   but insufficient for an arbitrary target. Brownfield (UC-1) adds
   **topology-driven profile selection/derivation** — the *detector* is
   net-new; it reuses this ADR's §D2 profile schema. Greenfield (UC-3)
   adds **profile authoring**.
3. **The claims come from the spec layer.** The OpenSpec **EARS
   acceptance criteria** ([ADR-057 §D3](057-openspec-graph-spec-model-and-create-change.md))
   are the *claims* this layer proves; the spec's `test_command`s are the
   *smoke/proof*; the harness profile is what makes those commands
   runnable.

**Status** stays **Proposed**; flip to **Accepted** when P3 is committed
to build (ADR-056 §How this decomposes).

## Related

- [ADR-056: OpenSpec-Compatible, Environment-Gated Spec-Driven Development (umbrella)](056-openspec-spec-driven-development-umbrella.md)
- [ADR-057: OpenSpec Graph Spec Model and `create_change`](057-openspec-graph-spec-model-and-create-change.md)
- [ADR-033](033-harness-anchored-verification-and-coordinator-authority.md)
- [ADR-034: Verification execution via verification-runner pattern](034-qa-runner-pattern-adoption.md)
- [ADR-036: Test-Harness Lifecycle and Verification Machinery](036-test-harness-lifecycle.md)
- [ADR-042: Coordinator-Instantiated Flows via Templates](042-coordinator-instantiated-flows-via-templates.md)
- [ADR-043: Devcontainer as Sandbox Spec](043-devcontainer-as-sandbox-spec.md)
- [ADR-044: Dev-via-Test Pack](044-dev-via-test-pack.md)
- [ADR-053 Adoption Plan](053-adoption-plan.md)
- [ADR-055: Formal Claim Analysis for Verification Gates](055-formal-claim-analysis-for-verification-gates.md)
