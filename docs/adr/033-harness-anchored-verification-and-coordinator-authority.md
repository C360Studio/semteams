# ADR-033: Harness-Anchored Verification and Coordinator-as-Decision-Authority

## Status

**Proposed — 2026-05-03.** Drafted following the R3.6.2.g real-LLM
smoke #6 quality assessment, which surfaced two coupled findings:
the dev-via-spec chain has no structural way to anchor "real" against
real systems, and the coordinator role is being undersold as an
intent classifier when it is actually the platform's decision
authority. This ADR addresses both as one architectural shape.

## Context

R3.6.2.g closed the R3.6 builder slice cleanly: 11 loops to
`builder_decide(tests_passing)`, 18/18 unit tests green, the chain
fully wired end-to-end. But qualitative review of the produced
artifact found that **every production dependency was stubbed**.
`AbstractSensorModule.java`: *"Minimal stand-in for the OSH
AbstractSensorModule base class. In production this would extend
org.sensorhub.api.sensor.AbstractSensorModule."* `MeshPacket.java`:
*"Lightweight stand-in for the Meshtastic protobuf MeshPacket."*
`pom.xml` had zero non-test dependencies. The chain reliably
converged to "tests pass on a self-mocked system" — technically
truthful, operationally useless.

A grounded survey of comparable frameworks (BMAD-METHOD V6,
OpenSpec opsx) confirmed the gap is industry-default, not
SemTeams-specific. Neither framework has a structural primitive for
"the produced code must compile and pass a smoke test linked
against real `org.sensorhub:sensorhub-core@<version>` and the real
protobuf schema from `meshtastic/protobufs`." Both rely on persona
discipline plus operator vigilance. Both would produce stubs given
the same OSH-Meshtastic prompt.

The R3.6.2.g review session (2026-05-03) reframed this. The first
draft of a fix proposed a `test_target` field on the dev-via-spec
artifact and an `integration-architect` role between architect and
builder. Coby pushed back on both:

> *"`test_target` feels like a goodhart footgun and integration
> architect feels like a patch on architect."*

He was right. Declaring a target makes it a metric; the metric
becomes the goal; the LLM optimises for "looks like X" instead of
"does X." Adding a role to compensate for a thin earlier role is
bureaucratic depth, not a fix.

The deeper question Coby surfaced:

> *"When and who declares what we create will be tested against a
> real stack? Are there meshtastic SITL harnesses or similar? This
> prompt is really hard for many reasons."*

And then:

> *"It seems like we keep selling the coordinator role short and
> treating it as 'just an intent classifier' — I wonder if we
> rename coordinator to el jefe if it would help? The coordinator
> should be able to make the decision to edit, start, stop flows
> with rules and infra to support. Config options allow the human
> to say before runtime what coordinator can decide for them."*

These are the same architectural concern at two levels. The chain
needs a verification anchor (the harness) and a decision authority
(the coordinator), and the platform shape — sandbox + compose
profiles + typed payloads + rule routing — already supports both
if we name them.

## Decision

Three coupled moves. Each is platform-shaped (operator-curated,
multi-tenant, audit-able), not chain-shaped (per-prompt, agent-
synthesised). The combination is what neither BMAD nor OpenSpec
can structurally add without rebuilding.

### 1. Harnesses are a curated platform asset

A versioned JSON registry — `configs/harnesses.json` — lists each
deployment's available harnesses:

```json
{
  "harnesses": [
    {
      "name": "meshtasticd-3.x",
      "compose_profile": "harness-meshtasticd",
      "image": "meshtastic/meshtasticd:3.5.0",
      "exposes": {
        "tcp": [{"port": 4403, "protocol": "meshtastic-protobuf"}]
      },
      "smoke_contract_schema": "meshtastic.smoke_contract.v1",
      "real_dependencies": [
        {"groupId": "com.geeksville.mesh",
         "artifactId": "meshtastic-protobufs",
         "version_range": "[2.x,3.x)"}
      ],
      "domain_description": "Real Meshtastic protocol over TCP.
        Boots as a sidecar in the sandbox network. Verifies
        protobuf-shaped behaviour without hardware."
    }
  ]
}
```

Operators add harnesses to a deployment by:
1. Adding a compose profile (e.g. `harness-meshtasticd`) that
   brings up the sidecar service
2. Adding a `harnesses.json` entry pointing to that profile
3. (Optional) Adding the `smoke_contract.<schema>.v1` payload type
   to the registered payload set

The catalog is **operator policy**. The chain consumes it; the
chain does not synthesise it. This is the explicit DevOps boundary.

### 2. Smoke contracts are runtime-executed, not declared

The architect's `dev_via_spec.artifact.v1` payload gains two fields:

```
harness: "meshtasticd-3.x"        // refers to harnesses.json entry
smoke_contract: {                 // schema validated against
                                  // harness.smoke_contract_schema
  scenarios: [
    {
      name: "POSITION_APP packet → CS API observation",
      given: "meshtasticd publishes a protobuf POSITION_APP
              packet on TCP/4403 from node 0xABCD",
      when:  "driver MeshtasticSensorModule is started",
      then:  "within 500ms a CS API observation MUST be emitted
              on the driver's output stream with non-null
              SensorML schema and matching node_id"
    }
  ]
}
```

The builder's `mvn verify -P<harness>` runs the smoke contract
against the real harness sidecar. The smoke contract is **executable
prose, not a declaration**. There is no "do you have the right
dependency on classpath" gate; there is "does the real packet
flowing through your real driver produce the real expected
observation."

The architect's persona requires populating `harness` and
`smoke_contract` before emitting the artifact. If no harness
matches the work the architect can scope, the architect terminates
with `decide(action="needs_harness", ...)` instead of forcing a
fit.

### 3. Coordinator as decision authority — `el jefe`

The coordinator is reframed from "intent classifier" to **the
platform's bounded-autonomy decision authority**. Its scope grows
as a function of operator config (set pre-runtime, audit-able,
revocable):

| Decision class | Default scope | Operator can extend to |
|---|---|---|
| Routing prompts to flows | Always coordinator | (already there) |
| Approving `add_source_repo` | Human approval | Coordinator approves within configured namespace allowlist + URL host allowlist |
| Approving `add_harness` (when harness-via-spec lands, ADR-034) | Human approval | Coordinator approves per per-deployment policy |
| Resolving `needs_clarification` from a sub-loop | Human review | Coordinator resolves if it has the context to do so |
| Resolving `needs_harness` from researcher | Human review | Coordinator routes: catalog has match (auto-resume) / no match (escalate to harness-via-spec / reject) |
| Authoring a new flow / rule from a prompt class | Human approval | Coordinator can `create_flow` / `create_rule` within a templated allowlist |
| Stopping a wedged loop | Human action | Coordinator can `manage_flow(stop)` if persistent failure pattern detected |

The configuration shape (proposed for v1):

```json
{
  "coordinator_authority": {
    "approve_add_source_repo": {
      "enabled": true,
      "allowed_url_hosts": ["github.com", "gitlab.com"],
      "allowed_namespaces": ["research"]
    },
    "approve_add_harness": {
      "enabled": false
    },
    "resolve_needs_clarification": {
      "enabled": true,
      "max_per_arc": 2
    },
    "resolve_needs_harness": {
      "enabled": "if_catalog_match",
      "escalate_to_harness_via_spec": false
    },
    "author_flow": {
      "enabled": false,
      "allowed_templates": []
    },
    "manage_flow_lifecycle": {
      "enabled": false
    }
  }
}
```

Pre-runtime: human declares the coordinator's bounded autonomy.
Runtime: coordinator decides within those bounds. Audit-time:
every coordinator decision is on the graph (typed payload), so the
human can review what was decided and tighten config if needed.
This is not "AI does anything"; this is "AI does what the human
authorised, structurally."

The R3.5 §addendum (ADR-031) coordinator-as-meta-reviewer pattern
becomes one specific instance of this general primitive. So does
the R3.6.2.f follow-up *"coordinator-as-source-approver"* note.

### 4. `harness-via-spec` arc — same chain shape, different deliverable

When the catalog has no match for a research-stage `needs_harness`
terminal, and the operator's `coordinator_authority.
escalate_to_harness_via_spec` is enabled, the coordinator spawns
a `harness-via-spec` arc. This is a co-equal arc to dev-via-spec
— same chain rules (research → mode-transition → planner →
reviewer → challenger → architect → builder), same sandbox
primitives, different personas and different deliverable.

The OSH-Meshtastic prompt is unusually clean for showing this:
**meshtasticd exists** as a real Linux daemon, the **Meshtastic
protobuf schemas are published**, Java protobuf-gen tooling is
off-the-shelf. The arc isn't building a SITL from nothing; it's
**wrapping the official daemon as a sandbox-compatible harness**
with a smoke-contract validator. That's the easier-end of
harness-via-spec, and it's the right shape for the showcase.

| Arc | Target | Smoke Verifies |
|---|---|---|
| dev-via-spec | application code that integrates with named harnesses | "given real packet, driver emits real observation" |
| harness-via-spec | harness wrapper (compose service + smoke-contract schema + protobuf binding adapter) | "given the official upstream image as ground truth, this harness exposes the same protocol surface and produces the same observable behaviour" |

The deliverable of harness-via-spec is a candidate
`harnesses.json` entry plus the compose profile + Dockerfile +
schema files. The arc terminates with `builder_decide(tests_passing)`
referencing **integration smoke against the upstream ground-truth
image** (e.g., the harness wrapper passes packets through the
real `meshtastic/meshtasticd:3.x` and validates round-trip
fidelity). Fidelity-against-ground-truth is the bottom of the
recursion — for genuinely novel domains with no upstream daemon,
the chain must terminate `needs_hardware` and the operator
provides the bottom of the stack.

#### Catalog promotion

A successful harness-via-spec arc produces a candidate harness
on disk, NOT a live catalog entry. Promotion is a separate
decision:

- **Default**: human operator approves promotion. The coordinator
  routes the candidate as a `harness.candidate.v1` payload + a
  diff against `harnesses.json`; operator reviews and merges (or
  rejects).
- **Configurable**: `coordinator_authority.author_harness.
  auto_promote_if` predicate enables coordinator-side promotion
  when the candidate satisfies operator-set criteria — e.g.,
  *"upstream image is on an allowlist of trusted publishers AND
  smoke contract has ≥N assertions covering each declared
  message type."* This is the exact same policy shape as
  `approve_add_source_repo`: pre-runtime config bounds runtime
  decision authority.

#### What the harness-via-spec arc produces (concrete)

For OSH-Meshtastic, harness-via-spec output is:

```
docker/compose/harnesses/meshtasticd.yml      // compose profile
                                              // brings up upstream
                                              // image as sidecar
schemas/meshtasticd.smoke_contract.v1.json    // schema for
                                              // contracts written
                                              // against this harness
configs/harnesses.json (additive entry)       // catalog entry
test/harness/meshtasticd/                     // smoke-validator
  ├── pom.xml                                 // Maven harness shim
  ├── src/main/java/.../HarnessShim.java      // exposes contract
  │                                           // surface; bridges
  │                                           // to upstream image
  └── src/test/java/.../FidelityTest.java     // verifies shim
                                              // matches upstream
                                              // behaviour
```

The shim is what application drivers connect to during smoke;
it's a thin wrapper that delegates to the upstream image but
exposes a stable contract surface for the sandbox. The fidelity
test is what the harness-via-spec builder's `mvn verify` runs to
prove the wrapper is faithful.

This shape sidesteps the "build a SITL from scratch" cost in the
common case (an upstream image exists). For novel domains the
chain still works but takes longer and may need real hardware as
a ground-truth oracle.

### How the four combine on the OSH-Meshtastic demo

**Catalog-hit path** (deployment already has `meshtasticd-3.x`):

```
User prompt: "Build OSH driver for Meshtastic..."
     ↓
[coordinator] routes to dev-via-spec arc
     ↓
[researcher] consults harnesses.json → match → emits research.artifact
             with harness="meshtasticd-3.x"
     ↓
[planner/reviewer/challenger/architect] iterate; architect emits
     dev_via_spec.artifact.v1 with smoke_contract scoped to the
     meshtasticd schema
     ↓
[builder] mvn verify -Pmeshtasticd-harness runs against real
          meshtasticd sidecar; smoke contract executes; tests pass
     ↓
[builder_decide] tests_passing(integration_smoke_passed=true,
                                harness="meshtasticd-3.x") → DONE
```

**Catalog-miss path with coordinator-author authority** (the
showcase — fresh deployment, OSH-Meshtastic prompt, coordinator
authorised to escalate to harness-via-spec):

```
User prompt: "Build OSH driver for Meshtastic..."
     ↓
[coordinator] routes to dev-via-spec arc
     ↓
[researcher] consults harnesses.json → no match → emits
             research.artifact with needs_harness="meshtasticd-3.x"
             AND terminates the arc with decide(needs_harness)
     ↓
[coordinator] consults coordinator_authority:
              - resolve_needs_harness.enabled = "if_catalog_match"
                → no match
              - escalate_to_harness_via_spec = true
                → SPAWN harness-via-spec arc with the same harness
                  identifier as target
     ↓
======= harness-via-spec arc =======
[harness-researcher] discovers meshtasticd as upstream image,
                     finds protobuf schemas published, names ground-
                     truth as meshtastic/meshtasticd:3.x
     ↓
[planner/reviewer/challenger/harness-architect] iterate; emits
     harness.spec.v1 with:
       - upstream_image: meshtastic/meshtasticd:3.x
       - exposed_protocols: [meshtastic-protobuf over TCP/4403]
       - smoke_contract_schema: meshtasticd.smoke_contract.v1
       - fidelity_assertions: [round-trip protobuf packets through
                                shim and through upstream; observable
                                behaviour matches]
     ↓
[harness-builder] writes Dockerfile + compose profile + harness
                  shim (Maven project that exposes the contract
                  surface and bridges to upstream image); runs
                  mvn verify which boots the upstream image as a
                  child sidecar and runs FidelityTest
     ↓
[builder_decide] tests_passing → harness-via-spec arc terminates
                 producing a `harness.candidate.v1` payload + a diff
                 against harnesses.json
     ↓
[coordinator] checks coordinator_authority.author_harness:
              - auto_promote_if predicate satisfied
                (upstream image on allowlist + assertions ≥ minimum)
                → promote to catalog
              - else → route to operator approval
======= back to dev-via-spec arc =======
     ↓
[dev-via-spec] resumes from the original prompt; researcher now
               finds meshtasticd-3.x in the catalog (just promoted);
               chain proceeds as in catalog-hit path
     ↓
DONE — operator gets:
  - a working OSH-Meshtastic driver
  - a freshly-promoted meshtasticd-3.x harness in the catalog
  - audit trail showing both arcs, both smoke-verified, both
    against real bytes
```

This is the platform pitch in one prompt. BMAD would have
produced stubs (verified 2026-05-03 against current docs).
OpenSpec opsx would have produced specs the user-or-developer
implements without a verification anchor (verified same date).
SemTeams would have **built the harness, then the driver, both
verified against real protocol bytes, both audit-trailed, both
within an operator's bounded-autonomy contract.** Neither
comparable framework can do this; both lack the persistent-
runtime + sandbox-isolation + decision-authority primitives.

**Catalog-miss path without coordinator-author authority**
(restrictive deployment):

```
[researcher] → needs_harness terminal
     ↓
[coordinator] consults config:
              escalate_to_harness_via_spec = false
              → user-facing escalation: "this prompt requires
                meshtasticd-3.x harness; not available in this
                deployment. Operator action: add to catalog or
                enable harness-via-spec."
     ↓
DONE (with clean signal, not stubs)
```

Three paths, three different pre-runtime postures, all with the
same property: **the chain has nowhere to mock.** If the harness
isn't there and the coordinator can't authorise building one, the
prompt is rejected. There is no fourth path where the LLM
invents its own reality.

## Why this is the SemTeams-distinguishing move

BMAD V6 and OpenSpec opsx (verified 2026-05-03 against current
sources) both rely on persona-discipline-plus-operator-vigilance
for real-stack integration. Neither has a curated runtime catalog,
neither boots harnesses next to code-gen, neither has a structural
decision-authority primitive. Both would produce the stubs we
produced in R3.6.2.g.

The pieces this ADR introduces are not pieces the comparable
frameworks can add without rebuilding:

- **Curated harness catalog** requires the platform to be a
  persistent runtime that operators configure. BMAD and OpenSpec
  are CLIs against the user's local machine; they have nowhere to
  put a catalog.
- **Harness-as-sidecar in the sandbox** requires sandbox isolation
  + compose-profile-ish runtime extension. SemTeams already has it
  via R3.6.1's compose plumbing. BMAD/OpenSpec do not run code at
  all (OpenSpec) or run it in the user's local checkout (BMAD).
- **Coordinator as configurable decision authority** requires a
  multi-tenant runtime where operator policy is set ahead of time
  and applied per-decision. BMAD's persona phases are one-shot;
  OpenSpec has no agent-side decision loop.

For dev-loop usage on a developer's laptop with a frontier model,
BMAD is faster and simpler. For a multi-tenant platform that runs
workflows on someone else's infrastructure with audit, retry, and
real-stack verification, this is the architecture.

## What this ADR explicitly does NOT decide

- **The full coordinator-authority config schema.** The seven rows
  in the proposed table are illustrative; the actual schema lands
  with the first slice that consumes it (probably
  `approve_add_source_repo` since that's already the friction
  point).
- **The smoke-contract schema for non-Meshtastic harnesses.** Each
  harness defines its own `smoke_contract.<schema>.v1` payload
  type. v1 ships with `meshtasticd-3.x` as the demo harness;
  subsequent harnesses extend the schema set additively.
- **The full harness-via-spec persona content.** §4 below
  designs the arc; the persona fragments + their port lineage
  land with R3.7.4. Architectural shape is decided here so the
  OSH-Meshtastic demo can run both arcs end-to-end.
- **Whether the coordinator persona re-uses the existing
  `coordinator` role or warrants a new role name.** The "el jefe"
  framing is a working name. The coordinator persona will gain
  fragments per authorised decision class; whether to fork roles
  by decision-class authority is a v2 question once the catalog of
  decisions is bigger.
- **Migration path for the existing `add_source_repo` approval
  gate.** Today it's human-only; the coordinator-extension is
  additive. First slice that wires `coordinator_authority.
  approve_add_source_repo` should land alongside the e2e fixture
  that exercises it.

## Phasing

Five slices in the R3.7 line, designed to converge on the dual-
arc OSH-Meshtastic showcase. Each is a separate PR with its own
reviewer pass. The first three are foundational; the last two
deliver the showcase demo.

### R3.7.1 — Harness catalog primitive

- `configs/harnesses.json` schema + loader (operator-readable, KV-
  backed, hot-reloadable).
- Catalog API exposed read-only at `/teams-dispatch/harnesses`
  for the researcher persona to consult.
- Researcher persona update: consult catalog; emit `harness`
  field on the research artifact, or `needs_harness` terminal
  if no match.
- Reviewer persona: gate on harness named (or `needs_harness`
  trail clean).
- e2e mock fixture covers both catalog-hit and catalog-miss
  paths (researcher emits expected terminal in each case).
- Initially the catalog is empty; first entry lands with R3.7.2.

### R3.7.2 — Smoke contract execution (dev-via-spec consumes catalog)

- First catalog entry: `meshtasticd-3.x` (operator-curated, NOT
  yet built by harness-via-spec — just wraps the upstream image
  in a hand-written compose profile + smoke schema). This is the
  **off-the-shelf-curation** path that exercises catalog
  consumption before the auto-build path lands.
- `dev_via_spec.artifact.v1` gains `harness` + `smoke_contract`
  fields. Schema migration via additive payload version bump
  (`v1` → `v2` with `omitempty` defaults so older artifacts
  validate).
- Architect persona update: emit smoke contract scoped to the
  named harness's contract schema.
- Builder persona update: `mvn verify -P<harness-profile>` is
  the required test command; unit `mvn test` is no longer the
  acceptance gate.
- `builder_decide` validator gains `integration_smoke_passed`
  required field (with `harness` evidence). The "exit 0 with no
  tests" loophole closure from ADR-032 §15 generalises here.
- Smoke #7: real-LLM run produces an OSH-Meshtastic driver that
  passes integration smoke against the operator-curated
  `meshtasticd-3.x` harness.

### R3.7.3 — Coordinator-as-decision-authority (config + first wiring)

- `coordinator_authority` config block in deployment configs.
- Coordinator persona gains decision-class fragments
  (`approve_add_source_repo`, `resolve_needs_clarification`,
  `resolve_needs_harness` shell — the latter is "consult catalog;
  if no match and no escalation enabled, escalate to operator").
- Wire the first decision: `approve_add_source_repo` per per-
  deployment URL/namespace allowlist. Replaces the current human-
  only approval gate when configured; falls back to human
  otherwise.
- e2e fixture: prompt that triggers `add_source_repo`;
  coordinator approves per config; chain resumes without human
  intervention.

### R3.7.4 — `harness-via-spec` arc

- New chain rules:
  `configs/rules/harness-via-spec/{01..06}.json` — same shape as
  `configs/rules/dev-via-spec/`, different role names.
- New persona fragments:
  `configs/personas/fragments/harness-{researcher,reviewer,
  challenger,architect,builder}/` — port lineage cited from the
  dev-via-spec personas; decision content scoped to harness
  authorship.
- New typed payload: `harness.spec.v1` (architect output) and
  `harness.candidate.v1` (builder output, ready for catalog
  promotion).
- Builder's `bootstrap_workspace` extension: optionally pulls a
  declared upstream image as a CHILD sidecar inside the sandbox
  network (so the harness-builder can run fidelity tests against
  the real upstream while building the wrapper).
- `coordinator_authority` extends with `author_harness` block
  (`enabled`, `auto_promote_if` predicate, allowlist of upstream
  publishers).
- e2e mock fixture: harness-via-spec arc converging to a
  candidate harness. Catalog-promotion path stubbed for the mock
  (real promotion needs operator config).

### R3.7.5 — Dual-arc OSH-Meshtastic showcase

- **Real-LLM smoke #8.** Fresh deployment with `harnesses.json`
  EMPTY but `coordinator_authority.escalate_to_harness_via_spec
  = true` and `author_harness.auto_promote_if` configured.
- Send the OSH-Meshtastic prompt. Watch the coordinator escalate
  to harness-via-spec, build the meshtasticd wrapper, promote it
  to the catalog, then resume the original dev-via-spec arc and
  produce the driver.
- Capture: total chain depth (probably ~25 loops across both
  arcs), total cost (~$5–8 estimate), audit trail, and the two
  resulting artifacts (harness + driver).
- Documentation for the demo: README under `docs/demos/osh-
  meshtastic-dual-arc.md` walking through the audit trail. This
  is the artifact early adopters compare against BMAD/OpenSpec.

R3.7.6+ (additional coordinator decision classes — `author_flow`,
`manage_flow_lifecycle` — and additional curated harnesses)
land additively as demand emerges.

## Anti-Goodhart guards on the smoke contract

The smoke contract is the place we re-introduce the metric problem
if we're not careful. Three guards:

1. **Smoke contract is executable, not declarative.** The
   architect writes Given/When/Then scenarios; the builder runs
   `mvn verify` which compiles them into JUnit integration tests
   that exercise the real harness sidecar. Pass/fail is real
   bytes, not field presence.
2. **Smoke contract schema is per-harness.** `meshtasticd-3.x`
   schema requires assertions referencing real protobuf message
   types from the protobuf-deps in the catalog. The architect
   can't just write "publishes something happens" — the schema
   validator forces it to reference observable behaviour the
   harness can produce.
3. **The harness is operator-controlled.** The chain doesn't
   declare what "real" means; the catalog does. The architect
   can scope the smoke contract within the harness's offered
   surface but cannot weaken it. If the architect's contract is
   too thin, the smoke runs but the assertions are trivial — and
   that's a reviewer/challenger gate concern, the same way thin
   seed requirements are today.

The combination: declarative contracts always game; executable
contracts against operator-curated harnesses don't have anywhere
to hide.

## Relationship to prior ADRs

- **ADR-027** (Ops Agent Phase 1, accepted) — the ops-agent
  pattern of "read-only diagnostic agent grounded in per-flow
  objective specs" is a sibling primitive. R3.7.3's coordinator-
  authority config is the analogous human-bounded-autonomy
  contract, applied to a different decision class.
- **ADR-028** (Orchestration Architecture) — the §Layer 2
  principle ("rules carry references; agents fetch content via
  read_loop_result / read_entity") still holds. `harness` is a
  reference to a catalog entry, not the catalog content.
- **ADR-029** (Product-Shell Wiring) — the harness catalog and
  the coordinator-authority config are product-shell-owned.
  Framework provides the runtime primitives (compose-profile
  isolation, typed payloads, rule routing); product wires the
  policy.
- **ADR-031** (Research-Flow + dev-via-spec) — extended by this
  ADR. The research artifact gains the `harness` field; the dev-
  via-spec artifact gains the smoke contract. The R3.5
  "coordinator-as-meta-reviewer" addendum is now one of the
  decision classes in §3 above.
- **ADR-032** (R3.6 Sandbox) — extended by this ADR. The sandbox's
  compose-profile-ish runtime is what hosts harness sidecars.
  §15's `tests_passing` gate evolves to require
  `integration_smoke_passed` once R3.7.2 lands.

## References

- `project_coordinator_architecture.md` (memory) — the
  coordinator-as-rule-triggered-decisions pattern, now generalised.
- `project_r35_coordinator_meta_reviewer.md` (memory) — R3.5
  needs_clarification routing, becomes one decision class here.
- `project_flow_editor_dropped.md` (memory) — *"Coordinator
  authors flows via tools; humans approve. /admin/flows is read-
  only inventory."* — the original framing of coordinator-as-
  flow-author. ADR-033's `coordinator_authority.author_flow`
  block makes that contract config-bounded.
- ADR-032 §addendum 2026-05-03 R3.6.2.f and R3.6.2.g — the
  smoke evidence that motivated this ADR.
- BMAD-METHOD V6 docs (verified 2026-05-03) — comparison
  framework; relies on persona discipline + optional context7
  retrieval; no harness primitive, no decision-authority primitive.
- OpenSpec opsx workflow (verified 2026-05-03) — comparison
  framework; specs explicitly forbid library declarations; verify
  step does not run code; no harness primitive.
