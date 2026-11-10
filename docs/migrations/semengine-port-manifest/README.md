# SemEngine Port Source-to-Destination Manifest and Adaptation Blueprint

This directory contains the machine-verified SHA-256 provenance manifest (`manifest.json`) and Milestone 2 engineering blueprint (`#402`, Epic `#412`) for absorbing the agent runtime and trimmed GraphQL gateway from frozen SemStreams `v1.0.0-beta.162.0.20260930150212-8b99efe9c66a` (`8b99efe9c66a4faa4fa509f9f62cc6bad8392128`) into SemTeams under [ADR-061](../../adr/061-agent-runtime-ownership-and-observable-work.md) and [ADR-062](../../adr/062-agent-contract-extraction-boundary.md).

> **Current runtime truth vs. Milestone 2 target:** On `main`, SemTeams runs against frozen SemStreams `v1.0.0-beta.162.0.20260930150212-8b99efe9c66a`, with `research` and `autoresearch` live and dev category packs (`create-change`, `proof-readiness`, `dev-from-task`, `dev-via-test`) parked per [ADR-058](../../adr/058-beta159-realignment-and-demo-lane-focus.md). Nothing in `#402` copies runtime code or changes `go.mod`, bootstrap configs, schemas, OpenAPI, or UI behavior on `main`; Milestone 2 (`#404`–`#407`, `#287`) executes this blueprint on the qualification branch after `C360Studio/semengine` `setup-04a-06` (`#25`–`#28`) and `#24` pin.

## Regenerating and verifying the manifest

`scripts/generate-semengine-port-manifest.go` is a `//go:build ignore`, Go-stdlib-only provenance verifier and AST analyzer. It queries `go list -m -json github.com/c360studio/semstreams` (rejecting any `go.mod` `replace` directive or version/`h1:` drift), verifies the upstream MIT `LICENSE`, walks all 21 absorbed units deterministically, asserts zero AST imports of excluded donor packages (`agentic/identity`, `agentic/research`, `pkg/fusion`), and classifies every external `github.com/c360studio/semstreams/...` import fail-closed.

```sh
# Emit the generated Markdown summary block and refresh manifest.json
go run ./scripts/generate-semengine-port-manifest.go -write-json docs/migrations/semengine-port-manifest/manifest.json

# Emit deterministic JSON to stdout
go run ./scripts/generate-semengine-port-manifest.go -format=json

# Independent shell verification of any verbatim-copy file (e.g., upstream MIT LICENSE)
MODDIR=$(go list -m -f '{{.Dir}}' github.com/c360studio/semstreams)
shasum -a 256 "$MODDIR/LICENSE"

# Run contract + mutation-sensitivity verification
go test ./test/contract/... -run TestSemEnginePortManifest
```

<!-- BEGIN GENERATED MANIFEST SUMMARY -->

### Provenance summary

- Frozen donor module: `github.com/c360studio/semstreams v1.0.0-beta.162.0.20260930150212-8b99efe9c66a` (`8b99efe9c66a4faa4fa509f9f62cc6bad8392128`, `h1:WAlGLMjE/3EnYyeoqounCfYnVyIB3igMlG+FLP1Iq+o=`)
- Upstream MIT `LICENSE`: `LICENSE` -> `internal/agentic/LICENSE` (`1061` bytes, SHA-256 `f04755968ca6dc99d31430e02ff6db65032586f7ddd5f955e86e2df31310a478`, `Copyright (c) 2025 C360`, verified: `true`)
- Absorbed units: **21** units, **549** manifest files (**182** prod `.go`, **347** `_test.go`, **20** non-Go docs/fixtures/license; **6422308** bytes total)
- Internal cross-unit import edges rewritten to `github.com/c360studio/semteams/internal/...`: **577**
- Required new Milestone 2 files tracked alongside absorbed units: **3** (`internal/agentic/graphmutation/client.go`, `internal/agentic/looptoken/token_test.go`, `internal/agentic/rulefamily/register.go`)

### 21 absorbed units (`8b99efe9c66a` -> `internal/...`)

| Unit | Frozen SemStreams source | SemTeams `internal/` destination | Prod `.go` | `_test.go` | Non-Go | Bytes | Adaptation class | Issue |
| --- | --- | --- | ---: | ---: | ---: | ---: | --- | --- |
| `agentic` | `agentic` | `internal/agentic` | 23 | 32 | 1 | 540227 | `structural-adaptation` | `#404` |
| `agentic/agentrun` | `agentic/agentrun` | `internal/agentic/agentrun` | 3 | 10 | 0 | 185636 | `structural-adaptation` | `#404` |
| `service/milestone_slice` | `service/milestone_*.go` | `internal/agentic/milestone` | 1 | 2 | 0 | 23115 | `structural-adaptation` | `#404` |
| `vocabulary/agentic` | `vocabulary/agentic` | `internal/agentic/vocab` | 4 | 1 | 0 | 114999 | `verbatim-plus-import-rewrite` | `#404` |
| `internal/looptoken` | `internal/looptoken` | `internal/agentic/looptoken` | 1 | 0 | 0 | 1980 | `verbatim-copy` | `#404` |
| `persona` | `persona` | `internal/persona` | 4 | 4 | 0 | 47775 | `structural-adaptation` | `#404` |
| `internal/deliverylane` | `internal/deliverylane` | `internal/agentic/deliverylane` | 1 | 2 | 0 | 33616 | `verbatim-plus-import-rewrite` | `#404` |
| `internal/agentterminal` | `internal/agentterminal` | `internal/agentic/agentterminal` | 1 | 1 | 0 | 14398 | `verbatim-plus-import-rewrite` | `#404` |
| `model/wire` | `model/wire` | `internal/agentic/modelwire` | 7 | 6 | 7 | 70942 | `verbatim-copy` | `#404` |
| `model/wire/responses` | `model/wire/responses` | `internal/agentic/modelwire/responses` | 9 | 3 | 5 | 98708 | `verbatim-plus-import-rewrite` | `#404` |
| `processor/agentic-loop` | `processor/agentic-loop` | `internal/agentic/loop` | 31 | 124 | 1 | 2456476 | `structural-adaptation` | `#404` |
| `processor/agentic-loop/internal/loopbucket` | `processor/agentic-loop/internal/loopbucket` | `internal/agentic/loop/internal/loopbucket` | 1 | 1 | 0 | 7104 | `verbatim-copy` | `#404` |
| `processor/agentic-loop/internal/looprequest` | `processor/agentic-loop/internal/looprequest` | `internal/agentic/loop/internal/looprequest` | 1 | 2 | 0 | 13680 | `verbatim-copy` | `#404` |
| `processor/agentic-loop/lessonmatch` | `processor/agentic-loop/lessonmatch` | `internal/agentic/loop/lessonmatch` | 1 | 2 | 0 | 23950 | `verbatim-copy` | `#404` |
| `processor/agentic-loop/prompt` | `processor/agentic-loop/prompt` | `internal/agentic/loop/prompt` | 3 | 2 | 0 | 22251 | `verbatim-copy` | `#404` |
| `processor/agentic-dispatch` | `processor/agentic-dispatch` | `internal/agentic/dispatch` | 19 | 43 | 1 | 792128 | `structural-adaptation` | `#404` |
| `processor/agentic-model` | `processor/agentic-model` | `internal/agentic/model` | 23 | 36 | 1 | 582127 | `structural-adaptation` | `#404` |
| `processor/agentic-tools` | `processor/agentic-tools` | `internal/agentic/tools` | 20 | 47 | 1 | 703148 | `structural-adaptation` | `#404` |
| `processor/agentic-tools/executors` | `processor/agentic-tools/executors` | `internal/agentic/tools/executors` | 24 | 17 | 0 | 382474 | `structural-adaptation` | `#404` |
| `processor/agentic-tools/runner` | `processor/agentic-tools/runner` | `internal/agentic/tools/runner` | 1 | 1 | 0 | 14216 | `verbatim-copy` | `#404` |
| `gateway/graph-gateway` | `gateway/graph-gateway` | `internal/gateway/graphgateway` | 4 | 11 | 2 | 292297 | `replace-with-trimmed-implementation` | `#406` |

### Excluded donor subpackages under `agentic/` (zero-import verified)

| Excluded donor subpath | Donor prod `.go` | Donor `_test.go` | Disposition |
| --- | ---: | ---: | --- |
| `agentic/identity` | 6 | 3 | Excluded — 0 AST imports in SemTeams or the 21 absorbed units at 8b99efe9c66a; DID/credential experiment outside ADR-061 runtime scope |
| `agentic/research` | 11 | 2 | Excluded — 0 AST imports in SemTeams or the 21 absorbed units at 8b99efe9c66a (sole importer of unported pkg/fusion; used only by uncomposed research-graph-* processors) |

### Required new Milestone 2 files

| Destination file | Issue | Obligation |
| --- | --- | --- |
| `internal/agentic/graphmutation/client.go` | `#404` | Narrow NATS graph.mutation.> helper over InterfaceType semengine.graph.mutation and semengine/graph.{CreateEntityRequest,AppendTriplesRequest,EntityMutationResponse} stamping ADR-110 Source and Timestamp |
| `internal/agentic/looptoken/token_test.go` | `#404` | Direct unit test required because upstream internal/looptoken/token.go ships with 0 *_test.go files at 8b99efe9c66a |
| `internal/agentic/rulefamily/register.go` | `#405` | Registers publish_agent action family onto semengine/processor/rule E1-E4 extension seam without introducing a local generic execution journal ahead of semengine#24 |

### External `semstreams/...` imports across absorbed units (34 paths, fail-closed)

| SemStreams import subpath | Absorbed prod `.go` | Absorbed `_test.go` | Category | Milestone 2 target / replacement | Issue |
| --- | ---: | ---: | --- | --- | --- |
| `component` | 36 | 87 | `semengine_main` | `github.com/c360studio/semengine/component` | `#404, #406` |
| `composition` | 1 | 1 | `semengine_main` | `github.com/c360studio/semengine/composition` | `#404` |
| `config` | 1 | 2 | `semengine_main` | `github.com/c360studio/semengine/config` | `#404` |
| `gateway` | 1 | 0 | `drop_in_trimmed_gateway` | `dropped in internal/gateway/graphgateway` | `#406` |
| `graph` | 15 | 31 | `semengine_main` | `github.com/c360studio/semengine/graph` | `#404, #406` |
| `graph/inference` | 2 | 1 | `drop_in_trimmed_gateway` | `dropped in internal/gateway/graphgateway` | `#406` |
| `graph/llm` | 2 | 2 | `semteams_local_adapter` | `github.com/c360studio/semteams/internal/agentic/modelwire + github.com/c360studio/semengine/model` | `#404` |
| `graph/query` | 1 | 1 | `semengine_main` | `github.com/c360studio/semengine/graph/query` | `#406` |
| `graph/readiness` | 2 | 2 | `drop_in_trimmed_gateway` | `dropped in internal/gateway/graphgateway` | `#406` |
| `health` | 1 | 1 | `semengine_main` | `github.com/c360studio/semengine/health` | `#404` |
| `internal/graphmutation` | 5 | 2 | `semteams_local_adapter` | `github.com/c360studio/semteams/internal/agentic/graphmutation` | `#404` |
| `internal/lifecyclecleanup` | 6 | 0 | `semengine_promoted_pkg` | `github.com/c360studio/semengine/pkg/lifecyclecleanup` | `#404, #287` |
| `internal/semantictest` | 0 | 1 | `test_only_rewire` | `inline 6-part EntityID / 3-part Predicate helper in loop/graph_writer_test.go` | `#404` |
| `message` | 40 | 92 | `semengine_main` | `github.com/c360studio/semengine/message` | `#404, #406` |
| `metric` | 5 | 31 | `semengine_main` | `github.com/c360studio/semengine/metric` | `#404` |
| `model` | 8 | 30 | `semengine_main` | `github.com/c360studio/semengine/model` | `#404` |
| `natsclient` | 38 | 121 | `semengine_main` | `github.com/c360studio/semengine/natsclient (+ internal/natstest in *_test.go)` | `#404, #406` |
| `payloadbuiltins` | 0 | 42 | `test_only_rewire` | `github.com/c360studio/semteams/internal/runtimecatalog.RegisterPayloads` | `#404` |
| `payloadregistry` | 2 | 15 | `semengine_main` | `github.com/c360studio/semengine/payloadregistry` | `#404` |
| `pkg/errs` | 48 | 46 | `semengine_main` | `github.com/c360studio/semengine/pkg/errs` | `#404, #406` |
| `pkg/graphview` | 2 | 6 | `semengine_main` | `github.com/c360studio/semengine/pkg/graphview` | `#404` |
| `pkg/lifecycle` | 2 | 6 | `semengine_main` | `github.com/c360studio/semengine/pkg/lifecycle` | `#404, #405` |
| `pkg/projection` | 4 | 6 | `semengine_main` | `github.com/c360studio/semengine/pkg/projection` | `#404` |
| `pkg/projection/contract` | 3 | 0 | `semengine_main` | `github.com/c360studio/semengine/pkg/projection/contract` | `#404` |
| `pkg/types` | 6 | 7 | `semengine_main` | `github.com/c360studio/semengine/pkg/types` | `#404` |
| `processor/gated-dag` | 0 | 1 | `test_only_rewire` | `inline string constant in internal/agentic/entity_ids_semantics_test.go` | `#404` |
| `processor/graph-ingest` | 0 | 4 | `semengine_main` | `github.com/c360studio/semengine/processor/graph-ingest` | `#404` |
| `processor/rule` | 1 | 2 | `semengine_setup_04a_06` | `github.com/c360studio/semengine/processor/rule` | `#404, #405` |
| `service` | 3 | 0 | `semengine_main` | `github.com/c360studio/semengine/service` | `#404, #406` |
| `storage` | 1 | 1 | `semengine_main` | `github.com/c360studio/semengine/storage` | `#404` |
| `storage/storeregistry` | 1 | 4 | `semengine_main` | `github.com/c360studio/semengine/storage/storeregistry` | `#404` |
| `types` | 7 | 20 | `semengine_main` | `github.com/c360studio/semengine/types` | `#404` |
| `vocabulary` | 6 | 4 | `semengine_main` | `github.com/c360studio/semengine/vocabulary` | `#404` |
| `vocabulary/builtins` | 0 | 3 | `test_only_rewire` | `github.com/c360studio/semteams/internal/agentic/vocab.Register()` | `#404` |

<!-- END GENERATED MANIFEST SUMMARY -->

## Milestone 2 engineering blueprint (`D1`–`D4`)

### D1 — `internal/` package layout and MIT license preservation

All 21 absorbed units land under `internal/` so `github.com/c360studio/semteams` exports zero accidental public Go runtime packages (SemDev absorbs its own copy per [ADR-061 §2](../../adr/061-agent-runtime-ownership-and-observable-work.md)).

- **MIT copyright preservation (`#404`):** Copy frozen SemStreams `LICENSE` (`SHA-256 f04755968ca6dc99d31430e02ff6db65032586f7ddd5f955e86e2df31310a478`) verbatim to `internal/agentic/LICENSE`, preserving `"Copyright (c) 2025 C360"` and the MIT permission notice alongside SemTeams's root `LICENSE`.
- **Excluded donor subpackages:** `agentic/identity` and `agentic/research` stay out of SemTeams (`0` imports in SemTeams or the 21 absorbed units; `agentic/research` is the sole importer of unported `pkg/fusion`).
- **Import-path rewrite table (`577` internal cross-unit edges):**
  - `github.com/c360studio/semstreams/agentic` -> `github.com/c360studio/semteams/internal/agentic`
  - `github.com/c360studio/semstreams/agentic/agentrun` -> `github.com/c360studio/semteams/internal/agentic/agentrun`
  - `service/milestone_*.go` (`NewMilestoneService`) -> `github.com/c360studio/semteams/internal/agentic/milestone` (`milestone.NewService`)
  - `github.com/c360studio/semstreams/vocabulary/agentic` -> `github.com/c360studio/semteams/internal/agentic/vocab`
  - `github.com/c360studio/semstreams/internal/looptoken` -> `github.com/c360studio/semteams/internal/agentic/looptoken` (+ new `token_test.go`)
  - `github.com/c360studio/semstreams/persona` -> `github.com/c360studio/semteams/internal/persona`
  - `github.com/c360studio/semstreams/internal/deliverylane` -> `github.com/c360studio/semteams/internal/agentic/deliverylane`
  - `github.com/c360studio/semstreams/internal/agentterminal` -> `github.com/c360studio/semteams/internal/agentic/agentterminal`
  - `github.com/c360studio/semstreams/model/wire` -> `github.com/c360studio/semteams/internal/agentic/modelwire`
  - `github.com/c360studio/semstreams/model/wire/responses` -> `github.com/c360studio/semteams/internal/agentic/modelwire/responses`
  - `github.com/c360studio/semstreams/processor/agentic-loop` (+ `internal/loopbucket`, `internal/looprequest`, `lessonmatch`, `prompt`) -> `github.com/c360studio/semteams/internal/agentic/loop` (+ matching subpackages)
  - `github.com/c360studio/semstreams/processor/agentic-dispatch` -> `github.com/c360studio/semteams/internal/agentic/dispatch`
  - `github.com/c360studio/semstreams/processor/agentic-model` -> `github.com/c360studio/semteams/internal/agentic/model`
  - `github.com/c360studio/semstreams/processor/agentic-tools` (+ `executors`, `runner`) -> `github.com/c360studio/semteams/internal/agentic/tools` (+ `executors`, `runner`)
  - `github.com/c360studio/semstreams/gateway/graph-gateway` -> `github.com/c360studio/semteams/internal/gateway/graphgateway`

### D2 — Explicit `ExecutorRegistry` dependency injection (`semengine#29`, `#404`, `#407`)

`semengine/component.Dependencies` drops `ToolRegistry` (`C360Studio/semengine#29`). SemTeams replaces ambient `Dependencies.ToolRegistry` lookup with explicit catalog closure injection:

| Call site | Frozen SemStreams `8b99efe9` seam | Milestone 2 SemTeams contract |
| --- | --- | --- |
| `internal/agentic` | Implicit `interface{ ListTools() []agentic.ToolDefinition }` duplicated in `loop` and `dispatch` | Export `type ToolRegistryReader interface { ListTools() []ToolDefinition }` in `internal/agentic` (`*tools.ExecutorRegistry` satisfies it directly) |
| `internal/agentic/tools` | Reads `deps.ToolRegistry` inside `NewComponent` (`processor/agentic-tools/component.go:51,168`) | `RegisterWithExecutorRegistry(reg *component.Registry, shared *ExecutorRegistry) error` (`Register(reg)` delegates with `nil` for schema-only callers) |
| `internal/agentic/loop` | Type-asserts `deps.ToolRegistry` (`processor/agentic-loop/component.go:393`) | `RegisterWithToolRegistry(reg *component.Registry, shared agentic.ToolRegistryReader) error` (`Register(reg)` delegates with `nil`) |
| `internal/agentic/dispatch` | Type-asserts `deps.ToolRegistry` (`processor/agentic-dispatch/component.go:45,60`) | `RegisterWithToolRegistry(reg *component.Registry, shared agentic.ToolRegistryReader) error` (`Register(reg)` delegates with `nil`) |
| `internal/runtimecatalog` | `RegisterComponents(reg *component.Registry) error` | `RegisterComponents(reg *component.Registry, tools *agentictools.ExecutorRegistry) error` plus `RegisterSchemaComponents(reg *component.Registry) error` (passes `nil` for `cmd/openapi-generator` and offline schema tests) |
| `cmd/semteams/main.go` | Registers catalog first, then builds `toolRegistry` in `setupToolsAndPreprocessor` and mutates `svcDeps.ToolRegistry` (`main.go:190,207-214`) | Construct `toolRegistry := agentictools.NewExecutorRegistry()` before `runtimecatalog.RegisterComponents(componentRegistry, toolRegistry)`, then populate executors in `setupToolsAndPreprocessor` before `serviceManager.ConfigureFromServices` (`executors.RegisterBuiltins` captures `componentRegistry` by pointer lazily for `list_components`/composition tools, so both registries are populated before component factories execute); delete `svcDeps.ToolRegistry` |

### D3 — Substrate and transitive helper adaptations (`#404`, `#405`, `#287`)

1. **Failed-start rollback (`semengine#77` / `#287`) & ADR-113 lifecycle:**
   - Rewrite all 6 `internal/lifecyclecleanup` imports (`agentrun.go`, `dispatch/component.go`, `loop/component.go`, `loop/trajectory_reconciler.go`, `model/component.go`, `tools/component.go`) to `github.com/c360studio/semengine/pkg/lifecyclecleanup`. Note `semengine/pkg/lifecyclecleanup.RollbackFailedStart` returns an error when `rollback == nil`; always pass a non-nil rollback closure.
   - Per SemEngine ADR-113 (already landed on `semengine` `main`; avoid the `WithoutCancel` host anti-pattern noted in SemSource follow-up `semengine#361`), `Start(ctx)` treats `ctx` as startup-scoped only while long-lived component background loops and `internal/agentic/milestone.Service` derive internal run contexts cancelled exclusively by `Stop(ctx)`.
   - In `internal/agentic/milestone/milestone_health_test.go` (from `service/milestone_health_test.go:42`), replace the intra-package `manager.handleSystemHealth` call with public `Service.Health()` and `/health` HTTP handler assertions.
2. **Replacing `graph/llm` in `internal/agentic/loop` (`semengine#32`, `#484`):**
   - `semengine` excludes `graph/llm` at Tier 0. Rebind `LLMSummarizer` in `internal/agentic/loop/{component.go,summarizer.go}` onto `internal/agentic/modelwire.NewClient` + `semengine/model.NewHTTPClient` (the exact zero-`go-openai` pattern already proven in `processor/agentic-dispatch/intent_classifier.go:106-135`).
3. **Graph mutation client seam & port contract (`semengine#69`, `#137`, ADR-110, `v0.1.0-alpha.3`):**
   - `semengine/internal/graphmutation` is module-private (`internal/graphmutation/port.go:27-35`) and classifies any port on `SubjectFamily = "graph.mutation.>"` as a graph-mutation port, refusing component validation unless `InterfaceType == "semengine.graph.mutation"` (`InterfaceVersion == "v1"`, `Required == true`).
   - Add `internal/agentic/graphmutation/client.go` (~90 LOC) exposing `MutationPortDef(name, description string)` (`Interface: "semengine.graph.mutation"`) and single-attempt `CreateEntity` / `AppendTriples` over `natsclient.RequestClassified` + `semengine/graph.{CreateEntityRequest,AppendTriplesRequest,EntityMutationResponse}` (`KVRevision` JSON `kv_revision`), enforcing non-empty ADR-110 `Source` and non-zero `Timestamp` on every `message.Triple`.
   - In `#407`, update `"semstreams.graph.mutation"` to `"semengine.graph.mutation"` on `graph-ingest` (`inputs`) and `rule-processor` (`outputs`) in `configs/flow-bootstrap.json` and `configs/e2e-flow-bootstrap.json`, and replace `graph-query`'s `"query_timeout": 30000000000` + `"max_depth": 5` with `"query_timeout_ms": 30000` (`processor/graph-query/config.go:19-25` strict unknown-field refusal).
4. **`publish_agent` rule action family (`semengine#25`–`#28`, `#405`) & durability (`semengine#24`):**
   - Once `setup-04a-06` pins, `internal/agentic/rulefamily` registers the `publish_agent` action executor/validator onto `semengine/processor/rule`'s `E1`–`E4` extension seam, reusing `internal/agentic` `TaskMessage` + `DispatchEnforcedMetadataKeys`.
   - `internal/agentic/loop` consumes `semengine/pkg/lifecycle` durability contracts delivered by `semengine#24` without inventing a competing SemTeams-local generic execution journal.
5. **Absorbed `_test.go` helper rewires (`#404`, `#406`):**
   - `payloadbuiltins` (42 test files) -> `internal/runtimecatalog.RegisterPayloads`
   - `vocabulary/builtins` (3 test files) -> `internal/agentic/vocab.Register()`
   - `natsclient.NewTestClient` / `TestClient` -> `internal/natstest` (landed in `#400`, validated by `semengine#568`)
   - `processor/gated-dag` (`entity_ids_semantics_test.go`) -> inline string constant
   - `internal/semantictest` (`loop/graph_writer_test.go`) -> inline 6-part `EntityID` / 3-part `Predicate` helper

### D4 — Trimmed SemTeams `/graphql` and `TrajectoryPage` gateway contract (`#406` & `semengine#567`)

`internal/gateway/graphgateway` replaces the 292 KB frozen `gateway/graph-gateway` with a narrow product gateway tailored to SemTeams's UI and E2E contract:

- **Reconciliation with upstream `C360Studio/semengine#567` (`v0.1.0-alpha.3` follow-up):** Upstream opened `semengine#567` on `2026-11-09` proposing to re-admit a trimmed `gateway/graph-gateway` into `semengine` (`entity`, `entitiesByPrefix`, `entitiesByPredicate`, `traverse`, `trajectory`, and `pathSearch`). When Milestone 2 opens:
  - **Path A:** If `semengine#567` has landed in `semengine` with all 4 SemTeams queries below (including `trajectory` / `agentic.TrajectoryPage` decoding), wire `semengine/gateway/graph-gateway` in `internal/runtimecatalog` rather than carrying a duplicate local package.
  - **Path B:** If `semengine#567` remains open or excludes `agentic_queries` / `trajectory` per ADR-061 layer-1 neutrality, execute this `internal/gateway/graphgateway` blueprint directly.
- **HTTP & reverse-proxy contract:** Preserves factory/component name `"graph-gateway"` and `graphql_path: "/graphql"` mounted via `RegisterHTTPHandlers(prefix string, mux *http.ServeMux)` (`POST /graphql`, `GET /graphql` Playground disabled unless configured), keeping `ui/Caddyfile.dev:52-53` and `ui/Caddyfile.e2e:62-63` (`rewrite /graph-gateway/graphql`) unchanged.
- **NATS port contract & strict bootstrap cleanup:** Declares exactly 2 NATS request-reply output ports:
  - `graph_queries` -> `graph.query.*`
  - `agentic_queries` -> `agentic.query.*`
  In `#406`, remove `"graph_index_queries"` (`configs/flow-bootstrap.json:746`, `configs/e2e-flow-bootstrap.json:724`) and `"mcp_path": "/mcp"` (`configs/flow-bootstrap.json:768`, `configs/e2e-flow-bootstrap.json:746`) in the same commit so SemEngine `config`'s strict unknown-key check and `composition.Validate` pass.

| GraphQL root query | SemTeams callers | NATS request subject & wire contract | `#406` gateway behavior |
| --- | --- | --- | --- |
| `trajectory(loopId: String!, limit: Int, cursor: String): TrajectoryPage` | `ui/src/lib/services/agentApi.ts:161-215,253` (`getLoopTrajectory`, `getLoopTokenTotals`), `ui/e2e/agentic/*.spec.ts` | `agentic.query.trajectory` -> `internal/agentic.TrajectoryPage` (`loopId`, `traceId`, `status`, `role`, `model`, `startedAt`, `completedAt`, `iterations`, `totalTokensIn`, `totalTokensOut`, `durationMs`, `entries`, `pageInfo`, `stepCount`) | Preserve exact `TrajectoryPage`, `TrajectoryEntry` (14 fields), and `PageInfo` (`endCursor`, `hasNextPage`, `totalCount`) field names plus `"trajectory not found"` GraphQL error substring (`agentApi.ts:214`) |
| `entitiesByPrefix(prefix: String!, limit: Int): [Entity!]!` | `ui/src/lib/components/DataView.svelte:85`, `graphApi.ts:251-280`, `ui/e2e/agentic/admin-flows-inventory.spec.ts:84-90` | `graph.query.prefix` -> `semengine/graph.QueryResponse` (`data`, `indexed_revision`, `producer`, `timestamp`) | Unwrap `QueryResponse.Data`, project `Entity` (`id`, `triples { subject predicate object }`) |
| `pathSearch(startEntity: String!, maxDepth: Int, maxNodes: Int, predicates: [String!]): PathSearchResult!` | `ui/src/lib/components/DataView.svelte:144`, `graphApi.ts:169-206` | `graph.query.pathSearch` (`graph.PathSearchQueryRequest`) -> `semengine/graph.QueryResponse` | Unwrap `QueryResponse.Data`, project `entities` and `edges { subject predicate object }` |
| `entity(id: String!): Entity` | `ui/src/lib/services/graphApi.ts:212-244` (`graphApi.test.ts`) | `graph.query.entity` -> `semengine/graph.QueryResponse` wrapping `graph.ExactEntity` (JSON `kv_revision`) | Unwrap `QueryResponse.Data` and map `ExactEntity.KVRevision` (`kv_revision`) onto GraphQL `Entity.kvRevision` (`String`) so `graphApi.getEntity` (`:228,236`) works unchanged; return `null` on not-found |
| `globalSearch` / `searchGraph` | **Excluded** (`0` `.svelte`/route callers; `toolExecutors.ts` & `server/graphql/client.ts` have `0` non-test imports) | Would require `COMMUNITY_INDEX` writer (`semengine/processor/graph-clustering`) and drops removed `max_communities` (`graphApi.ts:294`) | Excluded from `internal/gateway/graphgateway`; `#406` deletes dead `graphApi.globalSearch` (+ test), `ui/src/lib/server/ai/toolExecutors.ts` (+ test), and `ui/src/lib/server/graphql/client.ts` (+ test) |
