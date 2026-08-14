# SemTeams

> Reference/demo product for agentic teams, built on the
> [semstreams](https://github.com/c360studio/semstreams) framework.

SemTeams is the product shell — UI, configs, personas, rules, and a
small set of product-shell tools — that demonstrates how to build a
multi-agent system on top of semstreams. It has **no custom Go
components**: every processor (graph, rule, agentic-dispatch,
agentic-loop, agentic-model, agentic-tools, …) is imported from the
upstream framework.

If you are looking for the framework itself (component model,
knowledge graph, NATS streams, GraphQL gateway), see
[semstreams](https://github.com/c360studio/semstreams). This README
covers what SemTeams adds on top.

```
┌─────────────────────────────────────────────────────────────┐
│                       SemTeams                              │
│  Svelte UI · configs · personas · rules · product tools     │
├─────────────────────────────────────────────────────────────┤
│              semstreams (Go module dependency)              │
│  components · graph · rule engine · agentic loop · NATS     │
└─────────────────────────────────────────────────────────────┘
```

## What SemTeams adds

| Surface | Path | What it is |
|---|---|---|
| Web UI | `ui/` | Svelte 5 + SvelteKit 2 chat / graph explorer / runtime monitor |
| Bootstrap config | `configs/flow-bootstrap.json` | Production substrate-plus-overlays wiring; mock-LLM clone at `e2e-flow-bootstrap.json` |
| Category packs | `configs/rules/<category>/` | Category-keyed rule packs. Product-facing packs include `research/`, `autoresearch/`, `create-change/`, `proof-readiness/`, `dev-from-task/`, and `dev-via-test/`. Support packs include `coordinator/`, `agent-run/`, and `ops/`. New task classes add a pack — no new components. |
| Personas | `configs/personas/fragments/<role>/*.md` | Role-specific prompt fragments loaded by the category packs. See [`configs/README.md`](configs/README.md) for the current inventory. |
| Product tools | `cmd/semteams/tools/` | Tool executors that don't belong upstream: source ingest, artifact/spec emission, proof analysis/projection, sandbox bootstrap, and pack-specific measurement emitters. |
| Product shell | `cmd/semteams/main.go` | ~600 LoC binary that wires the framework primitives directly for this product shell |

Everything else — the `agentic-*` processors, the rule engine, the
graph, the NATS stream wiring — lives upstream in semstreams.

## Run it

### Prereqs

```bash
go version          # 1.25+
docker info         # daemon running
node --version      # 22+ for UI / Playwright journeys
task --version      # go install github.com/go-task/task/v3/cmd/task@latest
caddy version       # required only for the live local UI path: task dev:research
```

### First proof: no-key demo MVP

```bash
task ui:test:e2e:agentic:demo-mvp
```

That runs the black-box mock-LLM evidence pack for the current demo
claims: coordinator routing, OpenSpec author/review/export,
readiness fail-closed behavior, and the MAVLink-hard spec handoff.
It uses the dockerized e2e stack and requires no LLM API keys or
host Caddy install.

### Live chat UI

Copy `.env.example` to `.env` and set a live model key. The default
production registry uses Gemini Flash, with Gemini Pro preferred for
coordinator work and Anthropic endpoints available as fallbacks /
alternatives:

```bash
cp .env.example .env
# edit .env, uncomment GEMINI_API_KEY=...
```

```bash
task dev:research
```

That boots NATS, builds and starts `bin/semteams` against
`configs/flow-bootstrap.json` (the production bootstrap),
then starts the UI proxy. Open <http://localhost:3001> and type a
prompt. You can chat with the coordinator first: ask what SemTeams can
do, refine a rough idea, or ask which team fits. When the request is
ready, the coordinator classifies it and routes it to one of the live
category packs:

- a **research** question (compare X vs Y, how does Z work) → the
  research arc;
- a **spec authoring** ask (draft requirements for X, create an
  OpenSpec handoff for Y) → the create-change / proof-readiness
  path;
- an **optimize-a-metric** ask (make this faster / smaller) → the
  autoresearch iteration loop, in a sandbox;
- a **build-with-tests** ask (add an endpoint with unit tests) →
  the dev-via-test pack (Lisa plans → CBG gates the plan → Ralph
  implements in a sandbox → CBG gates the work).

Power users can prefix a prompt with `/research`, `/create-change`
(`/spec` also works), `/optimize`, or `/dev-via-test`. Those are
coordinator-routed hints, not bypasses; SemTeams still validates the
prompt shape and keeps the usual sandbox, readiness, approval, and
review gates.

Artifacts are meant to travel between teams. When a run emits a
research, spec, optimization, or implementation artifact, the UI
surfaces it as an artifact card with copy and "use as context"
actions. Attaching an artifact adds a visible, removable context chip
to the chat bar, and the next prompt carries the artifact title,
source tool, and content back through the coordinator. A research
artifact can seed a spec prompt, a spec can inform implementation, or
any artifact can simply anchor a follow-up question.

See [`docs/architecture.md`](docs/architecture.md) for what each
pack does and **how the sandbox is created**.

`task dev:stop` tears it down.

### Other useful starting points

| You want | Run | Notes |
|---|---|---|
| No-key demo claim proof | `task ui:test:e2e:agentic:demo-mvp` | Black-box Playwright + mock-LLM evidence pack. No API keys. |
| Live chat UI | `task dev:research` | Needs `GEMINI_API_KEY` for the default model registry; `BRAVE_SEARCH_API_KEY` recommended for web search. |
| OpenSpec author/review/export journey | `task ui:test:e2e:agentic:create-change` | Mock-LLM journey for producing a reviewed OpenSpec handoff. |
| MAVLink-hard spec handoff | `task ui:test:e2e:agentic:mavlink-hard-spec` | Mock-LLM hard-domain OpenSpec handoff journey. |
| Spec-to-dev bridge proof | `task ui:test:e2e:agentic:spec-to-dev-demo` | Fixture-seeded bridge proof; not counted as pure black-box MVP evidence. |
| Paid Gemini OpenSpec smoke | `task ui:test:e2e:agentic:create-change:gemini-smoke` | Real LLM smoke for spec authoring. Captures trajectories under `/tmp/`. |

`task --list` shows everything.

## Find your way around

- **You are running a journey and want to debug it** →
  [`docs/getting-started.md`](docs/getting-started.md). What the
  ports are, how to tail logs, how to inspect KV, how to abort a
  wedged loop.
- **You want to know what the demo can honestly claim** →
  [`docs/demo-mvp-claims.md`](docs/demo-mvp-claims.md). Supported
  claims, non-claims, black-box evidence rules, and MAVLink-hard
  scope.
- **You want to know how prompts move through teams** →
  [`docs/architecture.md`](docs/architecture.md). Coordinator routing,
  category packs, artifacts, gates, and sandbox handoff.
- **You want to extend the product shell with a new tool, rule,
  persona, or KV bucket** → read
  [`cmd/semteams/tools/README.md`](cmd/semteams/tools/README.md)
  first. There is a mandatory framework-alignment review before
  adding to the shell — the semspec accretion lesson.
- **You want a flow other than the stock ones** → copy a config
  from `configs/`, swap personas / rules, point the binary at it.
- **You want framework concepts (graph, rules, NATS streams,
  Graphable, payload registry)** → upstream
  [semstreams docs](https://github.com/c360studio/semstreams/tree/main/docs).
  This repo doesn't re-document them.

## Develop

```bash
task build              # Build bin/semteams
task check              # Go lint + test (fast, no Node)
task check:all          # + UI lint + type-check + test + build

task ui:dev             # Vite dev server (UI only)
task ui:test:e2e        # Playwright E2E (auto-manages Docker stack)

task schema:generate    # Regenerate schemas/ + specs/openapi.v3.yaml
```

CI runs `ci.yml` (Go lint/test/build/schema) and `ui.yml` (UI
lint/check/test/build, path-filtered). Both must pass.

Before pushing:

```bash
task lint
go test -race ./...
task schema:generate && git diff schemas/ specs/   # must be clean
go test ./test/contract/...
```

See [CLAUDE.md](CLAUDE.md) for the deeper project context — config
layering, product-shell wiring map, mandatory protocols
(reviewer-pass, framework-alignment, E2E active monitoring).

## Status

Active development. Breaking changes expected. Current architecture
is **substrate-plus-overlays**: a single product-shell flow wires
substrate singletons, and task classes are added as category-keyed
rule packs + named persona bundles rather than separate flow
configs. Product-facing packs:

- **research** — coordinator-routed prose research arc.
- **autoresearch** — Karpathy-style propose/execute iteration loop
  with empirical keep/revert decisions and per-tenant devcontainer
  attestation. Shipped 2026-06-03.
- **create-change / proof-readiness / dev-from-task** —
  OpenSpec-compatible spec production, review, export, readiness
  gating, and the approved-spec-to-task bridge. See
  [`docs/demo-mvp-claims.md`](docs/demo-mvp-claims.md).
- **dev-via-test** — Lisa / Ralph / CBG software-development loop
  with plan and work gates.

## License

[LICENSE](LICENSE).
