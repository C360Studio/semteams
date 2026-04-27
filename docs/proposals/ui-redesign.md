# SemTeams UI Redesign

**Status:** In progress · 2026-04-27

## Why

The current UI was lifted from semstreams-ui — a flow-builder/data-viz
tool — and never restructured for what SemTeams actually is. Three equal
top-nav tabs (Board / Graph / Flows) treat all three as first-class
destinations. That's correct for a flow-builder. SemTeams is a different
product: users delegate work to a team of agents and watch them do it.
Most users will never build, edit, or even view a flow.

## What SemTeams is, in one sentence

An inbox-and-workboard for delegating work to a team of agents — where
"agent" is something between a coworker and a function call.

## Use cases, in priority order

| # | Use case | Frequency |
|---|---|---|
| 1 | **Ask** — kick off work with a goal (prose or slash command) | every session |
| 2 | **Glance** — see what's running, done, and needing me | every session |
| 3 | **Approve / reject** — one click on a Needs-You loop | per task |
| 4 | **Inspect** — trajectory, messages, sub-loops, persona, entities | when curious or stuck |
| 5 | **Find past work** — search completed tasks | weekly |
| 6 | **Diagnose** — ops view: endpoint health, stuck rules | when paged |
| 7 | **Admin** — personas, model endpoints, allowlisted tools | rare |

1, 2, 3 are 80%+ of the user's day. Everything else is contextual or
admin work.

## Layout

VS-Code-shaped three-column with both rails collapsible. Fixed content
per rail (no user customization). Top-anchored ask bar.

```
┌─ ☰ ──────────────────────────────────────────── search · me ──┐
│                                                                │
│              ┌─────────────────────────────────────────┐      │
│              │ What should the team do?           ↩    │      │
│              │ @-mention to attach context             │      │
│              └─────────────────────────────────────────┘      │
│   LEFT RAIL  │ [Research] [Plan] [Implement] [Approve ▸2]    │
│   ☰ toggle   │                                                 │
│              ├─────────────────────────────────────────┤      │
│              │  Workboard                                │      │
│   Pinned     │  Thinking 1   Executing 0   Needs You 2  │     │
│   Recent     │  Done 4       Failed 0                   │     │
│   Personas   │  ────────────────────────────────────    │     │
│   Admin ⚙    │  [card] [card]                           │      │
│              │  [card]                                   │     │
│              │                                           │     │
│              └─────────────────────────────────────────┘      │
│                                                                │
│                                       RIGHT RAIL (toggle ☰)   │
│                                       contextual panel        │
└────────────────────────────────────────────────────────────────┘
```

**Ask bar**: anchored at the top of the work area, always visible
during scroll. Primary interaction. `@`-mention attaches context.
Action chips below it are persona-shaped (Research / Plan / Implement
/ Approve next ▸N) — clicking equals typing `@research` etc.

**Left rail**: fixed-content navigation. Pinned tasks, Recent, Personas
(as `@`-mention quick-attach), Admin gear at the bottom. Collapsible.

**Center**: workboard always present. Default view: kanban with column
toggles, count chips. Columns are visibility-toggleable but not
re-orderable (we keep the URL surface stable).

**Right rail**: contextual.
- *No selection* → Latest ops diagnoses · Endpoint health · Recently
  completed task summaries.
- *Task selected (`?task=#42`)* → Task drilldown: header (title, status
  pills, elapsed), and tabs:
  - **Activity** — trajectory + messages timeline
  - **Trace** — runtime NATS message sequence for this task across
    components (the right replacement for the broken pipeline-shaped
    flow viz; semstreams is event-driven, not pipeline-shaped)
  - **Entities** — scoped graph of entities this task's loops touched
    (port the working semdragons sigma component)
  - **Logs** — raw message-logger view, filtered

## Naming

Three layers, all interlocked.

### Title (every task)

Source: **first ~60 chars of the original prompt** at task creation.
Cheap, works offline, no LLM call. Editable inline on the card and the
detail header (click pencil → text input → save). Matches Claude
Desktop's pattern of auto-titling with later edit.

### Short ref (every task)

GitHub-style `#42` — per-deployment monotonic counter. Resolves to the
canonical `loop_id`. Backed by a NATS KV counter owned by the dispatch
component. The ref is stable for all time; the title can change.

### Synonyms (user-added)

Users can attach aliases to a task: `task.aliases: string[]`. The
`@`-mention resolver searches across `loop_id`, short_ref, title, and
aliases — fuzzy. So `@mqtt`, `@42`, `#42`, `MQTT vs NATS` all resolve
to the same task. Aliases editable from the task detail panel.

### Sub-entities

Loops, sub-loops, agents, entities show in the UI as `#42 → researcher`
or `#42 → step 2`, never as raw UUIDs. Full ids are hover-revealed for
copying and exposed in logs/trace views where they actually matter.

**Implication**: in the user-facing UI, **everything top-level is a
"task"**. Loops, coordinators, researchers, etc. are implementation
details surfaced only in drilldown.

## What's NOT in the new UI

### Top-level "Graph" tab

Removed from primary nav. The working semdragons graph viz lives in
the Entities tab of a task drilldown, scoped to that task's entity
subgraph. A standalone full-graph view, if ever needed, lives at
`/admin/graph`.

### Top-level "Flows" tab

Removed from primary nav. SemTeams stops trying to be a flow editor.
Two reasons:
1. The 3-panel canvas-with-properties layout was stapled on from
   semstreams-ui and never integrated visually.
2. Flow viz is genuinely hard for semstreams' actual shape: it's not
   a pipeline DAG, it's overlapping flows on a shared NATS subject
   bus. A single static topology diagram lies about runtime.

What replaces it:
- **Task-level Trace tab** for "what messages flowed for this task" —
  the runtime story is the truer answer.
- **Agent-proposed flow changes** (ADR-027 Phase 2) surface as a JSON
  diff + an "Open in flow editor" deep-link. If a semstreams-ui
  deployment is pointed at the same backend, the link opens there.
  Otherwise, just the diff (still useful for review/approval).
- **Power users** who want a flow editor can deploy semstreams-ui
  alongside semteams, both targeting the same backend. SemTeams owns
  the workboard; semstreams-ui owns the flow editor. Different
  products, different jobs.

### `flow-builder` HTTP endpoints

Stay enabled in the backend (we already wired this in
`e855051`) — they're consumed by the agent-proposed-flow surface and
by the optional sibling semstreams-ui deployment. Just not visible in
the SemTeams UI's primary nav.

## Routing

| Route | Purpose |
|---|---|
| `/` | Workboard. Right rail toggles by `?task=` |
| `/?task=#42` | Workboard with task #42 drilldown in right rail |
| `/admin` | Admin landing |
| `/admin/personas` | Persona CRUD (advanced) |
| `/admin/endpoints` | Model endpoint config + health (beta.15) |
| `/admin/tools` | Tool allowlist + governance |
| `/admin/flows` | (optional) Existing flow editor, gated |

Real route family for `/admin/*` rather than `?admin=1` — forward-
compat with multi-team / multi-tenant scoping later.

## Migration plan, in order

1. **Cleanup nav** (this commit batch). Remove Graph and Flows from the
   top nav. Both stay accessible at their current URLs for power users
   in this transitional period; they just don't show as primary tabs.
2. **Move ChatBar to top.** Anchored under the brand bar, full-width
   center column. Footer slot freed.
3. **Naming layer 1.** Add task title (first-N of prompt) to TaskInfo,
   show it on cards instead of the task_id. Inline edit deferred.
4. **Naming layer 2.** Add `#N` short ref counter to dispatch (NATS KV
   sequence), show alongside title. Resolver in `@`-mention.
5. **Right rail drilldown.** Click card → right rail expands with
   tabs. Activity tab first (trajectory + messages); Trace, Entities,
   Logs follow.
6. **Action chips.** Persona-shaped buttons under the ask bar.
7. **Inline title edit + aliases.** Pencil-to-edit; alias editor in
   detail panel.
8. **`/admin/...` route family.** Stub admin landing + move existing
   flow editor to `/admin/flows`.
9. **Trace tab.** Message-logger filtered to this task's subjects.
10. **Entities tab.** Port the working semdragons graph viz.
11. **Approval queue.** "Approve next ▸N" wires through Needs-You
    loops in priority order.

Each step is independently shippable and reversible. Order optimizes
for "remove dead weight first, then build replacements."

## Open questions for later

- **Title via LLM** as an opt-in toggle (haiku call, ~50ms). Worth a
  config knob once we have step 3 done.
- **"Search past tasks"** — full-text against task titles + results.
  Likely needs a dedicated KV index in dispatch.
- **Multi-user / team scoping** — when a deployment hosts multiple
  users, do task counters reset per-user or stay global? Punted.
- **Mobile / narrow viewports** — both rails collapse, ask bar stays.
  Detailed treatment deferred.
