# SemTeams Configs

Under [ADR-042 §Phase 2](../docs/adr/042-coordinator-instantiated-flows-via-templates.md)
(substrate-plus-overlays, MVP-7) there is **one** product-shell
flow config. New task classes ship as category-keyed rule packs +
persona bundles, not as new flow configs.

## SemTeams flow configs

| Config | Purpose | Model |
|---|---|---|
| [`flow-bootstrap.json`](flow-bootstrap.json) | Production substrate (ADR-042 MVP). Wires the singleton agentic stack (graph-ingest, graph-query, rule-processor, agentic-loop, agentic-dispatch, agentic-tools, agentic-model) + the live category rule packs (research, autoresearch, coordinator, ops) + the persona corpus that drives them. | claude-haiku (default); gemini-flash / claude-sonnet selectable per role |
| [`e2e-flow-bootstrap.json`](e2e-flow-bootstrap.json) | Mock-LLM clone of flow-bootstrap. Same packs + personas; model registry points at the in-process mock LLM. Used by every Playwright journey under `ui/e2e/agentic/`. | mock-llm |

`task ui:test:e2e:agentic:research-mvp` runs the e2e config end to
end against the mock LLM (no API keys); `task dev:research` runs
the production config (needs `ANTHROPIC_API_KEY` at minimum).

## What lives where

```
configs/
├── flow-bootstrap.json          ← production substrate
├── e2e-flow-bootstrap.json      ← mock-LLM clone
├── rules/
│   ├── coordinator/             ← chat front-door rules
│   ├── research/                ← prose research arc
│   ├── autoresearch/            ← Karpathy-style iteration loop
│   └── ops/                     ← parallel observability track
└── personas/fragments/
    ├── coordinator/             ← domain-neutral router persona
    ├── researcher-research-*/   ← plan / gather / synthesize roles
    ├── reviewer-research/
    ├── autoresearch-*/          ← baseline / propose / execute / synthesize
    ├── reviewer-autoresearch/
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
