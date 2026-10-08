# Deliver the read-only work board and work-item overview

## Why

[Issue #296](https://github.com/C360Studio/semteams/issues/296) owns the first observable-work slice of
[ADR-061 §3](../../../docs/adr/061-agent-runtime-ownership-and-observable-work.md): the operator opens SemTeams and
sees all work across the configured portfolio before any run exists. Today the UI starts from runs; work that has no
run, or whose run ended, is invisible, and the account of why a run moved lives only in trajectories. The owner ruled
on 2026-10-08 that this fixture-backed, read-only board is the first thing built while SemEngine is ported, with the
SemDev `semdev-workspace-ui` design (D1–D5, 2026-09-15, design-only) adopted as the design input, consistent with
[ADR-063](../../../docs/adr/063-development-pack-absorption-boundary.md).

This change starts from `main` at `7f26ca7d` (frozen SemStreams `8b99efe9c66a4faa4fa509f9f62cc6bad8392128`). [Draft PR #304](https://github.com/C360Studio/semteams/pull/304), on `claude/gh296-work-board`, owns this claim.

## What changes

- Add a **work lens** beside the existing runs lens: a board and a table whose cards are GitHub issue-backed work
  items across the configured repositories, grouped by project, with a work-item overview (purpose, owner, priority,
  milestone, dependencies, delivery context) and linked specs, runs and evidence on request.
- Columns follow GitHub Projects status when a Project is configured; the fallback is Todo / In Progress / Done with
  unknown metadata unset. GitHub owns identity, body, assignees, labels and milestone; priority and PM status cite
  the configured field or a documented mapping.
- Overlay graph-derived state with an explicit unknown value for each overlay: linked runs, execution stage from
  `agent.run.phase` and existing facts, needs-you (`ask_user`, approval, park). Attention/at-risk from Program Pulse
  stays absent until #267 lands; it is never simulated.
- Drill-in reaches the existing runs lens (`KanbanBoard`, `TaskDetailPanel`, `TaskStory`, `TrajectoryViewer`), which
  renders rule-fired controls as explained events: the rule, the firing fact and value, the control identity and its
  outcome (#298). The fixture set includes the ops observer firing on `agent.run.phase`.
- Land the minimal read-side portfolio configuration (program → project → repository) shared with #267, and a product
  read API with repository and run scoping as the only browser-facing boundary. Fixtures back GitHub enumeration and
  run facts; live enumeration arrives behind the same API from #273.
- Add one Playwright journey on the `chain-drill-in.spec.ts` template: the board renders from fixtures, drill-in
  reaches the existing run story, and unknown states render as unknown.

The `architect` read on the read-API shape precedes implementation and produces `design.md`. If the read API needs
runtime types that do not exist on `main`, the change stops at the design and says so on the issue.

## Non-goals

No write actions: no filing issues, no label or milestone edits, no approving a change or PR, no starting, approving
or completing a run. No drag changes a column. No live GitHub enumeration (#273), no evidence-body rendering beyond
the #261 limit, no Program Pulse findings (#267), no dev-pack activation (#290, #291), no new runtime component, rule
action, payload type, KV bucket or stream without a framework-alignment review. The browser never connects to NATS
and never holds admin credentials. Feature parity with Enjoy, HumanLayer, Pi or GitHub Projects is not a goal;
Enjoy is the UX reference for per-project grouping and needs-you state only.

## Impact

A new `observable-work` capability specification, Svelte components under `ui/src/lib/components/`, fixture files,
one Playwright journey, and the minimal read-API surface the architect read admits. Existing journeys
(`research-mvp`, `chain-drill-in`, approval and ask-user) keep passing with no new ESLint, svelte-check or revive
warnings. Research and autoresearch remain the only live product-facing packs; Program Pulse remains a target. No
release milestone membership or issue closure is implied; #296 stays open until the owner closes it.
