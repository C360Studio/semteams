# Proposal: OpenSpec-Compatible, Environment-Gated Spec-Driven Development

**Status**: Working notes (2026-06-21). The *settled* decisions here have
graduated to **[ADR-056](../adr/056-openspec-spec-driven-development-umbrella.md)**
(umbrella, **Accepted**) — which is now the authoritative home for the
cross-cutting decisions and the decomposed north-star deployment roadmap —
and the first buildable slice (P1/P2) is specified in
**[ADR-057](../adr/057-openspec-graph-spec-model-and-create-change.md)**
(**Proposed**). This doc stays as the open-decision scratchpad and
use-case walkthrough.

> **Correction (2026-06-21, verified against semstreams beta.113):** the
> UC-2 / North-star claim below that *"no GitHub issue/PR integration
> exists today"* is **stale**. Upstream ships `github_read`
> (`github_get_issue`/`github_list_issues`/`github_search_issues`/
> `github_get_pr`/`github_get_file`) and `github_write`
> (`github_create_branch`/`github_commit_file`/`github_create_pr`/
> `github_add_comment`/`github_add_label`); they are simply not wired into
> the semteams product shell yet. Issue *creation* is the one GitHub gap.
> See [ADR-056 §North-star deployment roadmap](../adr/056-openspec-spec-driven-development-umbrella.md)
> for the corrected picture (net-new D2 work = poll/scheduled trigger +
> autonomous cost breaker + wiring, *not* GitHub access).

> **Correction (2026-06-22, verified against the OpenSpec format):** the
> **EARS** references below (the "Acceptance criteria use EARS notation"
> claim and the `+ EARS acceptance` mapping-table rows) are **superseded**.
> OpenSpec does not use EARS — it uses RFC-2119 `SHALL` statements +
> `Given/When/Then` scenarios. See
> [ADR-057 §D3 + §Grounding addendum](../adr/057-openspec-graph-spec-model-and-create-change.md).
**Author**: Coby Leuschke (with Claude)
**Related ADRs**: [042](../adr/042-coordinator-instantiated-flows-via-templates.md)
(substrate-plus-overlays), [043](../adr/043-devcontainer-as-sandbox-spec.md)
(devcontainer sandbox), [044](../adr/044-dev-via-test-pack.md) (dev-via-test
/ Lisa·Ralph·CBG), [054](../adr/054-test-harness-team-proof-environments-before-code.md)
(proof environments before code — **Proposed**),
[055](../adr/055-formal-claim-analysis-for-verification-gates.md) (formal claim
analysis — **Proposed**). Supersession context:
[041](../adr/041-mvp-role-compression-and-graph-as-substrate.md) /
ADR-035 (the retired heavy dev-via-spec chain).

## Thesis

Three observations drive this proposal:

1. **Creating a spec from a prompt has standalone value.** It is a reusable,
   reviewable, hand-off-able deliverable in its own right — independent of
   whether (or how) it is later implemented. So *spec creation* and *dev from
   spec* are **two different journeys**, not one chain.
2. **The coordinator should pick the implementation strategy** that best fits a
   given prompt/spec — `autoresearch` (optimize a metric), the `ralph`
   test-convergence loop (build to passing tests), or a heavier path — rather
   than forcing one shape.
3. **The environment is the hard part.** The machine SemTeams runs on will not
   have the target repo's toolchain or services. Before any spec or dev work can
   honestly run, the coordinator must *ensure the proof environment exists* and
   agents must be sandboxed. The **foundation is already shipped** — ADR-043's
   devcontainer sandbox + attestation (`sandboxmanager`,
   `sandbox.attestation.{ready,verified}`, `request_sandbox` /
   `query_sandbox_attestation`) supports both autoresearch and dev-via-test
   today. What is **unbuilt** is the harder proof-environment layer for
   service-heavy scenarios — ADR-054/055's harness profiles, readiness records,
   proof dependencies, and formal claim gating (only the ADR docs exist; no
   `harness`/`readiness`/`formal_claims` code or rule pack).

We adopt **OpenSpec** as the spec interchange standard (read + write), because
it is the lean end of the SDD spectrum and its data model maps almost 1:1 onto a
graph-primary harness. **Graph is the canonical truth; OpenSpec markdown is a
rendered projection (hydration) and an interchange format**, never the source of
authority — exactly the stance ADR-054/055 already take.

## North star: deployment operating model (beyond current scope)

This frames *where this is headed* so the in-scope work below reads as steps
toward it. **Most of this is not in the current proposal scope** — captured here
so the destination is shared.

The intended deployment is a **self-hosted, always-on, single-operator
program-manager**: SemTeams installed on a Mac Mini on the operator's LAN,
running continuously, coordinating work across the **`sem*` repo family**
(semstreams, semspec, semteams, semdragons), with the repos available as **local
working copies** (so "brownfield" is a local checkout, not necessarily a remote
clone).

The coordinator's role grows from "answer a prompt" to **standing cross-team
program manager**:

- **Watch + participate in the `sem*` issue queues.** Poll the queues for work,
  AND **create issues** across repos to coordinate cross-team dependencies — the
  coordinator is an active participant in the queues, not just a consumer.
  Primary flow: **issue → PR**.
- **Operator-in-the-loop over real channels.** Anything needing the operator's
  attention — `ask_user`, an approval gate, a blocked proof environment — reaches
  them via **text and/or email**, with a **deep-link back to the UI** to resolve
  it. (Extends the ADR-053 HITL surface, which is UI-only today, to push
  notifications.)
- **Timed daily roll-ups.** A scheduled summary of what ran, what shipped, what's
  waiting, and what's blocked — pushed to the operator on a cadence.
- **Lessons-learned publishing.** The coordinator maintains a **GitHub Pages**
  site, blogging lessons learned (echoes semspec's lesson-decomposer sidecar and
  the ops-agent's diagnostic role).

Net-new surfaces this implies (none in current scope; listed so they are not a
surprise later): a **notification channel** (SMS/email + UI deep-links), a
**scheduled-trigger** mechanism (time-based, not just event-based rule fires),
**multi-repo** awareness (a coordinator that spans the `sem*` family, not one
target), **issue creation** (not just consumption), and a **publishing** path
(GitHub Pages). Several map onto existing seams — operator notifications extend
ADR-053 HITL; daily roll-ups + lessons-learned extend the ops-agent (ADR-027)
and the lesson-decomposer pattern; the autonomous policy
(`restricted_decide_actions`) already exists for the no-human stretches between
notifications.

**Scope relationship:** UC-2 (issue-queue → PR) is the first concrete step into
this model; the notification/roll-up/publishing/multi-repo capabilities are
later, separately-scoped additions. This proposal does not design them — it just
records that the issue→PR work should not foreclose them.

## Non-goals (settled, do not relitigate)

- **We do not rebuild the heavy dev-via-spec chain** (ADR-035's
  planner→reviewer→challenger→architect→builder→qa). It was retired with
  evidence in ADR-041/042: *"the ceremony itself is the cost"* — orchestration
  overhead of multi-role chains exceeds the cognitive benefit at frontier-model
  scale. External confirmation: BMAD (the ceremony-heavy method semspec tried to
  wrap) runs ~$800–2k/mo/dev and 80–100× tokens for routine work. **Ceremony ×
  agents × re-read-large-artifacts is the trap.** Keep agent count and artifact
  size down.
- **We do not become a DevOps/orchestration platform.** The env layer renders
  boring runners (Compose / `act`), not mini-k8s (ADR-054 §D4).
- **We do not make OpenSpec (or any markdown) the operating truth.** It hydrates
  from, and ingests into, the graph.

## Why OpenSpec (not Spec-Kit, not BMAD)

| | OpenSpec | Spec-Kit | BMAD |
|---|---|---|---|
| Artifact weight | ~250 lines, **delta-based** | ~800 lines, full per-feature | PRD blowup |
| Brownfield | native (changes are diffs) | greenfield-leaning | greenfield/regulated |
| Cost | light | medium | heavy ($800–2k/mo) |
| Fit to graph-primary | **high** (living-state + append-deltas + reconcile) | medium | low |

OpenSpec's structure *is* the graph model:

| OpenSpec artifact | Graph (authoritative) | Markdown (hydration / interchange) |
|---|---|---|
| `specs/<cap>/spec.md` (living) | `spec.<cap>.requirement.<id>` — **owned current-state** (`replace_owned`) | rendered on demand |
| `changes/<slug>/proposal.md` | `change.<slug>.proposal.*` — append | rendered |
| `changes/<slug>/specs/<cap>/spec.md` (delta) | `change.<slug>.delta.<cap>.<id>.{add,modify,remove}` + EARS acceptance | rendered |
| `changes/<slug>/tasks.md` | `change.<slug>.task.<i>.{goal,target_files,test_command,…}` | rendered (OpenSpec-visible subset; richer fields stay graph-only) |
| archive (merge delta → living) | a `replace_owned` mutation on the living-spec predicates | re-render both |

The governed-SKG ownership work we just landed (beta.113 `replace_owned` +
owner-lease) is the substrate for *"a living spec that evolves via archived
deltas"*: living spec = owned current-state, changes = append proposals, archive
= owned replace. **Acceptance criteria use EARS notation** (`When <trigger>, the
system shall <response>`) — machine-checkable, maps 1:1 to a future test, and is
becoming the de-facto SDD standard (Kiro ships it; Spec-Kit has it on the
roadmap).

## The three layers

This initiative is the integration of three layers, two of which are already
designed (ADR-054/055) and one of which is net-new (the OpenSpec spec layer):

1. **Spec layer (net-new).** Prompt ⇄ OpenSpec change/living-spec, graph-primary.
   A `create_change` journey produces `proposal + delta-spec + tasks` from a
   prompt; an ingest path parses existing OpenSpec files into the graph.
2. **Routing layer (extends the coordinator).** The coordinator (a) gates on
   environment readiness, then (b) selects the implementation method
   (autoresearch / ralph / …) for a spec/prompt. Extends the existing closed
   `decide()` taxonomy; no new components (ADR-042).
3. **Environment-readiness layer (ADR-043 shipped; ADR-054/055 extensions
   unbuilt).** The shipped foundation (ADR-043 devcontainer sandbox + attestation)
   already provisions a container and proves it is up. The *extensions* —
   detect/match a **harness profile**, produce a run-scoped **readiness record**,
   model **proof dependencies**, and fail-closed gate on them (ADR-054), plus
   **formal claim analysis** of claim/proof-dep/readiness coherence (ADR-055) —
   are unbuilt and needed for service-heavy targets. For a typical brownfield
   Go/Node repo, ADR-043 attestation + "the repo's own test suite runs green in
   the sandbox" may be a sufficient v1 readiness check; the full harness-profile
   machinery is for external-service scenarios (PX4 SITL, OSH gateways).

The ordering matters: **env-readiness gates the dev journey** — you cannot
honestly run tests (the ralph loop's whole premise) until the test harness is
real.

## Target use cases (work through each)

### UC-1 — Brownfield repo, pick up existing OpenSpec

> "Point SemTeams at this repo; here's what I want changed."

1. **Acquire + sandbox.** Clone the target repo into a per-tenant sandbox
   (ADR-043). The SemTeams host has none of the repo's toolchain — everything
   runs in the container.
2. **Detect topology + ingest spec.** Detect build roots / package manifests /
   module boundaries (semspec's "topology facts"). If an `openspec/` dir exists,
   **ingest its living specs into the graph** (read-compat → `spec.<cap>.*`
   triples). This is the "pick up current OpenSpec specs if any" ask.
3. **Env-readiness gate (ADR-054).** Match a harness profile to the detected
   topology (today's catalog is a fixed 3-profile set: `go-backend`,
   `svelte-ui`, `full-stack-e2e` — **insufficient for arbitrary repos**;
   §Open-decisions). Provision, run the smoke, stamp a **readiness record**. If
   it can't be made ready → route to the test-harness team or fail-closed.
4. **Create change.** Coordinator turns the prompt into an OpenSpec **change**
   (delta against the ingested living specs) — `proposal + delta-spec + tasks`.
   Reviewer-gated (gap analysis, `[NEEDS CLARIFICATION]` markers).
5. **Execute.** Coordinator routes the change's `tasks` to the execution method
   (default: ralph-per-task → CBG work-gate). Tests run inside the sandbox.
6. **Deliver.** Open a PR from the sandbox branch (see UC-2), and/or archive the
   change (merge delta → living spec via `replace_owned`).

**Answer to "can I point SemTeams at a brownfield repo and pick up current
OpenSpec specs?"** — Yes; that's step 2's ingest path. Net-new work: the OpenSpec
parser (files → triples) and topology detection.

### UC-2 — Poll an issue queue, open a PR (autonomous)

> "Watch this issue queue; for each issue, do the work and open a PR."

UC-1 with an automation trigger and a no-human policy:

1. **Trigger.** Poll a GitHub issue queue (label-filtered). **No GitHub
   issue/PR integration exists today** — net-new (a poll source + PR creation,
   framework-alignment review required: rule trigger vs. subscriber vs. tool).
2. **Per issue:** ingest issue prose → coordinator → (UC-1 steps: sandbox →
   env-readiness → create change → execute).
3. **Autonomous policy.** Run with `restricted_decide_actions: ["ask_user"]`
   (ADR-053 / the autonomous overlay) — the coordinator must resolve ambiguity
   without a human, or fail-closed with a readiness/claim finding rather than
   wedge.
4. **Open PR.** From inside the sandbox: branch, commit, `gh pr create`. The
   change's `proposal.md` + `tasks.md` (rendered from graph) become the PR body.

**Answer to "can SemTeams poll an issue queue and create a PR?"** — Not today;
this is the largest net-new surface (issue source + PR creation + autonomous
loop). It composes cleanly *on top of* UC-1, so it should come after UC-1 proves
out.

### UC-3 — Greenfield

> "Build me X" with no existing repo or spec.

1. **Full spec from scratch.** Prompt → a complete OpenSpec **living spec**
   (`specs/<cap>/spec.md` set), not a delta. Reviewer-gated. This is the
   standalone "prompt → reviewed spec" deliverable in its richest form.
2. **Env is *created*, not detected.** Greenfield must **scaffold topology** —
   choose a stack, create the build root (`go.mod` / `package.json`), and
   provision a matching sandbox (the harness profile is *authored*, not matched).
   This is where ADR-054's "the first profile for a new domain may be as hard as
   the feature work" bites.
3. **Execute + deliver.** Same execution layer; deliverable is the scaffolded
   repo + PR, or an artifact.

**Note:** Greenfield is the *least* differentiated use case (every SDD tool does
it) and the *most* env-creation-heavy. UC-1/UC-2 (brownfield, the OpenSpec
sweet spot) are the higher-value, better-fit targets; UC-3 is the
completeness case.

### Use-case → layer matrix

| | Spec layer | Env-readiness layer | Routing/automation |
|---|---|---|---|
| **UC-1 brownfield** | ingest existing + create change (delta) | **detect** topology, match profile, readiness record | front-door prompt → method |
| **UC-2 issue-queue** | create change per issue | detect (as UC-1) | **poll trigger + PR + autonomous** |
| **UC-3 greenfield** | create full living spec | **create** topology (scaffold profile) | front-door prompt → method |

## dev-via-test stays intact; add `dev-from-task` (DECIDED 2026-06-21)

**Lisa does not move. dev-via-test is not refactored.** It remains the proven,
shipped, prompt-first path: prompt → Lisa(plan) → Ralph → CBG. We add the
spec-first path as a **pure addition** (substrate-plus-overlays), so the working
dev-via-test happy path stays green.

The key reuse: **the ralph loop + CBG work-gate + the coordinator's
plan-walking are the shared execution primitive.** dev-via-test wraps that
primitive with Lisa (prompt → tasks). The new `dev-from-task` journey wraps the
*same* primitive with a spec-tasks feed instead:

- **dev-via-test** (existing, untouched): `prompt → Lisa → Ralph×N → CBG`.
  Lisa lives here, and only here.
- **`dev-from-task`** (new): `tasks → Ralph×N → CBG`. Equivalent to dev-via-test
  *minus Lisa* — entered with `plan.task.*` triples **already populated** (by a
  change, or hand-authored). Reuses the existing coordinator-walk (rules
  `03`/`05` + `30-plan-walking`), the Ralph execute loop (rule `03` →
  `dev-via-test-execute`, `04a`/`04b`), and the CBG work-gate (`06`/`07*`)
  unchanged. The only dropped piece is the Lisa planning prefix (`01`/`02`).
- **`create_change`** (new) produces those tasks. Its task output lands in the
  **same dispatch shape Ralph already consumes** (`plan.task.*` on the run
  entity, or `change.<slug>.task.*` projected to it at dispatch). **That mapping
  is the integration seam.**

So Lisa is a *prompt-first planner*; `dev-from-task` is the *spec-first
executor*; both call the identical Ralph+CBG primitive. No refactor, no removal,
two clean journeys.

## Environment-readiness layer (ADR-043 shipped + ADR-054/055 extensions)

The **foundation is shipped** — ADR-043's devcontainer sandbox + attestation
(`sandboxmanager`, `sandbox.attestation.*`, `request_sandbox` /
`query_sandbox_attestation`) provisions a container and proves it is up, and is
already used by autoresearch and dev-via-test. This proposal does **not**
redesign that. It *activates the unbuilt ADR-054/055 extensions* (the env-layer
design is done in those ADRs; only the docs exist) and adds brownfield support:

- **Harness profiles** (ADR-054 §D2) are the reusable, versioned env definitions
  (services, readiness probes, smoke command, artifacts, failure signatures).
  Today's `sandboxmanager` catalog is a **fixed 3-profile builtin**
  (`go-backend`/`svelte-ui`/`full-stack-e2e`) — fine for our own repo, **not for
  an arbitrary brownfield target**. Brownfield needs **topology-driven profile
  selection/derivation**; greenfield needs **profile authoring**.
- **Readiness records** (ADR-054 §D3) are run-scoped proof the harness is usable.
- **Formal claim analysis** (ADR-055) checks the claim/proof-dep/readiness/waiver
  graph is coherent *before* releasing a work packet — fail-closed when the proof
  story is internally inconsistent (e.g., "accepted claim has no evidence and no
  waiver").
- **Gate** (ADR-054 §D5): implementation packets require a passing readiness
  record per required profile, or an explicit, expiring waiver.

The OpenSpec spec layer *feeds* this: a change's EARS acceptance criteria are the
**claims**; the test commands are the **smoke/proof**; the harness profile is
what makes those commands runnable.

## Coordinator routing (the seam)

All of this plugs into the existing closed `decide()` taxonomy (ADR-042) — new
tokens + spawn rules + persona bundles, **no new components**:

- `decide(action="create_change")` / `create_spec` — spawn the spec journey.
- Env-readiness is gated *before* execution: the coordinator (or a rule on the
  formal-claims envelope) routes to the test-harness team when a proof dependency
  is missing, exactly as ADR-054 §D1 specifies.
- Method selection — the coordinator already routes `autoresearch` vs
  `dev_via_test`; extend its decision tree to pick the executor for a spec.

**Constraints to respect:**
- The **front-door coordinator has no `bash`** — it cannot read a repo or spec
  file on first contact. Spec/repo input arrives via a wake-up coordinator (which
  does have bash), an inline paste, or a minimal `read_spec`/`acquire_repo` tool
  (framework-alignment review required).
- **Classification quality is the risk, not the mechanism.** Routing is a
  battle-tested pattern; keep the router *thin* (it routes, it does not plan —
  Anthropic spent months suppressing over-spawning for simple inputs). Validate
  on real-LLM smoke; do not Goodhart via persona patches.

## Proposed phasing (each phase independently valuable)

| Phase | Deliverable | Depends on |
|---|---|---|
| **P1 — Spec artifact + render/ingest** | Graph spec model (`spec.*` / `change.*` predicates) + OpenSpec markdown render-from-graph + parse-into-graph. Prove round-trip on a fixture. | — |
| **P2 — `create_change` journey** | Prompt → reviewed OpenSpec change (proposal + delta + tasks), graph-primary, reviewer-gated. **Standalone deliverable = the change.** New journey; does NOT touch dev-via-test/Lisa. | P1 |
| **P3 — Env-readiness extension** | Brownfield: topology detection → "repo test suite runs green in the sandbox" readiness check on top of the shipped ADR-043 attestation. Service-heavy: activate ADR-054 harness profiles + readiness records + ADR-055 deterministic formal-claims checks. | — (parallel to P1/P2; ADR-043 foundation already shipped) |
| **P4 — `dev-from-task` execution journey** | Add a spec-first executor that reuses the Ralph loop + CBG + coordinator-walk, entered with change-produced `plan.task.*` triples (no Lisa). dev-via-test untouched. Gated on P3 readiness. → **UC-1 end-to-end (brownfield + existing OpenSpec).** | P2, P3 |
| **P5 — Automation + PR** | Issue-queue poll source + PR creation + autonomous policy. → **UC-2.** | P4 |
| **P6 — Greenfield scaffolding** | Topology *creation* (stack scaffold + profile authoring). → **UC-3.** | P4 |

P1+P2 deliver the standalone "prompt → reviewed spec" value with zero execution
risk. P3 is independently useful (it makes *any* hard scenario honest). UC-1
(brownfield) is the first end-to-end target; UC-2/UC-3 layer on.

## Open decisions

1. ~~**Lisa migration.**~~ **DECIDED (2026-06-21):** do not move Lisa / do not
   refactor dev-via-test. Add `dev-from-task` as a pure-addition spec-first
   executor reusing the Ralph+CBG primitive. [§dev-via-test stays intact]
2. **OpenSpec compat depth:** change-folder read/write only (v1), or the full
   living-spec lifecycle with archive/merge (the round-trip)? Recommend staging:
   change-folder first, living-spec second.
3. **Brownfield profile derivation:** how does topology detection pick/derive a
   harness profile beyond the fixed 3-profile catalog? Reuse ADR-054's profile
   schema; the *detector* is net-new.
4. **Repo/spec acquisition seam:** wake-up coordinator + bash, inline paste, or a
   net-new `acquire_repo`/`read_spec` tool? (Framework-alignment review.)
5. **Issue source + PR creation:** rule trigger vs subscriber vs tool; which gh
   surface; how the autonomous loop bounds cost (consecutive-failure breaker —
   cf. semteams#193). Largest net-new surface.
6. **Predicate namespaces:** `spec.*`, `change.*`, `harness.*`, `readiness.*`,
   `formal_claims.*` — and which become owned/`replace_owned` current-state vs
   append-evidence (ties to the governed-SKG model).
7. **Relationship to ADR-054/055:** does this proposal subsume them, or do they
   stay as the env-layer ADRs with this as the integrating umbrella? (Recommend:
   they stay; this is the umbrella that adds the OpenSpec spec layer + routing and
   sequences the work.)

## How this becomes ADRs (decomposition)

This proposal is the umbrella; committed decisions crystallize as **separate
ADRs, written just-in-time** — when a slice is committed to building, not all
upfront. The repo's ADR norm is granular (one pack/decision per ADR), and writing
the whole chain as one speculative ADR is the ADR-035 trap (it specified the
dev-via-spec arc upfront and was superseded wholesale by ADR-041/042). So:
decompose by *decision*, write each ADR at the start of its phase, and **reuse /
amend existing ADRs** wherever the decision already exists.

| Decision | ADR | When | Reuses / amends |
|---|---|---|---|
| OpenSpec spec model on the graph (predicate namespaces, render/ingest round-trip, EARS, compat depth) + the `create_change` journey | **New** | P1/P2 | — (foundational) |
| Spec-driven execution: `dev-from-task` (reuse Ralph+CBG, no Lisa) + coordinator method-routing + new `decide()` tokens | **New**, or an **addendum to ADR-044** (it is "dev-via-test minus Lisa") | P4 | ADR-044, ADR-042 |
| Proof-environment readiness + brownfield topology detection | **Amend ADR-054/055** (Proposed → Accepted) + a brownfield-detection addendum | P3 | ADR-054, ADR-055, ADR-043 |
| Issue-queue → PR automation (autonomous loop, GitHub integration) | **New** | P5 (UC-2) | ADR-053 (autonomous policy) |
| North-star surfaces (notifications, scheduled triggers, multi-repo, Pages) | **New, one each** | later, when scoped | ADR-027 (ops), ADR-053 (HITL) |

Near-term this is **~2 new ADRs** (spec model; spec-driven execution/routing) plus
**accepting/amending ADR-054/055** — not five. Two judgment calls deferred to
authoring time: (a) **routing** may fold into the two journey ADRs (each adds its
`decide()` token) or earn its own ADR if the method-selection logic + Goodhart
rationale prove substantial; (b) **`dev-from-task`** may be cleaner as an
**ADR-044 addendum** than a new ADR.

## Relationship to existing work

- **ADR-054/055** own the env-readiness layer; this proposal activates and
  extends them (brownfield detection) and connects them to the spec layer.
- **ADR-044 dev-via-test** is refactored, not replaced: its ralph loop + CBG gate
  become the execution layer; Lisa relocates to the spec journey.
- **ADR-042 substrate-plus-overlays** is the delivery mechanism: every new
  journey is a rule pack + persona bundle + `decide()` token, no new components.
- **ADR-043 devcontainer sandbox** + `sandboxmanager` are the env substrate; the
  fixed profile catalog is the thing brownfield support must generalize.
- **Governed-SKG (beta.113)** gives the ownership model for living-spec
  current-state evolving via archived deltas.
