# ADR-056: OpenSpec-Compatible, Environment-Gated Spec-Driven Development (Umbrella)

## Status

**Accepted (2026-06-21)** as the **umbrella** for the spec-driven
development initiative. This ADR records the *settled, cross-cutting
decisions* and sequences the work; the **per-phase detail is written
just-in-time in its own ADR** (§How this decomposes). It graduates the
settled decisions out of the working proposal
([`docs/proposals/openspec-spec-driven-dev.md`](../proposals/openspec-spec-driven-dev.md)),
which stays as the open-decision scratchpad.

This ADR does **not** supersede ADR-042/043/044/054/055. It is the
integrating umbrella above them: ADR-042 is the delivery mechanism,
ADR-043 is the env substrate, ADR-044 is the execution primitive,
ADR-054/055 are the env-readiness layer. The first buildable slice is
specified in **[ADR-057](057-openspec-graph-spec-model-and-create-change.md)**
(graph spec model + `create_change`, **Proposed**).

> **Anti-ADR-035 discipline.** ADR-035 specified a whole multi-role
> dev-via-spec arc upfront and was superseded wholesale by ADR-041/042.
> This umbrella deliberately records only what is *settled* plus a
> sequenced roadmap; it does **not** pre-specify the machinery of
> phases not yet committed to build. Each phase ADR is authored when its
> slice starts.

## Context

Three observations drive the initiative:

1. **Creating a spec from a prompt has standalone value.** It is a
   reusable, reviewable, hand-off-able deliverable independent of whether
   (or how) it is later implemented. *Spec creation* and *dev from spec*
   are therefore **two journeys**, not one chain.
2. **The coordinator should pick the implementation method** that fits a
   given prompt/spec — `autoresearch` (optimize a metric), the `ralph`
   test-convergence loop (build to passing tests), or a heavier path —
   rather than forcing one shape.
3. **The environment is the hard part.** The host SemTeams runs on will
   not have the target repo's toolchain or services. Before any spec or
   dev work can *honestly* run, the coordinator must ensure the proof
   environment exists and agents must be sandboxed.

The honest read of semspec's burn (thousands of dollars wrapping
BMAD/OpenSpec for determinism) is that **the ceremony itself is the
cost** — see [ADR-041](041-mvp-role-compression-and-graph-as-substrate.md).
This initiative keeps agent count and artifact size down and reuses
shipped primitives rather than rebuilding the retired heavy chain.

## Decision

### D1. Adopt OpenSpec as the interchange standard; graph stays canonical

We adopt **OpenSpec** (read + write) as the spec interchange format. It
is the lean end of the SDD spectrum and its data model maps almost 1:1
onto a graph-primary harness.

| | OpenSpec | Spec-Kit | BMAD |
|---|---|---|---|
| Artifact weight | ~250 lines, **delta-based** | ~800 lines, full per-feature | PRD blowup |
| Brownfield | native (changes are diffs) | greenfield-leaning | greenfield/regulated |
| Cost | light | medium | heavy ($800–2k/mo) |
| Fit to graph-primary | **high** | medium | low |

**Graph is the canonical truth; OpenSpec markdown is a rendered
projection (hydration) and an interchange format — never the source of
authority.** This is the same stance ADR-054/055 already take. The
governed-SKG ownership model (beta.113 `replace_owned` + owner-lease) is
the substrate for *"a living spec that evolves via archived deltas."*
Requirements use OpenSpec's native form — an RFC-2119 `SHALL` statement
plus `Given/When/Then` scenarios (**not** EARS; verified 2026-06-22,
ADR-057 §D3) — machine-checkable and 1:1 mappable to a dev-via-test step.

### D2. Two journeys, not one chain

- **`create_change` / `create_spec`** (net-new) — prompt ⇄ OpenSpec
  change/living-spec, graph-primary, reviewer-gated. The standalone
  deliverable is *the spec*.
- **`dev-from-task`** (net-new) — execute a spec's tasks. Reuses the
  shipped execution primitive (D5).

Spec creation never *requires* execution; execution never *requires*
re-deriving the spec. They compose but do not couple.

### D3. The coordinator selects the implementation method

Routing extends the existing **closed `decide()` taxonomy** (ADR-042) —
new tokens + spawn rules + persona bundles, **no new components**. The
coordinator (a) gates on environment readiness, then (b) selects the
method (`autoresearch` / `dev_via_test` / `dev_from_task` / …). The
router stays *thin* — it routes, it does not plan; classification
quality is validated on real-LLM smoke, never Goodharted via persona
patches ([[coordinator-first-not-persona-patches]]).

### D4. Environment-readiness gates execution

You cannot honestly run tests (the ralph loop's whole premise) until the
test harness is real. The ordering is **env-readiness → execution**,
fail-closed.

- **Foundation is shipped.** ADR-043's devcontainer sandbox + attestation
  (`sandboxmanager`, `sandbox.attestation.*`, `request_sandbox` /
  `query_sandbox_attestation`) provisions a container and proves it is up
  — already used by autoresearch and dev-via-test.
- **Extensions are the ADR-054/055 work** (harness profiles, readiness
  records, proof dependencies, formal-claim gating) — only the ADR docs
  exist; needed for **service-heavy** targets (PX4 SITL, OSH). For a
  typical brownfield Go/Node repo, ADR-043 attestation + *"the repo's own
  test suite runs green in the sandbox"* may be a sufficient v1 readiness
  check. ADR-054/055 stay **Proposed** until P3 commits to build; their
  §addenda fold them under this umbrella.

### D5. dev-via-test (Lisa) is untouched; add dev-from-task as a pure addition

**Lisa does not move. dev-via-test is not refactored.** The **ralph loop
+ CBG work-gate + coordinator plan-walking are the shared execution
primitive** ([ADR-044](044-dev-via-test-pack.md)). dev-via-test wraps it
with Lisa (prompt → tasks). `dev-from-task` wraps the *same* primitive
fed by a spec's tasks instead — equivalent to dev-via-test *minus Lisa*,
entered with `plan.task.*` triples already populated. The mapping from a
change's tasks onto that dispatch shape is **the integration seam**
(specified in ADR-057; detailed at P4). Pure addition via
substrate-plus-overlays; the working dev-via-test happy path stays green.

### D6. The three layers of this initiative (spec / routing / env-readiness)

1. **Spec layer (net-new)** — prompt ⇄ OpenSpec change/living-spec,
   graph-primary. ADR-057.
2. **Routing layer (extends the coordinator)** — gate on env-readiness,
   then select the method. ADR-042 taxonomy extension; no new components.
3. **Env-readiness layer (ADR-043 shipped; ADR-054/055 extensions)** —
   attestation today; harness profiles + readiness records + formal-claim
   gating for service-heavy targets.

### D7. Decompose by decision; write each ADR just-in-time

The repo's ADR norm is granular (one pack/decision per ADR). Writing the
whole chain as one speculative ADR is the ADR-035 trap. So: decompose by
*decision*, write each ADR at the start of its phase, and reuse/amend
existing ADRs wherever the decision already exists (§How this decomposes).

## Non-goals (settled — do not relitigate)

- **We do not rebuild the heavy dev-via-spec chain** (ADR-035's
  planner→reviewer→challenger→architect→builder→qa). Retired with
  evidence in ADR-041/042: *the ceremony is the cost*. Ceremony × agents
  × re-read-large-artifacts is the trap.
- **We do not become a DevOps/orchestration platform.** The env layer
  renders boring runners (Compose / `act`), not mini-k8s (ADR-054 §D4).
- **We do not make OpenSpec (or any markdown) the operating truth.** It
  hydrates from, and ingests into, the graph.

## Phasing (each phase independently valuable)

| Phase | Deliverable | Depends on | ADR |
|---|---|---|---|
| **P1 — Spec artifact + render/ingest** | Graph spec model (`spec.*`/`change.*`) + OpenSpec render-from-graph + parse-into-graph; round-trip on a fixture. | — | [057](057-openspec-graph-spec-model-and-create-change.md) |
| **P2 — `create_change` journey** | Prompt → reviewed OpenSpec change (proposal + delta + tasks), graph-primary, reviewer-gated. Standalone deliverable = the change. Does NOT touch dev-via-test/Lisa. | P1 | [057](057-openspec-graph-spec-model-and-create-change.md) |
| **P3 — Env-readiness extension** | Brownfield: topology detection → "repo test suite runs green in the sandbox" on top of shipped ADR-043 attestation. Service-heavy: activate ADR-054 profiles + readiness records + ADR-055 formal-claims. | — (parallel to P1/P2) | amend [054](054-test-harness-team-proof-environments-before-code.md)/[055](055-formal-claim-analysis-for-verification-gates.md) |
| **P4 — `dev-from-task` execution** | Spec-first executor reusing the ralph loop + CBG + coordinator-walk, entered with change-produced `plan.task.*` (no Lisa). dev-via-test untouched. → **UC-1 end-to-end.** | P2, P3 | new (or [044](044-dev-via-test-pack.md) addendum) |
| **P5 — Automation + PR** | Issue-queue poll trigger + PR creation + autonomous policy. → **UC-2.** | P4 | new |
| **P6 — Greenfield scaffolding** | Topology *creation* (stack scaffold + profile authoring). → **UC-3.** | P4 | new |

P1+P2 deliver the standalone "prompt → reviewed spec" value with zero
execution risk. P3 makes *any* hard scenario honest. UC-1 (brownfield) is
the first end-to-end target; UC-2/UC-3 layer on.

## North-star deployment roadmap

The destination is a **self-hosted, always-on, single-operator
program-manager**: SemTeams on a Mac Mini on the operator's LAN, running
continuously, coordinating the **`sem*` repo family** (semstreams,
semspec, semteams, semdragons) as **local working copies** (brownfield =
a local checkout). The coordinator grows from "answer a prompt" into a
**standing cross-team program manager**.

This roadmap sequences that growth into deployment milestones (**D-**),
each gated on the phases above and each graduating to its own ADR when
committed. **The initial deployment surface is D2 + D3:** D2 makes it
*run* (autonomous issue→PR on one repo); D3 makes it *safe to leave
running* (the operator channel for events the autonomous policy cannot
resolve). D2 alone is demo-grade.

| Deployment milestone | What the operator gets | Gated on | New surface(s) | Future ADR |
|---|---|---|---|---|
| **D0 — Substrate already-on** (today) | Long-running backend; coordinator answers prompts; research / autoresearch / dev-via-test packs; single-tenant, UI-driven, event-triggered. | shipped | — | — |
| **D1 — Spec + dev on a local brownfield checkout** (UC-1) | Point SemTeams at a local `sem*` working copy: `create_change` → `dev-from-task` → PR-or-archive. Operator-initiated. | P1–P4 | OpenSpec parser; brownfield topology detection | 057; P3 amend; P4 |
| **D2 — Initial deployment surface: autonomous issue→PR, one repo** (UC-2) | Poll one repo's issue queue (label-filtered); per issue run the D1 arc **autonomously** (`restricted_decide_actions:["ask_user"]`); open a PR. The first genuinely *standing PM* increment — **demo-grade until D3** lands the operator channel (an unattended autonomous run with `ask_user` barred has nowhere to surface a clarification/approval/blocked-env event). | D1 | **poll/scheduled trigger**; **autonomous cost/failure breaker**; **wire upstream `github_*` tools** (see note) | P5 |
| **D3 — Operator-in-the-loop over real channels** | `ask_user` / approval / blocked-env reach the operator via **text and/or email with a UI deep-link**. Makes D2 safe to leave running unattended. | D2 (required before D2 is leave-it-running-safe) | **notification channel** (SMS/email + deep-links) extending ADR-053 HITL | new (extends [053](053-adoption-plan.md)) |
| **D4 — Timed daily roll-ups** | Scheduled summary of what ran / shipped / waiting / blocked, pushed on a cadence. | D2 + D3 | **time-based trigger** (generalizes D2's poll); roll-up generator extending the ops-agent | new (extends upstream ADR-027 ops-agent, semstreams) |
| **D5 — Multi-repo program management** | Coordinator spans the `sem*` family: polls all queues AND **creates issues** across repos to coordinate cross-team dependencies. | D2 | **multi-repo awareness**; **issue creation** (absent from upstream `github_write`) | new (×2) |
| **D6 — Lessons-learned publishing** | Coordinator maintains a **GitHub Pages** site, blogging lessons learned (echoes semspec's lesson-decomposer + the ops-agent's diagnostic role). | D2+ (run history) | **publishing path** (GitHub Pages) | new |

**Critical path to the initial deployment surface:** D0 → D1 (P1–P4) →
**D2** + **D3**. D4/D5/D6 are later, separately-scoped additions; P6
(greenfield) is orthogonal completeness, not on this path.

> **GitHub-integration note (verified against beta.113, 2026-06-21).**
> The proposal assumed "no GitHub issue/PR integration exists today." That
> is now stale. Upstream **ships two tools** — `github_read` (dispatching
> to `get_issue` / `list_issues` / `search_issues` / `get_pr` / `get_file`)
> and `github_write` (`create_branch` / `commit_file` / `create_pr` /
> `add_comment` / `add_label`) — registered only when `GITHUB_TOKEN` is
> present, and **not yet wired into the semteams product shell**. So D2's
> GitHub surface is mostly *wiring* (ADR-029 pattern), not new integration.
> **Issue *creation* is not in the upstream write set** — that remains
> net-new for D5. The genuinely net-new D2 work is
> the **poll/scheduled trigger** and the **autonomous cost/failure
> breaker** (cf. semteams#193), not GitHub access.

The autonomous policy (`restricted_decide_actions`, ADR-053) already
exists for the no-human stretches between notifications. Several surfaces
map onto existing seams — operator notifications extend ADR-053 HITL;
roll-ups + lessons-learned extend the upstream ops-agent (semstreams
ADR-027). This umbrella
records the sequence so the issue→PR work does **not foreclose** them; it
does not design them.

## Target use cases

| | Spec layer | Env-readiness layer | Routing/automation |
|---|---|---|---|
| **UC-1 brownfield** | ingest existing + create change (delta) | **detect** topology, match profile, readiness record | front-door prompt → method |
| **UC-2 issue-queue** | create change per issue | detect (as UC-1) | **poll trigger + PR + autonomous** |
| **UC-3 greenfield** | create full living spec | **create** topology (scaffold profile) | front-door prompt → method |

UC-1/UC-2 (brownfield, the OpenSpec sweet spot) are the higher-value,
better-fit targets. UC-3 (greenfield) is the least differentiated and
most env-creation-heavy — the completeness case. Full UC walkthroughs
live in the proposal.

## How this decomposes into ADRs

| Decision | ADR | When | Reuses / amends |
|---|---|---|---|
| OpenSpec spec model on the graph + `create_change` journey | **[057](057-openspec-graph-spec-model-and-create-change.md)** | P1/P2 | — (foundational) |
| Spec-driven execution: `dev-from-task` + coordinator method-routing + new `decide()` tokens | **new**, or an **addendum to ADR-044** | P4 | ADR-044, ADR-042 |
| Proof-environment readiness + brownfield topology detection | **amend ADR-054/055** (Proposed → Accepted) + brownfield-detection addendum | P3 | ADR-054/055/043 |
| Issue-queue → PR automation (poll trigger, autonomous loop, GitHub wiring + breaker) | **new** | P5 (UC-2) | ADR-053, ADR-029 |
| North-star surfaces (notifications, scheduled triggers, multi-repo, issue creation, Pages) | **new, one each** | later, when scoped | upstream ADR-027 (ops), ADR-053 |

Two judgment calls are deferred to authoring time: (a) **routing** may
fold into the two journey ADRs (each adds its `decide()` token) or earn
its own ADR if the method-selection logic proves substantial; (b)
**`dev-from-task`** may be cleaner as an **ADR-044 addendum** than a new
ADR (it is "dev-via-test minus Lisa") — its reuse contract + the
task→`plan.task.*` seam are **already** recorded in ADR-044's 2026-06-21
§addendum; the deferred call is only whether the full P4 design ADR is
standalone or extends that addendum.

## Open Questions

These stay genuinely open (carried from the proposal; resolved at the
phase that needs them):

1. **OpenSpec compat depth** — change-folder read/write only (v1), or the
   full living-spec lifecycle with archive/merge (the round-trip)?
   Recommendation: stage — change-folder first, living-spec second
   (ADR-057 §D4).
2. **Brownfield profile derivation** — how does topology detection
   pick/derive a harness profile beyond the fixed 3-profile catalog?
   Reuse ADR-054's profile schema; the *detector* is net-new (P3).
3. **Repo/spec acquisition seam** — the front-door coordinator has no
   `bash`; spec/repo input arrives via a wake-up coordinator (which has
   bash), an inline paste, or a minimal `read_spec`/`acquire_repo` tool
   (framework-alignment review).
4. **Predicate ownership** — which `spec.*` / `change.*` / `harness.*` /
   `readiness.*` / `formal_claims.*` predicates are owned `replace_owned`
   current-state vs append-evidence (ADR-057 §D1 settles the spec half).
5. **Autonomous-loop cost bound** — the D2 consecutive-failure breaker
   shape (cf. semteams#193); resolved at P5.

## Related

- [ADR-042](042-coordinator-instantiated-flows-via-templates.md) —
  substrate-plus-overlays (delivery mechanism).
- [ADR-043](043-devcontainer-as-sandbox-spec.md) — devcontainer sandbox +
  attestation (env substrate).
- [ADR-044](044-dev-via-test-pack.md) — dev-via-test pack (execution
  primitive reused by `dev-from-task`); see §addendum 2026-06-21.
- [ADR-053](053-adoption-plan.md) — autonomous policy + HITL surface
  (extended by D3 notifications).
- [ADR-054](054-test-harness-team-proof-environments-before-code.md) /
  [ADR-055](055-formal-claim-analysis-for-verification-gates.md) —
  env-readiness layer (folded under this umbrella; see their §addenda).
- [ADR-057](057-openspec-graph-spec-model-and-create-change.md) — the
  first buildable slice (P1/P2).
- Working proposal:
  [`docs/proposals/openspec-spec-driven-dev.md`](../proposals/openspec-spec-driven-dev.md).
- Governed-SKG (beta.113 `replace_owned` + owner-lease) — ownership model
  for living-spec current-state evolving via archived deltas.
