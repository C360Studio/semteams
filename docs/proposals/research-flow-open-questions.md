# Research-Flow Open Questions Memo

**Status:** Decision-ready — five questions, each with options + a
recommendation. Companion to ADR-031. Answers to questions 1, 2, 3,
5 block phase R1 implementation. Question 4 (SemSource action shape)
blocks R2. None of them are large; all of them must be settled
before code lands so we don't churn the contract.

**Audience:** Coby + the SemSpec / SemSource maintainers, where
their input is required (flagged per question).

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
contract surface SemSpec actually consumes — it's the right place to
gate handoff regardless.

**Decide: ☐ A   ☐ B   ☐ C**

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

**Decide: ☐ A   ☐ B   ☐ C   ☐ D**

**Sub-decision if D:** initial `MaxIter`?  ☐ 3   ☐ 5   ☐ 7

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

**Decide: ☐ A   ☐ B   ☐ C**

**Implication for R1:** if C, R1 uses today's rule pattern for
spawn-researcher (already shipped) and adds a single new rule for
"researcher emits stable artifact" → "coordinator emits handoff."
The richer hybrid surfaces in R3.

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

**Recommendation: B if the SemSource team can commit to the
event contract; C otherwise.** Either is shippable; A is cleaner
ergonomically but couples versioning across products in a way
NATS-event coupling avoids.

**Sub-decisions to confirm with SemSource team:**

- **Auth shape.** Does the call carry an identity (`X-User-Id`
  header or NATS auth token)? We have the identity-lifting
  middleware from ADR-030; whatever shape SemSource expects, we
  can pair to it.
- **Approval allowlist of SemSource instances.** Per-deployment
  config? Hard-coded? Looked up via service registry?
- **What "indexed" means.** Full-text searchable, graph-queryable,
  or both? Researcher needs to know what it can ask after the
  call returns.
- **Failure shapes.** Repo too large, auth fail, indexing timeout
  — each needs a distinct researcher response.

**Decide: ☐ A   ☐ B   ☐ C   (pending SemSource conversation)**

---

## Q5 — `ResearchArtifact` home and shape

**Requires SemSpec team input.**

The handoff payload is consumed by SemSpec. Two shape questions:

| Question | Option | Recommendation |
|---|---|---|
| **Where it lives** | A) `semstreams/agentic/research/` (framework-level), B) `semspec/` (consumer-owned), C) duplicated in both | A — framework-level. Both products import from one source. Aligns with how `agentic.ApprovalResponse` already lives upstream. |
| **What it carries** | Minimal seed (actor + integration point lists, seed Requirements) vs. fuller artefact (preliminary work breakdown, risk register, prior-art notes) | Minimal seed. SemSpec's planner is the right place to expand into a work breakdown; the research flow's job is to make that planner's job possible, not pre-empt it. |
| **Versioning** | Strict (semver on the payload type); drift-tolerant (registered via `payloadbuiltins.Register`); both | Both. Strict semver in the schema; drift-tolerant via the registry's `Domain.Category.Version` triple. |

**Sub-decisions to confirm with SemSpec team:**

- **Reuse `ActorDef` / `IntegrationPoint` types.** Use SemSpec's
  existing types if they exist, or upstream a shared definition?
- **Seed Requirements granularity.** What does SemSpec's planner
  need to consume? Single-sentence requirement strings? Full
  structured `Requirement` objects? Somewhere in between?
- **What signals "stable" to SemSpec.** Is the existence of a
  `ResearchArtifact` event sufficient, or does SemSpec need a
  separate ready-to-consume signal?

**Decide:** Defer detailed schema until R3 spec; co-design with
SemSpec team. Above answers locked in for the strategic
direction.

---

## Decisions matrix (fill before R1 starts)

| # | Question | Recommendation | Decision |
|---|---|---|---|
| 1 | Reviewer scope | A — output-side only | ☐ |
| 2 | Stop condition | D — MaxIter + reviewer + optional human gate; MaxIter = 5 | ☐ |
| 3 | Coordinator action space | C — hybrid rules + tool-using persona | ☐ |
| 4 | SemSource action shape | B — NATS-event; pending SemSource team | ☐ |
| 5 | ResearchArtifact contract | upstream framework-level + minimal seed; pending SemSpec team | ☐ |

---

## What unblocks R1 specifically

R1 needs Q1, Q2, Q3 settled. Q4 and Q5 block R2 and R3
respectively. R1 can ship with:

- Output-side reviewer-as-enumerator
- MaxIter + reviewer stop condition (no human gate yet)
- Hybrid action space using existing rule pattern (one new rule for
  "research stable" → "coordinator emits handoff" placeholder; no
  SemSource integration; no real handoff yet — just a stable
  event)
- A bounded prompt against an already-indexed corpus

Bounded prompt candidate: **"Identify the actor types in OSH's
driver framework, given the OSH core repo is already indexed in
our test SemSource."** Single corpus, well-defined target, no
source acquisition. Strips the Q4 dependency from R1 and lets us
exercise R1's stop-condition decision against a real artefact.

---

## Risk if we skip the memo and code R1 directly

The fallback for "we'll figure it out as we go" is well-trodden
and bad. Specifically:

- **Stop condition decided by accident** — whichever of A/B/C/D we
  end up with is the one whoever wrote the loop chose. We then
  spend Q4/Q5 cycles relitigating it.
- **Reviewer scope creep** — output-side reviewer slowly accretes
  process-side checks because the trajectory is right there.
  Reviewer becomes a god object.
- **Coordinator action shape settled by accretion** — each new
  decision adds either a rule or a tool, no consistency. Becomes
  ungovernable.
- **`ResearchArtifact` shape leaks SemSpec internals** — without
  pre-coordination, R3 builds whatever the researcher happens to
  produce, SemSpec gets an awkward seed it has to massage.

The memo is two pages. The cost of getting these decisions wrong
is months of refactoring. The trade is obvious.
