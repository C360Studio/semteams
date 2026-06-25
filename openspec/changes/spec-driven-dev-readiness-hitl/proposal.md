# Proposal: Spec-Driven Development Readiness and HITL Review

## Intent
SemTeams should support spec-driven development without porting SemSpec's private planning and execution state
machines. The coordinator should create or ingest OpenSpec artifacts, prove the execution environment is ready before
coding, route approved work through the existing dev-via-test primitive, and give humans a clear review surface when the
run should not be fully autonomous.

## Scope
In scope:
- OpenSpec change creation and brownfield OpenSpec ingest as the bridge standard.
- Graph-backed spec facts as the authoritative state, with OpenSpec markdown as renderable interchange.
- Proof-dependency analysis and readiness gating before implementation tasks are released.
- `dev_from_task` execution that reuses the existing Ralph/CBG dev-via-test path without re-planning the spec.
- Human review, edit, approval, rejection, and waiver surfaces for specs, proof gates, and release gates.
- Export of generated OpenSpec artifacts so users can hand the spec to another implementation tool or team.
- Slash-command shortcuts for spec export, implementation, run status, evidence review, and approval control.
- UI status surfaces that answer whether a run is working, waiting, blocked, failing, or complete.
- Real-LLM validation that starts with Gemini through the model registry, plus Playwright for full e2e journeys.
- A demo evidence pack that separates black-box product journeys from fixture-seeded bridge proof.
- A MAVLink-hard spec-production journey that proves SemTeams can author a strong handoff before claiming implementation.

Out of scope:
- Porting SemSpec's `PLAN_STATES` / `EXECUTION_STATES` manager model.
- Making BMAD a runtime dependency.
- Making MCP-based handoff a required MVP capability.
- Importing Claude/Codex-style markdown skills as live SemTeams capability packs.
- Making OpenAI or Anthropic the default paid smoke provider.
- Requiring every organization to run spec-driven development fully unattended.
- Treating slash commands as a separate workflow engine or control plane.
- Turning SemTeams into a general DevOps platform or cluster scheduler.
- Counting direct NATS or graph seeding as black-box evidence for product behavior.
- Claiming MAVLink-hard implementation is sponsor-ready before the required harness profile and readiness records exist.

## Approach
Model the workflow as governed graph facts plus category rule packs. A `create_change` journey produces a reviewable
OpenSpec change and `change.<slug>.*` facts. A proof-readiness analyzer emits typed findings and readiness status. Rules
route missing proof dependencies to the test-harness team, approved and ready tasks to `dev_from_task`, and ambiguous or
risky steps to human review. NATS remains the reactive transport, but SemStreams flow, component, payload, port, rule,
tool-governance, and lifecycle contracts remain the control surface. The UI presents the current artifact, gate,
evidence, and next action instead of asking operators to infer state from raw logs. Approved artifacts can be exported
as standard OpenSpec files even when SemTeams does not perform the implementation itself.

Black-box demo evidence must enter through public UI, dispatch, slash-command, approval, export, or documented task
surfaces. Graphs, trajectories, logs, and downloads are valid read-side evidence, but direct writes into NATS, graph
storage, lifecycle state, or proof facts are fixture seeding and must be labeled as bridge proof rather than black-box
MVP evidence.

## Future Roadmap

### Skill Import Adapter
Many users ask how to add a "skill" because Claude Code, Codex, and adjacent harnesses use that word for markdown
guidance. SemTeams should explain the difference clearly: a SemTeams capability is a governed pack of persona guidance,
rules, tool permissions, schemas, tests, and UI surfaces, not prose alone.

A future import adapter MAY accept a trusted `SKILL.md`-style document as source material, classify its workflow,
tools, artifacts, approval boundaries, and evidence expectations, then draft an OpenSpec change for human review. Only
after review and approval would SemTeams materialize a category pack, rule pack, persona fragments, or tool-governance
changes. Imported markdown is source material, not executable authority.
