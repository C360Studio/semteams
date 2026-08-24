# SemTeams Vocabulary Map

Status: product vocabulary guidance, reconciled to beta.160 on 2026-08-24.

This document defines the public words SemTeams should use for the agentic-dev
experience. It does not rename internal predicates, payloads, ports, or rule
actions. The goal is simpler: make the UI and product docs sound like the
agentic-dev world users are already learning, while keeping SemTeams'
SemStreams-native architecture intact.

## Why this exists

SemTeams already has most of the layers that current agent harnesses expose:
agents, tasks, handoffs, gates, evidence, traces, memory, HITL approvals, and
run state. The sponsor-facing problem is usually not missing primitives. It is
the translation tax: users see "skills", "workflows", "task graphs", "world
models", or "guardrails" elsewhere, then have to map those ideas onto
SemTeams-specific labels like rule packs, lifecycle facts, trajectory, Ralph,
CBG, proof readiness, or SKG.

Use this map when naming UI tabs, docs sections, slash commands, OpenSpec
exports, and sponsor-facing diagrams. Keep lower-level SemStreams nouns in
developer docs where their precision matters.

## Source Vocabulary

This map is informed by:

- BuildAHarness / FlowSpec:
  <https://github.com/3IVIS/buildaharness>
- OpenAI Agents SDK:
  <https://openai.github.io/openai-agents-python/>
- LangGraph:
  <https://docs.langchain.com/oss/python/langgraph/overview>
- CrewAI:
  <https://docs.crewai.com/en/concepts/agents>,
  <https://docs.crewai.com/en/concepts/tasks>,
  <https://docs.crewai.com/en/concepts/crews>,
  <https://docs.crewai.com/en/concepts/flows>
- Microsoft Agent Framework:
  <https://learn.microsoft.com/en-us/agent-framework/overview/>

These systems do not agree on every term, but there is a stable center of
gravity: agent, workflow or flow, task, handoff, guardrail or gate, memory,
state, trace, tool, and human-in-the-loop.

## Naming Policy

Use an industry term publicly when it is accurate enough.

Use a SemTeams-specific term publicly only when it is part of the product's
actual differentiation, such as OpenSpec export, proof readiness, or governed
SKG evidence.

Keep implementation nouns in technical docs:

- SKG
- rule pack
- persona fragment
- payload
- port
- lifecycle fact
- NATS stream
- projection contract
- SemStreams component

Do not rename graph predicates or payload fields just to improve product copy.
Prefer UI labels, display-name maps, and docs language over churn in stored
state.

## Canonical Public Terms

### Harness

The configured agentic system that can run work, prove state, and involve
humans. Internally this is the SemTeams product shell over the SemStreams
substrate. Use it in sponsor docs; do not introduce it as a new runtime
component.

### Workflow / Flow

A user-visible job path. The live beta.160 examples are research and
autoresearch; spec authoring and implementation remain parked. Internally this
is a category rule pack plus persona bundles. Avoid exposing "flow config"
unless discussing SemStreams internals.

### Run

One execution instance with status, artifacts, evidence, and trace. Internally
this is loop lineage, chain entity, lifecycle facts, and artifacts. This should
be the primary UI noun.

### Agent

A role executing a bounded step of a run. Internally this is an agentic loop
plus role, persona, and tool allowlist. "Coordinator" is one agent role with
special authority.

### Coordinator

The front-door router and done-authority for a run. Internally this is the
coordinator persona plus the closed `decide()` taxonomy. "Manager" and
"supervisor" can explain it, but Coordinator is canonical.

The Coordinator also supports ordinary chat. Users can ask how SemTeams works,
refine an idea, or ask which team fits before starting a run. Slash commands
are team hints to the Coordinator, not direct execution bypasses.

### Task

A unit of work with acceptance evidence. The current live packs use run and
loop state; `plan.task.*` and dev-from-task are parked implementation
vocabulary, not a live beta.160 route.

### Plan / Task Graph

The ordered set of tasks, gates, and dependencies for a spec or run. Internally
this is `change.*`, `plan.task.*`, and dependency facts. Prefer "Plan" in
compact UI, and "Task Graph" in deeper views.

### Handoff

A transfer of work or responsibility between agents. Internally this is a
rule-spawned loop with lineage and related loops. This is a good fit for run
trace views.

### Gate / Guardrail

A check that can pass, fail, block, or require approval. Internally this covers
proof readiness, formal claims, approval filters, and CBG. Use "Gate" when
proof or evidence decides progress.

### Evidence

Artifacts or observations that justify a claim or status. Internally this is
evidence triples, logs, files, metrics, and test output. Evidence is central to
the "is this thing working?" UI.

### Artifact Context

An emitted artifact attached to a later chat prompt. This remains the preferred
label for the intended feature, but it is **not live on beta.160**: GraphQL
trajectory facts expose previews and `StorageReference` values, not the
evidence bodies `ArtifactCard` and the context chip require. Restore the label
to product UI only after issue
[#261](https://github.com/C360Studio/semteams/issues/261) lands its authorized
evidence-fetch change.

### Trace

The ordered history of a run. Internally this is trajectory, events, tool calls,
and graph facts. Use "Trace" publicly and "trajectory" internally.

### Run State / World Model

The current set of facts the harness believes about the run. Internally this is
governed SKG facts, lifecycle facts, and ownership registry state.
BuildAHarness' "world model" is good explanatory language.

### Memory

Persisted context available across steps or runs. Internally this is graph
facts, artifacts, object-store entries, and persona context. Avoid implying
opaque chat memory is the only form.

### Capability Pack

A reusable bundle that teaches SemTeams a job class. Internally this is a
category rule pack, persona fragments, and tool allowlists. This is a better
public label than "rule pack".

### Approval

A human decision required before progress continues. Internally this is HITL
approval state and approval-required tools. The UI should show what will happen
if the human approves.

### Export (parked)

A portable artifact generated from SemTeams state. The OpenSpec renderer and
spec-export surface remain on disk with the parked spec/dev packs and are not a
live beta.160 front-door capability. MCP export remains future work.

## Parked Spec/Dev Term Map

The following labels describe retained, unwired surfaces. They are migration
vocabulary, not the current command or capability inventory.

- `create_change` -> Spec Builder.
  UI: Specs / New Spec. Avoid saying `create_change` in product UI. The user
  wants the artifact, not the action token.
- OpenSpec change facts -> Spec / Change.
  UI: Specs. Avoid saying "graph hydration". "Spec" is the user's mental model;
  graph authority stays in docs.
- `change.<slug>.*` -> Spec Plan.
  UI: Specs / Plan. Avoid saying "change triples". This shows the approved
  planning artifact.
- `proof_readiness` -> Readiness Gate.
  UI: Run Health / Gates. Avoid saying `proof_readiness.route`. Users need
  pass, block, waiver, and next-action state.
- `formal_claims` -> Claim Analysis.
  UI: Gates / Evidence. Avoid saying `formal_claims`. Explain what is being
  proved.
- `dev_from_task` -> Implement Task.
  UI: Tasks / Run action. Avoid saying "dev-from-task". This is an execution
  action, not a planning method.
- artifact handoff -> Artifact Context (beta.160 regression).
  Future UI: Chat input / Artifact card. Do not show it as live until an
  authorized evidence-fetch path restores evidence bodies.
- Ralph -> Implementation Agent.
  UI: Trace / Tasks. Avoid saying "Ralph loop". Keep Ralph in developer docs
  and lore.
- CBG -> Final Review Gate.
  UI: Gates / Review. Avoid saying "CBG". Public UI should name the job, not
  the codename.
- trajectory -> Trace.
  UI: Runs / Trace. "Trace" is common across agent frameworks.
- category rule pack -> Capability Pack.
  UI: Settings / Advanced. Avoid saying "rule pack". Public users think in
  capabilities and workflows.
- persona fragment -> Agent Instructions.
  UI: Settings / Advanced. Avoid saying "fragment". "Instructions" maps to
  skills and prompts users know.
- SKG -> Run State / Evidence Graph.
  UI: Advanced / Evidence. Use SKG only when explaining SemTeams'
  differentiator.
- rule engine -> Automation Rules.
  UI: Advanced. Avoid saying "rule processor". Product labels should describe
  effect.
- payload / port -> Message Contract.
  UI: Developer docs. Correct but too framework-shaped for regular UI.

## BuildAHarness Terms Worth Borrowing

BuildAHarness is useful as vocabulary and product framing, not as a target
architecture. SemTeams should not adopt FlowSpec as a new control plane, because
SemStreams already owns flow execution and OpenSpec is the spec interchange
standard.

Borrow these layer names when explaining SemTeams:

- World Model: what the run currently believes, backed by governed SKG facts.
- Evidence and Reasoning: observations, artifacts, metrics, logs, and claim
  links.
- Hypothesis: useful mostly for autoresearch, where each iteration tests one
  measurable idea.
- Diagnostics: ops-agent findings and run-health explanations.
- Control State: coordinator/rule-derived phase, gate, and next-action state.
- Task Graph: the ordered task set from an approved OpenSpec change.
- Verification Gate: readiness, tests, final review, and acceptance checks.
- Recovery: bounded retry, blocked state, or human escalation.
- Reviewer Pass: human or agent review before integration.
- Output Contract: the approved spec, scenarios, task constraints, and done
  authority.
- Experience Store: future cross-run learning; do not imply it is MVP unless
  the storage and governance contracts exist.

Do not borrow these as MVP direction:

- Visual no-code canvas as the primary SemTeams surface.
- FlowSpec as an interchange standard for SemTeams specs.
- A second state machine beside SemStreams rules, lifecycle, and ownership.
- Silent skip behavior for unavailable proof tools.

## UI Labeling Guidance

The run UI should always answer one question first: "Is this thing working?"

Every run view should make these visible without digging:

- Current status: running, blocked, awaiting approval, failed, completed.
- Current gate: what must pass before the run can continue.
- Last progress: timestamp, agent, and action.
- Next action: what SemTeams will do next or what the human must decide.
- Evidence health: tests, metrics, logs, proof dependencies, and waivers.
- Cost/risk health: token budget, retry count, and autonomous-policy limit.

Recommended top-level UI nouns:

- Runs
- Specs
- Tasks
- Evidence
- Trace
- Gates
- Approvals
- Exports
- Settings

Use Prometheus and other operational metrics as run-health signals, not as a
separate observability island. If the metric answers whether the run is making
progress, blocked, or burning budget, surface it in Run Health.

## Slash Commands

Slash commands are power-user shortcuts for existing coordinator and UI actions.
They must not become a second control plane or a hidden workflow language.
Product-level team commands are intent hints carried to the coordinator; they do
not bypass routing, sandbox checks, readiness, approvals, review, or
clarification.

The current ChatBar inventory is deliberately smaller than the retained
`slashCommands.ts` registry.

Live front-door team hints (no task selected):

- `/research`: ask the coordinator to validate and route an evidence or
  comparison prompt.
- `/optimize` (registry alias `/autoresearch`): ask the coordinator to validate
  a measurable optimization prompt and sandbox target.

Governed controls when a task is selected:

- `/approve`
- `/reject`
- `/pause`
- `/resume`
- `/cancel`

Parked or non-surfaced commands are not current product claims. This includes
`/create-change`, `/spec`, `/dev-via-test`, `/implement-spec`, `/export-spec`,
`/run-status`, and `/evidence`. Some definitions remain in the broader command
registry or backend command bridge for compatibility and future migration, but
the ChatBar does not advertise them as live teams or run actions.

Avoid commands that expose internal roles or merge planning and execution:

- `/lisa`
- `/ralph`
- `/cbg`
- `/reviewer-dev-via-test`
- `/do-everything`
- `/auto-implement`

Future spec/dev restoration should still prefer separate, auditable actions:

- chat with the coordinator to shape the idea
- ask for a team with a public team command
- create or edit the spec
- approve the spec
- implement an approved spec or selected task
- review evidence
- export the spec

## Skills And Capability Packs

Users will ask how to add a "skill" because that term is common in other
harnesses. SemTeams should treat skill as an import and authoring concept, not
as the primary runtime primitive.

Suggested wording:

> A SemTeams capability pack is the runtime form of a skill: rules, agent
> instructions, tool permissions, and message contracts packaged for one job
> class.

Future roadmap:

- Import a Markdown skill from Codex, Claude, or another harness.
- Analyze the instructions, tools, constraints, and output contract.
- Generate a draft SemTeams capability pack.
- Require human review before enabling the pack.

This is not MVP for spec-driven dev. It is useful roadmap language because it
explains how SemTeams differs: prose skills can hydrate a governed flow/rule
pack, but the running system is still graph, rules, tools, ports, and evidence.

## OpenSpec Export Language

OpenSpec artifacts are intended to be portable deliverables, but their
author/review/export route is parked on beta.160 with the spec/dev packs.

Reserve "Export Spec" for the restored UI action; do not present it as a live
command or MVP capability today.

Keep MCP export as a future integration path. It could be useful when another
agent wants to pull the current approved spec directly, but it is not required
for the first product value:

- render OpenSpec from graph state
- let the human review and edit
- export the artifact
- optionally implement tasks inside SemTeams later

## Working Rule

When in doubt, name the user-facing thing by the question the user is asking:

- "What is being built?" -> Spec
- "Is it ready to build?" -> Readiness Gate
- "What is happening now?" -> Run
- "Why do we believe it?" -> Evidence
- "How did we get here?" -> Trace
- "Who decides done?" -> Coordinator / Final Review Gate
- "Can I take it elsewhere?" -> Export

Everything below that line can keep the precise SemStreams and SemTeams
implementation vocabulary.
