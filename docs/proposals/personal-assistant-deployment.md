# Proposal: SemTeams as a 24/7 Personal Assistant (D-ladder update)

**Status**: Research notes (2026-08-19). Not a decision. This updates and
partially corrects the **North-star deployment roadmap** in
[ADR-056 §North-star](../adr/056-openspec-spec-driven-development-umbrella.md)
rather than proposing a parallel plan — that ladder (D0–D6) already
describes this deployment. Three of its assumptions are stale at
`v1.0.0-beta.160` and one is invalidated by the semdev split.
**Author**: Coby Leuschke (with Claude)
**Verified against**: semstreams `v1.0.0-beta.160`, semteams `main`,
semdev `main`, semsource `v1.0.0-beta.5`.
**Amended 2026-08-24** — C5/C6, G7–G9, OD-6–OD-9, a tightened Hardware
section, and a re-verification list. Several of the surfaces cited below are
being actively reworked upstream; see
[§Re-verify at the next semstreams tag](#re-verify-at-the-next-semstreams-tag).
The companion research note
[`knowledge-org-para-gtd.md`](knowledge-org-para-gtd.md) carries the evidence
for the new items.

## The deployment

Run SemTeams continuously on a Mac mini beside SemSource. Explicitly **not**
email or personal comms — the operator handles those. Three standing jobs:

1. **Keep the `sem*` docs current** at `c360studio.github.io` (a GitHub Pages
   repo, cloned locally). The site is hand-written HTML today: no SSG, no
   `package.json`.
2. **Build and maintain an interactive course site** — Starlight/Astro,
   client-side quizzes, `localStorage` progress, zero backend. Curriculum
   framing is "AI Fundamentals for Systems Operators": a track for people who
   will *operate* harnesses, sitting between the researcher track (PyTorch and
   calculus) and the consumer track (prompt tips).
3. **Track agentic-AI trends**, weighted toward local LLMs, because that is
   where `sem*` is expected to fit best in 2027.

This is ADR-056's north star with a different first job. That ladder's D6 is
already *"coordinator maintains a GitHub Pages site, blogging lessons
learned"*; D4 is timed roll-ups; D3 is the operator channel. The docs and
course work are D6-shaped, and they do **not** depend on the D1/D2 dev arc.

## Corrections to the D-ladder

### C1. The poll/scheduled trigger is not net-new — cron rules already ship

ADR-056 lists *"poll/scheduled trigger"* as net-new D2 surface. It shipped.

`processor/rule/cron_rule.go` defines `type: "cron"` — POSIX 5-field
schedules plus `@daily`/`@hourly` descriptors, with `cooldown` and
`fire_every_n_events`. `processor/rule/processor.go:574-592` starts the
`CronScheduler` inside `rule-processor`, after watchers and subscriptions are
up, dispatching through **the same `ActionExecutor` expression rules use** —
so `publish_agent` fires from a clock tick.

SemTeams has **zero** cron rules. Our `rule` component is `rule-processor`,
so a heartbeat is a JSON file in `rules_files` and a restart. No new
component, no Go, no ADR-042 violation.

One constraint shapes everything downstream: a cron fire has **no entity in
scope**. `cron_substitution.go` supplies only `$schedule.id`,
`$schedule.spec`, `$schedule.last_fired_at`, and `$now`; `conditions`,
`on_enter`, `entity.pattern`, `related_patterns`, and `max_iterations` are
**rejected at config load**. The heartbeat is deliberately dumb. All
state-awareness must come from the spawned loop reading the graph — which is
why the seen-set (Phase 1 below) gates everything else.

The scheduler ships **four dispatch gates** (`cron_scheduler.go:370-445`):
existence, `FireEveryN`, `cooldown` (wallclock, since the previous fire), and
an **inflight** CAS guard that skips a tick when the previous fire is still
running. The last one matters most here: it is exactly the "a wedged chain
must not stack fires" protection Phase 0 needs, and it is free. Its
`inflight_skipped` metric is deliberately distinct from `cooldown_skipped`
because the operator response differs — the former means actions are slower
than the schedule.

> **Do not carry the semstreams#1007 caveat across to cron rules.**
> `CLAUDE.md` warns that `fire_every_n_events` does not gate `publish_agent`
> and that `cooldown` is per rule *instance* with an `on_exit`-firing
> suppression path. Both findings are about the **expression-rule** path,
> where `on_enter` runs through the stateful evaluator and never reads the
> counter. Cron rules dispatch through `CronScheduler.fire` instead, which
> consults all four gates directly. Per-instance cooldown is also the
> correct semantics for a heartbeat, which has exactly one instance.

### C2. The upstream `github_*` tools were removed by design

ADR-056's GitHub-integration note (verified at beta.113) says D2's GitHub
surface is *"mostly wiring, not new integration"*, citing upstream
`github_read` / `github_write`. **That is stale.**

At beta.113 four files existed:
`processor/agentic-tools/executors/{register_github,github_client,github_read,github_write}.go`.
At beta.160 they are gone, and `BuiltinGroupKeys` — the closed set of
builtin tool groups — has no GitHub entry.

The removal was deliberate. semstreams commit `a533306e`,
`refactor(framework)!: enforce package composition boundaries`:

> BREAKING CHANGE: product-owned adapters, vocabularies, **GitHub
> integrations**, dead facades, and ambient registrations are removed;
> binaries must select graph-research and optional adapters explicitly.

Upstream has classified GitHub integration as **product-owned by intent**.

This matters for the framework-alignment discipline in `CLAUDE.md`. Step 4 —
*"if not in scope upstream by intent, document why the SemTeams case
justifies a product-local primitive"* — is satisfied here by an explicit
upstream decision rather than by our own argument. A product-local GitHub
tool is the sanctioned answer, not accretion. The evidence trail is this
section.

### C3. `bash` + `gh` does not work as-is — `GITHUB_TOKEN` is stripped

The obvious workaround for C2 is `bash` + the `gh` CLI, which would need no
new tool at all. It does not work unmodified.

`processor/agentic-tools/executors/bash.go:591-601` strips secrets from the
child environment. `GITHUB_TOKEN` is matched twice over — by the `_TOKEN`
suffix in `sensitiveEnvSuffixes` and by an explicit `GITHUB_TOKEN` entry in
`sensitiveEnvPrefixes`. A `gh` invoked from the bash tool is unauthenticated.

There is a plausible path: `gh auth login` writes credentials to
`~/.config/gh/hosts.yml`, and env-stripping does not touch files. In
host-mode bash (`SANDBOX_URL` unset) `gh` would find them. **Untested**, and
it does not survive the sandbox path, where the config file is not mounted.

So the GitHub mechanism is a genuine open decision (OD-1 below), not a
detail.

### C4. D1/D2's dev arc belongs to semdev, not here

ADR-056's D1 and D2 have SemTeams running `create_change` → `dev-from-task` →
PR. Since then, `semdev` has taken that scope: *"GitHub issue in → reviewed,
clean-room-verified pull request out"*, two human gates, and a
`docs/port-manifest.md` enumerating T1–T9 of what it took from SemTeams —
`dev-from-task`, `projectspecplan`, `analyzeproof` + `proof-readiness`, the
sandbox attestation model — mostly as *patterns*.

Consequences:

- The packs ADR-058 parked (`create-change`, `dev-from-task`, `dev-via-test`,
  `proof-readiness`) are **donor material, not debt**. Parking reads as
  migration in hindsight. Do not plan to unpark them here.
- SemTeams becomes what `CLAUDE.md` already claims it is: a harness, not
  harness-and-product.
- **SemTeams never opens a PR.** Docs drift files an *issue*. semdev turns
  issues into PRs. SemTeams reviews the PR and hands the operator a summary.
- semdev is *"foundational — docs and OpenSpec scaffolding only"*, with the
  M0 walking skeleton as its first change. Nothing below blocks on it until
  Phase 3.

## Corrections to this proposal (2026-08-24)

C1–C4 correct the D-ladder. These two correct *this document*, and both
change costings below.

### C5. SemSource does not share our graph — it is a separate service

The **What already works** table originally read *"SemSource ingests … into
the same graph."* That is wrong, and it was wrong when written.

SemSource removed headless mode: `refactor!: remove headless mode — semsource
is standalone-only` (semsource `60b1982`, PR #10, ADR-0006). Its `config`
package now rejects `mode: "headless"` outright, and ADR-0003's load-bearing
premise — *"SemSource commonly runs embedded in a semstreams host app, sharing
NATS and a config KV bucket"* — is explicitly retired by ADR-0006. The stated
reason is resource cost: SemSource on a large corpus is heavy enough that
embedding it in a host process is the wrong default.

ADR-0006 also re-bases the integration model. SemSource is *"an optional
external service"* whose motivating consumer is *"an agent — Claude Code
pointing SemSource at targets over MCP or HTTP"*. So there are **two
deployments, two NATS, two graphs**, and reaching SemSource is a *tool*
problem, not a *query* problem.

Three consequences:

- **Phase 2 is not free.** It is specified below as "swap web gather for a
  SemSource graph query". There is no such query from our side — it is a
  cross-service call, and Phase 2 must be costed with the reach work in it.
- **The read side is reachable over plain HTTP**, which is the good news.
  `code-context` / `doc-context` mount `POST /<prefix>/{verb}` on the shared
  mux (`processor/code-context/component.go:420-432`), covering the verbs an
  agent actually wants — `code_context`, `code_search`, `code_impact`,
  `doc_context`, `code_changes`. **An MCP client is not required to integrate
  SemSource** (see G9).
- **`add_source_repo` needs re-verifying** — see OD-9. It is our existing
  product-local SemSource tool and it reaches SemSource over NATS
  (`graph.ingest.add.{namespace}`, via `RequestWithRetryClassified`), a
  transport chosen under the shared-NATS premise ADR-0006 retired.

### C6. `http_request` is a page fetcher, not an API client

Worth stating because it is the obvious first guess for every "just call the
thing" workaround in this document.

`http_request` accepts `GET` or `POST` but builds every request with a **nil
body** (`processor/agentic-tools/executors/httprequest.go:169`) and sets only
a fixed `User-Agent` / `Accept` pair (`:176-177`). There is no custom-header
parameter. It therefore cannot carry a query body, cannot set
`Content-Type`, and cannot authenticate.

Two things this rules out:

- **Reading our own graph.** graph-gateway's GraphQL handler is POST-only and
  expects a JSON body (`gateway/graph-gateway/component.go:1948`). A
  cron-spawned loop that needs to read graph state must use `bash` + `curl`,
  or a product-local tool.
- **Reaching SemSource.** Its read verbs are `POST` with a JSON body, and its
  MCP endpoint sits behind bearer auth (`processor/mcp-gateway/component.go:158-171`)
  — a token that would also hit the C3 `*_TOKEN` env-strip trap.

`bash` + `curl` is the only no-new-tool path in both cases.

## What already works

Verified at beta.160. None of this needs building.

| Capability | Where |
|---|---|
| Clock-driven dispatch | `processor/rule/cron_rule.go`; scheduler started at `processor/rule/processor.go:574-592` |
| Ollama as a first-class provider | `model/registry.go:230-262`; ops guide `docs/operations/04-ollama-setup.md` |
| Per-role model routing | `model_registry.capabilities.{coordinator,research,reviewing,general}` with `preferred`/`fallback` |
| Host-side shell | `bash` runs via `os/exec` when `SANDBOX_URL` is unset (`executors/bash.go`) |
| Web research | `web_search` (Brave, env-gated) and `http_request` builtins |
| Outbound to the operator | `decide(respond_direct)` publishes on `user.response.>` (`configs/flow-bootstrap.json:659`); `output.httppost` is a registered component (`componentregistry/register.go:138`) |
| Doc/code ingestion | SemSource ingests `git`, `docs`, `url`, `code`, `video`, `audio` — into **its own** graph, as a separate service. See C5. |
| Per-loop token facts | `agent.loop.tokens-in` / `tokens-out`, stamped on completion and failure (`processor/agentic-loop/graph_writer.go:567-580`, `:611-634`) |

The D3 notification channel is therefore **config-only**: point an
`output.httppost` at `user.response.>` and a webhook (ntfy, Pushover, Slack).

## Gaps

**G1 — No seen-set / dedupe memory.** `agentic-memory` is not in
`flow-bootstrap.json`; there is the graph and `emit_lesson`. A daily
trend-watcher without a seen-set reports the same three releases every
morning for a week. Compounded by C1: a cron tick carries no state, so
"what is new since last run" must be a graph query. This gates Phases 2 and 3.

**See G7 for the mechanism** — that graph query is not currently expressible
from an agent, and `emit_lesson` is inert without a promotion path (G8). G1 is
the symptom; G7 and G8 are the two causes.

**G2 — No docs-maintenance pack.** Live categories are `research` and
`autoresearch`. Nothing is doc-drift shaped.

**G3 — No GitHub mechanism.** See C2/C3.

**G4 — No spend ceiling.** Existing caps bound *shape* only —
`max_iterations`, `loop_max_iterations`, rule `max_iterations`, cron
`cooldown`, planner fan-out `N`. None bound dollars. Tracked as
semstreams#1005 (run-level rollup, amended to step-level cost) and
semteams#248 (no endpoint declares pricing, so `agent.loop.cost-usd` has
never been written in this deployment). ADR-056 named the *"autonomous
cost/failure breaker"* as net-new D2 surface and was right.

**G5 — Ops pack unwired — being resolved in flight.** As of `origin/main`
today the ops rules (`chain-terminal-observe.json`,
`observe-chain-progress.json`) are still out of the bootstraps'
`rules_files`, where MVP-7 left them. A rewiring is in flight on the
beta.160 branch (PR #247, not yet merged): one rule,
`configs/rules/ops/01-run-terminal-observe.json`, triggered on the **run
entity** reaching a terminal phase, with a single `ops-chain-observer` role.
Triggering on the run rather than on a reviewer role is what makes it
category-agnostic and lets it cover failed and cancelled runs.

Treat G5 as closed once that lands, with one caveat that lands squarely on
this proposal: a completion-triggered rule **structurally cannot observe a
stalled chain**. The rewiring notes this and calls for "a cron primitive with
an idle-cost gate that does not exist yet". Half of that now exists — per C1
the cron primitive ships. What is missing is only the idle-cost gate, and a
stall detector is a natural second consumer of the Phase 0 heartbeat: a
scheduled loop that queries for runs with no forward progress, rather than a
rule waiting for a terminal event that a wedged chain never emits.

**G6 — Use case 2 is a build project.** `c360studio.github.io` is
hand-written HTML. Standing up Starlight/Astro is ordinary dev work; SemTeams'
honest role is *maintaining* content freshness afterwards, which is the same
pack as use case 1.

**G7 — No agent-facing way to enumerate entities. This is the mechanism
behind G1.** G1 above states the symptom; this is the cause. An agent gets
five graph query tools, and the only one that could answer *"list all active
X"* is a stub: `query_by_type` returns an empty list plus the note *"Type-based
queries require entity type index"*, and its own comment reads `placeholder -
requires index` (`processor/agentic-tools/executors/graph_query.go:516-560`).
The other four all require a known entity ID — and a cron fire has **no entity
in scope** (C1), so none of them is reachable from the heartbeat.

The backing primitive exists and is in production use inside the framework:
`graph.ingest.query.prefix` is a paginated prefix listing
(`processor/graph-ingest/query.go:41`), consumed by
`agentic-loop/lessons.go:20` and `gated-dag/reader.go:64`, and re-exposed as
GraphQL `entitiesByPrefix` for graph-gateway (`processor/graph-query/query.go:56`).
Both components are wired in `flow-bootstrap.json`. So this is a missing
*exposure*, not a missing capability — `bash` + `curl` against graph-gateway
reaches it today (C6).

**G8 — Procedural memory is wired and inert.** `emit_lesson` registers
unconditionally (`executors/register.go:190`) and `agentic-loop` sets the
lesson reader unconditionally
(`processor/agentic-loop/component.go:292-295`), so every dispatch already
tries to inject matching lessons. But lessons are born `proposed`
(`agentic-tools/emit_lesson.go:60`), `lessonmatch.Match` injects **only**
`active`, and the sole `Promote` callers in the module are
`cmd/e2e-semstreams/main.go:153` and an E2E-only NATS subject. **In
production, lessons are written and never read back.**

This is deliberate. `LessonCurator`
(`processor/agentic-tools/lesson_promotion.go:31-42`) documents itself as
*"the OPERATOR/PRODUCT curation path, NOT an agent tool"*, notes that ADR-080
makes operator review the default promotion gate, and says a product may wrap
`Promote` in a curation UI, a rule action, or an explicit auto-promotion
policy. So a promotion path is product-local **by explicit upstream intent** —
the same posture as C2 — and the framework-alignment survey is satisfied by an
upstream statement rather than by our own argument. See OD-7.

**G9 — No MCP client, and semstreams' own MCP server is a stub.** There is no
MCP client anywhere in `agentic-tools`; an agent cannot consume an MCP server.
Separately, `componentregistry/register.go:60` advertises graph-gateway as
*"GraphQL + MCP HTTP servers"*, but `handleMCP` returns
`{"message": "MCP endpoint"}` with the comment *"In real implementation, this
would handle MCP protocol"* (`gateway/graph-gateway/component.go:2089-2100`).

**This does not block SemSource integration** (C5: the read verbs are plain
HTTP). It blocks reaching *arbitrary third-party* MCP servers. The
framework-vs-product split is worth stating now so it is not re-litigated
later:

- **Product.** Tools that reach SemSource specifically. The precedent is
  already set — `cmd/semteams/tools/README.md` records `add_source_repo`'s
  migration posture as *"Stays product-local. The namespace allowlist is
  per-deployment policy; SemSource is a sibling product, not framework"* — and
  that README's step 4 explicitly blesses *"calls an external service → mirror
  `add_source_repo`'s request/reply"*.
- **Framework.** A *generic* MCP client — remote tool discovery, schema
  translation, and integration with `allowed_tools` / `enable_categories` /
  `approval_required` — is not SemSource-specific, is wanted by every
  semstreams product, and touches the tool registry the framework owns.
  Building it product-local would reimplement discovery and governance outside
  the layer that owns them, which is the accretion trap `CLAUDE.md` names.
  This is an upstream ask, and not one this deployment needs.

## Phasing

**Phase 0 — Prove unattended operation. Config only.**
One cron rule firing a daily coordinator loop; `output.httppost` on
`user.response.>` to the operator's phone; SemSource pointed at the `sem*`
docs repos and the docs site — an explicit, minimal corpus, per §Hardware.

Ship the containment budget in the *same* phase, before the first tick:
conservative `max_iterations` on every role; a cron `cooldown` backed by the
scheduler's inflight guard so a wedged chain cannot stack fires (C1); a hard
fan-out ceiling in both the planner persona and the schema (schema is
load-bearing, prose is hopeful); **G5 landed**; and a kill switch verified by
actually flipping the cron rule's `enabled: false` through the Pattern-B rule
manager and confirming the heartbeat stops without a restart.

Steady-state cost is not the risk. The canonical research arc runs ~8 loops
in ~2 minutes for ~$0.30 on `gemini-flash`; a daily docs sweep plus a daily
radar is roughly $20/month. The risk is the 3am pathology — a wedged chain
re-firing, or a planner decomposing into 15 subtopics instead of 4, with
nobody watching until morning.

**Phase 1 — The seen-set (G1/G7).** Everything downstream needs it. This was
originally written as "no existing pattern to copy"; there is now a pattern
worth borrowing, and a blocker to clear first.

- **Clear G7 first — the store is inert without enumeration.** `bash` + `curl`
  against graph-gateway's GraphQL `entitiesByPrefix` is the no-new-tool path
  (C6 rules out `http_request`). Prove a cron-spawned loop can list its own
  entities before designing what they contain.
- **Borrow PARA's actionability axis for the store's shape.** Not the folder
  hierarchy — the single idea that items are organized by *whether they are
  live work*, which is precisely the axis our graph lacks: every durable write
  today is anchored to process lineage (run → loop → triples), and nothing
  says "this is open." Concretely: one entity type with a status predicate,
  archived by status transition through a reconcile projection contract rather
  than by moving anything. Of PARA's four categories, Areas are already our
  cron rules, Resources are SemSource, and Archives are that status
  transition — only Projects is net-new. See OD-8 for the boundary that has to
  be settled before the first entity is written.
- **Add a scheduled review loop in the same phase.** This is GTD's one genuinely
  missing step: a second cron rule, on a slower schedule, that reads the store,
  closes what finished, and resurfaces what stalled. It also retires the G5
  caveat — a completion-triggered rule structurally cannot observe a *stalled*
  chain, and this is the idle-cost gate that gap called for.
- **Cheap experiment, runnable now:** have one existing research-pack role call
  `emit_lesson`. It costs one persona fragment and tests whether push-memory
  earns its keep before anyone builds a project store. Pair it with OD-7 or it
  silently measures nothing (G8).

**Phase 2 — `docs-drift` category pack (G2).** Fork the research arc's shape
(plan → gather ×N → synthesize → review), swapping web gather for SemSource:
what the docs claim vs. what the code does. Terminal action is *file an
issue*, per C4.

**Re-costed by C5.** This was written as "a SemSource graph query" on the
assumption of a shared graph. It is a cross-service call, so the phase carries
its own reach work: a product-local tool per verb, `POST`ing to SemSource's
`code_changes` / `doc_context` / `code_impact` endpoints and mirroring
`add_source_repo`'s shape per `cmd/semteams/tools/README.md` step 4. The
compensating good news is that those verbs are exactly the docs-drift query
and they are already written — on SemSource's side, not ours.

**Phase 3 — `pr-review` pack.** The research arc again, gathering a diff, CI
status, and the linked issue. Gated on semdev M0.

**Phase 4 — Local model routing.** Cheapest to defer. By then the token facts
from G4 will show which roles actually burn budget.

## Hardware

**Target is a base M4 mini — 16GB unified memory, 256GB SSD.** That is a
decision, not an estimate, so resources are a design constraint rather than an
ops detail.

The harness itself fits comfortably: NATS, semteams, semsource, semembed, and
caddy are ~2–4GB resident. The **256GB SSD** is the binding constraint —
JetStream file storage, SemSource ingestion, devcontainer images, and any model
weights all compete for it. External NVMe is the cheap mitigation; a 512GB
build is the other.

**SemSource's corpus is the variable to bound, and it should be bounded in
Phase 0.** Per C5, SemSource dropped headless mode *because* it is expensive on
a large corpus — that is the same cost, now running as a separate always-on
service on the same 256GB. Two implications:

- **Scope SemSource to the doc repos it is actually needed for** (use case 1 —
  the `sem*` docs and `c360studio.github.io`), not to all of `~/Code/c360`.
  ADR-0006's model makes this natural: watch intent arrives per registration
  request, so the corpus is an explicit, revisable list rather than an ambient
  default.
- **Walking a corpus back is much harder than not adding it.** Ingested bytes
  become JetStream and ObjectStore state; the honest time to decide scope is
  before the first `add_source`, not after the SSD fills.

Worth measuring during Phase 0 rather than assuming: SemSource's steady-state
disk per repo, and whether ingestion is bursty enough to contend with
JetStream writes during a cron tick.

Local models are explicitly **not** on the critical path; Gemini API carries
the load until bigger iron (a 128GB Ryzen AI Max+ 395 class box is the
aspiration). Current guidance is that 16GB-class local models are unreliable
at multi-step tool calling, which is exactly what the `decide` contract
demands. The hybrid the config already expresses — local for `general`,
cloud for `coordinator` and `research` — is the shape when Phase 4 arrives.

If Ollama is used, note the `num_ctx` trap: the OpenAI-compat endpoint cannot
set context per-request, so a derived model must be built with
`PARAMETER num_ctx`, or every prompt past ~4096 tokens silently truncates
server-side. SemStreams warns once per endpoint. See
`docs/operations/04-ollama-setup.md` upstream.

## Open decisions

**OD-1 — GitHub mechanism.** Product-local `github_*` tool (upstream-sanctioned
per C2, and revivable from the beta.113 sources), versus `bash` + `gh`
authenticated by `~/.config/gh/hosts.yml` (C3 — untested, and does not survive
the sandbox path). Issue *creation* was absent from upstream's write set even
at beta.113, so it is net-new either way.

**OD-2 — Docs-drift output.** Filing an issue is assumed above (C4). Worth
confirming that an issue is something the operator would actually act on
versus a report they would read.

**OD-3 — Radar shape.** A separate pack, or a standing prompt against the
existing research pack? Reuse is the cheaper hypothesis.

**OD-4 — Gemini Pro's tiered pricing.** `computeCost` is flat per endpoint;
Gemini tiers at 200K prompt tokens. Under-reporting is the wrong direction
for a budget guard. See semteams#248.

**OD-5 — Course site scope.** Whether SemTeams builds the Starlight scaffold
or only maintains it (G6). Recommend the latter.

**OD-6 — Operator-authored or agent-inferred projects?** Meta's second-brain
rollout bootstrapped each workspace by *inferring* a portfolio from recent
activity, which worked because it had authenticated reach into task trackers,
doc editors, and code review. Our reach is git, docs, and the web (OD-1 still
open). Operator-authored is the honest starting point; inference only becomes
available after OD-1 resolves.

**OD-7 — Lesson promotion policy (G8).** Three options: operator approval,
routed through the D3 notification channel this deployment already needs;
a narrow auto-promotion rule scoped to one lesson class; or accept
`emit_lesson` as write-only telemetry and drop the injection expectation.
Must be settled before the Phase 1 `emit_lesson` experiment, or that
experiment measures nothing.

**OD-8 — Project vs. run vs. area boundary.** A `run` is one arc; a project
outlives many runs; an area never completes. If that is not explicit in the
predicate vocabulary, the project store degenerates into a second, worse copy
of run lineage — and "keep the docs current" becomes a project that never
terminates instead of the schedule it actually is. This has to be settled
**before the first entity is written**: the canonical predicate contract is
fail-closed and rule-pack JSON never recompiles, so a wrong vocabulary is a
migration, not an edit.

**OD-9 — Does `add_source_repo` still work? (C5)** It is our one shipped
SemSource integration and it reaches SemSource over NATS
(`graph.ingest.add.{namespace}`), a transport chosen under the shared-NATS
premise ADR-0006 retired. Either it requires cross-deployment NATS
connectivity, or it is latently broken. ADR-0006 landed HTTP
(`POST/DELETE/GET /sources`) and MCP registration surfaces alongside the NATS
one, so a port target exists either way. **Check this before Phase 0 points
SemSource at anything** — it is a five-minute verification and it determines
whether the reach work lands in Phase 0 or Phase 2.

## Re-verify at the next semstreams tag

Several findings above describe surfaces that are **actively being reworked
upstream** — lessons and graph queries in particular. They are recorded here
rather than fenced in CI on purpose: a canary pinned to a moving target costs
more maintenance than the finding is worth. The discipline is instead to
re-check this list whenever the semstreams pin moves, and to update the
`Verified against` line at the top when doing so.

| # | Claim to re-verify | Where |
|---|---|---|
| G7 | `query_by_type` is still a stub; no entity-type index | `agentic-tools/executors/graph_query.go:516-560` |
| G8 | Lessons still born `proposed`; still no production `Promote` caller | `agentic-tools/emit_lesson.go:60`; `lesson_promotion.go` |
| G9 | `handleMCP` is still a placeholder | `gateway/graph-gateway/component.go:2089-2100` |
| G9 | Still no MCP client in `agentic-tools` | `processor/agentic-tools/` |
| C6 | `http_request` still sends a nil body and fixed headers | `agentic-tools/executors/httprequest.go:169,176-177` |
| C2 | `github_*` tools still absent from `BuiltinGroupKeys` | `processor/agentic-tools/` |

If any row flips, the corresponding gap closes or shrinks — and G7 flipping
would remove the `bash` + `curl` workaround from Phase 1 entirely.

## Sources

Repository claims are cited inline by `file:line` at the versions named
above. External pricing and local-model claims were verified 2026-08-19
against vendor documentation; see semteams#248 for the price table and its
sources.
