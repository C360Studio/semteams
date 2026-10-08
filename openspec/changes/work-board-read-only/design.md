# Read-only work board design

## Scope and evidence

[Issue #296](https://github.com/C360Studio/semteams/issues/296) and
[PR #304](https://github.com/C360Studio/semteams/pull/304) own this slice. This is the `architect` read on the
read-API shape that task 1.2 required before implementation. SemTeams starts from `main` at `7f26ca7d` with
SemStreams frozen at `8b99efe9c66a4faa4fa509f9f62cc6bad8392128`; the design input is SemDev
`openspec/changes/semdev-workspace-ui/design.md` D1–D5 (design-only, 2026-09-15), adopted by the owner on
2026-10-08 consistent with [ADR-063](../../../docs/adr/063-development-pack-absorption-boundary.md).

Inspected, not executed: frozen `processor/agentic-dispatch/loop_info.go` (`LoopInfo` wire shape, `GET /loops`,
`GET /loops/{id}`), `processor/rule/actions.go` (`publish_agent` metadata and the `rule.task.spawned` back-reference),
`agentic/user_types.go` (`TaskMessage`), `agentic/agentrun` (`agent.run.phase` values), `service/graph_triples_http.go`
(`GET /graph/triples`); product `cmd/semteams/vocab/vocab.go` (run markers),
`configs/rules/ops/01-run-terminal-observe.json`;
UI `svelte.config.js` (adapter-node), `hooks.server.ts`, `routes/api/ai/*/+server.ts`, `lib/server/graphql/client.ts`,
`stores/{taskStore,runStatus,agentStore}.svelte.ts`, `types/{agent,task}.ts`, `Caddyfile.e2e`,
`docker-compose.agentic-e2e.yml`; donor `semdev/internal/vocab/vocab.go` (`run.issue.ref`).

**Result of the read:** no runtime type is missing on `main` for the fixture-backed slice. Two facts are not
written by any live pack (run-to-issue linkage; rule identity on a spawned task). Both are handled below without a
runtime change: the first as an explicit `unsupported` lookup; the second cannot be read on any surveyed path (measured
2026-10-08 on the running e2e stack, D3), so it renders unknown and the engine ask goes to #298. Task 1.3 does not
trigger.

## D1 — The read API is a SvelteKit server boundary; no new Go

The UI already runs a Node server (`@sveltejs/adapter-node`) behind Caddy, with server routes under
`ui/src/routes/api/` (`api/ai/chat`, `api/ai/generate-flow`), a server-side GraphQL client
(`lib/server/graphql/client.ts`) and `BACKEND_HOST` naming the backend for server-side fetches. Caddy's catch-all
sends `/api/*` to that server. The work read API lives there:

| Endpoint | Returns |
|---|---|
| `GET /api/work/portfolio` | `{ source, programs[] → projects[] → repositories[] }` from D4; `source` is `fixture`, `github` or `unconfigured` |
| `GET /api/work/items?repository=owner/name` | Work items for one configured repository with PM fields (D2), `lookup` status, pagination cursor |
| `GET /api/work/items/{owner}/{repo}/{number}` | One work item's overview: purpose, owner, priority, milestone, dependencies, delivery context |
| `GET /api/work/items/{owner}/{repo}/{number}/runs` | Linked runs with overlays (D2) and a `lookup` status for the linkage and for each run's facts |

Every list and lookup carries `lookup: "complete" | "partial" | "failed" | "unsupported"` plus a `reason` when not
complete. Overlay values are a discriminated union: `{ state: "known", value, reason? }`, `{ state: "none", reason? }`
or
`{ state: "unknown", reason }`; `none` is only emitted when the enclosing lookup is `complete`. Methods other than `GET`
answer 405. There is no endpoint that enumerates runs
outside a work item; run facts are fetched per linked run entity, so the browser cannot use the work API to walk the
graph. Repository scope is the D4 configuration: an unconfigured repository answers 404; an unconfigured work source
answers 503 `WORK_SOURCE_UNCONFIGURED` on item and run reads. The runs lens keeps its existing direct browser reads
(`/teams-dispatch/*`, `/graph/triples`, `/graphql` through Caddy); the work lens adds none, and narrowing those
existing routes is a separate hardening, not this slice.

Overlay facts are read server-side from the existing reads, never by the browser. Measured on the running stack
(2026-10-08, 8b99efe): `GET /teams-dispatch/loops` and `/loops/{id}` carry `loop_id`, `task_id`, `role`, `state`,
`iterations`, `max_iterations`, `outcome`, `user_id`, `channel_type` and `pending_approval`; they carry NO `prompt`,
`metadata` or `parent_loop_id`. The loop entity `<org>.<platform>.agentic-loop.agent.execution.<loop_id>` carries
`agent.loop.description` (the prompt, truncated upstream at 8 KiB), `agent.loop.role`, `agent.loop.run`,
`agent.run.entity-id`, `agent.loop.task`, `agent.lineage.root`, `agent.loop.outcome` and
`coordinator.decision.{next-action,reason}`. So: needs-you reads `/loops/{id}` (`state`, `pending_approval`) plus the
run markers; everything else reads `GET /graph/triples` by subject or by `predicate`+`object` (exact match, `limit`
applied inside the scan, so bindings use selective predicates). Each backend call failure degrades that lookup to
`partial` or `failed`, never to `none`.

Why not Go: every read the slice needs already exists on the frozen HTTP surface; a product-shell HTTP handler
would reimplement a boundary the Node server already has, and would need a framework-alignment review for no new
capability. The live GitHub source is a later slice: #273 owns the read adapter and the decision whether the Node
server calls GitHub with a server-held token or a product read endpoint fronts the Go adapter. The API shape above is
source-independent, so that decision does not change the browser contract.

Source selection is configuration (`WORK_SOURCE=fixture|github`, `WORK_FIXTURE_SET=<name>`,
`SEMTEAMS_PORTFOLIO_PATH=<file>`), read once by the server. Unset means `unconfigured`: the board renders a
"work source not configured" state, not an empty board.

## D2 — GitHub shapes for work items; existing runtime reads for overlays

A work item is `owner/repo#number` with title, body excerpt, open/closed state, labels, assignees, milestone,
optional Project status and priority fields, dependencies, and delivery context. GitHub owns all of it. Columns
follow the configured Project status field when D4 names one. The documented fallback, applied only from GitHub
fields and never from execution state: closed → Done; open with an assignee → In Progress; open without an
assignee → Todo. Priority and PM status without a configured source stay unset. Dependencies come from the issue
body's `Blocked by #n` / task-list references and are informational (SemDev D1). Delivery context (linked PRs) is
unknown unless the source supplies it; the fixture set supplies it for one item.

Overlays, each with its source on `main` and the condition that makes it unknown:

| Overlay | Source on `main` | Unknown when |
|---|---|---|
| Linked runs | Fixture mode: the item's declared `linked_runs` binding (D5). Live mode: the run-entity predicate `run.issue.ref` (donor name, one authority; no SemTeams writer yet) | Live mode on `main`: no writer is wired, so the lookup is `unsupported`, not empty. "No linked run" is shown only for a `complete` lookup |
| Execution stage | `agent.run.phase` on `*.*.chain.agent.execution.*`: `dispatched`, `executing`, `awaiting_approval`, `completed`, `failed`, `cancelled` | Run entity unreadable, predicate absent, or a value outside this list |
| Needs-you | Coordinator loop `state = awaiting_approval` or `pending_approval` present; run markers `agent.run.clarification-pending`, `agent.run.approval-outstanding > 0` | Loop or run read fails. "Park" has no SemTeams fact and is not rendered |
| Verification | No fact for research runs; the reviewer verdict is trajectory content | Always unknown in this slice; the development pack supplies it later |
| Attention / at-risk | Program Pulse (#267) | Absent until #267; never simulated |

PM status, execution stage and verification are three separately rendered values. A linked run never moves a card.

## D3 — Explained events from measured provenance; rule identity is unknown live

What the frozen engine records when a rule fires `publish_agent`, measured on the running stack: the firing entity
receives `rule.task.spawned = rule-<firing entity id>-<nanos>`; the spawned loop entity carries `agent.loop.task`
(that same task id), `agent.lineage.root` (the run instance, when the rule declares `related_loops.root`),
`agent.loop.description` (first line of its prompt) and, at the end, `coordinator.decision.{next-action,reason}`; the
run entity carries a `lifecycle.transition.{at,from,to,note,source}` history whose five triples per transition share
one triple `timestamp`. The trajectory's `loop.started` fact carries `source_kind: task` and `source_correlation` =
the task id, with the task body only as an evidence-store reference (#261). The rule's `properties` land in task
metadata, which no read path exposes: the loops REST drops it, no loop-entity triple carries it, and the trajectory
keeps the body in the evidence store. The rule's identity is therefore not readable anywhere on `main`.

Contract: an explained event is rendered from a loop whose `agent.loop.task` starts with `rule-`; the firing entity
is parsed from that task id (confirmed by `rule.task.spawned` on the firing entity and by `agent.lineage.root`). It
renders: the control identity (loop id, role); the firing entity; the firing entity's lifecycle transition that
precedes the spawn (`to` value and `source`, for example `agent.run.phase → completed`, source `rule`), labelled as
the fact at spawn; the outcome as the control loop's own state (accepted when the loop exists, applied when it
completes, rejected when it fails) with the terminal `decide` reason; and `rule: unknown` with the reason "rule
identity is not recorded by the frozen runtime (#298)". No pack-config change is made: carrying the rule id in the
ops rule's `properties` was withdrawn after measurement because nothing can read it, and stamping it from the pack
with a rule `add_triple` would be a product workaround for missing generic provenance. The generic ask, that the
engine stamp rule identity and the firing fact on the spawned loop entity, is posted on #298 with these
measurements.

Runs-lens change this requires: `taskStore` reads `agent.loop.task` and `agent.lineage.root` for rule-spawned loops
through `GET /graph/triples` (the runStatus store already polls triples by predicate), attaches each to the
coordinator card of its firing run as a control instead of rendering it as a top-level card as it does today, and
`TaskStory` renders controls as one narrative row each in the shape above. The ops journey selects observers by role,
so it is unaffected; task 4.2 verifies it.

## D4 — Portfolio configuration: the minimal read-side shape shared with #267

A JSON document (example at `configs/portfolio/portfolio.example.json`, hand-authored schema beside it; `schemas/` is
generated and stays untouched):

```json
{
  "programs": [{
    "id": "c360", "name": "c360",
    "projects": [{
      "id": "semteams", "name": "SemTeams",
      "repositories": [{
        "owner": "C360Studio", "name": "semteams",
        "project_board": { "owner": "C360Studio", "number": 3, "status_field": "Status", "priority_field": "Priority" }
      }]
    }]
  }]
}
```

Project membership is operator-authored; a repository can appear under more than one project and is never inferred
from activity (`docs/product/program-manager.md`). `project_board` is optional; without it the D2 fallback applies.
When a board is configured the source supplies the ordered status options (GitHub's single-select options; the fixture
declares them), and the items response returns
them as `columns` so empty columns still render.
The server validates required fields and refuses to start the work API on an invalid document (the board shows the
validation error). #267 and #273 read the same file from Go; a Go struct is theirs to add. Fixture sets carry their own
portfolio document.

## D5 — UI composition, fixtures and the journey

Route `/work` with `?view=board|table` and `?item=owner/repo%23n` for the overview panel, mirroring the runs lens's
`?task=` URL state; a "Work" link in `TopNav`. Components under `ui/src/lib/components/work/` (`WorkBoard`,
`WorkTable`, `WorkItemCard`, `WorkItemOverview`, `OverlayBadge`), a `workStore.svelte.ts` fed by `services/workApi.ts`
over the D1 endpoints, and `types/work.ts`. Cards carry no drag affordance at all; the read-only rule is structural,
not a disabled handler. Drill-in navigates to `/?task=<coordinator loop id>`, the existing runs lens.

Fixture set `board-mvp` under `ui/src/lib/server/work/fixtures/board-mvp/`: two repositories, one with a Project
status field and one without; items covering every column; one item with linked PRs as delivery context; and three
linkage cases: an item bound to a real run, an item declaring no runs (`none`), and an item bound to a run entity
that does not exist (a real `partial` lookup, not a simulated one). Fixture-mode binding is declared on the item as
`linked_runs: [{ by: "run_entity_id", value }]` or `[{ by: "coordinator_prompt", equals }]`; the prompt form resolves
against the coordinator loop entity's `agent.loop.description` triple (exact match) and exists only so a journey can
bind a run it creates at run time. Both forms are fixture-only; live linkage is `run.issue.ref` (D2). A declared binding
that resolves to nothing is a
`partial` lookup, never "no linked run"; only an empty `linked_runs` list is. In fixture mode a missing `linked_runs`
key means none (the fixture is the source of truth), while a missing `pull_requests` key means unknown delivery
context. Every `GET /graph/triples` read checks for truncation at its `limit` and degrades to `partial` when hit;
that endpoint is a full scan on the frozen backend, so the work lens loads runs only for the opened item.

Journey (`ui/e2e/agentic/work-board.spec.ts`, on the `chain-drill-in` template): start a research run through the chat
exactly as `chain-drill-in` does and wait for the ops observer to complete; open `/work`; assert the board renders
every fixture column with PM status, execution stage and verification as separate badges, with run-derived badges
unknown ("linked runs are read when an item is opened") until an item's runs are loaded; open the `none` item, load
its runs and assert "no linked run"; open the `partial` item, load and assert unknown with its reason; open the
bound item, load its runs and drill into the run; assert the
story shows the ops control as one explained row with `agent.run.phase` and its value visible and the rule shown as
unknown with its reason; assert the table
view renders the same items; assert `POST /api/work/items` answers 405.

Source map, in the SemDev D5 form:

| Product information | Source on `main` | Availability |
|---|---|---|
| Work items and PM metadata | GitHub issues and Project fields | Fixture now; live behind the same API from #273 |
| Linked runs | Fixture binding; `run.issue.ref` later | `unsupported` live until a writer exists |
| Coordinator loop and needs-you | `GET /teams-dispatch/loops/{id}`, run markers | Available |
| Execution stage | `agent.run.phase` via `GET /graph/triples` | Available |
| Rule-fired control | Spawned loop entity triples, `rule.task.spawned`, firing-entity lifecycle history | Available; rule identity unknown (#298) |
| Verification, attention, evidence bodies | Development pack, #267, #261 | Unknown or absent in this slice |

## Alternatives rejected

- A product-shell Go read handler: duplicates a boundary the Node server already provides; triggers the
  framework-alignment review for no new capability; defers to #273 for the live source.
- Serving fixture runs instead of real runs: the runs lens reads live loops and trajectories, so a fixture run could
  not be drilled into; the journey creates a real run and binds it.
- Simulating the `partial` case with a fixture flag: a dangling run binding exercises the real failure path instead.
- Deriving PM status from execution state: violates SemDev D2 and would move cards when runs move.
- Carrying rule identity through the ops rule's `properties`: they land in task metadata that no read path exposes;
  withdrawn after measurement. Stamping it from the pack with `add_triple`: a product workaround for missing generic
  provenance; the ask belongs to the engine (#298).

## Follow-ons (issues, not tasks here)

`run.issue.ref` writer for SemTeams runs (development pack intake, #290/#291; or #273's intake); engine stamping of
rule identity and the firing fact on spawned loop entities (#298, measured gap posted there); `github` source mode
(#273); attention overlay (#267); evidence bodies (#261).
