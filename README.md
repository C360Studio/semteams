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
| Flow library | `configs/*.json` | Loadable flow templates — agentic, deep-research, dev-via-spec, ops-observer, … |
| Personas | `configs/personas/fragments/<role>/*.md` | Role-specific prompt fragments (researcher, coordinator, builder, qa-reviewer, ops, …) |
| Rules | `configs/rules/<flow>/*.json` | Coordinator/router/approval/observe rules that trigger agent dispatch |
| Product tools | `cmd/semteams/tools/` | Tool executors that don't belong upstream (source ingest, artifact emission, builder terminal, sandbox bootstrap) |
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
`configs/deep-research.json`, then starts the UI proxy. Open
<http://localhost:3001> and type a research question.

`task dev:stop` tears it down.

### Other useful starting points

| You want | Run | Notes |
|---|---|---|
| Full agentic chat (general-purpose) | `./bin/semteams --config configs/agentic.json` | Needs NATS up (`task dev:nats:start`) |
| Deep research with web search | `task dev:research` | Needs `ANTHROPIC_API_KEY` + `BRAVE_SEARCH_API_KEY` |
| Onboarding interview demo | `./bin/semteams --config configs/onboarding.json` | Intent classification + `/onboard` |
| Ops observer over deep research | `./bin/semteams --config configs/e2e-ops-observer.json` | ADR-027 read-only diagnostic agent |
| Dev-via-spec / OSH demo | `./bin/semteams --config configs/osh-demo.json` | Architect → builder → qa chain (sandbox required) |

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
