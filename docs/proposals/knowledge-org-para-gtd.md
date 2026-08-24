# Research note: does PARA / GTD help the personal-assistant pack?

**Status**: Research notes (2026-08-24). Not a decision. Companion to
[`docs/proposals/personal-assistant-deployment.md`](personal-assistant-deployment.md)
— specifically its **G1 (no seen-set / dedupe memory)** gap and **Phase 1**,
which that doc describes as *"no existing pattern to copy; everything
downstream needs it."*
**Verified against**: semstreams `v1.0.0-beta.160`, semteams
`chore/semstreams-beta160` @ `adcc07a4`, semsource `main`.
**Tracking**: the actionable items are folded into the companion proposal as
C5/C6, G7-G9, OD-6-OD-9 and its re-verification list. This note is the
evidence trail.
**Sources**: Tiago Forte's PARA; David Allen's GTD; *How We Built an AI Second
Brain for 60K Knowledge Workers*, Analytics at Meta, 2026-04-29.

## Verdict

Split, and the split matters more than either half.

- **PARA — yes, as an entity taxonomy, not as folders.** Its one load-bearing
  idea is *organize by actionability, not by subject*. That is precisely the
  axis SemTeams' graph lacks: every durable write today is anchored to
  **process lineage** (run → loop → triples). Nothing in the graph says *"this
  is live work."* A cron tick with no entity in scope has nothing to ask.
  Adopt the axis; discard the folder hierarchy.

- **GTD — mostly already built, one piece missing.** Capture / Clarify /
  Organize / Engage map onto surfaces that exist. **Reflect** — the weekly
  review — does not, and it is the valuable one. It is also, independently,
  the answer to a caveat the deployment proposal already flagged (G5: a
  completion-triggered ops rule structurally cannot observe a *stalled*
  chain).

- **Neither is a phase.** Folded correctly, PARA gives Phase 1 a design it
  currently lacks and GTD adds one cron rule. Cost is roughly zero incremental
  phases. Treating either as a new workstream would be the mistake.

The largest finding is not from either method. It is that **two pieces of
memory machinery are already wired and inert** (§4), and that the
enumeration primitive a PARA store needs **exists but is not agent-facing**
(§5). Those gate any adoption and are worth more than the taxonomy.

## 1. What Meta actually built, and what transfers

Their stack: Claude Code + the latest Anthropic model, a filesystem workspace
laid out as PARA folders, a `CLAUDE.md` per project plus a **root** `CLAUDE.md`
carrying identity and active portfolio, MCP/CLI access to internal systems, and
skills-as-markdown (`/para-init`, `/start-project`, `/read-meeting-notes`,
`/debrief:team`). 63,000 installs in three months, ~10,000 DAU, every major
post-launch feature contributed by the community.

Three of their lessons transfer; two of their design choices do not.

**Transfers — infrastructure before taxonomy.** Their stated #1 lesson is that
*the agent is only as useful as the systems it can reach*, and that the
authenticated MCP/CLI layer, not PARA, was the adoption unlock. This lands
directly on **OD-1 / G3** (the GitHub mechanism) and says: the reach problem
outranks the organization problem. It is an argument for keeping Phase 0 first
and *not* inserting a PARA phase ahead of it.

**Transfers — progressive disclosure.** They found context *quality* beats
quantity, that too many context files degrade the agent, and that tiered
loading (root summary first, project detail on demand) was "one of the most
important design decisions." SemTeams already has the mechanism in the shape
upstream chose for lessons: `lessonmatch.Match` bounds injection by a count
ceiling **K** and a **byte budget**, reports `MatchedCount` vs `IncludedCount`
so truncation is visible, and orders severity DESC → created-at DESC. Adopting
PARA does not require inventing this; it requires reusing it.

**Transfers — a cold start is the adoption killer.** `/para-init` bootstrapped
a workspace by scanning recent activity, so users got value in session one.
The SemTeams analogue is whether the project store is operator-authored or
agent-inferred (OD-6 below).

**Does not transfer — the filesystem.** Their PARA is a directory tree the
agent reads. SemTeams' store is a triple graph behind a **fail-closed**
canonical predicate contract: exactly three dot-segments, lower-kebab, no
alias mode, enforced at persistence and fenced in CI by
`test/contract/predicate_contract_test.go`. Entity IDs are six parts
(`{org}.{platform}.{domain}.{system}.{type}.{id}`). Copying folders means
standing up a *second* store beside the graph, which is the accretion pattern
`CLAUDE.md` names as the semspec lesson. A PARA taxonomy here must be authored
in the graph dialect from the first line — `assistant.project.status`, never
`para.project_status`.

**Does not transfer — the human in the loop.** Meta's second brain is
prompt-driven, one workspace per person, with a human reading every output.
The personal-assistant pack is unattended and cron-driven. Their roadmap
("proactive agents… morning briefings, end-of-day digests" and the team-level
"Third Brain") is where SemTeams *starts*: cron rules ship, and the graph is
already shared across loops. **SemTeams is ahead of Meta on the autonomy axis
and behind on the reach axis.** Reading their post as a to-do list would
invert the actual priority.

## 2. PARA against the primitives that exist

| PARA | What it would be here | Status |
|---|---|---|
| **Projects** | An `*.*.assistant.project.record.{uuid}` entity with an actionability status — the thing a cron tick queries to decide what to do | **Net-new. The only real build.** |
| **Areas** | Standing responsibilities with no end state — "keep `sem*` docs current" | **Already exists, as the cron rule + category pack.** An area *is* a schedule. Do not duplicate it as data. |
| **Resources** | Reference material | **Already exists, but out of process** — SemSource ingests `git`/`docs`/`url`/`code`/`video`/`audio` into **its own** graph, reachable over HTTP (§6), not by querying ours. |
| **Archives** | Completed / abandoned | **Free.** A status transition, not a move: reconcile `assistant.project.status = archived` through a projection contract, the same lane `autoresearch.run-projection` already uses in `flow-bootstrap.json`. |

So three of PARA's four categories are already present or free. The method
reduces, for our purposes, to **one entity type carrying an actionability
status** — and to the discipline of Forte's warning about "false projects"
(no deadline, no completion state), which for an unattended agent is the
difference between a bounded work item and an unbounded one.

Two boundary questions this forces, and they are the reason to write the
taxonomy down rather than let it emerge:

- **Project vs. run.** A `run` is one arc; a project outlives many runs. If
  that distinction is not explicit in the predicate vocabulary, the two fuse
  and the store becomes a second, worse copy of run lineage.
- **Project vs. area.** "Keep the docs current" is an area (a schedule).
  "Ship the course site scaffold" is a project (§G6 / OD-5 in the deployment
  proposal). Getting this wrong produces either a cron rule that never
  terminates or a project that silently becomes a standing job.

## 3. GTD against the primitives that exist

| GTD step | SemTeams surface | Status |
|---|---|---|
| **Capture** | cron tick, SemSource ingest, user front door | exists ×3 |
| **Clarify** | coordinator `decide` — literally GTD's *"what is the next action?"*, as a closed 4-token taxonomy | exists |
| **Organize** | graph writes | exists, but process-shaped rather than actionability-shaped (§2) |
| **Reflect** | — | **missing** |
| **Engage** | category packs | exists |

GTD's contribution is therefore **one rule and one persona**, not a system.

**Reflect is worth building.** A scheduled review loop that reads the project
store, closes what finished, and resurfaces what stalled is the same cron
heartbeat Phase 0 already introduces, on a different schedule. It also closes
the G5 caveat that the ops rewiring left open: the deployment proposal notes a
stall detector needs "a cron primitive with an idle-cost gate that does not
exist yet," and that half of it now ships. The review loop *is* the other
half — a scheduled query for runs with no forward progress, rather than a rule
waiting for a terminal event a wedged chain never emits.

**Skip the rest.** Contexts (`@home`, `@calls`), the two-minute rule, and
someday/maybe lists are devices for managing *human* attention and *human*
task-switching cost. An agent has neither. Importing them would be cargo cult.

One transferable discipline, though: GTD insists every captured item resolves
to a concrete next action or leaves the system. The coordinator taxonomy is
closed at four actions, and a personal-assistant deployment will routinely
receive asks that are none of them. Today that path is `respond_direct` case
(d) — "this deployment doesn't support that" — which is honest but drops the
item. A project store gives those asks somewhere to land instead of dying in a
loop transcript.

## 4. Already wired, currently inert — check this before building anything

Two findings, both verified in the beta.160 module. Together they are a
cheaper experiment than any taxonomy work.

### 4a. ADR-080 procedural memory is fully wired and unused by SemTeams

- `emit_lesson` registers unconditionally in `RegisterBuiltins`
  (`executors/register.go:190`); `flow-bootstrap.json` sets
  `allowed_tools: null`, so every role already has it.
- `agentic-loop` sets the reader unconditionally —
  `handler.SetLessonReader(NewNATSLessonReader(deps.NATSClient))`
  (`processor/agentic-loop/component.go:292-295`) — so every dispatch's brief
  assembly already tries to inject matching lessons.
- Lessons carry a real lifecycle (`proposed → active → retired/superseded`),
  `applies-to` scope keys in a `tag:` / `id:` grammar, severity, and the
  bounded injection described in §1.

**Zero SemTeams personas call `emit_lesson`.** This is GTD's Reflect and
PARA's Areas, already implemented, sitting idle.

### 4b. …but there is no production promotion path, and that is the catch

`lessonmatch.Match` injects **only** `status == "active"`
(`lessonmatch.go`), and `emit_lesson` births every lesson `"proposed"`
(`emit_lesson.go:60`). The only `Promote` callers in the module are
`cmd/e2e-semstreams/main.go:153` and the E2E harness subject
`e2e.control.lesson.promote`. **In a production deployment, lessons are
written and never injected.**

This is deliberate, not an oversight, and upstream says so in terms that
resolve the framework-alignment question for us. `LessonCurator`'s doc comment
(`processor/agentic-tools/lesson_promotion.go:31-42`):

> This is the OPERATOR/PRODUCT curation path, NOT an agent tool: ADR-080 makes
> operator/product review the default promotion gate, so the framework ships
> no `promote_lesson` tool. A product may wrap `Promote` in a curation UI, a
> rule `reconcile_predicates` action (for mechanical/retirement transitions),
> or an explicit auto-promotion policy…

`Promote` also refuses to activate a lesson whose cited evidence entities do
not resolve in the graph — the honesty gate.

So a curation path is **product-local by explicit upstream intent**, the same
posture as the `github_*` removal in C2. `CLAUDE.md`'s framework-alignment
step 2 is satisfied by an upstream statement rather than by our own argument,
and this note is the evidence trail.

Practical consequence: for an unattended assistant, promotion needs a policy —
operator approval through the UI (safe, and it is the D3 notification channel
doing double duty), or a narrow auto-promotion rule for a specific lesson
class. Either is small. Neither exists today.

## 5. The enumeration gap — a PARA store hits this on day one

A project store is only useful if something can ask *"list all active
projects."* Today an agent cannot.

- The **primitive exists**: `graph.ingest.query.prefix` is a paginated prefix
  listing over entity IDs (`processor/graph-ingest/query.go:41`), consumed
  in-framework by the lesson reader (`agentic-loop/lessons.go:20,67`) and by
  `processor/gated-dag/reader.go:64`. `graph-query` re-exposes it as GraphQL
  `entitiesByPrefix` for `graph-gateway` (`processor/graph-query/query.go:56`,
  `router.go:21`), and both components are wired in `flow-bootstrap.json`.
- The **agent-facing tool is a stub**. `query_by_type` advertises "Query all
  entities of a specific type with optional limit" and then returns an empty
  list with the note *"Type-based queries require entity type index. Use
  query_entity or query_entities with known IDs"*
  (`executors/graph_query.go:517-560`; the function's own comment says
  `placeholder - requires index`).

The model therefore sees five graph query tools, one of which silently answers
"nothing" to the only question a PARA store needs to ask. It can fetch by
known ID and it can walk neighbours from a known entity — but a cron tick has
**no entity in scope**, so neither entry point is reachable from the
heartbeat. This is the concrete mechanism behind G1.

Two ways out, cheapest first:

1. **No new tool** — `bash` + `curl` against graph-gateway's GraphQL
   `entitiesByPrefix`. Zero framework-alignment surface, testable today, and
   consistent with the "fewer rich tools, bash subsumes file ops" principle.
   Try this first.

   **`http_request` cannot do this job** — worth stating because it is the
   obvious first guess. It accepts GET or POST but builds every request with a
   **nil body** (`executors/httprequest.go:169`) and sets only a fixed
   `User-Agent` / `Accept` pair (`:176-177`) with no custom-header parameter.
   graph-gateway's GraphQL handler is **POST-only**
   (`gateway/graph-gateway/component.go:1948`) and expects a JSON body. So
   `http_request` is a page fetcher, not an API client: it cannot carry a
   query body and cannot authenticate. `bash` + `curl` is the only
   no-new-tool path.
2. **A thin product-local tool** over `graph.ingest.query.prefix`. Justified
   if (1) proves too brittle for a persona to drive reliably. The alignment
   argument is unusually clean: the backing surface is upstream, proven, and
   paginated, and the tool that *should* expose it is a documented placeholder.

**This is also a better upstream issue than either currently filed.**
semstreams#1006 (no read path for trajectory `StorageReference`s) and #1007
(`fire_every_n_events` / hardcoded `submit_work`) both report missing or wrong
behaviour. This one reports a **stub whose backing implementation already
exists in the same repo** — an unusually cheap fix upstream, and it unblocks
any product that needs to enumerate its own entities.

## 6. Does SemSource + MCP change the picture?

Partly — and in a way that raises the value of §5 rather than dissolving it.

**SemSource is the right owner for "watch the scattered folders, repos, and
URLs."** That is literally its job description, it already ships the crawlers
and the provenance model, and building any of it inside a SemTeams category
pack would be the accretion trap.

**They do NOT share a graph — corrected 2026-08-24.** An earlier draft of this
section claimed they did, reasoning from the shared `semstreams
v1.0.0-beta.160` pin (`semsource/go.mod:8`) and SemSource running its own
`graph-ingest` onto a `GRAPH` JetStream (`cmd/semsource/run.go:658-671`). That
inference was wrong. SemSource **removed headless mode** — `refactor!: remove
headless mode — semsource is standalone-only` (`60b1982`, PR #10, ADR-0006) —
and its `config` package now rejects `mode: "headless"`. ADR-0003's premise
that SemSource *"commonly runs embedded in a semstreams host app, sharing NATS
and a config KV bucket"* is explicitly retired, on resource-cost grounds: on a
large corpus SemSource is heavy enough that embedding it is the wrong default.

So there are **two deployments, two NATS, two graphs.** ADR-0006 re-bases the
model around SemSource as *"an optional external service"* whose motivating
consumer is *"an agent — Claude Code pointing SemSource at targets over MCP or
HTTP"*. Reaching it is a **tool** problem, not a **query** problem.

**SemSource's MCP gateway is real, unlike ours.** `processor/mcp-gateway/`
builds a genuine server on the official Go SDK (`mcp.NewServer`, `tools.go:74`)
exposing nine tools — `add_source`, `remove_source`, `source_status`,
`code_context`, `code_impact`, `code_search`, `doc_context`, `code_changes`,
`graph_search` — over **Streamable HTTP behind bearer auth**
(`component.go:158-171`). It even ships `disclosure.go` and a schema-budget
test, i.e. Meta's progressive-disclosure lesson, already implemented.

Note the contrast, which is a **third inert surface**: semstreams'
`componentregistry/register.go:60` advertises graph-gateway as *"GraphQL + MCP
HTTP servers"*, but `handleMCP` returns `{"message": "MCP endpoint"}` with the
comment *"In real implementation, this would handle MCP protocol"*
(`gateway/graph-gateway/component.go:2089-2100`). **SemStreams' MCP server is
a placeholder; SemSource's is the real one.**

**But an MCP client is not what we need, and we do not have one anyway.**
There is **no MCP client in `agentic-tools`** — the only MCP references in the
semstreams module are the graph-gateway stub, a doc comment, and two type
files. An agent cannot call an MCP server, and `http_request` cannot stand in
(§5: nil body, no headers, so neither JSON-RPC nor the bearer token works;
`bash` also strips `*_TOKEN`, the C3 trap).

**The read side does not need MCP.** SemSource's `code-context` /
`doc-context` components mount `POST /<prefix>/{verb}` on the shared mux
(`processor/code-context/component.go:420-432`), covering `code_context`,
`code_search`, `code_impact`, `doc_context`, and `code_changes` over plain
HTTP/JSON. So the ranking is:

1. **Product-local tools over SemSource's HTTP verbs.** The path to take. The
   precedent is already set and documented: `cmd/semteams/tools/README.md`
   records `add_source_repo`'s posture as *"Stays product-local … SemSource is
   a sibling product, not framework"*, and its step 4 blesses *"calls an
   external service → mirror `add_source_repo`'s request/reply"*. No MCP, no
   upstream dependency.
2. **`bash` + `curl` against SemSource's MCP endpoint.** Mechanically possible
   (Streamable HTTP is JSON-RPC over POST) but means hand-rolling the handshake
   in a persona and solving the bearer token. A spike, not a plan — and
   strictly worse than (1) now that the HTTP verbs are known.
3. **A generic MCP client in `agentic-tools`.** Correct long-term, and a clean
   **upstream** ask — remote tool discovery, schema translation, and
   integration with `allowed_tools` / `approval_required` are framework
   concerns, and building them product-local would put discovery and governance
   outside the layer that owns them. **Not needed for SemSource**; it is what
   third-party MCP servers would require.

**Verify before relying on any of this: does `add_source_repo` still work?** It
is our one shipped SemSource tool and it uses NATS
(`graph.ingest.add.{namespace}` via `RequestWithRetryClassified`) — a transport
picked under the shared-NATS premise ADR-0006 retired. ADR-0006 landed HTTP and
MCP registration surfaces beside the NATS one, so a port target exists either
way. Tracked as OD-9.

One payoff survives the correction, and it is the important one: **Phase 2
(`docs-drift`) still gets its query for free.** `code_changes`, `doc_context`,
and `code_impact` are exactly "what do the docs claim vs. what does the code
do" — already written, on SemSource's side. What Phase 2 must now carry is the
reach work, not the query.

## 7. Recommendation

Against the existing phase plan, changing as little as possible:

1. **Do not insert a PARA phase before Phase 0.** Meta's own #1 lesson is
   infrastructure-first, and Phase 0 (containment + heartbeat + notify + the
   verified kill switch) *is* the infrastructure. Nothing here justifies
   reordering.
2. **Fold PARA into Phase 1 as the shape of the seen-set.** Phase 1 already
   exists and is explicitly undesigned. PARA supplies the design — one entity
   type, an actionability status, archive-by-status-transition — at no
   additional phase cost. It converts an open phase into a specified one.
3. **Resolve §5 first, inside Phase 1.** The store is inert without
   enumeration. `bash` + `curl` against graph-gateway GraphQL is the
   no-new-tool path; `http_request` is disqualified (nil body, no headers).
4. **Add GTD's Reflect as a second cron rule in the same phase**, not a new
   one. Same heartbeat, different schedule. It retires the G5 stall-detector
   caveat as a side effect.
5. **Try `emit_lesson` now, in the existing research pack** — one persona
   fragment, no phase, no new surface. It tests whether push-memory earns its
   keep *before* anyone builds a project store. Pair it with a decision on
   §4b promotion, or the experiment silently measures nothing.
6. **Verify `add_source_repo` before Phase 0 points SemSource at anything**
   (§6, OD-9), and bound SemSource's corpus in the same breath — the target is
   a base M4 mini (16GB / 256GB), and the corpus is the one variable that grows
   without limit. Scope it to the doc repos use case 1 actually needs.
7. **Skip** PARA folders, per-project `CLAUDE.md` files, GTD contexts, the
   two-minute rule, and someday/maybe lists.

## 8. Open decisions

Numbered to continue the deployment proposal's OD series.

- **OD-6 — Operator-authored or agent-inferred projects?** Meta's `/para-init`
  inferred a portfolio by scanning activity, which worked *because* they had
  authenticated reach into task trackers, doc editors, and code review.
  SemTeams' reach is git, docs, and the web. Operator-authored is the honest
  starting point; inference becomes available only after OD-1 resolves.
- **OD-7 — Lesson promotion policy (§4b).** Operator approval via the D3
  notification channel, a narrow auto-promotion rule for one lesson class, or
  leave lessons inert and treat `emit_lesson` as write-only telemetry. Must be
  decided before recommendation 7.5 measures anything.
- **OD-8 — Project vs. run vs. area boundary (§2).** Needs to be explicit in
  the predicate vocabulary before the first entity is written, because the
  predicate contract is fail-closed and rule-pack JSON never recompiles.
- **OD-9 — Does `add_source_repo` still reach a standalone SemSource? (§6)**
  It uses NATS under a shared-NATS premise ADR-0006 retired. Either
  cross-deployment NATS connectivity is required, or it is latently broken and
  should port to the HTTP registration surface ADR-0006 landed.

## Sources

Repository claims are cited inline by `file:line` at the versions named above.

- [The PARA Method — Todoist](https://www.todoist.com/productivity-methods/para-method)
- [Getting Things Done — Todoist](https://www.todoist.com/productivity-methods/getting-things-done)
- [The 5 Steps of GTD — gtd.be](https://www.gtd.be/en/what-is-gtd/the-5-steps-of-gtd)
- [How We Built an AI Second Brain for 60K Knowledge Workers — Analytics at Meta](https://medium.com/@AnalyticsAtMeta/how-we-built-an-ai-second-brain-for-60k-knowledge-workers-78c507dd795b)
- [How Meta Built an AI Second Brain — Forte Labs](https://optin.fortelabs.co/posts/how-meta-built-an-ai-second-brain-for-60k-knowledge-workers)
