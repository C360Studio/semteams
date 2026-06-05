# Getting Started with SemTeams

A practical guide for someone who just cloned the repo and wants to
run SemTeams, change a flow, and debug it when something goes
sideways. For framework concepts (graph, components, NATS streams,
payload registry), follow the upstream
[semstreams docs](https://github.com/c360studio/semstreams/tree/main/docs)
— this guide doesn't repeat them.

## Prereqs

```bash
go version          # 1.25+
docker info         # daemon running
task --version      # go install github.com/go-task/task/v3/cmd/task@latest
node --version      # 20+ if you'll touch the UI
```

## What's actually running when SemTeams is up

```
                ┌──────────────────────────────────────────┐
                │  ui (Caddy + Vite, port 3001)            │
                │  src: ui/                                │
                └─────────────────┬────────────────────────┘
                                  │ proxies /teams-* and /graphql
                                  ▼
┌──────────────────────────────────────────────────────────┐
│  bin/semteams (the product-shell binary, port 8080)      │
│  config: configs/<name>.json                             │
│   - agentic-dispatch   (intake → loop)                   │
│   - agentic-loop       (think/act state machine)         │
│   - agentic-model      (LLM client, OpenAI-compatible)   │
│   - agentic-tools      (tool executor, web/graph/bash/…) │
│   - rule processor     (coordinator/router/observe)      │
│   - graph-* components (entities, indexes, gateway)      │
│  metrics: 9090 · pprof: 8083 · graphql: 8084 (when on)   │
└─────────────────────────────────────────────────────────┘
                                  │
                                  ▼
                         ┌─────────────────┐
                         │  NATS JetStream │
                         │  port 4222      │
                         │  monitor 8222   │
                         └─────────────────┘
```

Everything in the inner box is **upstream semstreams** — SemTeams
contributes the `main.go` that wires it, plus product-shell tools,
personas, and rules.

> **Which port?** There are two run modes and they expose different
> ports — match the curls below to how you booted:
>
> | Mode | How | Backend HTTP | UI |
> |---|---|---|---|
> | **Direct dev** | `task dev:research` (or `./bin/semteams …`) | **`localhost:8080`** | Vite `localhost:3001` |
> | **Dockerized e2e / smoke stack** | `task ui:test:e2e:agentic:*` | via Caddy proxy on **`localhost:3100`** | `localhost:3100` |
>
> The Debug curls in this guide use `:8080` (direct dev). On the
> dockerized stack, swap `:8080` → `:3100`.

## Boot

### Fastest path (dev research with UI)

```bash
cp .env.example .env                     # add ANTHROPIC_API_KEY at minimum
task dev:research                        # NATS + backend + UI
# open http://localhost:3001
```

`task dev:stop` kills it. Use a second terminal for everything below.

### A specific config

```bash
task dev:nats:start                                 # if NATS isn't already up
go build -o bin/semteams ./cmd/semteams
./bin/semteams --config configs/flow-bootstrap.json
```

Under ADR-042 §Phase 2 (substrate-plus-overlays, MVP-7) there is
**one** product-shell flow config:

| Config | What it runs | Needs |
|---|---|---|
| `flow-bootstrap.json` | The single ADR-042 substrate (graph-ingest, graph-query, rule-processor, agentic-loop, agentic-dispatch, agentic-tools, agentic-model) plus the three live category rule packs (`research/`, `autoresearch/`, `dev-via-test/`) + `coordinator/` + `ops/`, and the persona corpus that drives them. Uses real LLMs. | `ANTHROPIC_API_KEY` / `GEMINI_API_KEY` (model registry; coordinator prefers `gemini-pro`); `BRAVE_SEARCH_API_KEY` for web_search |
| `e2e-flow-bootstrap.json` | Mock-LLM clone of the production bootstrap. Same packs + personas, points the model registry at the in-process mock LLM. Used by every Playwright journey under `ui/e2e/agentic/`. | nothing — mock-LLM |

Adding a task class is a new **category pack**, not a new config:
rule files under `configs/rules/<category>/`, persona bundles under
`configs/personas/fragments/<role>-<category>-<phase?>/`, plus a
coordinator-persona entry teaching the new `decide(action=<category>)`
token. See ADR-042 §Phase 2 redesign for the rationale and the
research / autoresearch / dev-via-test packs for working templates.

### Running a pack that needs a sandbox (autoresearch, dev-via-test)

The **research** pack runs anywhere — `task dev:research` is enough.
But **autoresearch** and **dev-via-test** run their `bash` inside a
per-tenant devcontainer (see [architecture.md](architecture.md)
§"How a sandbox gets created"), which needs the sandbox sidecar +
`@devcontainers/cli` + `SEMTEAMS_SANDBOX_RUNNER=api`. The dockerized
smoke tasks wire all of that up for you — the simplest way to run
one end-to-end locally is a real-LLM smoke with your own prompt:

```bash
# autoresearch or dev-via-test, real LLM, full sandbox lifecycle:
PROMPT="Add a Go HTTP service that … with unit tests." \
  DEBUG=1 KEEP_STACK_UP=1 RUN_ID=my-run \
  task ui:test:e2e:agentic:smoke13:run
# evidence (loops + trajectories) lands in /tmp/my-run/;
# DEBUG=1/KEEP_STACK_UP=1 leave the stack up so you can poke at it.
```

With the default `SEMTEAMS_SANDBOX_RUNNER=mock` (e.g. Playwright
journeys), `request_sandbox` returns a fabricated attestation and no
real container is created — fine for mock-LLM wiring tests, not for
actually building code.

The legacy concrete configs (`agentic.json`, `agentic-claude.json`,
`dev-research.json`, `onboarding.json`, `osh-demo.json`, plus all
the `e2e-*` siblings) retired in ADR-042 MVP-7 (PR #178) alongside
the `chain.mode` / `phasevalidator` / `chainstall` machinery they
depended on. They live in git history if you need archeology.

`./bin/semteams --validate --config configs/<name>.json` validates a
config without starting it.

### Verify it's healthy

```bash
curl -s http://localhost:8080/readyz | jq            # ready?
curl -s http://localhost:8080/health | jq            # component states
curl -s http://localhost:8080/api/components | jq    # what loaded?
```

If `/readyz` doesn't return 200 within ~10 seconds of boot, jump to
[Debug](#debug).

## Tune

### Picking a model

`configs/<name>.json` has a `model_registry` block declaring named
endpoints (`claude-haiku`, `claude-sonnet`, `gemini-flash`, …) and a
`defaults.model`. Every per-component model knob references one of
those endpoint names. **The defaults block must point at an
endpoint name, not a "capability" string** — the binary won't boot
otherwise (regression we hit on `osh-demo.json`).

API keys come from `.env` (loaded by Taskfile dotenv directive and
Docker Compose). Add new ones to `.env`, reference them via
`api_key_env: "MY_KEY"` in the endpoint definition.

### Editing a persona, rule, or tool

| Want to change… | Edit | Reload |
|---|---|---|
| What a role says it knows / how it should behave | `configs/personas/fragments/<role>/*.md` | Restart `bin/semteams` (fragments load at boot from disk) |
| What triggers an agent dispatch | `configs/rules/<flow>/*.json` | Restart, or use rule CRUD tools at runtime |
| A product-shell tool | `cmd/semteams/tools/<tool>/` | Rebuild + restart |
| Per-component config (`enable_intent_classification`, model, timeouts, …) | The component block in `configs/<name>.json` | Restart, with `down -v` if the change is a runtime KV-watched config (see KV cache trap below) |

Persona fragments are layered by digit-prefix (`05-…md`,
`10-…md`, `20-…md`) and stack on top of upstream's
`<role>/00-identity.md`. Add new ones with the next number.

### KV cache trap (read this once)

NATS KV holds the live config. On `docker compose restart` of the
backend, components read the *cached* KV value — not the file you
just edited on disk. If your config edit doesn't take effect:

```bash
docker compose -f ui/docker-compose.agentic-e2e.yml down -v
docker compose -f ui/docker-compose.agentic-e2e.yml up -d
```

`-v` drops the volume, forcing the binary to re-seed KV from disk.
This bites every time on e2e config iteration. (Memory:
`feedback_nats_config_caching`.)

### Adding a new product-shell tool / rule / payload / KV bucket

**Stop and read [`../cmd/semteams/tools/README.md`](../cmd/semteams/tools/README.md)
first.** There is a mandatory framework-alignment review before
landing any of those — survey upstream for an existing or
roadmapped equivalent, document the alternatives ruled out in the
relevant ADR addendum. The semspec accretion lesson is exactly
this trap. (Memory:
`feedback_framework_alignment_review`.)

## Debug

### Tail the logs

```bash
# Foreground: just watch the binary's stdout (already tailing if you ran it directly)
# Backgrounded via task dev:research:
docker compose -f ui/docker-compose.agentic-e2e.yml logs -f backend
# or just the recent ones:
docker compose -f ui/docker-compose.agentic-e2e.yml logs --since=30s backend
```

Set `--log-level=debug` (or `SEMSTREAMS_LOG_LEVEL=debug`) for the
loud version. `--log-format=text` is easier to read at the
terminal; default is `json` for pipelines.

### Inspect message flow (the message-logger gateway)

Available at `http://localhost:8080/message-logger/...` whenever
the message-logger gateway is enabled (it is in every shipped
config):

```bash
# Recent messages flowing through every subject
curl -s "http://localhost:8080/message-logger/entries?limit=20" | jq

# Stats per subject + JetStream stream depths
curl -s "http://localhost:8080/message-logger/stats" | jq

# Trace one message end-to-end through the pipeline
TRACE_ID=$(curl -s "http://localhost:8080/message-logger/entries?limit=1" | jq -r '.[0].trace_id')
curl -s "http://localhost:8080/message-logger/trace/$TRACE_ID" | jq

# KV bucket contents (no need to attach nats CLI)
curl -s "http://localhost:8080/message-logger/kv/AGENT_LOOPS?limit=20" | jq
curl -s "http://localhost:8080/message-logger/kv/PERSONAS?limit=20" | jq
```

The `task dev:messages` / `task dev:trace` / `task dev:stats` /
`task dev:kv` shortcuts wrap these with sensible defaults.

### Inspect a wedged loop

```bash
# Find recent loops
curl -s "http://localhost:8080/message-logger/kv/AGENT_LOOPS?limit=10" \
  | jq '.entries[] | {key, revision}'

# Pull one
curl -s "http://localhost:8080/message-logger/kv/AGENT_LOOPS/<loop_id>" | jq

# Or via the loop trajectory endpoint (better for UI parity)
curl -s "http://localhost:8080/teams-loop/trajectories/<loop_id>" | jq
```

What to look at:

- `state` — `awaiting_model`, `awaiting_tools`, `awaiting_approval`,
  `complete`, …
- `pending_tools` — non-empty here means a tool dispatched and the
  result hasn't come back. Restart `agentic-tools` or check its
  log for an executor panic.
- `iteration` vs `max_iterations` — runaway loop? Check
  `MaxIterations` in the loop config.
- `metadata.role` — for dispatch-spawned loops, omitted; assume
  the flow's default role. (Memory:
  `feedback_loopinfo_role_omitempty`.)

### Debugging a dev-via-test chain + its sandbox

The dev-via-test pack (Lisa → CBG plan-gate → Ralph → CBG work-gate)
leaves its whole state as triples on the run entity, and its work
on a real filesystem. Read both with the graph-triples endpoint
(predicate-filtered) and a `docker exec`. dev-via-test runs on the
**dockerized stack**, so these use `:3100` (see "Which port?").

```bash
# Where is the chain / which verdict did a gate emit? The decision
# tokens tell the story: plan_approved | plan_rejected_retry |
# plan_rejected (plan gate) vs approved | rejected_retry | rejected
# (work gate).
curl -s "http://localhost:3100/graph/triples?predicate=coordinator.decision.next_action&limit=30" \
  | jq -r 'sort_by(.timestamp)[] | "\(.timestamp[11:19])  \(.object)"'

# The plan Lisa actually emitted (the fidelity the plan gate checks):
for p in plan.goal plan.integration_test_command plan.revision; do
  curl -s "http://localhost:3100/graph/triples?predicate=$p&limit=5" \
    | jq -r --arg p "$p" 'sort_by(.timestamp)[-1] | "\($p) = \(.object)"'
done

# Did a retry fire? (Slice 5/6) — presence of these = a bounce happened:
curl -s "http://localhost:3100/graph/triples?predicate=dev_via_test.plan.retry.finding" | jq -r '.[].object'
curl -s "http://localhost:3100/graph/triples?predicate=dev_via_test.cbg.retry.finding"  | jq -r '.[].object'
```

The sandbox itself:

```bash
# Did the sandbox provision + attest? (request_sandbox → devcontainer up)
curl -s "http://localhost:3100/graph/triples?predicate=sandbox.attestation.ready" | jq -r '.[].object'
curl -s "http://localhost:3100/graph/triples?predicate=sandbox.attestation.profile" | jq -r '.[].object'

# Ralph's ACTUAL output (what CBG's integration test + git diff see):
#   - on the dockerized stack, the per-tenant workspace is bind-mounted
#     to the host under ui/.tenant-workspaces/<tenant>/
ls ui/.tenant-workspaces/*/                         # the workspaces
#   - or exec into the sandbox sidecar to poke the live container:
docker exec -it semteams-ui-agentic-sandbox bash
```

If the chain stalled right after `request_sandbox`, check the
backend log for `devcontainer up` errors (`context deadline
exceeded` on a cold first pull is the usual culprit) and that
`SEMTEAMS_SANDBOX_RUNNER=api` is set — a `mock` runner never makes
a real container.

### Active monitoring during a journey

E2E journeys are long-running. Don't block the foreground waiting
for them. Run with `run_in_background: true`, and every 20–30s pull
three things:

```bash
# 1. test stdout (Playwright, mock-llm, ...)
# 2. backend logs since last check
docker compose -f ui/docker-compose.agentic-e2e.yml logs --since=30s backend
# 3. message-logger
curl -s "http://localhost:3100/message-logger/entries?limit=10" | jq '.[].subject'
```

Anything `in_progress` with no forward progress for >2× expected
step time is wedged — abort, don't wait for the natural timeout.
(Memory: `feedback_e2e_active_monitoring`.)

### Common boot failures

| Symptom | Likely cause | Fix |
|---|---|---|
| `connect to NATS: ...` | NATS not running or wrong URL | `task dev:nats:start`, or set `SEMSTREAMS_NATS_URLS` |
| `defaults.model "<x>" does not match any endpoint` | Config drift — `defaults.model` must reference an endpoint name | Match against `model_registry.endpoints` keys |
| `"loading persona fragments" path=<empty>` | `--persona-fragments` flag unset and default path missing | Pass `--persona-fragments=configs/personas/fragments` or set `SEMSTREAMS_PERSONA_FRAGMENTS_PATH` |
| `agentic-tools registered but executor missing` | Pattern-A/B tool wiring drifted | See [ADR-029](adr/029-product-shell-wiring.md) — `executors.RegisterBuiltins` must be called |
| Backend boots, UI shows "no flow" | UI auto-discovers the active flow via `/components/`; check `curl :8080/api/components` | Restart, or pick a flow under `/flows` |
| Mock-LLM journey passes locally but flakes in CI | Stale `mock-llm` build (changes to mock fixtures don't trigger a rebuild on `compose restart`) | `docker compose ... build --no-cache mock-llm` (memory: `feedback_mock_llm_rebuild`) |
| Orphan `semteams-ui-agentic-*` containers from interrupted runs | Compose project-name mismatch — cleanup uses `--project-name semteams-agentic`, orphans were created with the default | `docker rm -f $(docker ps -aq --filter name=semteams-ui-agentic-)` |

### Profiling

```bash
./bin/semteams --debug --debug-port=8083 --config configs/flow-bootstrap.json
# pprof at http://localhost:8083/debug/pprof/
```

## Where to go from here

- **Why is the binary built this way?** —
  [`adr/029-product-shell-wiring.md`](adr/029-product-shell-wiring.md)
  is the single most useful read.
- **What runs end-to-end when I send a prompt?** —
  [`architecture.md`](architecture.md). The substrate-plus-overlays
  runtime, the three live category packs (research, autoresearch,
  dev-via-test), and **how a sandbox gets created**.
- **Why is it built this way (substrate-plus-overlays)?** —
  [`adr/042-coordinator-instantiated-flows-via-templates.md`](adr/042-coordinator-instantiated-flows-via-templates.md).
- **How does the sandbox work?** —
  [`adr/043-devcontainer-as-sandbox-spec.md`](adr/043-devcontainer-as-sandbox-spec.md)
  (current; ADR-032 was the precursor).
- **How do I write a journey?** —
  [`journeys/README.md`](journeys/README.md). The journey IS the
  Playwright test under `ui/e2e/agentic/`.
- **How do I add a flow objective spec for the ops agent?** —
  [`objectives/README.md`](objectives/README.md).
- **Framework concepts I'm missing** — upstream
  [semstreams docs](https://github.com/c360studio/semstreams/tree/main/docs).
