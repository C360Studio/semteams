# SemTeams

> An always-on program manager for a configurable portfolio, built on the
> [semstreams](https://github.com/c360studio/semstreams) framework.

SemTeams observes work across programs and projects, explains what changed,
identifies where attention is needed, and helps an operator carry approved
actions through. Research, planning, and design are supporting capabilities of
that program manager, not separate product identities. The target product and
MVP boundary are defined in
[`docs/product/program-manager.md`](docs/product/program-manager.md).

The current repository is the early product shell — UI, configs, personas,
rules, and a small set of product-shell tools — proving the multi-agent runtime
on top of SemStreams. It has **no custom Go
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
| Category packs | `configs/rules/<category>/` | Live: `research/` and `autoresearch/`. Support: `coordinator/`, `agent-run/`, and `ops/`. Parked dev packs are donor material; see [ADR-058](docs/adr/058-beta159-realignment-and-demo-lane-focus.md). New task classes add a pack, not a component. |
| Personas | `configs/personas/fragments/<role>/*.md` | Role-specific prompt fragments loaded by the category packs. See [`configs/README.md`](configs/README.md) for the current inventory. |
| Product tools | `cmd/semteams/tools/` | Tool executors that don't belong upstream: source ingest, artifact/spec emission, proof analysis/projection, sandbox bootstrap, and pack-specific measurement emitters. |
| Product shell | `cmd/semteams/main.go` | ~600 LoC binary that wires the framework primitives directly for this product shell |

Everything else — the `agentic-*` processors, the rule engine, the
graph, the NATS stream wiring — lives upstream in semstreams.

## Run it

### Prereqs

```bash
go version          # 1.26.3
docker info         # daemon running
node --version      # 22.20.0 for UI / Playwright journeys
task --version      # go install github.com/go-task/task/v3/cmd/task@v3.51.1
caddy version       # required only for the live local UI path: task dev:research
```

### First proof: no-key demo MVP

```bash
task ui:test:e2e:agentic:demo-mvp
```

That runs the black-box mock-LLM evidence pack for the current demo
claims: coordinator routing, the reviewed research arc, fail-closed
readiness before execution routing, and empirical autoresearch.
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
- an **optimize-a-metric** ask (make this faster / smaller) → the
  autoresearch iteration loop, in a sandbox.

Power users can prefix a prompt with `/research` or `/optimize`.
Those are coordinator-routed hints, not bypasses; SemTeams still
validates the prompt shape and keeps the usual sandbox and review
gates.

Spec-authoring and software-implementation asks are currently
**parked**, not live routes. The coordinator answers them honestly
instead of dispatching `create-change`, `proof-readiness`,
`dev-from-task`, or `dev-via-test`. Those packs remain on disk as donor
material; SemDev now owns the issue-to-PR implementation journey. Do not
reintroduce them as a shortcut to program-manager action. See
[ADR-058](docs/adr/058-beta159-realignment-and-demo-lane-focus.md).

Research results retain recoverable source evidence, but beta.160 does
not currently render the evidence bodies needed by `ArtifactCard` or
artifact-context handoff. GraphQL exposes trajectory previews and
`StorageReference` values; an authorized evidence-fetch pass must land
before copy, attach, context-chip, or cross-team artifact reuse can be
claimed live. ADR-059 records this accepted regression; issue
[#261](https://github.com/C360Studio/semteams/issues/261) owns restoration.

See [`docs/architecture.md`](docs/architecture.md) for what each
pack does and **how the sandbox is created**.

`task dev:stop` tears it down.

### Other useful starting points

| You want | Run | Notes |
|---|---|---|
| No-key demo claim proof | `task ui:test:e2e:agentic:demo-mvp` | Black-box Playwright + mock-LLM evidence pack. No API keys. |
| Live chat UI | `task dev:research` | Needs `GEMINI_API_KEY` for the default model registry; `BRAVE_SEARCH_API_KEY` recommended for web search. |
| Research arc proof | `task ui:test:e2e:agentic:research-mvp` | Mock-LLM plan/fan-out/join/synthesize/review journey. |
| Autoresearch proof | `task ui:test:e2e:agentic:autoresearch` | Mock-LLM metric iteration with sandbox admission and keep/revert evidence. |
| Coordinator routing proof | `task ui:test:e2e:agentic:coordinator-routing-matrix` | Includes honest responses for parked team asks. |

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

The single `Repository CI` workflow runs on every pull request to
`main`. Its Go, UI, and Governance jobs feed the always-reported
`CI Status Check` aggregate. The Governance job strictly validates
OpenSpec and its queue reporter. Required mock E2E and a main-branch
ruleset remain future work; workflow presence alone is not branch
protection.

Before pushing:

```bash
task lint
task test:race
task test:integration
go build ./...
task schema:generate && task schema:check-changes
task openspec:validate
task openspec:queue-test
```

See [CLAUDE.md](CLAUDE.md) for the deeper project context — config
layering, product-shell wiring map, mandatory protocols
(reviewer-pass, framework-alignment, E2E active monitoring).

## Status

Active development. Breaking changes expected. The shipped proof is not yet
the program-manager MVP: today the live product-facing packs are research and
autoresearch. The next product slice is a read-only, evidence-backed program
pulse across operator-configured projects and repositories; see the
[`roadmap`](docs/ROADMAP.md).

Current architecture is **substrate-plus-overlays**: a single product-shell flow wires
substrate singletons, and task classes are added as category-keyed
rule packs + named persona bundles rather than separate flow
configs. The demo scope is the inner and outer loops
([ADR-058](docs/adr/058-beta159-realignment-and-demo-lane-focus.md)):

- **research** — coordinator-routed prose research arc: plan →
  parallel gather fan-out → join → synthesize → review → artifact
  with recoverable sources.
- **autoresearch** — Karpathy-style propose/execute iteration loop
  with empirical keep/revert decisions and per-tenant devcontainer
  attestation.
- **Parked donor material** — the spec-authoring and software-implementation packs
  (`create-change`, `proof-readiness`, `dev-from-task`,
  `dev-via-test`) are unwired. SemDev owns that product journey; the
  coordinator answers those asks honestly instead of routing them. See
  [`docs/demo-mvp-claims.md`](docs/demo-mvp-claims.md).

## License

[LICENSE](LICENSE).
