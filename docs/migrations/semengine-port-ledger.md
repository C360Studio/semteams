# SemEngine Port Import Crosswalk and Runtime-Absorption Ledger

This document is the regenerable import crosswalk and consumer adaptation ledger for Epic `#412` (`SemTeams → SemEngine` migration, issue `#398`).

> **Current runtime truth vs. migration plan:** On `main` (`6581898e6a2d6bd78d0d21f6099a1819f18f2aaa`), SemTeams ships on the frozen SemStreams pin `v1.0.0-beta.162.0.20260930150212-8b99efe9c66a` (`8b99efe9c66a4faa4fa509f9f62cc6bad8392128`), with live category packs `research` and `autoresearch` and unwired dev packs (`create-change`, `proof-readiness`, `dev-from-task`, `dev-via-test`) held per [ADR-058](../adr/058-beta159-realignment-and-demo-lane-focus.md). All SemEngine rewires, Milestone 1 trims (`#399`–`#401`), and Milestone 2 runtime absorptions (`#404`–`#407`, `#287`) described below are target migration actions; none alter shipped behavior in `#398`.

## Pinned audit baseline and cross-repo authorities

| Artifact | Pinned reference | Role |
| --- | --- | --- |
| SemTeams base commit | `6581898e6a2d6bd78d0d21f6099a1819f18f2aaa` (`main`) | Baseline repository state audited by `scripts/measure-semengine-port-ledger.go` |
| Frozen SemStreams module | `github.com/c360studio/semstreams v1.0.0-beta.162.0.20260930150212-8b99efe9c66a` (`8b99efe9c66a4faa4fa509f9f62cc6bad8392128`) | Authoritative source for Milestone 2 agent-runtime absorption under [ADR-061](../adr/061-agent-runtime-ownership-and-observable-work.md) and the `#403` acceptance floor (`docs/migrations/semengine-port-baseline/`) |
| Audited SemEngine commit | `C360Studio/semengine@01567cc48d4d89ede05dfdf0aa1675a68bf9402b` (`main`, 2026-11-08) | Source for `docs/admission-ledger.yaml`, `ADR-108`..`ADR-113`, `processor/graph-query/MIGRATION.md`, and exported symbol checks |
| Sibling consumer crosswalk | `C360Studio/semsource#226` | SemSource consumer crosswalk; `C360Studio/semengine#9` (`issuecomment-6724212898`, 2026-11-08) sequences `processor/graph-clustering` ahead of `setup-04a-06` (`processor/rule`) and cuts `v0.1.0-alpha.3` for SemSource first |
| SemTeams ownership boundary | [ADR-061](../adr/061-agent-runtime-ownership-and-observable-work.md), [ADR-058](../adr/058-beta159-realignment-and-demo-lane-focus.md) | SemTeams absorbs the agent runtime layer (`agentic*`, `persona`, `vocabulary/agentic`, `MilestoneService`) in Milestone 2 while keeping SemDev packs unwired |

## Regenerating and verifying this ledger

`scripts/measure-semengine-port-ledger.go` is a `//go:build ignore`, Go-stdlib-only AST analyzer. It parses every `.go` file in the repository (skipping `.git`, `.worktrees`, `node_modules`, `vendor`, and `.svelte-kit`), classifies `github.com/c360studio/semstreams/...` imports into non-test `.go`, `_test.go`, and `scripts/*.go` counts, extracts string-literal-only occurrences in `scripts/`, and exits non-zero if any unclassified SemStreams import path is introduced. `TestSemEnginePortLedger` enforces exact equality between the script's default Markdown output and the generated crosswalk block below.

```sh
# Emit the full 56-row baseline crosswalk and group summary tables (Markdown)
go run ./scripts/measure-semengine-port-ledger.go

# Show only SemStreams paths that still have >= 1 active import as #399-#401 prune paths
go run ./scripts/measure-semengine-port-ledger.go -active-only

# Emit machine-readable JSON (including exact per-row file lists)
go run ./scripts/measure-semengine-port-ledger.go -format=json

# Run stdlib-only, fail-closed, doc-equality, and mutation-sensitivity contract tests
go test ./test/contract/... -run TestSemEnginePortLedger
```

## 56-path SemStreams import crosswalk (+ 2 `scripts/` string-literal paths)

The section between `BEGIN GENERATED CROSSWALK` and `END GENERATED CROSSWALK` is produced directly by `go run ./scripts/measure-semengine-port-ledger.go`.

- At baseline `6581898e6a2d6bd78d0d21f6099a1819f18f2aaa`, **56** distinct `github.com/c360studio/semstreams/...` import paths appeared in AST `import` declarations across repository `.go` files (`56` distinct non-test `.go` files and `72` distinct `_test.go` files; `scripts/measure-framework-footprint.go` also imports `message`, `payloadregistry`, and `agentic` in AST imports). Trimming `internal/runtimecatalog` in `#399` pruned **21** Group 4 paths, `#400` replaced `natsclient.NewTestClient` / `TestClient` across 11 `_test.go` files with `internal/natstest`, and `#401` decoupled `pkg/logging` and `vocabulary/builtins` (**23** of **23** Group 4 paths pruned), leaving **33** active repository import paths across `57` distinct non-test `.go` files and `73` distinct `_test.go` files below.
- **2** additional `github.com/c360studio/semstreams*` strings appear only as string literals inside `scripts/` (`github.com/c360studio/semstreams` module root and synthetic test fixture `github.com/c360studio/semstreams/agentic-extra`), reconciling the **56** baseline Go package imports with the **58** raw `git grep` path matches across the tree at baseline `6581898e6a2d6bd78d0d21f6099a1819f18f2aaa` (excluding synthetic fixture literals in `test/contract/semengine_port_ledger_test.go`).

<!-- BEGIN GENERATED CROSSWALK -->
### Summary counts

- Active repository `.go` import paths (excluding `scripts/`): **33** of **56** baseline paths (**23** pruned)
- Distinct importing files: **57** non-test `.go`, **73** `_test.go`, **1** `scripts/*.go`
- `scripts/` string-literal-only SemStreams paths: **2** (total string-matched paths across repo + `scripts/`: **35**)

| Group | Baseline paths | Active paths | Pruned paths | Non-test file imports | `_test.go` file imports | `scripts/` imports |
| --- | ---: | ---: | ---: | ---: | ---: | ---: |
| 1. SemEngine main | 20 | 20 | 0 | 94 | 147 | 2 |
| 2. SemEngine setup-04a-06 | 2 | 2 | 0 | 3 | 15 | 0 |
| 3. SemTeams Milestone 2 absorb | 11 | 11 | 0 | 69 | 71 | 1 |
| 4. Milestone 1 drop/decouple | 23 | 0 | 23 | 0 | 0 | 0 |

### 1. SemEngine main

| SemStreams subpath | Non-test | `_test.go` | `scripts/` | SemEngine disposition | Post-port home | Upstream refs | SemTeams action |
| --- | ---: | ---: | ---: | --- | --- | --- | --- |
| `component` | 4 | 9 | 0 | `adapt` | `semengine/component` | ADR-108, ADR-113, semengine#29 | M2 (#404, #407): rewire imports; Registry.Register/ListFactories in catalog_test.go; types.PlatformMeta; Dependencies drops ToolRegistry/LifecycleManager |
| `composition` | 0 | 2 | 0 | `repair-before-port` | `semengine/composition` | ADR-100 | M2 (#407): replace composition.AssertValid(t, registry, cfg) with composition.Validate(registry, cfg) (*Result, error) in test/contract/composition_contract_test.go |
| `config` | 3 | 13 | 0 | `adapt` | `semengine/config` | ADR-104, ADR-113 | M2 (#407): Manager.Stop(ctx); SEMENGINE_ env prefix and semengine_config_<org>_<stem> bucket; strict unknown-key rejection |
| `graph` | 2 | 8 | 0 | `adapt` | `semengine/graph` | ADR-109, ADR-110 | M2 (#406, #407): NewExactEntityReader returns (*ExactEntityReader, error); ExactEntity json kv_revision; EntityMutationResponse + ReconcilePredicatesRequest.Source (and per-triple Source/Timestamp) |
| `graph/inference` | 1 | 2 | 0 | `adapt` | `semengine/graph/inference` | semengine#97 | M1 (#399) / M2 (#402, #406): hierarchy slice on main; anomaly/detector types stay out of trimmed SemTeams graph-gateway |
| `message` | 22 | 36 | 1 | `adapt` | `semengine/message` | ADR-110 | M2 (#404, #407): rewire imports; stream-lane statements require Source and Timestamp |
| `metric` | 1 | 0 | 0 | `adapt` | `semengine/metric` | semengine#69 | M2 (#407): rewire cmd/semteams/main.go import; core metrics use semengine_ namespace |
| `model` | 0 | 2 | 0 | `adapt` | `semengine/model` | semengine#155 | M2 (#404, #407): rewire test/runtime imports; track semengine#155 reader/endpoint refactor before pin |
| `natsclient` | 12 | 4 | 0 | `adapt` | `semengine/natsclient` | ADR-110, ADR-112 | M1 (#400, done): NewTestClient/TestClient replaced by internal/natstest; M2 (#404, #407): rewire imports (incl. internal/natstest), use pkg/retry.Config and WaitConsumerHandlers |
| `payloadregistry` | 6 | 7 | 1 | `adapt` | `semengine/payloadregistry` | semengine#219 | M2 (#404, #407): rewire imports; GetRegistration/IndexingProfileFor take pkg/types.Type where called |
| `pkg/errs` | 6 | 6 | 0 | `adapt` | `semengine/pkg/errs` | ADR-112, semengine#146, semengine#167 | M2 (#404, #407): rewire imports; Classify defaults unknown errors to fatal and classifies by sentinel/type only |
| `pkg/lifecycle` | 3 | 6 | 0 | `adapt` | `semengine/pkg/lifecycle` | ADR-110, ADR-111, semengine#24 | M2 (#404, #405, #407): rewire imports; bind lifecycle.Manager onto rule processor at host construction; align with semengine#24 |
| `pkg/projection` | 2 | 2 | 0 | `repair-before-port` | `semengine/pkg/projection` | ADR-110, ADR-112 | M2 (#407): rewire imports; MutationError over ErrNoResponders carries transient Class field while errs.IsTransient stays false |
| `pkg/types` | 1 | 2 | 0 | `adapt` | `semengine/pkg/types` | ADR-102, ADR-104 | M2 (#404, #407): rewire imports; 6-part entity ID and UTF-8 Type.Validate contract unchanged for SemTeams |
| `processor/graph-ingest` | 1 | 1 | 0 | `repair-before-port` | `semengine/processor/graph-ingest` | ADR-108, ADR-109, ADR-110, ADR-113 | M2 (#407): rewire internal/runtimecatalog registration; enforces ADR-110 single write seam |
| `processor/graph-query` | 1 | 1 | 0 | `adapt` | `semengine/processor/graph-query` | ADR-108, ADR-109, ADR-113, semengine#9 | M2 (#406, #407): rewire catalog; adopt MIGRATION.md 4-key QueryResponse envelope, kv_revision, and COMMUNITY_INDEX readiness behavior |
| `service` | 5 | 8 | 0 | `adapt` | `semengine/service` | ADR-109, ADR-113, semengine#29, semengine#30, semengine#361 | M1 (#401, done): logging.go log-forwarder exclude_sources kept product-local without pkg/logging; M2 (#404, #407): absorb MilestoneService, pass signalCtx to StartAll, update NewBaseServiceWithOptions/MaybeStartPProf/ComponentManager |
| `storage/objectstore` | 2 | 2 | 0 | `adapt` | `semengine/storage/objectstore` | ADR-063, ADR-108, ADR-112, ADR-113 | M2 (#407): rewire internal/runtimecatalog and cmd/semteams imports |
| `types` | 21 | 34 | 0 | `adapt` | `semengine/types` | ADR-104 | M2 (#404, #407): rewire imports; ServiceConfigs named map type in Manager.ConfigureFromServices |
| `vocabulary` | 1 | 2 | 0 | `adapt` | `semengine/vocabulary` | semengine#69, semengine#72, semengine#234 | M1 (#401, done): RegisterPredicate(meta) in cmd/semteams/vocab/vocab.go replaced with vocabulary.Register; M2 (#407): rewire imports to semengine/vocabulary |

### 2. SemEngine setup-04a-06

| SemStreams subpath | Non-test | `_test.go` | `scripts/` | SemEngine disposition | Post-port home | Upstream refs | SemTeams action |
| --- | ---: | ---: | ---: | --- | --- | --- | --- |
| `processor/rule` | 3 | 12 | 0 | `unported (setup-04a-06)` | `semengine/processor/rule + semteams agent actions` | ADR-108, ADR-109, ADR-113, semengine#9, semengine#25, semengine#26, semengine#27, semengine#28 | M2 (#405, #407, #287): parked until setup-04a-06 lands after graph-clustering; register publish_agent action family via E1-E3 seam |
| `processor/rule/expression` | 0 | 3 | 0 | `unported (setup-04a-06)` | `semengine/processor/rule/expression` | semengine#9, semengine#25 | M2 (#405, #407): rewire 3 rule-pack contract test files once setup-04a-06 lands |

### 3. SemTeams Milestone 2 absorb

| SemStreams subpath | Non-test | `_test.go` | `scripts/` | SemEngine disposition | Post-port home | Upstream refs | SemTeams action |
| --- | ---: | ---: | ---: | --- | --- | --- | --- |
| `agentic` | 34 | 41 | 1 | `defer-exclude` | `semteams agent runtime (ADR-061)` | semengine#8 (03B D4) | M1 (#402) design; M2 (#404, #287): absorb contracts, payloads, and projection contracts from 8b99efe9 |
| `agentic/agentrun` | 1 | 3 | 0 | `defer-exclude` | `semteams agent runtime (ADR-061)` | semengine#8 (03B D4), semengine#30 | M1 (#402) design; M2 (#404, #287): absorb run projection + MilestoneService from 8b99efe9 |
| `gateway/graph-gateway` | 1 | 1 | 0 | `defer-exclude` | `semteams trimmed graph-gateway` | semengine#8 (03B D4), graph-query/MIGRATION.md | M1 (#402) design; M2 (#406): replace with trimmed SemTeams GraphQL gateway over graph.QueryVerbs() and kv_revision |
| `persona` | 1 | 0 | 0 | `unported (agentic cut)` | `semteams agent runtime (ADR-061)` | semengine#8 (03B D4) | M1 (#402) design; M2 (#404): absorb persona manager/fragments loader from 8b99efe9 |
| `processor/agentic-dispatch` | 6 | 4 | 0 | `unported (agentic cut)` | `semteams agent runtime (ADR-061)` | ADR-108, ADR-112, ADR-113, semengine#77 | M1 (#402) design; M2 (#404, #287): absorb component + internal/deliverylane + model/wire; adopt ADR-112/113 + pkg/lifecyclecleanup |
| `processor/agentic-loop` | 1 | 3 | 0 | `unported (agentic cut)` | `semteams agent runtime (ADR-061)` | ADR-108, ADR-111, ADR-112, ADR-113, semengine#24, semengine#77 | M1 (#402) design; M2 (#404, #287): absorb loop + internal/{looptoken,agentterminal}; align durability with semengine#24 |
| `processor/agentic-model` | 1 | 1 | 0 | `unported (agentic cut)` | `semteams agent runtime (ADR-061)` | ADR-108, ADR-112, ADR-113, semengine#77 | M1 (#402) design; M2 (#404, #287): absorb model processor + model/wire; adopt pkg/lifecyclecleanup (#77) and ADR-113 |
| `processor/agentic-tools` | 16 | 7 | 0 | `unported (agentic cut)` | `semteams agent runtime (ADR-061)` | ADR-108, ADR-112, ADR-113, semengine#29, semengine#77 | M1 (#402) design; M2 (#404, #287): absorb tool processor + ExecutorRegistry; inject registry directly instead of Dependencies.ToolRegistry |
| `processor/agentic-tools/executors` | 2 | 2 | 0 | `unported (agentic cut)` | `semteams agent runtime (ADR-061)` | semengine#29 | M1 (#402) design; M2 (#404): absorb builtin tool executors used by SemTeams |
| `processor/agentic-tools/runner` | 2 | 2 | 0 | `unported (agentic cut)` | `semteams agent runtime (ADR-061)` | semengine#8 (03B D4) | M1 (#402) design; M2 (#404): absorb sandbox/command runner used by SemTeams tools |
| `vocabulary/agentic` | 4 | 7 | 0 | `defer-exclude` | `semteams agent runtime (ADR-061)` | semengine#8 (03B D4), semengine#234 | M1 (#401, done): cmd/semteams/main.go calls agvocab.Register() directly; M1 (#402) design; M2 (#404): absorb agentic predicate constants and register via semengine/vocabulary |

### 4. Milestone 1 drop/decouple

| SemStreams subpath | Non-test | `_test.go` | `scripts/` | SemEngine disposition | Post-port home | Upstream refs | SemTeams action |
| --- | ---: | ---: | ---: | --- | --- | --- | --- |
| `componentregistry` | 0 | 0 | 0 | `unported (aggregator cut)` | `dropped in M1 (#399)` | semengine#3 (03B Q1) | M1 (#399): delete aggregator comparison in internal/runtimecatalog/catalog_test.go |
| `gateway/http` | 0 | 0 | 0 | `defer-exclude` | `dropped in M1 (#399)` | semengine#8 (03B D4) | M1 (#399): drop uncomposed factory from internal/runtimecatalog and regenerate schemas |
| `gateway/lifecycle-gateway` | 0 | 0 | 0 | `unported (gateway cut)` | `dropped in M1 (#399)` | semengine#8 (03B D4) | M1 (#399): drop uncomposed factory from internal/runtimecatalog and regenerate schemas |
| `governance` | 0 | 0 | 0 | `unported (governance cut)` | `dropped in M1 (#399)` | semengine#8 (03B D4), semengine#28 | M1 (#399): drop unused payload registration from internal/runtimecatalog/payloads.go once gated-dag/agentic-governance leave |
| `input/file` | 0 | 0 | 0 | `unported (I/O cut)` | `dropped in M1 (#399)` | semengine#8 (03B D4) | M1 (#399): drop uncomposed factory from internal/runtimecatalog and regenerate schemas |
| `input/udp` | 0 | 0 | 0 | `unported (I/O cut)` | `dropped in M1 (#399)` | semengine#8 (03B D4) | M1 (#399): drop uncomposed factory from internal/runtimecatalog and regenerate schemas |
| `input/websocket` | 0 | 0 | 0 | `unported (outside 04A)` | `dropped in M1 (#399)` | semengine#9 | M1 (#399): drop uncomposed factory from internal/runtimecatalog and regenerate schemas |
| `output/file` | 0 | 0 | 0 | `unported (I/O cut)` | `dropped in M1 (#399)` | semengine#8 (03B D4) | M1 (#399): drop uncomposed factory from internal/runtimecatalog and regenerate schemas |
| `output/httppost` | 0 | 0 | 0 | `unported (I/O cut)` | `dropped in M1 (#399)` | semengine#8 (03B D4) | M1 (#399): drop uncomposed factory from internal/runtimecatalog and regenerate schemas |
| `output/websocket` | 0 | 0 | 0 | `unported (setup-04a-05)` | `dropped in M1 (#399)` | semengine#34 | M1 (#399): drop uncomposed factory from internal/runtimecatalog and regenerate schemas |
| `payloadbuiltins` | 0 | 0 | 0 | `unported (aggregator cut)` | `dropped in M1 (#399)` | semengine#3 (03B Q1) | M1 (#399): delete aggregator comparison in internal/runtimecatalog/catalog_test.go |
| `pkg/logging` | 0 | 0 | 0 | `unported (cut)` | `dropped in M1 (#401)` | semengine#8 (tier1-cross-check) | M1 (#401, done): replaced pkg/logging in cmd/semteams/logging.go with stdlib slog.NewMultiHandler and product-owned natsLogHandler |
| `processor/agentic-governance` | 0 | 0 | 0 | `unported (agentic cut)` | `dropped in M1 (#399)` | semengine#8 (03B D4), semengine#28 | M1 (#399): drop uncomposed factory from internal/runtimecatalog and regenerate schemas |
| `processor/gated-dag` | 0 | 0 | 0 | `unported (aggregator cut)` | `dropped in M1 (#399)` | semengine#3 (03B Q1) | M1 (#399): drop uncomposed factory + payload registration from internal/runtimecatalog |
| `processor/graph-clustering` | 0 | 0 | 0 | `unported (next before 04a-06)` | `dropped in M1 (#399)` | semengine#9 (2026-11-08 ruling) | M1 (#399): drop uncomposed factory from internal/runtimecatalog (re-add from SemEngine only if NLQ text search is composed) |
| `processor/graph-embedding` | 0 | 0 | 0 | `unported (setup-04a-05)` | `dropped in M1 (#399)` | semengine#9 | M1 (#399): drop uncomposed factory from internal/runtimecatalog and regenerate schemas |
| `processor/graph-index` | 0 | 0 | 0 | `adapt (on SemEngine main)` | `dropped in M1 (#399)` | ADR-108, ADR-109 | M1 (#399): drop uncomposed factory from internal/runtimecatalog (available in semengine/processor/graph-index if needed later) |
| `processor/graph-index-spatial` | 0 | 0 | 0 | `unported (setup-04a-07)` | `dropped in M1 (#399)` | ADR-108, semengine#9 | M1 (#399): drop uncomposed factory from internal/runtimecatalog and regenerate schemas |
| `processor/graph-index-temporal` | 0 | 0 | 0 | `unported (setup-04a-07)` | `dropped in M1 (#399)` | ADR-108, semengine#9 | M1 (#399): drop uncomposed factory from internal/runtimecatalog and regenerate schemas |
| `processor/json_filter` | 0 | 0 | 0 | `unported (JSON cut)` | `dropped in M1 (#399)` | semengine#8 (03B D4) | M1 (#399): drop uncomposed factory from internal/runtimecatalog and regenerate schemas |
| `processor/json_generic` | 0 | 0 | 0 | `unported (JSON cut)` | `dropped in M1 (#399)` | semengine#8 (03B D4) | M1 (#399): drop uncomposed factory from internal/runtimecatalog and regenerate schemas |
| `processor/json_map` | 0 | 0 | 0 | `unported (JSON cut)` | `dropped in M1 (#399)` | semengine#8 (03B D4) | M1 (#399): drop uncomposed factory from internal/runtimecatalog and regenerate schemas |
| `vocabulary/builtins` | 0 | 0 | 0 | `unported (aggregator cut)` | `dropped in M1 (#401)` | semengine#3 (03B Q1), semengine#234 | M1 (#401, done): audited wired predicates (0 rulepacks hits) and replaced builtins.Register() in cmd/semteams/main.go with agvocab.Register() |

### `scripts/` string-literal-only occurrences

| Literal string | `scripts/` files |
| --- | --- |
| `github.com/c360studio/semstreams` | `scripts/measure-agent-contract-family.go`, `scripts/measure-agent-contract-family_test.go`, `scripts/measure-framework-footprint.go`, `scripts/measure-semengine-port-ledger.go` |
| `github.com/c360studio/semstreams/agentic-extra` | `scripts/measure-agent-contract-family_test.go` |
<!-- END GENERATED CROSSWALK -->

## Transitive and non-direct packages relevant to Milestone 2

Several SemEngine and frozen-SemStreams packages have zero direct AST imports in SemTeams today, so they do not appear as rows in the 56-path table above, yet they are load-bearing when Milestone 2 (`#404`–`#407`) lands:

| Package | Where it lives | Why Milestone 2 cares |
| --- | --- | --- |
| `github.com/c360studio/semengine/pkg/lifecyclecleanup` | SemEngine `main` (`semengine#77` CLOSED) | Exposed as a public helper specifically so consumer-owned processors (`agentic-dispatch`, `agentic-loop`, `agentic-model`, `agentic-tools`) can run bounded shutdown cleanup without duplicating internal lifecycle code. |
| `github.com/c360studio/semengine/pkg/retry` | SemEngine `main` | Replaces removed `natsclient.RetryConfig` and `natsclient.DefaultRetryConfig()` in `cmd/semteams/tools/addsource/executor.go` and absorbed agentic processors. |
| `semstreams/model/wire` | Frozen SemStreams `8b99efe9` (`defer-exclude`) | Imported transitively by `processor/agentic-model` and `processor/agentic-dispatch`; absorbed into SemTeams alongside the model/dispatch processors in `#404` (see [`agent-runtime-foundation/report.md`](agent-runtime-foundation/report.md)). |
| `semstreams/internal/{looptoken,deliverylane,agentterminal}` | Frozen SemStreams `8b99efe9` (`internal/`) | Private helpers behind `processor/agentic-loop` and `processor/agentic-dispatch`; absorbed into SemTeams internal packages in `#404`. |
| `configs/flow-bootstrap.json` `graph_index_queries` port | `configs/flow-bootstrap.json:746` | `processor/graph-index` is uncomposed as a component in both `configs/flow-bootstrap.json` and `configs/e2e-flow-bootstrap.json` (its unused factory registration leaves in `#399`), but `graph-gateway`'s port list at `configs/flow-bootstrap.json:746` still declares `graph_index_queries`. Reconcile this port declaration in `#402`/`#406` when replacing `gateway/graph-gateway`. |

## Verified symbol and wire adaptations (Group 1)

Comparing SemTeams's Group 1 symbol and wire usage against `C360Studio/semengine@01567cc48d4d89ede05dfdf0aa1675a68bf9402b` surfaced the following concrete compile-time and wire-format adaptations. Every item maps to an existing SemTeams Milestone 1 or Milestone 2 issue; none requires a new upstream SemEngine change.

| Package | SemStreams `8b99efe9` usage in SemTeams | SemEngine `01567cc` contract | SemTeams call sites & target issue |
| --- | --- | --- | --- |
| `component` | `component.PlatformMeta` | Moved to `types.PlatformMeta`; `component.Dependencies` drops `ToolRegistry` (`semengine#29`) and `LifecycleManager` | `cmd/semteams/rule_hot_reload_integration_test.go:82`, `cmd/semteams/main.go:207,212` (`#404`, `#407`) |
| `component` | `Registry.RegisterFactory` / `Registry.GetFactory` | Replaced by `Registry.Register(Registration)` and `Registry.ListFactories()` | `internal/runtimecatalog/catalog_test.go:112,113,223,224` (`#407`) |
| `composition` | `composition.AssertValid(t, registry, cfg)` | Replaced by `composition.Validate(catalog *component.Registry, cfg *config.Config) (*Result, error)` (`composition/validate.go:21`) | `test/contract/composition_contract_test.go:30` (`#407`) |
| `config` | `(*config.Manager).Stop(5 * time.Second)` | `(*config.Manager).Stop(ctx context.Context) error` (ADR-113); `SEMENGINE_` env override prefix; `semengine_config_<org>_<stem>` KV bucket; case-insensitive unknown-key rejection | `cmd/semteams/main.go:149`, `cmd/semteams/loop_runtime_integration_test.go:53`, `cmd/semteams/loop_cost_stamp_integration_test.go:139` (`#407`) |
| `graph` | `graph.NewExactEntityReader(requester, timeout)` returning interface | Returns concrete `(*graph.ExactEntityReader, error)` (`graph/exact_entity.go:38,46`) | `cmd/semteams/approvalpause/projection.go:49`, `cmd/semteams/loop_runtime_integration_test.go:95`, `cmd/semteams/loop_cost_stamp_integration_test.go:161` (`#407`) |
| `graph` | `ExactEntity.KVRevision` JSON tag `kvRevision` | JSON tag is snake_case `kv_revision` (`graph/exact_entity.go:19`) | `cmd/semteams/chain/entity_reader.go:189`, `cmd/semteams/chainpause/decision_handler.go:439`, test fixtures, and `ui/src/lib/services/graphApi.ts:228,236` (`#402`, `#406`, `#407`) |
| `graph` | `graph.ReconcilePredicatesResponse` (`KVRevision` JSON `kvRevision`) and `ReconcilePredicatesRequest` without `Source` | Renamed `graph.EntityMutationResponse` (`kv_revision`); `ReconcilePredicatesRequest` (`graph/mutation_requests.go:28-36`) requires non-empty `Source` matching every desired `message.Triple.Source`, and every desired `message.Triple` requires non-empty `Source` and non-zero `Timestamp` (ADR-110 Decision 5) | `cmd/semteams/approvalpause/projection.go:103,124` (`#407`) |
| `natsclient` | `natsclient.RetryConfig`, `natsclient.DefaultRetryConfig()` | Moved to `pkg/retry.Config` and `retry.DefaultConfig()` | `cmd/semteams/tools/addsource/executor.go:55,183` (`#407`) |
| `natsclient` | `natsclient.FilteredKeys(ctx, kv, pattern)` over `ListKeysFiltered` | Parameter interface uses `WatchFiltered(ctx, filter, ...WatchOpt)` (`jetstream.KeyValue` implements both; verify test doubles) | `cmd/semteams/tools/emitartifact/derivation.go:979` (`#407`) |
| `natsclient` | `NewTestClient`, `TestClient`, `CreateKVBucket`, `WithJetStream`, `WithKV`, `WithNATSVersion` | Removed from `semengine/natsclient` | 11 `_test.go` files (`10` under `cmd/semteams/**` + `test/contract/autoresearch_reply_integration_test.go`) → migrated to local `internal/natstest` helper (`#400`, done; Milestone 2 swaps `internal/natstest`'s `natsclient` import path) |
| `service` | `service.NewBaseServiceWithOptions(name, cfg, opts...)` | Drops unused `cfg *config.Config` parameter → `NewBaseServiceWithOptions(name, opts...)` | `cmd/semteams/rule_config_service.go:41` (`#407`) |
| `service` | `ComponentManager.Initialize()` | Removed; `NewComponentManager` constructs components directly | `cmd/semteams/loop_runtime_integration_test.go:76`, `cmd/semteams/loop_cost_stamp_integration_test.go:153`, `cmd/semteams/model_runtime_integration_test.go:104` (`#407`) |
| `service` | `service.MaybeStartPProf(debug, port)` | `service.MaybeStartPProf(debug, port, logger)` returning `*service.PProfServer` (`Shutdown(ctx)`) | `cmd/semteams/main.go:93` (`#407`) |
| `service` | `Manager.ConfigureFromServices(map[string]types.ServiceConfig, ...)` | Takes named `types.ServiceConfigs` | `cmd/semteams/main.go`, integration tests (`#407`) |
| `service` | `service.NewMilestoneService` | Excluded from SemEngine (`semengine#30` CLOSED) | `cmd/semteams/main.go:858` → absorb alongside `agentic/agentrun` (`#404`) |
| `vocabulary` | `vocabulary.RegisterPredicate(PredicateMetadata{...})` | Removed (`semengine#72` CLOSED); replaced by `vocabulary.Register(iri, opts...)` and `vocabulary.RegisterPack` | `cmd/semteams/vocab/vocab.go` → migrated to `vocabulary.Register` (`#401`, done; `#407` swaps import path to `semengine/vocabulary`) |
| `processor/graph-query` | Legacy query reply fields & `globalSearch.max_communities` | 4-key `graph.QueryResponse` envelope (`data`, `indexed_revision`, `producer`, `timestamp`), `kv_revision`, `max_communities` rejected as `invalid_request`, and text-tier `globalSearch`/`searchGraph` returning transient `index_not_ready` unless `COMMUNITY_INDEX` has a writer (`processor/graph-query/MIGRATION.md`) | `ui/src/lib/services/graphApi.ts:294` (`maxCommunities`), trimmed SemTeams `graph-gateway` (`#402`, `#406`) |

## Consumer-facing SemEngine ADRs (`ADR-108`–`ADR-113`) and Milestone 2 gate

### ADR-108 — Processor shell (`Start` / `Stop` / health / metrics)

SemEngine processors expose a uniform lifecycle shell (`Initialize`, `Start(ctx)`, `Stop(ctx context.Context) error` per `component/lifecycle.go:90`, health, and `semengine_`-namespaced metrics) without ad-hoc background goroutines escaping `Stop`.

- **SemTeams impact:** Absorbed Milestone 2 processors (`agentic-dispatch`, `agentic-loop`, `agentic-model`, `agentic-tools`) and the trimmed `graph-gateway` (`#402`, `#404`, `#406`) adopt `Stop(ctx)` and use `semengine/pkg/lifecyclecleanup` (`semengine#77`) for bounded shutdown cleanup.

### ADR-109 — Readiness on query and service replies

Query and service endpoints do not silently serve empty results before projection/index readiness; replies carry explicit readiness/revision metadata (`indexed_revision`, `kv_revision`) and return typed transient `index_not_ready` errors when backing buckets or writers are not ready.

- **SemTeams impact:** In `#402` and `#406`, the trimmed SemTeams `graph-gateway` and `ui/src/lib/services/graphApi.ts` must consume `graph.QueryResponse` (`indexed_revision`, `kv_revision`), stop sending removed `max_communities`, and decide whether NLQ text-tier queries remain out of SemTeams's trimmed schema or require composing `processor/graph-clustering` as the `COMMUNITY_INDEX` writer.

### ADR-110 — Single graph write seam and statement provenance

`processor/graph-ingest` is the sole authority writing `ENTITY_STATES`. Every stored statement (`message.Triple`) carries a non-empty `Source` and non-zero `Timestamp`, and `ReconcilePredicatesRequest` carries a request-level `Source` matching its desired triples.

- **SemTeams impact:** `cmd/semteams/approvalpause/projection.go:103,124` (`reconcileEntity`) must populate `ReconcilePredicatesRequest.Source`, ensure every desired `message.Triple` sets matching `Source` and non-zero `Timestamp`, and unmarshal `graph.EntityMutationResponse` (`#407`). Absorbed agentic/product-shell emitters (`#404`, `#405`) must preserve `Source` and `Timestamp` on emitted triples.

### ADR-112 — `natsclient` consumer ownership and `pkg/errs` classification

Components register JetStream consumers through `natsclient` ownership APIs and drain handlers deterministically on shutdown via `WaitConsumerHandlers` rather than raw unmanaged loops. `pkg/errs.Classify` defaults unknown errors to fatal and classifies strictly by typed wrapper or sentinel (never substring matching); `pkg/projection.MutationError` over `nats.ErrNoResponders` sets its own `Class` field to transient while `errs.IsTransient(nats.ErrNoResponders)` remains `false`.

- **SemTeams impact:** Absorbed `agentic-{dispatch,loop,model,tools}` processors (`#404`) and `cmd/semteams/approvalpause` / `chainpause` callers (`#407`) must rely on typed `errs` / `projection.MutationError` classification and `WaitConsumerHandlers` on stop.

### ADR-113 — `Start` context bounds `Start` only (`semengine#361`)

Under SemEngine ADR-113 ("Stop Is the Only Stop"), `Stop(ctx context.Context) error` is the sole shutdown signal and the `ctx` passed to `service.Manager.StartAll(ctx)` and `component.Startable.Start(ctx)` bounds **startup readiness only**; components and services derive their own internal runtime lifecycle context cancelled by `Stop(ctx)`, rather than tying background loops to `Start`'s `ctx`.

- **SemTeams impact (two concrete call sites):**
  1. `cmd/semteams/main.go:901-904,916` currently passes an uncancelled `context.Background()` (`ctx`) instead of `signalCtx` to `manager.StartAll(ctx)` because SemStreams `8b99efe9` components bound long-lived loops to `Start`'s `ctx`. When rewiring to SemEngine in `#407`, `main.go` **must** pass `signalCtx` (or a startup-timeout child of `signalCtx`) into `StartAll` so `SIGINT`/`SIGTERM` aborts a hung boot cleanly (`semengine#361`), and every absorbed Milestone 2 component (`#404`) must own its internal run context until `Stop(ctx)`.
  2. `cmd/semteams/rule_config_service.go:56,73` (`ruleConfigService.Start(ctx)` / `Stop(ctx)`) delegates to `rule.ConfigManager.Start(ctx)`; verify during `#405`/`#407` that `ConfigManager` and `ruleConfigService` keep KV watch reconciliation alive until `Stop(ctx)` after `Start(ctx)` returns.

### Milestone 2 parking gate (`semengine#9` 2026-11-08 ruling)

Per the 2026-11-08 owner ruling on `C360Studio/semengine#9` (`issuecomment-6724212898`), SemEngine ports `processor/graph-clustering` immediately after `setup-04a-05` (`graph-embedding`) and cuts `v0.1.0-alpha.3` for `C360Studio/semsource` before starting `setup-04a-06` (`processor/rule`). SemTeams Milestone 2 (`#404`–`#407`, `#287`) remains parked until:

1. Milestone 1 (`#398`–`#403`, including the `#403` `8b99efe9` baseline floor under `docs/migrations/semengine-port-baseline/`) is complete;
2. SemEngine lands `setup-04a-06` (`processor/rule` + `E1`–`E4` action-family/trigger seams `#25`–`#28`); and
3. SemEngine lands `semengine#24` (`ADR-111` execution durability) and `semengine#361` (`ADR-113` host `StartAll` caller audit).

## Upstream substrate gap audit (`C360Studio/semengine`)

Auditing all 56 SemStreams import paths against `C360Studio/semengine@01567cc48d4d89ede05dfdf0aa1675a68bf9402b` and open/closed upstream issues found **zero unfiled substrate gaps** outside packages already excluded in SemEngine 03B D4 (`semengine#8`) or aggregator cuts (`semengine#3`). No new `C360Studio/semengine` issues were needed.

| Upstream issue | State (2026-11-08) | Coverage for SemTeams |
| --- | --- | --- |
| `semengine#3` | CLOSED | Rules `componentregistry`, `payloadbuiltins`, `vocabulary/builtins`, and `processor/gated-dag` out of SemEngine (`#399`, `#401`). |
| `semengine#8` | CLOSED | 03B D4 boundary excluding `agentic*`, `persona`, `vocabulary/agentic`, `gateway/graph-gateway`, `gateway/http`, `governance`, `pkg/logging`, JSON processors, and file/UDP/HTTP I/O (`#399`, `#401`, `#402`, `#404`, `#406`). |
| `semengine#9` | OPEN | Tracks 04A wave (`graph-embedding` → `graph-clustering` → `setup-04a-06` `processor/rule` → spatial/temporal). |
| `semengine#24` | OPEN | ADR-111 execution durability contract gating Milestone 2 (`#404`, `#405`). |
| `semengine#25`–`#28` | OPEN | `setup-04a-06` `E1`–`E4` rule action-family registry, subject/payload decoupled actions, `RuleTrigger` interface, and governance hook removal (`#405`, `#287`). |
| `semengine#29` | OPEN | Confirms `component.Dependencies.ToolRegistry` removal and executor injection seam for absorbed `agentic-tools` (`#404`). |
| `semengine#30` | CLOSED | Excludes `service.MilestoneService` from SemEngine; SemTeams absorbs it with `agentic/agentrun` (`#404`). |
| `semengine#72`, `#234` | CLOSED | Replaces `vocabulary.RegisterPredicate` with `vocabulary.Register` / `RegisterPack` (`#401`). |
| `semengine#77` | CLOSED | Publishes `semengine/pkg/lifecyclecleanup` for absorbed consumer components (`#404`). |
| `semengine#97` | CLOSED | Ports hierarchy inference slice in `semengine/graph/inference` (`#399`, `#406`). |
| `semengine#155` | OPEN | Tracks `semengine/model` reader/endpoint cleanup ahead of the Milestone 2 pin (`#404`, `#407`). |
| `semengine#361` | OPEN | Tracks ADR-113 `StartAll` caller-context semantics (`cmd/semteams/main.go:901-916` in `#407`). |
