# ADR-059: Adopt semstreams v1.0.0-beta.160 (graph/framework foundation cutover)

**Status:** Accepted (2026-08-14)

## Context

Upstream published `v1.0.0-beta.160` on 2026-08-12 as "the downstream
migration checkpoint for the graph and framework foundation refactor" —
intentionally breaking, with no compatibility shims, aliases, dual
formats, or state migration. 133 commits / 1,247 files over beta.159.
The release-attached `migration-beta159-to-beta160.md` is the canonical
downstream entry point; five companion guides in upstream
`docs/operations/` carry the detailed contracts. The adoption contract
requires starting on **freshly provisioned NATS storage** (there is no
migration path; never point beta.160 at beta.159 volumes) and NATS
server `2.14.4`.

The load-bearing upstream changes for this product shell:

- **Ownership substrate deleted.** `pkg/ownership` (registries,
  owner-token leases, heartbeaters, `OWNER_PRESENCE`), `WireOwnership`,
  and `BindRulePackContracts` are gone. Graph writes go through one
  typed `semstreams.graph.mutation/v1` `nats-request` port with four
  operations (`entity.create`, `entity.reconcile`, `triple.append`,
  `entity.delete`), CAS-fenced per mutation against local
  `pkg/projection.Contract` intent. Append is must-exist; nothing
  vivifies missing entities.
- **Rule dialect**: `replace_owned` → `reconcile_predicates` (same
  clear-the-group / author-one-triple semantics); contract groups use
  mode `reconcile`.
- **Strict port envelope**: every component port is
  `{name, required?, description?, config: {kind, ...}}` with a closed
  kind set; jetstream configs take `stream_name` + a `subjects` array;
  side lanes (`kv_write`, `kv_read`) are rejected at decode. Overrides
  are complete named replacements and must match the default's kind.
- **Services are restart-only composition**: `ServiceConfig.Name` is
  deleted (map key is the identity), inner `log_level` /
  `enable_kv_query` / `include_go_metrics` fields are strict-decode
  errors, and `ConfigureFromServices` now *creates* every enabled
  service itself.
- **Tool discovery** moved to `nats-request` on `discovery.tool.list`;
  the request subject must not be stream-captured, so `tool.>` capture
  is replaced by `tool.execute.>` + `tool.result.>`.
- **Trajectories are immutable audit facts**; the `/teams-loop/*` HTTP
  surface (including the trajectory endpoints the UI consumed) is
  deleted. The GraphQL `trajectory` field on graph-gateway is the sole
  public trajectory query API; full evidence is content-addressed
  through the store named by `trajectory_evidence_storage_instance`.
- **MilestoneSubscriber is observe-only** — no `TriplePublisher`, and
  it no longer auto-transitions a dispatched root run to
  failed/cancelled (the "D3 zombie guard" is deleted).
- **Config version discipline**: an equal-or-older top-level `version`
  does not replace the configuration already selected from KV.

ADR-058's realignment (canonical predicates, parked dev packs, 4-action
taxonomy, team-hint commands) carries forward unchanged — this ADR is
the next flag-day on top of it.

## Decision

Adopt `v1.0.0-beta.160` in one flag-day pass on
`chore/semstreams-beta160`, keeping the ADR-058 demo scope. The
product-shell decisions, in dependency order:

1. **Graph runtime wiring** (`cmd/semteams/main.go`, ADR-029 mirror):
   `service.WireGraphRuntime(ctx, nats, logger, builtinProjectionContracts()...)`
   replaces the ownership substrate wholesale (no shutdown joins — the
   mutation client runs no background goroutines);
   `service.ConfigureRulePackMutations(manager)` is the single §11b
   bind site. `builtinProjectionContracts` re-mirrors upstream: the
   todos group is ONE rule-opaque `agent.todo.record` literal in
   `reconcile` mode.
2. **`configureAndCreateServices` collapsed** to the single
   `ConfigureFromServices` call — the pre-160 per-service
   `CreateService` loop double-registers every service at 160.
3. **Both bootstraps re-authored to the strict contract** (config
   version `1.1.0` → `2.0.0`): canonical port envelopes throughout;
   `graph_mutations` as a required `nats-request` on `graph.mutation.>`
   (multi-token operation subjects need `>`, not `*`) with the
   mutation interface on both graph-ingest (input) and the rule
   processor (output); graph-query's input renamed to the validated
   `graph_queries` / `graph.query.*` / `graph.query/v1` shape;
   `teams-dispatch.user.response` flipped `nats` → `jetstream`/USER
   (`user.response.>` — the upstream default; kind-mismatch overrides
   are boot errors); the TOOL stream narrowed to
   `["tool.execute.>", "tool.result.>"]`.
4. **graph-gateway wired into both bootstraps** (new component,
   canonical three-output shape, ServiceManager HTTP surface on 8080 —
   `standalone_server` stays false; Caddy's `/graphql` route repointed
   8082 → 8080). This restores the UI graph-explorer read path — the
   `/graphql` route had pointed at a port nothing served since the
   MVP-7 config retirement — and provides the trajectory query API the
   UI now requires. Framework-alignment review: pure upstream
   primitive, zero product-local code.
5. **objectstore wired into both bootstraps** (`AGENT_CONTENT`): the
   loop's `trajectory_evidence_storage_instance` default named a store
   nothing provided — observed live as a loud `provider_resolution`
   ERROR on the first successful boot. Evidence bodies now resolve.
6. **Rule `agent-run/04b` (dispatched→failed) replaces the deleted D3
   zombie guard.** Rules 05/06 stamp the durable
   `agent.run.outcome=failed` marker on any involuntary coordinator
   terminal keyed on the run anchor (handoff or not); 04b consumes it
   under a `dispatched` phase guard exactly as rule 04 does under
   `executing`. Four wired artifacts that documented depending on the
   deleted framework guard were rewritten. Residual gap (same as the
   D3 era): a hard coordinator *crash* that emits no terminal event
   stamps no marker — that class was never covered by the
   event-driven guard either and stays an operator-visible wedge.
7. **UI trajectory surface** re-built on the GraphQL `trajectory`
   field (facts + previews + storage references — no bodies). Product
   regressions accepted for this pass and recorded in commit
   `9f4e9aea`: decide-verdict chips, model prose, ArtifactCard /
   ProofReadinessCard rendering, and the truncated-outcome label all
   lose their data source. The follow-up is an **evidence-fetch pass**
   that dereferences `StorageReference`s through an authorized store
   reader; the orphaned components are left in the tree for it.
8. **Parked packs stay parked** (ADR-058 posture). The dev-via-test
   remover's raw `graph.mutation.triple.remove` wire no longer exists;
   it now fails loudly with a named error directing re-authoring onto
   `reconcile_predicates` before any re-wiring. The parked files stay
   in the pre-160 `replace_owned` dialect; their owned-lane contract
   pins are parked with them.
9. **Deliberate ADR-029 divergence, documented in `run()`**: upstream's
   `rulepackcap.ValidateConfig` + `graphresearch.ValidateConfig` boot
   gates are omitted — SemTeams composes packs in component config and
   wires no capability blocks, so both are structural no-ops here.

## Live-verification findings (2026-08-14)

The boot contract was proven by cycling the research-mvp mock journey
on fresh volumes; each cycle surfaced one runtime semantic that static
recon (including an empirical decode harness) could not see:

1. `message-logger.enable_kv_query` — retired field, strict-decode
   boot abort.
2. `metrics.include_go_metrics` — same class, plus the same field in
   the product shell's own `ensureMetricsConfig` seeding.
3. `ConfigureFromServices` creates services — the mirrored per-service
   loop aborted boot with "already registered".
4. Trajectory evidence store: boots green with facts recording but a
   loud per-fact `provider_resolution` ERROR without a live
   `objectstore` — the "boots green but degraded" class the migration
   guide warns about.

research-mvp GREEN end-to-end on beta.160 (six loops, thirteen mock
round-trips) after fix 3; the full journey sweep and the real-LLM smoke
are this migration's product proof and run once, here.

## Consequences

- Fresh NATS storage on every stack crossing the boundary (`down -v`;
  blue/green for anything long-lived). Compose files pin
  `nats:2.14.4-alpine`.
- The governed-SKG ownership model (ADR-054/055/056/057 owner-lease
  compliance) is superseded by per-mutation projection-contract
  validation; `enforce_owner_lease` and owner-token machinery have no
  beta.160 equivalent. semstreams#313 (HITL owned-write lane) should be
  re-evaluated against the new mutation port before any HITL work.
- Per-rule `entity.pattern` narrowing (the blanket `*.*.*.*.*.*` from
  ADR-058 finding 6/9) is **done**. Every wired rule fires on one of two
  entity families, and which one is derivable from the rule's own
  conditions rather than a hand-maintained list: `agent.loop.*` /
  `coordinator.decision.*` / `agent.lineage.*` facts live on the loop
  entity (`*.*.agent.agentic-loop.execution.*`, 32 rules), `agent.run.*`
  facts live on the run entity (`*.*.agent.chain.execution.*`, 8 rules).
  `autoresearch/05` was already narrow. The change is behaviourally
  inert: non-array operators return false on a missing predicate, so
  narrowing only skips evaluations guaranteed not to fire.
  - **Hazard for the next rule author**: *array* operators do NOT behave
    that way. A missing predicate is given an EMPTY LIST, so
    `length_eq 0` returns **true** on an entity lacking the field
    entirely. Rules agent-run/05, 07, 09 and 12 depend on `length_eq 0`
    and are safe only because each is AND-composed with a family-pinning
    `eq`. A rule whose conditions were all array operators would match
    every entity the watcher delivers.
  - Fenced by `TestWiredRulesNarrowEntityPattern`: blanket pattern, wrong
    family for the conditions, a pattern unreachable under either
    bootstrap's `entity_watch_buckets`, and unclassifiable conditions.
    The firing-entity containment rule that ADR-058 finding 9 hit is
    fenced separately and pre-existing, by
    `TestReconcile_ResolvedTargetsWithinContractScope`.
  - The unwired `configs/rules/ops/` pack deliberately keeps the blanket:
    the fence covers bootstrap-wired rules only, so re-wiring ops trips
    it and forces the narrowing decision at that point.
  - Component-level `entity_watch_buckets` stays broad, per the existing
    decision pinned in `test/contract/flow_bootstrap_test.go`.
  - Journey verdict: 15 green / 5 parked (`describe.skip`) / 1 red —
    identical to the pre-change beta.160 baseline. The red is
    `ops-agent`, which cannot pass because its rules were never wired
    into either bootstrap (its spec header still cites
    `configs/e2e-ops-observer.json`, deleted in ADR-042 MVP-7). The
    load-bearing evidence is `run-failed`: it asserts on `agent.run.phase`
    triples reaching terminal `failed` via `research/09` (loop family)
    stamping `agent.run.outcome` which `agent-run/04` (run family) then
    consumes — so both narrowed families and the handoff between them are
    confirmed empirically, not merely by static argument.
  - **Residual**: agent-run rules 09/11/12/13 (clarification and approval
    pause/resume) get no journey coverage — `ask-user-pause` and
    `clarification-resume` are parked, and the approval pair is not in
    the `test:e2e` chain. Their pattern is the same run-family string
    proven by 02/03/04/04b, so what is unexercised is those rules'
    conditions, not the narrowing.
- The evidence-fetch UI pass (decision 7) is the gate for restoring
  rich trajectory rendering.
