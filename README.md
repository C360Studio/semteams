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
| Bootstrap config | `configs/flow-bootstrap.json` | ADR-042 substrate-plus-overlays wiring (production); mock-LLM clone at `e2e-flow-bootstrap.json` |
| Category packs | `configs/rules/<category>/` | Category-keyed rule packs (currently: `research/`). Future packs add new task classes without new components. |
| Personas | `configs/personas/fragments/<role>/*.md` | Role-specific prompt fragments (coordinator, researcher-research-{plan,gather,synthesize}, reviewer-research, ops*) |
| Product tools | `cmd/semteams/tools/` | Tool executors that don't belong upstream (source ingest, artifact emission, sandbox bootstrap) |
| Product shell | `cmd/semteams/main.go` | ~600 LoC binary that wires the framework primitives per [ADR-029](docs/adr/029-product-shell-wiring.md) |

Everything else — the `agentic-*` processors, the rule engine, the
graph, the NATS stream wiring — lives upstream in semstreams.

## Run it

### Prereqs

```bash
go version          # 1.25+
docker info         # daemon running
task --version      # go install github.com/go-task/task/v3/cmd/task@latest
```

Copy `.env.example` to `.env` and set at least one LLM key
(`ANTHROPIC_API_KEY` recommended — most configs default to
`claude-haiku`):

```bash
cp .env.example .env
# edit .env, uncomment ANTHROPIC_API_KEY=sk-ant-...
```

### One command to a working chat UI

```bash
task dev:research
```

That boots NATS, builds and starts `bin/semteams` against
`configs/flow-bootstrap.json` (the ADR-042 production bootstrap),
then starts the UI proxy. Open <http://localhost:3001> and type a
research question — the coordinator persona will route it through
the research-category arc.

`task dev:stop` tears it down.

### Other useful starting points

| You want | Run | Notes |
|---|---|---|
| Production bootstrap (research category) | `task dev:research` | Needs `ANTHROPIC_API_KEY`; `BRAVE_SEARCH_API_KEY` recommended (web_search falls back to a stub without it). |
| Mock-LLM research-category journey | `task ui:test:e2e:agentic:research-mvp` | Playwright + mock-llm + e2e-flow-bootstrap. No API keys. |
| Real-LLM research-category smoke | `task ui:test:e2e:agentic:smoke6:run` | ~$0.30–$0.50 on claude-haiku. Captures trajectories to `/tmp/smoke6-<RUN_ID>/`. |

`task --list` shows everything.

## Find your way around

- **You are running a journey and want to debug it** →
  [`docs/getting-started.md`](docs/getting-started.md). What the
  ports are, how to tail logs, how to inspect KV, how to abort a
  wedged loop.
- **You want to know why something is built the way it is** →
  [`docs/adr/`](docs/adr/). Every product-shell decision lands as an
  ADR.
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

Active development. Breaking changes expected. The active product
arc is ADR-031 (research flow + dev-via-spec internal mode); see
the ADR for current phase.

## License

[LICENSE](LICENSE).
