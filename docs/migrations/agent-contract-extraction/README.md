# Agent-contract extraction evidence

[Issue #284](https://github.com/C360Studio/semteams/issues/284), design
[PR #286](https://github.com/C360Studio/semteams/pull/286), reviewed source date 2026-10-03.
The PR is stacked on [foundation #283](https://github.com/C360Studio/semteams/pull/283). SemStreams remains pinned to
`v1.0.0-beta.162.0.20260930150212-8b99efe9c66a`, source
`8b99efe9c66a4faa4fa509f9f62cc6bad8392128`. This is design evidence, not copied runtime or SemEngine qualification.

## Result

The root `agentic` + `vocabulary/agentic` + `internal/looptoken` family cannot independently transfer concrete type
authority while its frozen consumers retain original Go types. The hold extends beyond agent processors: shared
component interfaces, authored rule fields and the graph gateway also name those types. Keep a single frozen type
family until a compatible consumer transfer is established. [ADR-062](../../adr/062-agent-contract-extraction-boundary.md)
records the decision and alternatives.

Whole model and governance implementation moves were also examined. Both depend on `internal/deliverylane` and
`internal/lifecyclecleanup`, but the two helpers have different ownership. The delivery lane is agent-side owner
reaction; generic transport settlement remains a substrate responsibility. Lifecycle cleanup is admitted into
SemEngine's private namespace and its accessible rollback-policy boundary is unresolved. No whole-component copy is
authorized by this evidence.

**Owner refinement, 2026-10-03:** keep shared types, sentinel identities, failed-start rollback and similar shared
contracts upstream during preparation, and track their integration for the SemEngine migration. The whole model move
is deferred while shared rollback is inaccessible; do not adapt local policy to force that extraction or copy an
isolated delivery lane with no live consumer. The lane's agentic ownership is unchanged.
[Issue #287](https://github.com/C360Studio/semteams/issues/287) tracks upstream integration, not a local adaptation
prerequisite. [ADR-062's dated addendum](../../adr/062-agent-contract-extraction-boundary.md#addendum-2026-10-03-keep-shared-dependencies-upstream)
records the ruling. All measurements below remain the historical design evidence; none was rerun or changed by it.

## What the measurements mean

The [machine report](family-report.json), produced by
[`measure-agent-contract-family.go`](../../../scripts/measure-agent-contract-family.go), retains selected
module/source identities, checksums, build contexts, errors and source positions. It records Go 1.26.4 on
`darwin/arm64`, `CGO_ENABLED=1`, and empty `GOFLAGS`. The SemTeams revision is
`8d9a439541d9a1dc1bccb34662a4782852a38c7b`; per-file hashes identify the measured working source.

Forward closure uses fully qualified roots in SemTeams' selected module graph. Framework typed-reference analysis
uses the frozen framework's module graph; SemTeams typed-reference analysis uses the consumer's graph. The report
labels those contexts rather than silently treating the module selections as identical. The analyzer binds its
compiled factory/type probe to the consumer-selected version and checksum and rejects module replacements.
Keep these measures separate:

- **Production forward closure:** compiler dependencies needed to build the selected roots, including shared
  substrate and standard-library packages. Dependency inclusion does not assign extraction ownership.
- **Test forward closure:** the corresponding test graph, including test variants and generated test mains where
  reported. Compiler package IDs are not the number of distinct production package paths.
- **Direct reverse references:** source packages importing a target and typed references to its declarations. A
  direct reverse scan is not a minimal transitive reverse closure or a proof that every consumer can move atomically.
- **Consumer membership:** a reference present somewhere in the pinned upstream tree is not automatically admitted
  to SemTeams. Declared config, the current advertised catalog and compile reach are different sets.

The foundation preserves 27 advertised component factories. Production and mock bootstrap configs each name nine
factory types. Neither count proves dynamic activity; inactive advertised factories are not automatically removable.
The prior [framework footprint](../semstreams-8b99efe/framework-footprint.md) remains historical context, not a newly
measured count for this design.

## Measured forward sets and diagnostic scopes

All forward sets below resolved with empty error lists. Counts are compiler package IDs, including test variants,
not distinct source packages, executed tests or permission to copy their dependencies.

| Selected roots | Context | All package IDs | SemStreams package IDs |
|---|---|---:|---:|
| Three-package contract family | Production | 137 | 12 |
| Three-package contract family | Default tests | 491 | 46 |
| Three-package contract family | Integration-tag tests | 491 | 46 |
| Three-package contract family | Parked-tag tests | 491 | 46 |
| `processor/agentic-model` | Production | 467 | 34 |
| `processor/agentic-model` | Default tests | 492 | 47 |
| `processor/agentic-model` | Integration-tag tests | 496 | 47 |
| `processor/agentic-governance` | Production | 474 | 38 |
| `processor/agentic-governance` | Default/integration-tag tests | 521 | 40 |

The root family has 23 production and 32 test files in `agentic`, four production and one test file in
`vocabulary/agentic`, and one production file with no test file in `internal/looptoken`. Exact files and hashes are in
`forward_closures.root_packages` and `framework.measured_file_sha256`; declared test-function inventories are not pass
counts.

Framework direct-consumer default, integration and parked contexts resolved without reported errors. SemTeams
default and integration direct-consumer contexts also resolved, but its parked context is **partial diagnostic
evidence**: `test/contract/create_change_fixture_test.go:72` passes `recordingPublisher` to
`agentictools.TriplePublisher` without the required `Append` method. The report retains both emitted diagnostics of
that single defect. A successful parked forward root set does not make this wider consumer context pass; no donor
code was repaired or activated.

The offline `nominal_type_probe` reports equal wire JSON but false assignability and a failed assertion to the frozen
type. Its alias keeps `github.com/c360studio/semstreams/agentic` as the definition package, and a second registration
of `agentic.request.v1` is rejected. This is the direct evidence that wire equality, aliases and duplicate registration
do not accomplish contract authority transfer. The error probe also shows that same-text errors have different
identity while the retained shared error matches. Exported package variables are included in the typed-reference
census. `agentic_payloads` records all 20 concrete factories, source positions, profiles, projection contracts and
direct consumer packages.

## Concrete authority and consumers

The frozen [root registrar](https://github.com/C360Studio/semstreams/blob/8b99efe9c66a4faa4fa509f9f62cc6bad8392128/agentic/payload_registry.go#L26-L63)
defines 20 payload registrations. `LoopExecutionEntity` and `AgentLessonEntity` attach the two projection contracts;
`pkg/projection/contract` remains a shared substrate dependency. Registration metadata and exact concrete factories
must transfer together with their consumers, not merely retain equal JSON.

| Authority or consumer | Source evidence at the frozen pin | Consequence |
|---|---|---|
| Payload construction/decoding | `agentic/payload_registry.go:34–56` constructs the 20 root types | One domain/category/version identity must decode to the type the consumer expects; two registrars do not resolve disagreement |
| Model request execution | `processor/agentic-model/component.go:791` asserts `*agentic.AgentRequest` | A separately defined lookalike request fails the concrete boundary despite matching wire fields |
| Shared tool dependency | `component/dependencies.go:47–57`: `Execute` takes `ToolCall`, returns `ToolResult`; `ListTools` returns `[]ToolDefinition` | Moving only agent processors leaves a shared interface tied to the old family; engine removal is coordinated by #27/#29 |
| Authored rules and spawning | `processor/rule/actions.go:191,206,1445`: `ResponseFormat`, `ToolChoice`, `[]ToolDefinition`; task stamp helpers use `*TaskMessage` | Root transfer needs the rule action/schema/validation boundary, not only loop payload changes |
| Graph gateway trajectory | `gateway/graph-gateway/component.go:2093–2106` decodes and validates `agentic.TrajectoryPage` | Typed query consumers remain part of the contract even though trajectory is not a registered message factory |
| Projection ownership | `agentic/payload_registry.go:52–53`: `LoopExecutionContract` and `LessonContract` | Preserve owner/group/predicate contracts and one registration authority over shared projection interfaces |
| Sentinel and shared-value authority | `agentic.ErrToolNotFound`, `ErrMaxIterationsReached`, `DispatchEnforcedMetadataKeys`; tools wrap the sentinel and check `errors.Is`, and loop handlers re-export/check it | A fresh error with matching text is a different object; type compatibility alone does not preserve error matching or shared policy authority |
| Private loop identity helper | `agentic/user_types.go` imports `internal/looptoken`; agentrun/dispatch/loop also consume it | The root forward edge does not make all helper consumers disappear; preserve identity grammar and actual callers |
| Retained model client | `processor/agentic-dispatch/intent_classifier.go:106–125` calls `NewClient`, `AdapterFor`, `ChatCompletion` | Moving the configured model component alone does not remove the old package dependency or complete all implementation transfer |

This table names decisive boundaries rather than claiming an exhaustive reverse closure. In particular, the full-pin
`cmd/detonate-injections` model-client reference does not authorize copying that command into SemTeams. The machine
report retains the wider reference inventory and distinguishes its measured scopes.

## Private helpers and engine contracts

SemEngine's
[pinned admission decision](https://github.com/C360Studio/semengine/blob/2ec3bcf08bc1d2794d6cb1081ebd1982e08cf2c5/openspec/changes/archive/2026-10-01-setup-03b-contract-boundary/design.md#d4-port-set-admission-or-separation-of-each-questioned-package)
separates `agentic/agentrun`, `internal/deliverylane` and `internal/agentterminal` as agentic domain. It admits
`internal/lifecyclecleanup` under engine `internal/`. An inaccessible Go import proves an access problem, not the
helper's architectural owner.

The delivery-lane
[migration contract](https://github.com/C360Studio/semstreams/blob/8b99efe9c66a4faa4fa509f9f62cc6bad8392128/docs/operations/migration-restart-safe-nats-client.md#L82-L101)
explicitly allows outside consumers to implement owner reaction and promises no export. Required properties include
one first-result latch, recorded stop reason before observer-driven drain, the exact acquired handle and one drain.
Owner `Stop` has a separate documented race boundary; the helper does not give the component lifecycle authority.
Transport metadata validation, heartbeat, work join and terminal settlement remain in `natsclient`.

The initial design considered accessible shared rollback policy or a narrowly approved owner adaptation. The dated
owner refinement above selects upstream integration and defers the dependent component move; a local adaptation is
not the next step. No equivalent reusable public rollback policy was identified, and manager-level rollback cannot
discharge resources acquired inside a component's own failed start. Preserve the source requirements for eventual
integration: a fresh five-second context budget and synchronous cleanup, without preemption of callbacks that ignore
cancellation or a five-second wall-clock exit guarantee. The delivery lane needs no hypothetical engine export, and
#24 remains a separate generic durability requirement rather than the cause of this rollback-access hold.

For eventual root-type/engine integration, the named requirements remain
[E1 #25](https://github.com/C360Studio/semengine/issues/25) action-family registration,
[E2 #26](https://github.com/C360Studio/semengine/issues/26) authored agent-field representation,
[E3 #27](https://github.com/C360Studio/semengine/issues/27) agent dispatch/stamping/validation,
[E4 #28](https://github.com/C360Studio/semengine/issues/28) governance handlers and auditing, and
[#29](https://github.com/C360Studio/semengine/issues/29) removal of shared `ToolRegistryReader` wiring after E3.
The [pinned contract](https://github.com/C360Studio/semengine/blob/2ec3bcf08bc1d2794d6cb1081ebd1982e08cf2c5/docs/contract.md#rules-and-entity-workflows)
already requires unknown pack actions to fail startup; only the extension shape/implementation is unresolved.
The [latest #24 ruling](https://github.com/C360Studio/semengine/issues/24#issuecomment-5937829621) bases generic
attempt/effect/replay work on lifecycle and projection after 04A change 2. That work is planned; first-floor
[PR #48](https://github.com/C360Studio/semengine/pull/48) is draft at this review baseline. No shipped engine behavior
is inferred from those targets.

## Tests and retained limitations

The frozen family baseline passed this source test command from the SemTeams worktree:

```sh
go test -race github.com/c360studio/semstreams/agentic \
  github.com/c360studio/semstreams/vocabulary/agentic \
  github.com/c360studio/semstreams/internal/looptoken
```

`agentic` and `vocabulary/agentic` have passing tests; `internal/looptoken` reports `[no test files]`. Its use through
entity-ID consumers does not become a standalone helper-test pass. The separate frozen implementation baseline passed
`-race` for `processor/agentic-model`, `internal/deliverylane` and `internal/lifecyclecleanup`. These are default source
tests, not extracted-package, integration/restart, retained-state or live-provider qualification. The [family command](evidence/frozen-family-tests.json) and [log](evidence/frozen-family-tests.txt), and
[implementation command](evidence/frozen-implementation-tests.json) and [log](evidence/frozen-implementation-tests.txt)
retain exact execution evidence. The [private-import probes](evidence/private-import-probes.json), their `.go.txt`
sources and logs retain the expected compiler refusal for importing either helper from the consumer module; that
refusal proves access constraints, not ownership.

Preserve these exact source limits:

| Boundary | Frozen behavior and limit |
|---|---|
| Request settlement | `agentic/state.go:68–89`: `PublishedRequestID` identifies the durably retained request; it says nothing about whether a process is still working on it |
| Continuation recovery | `agentic/state.go:143–164`: uncarried continuation replay can send a turn twice in the documented adoption window (#1365); the latest deferred turn replaces an earlier uncarried turn |
| Approval replay | `agentic/approval.go:114–126`: `ExecutionID` matches approval to execution; `RequestID` is audit correlation only, and stale/missing execution identity is refused when required |
| Evidence access | `agentic/trajectory_query.go:33–36`: a page contains immutable fact metadata and durable evidence references, never evidence bodies; #261 remains |

No observation here repairs those limits or establishes exactly-once effects. Parked-tag compilation and any partial
measurement retain their actual error status. Research/autoresearch remain live; no parked pack, SemDev workflow,
Program Pulse, UI capability or production-state migration is activated.

## Provenance for a later extraction

Preserve the frozen source SHA, module checksums, per-file source/destination mapping, test fixtures and a list of
intentional adaptations. Carry the upstream MIT `Copyright (c) 2025 C360` and permission notice with copied substantial
portions. Keep a diff against the frozen source that exposes every algorithm, contract and test change. Dependency
closure is an inventory, not permission to copy shared substrate code. No runtime files are copied in this design.

## Review and reproduction

Final independent architecture, source and evidence review passed. Review required two measurement repairs:
include exported package variables in the typed-reference census, and bind the executing factory/type probe to the
consumer-selected module version/checksum without replacements. The
[sentinel census regression](evidence/sentinel-analyzer-regression.txt) and
[provenance-guard mutation](evidence/provenance-guard-mutation.txt) preserve their verification evidence. The reviewed
report SHA-256 is `549113c4c80798659fb65b54a397437e2e05cc4ba8f4bef9b9c4446627b8de6b`.

From the SemTeams checkout, verify the analyzer explicitly; its build-ignored files are not exercised by `go test ./...`:

```sh
go test -race scripts/measure-agent-contract-family.go scripts/measure-agent-contract-family_test.go
go vet scripts/measure-agent-contract-family.go scripts/measure-agent-contract-family_test.go
```

With the frozen module resolved, reproduce the report:

```sh
go run scripts/measure-agent-contract-family.go \
  -framework "$(go list -m -f '{{.Dir}}' github.com/c360studio/semstreams)" \
  -consumer "$PWD" > /tmp/agent-contract-family-report.json
```

Inspect every `errors` list under `forward_closures` and each repository's `direct_consumer_symbols`. A zero tool exit
is not proof that every context compiled. Compare declared source identities and measured file hashes before
comparing counts. The report is a compiler/source census plus offline type probe; it does not start NATS or exercise
provider behavior. Repeat the recorded source-test commands separately when qualifying a changed extraction.
