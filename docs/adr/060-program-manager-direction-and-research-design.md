# ADR-060: Program-manager direction and the research design for non-verifiable domains

**Status:** Accepted (2026-09-03). Records decisions the owner has already ruled; the ruling comments cited
below are the authority, and this record is the durable explanation of why. Supersedes
[ADR-056](056-openspec-spec-driven-development-umbrella.md) §"North-star deployment roadmap" and its
"Addendum 2026-07-30", that section only. The rest of ADR-056 is unchanged: its OpenSpec decisions stand and
its packs remain parked per [ADR-058](058-beta159-realignment-and-demo-lane-focus.md).

## Context

ADR-056 (2026-06-21) named the destination: a self-hosted, always-on, single-operator program manager
coordinating the `sem*` repository family. It sequenced that growth as deployment milestones D0–D6, with
autonomous issue→PR on one repository (D1/D2) as the initial surface, an operator channel (D3), roll-ups (D4),
multi-repository coordination and issue creation (D5), and lessons publishing (D6). Its 2026-07-30 addendum
locked idempotency and credential invariants for the D2/P5 external-effect contract.

Four things changed underneath that roadmap. Each is verified in
[`docs/proposals/personal-assistant-deployment.md`](../proposals/personal-assistant-deployment.md) (corrections
C1–C6, August 2026) or in the owner rulings of 2026-09-03:

- **The issue→PR arc left SemTeams.** SemDev took "GitHub issue in, reviewed, clean-room-verified pull request
  out" with its own human gates (C4). ADR-058 parked the dev-side packs; C4 reclassifies them as donor material.
  SemTeams never opens a PR.
- **The GitHub surface is product-local.** Upstream removed its `github_*` tools by design (C2) and strips
  `GITHUB_TOKEN` from `bash` (C3), so ADR-056's "mostly wiring" note is stale. Cron rules ship upstream (C1), so
  the scheduled trigger is configuration, not new surface.
- **The neighbours are separate services.** SemSource does not share our graph (C5). SemMem was rechartered as
  the cross-product knowledge curator that owns the org standards (SOP) repository's content policy
  (C360Studio/semmem#3); upstream ADR-080's line that semmem was retired predates that recharter. Upstream ruled
  the lesson layers as constitution / standards (= SOPs) / lessons (semstreams#1167) and made lessons push-based,
  evidence-cited graph entities (semstreams ADR-080).
- **The identity question was settled on #264 (2026-09-03).** SemTeams is an always-on program manager whose
  core competence is evidence-verified research. Every direction on the table (program pulse, docs drift, trend
  radar, pr-review, deep research) reduces to the research arc plus a domain plus a terminal action. What remained
  was sequencing. The same ruling recorded the research-pack gaps on `main` 8a70b7e7 / semstreams beta.160: the
  artifact schema carries no source field, a gatherer's sources live in a loop-private scratchpad, the reviewer
  receives only the synthesize loop and rendered markdown and cannot falsify, research spawns inherit the
  50-iteration component ceiling, and the retry cap is per-entity and informational.

A second ruling the same day (the research-design addendum on #264) recorded the design for research over
domains with no ground truth, so that the GitHub-first MVP, whose domain is verifiable, does not erase the risk
profile of the open-web profile that follows it.

The product destination is written in [`docs/product/program-manager.md`](../product/program-manager.md)
(owner-approved 2026-08-26, amended 2026-09-03) and the delivery sequence in [`docs/ROADMAP.md`](../ROADMAP.md).
This ADR records the *why* behind six decisions those documents state. It owns no status, sequencing, or
checklist; epic #269 and its children own those. No OpenSpec change accompanies it: nothing here changes shipped
behavior.

## Decision

Each decision cites the ruling it records. A draft that finds a contradiction comments on #278 or #264; it does
not resolve it here.

### 1. The authority ladder is re-based on observation, not on autonomous delivery

ADR-056's D0–D6 is replaced by: **observe** (the program pulse, on demand and then scheduled; #267, #268) →
**operator channel** (ADR-056's D3: `ask_user`, approval, and blocked-run events reach the operator over a real
channel, extending the human-in-the-loop seams adopted in [ADR-053](053-adoption-plan.md)) → **propose and act by
filing issues** (ADR-056's D5, authority stage 5 of the product doc's ladder) → **lessons** (ADR-056's D6,
re-shaped by decision 3). ADR-056's D1/D2 dev arc moved to SemDev (ADR-058; proposal C4). ADR-056's D4 roll-up is
the scheduled pulse of #268, not a separate surface. Multi-repository awareness, the first half of D5, is present
from the first pulse because the portfolio configuration spans projects and repositories.

pr-review is deferred by this ladder, not superseded: it is the falsifier of decision 6 pointed at a diff, and it
arrives with issue filing under explicit approval policy. The MVP stops at stage 3 (observe, recommend, explain).
Ruling: #264 (2026-09-03) §Consequences 5–6; the product doc's §Authority ladder (owner-approved 2026-08-26);
proposal C4.

### 2. The containment budget precedes the first unattended tick

Before any cron-fired research loop runs unattended: every research-pack `publish_agent` carries an explicit
`loop_max_iterations`; the planner's fan-out is bounded at the schema (`maxItems` on the subtopic list);
scheduled spawns carry a cron `cooldown` backed by the scheduler's inflight guard so a wedged chain cannot stack
fires; and the kill switch (`enabled: false` on the cron rule through the Pattern-B rule manager) is verified to
stop the heartbeat without a restart. The risk this bounds is not steady-state cost but the unattended pathology:
a re-firing wedged chain, or a planner decomposing into fifteen subtopics instead of four, with nobody watching
(proposal §Phasing, Phase 0).

No persona-level call-count rules. Ceilings live on the spawn and at the schema; the framework's
`[Iteration Budget]` signal applies the pressure. Counters written in prose get gamed. Ruling: owner steer
2026-08-20, restated on #272; #264 §Consequences 3.

### 3. Inter-product edges are GitHub artifacts or governed SemStreams contracts

Every edge between SemTeams and another product is one of two things: a visible artifact on GitHub (an issue, a
pull request, a release) or a governed SemStreams contract that carries provenance and is idempotent. Never a
shared graph (C5), never an ad-hoc private API. Managers and curators file issues; SemDev instances are the only
PR authors in the ecosystem, including on the SOP repository (semmem#3).

Lessons flow **in** to SemMem by push over SemStreams federation: SemTeams selects `active`, export-tagged
lessons and sends them as the framework's registered lesson payload; SemMem owns the ingest contract and never
pulls (#276). No new framework primitive is needed for the push. Institutional knowledge flows **out** to
products only as versioned SOP releases (#275; decision 4). SemTeams is a lesson source as well as an observer of
the SOP process. Ruling: #264 §Consequences 5; owner discussion of 2026-09-03 recorded on #275, #276, semmem#3,
and the semstreams#1167 consumer note.

### 4. SOP releases are the institutional-practice trigger

The trigger for propagating institutional practice is a **new SOP release**: a fact with a version, a diff, a
date, and a reviewer. An individual lesson is a stochastic artifact that still needs corroboration and is never
the trigger. MCP is the runtime retrieval path for active practices and their evidence trail; it is never the
trigger either.

SOP-version drift is the general form of docs drift: a project whose declaration pins an org standards bundle
behind the current release, with at least one applicable change since the pin. The per-project declaration is
SemDev's `.semdev/standards.yaml`; giving it a place to name the org standards source and version is a SemDev
schema change the owner coordinates, noted on semstreams#1167 so the upstream lesson ADR leaves room for it.
SemTeams invents no manifest of its own. Docs drift is a later recommendation class, not a product, and is not
exemplified via SemSource: the docs site is one more portfolio project with its own SemDev instance.

Vocabulary follows semstreams#1167: **constitution / standards (= SOPs) / lessons**. Products say "standards
(SOPs)" for the middle layer and never call it a constitution. Ruling: #264 §Consequences 5; #275; semmem#3;
semstreams#1167 consumer note (2026-09-03).

### 5. `program-report` is the first depth profile of the research capability

`program-report` is built **as** a profile of the research pack, not beside it: the planner decomposes the
operator's portfolio configuration (one gatherer per project), the `for_each` fan-out and join rules run
unchanged, synthesize emits a pulse artifact whose every factual claim carries an evidence record, and review is
derivation plus plan conformance. The MVP sentence "targeted research is not a prerequisite for the first
end-to-end proof" is not license to build a fetch-and-summarize pipeline with its own evidence shape.

The evidence contract (#271) is the shared foundation: a channel-typed evidence record (channel, locator,
retrieved-at, quote, tool-result reference, plus the origin identity, source class, and replica index of
decision 6) checked mechanically at emit against the run's own tool results. GitHub is the first domain because
it is the only candidate domain with ground truth: PR state, CI status, and commit timestamps can be checked
against the API rather than against "the model fetched a page." The open web and SemSource are later channels of
the same engine.

General and deep research are depth or budget profiles of one capability. A depth profile is a config bundle
(replica count, falsifier on or off, per-spawn cap, budget), not a coordinator token: the earlier `for_each(N=1)`
ruling put N at the planner, and no deep-versus-vanilla classifier exists. The GitHub read adapter (#273) is a
product-local tool, because upstream removed its `github_*` tools by design and strips `GITHUB_TOKEN` from
`bash`, and it exists only after the framework-alignment review. Ruling: #264 §Consequences 1, 2, 4; PR #265.

### 6. Research over non-verifiable domains: ensemble synthesis over bounded walkers

Open-web research has no compiler or API to check against. Two failure classes are bounded structurally, and
they need different countermeasures:

- **Geometric error decay.** A serial chain of n unverified steps fails with probability 1−(1−p)^n. Countered by
  width: the planner fans out independent gatherers and the synthesizer cross-examines their evidence records
  instead of generating facts.
- **Context and attention drift.** A long loop loses its sub-task or conditions later steps on its own earlier
  summaries. Countered by horizons: every walker is a fresh loop scoped to one sub-task under a hard per-spawn
  cap, terminating by serializing a typed evidence record that every downstream role reads (#271, #272). The
  residual drift risk sits in the synthesizer, the one role that reads everything; it reads compact typed records,
  not narratives, and carries its own cap.

**Convergence is scored over distinct origins, not over branches.** Branches sharing a model, a prompt, and a
search index are correlated; their agreement is one sample repeated, and a hallucination from the model's prior
appears in every replica. Two branches citing one canonical locator are one vote. Diversity is engineered in
order of leverage: query framing and source class at the planner; a second retrieval path (upstream `web_search`
is Brave-only at beta.160; provider abstraction is semstreams#1258); then a gatherer model pool, priced per
replica. Model-family diversity does not fix retrieval correlation, so "three model families" is not the answer
to a single search index. The structural minimum is two families across the generator/critic boundary.

**Provenance is mandatory and checked mechanically at emit.** A claim whose locator appears in no tool result of
the run's own trajectory, or whose quote does not appear in the fetched body, is rejected before any reviewer
sees it. No LLM takes part in the check.

**Critique is a decoupled falsifier, not the conformance reviewer.** A separate role, spawned with the artifact's
evidence records and the means to re-fetch, whose only terminal is `decide(action=survives | falsified)`, and
whose model family differs from the generator's. A falsified result routes to a targeted re-gather of those
claims, not a full replan. `reviewer-research` stays the plan-conformance grader. Two jobs, two roles (#274).

**The coordinator never branches mid-run.** Rules read only the firing entity and the coordinator cannot see the
graph mid-run; putting it at every branch point would recreate the serial chain. The coordinator classifies and
recovers; the planner owns width; the rules own the join.

**The GitHub program pulse is the verifiable degenerate case.** The API is ground truth, so the falsifier and
redundant width are off and review reduces to derivation plus plan conformance. Nothing in the MVP will exercise
this decision, which is why it is recorded here rather than left to be rediscovered. The shared evidence contract
still carries origin identity, source class, and replica index so the non-verifiable profile needs no second
schema. Ruling: #264 research-design addendum (2026-09-03); #274; #271.

## Consequences

- **ADR-056 §North-star and its 2026-07-30 addendum are superseded**, that section only. ADR-056's OpenSpec
  decisions D1–D7 stand; its packs remain parked donor material per ADR-058 and are not to be unparked as a
  shortcut to program-manager action.
- **The D2/P5 invariants travel with the arc.** Idempotent external-effect identity, the request hash, the outcome
  set, server-side nonretrievable credentials, and "no parallel lifecycle, KV bucket, or durable object" were
  written for issue→PR; that arc is SemDev's, so applying them there is SemDev's decision. The same invariants
  re-apply to SemTeams's own external writes when authority stage 5 (issue filing) is designed, and the scheduled
  pulse (#268) inherits the durable-dedupe discipline for its cursors. Neither is designed here.
- **`docs/product/program-manager.md` is amended in the PR that lands this ADR** to mark its "review is a
  mechanical derivation check" sentence as the verifiable-domain shortcut of decision 6.
- **The research pack evolves in place.** No `deepresearch` fork, no new coordinator token, no second evidence
  schema. Category packs and persona bundles remain the extension mechanism (ADR-042), and the shared persona
  corpus stays domain-neutral.
- **Work stays in issues.** Epic #269 owns the order; #271–#274, #275, and #276 own their target states;
  semstreams#1258 and the upstream lesson ADR called for by semstreams#1167 are the two upstream asks. Nothing
  here is a checklist.
- **Two contracts must not close off the SOP pin.** SemDev's `.semdev/standards.yaml` needs a place for the org
  standards source and version, and the upstream lesson ADR should leave room for it. Both are coordinated by the
  owner.

## Alternatives considered

- **Keep ADR-056's ladder with autonomous issue→PR as the initial surface.** Rejected: SemDev owns that arc (C4);
  SemTeams never opens a PR, and a maker workflow inside the program manager would duplicate SemDev's gates.
- **A `deepresearch` pack or a deep-versus-vanilla coordinator token.** Rejected by the `for_each(N=1)` ruling and
  PR #265: depth is a profile of one capability, set at the planner and in config.
- **Build the pulse as a fetch-and-summarize pipeline with its own evidence shape.** Rejected on #264: it would
  ship a second evidence schema and leave the research pack's chain-of-custody gaps in place.
- **A shared graph with SemSource, SemDev, or SemMem.** Rejected: SemSource is a separate service (C5), and a
  shared graph is an edge without provenance or idempotency.
- **Per-lesson federation back to products, or MCP as the trigger.** Rejected on semmem#3 and #275: a lesson is a
  stochastic artifact; a release is a fact with a version and a reviewer.
- **Docs drift as a product, exemplified via SemSource.** Rejected (owner, 2026-09-03): it is one recommendation
  class of SOP-version drift, and the docs site is an ordinary portfolio project.
- **Persona-level call-count rules for containment.** Rejected (owner, 2026-08-20): counters in prose get gamed;
  ceilings live on the spawn and at the schema.
- **Three model families in the gather fan-out to defeat Brave-only correlation.** Rejected in the research-design
  addendum: the correlation lives in retrieval; the levers are origin-counted convergence, framing and
  source-class diversity, a second retrieval path, and a different-family critic.
- **Coordinator-controlled branching mid-run.** Rejected: rules read only the firing entity and the coordinator
  has no mid-run graph view; it would recreate the serial chain the design exists to avoid.

## Related

- Rulings: semteams#264 (direction, 2026-09-03; research-design addendum, 2026-09-03); #278 (this ADR's
  inventory).
- Product: [`docs/product/program-manager.md`](../product/program-manager.md); [`docs/ROADMAP.md`](../ROADMAP.md);
  [`docs/proposals/personal-assistant-deployment.md`](../proposals/personal-assistant-deployment.md) (research
  notes and corrections C1–C6; not a decision record).
- Work: epic #269; #271 evidence contract; #272 containment budget; #273 GitHub read adapter; #267 and #268
  program pulse; #274 deep depth profile; #275 SOP-version drift; #276 lesson export.
- Ecosystem: C360Studio/semmem#3 (SOP repository ownership); semstreams#1167 and its 2026-09-03 consumer note
  (lesson layers and vocabulary); semstreams#1258 (`web_search` provider abstraction); semstreams ADR-080
  (push-based lessons); semstreams ADR-027 (ops agent).
- Local ADRs: [042](042-coordinator-instantiated-flows-via-templates.md) (substrate plus overlays);
  [043](043-devcontainer-as-sandbox-spec.md) (sandbox attestation); [053](053-adoption-plan.md) (human-in-the-loop
  seams); [056](056-openspec-spec-driven-development-umbrella.md) (superseded in part);
  [058](058-beta159-realignment-and-demo-lane-focus.md) (parked packs; SemDev boundary);
  [059](059-semstreams-beta160-graph-foundation-adoption.md) (framework baseline).
