# ADR-033: Coordinator-as-Decision-Authority and Multi-Arc Dependency

## Status

**Proposed — 2026-05-03.** Drafted following the R3.6.2.g real-LLM
smoke #6 quality assessment.

**Slimmed 2026-05-06.** Test-harness machinery (§1 "Harnesses are a
curated platform asset", §2 "Smoke contracts are runtime-executed",
plus R3.7.1 and R3.7.2 phasing sections, the "Anti-Goodhart guards
on the smoke contract" section, and the 2026-05-04 addenda about
catalog primitive + schema relaxation) moved to
[ADR-036](036-test-harness-lifecycle.md). ADR-033 retains the
coordinator-as-decision-authority decision (§3), the
`harness-via-spec` chain variant (§4), multi-arc dependency
management (§5), and intent-stability + inter-arc escalation (§6).
Title narrowed accordingly.

Sections below still reference §1/§2 in places; the prose-cleanup
pass to remove dangling cross-refs is a follow-up — the substance
of every kept section stands on its own.

The strategic pivot driving the split is recorded in memory at
`project_strategic_pivot_2026_05_06.md` and ADR-036 names the
test-harness lifecycle decision the post-builder slice depends on.

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

### 5. Dependency management between arcs — sequential, gated, re-spawn

The dev-via-spec arc and harness-via-spec arc have a **hard
dependency**: dev-via-spec cannot run smoke contracts against a
harness that doesn't exist yet. v1 manages this with the simplest
shape that's correct, even at the cost of extra wall-clock time.

#### Organic discovery in research, not catalog lookup

The research artifact gains a `verification_strategy` field. The
researcher's job grows from *"name actors and integrations"* to
*"name actors, integrations, AND for each integration boundary
the verification harness it requires."*

```json
"verification_strategy": {
  "required_harnesses": [
    {
      "boundary_id": "meshtastic-radio→osh-driver",
      "name": "meshtasticd-3.x",
      "status": "available" | "needs_to_be_built" | "needs_hardware",
      "rationale": "Meshtastic protocol verification requires a
                    real-protocol SITL or daemon. The official
                    upstream image meshtastic/meshtasticd:3.x
                    runs the same protobuf surface as hardware.",
      "upstream_source": "meshtastic/meshtasticd:3.x",
      "catalog_match": null   // or harness name if status=available
    }
  ]
}
```

This is **organic discovery**, not mechanical lookup. The
researcher consults the catalog (already done in §1) but ALSO
reasons about each integration boundary: *what does it take to
verify real behaviour across this boundary?* If the catalog has
nothing matching, the researcher names what *would* match, and
why. If the verification fundamentally requires hardware (e.g.,
RF physics, custom PCB), the researcher emits
`status="needs_hardware"` — the chain cannot synthesize hardware
and the coordinator escalates to operator.

The reviewer gates on **honesty**: every named integration
boundary must have a verification entry, and each entry's
rationale must be specific (not "we'll figure it out later").
The challenger probes the OPPOSITE: are any of the named
harnesses overkill for the integration boundary they cover? A
prompt that needs a Meshtastic SITL probably doesn't ALSO need a
full LoRa-physics simulator.

#### Sequential, gated execution

When the dev-via-spec research arc completes with a
`verification_strategy` containing one or more
`needs_to_be_built` entries, the dev arc **terminates cleanly**
with `decide(action="needs_harness", reason="...",
required_harnesses=[...])`. No further dev-via-spec work
happens.

The coordinator picks up the terminal:

1. **Catalog match check** — if all entries are now `available`
   (catalog could have been updated mid-run), proceed to dev-via-
   spec re-spawn (skip step 2).
2. **Authorisation check** — for each `needs_to_be_built` entry,
   consult `coordinator_authority.escalate_to_harness_via_spec`
   AND `coordinator_authority.author_harness.allowed_publishers`
   for the named upstream source. If any check fails, escalate
   to operator with structured rationale; stop.
3. **Sequential harness builds** — for each authorised entry,
   spawn a harness-via-spec arc. Wait for completion AND catalog
   promotion (per `coordinator_authority.author_harness.
   auto_promote_if`) before starting the next.
4. **Re-spawn dev-via-spec** — once all required harnesses are in
   the catalog, the coordinator re-issues the user's original
   prompt as a fresh dev-via-spec arc. The new arc's research
   pass finds all harnesses available; the chain proceeds to
   completion.

**No parallelism in v1.** Each harness-via-spec arc runs to
completion before the next starts. The dev-via-spec re-spawn
waits for all harnesses to land. Total wall-clock cost is
sum-of-arcs, not max. We accept that cost.

**No resume-from-pause.** When dev-via-spec terminates with
`needs_harness`, the arc IS DONE. We do not park its state and
resume later. The new dev-via-spec arc that runs after harness
build is a fresh chain instance with its own loop IDs, its own
research, its own audit trail. The cost is one duplicated
research arc; the benefit is statelessness — every coordinator
decision is independent and replayable.

#### Why sequential + re-spawn is right for v1

Three reasons, all decided up-front:

1. **Right > fast.** Parallelism is a coordination problem with
   partial-failure modes (one arc fails, others succeed; how do
   we resume?). Sequential has one failure mode: the slow path
   fails, the slow path is retried. We deal with one bug at a
   time.
2. **Audit-clean.** Each arc has its own loop IDs, own typed
   payloads, own decisions. No cross-arc state. Operator review
   is "did this arc do what it should have, given its inputs?" —
   not "what was the global system state when this decision was
   made?"
3. **Re-spawnability is a property we want anyway.** If a
   harness build fails partway, the operator should be able to
   fix it and re-run the dev-via-spec arc by re-issuing the
   prompt. If we'd built dev-via-spec to resume from pause, we'd
   have made the failure-recovery path *more* complex, not less.

#### What v2 might add (not now)

- **Pipelining**: dev-via-spec research could run in parallel
  with harness build, since research doesn't need the harness
  yet. Saves the duplicated research cost on re-spawn. Adds
  "wait state" complexity to dev-via-spec. Defer until research
  cost is observed to matter.
- **Parallel harness builds**: when multiple `needs_to_be_built`
  entries are independent, build them concurrently. Adds
  failure-mode complexity. Defer until prompt classes naturally
  produce multi-harness deps.
- **Coordinator-led upfront orchestration**: coordinator does
  prompt analysis BEFORE spawning any arc, identifies the full
  dependency graph, then schedules. More elegant; requires
  coordinator to be smarter about prompts than v1's coordinator
  is. Defer until coordinator has more decision classes wired.

v1's shape is deliberately the simplest correct one. The OSH-
Meshtastic showcase uses sequential re-spawn; the audit trail is
linear and easy to follow. If wall-clock becomes the constraint
on adoption, parallelism follows the data, not theory.

### 6. Intent stability and inter-arc escalation

The chain shape above (research arc finds harness gap → coordinator
escalates to harness-via-spec → coordinator re-spawns dev-via-spec)
preserves a critical invariant: **the user's intent is stable for
the duration of a chain.** The coordinator's escalation handles
*tactical sub-dependencies* within the user's intent; it does not
change the intent.

This invariant is load-bearing. Without it, a researcher persona
could decide unilaterally to start writing code, a planner could
escalate to ops, and the audit trail would just say *"the LLM
decided to switch."* That's the BMAD-style "agent figures everything
out" trap. We don't want it.

#### Where intent is classified

Intent classification is **a coordinator function**, not a
dispatch-processor function. The upstream
`agentic-dispatch.enable_intent_classification` flag is a legacy
fast-path that bypasses the actual decision-maker; it remains
available for deployments that want a cheap shortcut, but
el-jefe deployments leave it off (as semspec and semteams already
do today). Classification belongs to the role that owns reasoning,
audit, policy, and escalation — that's coordinator.

The coordinator-authority block extends naturally:

```json
{
  "coordinator_authority": {
    "classify_intent": {
      "enabled": true,
      "supported_intents": ["research", "dev", "ops", "onboarding"],
      "ambiguity_action": "ask_user"
        // or "default_to_research" / "reject"
    },
    // ... approve_add_source_repo, author_harness, etc.
  }
}
```

Slash commands (`/research`, `/build`, `/onboard`) bypass the
classifier — they're rule-routed deterministically, since the
intent is unambiguous (per
`feedback_rules_cant_classify_chat.md`). Free-form prose hits the
coordinator's classifier, which emits a typed
`intent.classification.v1` payload on the graph, and a rule fires
on the classification to spawn the appropriate downstream flow.
Same audit-able pattern as every other coordinator decision.

#### What roles within an arc may NOT do

A role inside an arc emits a terminal payload. **No role may
change the user's intent.** Specifically:

- A `dev-via-spec-architect` who realises mid-chain that the work
  is actually purely research **terminates with `decide(action=
  needs_clarification, ...)`** — does NOT escalate to a research
  arc. Coordinator decides.
- A `researcher` who realises the prompt requires building
  something **terminates with the research artifact + a
  `dev_implication` field** — does NOT spawn a dev arc.
  Coordinator decides (and per default config, this means
  returning the research result to the user; the user re-prompts
  with dev intent if they want it built).
- An `ops-analyst` who finds a bug **emits `emit_diagnosis(...)`**
  — does NOT spawn a dev arc. Coordinator decides per
  `coordinator_authority.escalate_diagnosis_to_dev` config (which
  defaults to `enabled: false`).

The pattern: **roles emit terminal payloads; coordinator decides
what comes next.** Within-arc tactical recursion (dev → harness-
via-spec → dev) is fine because the user's intent is preserved.
Inter-intent escalation (research → dev, diagnosis → dev) is a
COORDINATOR decision gated by config, not a role's decision.

#### Inter-intent escalation policy

Same primitive as the harness escalation. Operator decides
pre-runtime which inter-intent escalations the coordinator may
authorise; coordinator decides per-prompt within those bounds:

| Escalation | Default | Operator can extend to |
|---|---|---|
| `research → dev` | Disabled (research artifact returned to user) | Coordinator escalates if config permits, with optional `require_user_confirmation` |
| `diagnosis → dev` | Disabled | Same shape; high-stakes, default off |
| `onboarding → research` | Disabled | Could enable for "auto-explore my org's stack" workflows |

Each escalation produces a typed payload (e.g.,
`intent.escalation.v1`) capturing: source intent, target intent,
reason, the source artifact that motivated escalation, the policy
rule that authorised it. Audit-trail-complete.

#### Why this matters more than it looks

Without this invariant explicitly stated, a careless persona
update could make a researcher persona that "helpfully" starts
writing code when it sees a gap. That's a slippery slope into
BMAD-shape "the LLM does whatever seems right." The invariant is
a structural promise: **what the user asked for is what gets
done; if more is needed, the coordinator says so explicitly,
and the operator's policy bounded that decision pre-runtime.**

That promise is exactly what makes the platform pitch defensible:
"audit + policy + bounded autonomy" requires that intent be
tracked first-class through the chain. ADR-033 makes intent a
typed payload, not a derivable property of whichever role last
spoke.

### How the six combine on the OSH-Meshtastic demo

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
              (each blocking; sequential not parallel — see §5)
======= coordinator re-spawns dev-via-spec =======
     ↓
[coordinator] re-issues the user's original prompt as a fresh
              dev-via-spec arc. New loop IDs, new audit trail. Old
              arc remains terminated (no resume).
     ↓
[dev-via-spec] researcher now finds meshtasticd-3.x in the catalog
               (just promoted); verification_strategy entries all
               status="available"; chain proceeds as in catalog-hit
               path.
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

### R3.7.3 — Coordinator-as-decision-authority (config + first wirings)

- `coordinator_authority` config block in deployment configs.
- Coordinator persona gains decision-class fragments:
  - `05-intent-classification.md` — runs on free-form
    `user.message` prose; emits `intent.classification.v1`
    typed payload; rule routes downstream flow per
    classification. Per §6, this **replaces** the legacy
    `agentic-dispatch.enable_intent_classification` for
    el-jefe deployments.
  - `10-approve-add-source-repo.md` — per per-deployment
    URL/namespace allowlist; replaces human-only approval
    when configured.
  - `20-resolve-needs-clarification.md` — shell for the R3.5
    routing pattern.
  - `30-resolve-needs-harness.md` — shell that consults
    catalog; if no match + no escalation enabled, returns to
    operator with rationale. Full escalation lands in R3.7.4
    once `harness-via-spec` exists.
- Slash-command rules continue to route deterministically;
  classifier only fires on prose.
- e2e fixture: TWO scenarios — (a) prose prompt → coordinator
  classifies → routes to dev-via-spec; (b) prose prompt that
  triggers `add_source_repo` → coordinator approves per config →
  chain resumes without human intervention. Both verify
  classification + approval via typed-payload audit trail.

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

## Addendum 2026-05-04 — R3.7.1 framework-alignment review

R3.7.1 landed the harness catalog primitive across six slices on
PR #55: `.a` Pattern-B catalog manager + boot-time file loader,
`.b` `research.Artifact.harness` field + emit-tool plumbing,
`.c` researcher persona consults catalog (static instructions
fragment + auto-rendered list), `.d` research-reviewer enforces
the harness selection gate, `.e` mock-LLM e2e fixture verifying
the catalog-hit wire-shape end-to-end, `.f` `/harnesses` HTTP
read API. Recording the framework-alignment review here per
the project's product-shell-tool discipline (CLAUDE.md "Product-
Shell-Tool Discipline") so future agents don't re-litigate the
shape choices.

### Survey of upstream beta.39

- **Catalog primitive**: none. Closest analogues are
  `flowtemplate.Manager` (parameterised flow definitions),
  `flowstore.Manager` (flow instances), `persona.Manager`
  (prompt fragments), `rule.ConfigManager` (rule definitions).
  All are KV-backed Pattern-B managers per ADR-029. No
  test-harness / verification-target / sidecar-registry concept
  exists in the framework.
- **HTTP routing for product code**: components own
  `RegisterHTTPHandlers(prefix, mux)`; gateways the same. No
  public mux accessor on `service.Manager`. The only product-
  shell hook for wrapping framework routes is
  `Manager.UseHTTPMiddleware`.
- **Persona-fragment runtime injection**: `persona.Manager.Upsert`
  is public and idempotent against the PERSONAS KV bucket.
  `persona.LoadFromDirectory` doesn't parse the `\d+-` filename
  prefix into Priority — every file-loaded fragment defaults to
  `Category=0` / `Priority=0`. Intra-priority ordering is then
  governed by map-iteration order (a pre-existing framework gap;
  the chain works in practice because LLMs read prose well).
- **Tool surface**: `agentictools.NewNATSTriplePublisher` is the
  shared shape for tools that publish marker triples on the
  graph via request/reply on `graph.mutation.triple.add`. The
  product-shell `emit_research_artifact` tool (R3.2.2) uses it.
  No upstream tool authors a "consult external catalog" pattern
  — confirms LLM-facing catalog query is not framework-shaped.

### Decisions taken in R3.7.1

| Move | Choice | Rationale |
|---|---|---|
| Manager pattern | Mirror `flowtemplate.Manager` (KV-backed Pattern-B, History:5, file loader on boot) | Closest upstream analogue; pre-existing convention. |
| Bucket name | `HARNESSES` | Matches upstream all-caps convention (`FLOW_TEMPLATES`, `PERSONAS`, `RULES`). |
| LLM consultation | Persona-fragment auto-render (no new tool) | Coby's 2026-05-03 fewer-rich-tools principle. Catalog data is small, ambient, read-only — prompt context is the cleaner injection. A `query_harnesses` tool would force every researcher run to pay a tool-call round-trip for ambient data. |
| Synthetic fragment ID | `harness-catalog.rendered` (dot-separator, prefix-less) | Visibly synthetic; operators conventionally use `\d+-name.md`. |
| Synthetic fragment Category/Priority | `Category=0` (matches project baseline), `Priority=45` | Sorts after the static `40-harness-catalog.md` instructions within the same category. Pre-existing nondeterminism between Priority=0 peers remains. |
| Reviewer gate | Reviewer-as-enumerator: presence-or-honest-gap + membership in catalog | Reviewer can verify membership because the rendered catalog is in its prompt context (multi-role record). Architect verifies fitness in R3.7.2. |
| `harness` field on artifact | Additive `omitempty` on v1 | No schema bump; older v1 consumers see no drift. Validate stays structural; semantic XOR is reviewer's job. |
| Catalog-miss signal | `needs_harness: <description>` in `open_gaps` | No new `decide` action; structured marker fits within existing reviewer-as-enumerator pattern. |
| Triple emission | Conditional — `research.artifact.harness` triple emitted only when `harness != ""` | Triple absence = catalog-miss signal. Easier downstream rule shape (presence-test) than always-emit-with-empty-string. |
| HTTP path | `/harnesses` (not `/teams-dispatch/harnesses`) | LLM doesn't consume it; clean separation of operator/UI surface from chain-internal `/teams-dispatch/*`. |
| HTTP wiring mechanism | Product middleware (`Manager.UseHTTPMiddleware`) | Service-manager owns the chain mux internally; middleware is the only public hook for product-shell HTTP. Foundational for any future product-shell endpoint outside the component-owned namespace. |
| Test split | unit (pure-Go renderer + middleware routing); integration (KV-backed Manager + persona injection); e2e (Playwright catalog-hit); skipping a separate catalog-miss e2e because every existing journey emits artifacts without `harness` and thus implicitly tests the miss path | Marginal coverage of a dedicated catalog-miss fixture is low; it duplicates the structural shape of every research-iterative journey. |

### Migration posture

The harness catalog is **product-shell-local** in v1 because the
harness concept is currently SemTeams-specific (no semspec /
semdragon use case yet). Mirror the shape upstream when a 2nd
product needs it. Until then the package's `cmd/semteams/harness/`
location and the product-shell-tool README's migration table are
the operator-readable trail.

### Drift signals captured during the slice

These came up while building R3.7.1; future agents working in
this space should push back on them:

- "Render the catalog into the source-tree fragment file at boot."
  No — mutates `configs/personas/fragments/researcher/`, makes
  every boot a git-diff. Use `persona.Manager.Upsert` directly.
- "Use NATS-style `>` wildcard in message-logger filter."
  No — message-logger's filter is `*` wildcard, applied AFTER
  the limit window. Documented inline in
  `ui/e2e/agentic/research-harness-hit.spec.ts`.
- "Bind `graph.mutation.>` to a JetStream stream so message-logger
  retains decide's triples." NO — JetStream binding intercepts
  the request before graph-ingest's NATS request/reply responder
  can answer, breaking decide and emit_research_artifact.
  Mode-transition's GRAPH stream binds `graph.ingest.*` (the
  source-acquisition namespace) which is a different shape.
- "Add a `query_harnesses` tool so the researcher can re-fetch the
  catalog mid-loop." No — the catalog state is stable for the
  duration of a chain (no live mutations from agent code in v1)
  so the persona-fragment snapshot is sufficient. R3.7.4 may
  revisit if `harness-via-spec`'s candidate-promotion path
  introduces mid-chain catalog updates.

### Verification evidence

Smoke evidence that the wire works end-to-end:

- Unit + integration tests green: `task test`, `task test:race`,
  `go test -tags=integration ./cmd/semteams/harness/...`
- Mock-LLM e2e green: `task test:e2e:agentic:research-harness-hit`
  asserts loop-count, role distribution, terminal convergence,
  and `research.artifact.{loop_id}` payload carrying
  `harness: "stub"`.
- Boot-log evidence: `harness catalog loaded entries_loaded=1`
  + `harness catalog: rendered persona fragment injected
  fragment_id=harness-catalog.rendered catalog_entries=1
  roles=[researcher researcher-with-source-acquisition
  research-reviewer]`.
- Manual HTTP smoke: `GET /harnesses` returns the catalog,
  `GET /harnesses/stub` returns the entry, `GET /harnesses/ghost`
  returns 404 + structured error.

R3.7.2 (smoke contract execution against `meshtasticd-3.x`) is
the next slice; the harness catalog primitive built here is its
foundation.

## Addendum 2026-05-04 #2 — R3.7.2.e′ schema relaxation + first real entry

R3.7.2.e′ ships the first real catalog entry (`meshtasticd-3.x`)
and relaxes the schema to match the post-ADR-034 execution model.
Recorded here so a future agent reading §1 standalone doesn't
infer the original `compose_profile`-required shape is still
canonical.

### Schema change

- `compose_profile` is now OPTIONAL. ADR-034 §"What R3.7.2 work
  is preserved" makes the field meaningful only for the
  external-sidecar Approach (operator pre-provisions the sidecar
  via a compose profile). The dominant path going forward —
  process-local-testcontainer (sandbox + DooD + Testcontainers
  managing lifecycle in-process) — does not consult the field.
  Greenfield browser-flow chains via verification-runner inline
  `services:` in workflow YAML and also don't consult it.
- `Validate()` no longer rejects entries that omit
  `compose_profile`. When set, the field is still parsed +
  preserved on the wire (entries authored under R3.7.1's
  required-field schema, including `configs/harnesses-stub.json`,
  remain valid and demonstrate the field's continued use for
  external-sidecar flows).

### Field corrections vs. §1's example

The §1 example was the design-time sketch; the shipped entry
deviates in two places that are intentional, not drift:

- **`smoke_contract_schema`**: §1 had `meshtastic.smoke_contract.v1`;
  shipped is `meshtasticd.smoke_contract.v1`. The harness name is
  `meshtasticd-3.x` (the daemon, not the protocol family);
  daemon-specific schema name keeps a future
  `meshtastic-radio-x` (LoRa physics, RF behaviour) free to ship
  its own schema rather than overloading a shared `meshtastic.*`
  umbrella. The `domain_description` on `meshtasticd-3.x`
  explicitly excludes RF physics, making the namespace fork
  intentional.
- **`real_dependencies[0].version_range`**: §1 had `[2.x,3.x)`;
  shipped is `[3.0,4.0)`. The harness name carries `-3.x`, so a
  2.x-through-pre-3.x range was a §1 typo or stale-during-drafting
  artifact; the corrected range matches the daemon's 3.x line.

### Slice scope and verification

- `cmd/semteams/harness/catalog.go` `Validate` no longer requires
  `compose_profile`; struct tag becomes `omitempty`.
- `configs/harnesses.json` ships the meshtasticd-3.x entry per
  the corrected shape above (no `compose_profile`).
- `test/contract/harness_catalog_contract_test.go` (new) globs
  `configs/harnesses*.json`, parses + validates each, and pins
  the meshtasticd-3.x entry's stable fields (schema name,
  protobuf port, `compose_profile` absence). Image tag pinned to
  the repository (`meshtastic/meshtasticd:`) but version floats
  so routine upstream bumps don't masquerade as contract breaks.
- `cmd/semteams/harness/doc.go` example block updated to the
  corrected shape.

The renderer (`harness.RenderResearcherFragment`) was already
agnostic to `compose_profile` (the field never appeared in the
fragment) — no renderer change required. Smoke-contract execution
against this entry lands with R3.7.2.h′ + smoke #7.
