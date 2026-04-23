# SemTeams Project Context

SemTeams is the reference/demo product for agentic teams built on the
[semstreams](https://github.com/c360studio/semstreams) framework. It has
**no custom Go components** — every processor comes from semstreams via the
`github.com/c360studio/semstreams` Go module dependency. The product is the
Svelte UI, the flow-template config library, and the docs.

## Tech Stack

- Go 1.25 — thin `cmd/semteams` binary that wraps semstreams'
  `componentregistry.Register`
- Go module: `github.com/c360studio/semstreams` (currently `v1.0.0-beta.8`)
- NATS JetStream (KV, ObjectStore), Prometheus, slog — via semstreams
- Task (task runner) — run `task --list` for all commands
- `ui/` — Svelte 5 + SvelteKit 2 + TypeScript frontend (subtree-imported
  from semstreams-ui on 2026-04-10, see `ui/.claude/CLAUDE.md` for UI
  conventions)

## What lives here

| Path | Purpose |
|------|---------|
| `cmd/semteams/` | Branded binary. Wraps semstreams' componentregistry.Register — no custom components |
| `cmd/openapi-generator/` | Dev tool: generate OpenAPI spec from component registry |
| `configs/` | Flow-template library. Loadable at runtime via UI |
| `docs/` | Product and integration documentation |
| `schemas/`, `specs/` | Generated (via `task schema:generate`) — do not hand-edit |
| `test/contract/` | Contract tests: payload registry consistency, config sanity checks |
| `test/e2e/mock/` | Mock OpenAI / AGNTCY server for UI Playwright journeys |
| `test/fixtures/journeys/` | Playwright journey fixtures (YAML) |
| `ui/` | Svelte 5 + SvelteKit 2 frontend (graph explorer, flow builder, agentic UI) |
| `docker/` | Production Dockerfile + optional services compose (observability) |

## What does NOT live here

- Framework code (components, gateways, NATS clients, the graph engine) —
  all upstream in semstreams.
- Backend e2e scaffolding — deliberately removed; will be rebuilt from
  scratch when coordinator/ops-agent work lands.
- Custom `agentic-*` processors — upstreamed to semstreams as of beta.8.

## Common Tasks

```bash
task build              # Build bin/semteams
task test               # Run Go tests (fast)
task check              # Go lint + test
task check:all          # Go + UI lint + type-check + test + build

# UI
task ui:dev             # Start Vite dev server
task ui:test            # Vitest unit/component tests
task ui:test:e2e        # Playwright E2E tests (auto-manages Docker stack)
task ui:lint            # ESLint
task ui:check           # svelte-check (TypeScript)
task ui:build           # Production build
```

## Config Layering

| Config | Purpose | Model |
|--------|---------|-------|
| `agentic.json` | Production general-purpose | claude-haiku |
| `agentic-claude.json` | Production Claude variant | claude-haiku |
| `deep-research.json` | Production researcher workflow | claude-haiku |
| `onboarding.json` | Onboarding interview demo (intent classification, profile context, /onboard command) | claude-haiku |
| `e2e-agentic.json` | E2E testing | mock-llm |
| `e2e-deep-research.json` | E2E deep-research testing | mock-llm |
| `e2e-ops-observer.json` | E2E ops observer (ADR-027 Phase 1) — deep-research + observe rule | mock-llm |

UI Playwright journey tasks (in `ui/Taskfile.yml`) manage the Docker stack
lifecycle — Playwright does NOT auto-start the stack. Each task: start →
health-check → test → cleanup.

### Ops Agent Phase 1 (ADR-027, accepted)

Read-only diagnostic agent grounded in per-flow objective specs.
Status accepted upstream 2026-04-18 (ADR-027 Proposed → Accepted).
Framework support landed in semstreams `v1.0.0-beta.9`:
`fire_every_n_events` rule field, persona file-loader,
`emit_diagnosis` tool, `GET /graph/triples` endpoint. ADR lives at
`../semstreams/docs/adr/027-ops-agent-meta-harness.md`.

Single-process deployment: the ops agent runs in the same backend as
the flow it observes. The observe rule fires `publish_agent` with
`role: ops-analyst`, and the existing `agentic-loop` consumes it
without a second dispatch. (Upstream ships
`../semstreams/configs/flows/ops-agent.json` as a reference for
operators who prefer a standalone ops binary; SemTeams does not
deploy it.)

- `configs/personas/ops.json` — persona definition. Read-only tool
  allowlist: `query_entities`, `query_relationships`,
  `read_loop_result`, `emit_diagnosis`, `submit_work`.
- `configs/personas/fragments/ops/05-semteams-identity.md`,
  `10-objective-grounding.md`, `20-diagnostic-rules.md` — persona
  fragments layered above upstream's `ops/00-identity.md` via the
  beta.9 file-loader (digit-prefix ordering).
- `configs/rules/ops/observe-complete-loops.json` — fires once per
  `fire_every_n_events: 20` completed researcher loops.
- `configs/e2e-ops-observer.json` — single-process e2e config
  (deep-research components + observe rule) for the Playwright
  journey.
- `docs/objectives/README.md` — objective-spec schema.
- `docs/objectives/deep-research.md` — first concrete spec.

The ops agent emits findings via the `emit_diagnosis` tool (not raw
triples). Each call requires `finding`, `recommendation`, `confidence`
(0.0–1.0), and `evidence` (≥1 graph entity ID). The framework's
executor mints `{org}.{platform}.ops.diagnosis.finding.{uuid}`
entities with `ops.diagnosis.{finding,recommendation,confidence,
evidence,observed_role,severity}` predicates.

Phase 2 (ops proposes changes) is **config-only** per upstream's
`_phase2_note`: add `create_rule`/`manage_flow`/etc. to
`allowed_tools` and mirror into `approval_required`. The existing
`ApprovalFilter` transitions the loop to `LoopStateAwaitingApproval`
for human review. No framework blocker remaining.

### Component Instance vs Factory

Configs use instance names `teams-dispatch` and `teams-loop` (so HTTP
endpoints at `/teams-dispatch/*` and `/teams-loop/*` match the UI's
hardcoded URL paths). The `name` field points at the upstream factory
(`agentic-dispatch`, `agentic-loop`):

```json
"components": {
  "teams-dispatch": {         // instance name → HTTP prefix
    "type": "processor",
    "name": "agentic-dispatch", // factory lookup
    ...
  }
}
```

### Personalization Toggles (agentic-dispatch, agentic-memory, agentic-tools)

These upstream config fields default `false`; enable per config as needed:

- `agentic-dispatch.enable_intent_classification` — LLM-assisted intent
  classifier (used by onboarding.json)
- `agentic-dispatch.enable_onboarding` — `/onboard` command + interview
  state machine (onboarding.json)
- `agentic-memory.enable_profile_context` — assemble operating-model
  profile context on loop creation (onboarding.json)
- `agentic-tools.approval_required` — list of tool names requiring human
  approval (agentic.json, agentic-claude.json have rule-write tools gated)
- `agentic-tools.enable_categories` — tool category filtering for
  role-based access
- `agentic-governance.enable_tool_governance` — pre-execution governance
  filtering

## E2E Active Monitoring Protocol (MANDATORY)

UI Playwright journeys are long-running. MUST monitor actively — never
block in foreground.

1. Launch via `run_in_background: true`
2. Monitor three sources every 20–30s:
   - Test output: non-blocking `TaskOutput` read
   - Backend logs: `docker compose -f ui/docker-compose.agentic-e2e.yml logs --since=30s`
   - Message logger: `curl -s http://localhost:3100/message-logger/entries?limit=10 | jq '.[].subject'`
3. Dump evidence to `/tmp/` for post-mortem
4. Abort early if stuck in loops or burning tokens on retries
5. Report with evidence — quote log lines, never guess at root cause

## CI Requirements

Two workflows run:

**`.github/workflows/ci.yml`** (Go):
1. Lint — `go vet`, `go fmt` (must be clean), `revive` (warnings = failure)
2. Test — Unit tests with `-race`
3. Build — Cross-compile Linux binary
4. Schema Validation — `task schema:generate`, check for uncommitted
   changes

**`.github/workflows/ui.yml`** (Svelte, path-filtered to `ui/**`):
1. Lint — `npm run lint`
2. Type Check — `npm run check`
3. Unit Tests — `npm run test:unit`
4. Build — `npm run build`

Before pushing:

```bash
task lint
go test -race ./...
task schema:generate
git diff schemas/ specs/
go test ./test/contract/...
```

## Related Repos

- [semstreams](https://github.com/c360studio/semstreams) — framework.
  Owns all `agentic-*`, `graph-*`, `rule`, I/O, and gateway components.
  The place to make framework-level changes.
- [semdragons](https://github.com/c360studio/semdragons),
  [semspec](https://github.com/c360studio/semspec) — sibling products
  that also import semstreams.
