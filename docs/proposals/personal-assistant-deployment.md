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
| Doc/code ingestion | SemSource ingests `git`, `docs`, `url`, `code`, `video`, `audio` into the same graph |
| Per-loop token facts | `agent.loop.tokens-in` / `tokens-out`, stamped on completion and failure (`processor/agentic-loop/graph_writer.go:567-580`, `:611-634`) |

The D3 notification channel is therefore **config-only**: point an
`output.httppost` at `user.response.>` and a webhook (ntfy, Pushover, Slack).

## Gaps

**G1 — No seen-set / dedupe memory.** `agentic-memory` is not in
`flow-bootstrap.json`; there is the graph and `emit_lesson`. A daily
trend-watcher without a seen-set reports the same three releases every
morning for a week. Compounded by C1: a cron tick carries no state, so
"what is new since last run" must be a graph query. This gates Phases 2 and 3.

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

**G5 — Ops pack unwired.** `configs/rules/ops/*.json` left the bootstraps'
`rules_files` during MVP-7. For an unattended box this is the only thing that
notices a chain spinning. Promoted here from a nice-to-have to a Phase 0
prerequisite.

**G6 — Use case 2 is a build project.** `c360studio.github.io` is
hand-written HTML. Standing up Starlight/Astro is ordinary dev work; SemTeams'
honest role is *maintaining* content freshness afterwards, which is the same
pack as use case 1.

## Phasing

**Phase 0 — Prove unattended operation. Config only.**
One cron rule firing a daily coordinator loop; `output.httppost` on
`user.response.>` to the operator's phone; SemSource pointed at the `sem*`
repos and the docs site.

Ship the containment budget in the *same* phase, before the first tick:
conservative `max_iterations` on every role; cron `cooldown` so a wedged
chain cannot stack fires; a hard fan-out ceiling in both the planner persona
and the schema (schema is load-bearing, prose is hopeful); **G5 rewired**;
and a kill switch verified by actually flipping the cron rule's
`enabled: false` through the Pattern-B rule manager and confirming the
heartbeat stops without a restart.

Steady-state cost is not the risk. The canonical research arc runs ~8 loops
in ~2 minutes for ~$0.30 on `gemini-flash`; a daily docs sweep plus a daily
radar is roughly $20/month. The risk is the 3am pathology — a wedged chain
re-firing, or a planner decomposing into 15 subtopics instead of 4, with
nobody watching until morning.

**Phase 1 — The seen-set (G1).** No existing pattern to copy; everything
downstream needs it.

**Phase 2 — `docs-drift` category pack (G2).** Fork the research arc's shape
(plan → gather ×N → synthesize → review), swapping web gather for a SemSource
graph query: what the docs claim vs. what the code does. Terminal action is
*file an issue*, per C4.

**Phase 3 — `pr-review` pack.** The research arc again, gathering a diff, CI
status, and the linked issue. Gated on semdev M0.

**Phase 4 — Local model routing.** Cheapest to defer. By then the token facts
from G4 will show which roles actually burn budget.

## Hardware

The harness runs comfortably on a base M4 mini (16GB): NATS, semteams,
semsource, semembed, caddy is ~2–4GB resident. The **256GB SSD** is the real
constraint — JetStream file storage, SemSource ingestion, devcontainer images,
and any model weights all compete. External NVMe or 512GB.

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

## Sources

Repository claims are cited inline by `file:line` at the versions named
above. External pricing and local-model claims were verified 2026-08-19
against vendor documentation; see semteams#248 for the price table and its
sources.
