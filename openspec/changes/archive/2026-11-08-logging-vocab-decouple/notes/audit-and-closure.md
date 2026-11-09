# Predicate Audit, Dependency Closure Delta, and Mutation Evidence (`#401`, `logging-vocab-decouple`)

Measured on `darwin/arm64` (`go1.26.4`, default build tags) against frozen `github.com/c360studio/semstreams v1.0.0-beta.162.0.20260930150212-8b99efe9c66a` (`8b99efe9c66a4faa4fa509f9f62cc6bad8392128`), starting from branch base commit `916861f09de6bafe2f562c14e8d4dcc6dd7cdadd`.

## 1. Wired-Rule and Projection-Contract Predicate Ownership Inventory

Boot-time authoring validation (`vocabulary.RequireDeclaredPredicate` at `semstreams/vocabulary/namespace_authority.go:47`) runs across:
- Rule `conditions[].field` (non-`$` fields; `processor/rule/config_validation.go:306`)
- Rule triple-mutation action `predicate` (`processor/rule/config_validation.go:376`)
- Projection contract group `predicates` (`pkg/projection/contract/contract.go:132`), covering both `configs/flow-bootstrap.json` / `configs/e2e-flow-bootstrap.json` `projection_contracts` and `cmd/semteams/main.go`'s `agentic.LoopExecutionContract()` / `agentic.LessonContract()`
- Lifecycle participant struct tags and workflow phase predicates (`pkg/lifecycle/tags.go:192`, `pkg/lifecycle/workflow.go:246`)

### 1.1 Summary by declaring package

| Declaring package (`8b99efe9c66a` / repo) | Registration mechanism | Post-port home (`ADR-061`) | Total declared | Wired rule/projection `RequireDeclaredPredicate` hits | Wired `$entity.triple.*`-only hits |
| --- | --- | --- | ---: | ---: | ---: |
| `semstreams/vocabulary` (`labels.go:16`, `lifecycle.go:16`, `hierarchy.go:17`, `relationships.go:7`) | Package `init()` (`dc.terms.title`, 5 `lifecycle.transition.*`, 7 `hierarchy.*.*`, 12 `graph.rel.*`) | SemEngine `semengine/vocabulary` (Group 1) | 25 | 0 in rule/flow JSON (used by `pkg/lifecycle` & `graph/inference`) | 0 |
| `semstreams/agentic/agentrun` (`agentrun.go:94-106`) | Package `init()` (`agent.run.phase`, 4 `agent.run.last-transition-*`, `agent.run.parent-entity-id`, `agent.run.origin-entity-id`) | SemTeams M2 `#404` (Group 3) | 7 (1 shared with `vocabulary/agentic`) | 5 (`4` exclusive + `agent.run.origin-entity-id`) | 0 |
| `semstreams/vocabulary/agentic` (`register.go`) | Explicit `agvocab.Register()` in `cmd/semteams/main.go` | SemTeams M2 `#404` (Group 3) | 127 | 7 (`6` exclusive + `agent.run.origin-entity-id`; also backs `LoopExecutionContract` & `LessonContract`) | 1 (`coordinator.decision.reason`) |
| `cmd/semteams/vocab` (`vocab.go:36-44`) | Explicit `vocab.Register()` via `vocabulary.Register` in `cmd/semteams/main.go` | SemTeams product shell (`cmd/semteams/vocab`) | 38 | 22 | 12 (remaining 4 are Go pause/ops/evidence stamps) |
| `semstreams/vocabulary/rulepacks` (`predicates.go:37-62`) | Unported (`init()` + `Register()`, previously pulled in via `vocabulary/builtins`) | Dropped in M1 `#401` (Group 4) | 13 | **0** | **0** |

### 1.2 All 33 distinct predicates on boot-validated rule & `projection_contracts` surfaces

Counted across all **49** `rules_files` wired in `configs/flow-bootstrap.json` and `configs/e2e-flow-bootstrap.json` (`TestBootstrapRuleListsMatch`) plus all `projection_contracts` groups in both flow configs:

| Predicate | Rule `conditions` files | Rule `actions` files | Flow `projection_contracts` configs | `$entity.triple.*` files | Declaring package → post-port home |
| --- | ---: | ---: | ---: | ---: | --- |
| `agent.lineage.run-loop-entity-id` | 7 | 0 | 0 | 11 | `cmd/semteams/vocab.Register()` → SemTeams product shell |
| `agent.loop.outcome` | 12 | 0 | 0 | 0 | `semstreams/vocabulary/agentic.Register()` → SemTeams M2 `#404` |
| `agent.loop.reply-to` | 1 | 0 | 0 | 0 | `semstreams/vocabulary/agentic.Register()` → SemTeams M2 `#404` |
| `agent.loop.role` | 35 | 0 | 0 | 2 | `semstreams/vocabulary/agentic.Register()` → SemTeams M2 `#404` |
| `agent.run.approval-outstanding` | 2 | 0 | 0 | 0 | `cmd/semteams/vocab.Register()` → SemTeams product shell |
| `agent.run.clarification-pending` | 2 | 3 | 0 | 0 | `cmd/semteams/vocab.Register()` → SemTeams product shell |
| `agent.run.clarification-resumed` | 2 | 2 | 0 | 0 | `cmd/semteams/vocab.Register()` → SemTeams product shell |
| `agent.run.entity-id` | 8 | 0 | 0 | 7 | `semstreams/vocabulary/agentic.Register()` → SemTeams M2 `#404` |
| `agent.run.handoff` | 1 | 2 | 0 | 1 | `cmd/semteams/vocab.Register()` → SemTeams product shell |
| `agent.run.last-transition-from` | 1 | 0 | 0 | 0 | `semstreams/agentic/agentrun` `init()` → SemTeams M2 `#404` |
| `agent.run.last-transition-note` | 1 | 0 | 0 | 0 | `semstreams/agentic/agentrun` `init()` → SemTeams M2 `#404` |
| `agent.run.last-transition-source` | 1 | 0 | 0 | 0 | `semstreams/agentic/agentrun` `init()` → SemTeams M2 `#404` |
| `agent.run.origin-entity-id` | 3 | 0 | 0 | 3 | `semstreams/agentic/agentrun` `init()` + `vocabulary/agentic.Register()` → SemTeams M2 `#404` |
| `agent.run.outcome` | 3 | 7 | 0 | 0 | `cmd/semteams/vocab.Register()` → SemTeams product shell |
| `agent.run.phase` | 12 | 0 | 0 | 1 | `semstreams/agentic/agentrun` `init()` → SemTeams M2 `#404` |
| `autoresearch.best.experiment-id` | 0 | 1 | 2 | 1 | `cmd/semteams/vocab.Register()` → SemTeams product shell |
| `autoresearch.best.value` | 0 | 1 | 2 | 3 | `cmd/semteams/vocab.Register()` → SemTeams product shell |
| `autoresearch.experiment.completed` | 0 | 2 | 0 | 2 | `cmd/semteams/vocab.Register()` → SemTeams product shell |
| `autoresearch.experiment.loop-failed` | 0 | 1 | 0 | 0 | `cmd/semteams/vocab.Register()` → SemTeams product shell |
| `autoresearch.iteration.pending` | 1 | 3 | 2 | 0 | `cmd/semteams/vocab.Register()` → SemTeams product shell |
| `autoresearch.measurement.outcome` | 1 | 0 | 0 | 0 | `cmd/semteams/vocab.Register()` → SemTeams product shell |
| `autoresearch.reply.approved` | 2 | 2 | 0 | 2 | `cmd/semteams/vocab.Register()` → SemTeams product shell |
| `autoresearch.reply.clarification` | 2 | 2 | 0 | 2 | `cmd/semteams/vocab.Register()` → SemTeams product shell |
| `autoresearch.reply.failed` | 2 | 3 | 0 | 2 | `cmd/semteams/vocab.Register()` → SemTeams product shell |
| `autoresearch.run.cap` | 1 | 0 | 0 | 1 | `cmd/semteams/vocab.Register()` → SemTeams product shell |
| `autoresearch.run.status` | 1 | 1 | 2 | 0 | `cmd/semteams/vocab.Register()` → SemTeams product shell |
| `chain.paused.marker` | 0 | 2 | 0 | 0 | `cmd/semteams/vocab.Register()` → SemTeams product shell |
| `coordinator.clarification.question` | 0 | 1 | 0 | 0 | `cmd/semteams/vocab.Register()` → SemTeams product shell |
| `coordinator.clarification.reply` | 0 | 1 | 0 | 0 | `cmd/semteams/vocab.Register()` → SemTeams product shell |
| `coordinator.decision.next-action` | 22 | 0 | 0 | 0 | `semstreams/vocabulary/agentic.Register()` → SemTeams M2 `#404` |
| `coordinator.decision.subtopics` | 2 | 0 | 0 | 2 | `semstreams/vocabulary/agentic.Register()` → SemTeams M2 `#404` |
| `research.gather.completed-subtopic` | 1 | 1 | 0 | 1 | `cmd/semteams/vocab.Register()` → SemTeams product shell |
| `rule.task.spawned` | 2 | 0 | 0 | 0 | `cmd/semteams/vocab.Register()` → SemTeams product shell |

### 1.3 Additional `$entity.triple.<predicate>` template-substitution predicates in wired rules (14)

14 additional canonical predicates appear in the 49 wired rules exclusively inside `$entity.triple.<predicate>` prompt/subject/object interpolation tokens:
- **12** declared in `cmd/semteams/vocab.Register()`: `agent.lineage.autoresearch-run` (4), `agent.lineage.command` (1), `agent.lineage.metric-parser` (1), `agent.lineage.plan-loop-entity-id` (1), `agent.lineage.researcher` (2), `agent.lineage.surface` (1), `autoresearch.artifact.path` (1), `autoresearch.measurement.value` (1), `autoresearch.run.command` (1), `autoresearch.run.metric-parser` (1), `autoresearch.run.surface` (1), `research.artifact.path` (1).
- **1** declared in `semstreams/vocabulary/agentic.Register()`: `coordinator.decision.reason` (2).
- **1** runtime-only tool triple (`autoresearch.baseline.value` in `configs/rules/autoresearch/05-iteration-dispatch.json` prompt text): stamped at runtime by `cmd/semteams/tools/emitautoresearchbaseline/executor.go:66` (runtime graph persistence uses syntax-only `vocabulary.ParsePredicate`, not `RequireDeclaredPredicate`).

The remaining 4 of the 38 predicates in `cmd/semteams/vocab/vocab.go` (`agent.run.approval-pending`, `agent.run.approval-answered`, `agent.lineage.trigger-loop`, `chain.evidence.summary-ready`) are product-shell Go subscriber / ops lineage declarations (`cmd/semteams/approvalpause`, `cmd/semteams/chainpause`, ops observer spawns).

### 1.4 13 `semstreams/vocabulary/rulepacks` predicates with 0 wired references

Every predicate declared in `semstreams/vocabulary/rulepacks/predicates.go:8-62` has **0** references across all 49 wired rules (conditions, actions, and `$entity.triple.*` tokens) and both flow configs' `projection_contracts`, and is now asserted unregistered by `TestWiredPredicatesDeclaredWithoutBuiltinsAggregator`:

1. `agentic.checkpoint.completed`
2. `agentic.checkpoint.iteration`
3. `agentic.checkpoint.started`
4. `agentic.decision.detected`
5. `agentic.file.modified`
6. `agentic.tool.file-operation`
7. `agentic.tool.used`
8. `entity.identity.type`
9. `gather.child.completed`
10. `workflow.review.rejections`
11. `workflow.state.phase`
12. `workflow.state.status`
13. `workflow.tokens.total`

---

## 2. `./cmd/semteams` Dependency Closure Delta

| Target & Command | Pre-`#401` Total | Post-`#401` Total | Total Delta | Pre-`#401` `semstreams` | Post-`#401` `semstreams` | `semstreams` Delta |
| --- | ---: | ---: | ---: | ---: | ---: | ---: |
| `go list -deps ./cmd/semteams` | 553 | 550 | -3 | 74 | 71 | -3 |

Direct repository AST import paths (`go run ./scripts/measure-semengine-port-ledger.go`):
- Active `github.com/c360studio/semstreams/...` import paths across repository `.go` files: **35 → 33** (**23 of 23** Group 4 Milestone 1 paths now pruned; **0** active Group 4 imports remain).
- Note on `internal/logforwarderpolicy`: `github.com/c360studio/semstreams/internal/logforwarderpolicy` remains in the 71-package transitive closure because `github.com/c360studio/semstreams/service` imports it internally at `8b99efe9c66a`; it leaves when `service` rewires to `semengine/service` in `#407`.

### 3 SemStreams packages removed from `go list -deps ./cmd/semteams`

1. `github.com/c360studio/semstreams/pkg/logging`
2. `github.com/c360studio/semstreams/vocabulary/builtins`
3. `github.com/c360studio/semstreams/vocabulary/rulepacks`

Zero packages were added (`cmd/semteams/logging.go` uses Go 1.26 stdlib `log/slog.NewMultiHandler` + `slices.Clone`).

### Pre-`#401` `github.com/c360studio/semstreams/...` Closure (`./cmd/semteams`, 74 packages)

```text
github.com/c360studio/semstreams/agentic
github.com/c360studio/semstreams/agentic/agentrun
github.com/c360studio/semstreams/component
github.com/c360studio/semstreams/component/flowgraph
github.com/c360studio/semstreams/composition
github.com/c360studio/semstreams/config
github.com/c360studio/semstreams/gateway
github.com/c360studio/semstreams/gateway/graph-gateway
github.com/c360studio/semstreams/governance
github.com/c360studio/semstreams/graph
github.com/c360studio/semstreams/graph/clustering
github.com/c360studio/semstreams/graph/embedding
github.com/c360studio/semstreams/graph/inference
github.com/c360studio/semstreams/graph/llm
github.com/c360studio/semstreams/graph/query
github.com/c360studio/semstreams/graph/readiness
github.com/c360studio/semstreams/graph/structural
github.com/c360studio/semstreams/health
github.com/c360studio/semstreams/internal/agentterminal
github.com/c360studio/semstreams/internal/componentadmission
github.com/c360studio/semstreams/internal/deliverylane
github.com/c360studio/semstreams/internal/graphmutation
github.com/c360studio/semstreams/internal/lifecyclecleanup
github.com/c360studio/semstreams/internal/logforwarderpolicy
github.com/c360studio/semstreams/internal/looptoken
github.com/c360studio/semstreams/message
github.com/c360studio/semstreams/metric
github.com/c360studio/semstreams/model
github.com/c360studio/semstreams/model/wire
github.com/c360studio/semstreams/model/wire/responses
github.com/c360studio/semstreams/natsclient
github.com/c360studio/semstreams/payloadregistry
github.com/c360studio/semstreams/persona
github.com/c360studio/semstreams/pkg/acme
github.com/c360studio/semstreams/pkg/cache
github.com/c360studio/semstreams/pkg/dispatch
github.com/c360studio/semstreams/pkg/errs
github.com/c360studio/semstreams/pkg/graphview
github.com/c360studio/semstreams/pkg/lifecycle
github.com/c360studio/semstreams/pkg/logging
github.com/c360studio/semstreams/pkg/platform
github.com/c360studio/semstreams/pkg/projection
github.com/c360studio/semstreams/pkg/projection/contract
github.com/c360studio/semstreams/pkg/resource
github.com/c360studio/semstreams/pkg/retry
github.com/c360studio/semstreams/pkg/rulepack
github.com/c360studio/semstreams/pkg/security
github.com/c360studio/semstreams/pkg/timestamp
github.com/c360studio/semstreams/pkg/tlsutil
github.com/c360studio/semstreams/pkg/types
github.com/c360studio/semstreams/pkg/worker
github.com/c360studio/semstreams/processor/agentic-dispatch
github.com/c360studio/semstreams/processor/agentic-loop
github.com/c360studio/semstreams/processor/agentic-loop/internal/loopbucket
github.com/c360studio/semstreams/processor/agentic-loop/internal/looprequest
github.com/c360studio/semstreams/processor/agentic-loop/lessonmatch
github.com/c360studio/semstreams/processor/agentic-loop/prompt
github.com/c360studio/semstreams/processor/agentic-model
github.com/c360studio/semstreams/processor/agentic-tools
github.com/c360studio/semstreams/processor/agentic-tools/executors
github.com/c360studio/semstreams/processor/agentic-tools/runner
github.com/c360studio/semstreams/processor/graph-ingest
github.com/c360studio/semstreams/processor/graph-query
github.com/c360studio/semstreams/processor/rule
github.com/c360studio/semstreams/processor/rule/expression
github.com/c360studio/semstreams/service
github.com/c360studio/semstreams/storage
github.com/c360studio/semstreams/storage/objectstore
github.com/c360studio/semstreams/storage/storeregistry
github.com/c360studio/semstreams/types
github.com/c360studio/semstreams/vocabulary
github.com/c360studio/semstreams/vocabulary/agentic
github.com/c360studio/semstreams/vocabulary/builtins
github.com/c360studio/semstreams/vocabulary/rulepacks
```

### Post-`#401` `github.com/c360studio/semstreams/...` Closure (`./cmd/semteams`, 71 packages)

```text
github.com/c360studio/semstreams/agentic
github.com/c360studio/semstreams/agentic/agentrun
github.com/c360studio/semstreams/component
github.com/c360studio/semstreams/component/flowgraph
github.com/c360studio/semstreams/composition
github.com/c360studio/semstreams/config
github.com/c360studio/semstreams/gateway
github.com/c360studio/semstreams/gateway/graph-gateway
github.com/c360studio/semstreams/governance
github.com/c360studio/semstreams/graph
github.com/c360studio/semstreams/graph/clustering
github.com/c360studio/semstreams/graph/embedding
github.com/c360studio/semstreams/graph/inference
github.com/c360studio/semstreams/graph/llm
github.com/c360studio/semstreams/graph/query
github.com/c360studio/semstreams/graph/readiness
github.com/c360studio/semstreams/graph/structural
github.com/c360studio/semstreams/health
github.com/c360studio/semstreams/internal/agentterminal
github.com/c360studio/semstreams/internal/componentadmission
github.com/c360studio/semstreams/internal/deliverylane
github.com/c360studio/semstreams/internal/graphmutation
github.com/c360studio/semstreams/internal/lifecyclecleanup
github.com/c360studio/semstreams/internal/logforwarderpolicy
github.com/c360studio/semstreams/internal/looptoken
github.com/c360studio/semstreams/message
github.com/c360studio/semstreams/metric
github.com/c360studio/semstreams/model
github.com/c360studio/semstreams/model/wire
github.com/c360studio/semstreams/model/wire/responses
github.com/c360studio/semstreams/natsclient
github.com/c360studio/semstreams/payloadregistry
github.com/c360studio/semstreams/persona
github.com/c360studio/semstreams/pkg/acme
github.com/c360studio/semstreams/pkg/cache
github.com/c360studio/semstreams/pkg/dispatch
github.com/c360studio/semstreams/pkg/errs
github.com/c360studio/semstreams/pkg/graphview
github.com/c360studio/semstreams/pkg/lifecycle
github.com/c360studio/semstreams/pkg/platform
github.com/c360studio/semstreams/pkg/projection
github.com/c360studio/semstreams/pkg/projection/contract
github.com/c360studio/semstreams/pkg/resource
github.com/c360studio/semstreams/pkg/retry
github.com/c360studio/semstreams/pkg/rulepack
github.com/c360studio/semstreams/pkg/security
github.com/c360studio/semstreams/pkg/timestamp
github.com/c360studio/semstreams/pkg/tlsutil
github.com/c360studio/semstreams/pkg/types
github.com/c360studio/semstreams/pkg/worker
github.com/c360studio/semstreams/processor/agentic-dispatch
github.com/c360studio/semstreams/processor/agentic-loop
github.com/c360studio/semstreams/processor/agentic-loop/internal/loopbucket
github.com/c360studio/semstreams/processor/agentic-loop/internal/looprequest
github.com/c360studio/semstreams/processor/agentic-loop/lessonmatch
github.com/c360studio/semstreams/processor/agentic-loop/prompt
github.com/c360studio/semstreams/processor/agentic-model
github.com/c360studio/semstreams/processor/agentic-tools
github.com/c360studio/semstreams/processor/agentic-tools/executors
github.com/c360studio/semstreams/processor/agentic-tools/runner
github.com/c360studio/semstreams/processor/graph-ingest
github.com/c360studio/semstreams/processor/graph-query
github.com/c360studio/semstreams/processor/rule
github.com/c360studio/semstreams/processor/rule/expression
github.com/c360studio/semstreams/service
github.com/c360studio/semstreams/storage
github.com/c360studio/semstreams/storage/objectstore
github.com/c360studio/semstreams/storage/storeregistry
github.com/c360studio/semstreams/types
github.com/c360studio/semstreams/vocabulary
github.com/c360studio/semstreams/vocabulary/agentic
```

---

## 3. Non-Destructive Mutation Checks (`task mutate:check`)

Mutant files were generated in host-local scratch `/tmp/semteams-mutate-401/` (outside the repository) and deleted immediately after verification. Original file SHA-256s:
- `cmd/semteams/logging.go`: `bd873c999a013e4ce730848c4b31dd43da2010d7646142c63923b6acf67b7d64`
- `cmd/semteams/vocab/vocab.go`: `bc08e83b929bb4adaa458b6429f4b865d5c956932a51e21c1853482ae765ad10`

| # | Target file & mutated behavior | Mutant SHA-256 (local `/tmp` scratch) | Command | Matched failure & verdict |
| --- | --- | --- | --- | --- |
| 1 | `cmd/semteams/logging.go`: break `source > component > service` precedence (`service` checked before `source`/`component`) | `2e25e57fd6bfdfe8d4b17791d80e731e28811a50402aa9817cc38c21a1ac0e80` | `task mutate:check -- -pkg ./cmd/semteams -test TestNATSLogHandlerSourcePrecedenceAndPayload -file cmd/semteams/logging.go -mutant /tmp/semteams-mutate-401/logging_m1_precedence.go -expect logging_test.go:213` | `logging_test.go:213: component subject = "logs.WARN.svc-a", want logs.WARN.comp-b` → **`detection`** |
| 2 | `cmd/semteams/logging.go`: drop dot-boundary in `isExcluded` (`strings.HasPrefix(source, prefix)` instead of `source == prefix \|\| strings.HasPrefix(source, prefix+".")`) | `74f872a6488ba71c60eb70c616bfe2797b35224cf3789f95527c9dc96aa5a31a` | `task mutate:check -- -pkg ./cmd/semteams -test TestGetExcludeSourcesAndPrefixFiltering -file cmd/semteams/logging.go -mutant /tmp/semteams-mutate-401/logging_m2_exclude_prefix.go -expect-text "timed out waiting for async NATS log publish"` | `logging_test.go:305: timed out waiting for async NATS log publish` (sibling `flow-service.websocket2` wrongly suppressed) → **`detection`** |
| 3 | `cmd/semteams/logging.go`: swap level and source in NATS subject (`logs.<source>.<LEVEL>`) | `2ad2f9f94c4fc77df37719817640ac6f19dd0dd2ed9223ce2d1d62468b1bd15c` | `task mutate:check -- -pkg ./cmd/semteams -test TestNATSLogHandlerSourcePrecedenceAndPayload -file cmd/semteams/logging.go -mutant /tmp/semteams-mutate-401/logging_m3_subject.go -expect logging_test.go:185` | `logging_test.go:185: bare subject = "logs.system.INFO", want logs.INFO.system` → **`detection`** |
| 4 | `cmd/semteams/vocab/vocab.go`: omit `vocabulary.WithDataType(p.dataType)` in `vocab.Register()` | `150b810ace9c1cec6f91dfdf5b0b0c3828cc93384aeddc61f2db5db01797cb6c` | `task mutate:check -- -pkg ./cmd/semteams/vocab -test TestRegisterDeclaresSemTeamsVocabularyIdempotently -file cmd/semteams/vocab/vocab.go -mutant /tmp/semteams-mutate-401/vocab_m4_no_datatype.go -expect vocab_test.go:249` | `vocab_test.go:249: research.gather.completed-subtopic: DataType = "", want "string"` (+ 37 siblings) → **`detection`** |
| 5 | `cmd/semteams/vocab/vocab.go`: skip `predicates[0]` (`research.gather.completed-subtopic`) in `vocab.Register()` | `36f3aa6a59732794607a94517cb1934166b26209decc0c08c2a556b3d8c92fbb` | `task mutate:check -- -pkg ./test/contract -test TestWiredPredicatesDeclaredWithoutBuiltinsAggregator -file cmd/semteams/vocab/vocab.go -mutant /tmp/semteams-mutate-401/vocab_m5_skip_predicate.go -expect-text "research.gather.completed-subtopic"` | `predicate_contract_test.go:729: ... predicate "research.gather.completed-subtopic" is canonical but not declared in the vocabulary registry` → **`detection`** |

---

## 4. Framework-Alignment Review ("Before adding anything new", `.agents/contracts/go-developer.md`)

1. **Who owns this responsibility today?**
   - *Logger composition & NATS log forwarding (`cmd/semteams/logging.go`):* SemEngine explicitly cuts `pkg/logging` (`C360Studio/semengine#8`) and Go 1.26 stdlib provides `log/slog.NewMultiHandler` directly. The product composition root (`cmd/semteams`, `package main`) owns stdout + `logs.<LEVEL>.<source>` handler wiring over the existing `LOGS` stream (`configs/flow-bootstrap.json`).
   - *Boot predicate registration (`cmd/semteams/main.go`, `cmd/semteams/vocab/vocab.go`):* SemEngine cuts `vocabulary/builtins` (`C360Studio/semengine#3`) and removes `vocabulary.RegisterPredicate` in favor of `vocabulary.Register` (`C360Studio/semengine#72`). `semstreams/vocabulary/agentic.Register()` owns agentic vocabulary declarations (scheduled for Milestone 2 absorption under `#404` per `ADR-061`), and `cmd/semteams/vocab.Register()` owns the 38 product-authored SemTeams predicates. No new tool, rule action, payload type, KV bucket, stream, or config key is introduced.
2. **Is the premise true?**
   - Verified against frozen `semstreams@v1.0.0-beta.162.0.20260930150212-8b99efe9c66a`: `vocabulary/builtins/register.go:12-15` calls only `agentic.Register()` and `rulepacks.Register()`; zero of the 13 predicates in `vocabulary/rulepacks/predicates.go:8-62` appear anywhere in SemTeams's 49 wired rules or projection contracts; and `cmd/semteams/vocab/vocab.go` declares 38 predicates whose `Description` and `DataType` metadata are preserved identically via `vocabulary.Register(name, WithDescription(...), WithDataType(...))`.
3. **Who is it for?**
   - `logStreamPublisher`, `newLogger`, and `natsLogHandler` (all unexported inside `cmd/semteams` `package main`) serve `setupLogger` called by `cmd/semteams/main.go:133`, backed by `openspec/specs/runtime-composition/spec.md#Requirement: Product-owned registration boundary` and proved by `cmd/semteams/logging_test.go`, `cmd/semteams/vocab/vocab_test.go`, and `test/contract/predicate_contract_test.go`.
4. **No test-only back doors in production code:**
   - `setupLogger` accepts the narrow `logStreamPublisher` interface satisfied directly by `*natsclient.Client` in `cmd/semteams/main.go` and delegates to `newLogger(os.Stdout, ...)`. No test-only hooks, global mutable variables, or private-state inspection shims were added.
5. **Intentional logger hardening deltas vs. frozen `semstreams/pkg/logging` (zero wire-subject or entry-key break):**
   - `slog.Value.Resolve()` is called in `applySourceAttr` and `buildLogEntry` so `slog.LogValuer` attributes serialize their resolved values rather than raw struct fields.
   - `AddSource` keys on `logLevel == slog.LevelDebug` (`parseLogLevel`) rather than raw `level == "debug"`, so case variants such as `-log-level DEBUG` also include source metadata.
   - Asynchronous `PublishToStream` uses a bounded `5s` `context.WithTimeout` deadline rather than unbounded `context.Background()`.

