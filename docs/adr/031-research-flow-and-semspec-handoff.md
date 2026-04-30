# ADR-031: Research Flow Ownership and the Internal Dev-via-Spec Mode

## Status

Accepted — 2026-04-29 (reframed same day from the original
"SemTeams → SemSpec handoff" framing — see Q5 in the open-questions
memo for the reframe rationale).

## Context

The c360 ecosystem ships two products that look superficially similar
but solve different problems:

- **SemSpec** — issue-to-PR planner. Takes intent that is already
  shaped enough for an `ArchitectureDocument` to be assembled, runs
  a fixed pipeline (planner → reviewer → architect → challenger),
  produces structured output. Strong on bounded plans; explicitly
  scoped at MVP to assume "form is known."
- **SemTeams** — coordinator-as-classifier with runtime flow
  composition. Coordinator classifies intent, swaps personas, and
  triggers sub-flows via rules at decision points. The coordinator
  has agency at the coordination layer — it decides *when* to
  invoke a more structured sub-flow.

Two recent observations force a strategic decision:

1. **The OSH-class prompt** ("create a driver for OpenSensorHub
   using OGC Connected Systems for Meshtastic devices") is the
   north-star ask the team keeps trying to put through SemSpec.
   SemSpec chokes on it because the prompt is doing three jobs at
   once: domain research (understand OGC CS, OSH driver patterns,
   Meshtastic protocol), source acquisition (those three corpuses
   need to land in a SemSource instance before any planning can
   happen), and scope shaping (turn "build a driver" into the right
   set of epics with the right architectural boundaries). Items 1
   and 2 are explicitly out of scope for SemSpec by design.

2. **Graph-grows-during-execution.** SemSpec's planner runs against
   a stable substrate — the graph is mostly read during execution.
   A research flow's graph *grows during execution* (sources are
   added to SemSource as a side effect of the flow). That's not a
   phase difference; it's a structural difference. Retrofitting it
   into SemSpec would violate SemSpec's planner-stability
   assumption.

The question is not "should SemSpec add a research phase." It is
"what is the right home for the work that has to happen before
SemSpec can do its job, and is that home SemTeams?"

## Decision

**SemSpec stays narrow.** SemSpec's defensible position is that it
is the right shape for issue-to-PR work where the form of the
answer is known. Adding a research phase makes SemSpec worse at
that core job (more state, more stall points, more concepts in the
user's head) and only marginally better at the harder problem,
because the real research need is unbounded and iterative.

**SemTeams owns the entire arc — research AND dev-via-spec.** The
coordinator classifies "form not yet known, needs exploration,"
spawns a research flow that can iterate, modifies the SemSource
substrate (adds sources) as a side effect of execution, stabilises
on a research-shaped artifact, then **transitions into a
SemTeams-internal dev-via-spec mode** that mirrors SemSpec's
planner / reviewer / architect / challenger arc without runtime
dependency on SemSpec.

**SemSpec is a pattern source, not a runtime dependency.** SemTeams
ports the patterns from SemSpec that earn their keep — reviewer-as-
enumerator, failure-class taxonomy, `ActorDef` / `IntegrationPoint`
emission, adversarial Challenger at the issue level — and assembles
them into a thinner dev-via-spec flow inside the same coordinator.
We read SemSpec's code, port what works, leave what doesn't. No
cross-product API to version, deprecate, or co-design.

**The boundary between research and dev-via-spec is internal.**
When research stabilises, the coordinator records a stable
`ResearchArtifact` as a SemTeams-internal payload type and switches
into dev-via-spec mode via the existing persona-swap pattern. The
artifact gates the mode transition; the same coordinator owns both
sides. No wire contract with another product.

### Why the original cross-product framing was wrong

The original ADR draft (same day) proposed `ResearchArtifact` as a
cross-product event SemSpec would consume. We chose differently
after weighing:

- **Versioning cost.** A cross-product wire contract has to be
  versioned and deprecated across two release cadences. An
  internal payload type in SemTeams is one repo's concern.
- **Velocity coupling.** SemSpec's roadmap and SemTeams' roadmap
  are independent. Coupling them at runtime makes either system's
  delivery hostage to the other's priorities.
- **Boundary placement.** "SemTeams owns research, SemSpec owns
  planning" splits the natural arc of an OSH-class prompt across
  two products with different operational maturity, ownership,
  and demo lifecycles. Single-coordinator ownership is cleaner.
- **Agency posture.** The Lubin unbounded-agency warning surfaces
  most sharply at the coordinator's composition layer. Keeping
  the whole arc inside one coordinator keeps that surface
  inspectable in one place.

The reframe is not "we couldn't make the cross-product contract
work" — it is "we shouldn't have needed it in the first place."

## Reusable patterns from SemSpec

The following SemSpec patterns port cleanly into SemTeams. The
research flow uses the first three; the dev-via-spec flow uses
all four:

| Pattern | Where it earns its keep |
|---|---|
| Reviewer-as-enumerator | Research stop condition (Q1 / Q2): "did we actually find the things we needed to find." Same pattern reapplied in dev-via-spec for "is this plan actually decomposable into work." |
| Failure-class taxonomy + negative memory injection | Both flows. Research that hits the same dead end and dev-via-spec that revisits the same architectural choice are the same shape of thrash; same mitigation. |
| `ActorDef` / `IntegrationPoint` emission (was Architect role) | Research output. Stabilised research artifacts enumerate actors + integration points; dev-via-spec consumes the same shape internally as planning seed. |
| Adversarial Challenger at issue/epic level | Dev-via-spec. "Does this issue actually represent the work, or is it confidently wrong" guards the dev-via-spec flow's plan quality before execution. |

What we **do not** port from SemSpec is the **prescribed pipeline**.
The whole point of SemTeams is that the form is not known yet; the
coordinator composes the flow rather than running a fixed one. We
port roles and patterns; we assemble our own pipeline.

Concretely, the port lives at:

- `configs/personas/fragments/dev-via-spec/` — ported persona
  fragments (reviewer, challenger, architect-light)
- `configs/dev-via-spec.json` — flow config wiring the personas
- `configs/rules/dev-via-spec/` — rules that drive transitions
  within the dev-via-spec arc (plan → review → execute)

All three layered on the existing coordinator infrastructure.
No new components required from semstreams.

## What "research-shaped" means at the internal boundary

The internal contract that gates the coordinator's transition from
research mode to dev-via-spec mode is the same shape we'd have
demanded of any cross-product handoff — the difference is the
consumer is the same coordinator, not another product:

- **Sources are reachable.** The named SemSource instance contains
  every corpus the artifact references. No "we'll need OGC docs"
  TODOs left for the dev-via-spec flow to discover.
- **Actors are enumerated.** Every external system / framework /
  library the work touches is named, with a one-line role
  description. (Example for OSH: the OSH driver framework, the OGC
  CS endpoints, the Meshtastic radio interface.)
- **Integration points are enumerated.** Every actor-to-actor
  interaction is named, with the data flow direction specified.
- **Seed Requirements at decomposable granularity.** Not "build a
  driver" — closer to "implement OSH `IDriver` interface backed by
  Meshtastic radio events, exposing OGC CS observation endpoints."
  Dev-via-spec's planning persona can decompose from there.

If any of those are missing, the research flow has not stabilised
and must iterate. The reviewer-as-enumerator gate enforces this.

Although the contract is internal, treating it as a typed payload
(rather than ad-hoc state) buys us:

- A clear gating signal for the rule that fires the mode
  transition.
- A persistent audit trail of what each stable artifact contained,
  by `loop_id`.
- Forward-compatibility if a future external consumer (an
  observer, a UI inspector, a dashboard) wants to subscribe to
  research stabilisation events without becoming a runtime
  dependency.

## What SemTeams owns end-to-end

With the cross-product handoff dropped, SemTeams owns every
capability the OSH-class arc requires:

- **Source acquisition.** Tools like `add_source_repo` and
  `add_source_docs` publishing on `graph.ingest.add.{namespace}`
  per the SemSource team's contract (Q4). Approval-gated by
  default — adding to shared infra is a side effect that warrants
  the human-in-the-loop pattern landed in
  [ADR-030](030-approval-flow-ui-and-identity.md).
- **Iteration with stabilisation.** Researcher loop with reviewer
  gate. Re-iterates if gaps; records stable artifact when reviewer
  approves.
- **Coordinator action-space bounds.** The coordinator decides
  research is needed, spawns sub-flows, decides research has
  stabilised, transitions into dev-via-spec mode, drives the
  ported planner / reviewer / challenger arc. Each step is
  bounded; the *composition* is the unbounded-agency surface that
  needs guard rails. Bounding strategy: hybrid rules + persona
  tool-allowlist + approval gates on side effects (Q3 in the
  memo).
- **Dev-via-spec execution.** Light port of SemSpec's planner /
  reviewer / architect / challenger patterns running as a
  SemTeams-internal flow. Personas under
  `configs/personas/fragments/dev-via-spec/`; flow under
  `configs/dev-via-spec.json`; rules under
  `configs/rules/dev-via-spec/`.

## Consequences

Positive:

- SemSpec's defensible position holds. The narrow product stays
  narrow. We don't expand it under demo pressure.
- SemTeams gets the canonical use case it was architected for.
  Coordinator-as-classifier, runtime flow composition, agency at
  the coordination layer, persona-driven mode transitions all
  light up against a real problem.
- No cross-product API. Versioning, deprecation, schema drift,
  cross-team coordination — all dropped. SemTeams ships on its
  own cadence.
- Single coordinator owns the whole arc. The Lubin
  unbounded-agency surface is in one inspectable place; bounding
  strategy (rules + persona allowlist + approvals) applies
  uniformly.
- SemSource integration is a first-class SemTeams concern, with
  the contract pinned by the SemSource team's reply (Q4).

Negative:

- SemTeams now owns more code than originally scoped. The
  dev-via-spec flow port is real work, not a thin wrapper around
  a SemSpec call. Light port (Q5 option B) keeps it bounded but
  it is still meaningful effort in R3.
- We have to read SemSpec carefully to extract the patterns
  worth porting, while resisting the temptation to port the
  prescribed pipeline along with them. That requires discipline,
  not just engineering.
- The OSH-class prompt is not demoable end-to-end until R3
  completes. Until then, the answer to early adopters is "this
  whole arc lives in SemTeams; here's the timeline; here's the
  bounded R1 prompt we're starting with." That is a roadmap
  statement, not a capability concession — but it requires
  message discipline.
- If SemSpec eventually wants to consume some shape of stable
  research artifact for its own purposes, we will need to design
  a cross-product subscriber surface at that point. The internal
  `ResearchArtifact` payload is forward-compatible (typed
  payload, stable subject, persistent audit trail) but is not
  yet an external contract. That is the correct posture today.

## Demo discipline

When the OSH prompt comes up before R3 is ready, the answer is
**not** "we are adding research to SemSpec." That concedes an
architecture problem that does not exist. The answer is:

> "SemTeams runs the whole arc — research, then dev-via-spec —
> inside a single coordinator. SemSpec is a pattern source we
> ported from, not a runtime dependency. This prompt is exactly
> what SemTeams was architected for. Here is the R1 → R2 → R3
> timeline; here is the bounded prompt R1 is starting with."

That is a roadmap statement. Hold the line.

When the question "why doesn't SemSpec just do this" comes up:

> "SemSpec's MVP scope is intent that arrives already shaped enough
> for an ArchitectureDocument. Domain research, source acquisition,
> and scope shaping are upstream of that bound. Adding them to
> SemSpec makes it worse at its core job and only marginally
> better at the harder problem. SemTeams owns the upstream arc;
> SemSpec stays narrow."

## Out of scope for this ADR

- The specific bounding strategy for coordinator action space at
  scale. Hybrid (rules + persona allowlist + approvals) is the
  decision; concrete bounds per decision point will land as a
  follow-on ADR after R1 surfaces real examples.
- The internal `ResearchArtifact` payload's exact field set.
  Land it in R1 with the minimum the reviewer-as-enumerator
  needs to gate on; expand in R3 as the dev-via-spec consumer
  surfaces requirements.
- The dev-via-spec flow's exact persona / rule / role layout.
  Pinned in R3; the light-port commitment (Q5 option B) bounds
  the scope but does not pre-decide the layout.

## Phasing (summary; full plan in proposals)

| Phase | Decision answered | Ships |
|---|---|---|
| R1 | Stop condition for research iteration (Q1, Q2, Q3) | Iterative researcher with reviewer-as-enumerator gate. No SemSource yet. Bounded prompt against existing graph. Stable artifact recorded but not yet consumed. |
| R2 | Action-space bounds for side effects (Q4) | `add_source_*` tools publishing on `graph.ingest.add.{namespace}` per the SemSource contract, approval-gated. Researcher requests, human approves, corpus indexes. |
| R3 | Internal dev-via-spec mode (Q5 reframed) | Coordinator transitions on stable research artifact into ported dev-via-spec persona set. Personas + rules + flow templates layered on existing infrastructure. End-to-end OSH-class demo. |

OSH is the north-star forcing function for R3. R1 starts against
a bounded prompt against an already-indexed corpus.

## References

- Strategic conversation captured 2026-04-29 (Opus 4.7,
  transcript in session memory). The original cross-product
  framing and the same-day reframe both originated here.
- Open-questions memo:
  [`docs/proposals/research-flow-open-questions.md`](../proposals/research-flow-open-questions.md).
  Q5 captures the reframe rationale in detail.
- ADR-030: Approval-Flow UI — provides the human-in-the-loop
  primitive R2 builds on for source-acquisition gates.
- ADR-029: Product-Shell Wiring — the framework integration
  pattern R1/R2/R3 will mirror.
- Lubin, "Bounded vs unbounded agency in AI systems" — the
  bounding-the-action-space concern that informed Q3's hybrid
  recommendation.
- SemSpec patterns to port: reviewer-as-enumerator,
  failure-class taxonomy, `ActorDef` / `IntegrationPoint`
  emission, adversarial Challenger. Read SemSpec's code for the
  shape; port what earns its keep, leave the prescribed
  pipeline.

## Addendum 2026-04-30 — Stabilisation as converged change-log

**Status:** Accepted (R2.5 retrospective).

R2.5 shipped the chained research+source-acquisition journey (PR #38)
and surfaced an open question the original ADR did not resolve: when
the substrate (the indexed corpus) actively mutates during a research
run, what does "stabilised" mean?

The original ADR carried an implicit *stabilised = frozen* model
inherited from the cross-product framing it later dropped — the
internal `ResearchArtifact` would be a post-research snapshot of a
substrate that had stopped changing. R2.5 falsified that. Adding a
source mid-run is the *intended* behaviour, not a corner case (it is
the structural test of this ADR's invariant: research can modify the
substrate it is reasoning over).

### Decision

**Stabilisation is convergence of the change-log, not freeze of the
substrate.** Concretely:

- The artifact records substrate mutations in-flight as they happen.
  Every successful `add_source_repo` (and any future substrate-mod
  tool) appends a `Mutation` entry with `loop_id`, `revision`,
  `approved_by`, `status`, `timestamp`, original args.
- The artifact has a `revision` integer; each researcher pass
  produces a new revision (1, 2, 3, …). The mutation log is
  monotonic and append-only across revisions.
- *Stabilised* is the derived predicate
  `reviewer_approved == true && this revision's mutation delta == ∅`.
  The reviewer's approval criteria do not change because the
  substrate did — the reviewer evaluates whatever the latest revision
  is, on its own terms.

This model:

- Honours the ADR-031 invariant rather than apologising for it.
- Gives the dev-via-spec mode (R3.2+) a complete audit trail of
  *what was added* during research, not just the post-hoc actor
  list.
- Makes the gating logic a rule predicate against artifact-derived
  triples, not Go code — consistent with how every other coordinator
  decision point works.

### Pattern source vs implementation source

SemSpec models plan revisions in its `plan-manager`. We borrow the
*concepts* (numbered revisions, append-only mutation history,
reviewer-evaluates-latest) but **explicitly do not borrow** its
implementation. SemSpec's per-Plan bespoke KV-bucket ownership is
exactly the friction SemTeams was architected to avoid. SemTeams
uses its existing primitives:

- `payloadregistry` for the typed `ResearchArtifact` payload (one
  registration; no new bucket).
- `graph.ingest` for marker triples on the loop entity (revision,
  reviewer-approved, last-revision-mutation-count). Rules already
  match on these.
- `rule` (Pattern B) for the stabilisation predicate. No bespoke Go
  state machine.

If we later need to query historical artifacts, the typed payload is
on a stable JetStream subject; replay or projection lives in the
existing message-logger / objectstore primitives, not a new
plan-manager-style component.

### Ownership

`ResearchArtifact` is **SemTeams-local**. Registered in
`cmd/semteams/main.go` after `payloadbuiltins.Register(...)` via
`research.RegisterPayloads(payloadReg)`. Not upstreamed to
semstreams. Per the ADR-031 reframe rationale: no cross-product API,
no version coupling, single-product cadence.

### What ships, in order

| Slice | Ships |
|---|---|
| **R3.1** | `cmd/semteams/research.Artifact` payload type + `RegisterPayloads(reg)` wired in `main.go`. Type-only; no producer/consumer yet. Unit tests for round-trip + Validate. |
| **R3.2** | Mode-transition machinery: a tool/executor that emits the artifact onto a stable subject and writes the marker triples; a stabilisation rule that fires when the predicate is true; a coordinator transition rule that swaps personas. |
| **R3.3** | Dev-via-spec persona fragments (planner / reviewer / challenger / architect-light), flow config, dev-via-spec rules. Configs only. |
| **R3.4** | End-to-end OSH journey. Unbounded prompt → R1+R2.5 chain → stabilisation → mode transition → dev-via-spec → epic-shaped output. The demo. |

Each slice ships a Playwright spec proving the marginal capability;
prior slices stay green.

## Addendum 2026-04-30 — Framework-alignment review for R3.2 emission shape

**Status:** Accepted (R3.2.1 design review).

Before committing R3.2.1's tool-emits-artifact design, we surveyed
upstream semstreams (bumped to beta.27 in the same PR) for guidance
on generating artifacts from rules vs from agent terminal tools. The
semspec-trauma motivation: avoid silently inventing a bespoke
artifact-emission pattern when the framework has a canonical one.

### What upstream documents

Two patterns ship with explicit guidance:

| Pattern | Documented home | Consumer shape |
|---|---|---|
| Rule → `publish` action → output component | `docs/concepts/18-rule-driven-artifacts.md` (new in beta.26) | **External.** Markdown / JSON / CSV / HTTP webhook leaving the system. Doc explicitly: "If the consumer is *inside* SemStreams, prefer querying the graph directly." |
| Agent terminal tool emits triples (+ payload) | `processor/agentic-tools/decide.go`, `emit_diagnosis.go`; ADR-028 §Layer 3 | **Internal.** Coordinator/specialist agent emits a structured terminal artifact a downstream rule matches on deterministically. |

Two further upstream constraints pin the choice:

- **ADR-028 §Layer 2:** "Rules do **not** parse agent output, make
  quality judgments, or branch on the semantic content of a result.
  If a rule condition needs to branch on the content of a result,
  the rule should trigger a coordinator; the coordinator's terminal
  tool result emits a triple that a subsequent rule can match on
  deterministically."
- **ADR-028 §What's not built here:** "**Artifact store** for dev
  flows — named workspace + `write_artifact` / `read_artifact` /
  `list_artifacts` tools. Follow-up plan." Verified absent in
  beta.27. The framework anticipates artifact-emitting tools as the
  canonical pattern; the generic primitive is on the roadmap but not
  yet shipped.

### What we considered and ruled out

1. **Rule emits the typed payload from triples.** Rejected. Rule
   actions substitute triple objects as `fmt.Sprintf("%v", obj)`
   strings (`processor/rule/execution_context.go:75-101`); they
   cannot compose nested structured data (actors[],
   integration_points[], substrate_mutations[]) from per-predicate
   triples. The framework deliberately does not ship a
   `render_template` rule action — concept-doc 18 calls this out as
   product-side concern.

2. **Spawn a separate renderer agent on completion.** Rejected.
   This is concept-doc 18's pattern for shape-changing artifacts —
   a clean fit when the renderer reads from a stable substrate. For
   our case the substrate is the researcher's own in-flight tool
   calls (R2.5's `add_source_repo` calls). A separate renderer agent
   would have to re-extract that mutation log from unstructured
   `submit_work` content — exactly the
   "rules-cannot-quality-judge-unstructured-content" problem from
   ADR-028, shifted to a different agent. Inline emission keeps
   mutation-knowledge local to where it is known.

3. **Per-loop KV bucket for artifact state.** Rejected at R3.1
   (see addendum 2026-04-30 above). SemSpec's plan-manager friction
   is exactly what ADR-031's reframe was avoiding.

4. **A new payload-emission rule action type.** Rejected. The
   framework's `publish` action publishes a generic JSON envelope
   (`entity_id` + `subject` + `timestamp` + `source` + static
   `properties`); it is not typed-payload-aware and cannot draw
   nested structured data from arbitrary completion content.
   Inventing a typed-payload rule action would fork the rule engine
   for our case — the bespoke-monster path.

### What we're building

R3.2.1 implements the agent-terminal-tool pattern, scoped to the
research domain: `emit_research_artifact`, modelled on upstream's
`decide` and `emit_diagnosis`. The researcher persona instructs the
LLM to call it once per pass with the full artifact JSON before
`submit_work`. The tool:

- Validates the artifact via `research.Artifact.Validate`.
- Writes a deterministic set of marker triples on the calling loop
  entity using the framework's `agentictools.TriplePublisher`
  (revision, four count predicates, `last_revision_mutation_count`,
  `produced_at`). Rules in R3.2.2 match on these.
- Publishes the typed `research.artifact.v1` payload on the stable
  subject `research.artifact.{loop_id}` via core NATS for audit and
  forward-compat. The subject is **not** a rule trigger — the
  marker triples are. The payload is the audit trail and the
  forward-compat surface for any future external consumer.

### Migration posture

- **When upstream ships the planned generic `write_artifact` tool**
  (ADR-028 follow-up): evaluate migrating
  `emit_research_artifact` onto it. The migration is replacing a
  concrete executor with a configured one; the tool name +
  triple/payload contract stay in product code.
- **If a future external consumer surfaces** (UI dashboard, audit
  observer, external pipeline): wire an `output/file` or
  `output/httppost` component subscribed to
  `research.artifact.>`. Additive — no changes to the tool, the
  rule, or the persona.
- **If R3.4's OSH journey reveals a gap** that the agent-terminal-
  tool shape cannot cover (e.g. the artifact actually does need to
  be assembled from an in-system multi-agent reduction rather than
  a single researcher's pass): revisit and document why the
  renderer-agent pattern is needed; do not silently grow the tool's
  responsibility.

### Demo discipline (load-bearing)

For the question "why doesn't this just live in a rule":

> Rules do mechanical routing on metadata triples. Rules do not
> parse agent output or compose structured artifacts from
> unstructured content — that's the framework's documented
> contract (ADR-028 §Layer 2; concept-doc 18). Our research
> artifact is the structured terminal output of an LLM
> pass; the canonical place to mint it is the agent's terminal
> tool — same shape as `decide` and `emit_diagnosis`. We're using
> the framework idiom, not inventing one.

For "why isn't this upstream then":

> Upstream's roadmap anticipates a generic `write_artifact` tool
> (ADR-028 §What's not built here). When it ships, we evaluate
> migrating. Until then, our domain-specific
> `emit_research_artifact` is a thin product-shell tool —
> ~200 LoC + tests, no new framework primitives.

## Addendum 2026-04-30 — R3.3 dev-via-spec port: pattern-source survey

**Status:** Accepted (R3.3 framework-alignment review).

Before committing R3.3's persona-fragment port from SemSpec, we
surveyed upstream semstreams (still beta.27) and SemSpec source for
guidance on what to port and what to skip. The semspec-trauma
motivation: avoid silently growing the product shell into a bespoke
monster by importing SemSpec's prescribed pipeline along with its
prompts.

### What upstream provides (and does not)

| Surface | Status at beta.27 | Implication for R3.3 |
|---|---|---|
| `persona/` package + file-loader | Stable since beta.9; no new primitives in beta.27. | Use the existing digit-prefix fragment-file pattern. |
| Built-in tool executors (`processor/agentic-tools/executors/`) | `read_loop_result`, `decide`, `emit_diagnosis`, `personas`, `rules`, `flows`, `flow_templates`, `flow_monitor`, `bash`, `github_*`, `graph_query`, `httprequest`, `web_search`, `component_catalog`. | All four R3.3 roles run on `read_loop_result` + `decide` only — no new SemTeams-local tool needed. |
| `write_artifact` / `read_artifact` / `list_artifacts` | Still listed in ADR-028 §"What's not built here." | The dev-via-spec terminal output stays on the same pattern as R3.2.1: loop completion message + `decide` for downstream rule routing. When the artifact-suite ships, we evaluate migrating the architect-light's terminal output. |
| Cron-rule primitive (beta.27) | New, additive. | Not relevant to R3.3 — the dev-via-spec chain is event-driven on `decide` outcomes, not time-triggered. |

### What we port from SemSpec

Light port — concepts and prompt content, not pipeline. Source
files at `~/Code/c360/semspec/prompt/`:

| SemSpec source | Maps to (in this PR) | What ports across | What changes |
|---|---|---|---|
| `domain/software.go:336` (planner) | `configs/personas/fragments/dev-via-spec-planner/` | "Decompose intent into a development plan with goal/context/scope; revision path on reviewer rejection." | Input is a stable `research.Artifact` (with `actors[]`, `integration_points[]`, `seed_requirements[]`), not a freshly-shaped intent. |
| `domain/software.go:426` (plan-reviewer) | `configs/personas/fragments/dev-via-spec-reviewer/` | "Reviewer-as-enumerator: walk an explicit checklist, decide approved/insufficient with bullet-list gaps." | One-round review (not SemSpec's R1+R2 split); checklist is dev-via-spec-shaped (epic decomposition, scope coherence) not SOP-driven. |
| `domain/software.go:575` + `:1445` (adversarial QA) | `configs/personas/fragments/dev-via-spec-challenger/` | "Adversarial: find what could go wrong, not approve." Standalone role per ADR-031's four-role plan. | Operates on a planner output (not implementation/test artifacts). Probes for decomposition coarseness, scope creep, missing integration concerns. |
| `domain/software.go:832` (architect) | `configs/personas/fragments/dev-via-spec-architect-light/` | "Map plan to actors / integration points / decisions with rationale." | Lighter — the research artifact already enumerates actors and integration points; the architect's job is to ratify those into final epic-shaped seed requirements. No greenfield architecture decisions. |

### What we explicitly do NOT port

| SemSpec mechanism | Why we skip |
|---|---|
| Fixed sequential processor chain (planner→plan-reviewer→req-generator→scenario-generator→scenario-reviewer) | We use the coordinator pattern with one `agentic-loop` and rule-driven role swaps. ADR-031 §Decision. |
| Per-Plan KV bucket + plan-manager state machine | Rejected at R3.1 (see addendum 2026-04-30 above). Use `payloadregistry` + `graph.ingest` + rules. |
| `StatusRejected` / `ReviewIteration` / `MaxReviewIterations` Plan struct fields | Equivalent is the rule's `max_iterations` field + reviewer's `decide(approved\|insufficient)` terminal output. |
| `plan.mutation.revision` event surface | Equivalent is `agent.complete.>` + the rule-driven retry pattern. |
| Two-round R1/R2 review structure | One reviewer pass per role transition. R3.4's OSH journey will tell us if a second round is needed. |

### The SemTeams dev-via-spec chain

Five rules under `configs/rules/dev-via-spec/`, all event-driven on
prior agent's structured terminal output (`decide` action):

```
research-reviewer.decide(approved)              → rule_03 (R3.2.2) → planner
                                                  ↓
planner.decide(planned)                         → rule_01 → reviewer
                                                  ↓
reviewer.decide(approved)                       → rule_03 → challenger
reviewer.decide(insufficient)                   → rule_02 → planner (retry, max_iter=5)
                                                  ↓
challenger.decide(accept)                       → rule_05 → architect-light
challenger.decide(concerns_raised)              → rule_04 → planner (retry, max_iter=5)
                                                  ↓
architect-light.decide(seed_requirements_emitted) → terminal (no rule fires)
```

The `dev-via-spec` rules dir is loaded only by the new
`e2e-dev-via-spec.json` config. The R3.2.2 e2e config remains
unchanged (it does not load these rules), so R3.2.2's seven-loop
smoke test stays stable — the persona-content swap to the real
planner does not break the R3.2.2 spec because mock-llm replays
the fixture sequence regardless of persona content, and no
dev-via-spec rule loads to spawn an eighth loop.

### What we ship

- Four persona fragment dirs under `configs/personas/fragments/dev-via-spec-*/`. The `dev-via-spec-planner` stub introduced in R3.2.2 is replaced with the real planner contract; three dirs are new (reviewer, challenger, architect-light).
- Five rule files under `configs/rules/dev-via-spec/`.
- `configs/e2e-dev-via-spec.json` flow config — extends R3.2.2's research-mode-transition config with the new rule chain.
- `test/fixtures/journeys/dev-via-spec.yaml` mock-llm fixture covering the chain through architect-light terminal.
- `ui/e2e/agentic/dev-via-spec.spec.ts` Playwright journey assertion.

Configs only per ADR-031 — zero Go changes.

### Per-role rigour: each role addresses only its prior loop

semstreams beta.27's rule engine substitutes `$entity.id` /
`$entity.<predicate>` against the *triggering* entity only — there
is no mechanism for a rule's `properties` block to forward an
arbitrary upstream entity id from a prior rule's spawn
(`processor/rule/execution_context.go:75-101`). R3.3 was initially
read as constrained by this: "downstream roles can't cross-ground
against the original research artifact, so the chain compresses
content across hops." The reviewer-pass reframe corrected that
read.

**This is the right design, not a defect.** The reviewer-as-
enumerator pattern works *because* each role has one concrete
input to walk a checklist against. Forcing the reviewer to also
cross-ground against the upstream artifact splits attention,
dilutes the primary check, and (with small LLMs) likely degrades
reasoning quality. SemSpec's chain works the same way — its
plan-reviewer reads the plan, not the original intent. Chain
quality emerges from per-role rigour, not per-role exhaustive
backward reach.

R3.3 ships with each role reading **only its prior loop**, and
that is the intended shape:

```
research-reviewer.loop  ←  planner reads        (R3.2.2 rule 03 forwards prior_loop_id)
planner.loop            ←  reviewer reads       (dev-via-spec rule 01 forwards prior_loop_id)
reviewer.loop           ←  challenger reads     (dev-via-spec rule 03 forwards prior_loop_id)
challenger.loop         ←  architect-light reads(dev-via-spec rule 05 forwards prior_loop_id)
```

Each downstream role grounds against what its prior role
summarised in the `decide` reason field. That summary is the
contract; rigour at the prior role is what the next role builds
on.

**Why echo-forward (embedding upstream IDs in persona prompts) is
a Goodhart loader, not a fix.** The tempting alternative —
instruct each persona to echo upstream IDs through `decide`
reasons so the next role can `read_loop_result` deeper into the
chain — fails on four counts: it splits the LLM's attention
between primary task and bookkeeping; it creates a structural
proxy ("did the LLM include the IDs?" replaces "did the LLM
actually reason about upstream constraints?"); it encourages
performative `read_loop_result` calls that satisfy the contract
without informing reasoning; and more upstream content in context
does not equal better grounding when effective attention is
bounded. We resist the upstream rule-engine "forward-prop"
feature ask on the same logic — the persona layer does not need
it, and adding it would invite exactly the multi-hop
cross-grounding that this section argues against.

**If R3.4's OSH journey reveals drift, the right mitigation is a
structural terminal validator, not forward-prop.** A rule (or
thin Go checker) fires after architect-light emits and asserts:

- Every final seed_requirement cites an actor that exists in the
  original research artifact's `actors[]`.
- Every cited integration boundary appears in
  `integration_points[]`.
- No invented entities pass through the chain undetected.

This is structural (no LLM judgment), deterministic (rule engine
can do it; `emit_research_artifact` already mints the marker
triples the validator reads), Goodhart-resistant (the validator
reads triples, not summaries — nothing for the chain to optimise
against), and deferrable (ship R3.3 without it, add in R3.4 only
if drift is observed). The discipline generalises: prefer
structural checks over LLM-enforced cross-grounding whenever the
constraint admits a deterministic predicate (see also ADR-028
§Layer 2 on rules-as-mechanical-routing).

### Compatibility note for R3.2.2

The `dev-via-spec-planner` persona content is replaced wholesale
in this PR (R3.2.2's stub becomes R3.3's real planner). R3.2.2's
spec (`ui/e2e/agentic/research-mode-transition.spec.ts`) stays
green **only because mock-llm is sequence-based** — it replays
the R3.2.2 fixture's stub completion regardless of persona
content, and R3.2.2's e2e config does not load the dev-via-spec
rules so no eighth loop spawns. If R3.2.2's spec is ever run
against a real LLM, the new planner persona will use `decide`
rather than emit a completion message; the R3.2.2 rule's tools
list now includes `decide` (added in this PR) so dispatch
succeeds, but the completion-content assertion would no longer
match. This is an acceptable trade-off — R3.2.2 is a mock-llm
e2e by design, and a future real-LLM smoke check should target
R3.3 rather than R3.2.2.

### Migration posture

- **When upstream ships `write_artifact`** (ADR-028 follow-up): evaluate migrating the architect-light's terminal output onto the typed artifact-store path. The migration is replacing a `decide(action="seed_requirements_emitted")` terminator with a structured `write_artifact` call; persona content adapts; rules unchanged.
- **If R3.4's OSH journey reveals coverage gaps** (planner consistently mis-decomposing, reviewer missing the same class of failure): port more from SemSpec — the failure-class taxonomy (`error_categories.json`) maps to negative-memory-injection in reviewer prompts; the second-round review (R2) maps to a dual reviewer fragment. Document in a follow-up R3.4 addendum.
- **If a future external consumer wants the dev-via-spec output** (UI dashboard rendering plans, audit observer): add an `output/file` or `output/httppost` component subscribed to whatever the architect-light terminal emits; additive.

### Demo discipline (load-bearing)

For the question "why don't you just call SemSpec":

> SemSpec's MVP scope is intent that arrives already shaped enough
> for an `ArchitectureDocument`. The whole reason SemTeams owns
> this arc is that domain research, source acquisition, and scope
> shaping are upstream of SemSpec's bound. The dev-via-spec mode
> ports SemSpec's *patterns* into a coordinator that sees the
> upstream context too. SemSpec is a pattern source, not a runtime
> dependency.

For "isn't this just rebuilding SemSpec":

> We port four prompts and one taxonomy. SemSpec is sixteen
> components, a per-Plan KV bucket, a state machine, and a fixed
> processor chain — all of which we explicitly do not port (see
> table above). The dev-via-spec mode is configs only.
