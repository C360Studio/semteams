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

## Addendum 2026-10-01: execution-correlated approval projection

The frozen SemStreams migration requires SemTeams to handle repeated approval-pending events after an answer.
Review of the initial receipt implementation also reproduced answer-before-pending and delayed-answer ordering
failures: loop-only resume markers could block every later gate or resume a different execution. Clearing all markers
could delete a newly arrived gate. These are product run-phase projection defects; loop/tool admission stays upstream.

### Decision and framework alignment

Use the existing canonical graph owner to maintain exactly three predicates on an existing local run entity:
`agent.run.approval-pending`, `agent.run.approval-answered`, and `agent.run.approval-outstanding`. The two sets contain
canonical JSON string pairs `[bareLoopID, executionID]`; JSON encoding preserves opaque execution IDs without delimiter
ambiguity. An answer records both pairs, so answered is always a subset of pending. The scalar is their set difference
cardinality. The sets grow monotonically; annotations on existing tuples are preserved. These are literal JSON values,
not graph edges. Vocabulary declares the pairs as `json` and the derived count as `int`.

The product adapter exact-reads the graph owner and submits a `graph.ReconcilePredicatesRequest` with that same
`ExpectedRevision`. It binds the effective local authority, `chain.agent.execution` identity and existing
`lifecycle.harness.v1` type. Its fixed `projection.Contract` declares one `ModeReconcile` group containing only these
three predicates. Contract validation requires their vocabulary declarations before construction. This is a write to
an already registered framework entity, so it creates no payload type or entity-birth registration obligation.

Framework survey at SHA `8b99efe9c66a4faa4fa509f9f62cc6bad8392128`:

- [ADR-091's canonical mutation protocol][approval-protocol] and the public
  [ReconcilePredicatesRequest][approval-request] already provide the revision fence. The documented operation is
  `graph.mutation.entity.reconcile`, backed by the configured graph-ingest canonical mutation port.
- [ExactEntityReader][approval-exact] returns validated entity bytes and their same-entry authority revision. It
  distinguishes absent, invalid and unavailable authority; none becomes an empty successful approval history.
- The public [projection MutationClient][approval-client] accepts Desired, then independently reads a revision inside
  `Reconcile`. It cannot fence a desired set computed from the caller's earlier snapshot. An intervening gate could be
  overwritten despite passing that later revision check. Calling it repeatedly does not repair this mismatch.
- The lower-level `internal/graphmutation.Client` cannot be imported by SemTeams. The product adapter consumes only
  the existing public typed operation; it is not a copy of the internal client or a general graph client.
- Rule `.length` substitution counts the first triple object's list elements, whereas `length_*` operators count
  matching triples. Comparing those for two multivalue pair sets is invalid; executable frozen-rule tests rejected
  that attempted expression. No new rule action, operator, store, bucket, stream, worker or authority is introduced.

Only a definite classified revision conflict causes a fresh read, recomputation and retry, bounded to eight attempts
and the caller context with a five-second ceiling. One logical event retains its request ID and provenance across
conflicts. Classified server refusals and no responders are definite noncommits. Other post-dispatch transport failures
or invalid replies are commit-unknown and are not retried or inferred successful from matching stored content. Accepted
receipts must match entity identity, type, request ID and desired facts; applied revisions advance the fence and unchanged
revisions equal it. Malformed or inconsistent existing history fails closed, including on duplicate events.

### Lifecycle and migration boundaries

The adapter never writes run phase or lifecycle audit. Rule 12 pauses only an executing run with outstanding gates and
reserves transition reason `tool approval pending`. Rule 13 resumes only an awaiting run with zero outstanding gates,
that exact reason, source `rule`, previous phase `executing`, and no pending clarification. Phase and audit are one
framework lifecycle transition. The source and previous-phase guards matter because an empty later transition note
can retain an older note. An already selected resume can briefly precede a newer gate's pause, but no history is removed,
so the rules converge without losing the newer gate. Terminal phases never re-enter execution through these rules.

Fresh graph storage is required. There is no interpretation of the old loop-reference marker format. Terminal runs
retain history and the UI suppresses actionable waits for completed, failed or cancelled runs. A live loop's
`pending_approval.execution_id` remains the only approval request identity; these graph facts never authorize a tool.
Core-NATS event loss remains a separate existing limitation; this change adds neither replay nor repair workers.

The architect and independent Go reviewer approved this narrow existing-protocol adaptation for TDD. Behavioral tests
exercise answer-first, failed pending mutation, late A while B waits, separate concurrent writers, subscriber recreation,
phase-transition races, terminal history, refusal/ambiguity and cancellation. Actual graph-owner browser qualification
is recorded with the migration evidence rather than inferred from the protocol fixture.

Migration posture: adopt an upstream public projection operation that accepts the caller's exact revision, or an
upstream run-approval projection with equivalent ordering guarantees, when available. Replace this domain adapter at
that point. Do not grow it into a generic mutation client or use it to bypass another framework contract. This migration
neither expands SemEngine's initial release nor enables SemSource-backed dogfooding.

[approval-protocol]: https://github.com/C360Studio/semstreams/blob/8b99efe9c66a4faa4fa509f9f62cc6bad8392128/docs/adr/091-graph-mutation-authority-without-semantic-ownership.md
[approval-request]: https://github.com/C360Studio/semstreams/blob/8b99efe9c66a4faa4fa509f9f62cc6bad8392128/graph/mutation_requests.go
[approval-exact]: https://github.com/C360Studio/semstreams/blob/8b99efe9c66a4faa4fa509f9f62cc6bad8392128/graph/exact_entity.go
[approval-client]: https://github.com/C360Studio/semstreams/blob/8b99efe9c66a4faa4fa509f9f62cc6bad8392128/pkg/projection/mutation_client.go
