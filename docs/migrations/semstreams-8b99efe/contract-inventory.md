# Frozen SemStreams migration contract inventory

This is the architect's pre-implementation contract inventory for [#280][issue], prepared on 2026-10-01.
It authorizes compatibility work and defines qualification obligations; it does not report tests as passed.
The behavioral comparison and execution evidence belong beside this inventory, not in this inventory's assertions.

## Provenance and scope

- SemTeams baseline: `ce22c961d3` on `main`, using `v1.0.0-beta.160`.
- Frozen SemStreams: `8b99efe9c66a4faa4fa509f9f62cc6bad8392128`.
- Exact module: `v1.0.0-beta.162.0.20260930150212-8b99efe9c66a`.
- Reconnaissance read the module cache at that exact version and the baseline via `git show ce22c961:<path>`.
- [Frozen migration ledger][wave] and the frozen source are authoritative. Sections labelled beta.163/beta.165
  describe code already present in this SHA; their titles do not authorize substituting another release.
- Draft [#270][prior] is reference material only. Its beta.161 changes do not cover the frozen SHA's later removals.

The product remains one substrate plus category packs. The shipped product-facing packs are research and autoresearch.
The target evidence-backed program pulse is not shipped by this migration. Parked implementation packs remain parked.
SemSource-backed dogfooding waits for SemSource readiness; it does not block this SemStreams upgrade. A SemEngine switch
requires its own approved consumer contract and qualification. Measured SemTeams dependencies do not expand SemEngine's
first release automatically.

## Baseline declarations, before executing any tests

Both baseline bootstraps declare nine component instances: graph-ingest, graph-query, rule, teams-loop, agentic-model,
agentic-tools, teams-dispatch, graph-gateway, and objectstore. Both select 42 rule files: research 10, autoresearch 14,
coordinator 2, agent-run 15, and ops 1. The ops rule **is wired at this baseline**; ADR-059's earlier unwired finding is
historical evidence, not the state of `ce22c961`.

The baseline files are `configs/flow-bootstrap.json` and `configs/e2e-flow-bootstrap.json`, both version `2.0.0`.
The production platform declares `c360/semteams-bootstrap`, plus retired `instance_id: bootstrap-001`; mock declares
`c360/semteams-bootstrap-e2e`, plus `instance_id: bootstrap-e2e-001`. Baseline extraction prefers `instance_id`.
Production loops permit 50 iterations and 300 seconds; mock loops permit 8 iterations and 60 seconds, disable effective
compaction, and set a 60-second acknowledgement wait, 10-second heartbeat, and two deliveries. Mock tools gate
`create_rule`; production does not configure approval-required tools. Both retain unrestricted clarification policy.
Both declare AGENT_CONTENT storage and GraphQL through graph-gateway on the shared HTTP service.

These are configuration facts, not proof that every declared path works. Capture the actual effective configuration,
Compose project, image/module identities, ports, fixture, overrides, platform identity and storage policy for each run.

## Bootstrap and configuration contracts

### Composition

**Baseline exposure:** `main.go` imports `engine`, `flowstore`, `flowtemplate`; template loader and seed test
remain.

**Frozen contract and approved adaptation:** All three packages, FlowManager/FlowTemplateManager dependencies and
flow-builder service are removed. Delete their wiring and obsolete seeding. Do not copy retired framework
implementations into the product. [Flow retirement][wave-flow]

### Rule manager

**Baseline exposure:** `buildRuleManager` constructs the old manager and initializes KV independently.

**Frozen contract and approved adaptation:** Construct one `rule.NewConfigManager(logger)`, pass
`config.WithKeyFamily(rcm.KeyFamily())` to the config manager, and give that same manager to executors. Start it
after rule processors, stop it before them. [Configuration ownership][wave-config]

### Runtime contexts

**Baseline exposure:** `shutdown` supplies a duration to `manager.StopAll`.

**Frozen contract and approved adaptation:** `Start(ctx)` owns runtime lifetime; pass a fresh, separately bounded
shutdown context to `Stop(ctx)`/`StopAll(ctx)`. Preserve orderly cleanup while runtime authority remains live. No
stored contexts or duration adapter. [Lifecycle guide][lifecycle]

### Platform

**Baseline exposure:** `extractPlatformMeta` prefers removed `InstanceID`; configs predict the deployment prefix.

**Frozen contract and approved adaptation:** Remove `instance_id`; `platform.id` is the declared stem. Config
startup persists and adopts an effective `<stem>-<6hex>` identity. Obtain it after startup from the config manager;
fixtures must discover or explicitly pre-create it. [Authority][wave-authority]

### Configuration storage

**Baseline exposure:** Shared `semstreams_config` assumptions survive in comments and mirrored boot code.

**Frozen contract and approved adaptation:** Framework derives `semstreams_config_<org>_<stem>` through
`config.BucketName`. Environment is a label, not storage isolation. Key families are owned through the config
manager. [Configuration ownership][wave-config]

### Composition validation

**Baseline exposure:** Named input/output overrides predate the one-validator closure.

**Frozen contract and approved adaptation:** Required orphan inputs fail boot. Declare `external: true` only for
genuinely external inputs; keep necessary default declarations when overriding named ports. Validate both
bootstraps using the admitted catalog. Do not paper over a missing in-process owner as external. [Composition
contract][wave-composition]

### Loop storage

**Baseline exposure:** `teams-loop.config.loops_bucket` is present in both bootstraps.

**Frozen contract and approved adaptation:** Remove this retired loop key; the `loops` KV-write port names the
bucket. Observe History 10, TTL/MaxAge 24h, nonbinding MaxBytes. Tools' own `loops_bucket` is a separate unchanged
field. [Loop policy][wave-loop-policy]

### Approval timeout

**Baseline exposure:** Omission previously meant indefinite waiting.

**Frozen contract and approved adaptation:** Omission means 12h; supplied value must be positive and at most 12h.
Do not claim indefinite approval durability. [Loop policy][wave-loop-policy]

### Health

**Baseline exposure:** Existing Docker/probe behavior can conflate ready and healthy.

**Frozen contract and approved adaptation:** HTTP binds during startup; `/readyz` remains the readiness gate.
Milestone delivery-lane failure can make `/health` 503 while `/readyz` stays ready. Record both and do not use TCP
connect as readiness. [Startup guide][startup]


The exact public rule-manager recipe is verified in frozen `processor/rule/kv_config_integration.go`
(`NewConfigManager`, `KeyFamily`, `Start`, `Stop`) and `config.WithKeyFamily`. Frozen
`internal/boot/rule_config_service.go` demonstrates start-after/stop-before ordering. That adapter is internal and
must not be imported. The product may mirror composition-root ordering under ADR-029. If it uses the public recipe
with rule startup after `StartAll`, record that `/readyz` can precede rule reconciliation; do not claim the internal
adapter's readiness barrier without implementing the same ordering.

## Identity, graph, rules and payload contracts

### Entity identity

**Baseline exposure:** Rule patterns, projection groups, test fixtures and sandbox attestation use
`agent.agentic-loop` / `agent.chain`.

**Frozen contract and required evidence:** Canonical order is `org.platform.system.domain.type.instance`: loop
`agentic-loop.agent.execution`, run `chain.agent.execution`. Update owned builders, watcher patterns, rule subject
templates and consumers together. Lexical six-segment validation alone does not catch the old order. [Entity
ordering][wave-identity]

### Platform authority

**Baseline exposure:** Hard-coded fixture prefixes may remain lexically valid after reordering.

**Frozen contract and required evidence:** Graph births and local writes must match the effective deployment pair.
No foreign import lane is introduced here; SemSource is a separate service and dogfooding is held. Follow runtime
identity in GraphQL, probes and sandbox subjects. [Authority][wave-authority]

### Loop tokens

**Baseline exposure:** Tests can fabricate readable `loop_*` identifiers.

**Frozen contract and required evidence:** Loop tokens are canonical lowercase UUIDs; echo framework-issued IDs.
Test fixtures crossing an admission boundary must use canonical values. UUID form is not authentication. [Loop
admission][wave-admission]

### Projection contracts

**Baseline exposure:** `builtinProjectionContracts` copies framework loop/lesson contracts and uses string
MessageType keys.

**Frozen contract and required evidence:** Use `agentic.LoopExecutionContract()` and `agentic.LessonContract()`;
structured `message.Type` is the sole type authority. Keep payload registry injection and builtin registration. No
new product payload is needed. [Type authority][wave-types]

### Local vocabulary

**Baseline exposure:** `cmd/semteams/vocab/vocab.go` declares three `DataType: "number"` values.

**Frozen contract and required evidence:** Closed vocabulary rejects them at registration. Use `int` for the
iteration cap and `float` for measured/best values. Audit all local declarations. [Datatype guide][datatype]

### Coordinator replies

**Baseline exposure:** Two flat rule publishers target `user.response.*`.

**Frozen contract and required evidence:** This family carries registered `agentic.user_response.v1` only. Remove
unconsumed flat actions; preserve typed dispatch output interface. `violations.notify_user` is invalid even when
false. Logger observation is not end-user delivery. [Response guide][responses]

### Rule activation

**Baseline exposure:** File/inline packs and agent-created rules share the substrate.

**Frozen contract and required evidence:** Preserve the 42-file live pack set and prove rule CRUD reaches the same
active rule manager. Rule IDs must be a single KV literal token, with no dots. No new category or activation of
parked packs. [Configuration ownership][wave-config]

### Read tools

**Baseline exposure:** Personas/executors consume direct graph queries.

**Frozen contract and required evidence:** Relationships are outgoing reference triples; unsupported direction is
invalid. Neighbor depth now counts actual hops, type filters work, absent targets are unresolved and result size is
bounded. Attribute smaller/larger results to the specific new semantics. [Read-tool guide][graph-read]

### Graph/UI reads

**Baseline exposure:** GraphQL graph and trajectory clients coexist with flow inventory calls.

**Frozen contract and required evidence:** `/components/validate` and `/components/flowgraph` replace retired
composition judgments/inventory; `/gaps` and `/flowbuilder/*` do not exist. Graph entity queries remain distinct
from composition projection. [Flow retirement][wave-flow]


Graph rule qualification must include the cross-family handoff: coordinator loop writes a run anchor/handoff, run
transitions to executing, gather completions accumulate on the plan loop, the join dispatches synthesis once all
subtopics complete, review wakes the coordinator, and the run ends terminal. Never infer success from a loop count.
Preserve top-level lifecycle guards and the clarification-versus-tool-gate distinction.

## Agentic, tool, approval and terminal contracts

### Custom command authority

**Frozen obligation:** `LoopTracker` and `CommandContext.LoopTracker` are removed; `/implement-spec` must use
`LookupLoopOwner(ctx, id)`.

**Behavior to prove:** Correct owner admitted; foreign owner, absence, malformed record and unavailable lookup stay
distinct. This does not activate the parked implementation lane. [Dispatch authority][wave-dispatch]

### Dispatch ports

**Frozen obligation:** Keep the `agent_loops` read declaration and terminal inputs; removed created/pending input
overrides no longer feed a tracker.

**Behavior to prove:** `/loops`/debug/activity and explicit control use durable records; unavailable or poisoned
view is 503, not empty. Same user/channel routing is required for continuation. [Dispatch authority][wave-dispatch]

### Continuation

**Frozen obligation:** Existing live loops attach accumulated conversation; settled or in-flight gated/tool work
refuses continuation.

**Behavior to prove:** Coordinator follow-up preserves the intended thread; invalid/stale/foreign tokens do not
overwrite a loop or redirect its response. Auto-continue ambiguity is 409. [Admission][wave-admission]

### Cancellation

**Frozen obligation:** Generic `/loops/{id}/signal` and unsupported pause/resume/approve signal verbs are gone.

**Behavior to prove:** Send `/cancel <loopID>` through `/teams-dispatch/message` with the loop owner. Verify
terminal cancelled state, no continued user-visible work, and no assumption that HTTP acknowledgement alone means
cancellation completed. [Admission][wave-admission]

### Tool correlation

**Frozen obligation:** Execution identity replaces provider call ID for tool results/governance. Request IDs are
opaque correlation values.

**Behavior to prove:** Fixtures echo request/execution identities; they do not mint or parse UUID suffix
assumptions. Governed verdict payloads must carry execution identity plus recoverable loop/request identity. [Tool
identity][wave-tools]

### Approval request

**Frozen obligation:** HTTP approval body requires `execution_id`; acceptance echoes it.

**Behavior to prove:** Browser displays a specific pending gate and posts its identity. Missing identity returns
400; stale identity returns 409 without authorizing the next gate. Approve/reject/modify all preserve audit
correlation. [Approval HTTP][wave-approval]

### Approval replay

**Frozen obligation:** A gated result may republish the same pending event after an answer has been published.

**Behavior to prove:** Persist answered execution identity, keyed by loop and execution, so
pending→answer→duplicate pending cannot re-pause the run, including after subscriber recreation. The loop record
can still show the old gate during this window. [SemTeams exposure][wave-repause]

### Terminal ordering

**Frozen obligation:** Create-once `COMPLETE_<loopID>` precedes graph stamps, event, then terminal loop-record CAS.
First durable terminal wins.

**Behavior to prove:** Event consumers do not require an already-terminal loop record. SSE marker then loop update
cannot regress the terminal state. Failed/coordinator/cancelled paths retain run attribution. [Terminal
owner][wave-terminal]

### Terminal residuals

**Frozen obligation:** Record CAS conflict can leave a durable terminal above a live record until the next
terminal; missing retained approval continuation yields `continuation_unavailable`.

**Behavior to prove:** Report disagreement and its evidence honestly; no product-local repair worker or fabricated
completion. Distinguish record, durable marker, graph phase and trajectory observation. [Terminal
owner][wave-terminal]

### Sandbox

**Frozen obligation:** Existing `request_sandbox`, attestation, deny-by-default policy and container execution
remain the product boundary.

**Behavior to prove:** Fresh attestation uses the effective run ID; approval does not bypass sandbox restrictions;
unauthorized/expired/mismatched sandbox paths remain refused. No host-execution fallback.

### Autoresearch

**Frozen obligation:** Existing bounded empirical keep/revert loop remains a demonstration pack.

**Behavior to prove:** Baseline measurement, bounded experiment, best-value promotion on keep, revert/failure
behavior, cap, synthesis/review and terminal state all survive the identity changes.


The repeated-approval exposure is specifically documented upstream against SemTeams `ce22c961`, not guessed from
API differences. Its evidence is the pending-event replay window and the existing run rules 12/13 clearing transient
markers. A durable answer predicate using the existing graph append/reconcile contract is an adaptation; a new KV
bucket, daemon or framework fork is not authorized by this inventory. A stale answer must not mark a later execution
answered. Persisted execution correlation must distinguish repeated gates on the same loop.

## Trajectory and UI claims

GraphQL `trajectory(loopId, limit, cursor)` remains the public trajectory query surface. Qualify pagination,
observed totals, terminal observations, no-facts-yet behavior, and storage-reference presence against the frozen code.
Trajectory observations are audit evidence, not the authority for current loop state. Read failures must not be
reported as an empty, successful trajectory.

The existing [evidence-body rendering limitation #261][evidence] remains explicit. Baseline
`ui/src/lib/services/agentApi.ts` retrieves bounded facts and references; `TrajectoryViewer.svelte` does not dereference
full evidence bodies. Successful fact retrieval and AGENT_CONTENT storage do not restore model prose, tool arguments,
tool result bodies, decide verdict chips, ArtifactCard/ProofReadinessCard data or a full narrative. Tests that assert
those absent renderings measure an existing limitation, not a migration regression. Do not weaken them silently or
claim rich evidence rendering after proving only metadata access.

Remove the unsupported loop state `paused` and its control affordances. Preserve the distinct product-owned run
waiting concept (`clarification` or `tool_gate`), which reflects `awaiting_approval`. Keep useful graph exploration and
read-only composition inventory; retire unreachable saved-flow authoring and deployment controls. Regenerate schemas,
OpenAPI and TypeScript from the frozen registry. Remove the orphan `workflow-definition` schema with its exemptions.

## Qualification and difference attribution

Record each applicable Go/frontend gate and each selected mock browser journey against isolated fresh infrastructure
for both baseline and target. A passing mock fixture proves deterministic orchestration, not real-model research
quality or SemSource readiness. Production and mock bootstrap differences must be listed, including autonomous persona
and approval overrides, sandbox fixture, retries, limits and run directory. Monitor output, backend JSON logs and
message-logger/authoritative state during long journeys; terminal graph and loop timestamps outrank a quiet log.

Classify every observed difference as exactly one of:

1. **Intended upstream change:** name the frozen source/contract and show the changed expectation.
2. **Existing limitation:** reproduce at baseline or identify directly absent baseline capability, including #261.
3. **Reproduced defect:** preserve input, effective config, states/logs and minimal reproduction; state whether it
   occurs at baseline, target, or both. Do not relabel a failing critical path as an accepted upstream difference.

The qualification matrix must cover coordinator routing/direct reply/clarification; research fan-out/join/review;
autoresearch keep/revert/limits; approval and sandbox boundaries; completed/failed/cancelled run and loop handling;
trajectory access; and graph/UI behavior. Add focused regressions where upstream changed a boundary that existing
journeys do not exercise. Existing skipped donor journeys remain explicitly excluded. No real paid LLM run is implied.

## SemEngine measurement boundary

Report separately: direct SemStreams imports in production and tests; referenced exported symbols and call sites;
the production binary's package dependency closure; module dependency closure; dynamically selected component factories,
service registrations, tools, payloads, projection contracts, vocabulary, config/rule dialect and HTTP/GraphQL/NATS
contracts. Measure baseline and migrated target, because removing dead flow APIs changes closure without changing the
research product. Use compiler/AST-backed symbol resolution, `go list -deps -json` and module metadata; grep counts are
reconnaissance and must not be labelled a complete symbol inventory.

Dependencies pulled by `componentregistry.Register` or `payloadbuiltins.Register` may exceed the nine configured
components. Mark that registry closure separately from product requirements. Retained parked Go packages and their tests
are source obligations of this build, not first-release SemEngine capabilities. Every proposed SemEngine inclusion needs
its own consumer decision; absence from the initial release is a compatibility gap to assess later, not permission to
copy retired SemStreams facilities into SemTeams.

## Approval ordering review and accepted projection contract

Independent Go review reproduced two failures in the initial receipt adaptation: an answer delivered before pending
can leave a permanent resume guard, and a first delayed answer for execution A can resume execution B. Predicate-wide
cleanup can also erase B if B arrives after A's resume action was selected. The executable regression suite is
`cmd/semteams/approvalpause/rule_order_test.go`; `/tmp/semteams-approval-order-red.log` records the first behavioral red.
It runs the checked-in definitions through frozen `rule.Matches` and `rule.ActionExecutor`; lifecycle persistence is
a deterministic fixture. The initial mechanism failed review; the replacement below passes focused tests and awaits final browser qualification.

The UI contract uses append-only JSON string pairs `[bareLoopID, executionID]` on `agent.run.approval-pending` and
`agent.run.approval-answered`. JSON encoding makes delimiter-containing execution IDs unambiguous. An answer records
both pairs atomically, preserving answered as a subset of pending. Pair literals are not graph edges. UI subtraction
identifies waiting gates; current loop `pending_approval.execution_id` still authorizes the actual request. Terminal
runs retain historical facts and suppress actionable pause presentation. Fresh storage is required; legacy loop-reference
values are not interpreted by an alias or migration shim. Core-NATS event loss remains a separate existing limitation.

### Rejected count expression

The first proposed rules compared pending cardinality with `$entity.triple.agent.run.approval-answered.length`.
Actual frozen evaluation disproved it: `processor/rule/execution_context.go:458` scans for the **first** matching triple,
coerces that triple's object as a list, and returns the list's length. Each pair has length two regardless of the number
of answered gates. By contrast, `processor/rule/expression/evaluator.go:412` gathers all matching triples for a left-hand
`length_*` operator. These are different contracts. The `.triples` substitution enumerates values but cannot compose
with `.length` into a multivalue count. The supported operators expose no set difference or dynamic predicate-cardinality
right-hand side. `/tmp/semteams-approval-order-green.log` retains this still-red attempt; its filename is not a success
claim. That draft was rejected and replaced; no approval sign-off attaches to its expression.

### Accepted existing-protocol adaptation

The approved domain adapter exact-reads the run, unions the two tuple sets, computes their difference, and reconciles
those sets plus integer `agent.run.approval-outstanding` at the **same** authority revision. It preserves the existing
graph as sole authority, uses no new KV/stream/worker, and lets rules use scalar `gt 0` / `eq 0`. The constructor validates
an explicit three-predicate `ModeReconcile` contract and vocabulary; existing harness entities need no new type
registration. Bounded revision-conflict retry, cancellation, classified refusals, commit-unknown failures and independent
concurrent writers are tested. Terminal history remains. The durable decision and planned migration to an upstream
public revision-fenced projection API are in [ADR-029](../../adr/029-product-shell-wiring.md#addendum-2026-10-01-execution-correlated-approval-projection).

Frozen public evidence:

- `graph.ExactEntityReader` and `graph.NewExactEntityReader` in `graph/exact_entity.go` return validated entity bytes
  plus the same-entry `KVRevision`; the query is `graph.ingest.query.entity`.
- `graph.ReconcilePredicatesRequest` in `graph/mutation_requests.go:27` accepts `ExpectedRevision`, a predicate group,
  and its desired triples. `processor/graph-ingest/canonical_mutations.go:297` is the canonical owner implementation.
- `pkg/projection/contract` supplies `Contract`, `PredicateGroup`, and `ModeReconcile`; these validate caller intent,
  not exclusive ownership. Existing run/phase predicates remain lifecycle-owned and must never be in this group.
- The public `projection.MutationClient.Reconcile` (`pkg/projection/mutation_client.go:184`) **rereads authority after**
  the caller has supplied Desired, then uses its newly read revision. A Desired set computed from an earlier read can
  therefore overwrite an intervening gate without a conflict. Retrying that public call does not repair the mismatch.
- The request-fenced client is `internal/graphmutation.Client.Reconcile` (`internal/graphmutation/client.go:102`),
  unavailable to external Go modules. Sending the public typed protocol directly would need a new narrow product
  adapter and response validation; the reviewer approved the narrow domain operation, with strict response validation and no generic framework client.

Lifecycle transitions remain one action each. Rule 12 reserves reason `tool approval pending`.
Rule 13 must require that exact note, last source `rule`, last from `executing`, and no active clarification marker.
`pkg/lifecycle/manager.go:574` atomically projects phase and audit fields, but omits an empty note, so note alone is
insufficient to distinguish unrelated later pauses. A B arriving after A's resume was selected may briefly observe
executing, but B facts must survive and rule 12 must converge back to awaiting approval. No cleanup is permitted.

The reviewed alternative was to block pending a public revision-fenced projection operation or supported set/cardinality
expression. The architect and reviewer instead approved the existing documented wire operation, with a planned upstream
public-API replacement. No internal client was copied, private store introduced, or frozen SHA advanced. Focused tests
are green; browser qualification and independent final review remain separate evidence.

## Architect handoff

**Scoped contract sign-off: approved for TDD adaptation on 2026-10-01.** Backend and frontend lanes received these
mandatory changes before implementation. Approval covers the existing SemTeams product paths and removal/replacement
of frozen-retired surfaces. It does not assert qualification, authorize merging, authorize issue closure, enable
SemSource dogfooding, approve a SemEngine switch, or add product capabilities. Independent Go and frontend review plus
recorded local/browser evidence remain required before the migration can be called complete.

[issue]: https://github.com/C360Studio/semteams/issues/280
[prior]: https://github.com/C360Studio/semteams/pull/270
[evidence]: https://github.com/C360Studio/semteams/issues/261
[wave]: https://github.com/C360Studio/semstreams/blob/8b99efe9c66a4faa4fa509f9f62cc6bad8392128/docs/operations/migration-beta162-to-beta163.md
[wave-types]: https://github.com/C360Studio/semstreams/blob/8b99efe9c66a4faa4fa509f9f62cc6bad8392128/docs/operations/migration-beta162-to-beta163.md#single-type-authority-adr-103
[wave-composition]: https://github.com/C360Studio/semstreams/blob/8b99efe9c66a4faa4fa509f9f62cc6bad8392128/docs/operations/migration-beta162-to-beta163.md#composition-validation-substrate-adr-100--get-componentsgaps-is-removed
[wave-flow]: https://github.com/C360Studio/semstreams/blob/8b99efe9c66a4faa4fa509f9f62cc6bad8392128/docs/operations/migration-beta162-to-beta163.md#flow-authoring-retirement-adr-100-d5--the-saved-diagram-surface-is-removed
[wave-identity]: https://github.com/C360Studio/semstreams/blob/8b99efe9c66a4faa4fa509f9f62cc6bad8392128/docs/operations/migration-beta162-to-beta163.md#entity-id-segment-semantics-slice-a-adr-102-1095--the-canonical-order-is-orgplatformsystemdomaintypeinstance
[wave-authority]: https://github.com/C360Studio/semstreams/blob/8b99efe9c66a4faa4fa509f9f62cc6bad8392128/docs/operations/migration-beta162-to-beta163.md#unique-platform-authority-adr-104-1168--platformid-gains-a-framework-minted-suffix-on-first-boot
[wave-admission]: https://github.com/C360Studio/semstreams/blob/8b99efe9c66a4faa4fa509f9f62cc6bad8392128/docs/operations/migration-beta162-to-beta163.md#one-admission-gate-for-every-loop-naming-request-1227-1228-1225-1233--post-loopsidsignal-is-removed
[wave-tools]: https://github.com/C360Studio/semstreams/blob/8b99efe9c66a4faa4fa509f9f62cc6bad8392128/docs/operations/migration-beta162-to-beta163.md#tool-results-are-addressed-by-execution-identity-and-an-uncorrelated-tool-call-is-refused-1328
[wave-approval]: https://github.com/C360Studio/semstreams/blob/8b99efe9c66a4faa4fa509f9f62cc6bad8392128/docs/operations/migration-beta162-to-beta163.md#post-loopsidapproval-now-requires-execution_id-in-the-body--breaking
[wave-loop-policy]: https://github.com/C360Studio/semstreams/blob/8b99efe9c66a4faa4fa509f9f62cc6bad8392128/docs/operations/migration-beta162-to-beta163.md#loop-bucket-declaration-and-approval-wait-limit-1146
[wave-dispatch]: https://github.com/C360Studio/semstreams/blob/8b99efe9c66a4faa4fa509f9f62cc6bad8392128/docs/operations/migration-beta162-to-beta163.md#dispatch-reads-loop-authority-instead-of-tracking-notifications-1329
[wave-terminal]: https://github.com/C360Studio/semstreams/blob/8b99efe9c66a4faa4fa509f9f62cc6bad8392128/docs/operations/migration-beta162-to-beta163.md#one-terminal-owner-and-an-approval-answer-survives-a-process-replacement-1362-restart-safety-l4b
[wave-repause]: https://github.com/C360Studio/semstreams/blob/8b99efe9c66a4faa4fa509f9f62cc6bad8392128/docs/operations/migration-beta162-to-beta163.md#semteams--pinned-ce22c961-a-repeated-approvalpendingevent-re-pauses-a-resumed-run
[wave-config]: https://github.com/C360Studio/semstreams/blob/8b99efe9c66a4faa4fa509f9f62cc6bad8392128/docs/operations/migration-beta162-to-beta163.md#the-configuration-bucket-is-named-by-the-authority-pair-1188--breaking
[lifecycle]: https://github.com/C360Studio/semstreams/blob/8b99efe9c66a4faa4fa509f9f62cc6bad8392128/docs/operations/migration-restore-go-lifecycle-ownership.md
[startup]: https://github.com/C360Studio/semstreams/blob/8b99efe9c66a4faa4fa509f9f62cc6bad8392128/docs/operations/migration-startup-observability.md
[datatype]: https://github.com/C360Studio/semstreams/blob/8b99efe9c66a4faa4fa509f9f62cc6bad8392128/docs/operations/migration-predicate-datatype.md
[responses]: https://github.com/C360Studio/semstreams/blob/8b99efe9c66a4faa4fa509f9f62cc6bad8392128/docs/operations/migration-beta160-user-response-subjects.md
[graph-read]: https://github.com/C360Studio/semstreams/blob/8b99efe9c66a4faa4fa509f9f62cc6bad8392128/docs/operations/migration-graph-read-tools.md
