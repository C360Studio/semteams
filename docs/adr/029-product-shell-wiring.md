# ADR-029: Product-Shell Wiring of Framework Primitives

## Status

Accepted — 2026-04-23

## Context

Semteams is a product shell over the semstreams framework. It imports
semstreams as a Go module dependency (`github.com/c360studio/semstreams`,
currently `v1.0.0-beta.9`) and ships a thin `cmd/semteams/` binary that
wraps `componentregistry.Register`. Per semstreams ADR-028 (upstream),
framework code stays free of product opinion; products compose framework
primitives in their own binaries.

Upstream ADR-029 ("Instance-Type Patterns") names three patterns for how
the framework exposes new instance types:

| Pattern | Used for | Wiring |
|---|---|---|
| **A — Boot-registry** | tools, payload factories, command registry | global singleton, `init()` or boot-time `Register`, no persistence |
| **B — KV-backed CRUD Manager** | rules, flows, **personas**, flow-templates | `{Type}Manager` over a NATS KV bucket + matching `{Type}Executor` |
| **C — Lifecycle + factory registry** | components, services | `{Type}Registry` of factories + `{Type}Manager` that owns Start/Stop |

Upstream's `cmd/semstreams/main.go` demonstrates each pattern's wiring.
It is **reference-by-example**, not shared code. Downstream products
(semteams, semspec, semdragons, and anyone building on the framework)
replicate the same wiring in their own `main.go`.

During the 2026-04-23 coordinator-pattern investigation we discovered
that `cmd/semteams/main.go` had silently drifted from the canonical
wiring:

- Pattern C registration was present (`componentregistry.Register`).
- Pattern B `persona.Manager` was **not** loaded at startup, so the
  PERSONAS KV bucket was empty and `agentic-loop`'s per-role prompt
  assembly could not ground anything. Fixed in commit c226c50.
- Pattern A/B **tool executor registration** (`executors.RegisterAll`)
  is not called. `web_search` still works because it self-registers in
  an `init()`; stateful tools that need a NATS client, persona manager,
  or loop bucket (`decide`, `read_loop_result`, `create_rule`,
  `update_persona`, …) have no runtime executor. The coordinator pattern
  cannot run without these.

The drift was invisible until a journey tried to exercise it. The fix
was obvious only because upstream's `cmd/semstreams/main.go` documented
the pattern by example.

## Decision

### Principle

`cmd/semteams/main.go` independently implements every framework-wiring
pattern the product relies on. It does **not** import anything from
`cmd/semstreams/`, including helper functions, dependency-builders, or
wiring code. Upstream's `main.go` is reference, not library.

This intentional duplication:

- keeps semstreams free of product opinion (ADR-028 upstream);
- teaches product builders how to wire the framework by showing the
  wiring in their own binary;
- lets each product adopt new framework features on its own schedule,
  without transitive breakage from upstream main.go refactors.

Approximate scope: a well-wired product binary has ~50 lines of boot
code that mirrors upstream's equivalents. That's the cost of admission
and it is load-bearing documentation.

### Wirings semteams adopts today

| Framework surface | Pattern | Call site in `cmd/semteams/main.go` | Status |
|---|---|---|---|
| `componentregistry.Register` | C | `setupRegistriesAndManager` | ✅ live |
| `persona.NewManager` + `persona.LoadFromDirectory` | B | `loadPersonaFragments` after services configured | ✅ live (c226c50) |
| `rule.NewConfigManager` (+ `InitializeKVStore`) | B | `buildRuleManager`, passed to `executors.RegisterAll` | ✅ live |
| `flowstore.NewManager` | B | `buildFlowManager`, passed to `executors.RegisterAll` | ✅ live |
| `flowtemplate.NewManager` | B | `buildFlowTemplateManager`, passed to `executors.RegisterAll` | ✅ live |
| `executors.RegisterAll` | A + B tool executors | after persona load, before `StartAll` | ✅ live |

All four Pattern-B managers are wired at boot. Wire-once discipline: the
alternative (deferring rule/flow/flow-template managers until a journey
asks for them) is exactly the silent-drift failure mode this ADR exists
to prevent. Builder functions return nil on KV init failure, and each
`register*` inside `executors.RegisterAll` skips when its manager is nil,
so boot remains resilient to partial NATS unavailability.

### Wirings deferred

- `registerExampleComponents` (upstream's `iot_sensor`, `document`) —
  semteams does **not** register these. They are framework example
  processors; keeping them out of our binary is part of the product
  shell's job. If a semteams journey needs them, they become product
  components here.

### Verification

Every adopted wiring must be demonstrable at boot:

- `persona.LoadFromDirectory`: backend log shows `"loading persona
  fragments"` and `"persona file loader: load complete fragments=N"`;
  `agentic-loop` logs `"persona overrides seeded count=N"` on
  `initPromptRegistry`.
- `executors.RegisterAll`: a Pattern B tool (`read_loop_result`) called
  from a fixture resolves to its executor and returns a non-error result.
- `componentregistry.Register`: `"Component factories registered"` log
  line with `count=N` matching the configs the binary supports.

## Consequences

### Positive

- Drift between framework capability and product wiring becomes
  visible: a missing wiring is a missing log line, not a silent
  feature gap.
- Downstream products (semspec, semdragons, others) have a clear
  reference for what their own `main.go` should do — two working
  examples (upstream canon + semteams) instead of one.
- No implicit coupling between semteams and upstream's main.go
  refactoring cadence.

### Negative

- ~50 lines of boot code duplicated across each product binary. When a
  new Pattern lands upstream, every product binary must update. The
  alternative (shared main helper) would hide what we are trying to
  teach.

### Neutral

- This ADR does not commit semteams to wiring every upstream pattern.
  It commits semteams to *knowing which patterns it has wired* and
  *matching upstream's shape when it does wire one*.

## Alternatives considered

- **Import `cmd/semstreams/` into `cmd/semteams/`.** Rejected —
  violates framework/product split (ADR-028 upstream) and would mean
  framework main.go changes transitively break product builds.
- **Extract a shared "default main" helper library** (e.g.
  `semstreams/mainhelper`). Rejected — hides the contract the wiring
  patterns are meant to teach. New product authors would see a one-line
  helper call and not learn what to adopt.
- **Defer everything until a real product journey demands it.**
  Rejected for `executors.RegisterAll` specifically: the coordinator
  pattern is explicitly next on our roadmap and it cannot run without
  the Pattern B tools wired.

## Related decisions

- semstreams ADR-028 (upstream) — framework/product split.
- semstreams ADR-029 (upstream) — instance-type patterns.
- semstreams ADR-026 (upstream) — coordinator agent; requires
  `executors.RegisterAll` tool wiring in the consuming product.
- semteams ADR-025 — product-shell consolidation; this ADR is the
  concrete wiring contract that consolidation requires.
