# SemTeams Configs

Under [ADR-042 §Phase 2](../docs/adr/042-coordinator-instantiated-flows-via-templates.md)
(substrate-plus-overlays, MVP-7) there is **one** product-shell
flow config. New task classes ship as category-keyed rule packs +
persona bundles, not as new flow configs.

## SemTeams flow configs

| Config | Purpose | Model |
|---|---|---|
| [`flow-bootstrap.json`](flow-bootstrap.json) | Production substrate (ADR-042 MVP). Wires the singleton agentic stack (graph-ingest, graph-query, rule-processor, agentic-loop, agentic-dispatch, agentic-tools, agentic-model) + the live product-facing rule packs (research, autoresearch, create-change, proof-readiness, dev-from-task, dev-via-test) + support packs (coordinator, agent-run, ops) + the persona corpus that drives them. | gemini-flash (default); coordinator capability prefers gemini-pro with gemini-flash / claude-haiku fallback |
| [`e2e-flow-bootstrap.json`](e2e-flow-bootstrap.json) | Mock-LLM clone of flow-bootstrap. Same packs + personas; model registry points at the in-process mock LLM. Used by every Playwright journey under `ui/e2e/agentic/`. | mock-llm |

`task ui:test:e2e:agentic:demo-mvp` runs the black-box demo
evidence pack against the mock LLM (no API keys); `task
dev:research` runs the production config (default registry needs
`GEMINI_API_KEY`).

## What lives where

```
configs/
├── flow-bootstrap.json          ← production substrate
├── e2e-flow-bootstrap.json      ← mock-LLM clone
├── rules/
│   ├── coordinator/             ← chat front-door rules
│   ├── research/                ← prose research arc
│   ├── autoresearch/            ← Karpathy-style iteration loop
│   ├── create-change/           ← OpenSpec author / review / export
│   ├── proof-readiness/         ← proof dependency + readiness gates
│   ├── dev-from-task/           ← approved-spec task bridge
│   ├── dev-via-test/            ← Lisa / Ralph / CBG implementation loop
│   ├── agent-run/               ← shared run lifecycle support
│   └── ops/                     ← parallel observability track
└── personas/fragments/
    ├── coordinator/             ← domain-neutral router persona
    ├── researcher-research-*/   ← plan / gather / synthesize roles
    ├── reviewer-research/
    ├── autoresearch-*/          ← baseline / propose / execute / synthesize
    ├── reviewer-autoresearch/
    ├── author-create-change/    ← OpenSpec author
    ├── reviewer-create-change/  ← OpenSpec reviewer
    ├── dev-via-test-*/          ← Lisa / Ralph
    ├── reviewer-dev-via-test/   ← CBG plan + work gate
    ├── ops/                     ← read-only diagnostic agent
    ├── ops-chain-observer/
    └── ops-progress-observer/
```

Adding a new task class is a new overlay: drop a rule pack
directory under `rules/<category>/`, drop persona bundles under
`personas/fragments/<role>-<category>-<phase?>/`, and teach the
coordinator's `decide(action=…)` contract one new token. See the
rule packs above for working templates.

## Legacy configs

Other JSON files in this directory (`structural.json`,
`statistical.json`, `semantic.json`, `protocol-flow.json`,
`hello-world.json`, federation configs, etc.) are upstream
SemStreams graph-tier deployment examples carried over during the
initial subtree fork. They are **not** SemTeams product configs.
A couple are still wired into legacy e2e plumbing under `ui/`
(see `ui/E2E_SETUP.md`); the rest are unused.

For the upstream graph-tier deployment model (Structural /
Statistical / Semantic tiers, graph component family, KV bucket
layout), see
<https://github.com/c360studio/semstreams/tree/main/docs>.

## Further reading

- [`docs/architecture.md`](../docs/architecture.md) — what runs
  on top of these configs end-to-end.
- [`docs/adr/042`](../docs/adr/042-coordinator-instantiated-flows-via-templates.md)
  — why there is just one config (substrate-plus-overlays).
- [`docs/adr/029`](../docs/adr/029-product-shell-wiring.md) — how
  `cmd/semteams/main.go` wires the substrate.
- [`cmd/semteams/tools/README.md`](../cmd/semteams/tools/README.md)
  — product-shell tool executors that the `agentic-tools`
  registry runs.
