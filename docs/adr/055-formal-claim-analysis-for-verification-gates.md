# ADR-055: Formal Claim Analysis for Verification Gates

## Status

**Proposed (2026-06-08).** Companion ADR to
[ADR-054](054-test-harness-team-proof-environments-before-code.md).
ADR-054 decides that proof environments must exist before hard-scenario
implementation begins. This ADR decides how SemTeams should reason about
whether the claims, proof dependencies, harness profiles, evidence, and
waivers are coherent enough to route work.

## Context

AWS and Kiro have made automated reasoning feel newly relevant to
agentic development. The useful lesson is not "import a theorem prover
and declare victory." The useful lesson is that requirements and
claims can be checked for contradictions, ambiguity, and completeness
gaps before they become expensive implementation loops.

SemTeams needs the same shape, but with different constraints:

- The backend stays Go-first and dependency-light.
- Large solver or model-checking dependencies should not become core
  runtime dependencies unless they prove unavoidable.
- Formal analysis must not become another spec-driven-development
  religion. It is a verifier in the loop, not the source of truth.
- The rule engine is good at routing and gating on graph facts, but
  it is intentionally not a cross-graph theorem prover.
- The first hard use case is practical: avoid releasing an implementation
  packet when the claim's proof story is internally inconsistent.

The current rule engine is entity-local by design. Conditions read
triples on the firing entity, and actions update triples or spawn agents.
That is the right boundary for state transitions, retries, fan-out, and
gates. Cross-entity or chain-aware analysis belongs in product tools,
subscribers, or runners that stamp typed findings back onto the graph.

## Decision

SemTeams will introduce **formal claim analysis** as a staged
verification gate. It starts with a small Go-native analyzer and treats
solver-backed analysis as an optional runner, not a core dependency.

The analyzer reads structured claim/evidence state and emits typed graph
findings. The rule engine consumes those findings to route work.

```
claim graph + harness profiles + readiness records + waivers
        |
        v
formal-claim analyzer
        |
        v
formal_claims.status = passed | failed | ambiguous
formal_claims.finding.* = ...
        |
        v
rules route to implementation, test-harness, clarification, or pause
```

### D1. Analyze structured claims, not raw prose

The analyzer operates on structured product facts:

- claim ID and parent/child relationships
- proof dependencies
- harness-profile support declarations
- readiness records
- evidence links
- waivers
- explicit conflicts
- implementation-gate state

Prose specs, OpenSpec documents, BMAD-style stories, and agent plans may
hydrate this structure, but they are not the analyzer's input contract.
This keeps formal analysis inspectable and avoids pretending that natural
language autoformalization is always faithful.

Minimal input shape:

```yaml
claims:
  - id: mavlink.mission_upload.verifiable
    requires:
      - px4_sitl.boots_headlessly
      - mavsdk_server.reachable
      - vehicle_health.ready_detectable
      - mission_upload_smoke.exists
    conflicts_with: []
harness_profiles:
  - id: mavlink.px4-sitl.mavsdk
    supports:
      - mavlink.telemetry.visible
      - mavlink.mission_upload.verifiable
    smoke:
      command: task harness:mavlink:smoke
    artifacts:
      - smoke-results.json
readiness_records:
  - profile: mavlink.px4-sitl.mavsdk
    status: passed
    evidence:
      - evidence.harness.mavlink-smoke.001
waivers: []
```

### D2. Start with deterministic Go-native checks

The MVP analyzer is a small Go package or product-shell tool that checks
the high-value cases without a solver:

- missing proof dependency
- dependency cycle
- explicit conflict between accepted claims
- harness profile advertises a claim without a smoke/artifact mapping
- readiness record references an unknown profile
- accepted claim has no evidence and no waiver
- waiver covers a child claim but leaves a parent claim overclaimed
- implementation packet released before formal status and readiness pass
- stale readiness record relative to profile version
- finding with no routeable next action

These checks are intentionally boring. They should catch the majority of
the "we cannot actually prove this yet" failures before SemTeams takes
on solver complexity.

### D3. Emit findings as graph facts

The analyzer writes a small, stable envelope to the run entity:

```text
formal_claims.status = "passed" | "failed" | "ambiguous"
formal_claims.analyzer.version = "go-native-v1"
formal_claims.finding.<id>.kind = "missing_proof_dependency"
formal_claims.finding.<id>.claim = "mavlink.mission_upload.verifiable"
formal_claims.finding.<id>.dependency = "vehicle_health.ready_detectable"
formal_claims.finding.<id>.route = "test_harness"
formal_claims.finding.<id>.severity = "blocker"
formal_claims.finding.<id>.reason = "No readiness record proves the dependency"
```

The rule engine gates on the envelope, not on the analyzer internals.

### D4. Use the rule engine for routing, not reasoning

Rules consume analyzer facts:

- `formal_claims.status = passed` plus readiness evidence
  -> release implementation.
- `formal_claims.finding.*.route = test_harness`
  -> route to the test-harness team.
- `formal_claims.status = ambiguous`
  -> coordinator asks the user or operator for clarification.
- `formal_claims.status = failed` with no routeable finding
  -> pause or escalate.

The rule engine should not learn graph-wide search, dependency solving,
SMT-LIB, Alloy, TLA+, or chain-aware inference. Those belong behind the
analyzer boundary.

### D5. Keep solver-backed analysis optional and runner-based

If deterministic checks stop being expressive enough, SemTeams may add a
solver runner. The runner is optional and isolated:

- SemTeams emits `formal-claims.json` or `.smt2`.
- A pinned container or runner executes Z3, Alloy, TLA+, or another tool.
- The runner returns the same finding envelope.
- Product code depends on the envelope, not the solver library.

No solver is required to boot SemTeams. No solver is on the request path.
No solver becomes a core Go dependency without a follow-up ADR.

### D6. Treat LLM autoformalization as advisory

LLMs may propose claim structures, dependency edges, or solver encodings.
Those outputs are drafts. They must be:

- schema-validated
- grounded to source claims or artifacts
- reviewed by the analyzer or a human before they gate implementation
- preserved with provenance when they affect routing

The product must not claim that solver green means natural-language
intent was perfectly captured.

### D7. Scope formal validity narrowly

Formal claim analysis proves only the modeled properties. A passing
result means:

- no modeled dependency gap was found
- no modeled contradiction was found
- every modeled accepted claim has evidence or waiver coverage
- routing may proceed according to the configured gates

It does not mean:

- PX4 behavior is fully verified
- OSH/CS API driver correctness is fully proven
- the harness profile is operationally healthy forever
- unmodeled domain assumptions are true
- the implementation will pass tests

This mirrors the Bedrock-style limitation: validity is scoped to the
policy variables or claims that were actually modeled.

## MVP

The MVP pairs with ADR-054's MAVLink/PX4 SITL proof-environment slice.

1. Define a minimal claim/proof-dependency/readiness/waiver model.
2. Implement the Go-native analyzer as a product-shell tool or subscriber.
3. Emit `formal_claims.*` findings onto the run entity.
4. Add a small rule pack path that routes:
   - passed -> implementation
   - missing proof dependency -> test-harness team
   - ambiguous -> coordinator clarification
   - failed/no route -> pause
5. Run the analyzer before releasing an implementation packet.

Success means SemTeams can refuse to start coding a hard MAVLink claim
when the formal claim graph says the proof environment is missing or the
waiver would leave the parent claim overclaimed.

## Consequences

### Positive

- Keeps formal methods practical and product-shaped.
- Adds a cheap pre-code gate before expensive model loops.
- Makes claim/evidence gaps explicit graph facts.
- Preserves a clean path to solvers without taking large dependencies now.
- Lets the rule engine stay declarative and small.

### Negative

- Requires maintaining a small formal-claim schema.
- The first analyzer may look mundane compared with SMT-based systems.
- False confidence is possible if the modeled claim graph is incomplete.
- Optional solver support will need runner and artifact discipline later.

### Neutral

- This is product-shell work until reuse proves a SemStreams substrate
  primitive is warranted.
- Existing dev-via-test and test-harness packs remain valid. This gate
  composes with them.
- The analyzer may eventually become a SemStreams component if multiple
  products need the same envelope and lifecycle.

## Alternatives Considered

### Embed Z3 or another solver directly in SemTeams

Rejected for MVP. It adds dependency weight and shifts the product toward
solver semantics before we know that deterministic checks are insufficient.

### Make the rule engine perform formal reasoning

Rejected. The rule engine's job is entity-local state transitions and
actions. Formal reasoning is graph-wide product analysis and belongs
behind a tool/subscriber/runner boundary.

### Rely on LLM reviewers only

Rejected. Reviewer loops are useful, but this decision exists to make
obvious structural gaps deterministic and inspectable.

### Store findings only in prose artifacts

Rejected. Prose is useful for operators, but routing needs structured
facts on the graph.

## Open Questions

1. Should the first analyzer be a tool invoked by a coordinator-facing
   agent, or a subscriber that reacts to claim/harness/evidence changes?
2. Should the finding envelope use dotted predicates only, or also emit a
   typed artifact for UI rendering and audit?
3. Which existing evidence predicates should be reused versus mirrored
   into `formal_claims.*`?
4. What is the minimal waiver schema that avoids quiet overclaiming?
5. Which finding kinds are blockers versus warnings in the first MVP?
6. When does a solver runner earn a follow-up ADR?

## Addendum 2026-06-21 — Folded under the ADR-056 umbrella

[ADR-056](056-openspec-spec-driven-development-umbrella.md) (OpenSpec-
compatible, environment-gated spec-driven development) is the integrating
umbrella; this ADR is its **formal-claims gate** (ADR-056 §D4/§D6, P3),
companion to ADR-054.

The spec layer *feeds* this analyzer with concrete inputs: the OpenSpec
**Given/When/Then scenarios** ([ADR-057 §D3](057-openspec-graph-spec-model-and-create-change.md))
are the **claims**; the harness profiles + readiness records (ADR-054 §D2/§D3)
are the **proof**; the spec's `test_command`s are the **smoke**. The
analyzer's job is unchanged — check that this claim/proof-dep/readiness/
waiver graph is coherent *before* releasing an implementation packet —
but the umbrella names where its inputs come from.

**Status** stays **Proposed**; flip to **Accepted** when P3 is committed
to build (ADR-056 §How this decomposes).

## Related

- [ADR-056: OpenSpec-Compatible, Environment-Gated Spec-Driven Development (umbrella)](056-openspec-spec-driven-development-umbrella.md)
- [ADR-057: OpenSpec Graph Spec Model and `create_change`](057-openspec-graph-spec-model-and-create-change.md)
- [ADR-033](033-harness-anchored-verification-and-coordinator-authority.md)
- [ADR-036](036-test-harness-lifecycle.md)
- [ADR-042](042-coordinator-instantiated-flows-via-templates.md)
- [ADR-043](043-devcontainer-as-sandbox-spec.md)
- [ADR-044](044-dev-via-test-pack.md)
- [ADR-054](054-test-harness-team-proof-environments-before-code.md)
