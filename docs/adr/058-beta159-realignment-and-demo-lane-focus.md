# ADR-058: Realign to semstreams v1.0.0-beta.159 and focus on the demo lanes

**Status:** Accepted (2026-08-07)

## Context

SemTeams was pinned to semstreams `v1.0.0-beta.115` (2026-06-24) while
upstream moved 44 betas ahead, several of them breaking, with a larger
refactor still in progress upstream. The load-bearing upstream changes for
this product shell:

- **Canonical predicate contract** (upstream ADR-074, accepted 2026-07-14,
  enforced fail-closed at persistence across beta.147–150): every stored
  predicate is exactly three lower-kebab dot-segments. There is no alias
  mode and no escape hatch. 73% of our rule-pack predicate vocabulary was
  illegal under it, and rule-pack JSON never recompiles — configs are the
  silent casualty class. Sticky poisoning makes this worse: graph-index
  degrades for the process lifetime on reading one bad predicate, and
  `GET /graph/triples` hard-fails the whole request on a poisoned entity.
- **Fail-closed ownership wiring**: `WireOwnership` returns
  `(Registry, Heartbeater, MutationClient, error)` and aborts boot on
  failure; the `MutationClient` must thread into `executors.RegisterBuiltins`;
  `BindRulePackContracts` returns a boot-gating error.
- **Rule-processor config contract**: `pack_id` universally required;
  `entity_watch_patterns` deleted in favor of `entity_watch_buckets`
  (six-position patterns on ENTITY_STATES only); `enable_graph_integration`
  default flipped to false; every `replace_owned` action requires
  `projection_contract` + `projection_group`; per-rule `entity.pattern`
  must be an exact six-position wildcard pattern.
- **Stream bounds mandate**: every declared stream must carry
  `max_age` + `max_bytes` + `discard` or the config fails validation
  before NATS is even dialed.
- **Vocabulary renames**: the underscore forms of upstream constants are
  gone (`agent.run.entity-id`, `coordinator.decision.next-action`,
  `agent.loop.cost-usd`, …); the lineage namespace moved to
  `agent.lineage.<key>`; the spawned-task marker is `rule.task.spawned`.

Meanwhile the product goal for this phase narrowed to a basic demo of the
two loops that define the harness: the **outer loop** (a human asks through
the front door; the coordinator classifies, routes or answers; the result
comes back as a reviewable artifact) and the **inner loop** (a category arc
plans, fans out N gatherer loops, joins, synthesizes, and is reviewed).
The research and autoresearch packs are those two loops. The dev-side packs
(dev-via-test, create-change, proof-readiness, dev-from-task) are not needed
for that demo, and migrating their ~40 files of pre-contract predicates
would have roughly doubled this realignment for surface the demo does not
exercise.

## Decision

1. **Pin to the current upstream tag** (`v1.0.0-beta.159`, the same tag the
   most current sibling shell runs) rather than trailing further behind or
   chasing HEAD mid-refactor. Adopt its wiring contracts in
   `cmd/semteams/main.go` verbatim-mirrored from upstream per ADR-029.
2. **Re-author the kept lanes to the canonical predicate contract**:
   coordinator, research, autoresearch, agent-run, and ops — rules,
   personas, product-shell Go emitters, UI readers, contract tests, mock
   fixtures, and Playwright journeys. SemTeams-local vocabulary was
   re-authored (not aliased): pause markers are
   `agent.run.{approval,clarification}-{pending,resumed}`, clarification
   prose lives at `coordinator.clarification.{question,reply}`, run-scoped
   autoresearch config at `autoresearch.run.*`, and sandbox capability
   attestations at `sandbox.attestation.verified-<kebab-probe>`.
3. **Park the dev-side packs in place.** Their rule files, personas, Go
   tools, contract tests, fixtures, and journeys stay in-repo in the
   pre-migration dialect, but nothing is wired: they are out of
   `flow-bootstrap.json`, out of the coordinator's closed taxonomy (now
   `research | autoresearch | respond_direct | ask_user`), out of the UI's
   chips/slash hints, and their tests/journeys are excluded via a build tag
   / `describe.skip`. The coordinator answers parked-team asks with an
   honest `respond_direct` — never a dead-end token, never a silent hang.
4. **Fence the contract in CI.** `test/contract/predicate_contract_test.go`
   audits every bootstrap-wired rule file (condition fields, action
   predicates, `$entity.triple.*` tokens, `related_loops` keys) and both
   configs' projection contracts against the canonical grammar. Re-wiring a
   parked pack without re-authoring it fails CI. The taxonomy linter
   (`coordinator_slice1_test.go`) keeps persona and rule layer bidirectionally
   consistent so no action token can dead-end.

## Consequences

- The demo surface — front door plus research/autoresearch arcs with the
  agent-run lifecycle and ops observers — runs on a current framework and
  canonical predicates, end to end.
- Restoring a dev-side pack is a deliberate migration with a checklist:
  re-author its predicates (the Go emitters' concatenated
  `plan.task.<id>.<field>` / `change.<slug>.…` families need a design pass,
  not just renames — identity belongs in the entity ID or a JSON-object
  value under the three-segment contract), re-wire `rules_files`, restore
  the taxonomy token + persona rows, drop the `parked_packs` build tag,
  un-skip the journeys, and re-add the Taskfile entries. Every parked
  surface carries a restore note pointing here.
- Known dialect residue is confined to parked surfaces; the predicate audit
  is the tripwire that keeps it there.
- Persisted graph data written under the old dialect (developer
  environments, old NATS volumes) is unreadable poison to beta.159 readers:
  always `docker compose down -v` / fresh buckets when crossing this
  boundary. There is no in-place data migration.
- The `$entity.triple.*` first-written-wins substitution ambiguity is NOT
  fixed at beta.159 (verified in the rule engine source); the agent-run
  01/01b dual-anchor workaround remains load-bearing.

## Alternatives considered

- **Migrate everything at once** — rejected: roughly doubles the pass for
  packs the demo does not exercise, and the dev-lane emitters need a real
  design pass (4-to-6-segment concatenated predicate families), not a
  mechanical rename.
- **Stay on beta.115** — rejected: the gap only grows, sibling shells are
  already current, and the pre-contract dialect is now unwritable upstream;
  every week of drift adds re-authoring surface.
- **Alias/compat layer in the product shell** — rejected: upstream ADR-074
  explicitly refuses dual formats, and a product-local alias layer would
  re-introduce exactly the historical-dialect reasoning upstream removed;
  it would also break the moment the framework reads our writes.

## Related

- [ADR-029](029-product-shell-wiring.md) — the wiring-mirror contract this
  realignment re-exercised.
- [ADR-042](042-coordinator-instantiated-flows-via-templates.md) —
  substrate-plus-overlays; parking a pack is exactly the overlay model
  working in reverse.
- [ADR-053](053-adoption-plan.md) — the run-lifecycle machinery whose
  markers were re-authored here.
- Upstream ADR-074 (canonical predicate contract) and the beta.147–159
  release arc in the semstreams repository.
