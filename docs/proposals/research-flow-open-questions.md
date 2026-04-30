# Research-Flow Open Questions Memo

**Status:** Q1, Q2, Q3 decided 2026-04-29. Q4 decided in shape
(Option B); SemSource team confirmed the contract specifics inline
below; the few remaining sub-codes (size cap, timeout) tracked
upstream. Q5 reframed 2026-04-29 — no cross-product handoff to
SemSpec; SemTeams ports best-of-SemSpec patterns as an internal
"dev-via-spec" flow. R1 implementation is unblocked.

**Audience:** Coby + the SemSource maintainers (Q4 sub-codes only).
SemSpec team is no longer in the loop on Q5 — the reframe drops
that dependency.

---

## Q1 — Reviewer scope: output-side, process-side, or both?

The reviewer-as-enumerator pattern from SemSpec evaluates the
*output*. The research flow has a parallel choice: does the reviewer
also evaluate the *trajectory* (did we keep searching the same dead
end, did we miss an obvious source)?

| Option | What the reviewer checks | Cost |
|---|---|---|
| **A — Output-side only** | "Does the artifact name actors X, Y, Z? Does it enumerate integration points? Does seed Requirements decompose?" | Cheap. Stateless. Ports SemSpec pattern as-is. |
| **B — Process-side only** | "Did the researcher revisit dead ends? Did it search adjacent sources?" | Needs trajectory introspection. Catches thrash that A misses. |
| **C — Both** | A + B in one reviewer pass | Most thorough. Couples evaluation to internal trajectory shape; harder to evolve the researcher independently. |

**Recommendation: A.** Ship output-side first. Process-side becomes
relevant only if A produces visible thrash; we can layer it in as a
second reviewer once we have evidence. Output-side is also the
contract surface that gates the coordinator's mode transition into
dev-via-spec — it's the right place to gate the boundary regardless.

**Decided: ☑ A   ☐ B   ☐ C**  *(2026-04-29)*

---

## Q2 — Stop condition for research iteration

The hardest question. Three candidates, each with a different
failure mode.

| Option | How it stops | Failure mode |
|---|---|---|
| **A — Reviewer approval only** | Reviewer says "no gaps"; loop terminates | Runaway iteration if reviewer is too strict; no fallback when reviewer itself loops |
| **B — MaxIter + degraded handoff** | Loop terminates at `max_iterations`; emits a marked-incomplete artifact | Always terminates. May produce confidently-wrong artifacts when iteration runs out before quality is reached |
| **C — Human approval at the boundary** | Researcher emits "ready?" signal; human approves before handoff. Reuses the approval gate from ADR-030 | Reliable. Gates every research run on a human; unscalable past a few flows per day |
| **D — MaxIter + reviewer + optional human gate** | Reviewer is the primary gate; MaxIter is the safety net; human-gate is an opt-in config flag | Fails open to a marked-incomplete artifact under pathological cases; promotes to human review when the deployment opts in |

**Recommendation: D.** Start strict — `MaxIter = 5`, reviewer
required, human-gate off by default but present in config as
`research_handoff_requires_approval: false`. If we see runaway
iteration in practice, tighten the reviewer or flip the flag on per
deployment. If we see confidently-wrong artifacts at MaxIter,
tighten the reviewer's enumeration or raise the cap with eyes open.

The combined gate is the only one that always terminates, always
has a defensible artifact at termination, and gives operators a
knob without forcing every deployment to use it.

**Decided: ☐ A   ☐ B   ☐ C   ☑ D**  *(2026-04-29)*

**Sub-decision if D:** initial `MaxIter`?  ☐ 3   ☑ 5   ☐ 7

---

## Q3 — Coordinator action space: rules-driven or tool-using persona?

Today the coordinator decides via rule-triggered persona swap. The
research flow needs the coordinator to make several runtime
decisions (spawn researcher, request source, decide stabilisation,
emit handoff). Two shapes for that:

| Option | How the coordinator decides | Bounded-agency posture |
|---|---|---|
| **A — Parameterised rules** | Rules carry conditions like `if research_state=needs_corpus, request add_source`. Coordinator's job is to update `research_state`; rules drive the action | Strongly bounded. Action space is the rule set. Predictable. Less expressive — every new decision shape needs a new rule |
| **B — Tool-using coordinator persona** | Coordinator persona has an allowlist of tools (`spawn_researcher`, `request_source`, `emit_artifact`). LLM decides which to call when, gated by `approval_required` for side effects | Less strongly bounded. Action space is the tool allowlist. Flexible. The Lubin warning — unbounded-agency at the coordination layer — sharpens here |
| **C — Hybrid** | Rules drive the *decision points* (when to act); coordinator persona decides *how* (which tool with what args) within an approved action set | Most pragmatic. Rules bound when the coordinator gets to act; the persona bounds what it can do; approvals bound side effects |

**Recommendation: C.** The hybrid is what we already do for
researcher-spawn (rule fires on classifier output, coordinator
runs); generalising it to source-acquisition and handoff-emission
is consistent with the existing pattern. It also gives us a clean
ADR follow-on after R1 surfaces concrete examples — "here's what
the coordinator actually decided when, here's the bounding."

Pure-rules (A) understates how much of research is genuinely
emergent. Pure-tool-using-persona (B) is the unbounded-agency
case Lubin flagged; we don't have the operational maturity to run
it without a forcing function for every governance question at
once.

**Decided: ☐ A   ☐ B   ☑ C**  *(2026-04-29)*

**Implication for R1:** R1 uses today's rule pattern for
spawn-researcher (already shipped) and adds a single new rule for
"researcher emits stable artifact" → "coordinator advances to next
stage." The richer hybrid surfaces in R3 when dev-via-spec mode is
wired (see Q5 reframe).

---

## Q4 — SemSource action space and contract

**Requires SemSource team input.** This is the only question that
isn't internal to SemTeams.

The research flow needs to add sources. Three shapes:

| Option | What the action is | Coupling |
|---|---|---|
| **A — Direct REST/MCP call to a SemSource instance** | Tool calls SemSource's HTTP API; awaits indexing-complete response | Tight. SemSource API is a stable seam |
| **B — Fire-and-forget event** | Tool publishes `source.add.requested` on a shared NATS subject; SemSource consumes, indexes, publishes `source.add.indexed`; researcher waits on the indexed event | Loose. Decoupled via NATS. Same pattern as our other agentic events |
| **C — Manual today, automated later** | R2 ships with a documented manual step: "human adds source to SemSource via its UI; researcher proceeds." R2.5 wires automation | Smallest scope; fastest to ship; trades end-to-end automation for cross-product API stability |

**Recommendation: B.** NATS-event decoupled. SemSource team
confirmed the contract shape inline below.

**SemSource team's reply (received 2026-04-29):**

> 1. **Auth:** NATS connection creds for authz (no headers).
>    Provenance (`actor`, `on_behalf_of`, `trace_id`) goes in the
>    request body and we'll stamp it onto source metadata.
>    ADR-030's lifted identity drops into the provenance block
>    as-is.
> 2. **Targeting:** Per-namespace subject
>    `graph.ingest.add.{namespace}` for v1. Allowlist of
>    namespaces lives in semteams config — we trust whoever can
>    publish. In-process Go API offered for co-located
>    deployments.
> 3. **Indexed:** semsource reports up through "graph-queryable"
>    via the existing `graph.ingest.status` subject — researcher
>    waits for `source_status.phase ∈ {watching, idle}` then
>    queries via `graph.query.*`. Full-text/embedding readiness
>    is downstream (semembed/semsearch) and out of scope for
>    their reply.
> 4. **Failures:** Sync errors in the reply; async errors in
>    `last_error` field on `SourceStatus` (new field to add).
>    v1 codes listed; size and timeout codes deferred.
>    Researcher gets typed handles, no log scraping.

**Resolved for R2:**
- Authn via NATS creds; identity drops into the request-body
  provenance block via the ADR-030 middleware.
- Publish on `graph.ingest.add.{namespace}`; namespace allowlist
  lives in semteams config.
- Wait on `graph.ingest.status.phase ∈ {watching, idle}` to
  proceed; query via `graph.query.*`.
- Failures: sync errors in the request reply; async errors via
  `SourceStatus.last_error` typed-code field.

**Pending (does not block R2 start):**
- Specific size-cap and indexing-timeout error codes — SemSource
  team is finalising. R2 ships with whatever codes are stable at
  build time; later codes plug into the same `last_error` field.

**SemSource readiness confirmed 2026-04-29 (later same day):**

> "Ready for SemTeams to integrate against — they can publish
> AddRequest to graph.ingest.add.{namespace} and parse AddReply.
> Open questions from the ADR (queue-group semantics under
> federation, on-transition status latency) are unaffected by the
> implementation; we can revisit if they hit them."

R2 is fully unblocked. The two ADR-level open questions
(federation queue-group semantics, on-transition status latency)
are implementation-agnostic — we can build R2 against the
contract as specified and revisit only if our journey hits the
edge cases.

**Decided: ☐ A   ☑ B   ☐ C**  *(2026-04-29; SemSource team ready
for integration; sub-codes for size/timeout still finalising
async, do not block R2)*

---

## Q5 — Cross-product handoff vs internal dev-via-spec mode (REFRAMED 2026-04-29)

**Original framing was wrong.** The original Q5 assumed SemTeams
emits a `ResearchArtifact` event that SemSpec consumes via its
planner pipeline. After re-examining the cost of cross-product
coordination — versioning, schema drift, blocking on SemSpec team
velocity, the awkwardness of SemTeams owning the unbounded research
arc and SemSpec owning the bounded planning arc with a wire contract
between them — we concluded the boundary is in the wrong place.

**Reframed direction:** SemTeams does NOT hand off to SemSpec at
runtime. SemTeams ports the *patterns* from SemSpec that earn their
keep, and runs them as an internal "dev-via-spec" flow inside the
same coordinator. After research stabilises, the coordinator
transitions into dev-via-spec mode — a SemTeams-internal flow + rule
set + persona set that mirrors SemSpec's planner / reviewer /
architect / challenger arc, without runtime dependency on SemSpec.

**Why this is actually cleaner:**
- No cross-product API to version, deprecate, or coordinate. The
  `ResearchArtifact` becomes an *internal* state object that gates
  a mode transition within the coordinator, not a wire contract
  with another product.
- SemSpec stays as a *pattern source*, not a runtime dependency.
  We read its code, port what works, leave what doesn't.
- Single coordinator owns the whole arc — research → stabilisation
  → dev-via-spec — which is the natural shape of the OSH-class
  prompt.
- Avoids the SemSpec team's velocity blocking SemTeams' delivery.
- Keeps the Lubin warning surface in one place: the coordinator's
  action space across the whole arc.

**Three porting options:**

| Option | Scope | Trade-off |
|---|---|---|
| **A — Heavy port** | Recreate the full SemSpec pipeline (planner / reviewer / architect / challenger / their fixed transitions) as a SemTeams flow with the same role count | Months of work. Risks rebuilding SemSpec rather than leveraging it. The "fixed pipeline" shape is exactly what SemTeams was supposed to avoid. |
| **B — Light port** | Pull the patterns we need: reviewer-as-enumerator (already in Q1), `ActorDef` / `IntegrationPoint` emission, adversarial Challenger at issue level, failure-class taxonomy with negative-memory injection. Assemble a thinner dev-via-spec flow — plan → review → execute, with coordinator-driven architecture decisions inline | Honest about what earns its keep. Skips the rigid Architect role; coordinator does light architecture. Can grow toward A later if we hit a wall. |
| **C — Defer the port** | R3 only ships the mode transition (research stabilises → coordinator switches into placeholder dev-via-spec mode that just logs). Real port lands in R4+ | Smallest R3 scope. Punts the structural decision to a later session. |

**Recommendation: B.** "Best-of" framing maps directly to light
port. Heavy port (A) reproduces the rigid-pipeline shape SemTeams
was supposed to avoid; defer (C) ships R3 with no real content.
Light port lets R3 demo the OSH-class arc end-to-end against a
realistic dev-via-spec flow without committing to a full SemSpec
clone we'd have to maintain.

**What lives where (resolved):**
- The internal stable-research artifact (was: `ResearchArtifact`
  cross-product type) is now a SemTeams-internal payload type.
  Lives in semstreams under `agentic/research/` so the agentic-loop
  can recognise it for the mode-transition rule, but it is **not**
  a cross-product contract — only SemTeams reads it.
- Ported personas live as fragments under
  `configs/personas/fragments/dev-via-spec/`.
- Ported flow lives as a config under `configs/dev-via-spec.json`.
- Ported rules live under `configs/rules/dev-via-spec/`.
- All three layered on the existing coordinator infrastructure;
  no new components required from semstreams.

**Decided: ☑ B (light port)   ☐ A (heavy)   ☐ C (defer)**
*(2026-04-29 — reframed from cross-product handoff)*

**Implication for ADR-031:** the central architectural decision
("two products communicate via ResearchArtifact contract") is
replaced by ("SemTeams runs the whole arc internally, leveraging
ported best-of-SemSpec patterns"). The ADR has been updated to
reflect this; demo discipline message changes accordingly.

---

## Decisions matrix

| # | Question | Decision | Status |
|---|---|---|---|
| 1 | Reviewer scope | A — output-side only | ☑ 2026-04-29 |
| 2 | Stop condition | D — MaxIter + reviewer + optional human gate; MaxIter = 5 | ☑ 2026-04-29 |
| 3 | Coordinator action space | C — hybrid rules + tool-using persona + approval gates | ☑ 2026-04-29 |
| 4 | SemSource action shape | B — NATS-event on `graph.ingest.add.{namespace}`; NATS-creds authz with provenance in body | ☑ 2026-04-29 — SemSource team ready for integration (size/timeout sub-codes still finalising async, does not block R2) |
| 5 | Cross-product handoff vs internal dev-via-spec | B — light port of best-of-SemSpec patterns into an internal SemTeams flow (REFRAMED from original handoff framing) | ☑ 2026-04-29 |

---

## What unblocks R1 specifically

R1 unblocked as of 2026-04-29. Q1/Q2/Q3 decided; Q5 reframed
without changing R1's shape (the post-stabilisation behavior
moved from "emit cross-product handoff event" to "advance
coordinator state for the future dev-via-spec mode transition" —
R1 just logs the stable artifact, real consumption is R3 work).

R1 ships:

- Output-side reviewer-as-enumerator
- MaxIter (=5) + reviewer stop condition (no human gate yet —
  config flag present but defaulted off)
- Hybrid action space using existing rule pattern + one new rule
  for "research stable" → "coordinator records stable artifact"
- No SemSource integration (R2 work)
- No dev-via-spec mode transition (R3 work)
- A bounded prompt against an already-indexed corpus

Bounded prompt candidate: **"Identify the actor types in OSH's
driver framework, given the OSH core repo is already indexed in
our test SemSource."** Single corpus, well-defined target, no
source acquisition. Strips the Q4 dependency from R1 and lets us
exercise R1's stop-condition decision against a real artefact.

---

## Why the memo paid off

Three concrete saves from settling decisions before code:

- **Q5 reframe surfaced before R1.** Building R1 against the
  original cross-product framing would have shipped a placeholder
  "emit handoff event" that we would have ripped out in R3 once
  the dev-via-spec direction settled. Catching it at the memo
  stage saved one round-trip through code + revert.
- **Stop condition is a deliberate choice, not an accident.**
  MaxIter = 5 with reviewer-as-primary-gate and an opt-in human
  gate is what we chose; whichever we end up with at runtime is
  not an accident of whoever wrote the loop first.
- **SemSource contract is a stable seam from day one.** R2 is
  not going to relitigate auth shape, subject convention, or
  status-watching protocol. The team's reply nailed those down.

The memo was two pages. The cost of skipping it would have been
months of refactoring across two products and one cross-team
contract.
