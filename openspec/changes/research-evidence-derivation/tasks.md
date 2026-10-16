# Tasks: `research-evidence-derivation` (#271)

- [ ] 1. Extend `research.Actor` and `research.IntegrationPoint` in `cmd/semteams/research/artifact.go` with required `evidence []EvidenceRecord`, citation validation (`revision <= artifact.Revision`), and markdown provenance rendering in `cmd/semteams/research/render.go`.
- [ ] 2. Implement the mechanical derivation check and `ToolResultResolver` in `cmd/semteams/tools/emitartifact/` and wire the NATS-backed resolver in `cmd/semteams/product_tools.go`.
- [ ] 3. Update the `emit_research_artifact` tool schema and regenerate `schemas/emit_research_artifact.json` (if applicable) and `specs/openapi.v3.yaml` via `task schema:generate`.
- [ ] 4. Update `configs/personas/fragments/researcher-research-gather/`, `researcher-research-synthesize/`, and `reviewer-research/` for typed evidence handoff, citation synthesis, and reviewer evaluation.
- [ ] 5. Support optional deterministic `tool_call.id` in `test/e2e/mock/` and update `test/fixtures/journeys/research-mvp.yaml`, `research-fanout.yaml`, and `ops-run-terminal.yaml` plus unit and contract tests in `cmd/semteams/research/`, `cmd/semteams/tools/emitartifact/`, and `test/contract/`.
