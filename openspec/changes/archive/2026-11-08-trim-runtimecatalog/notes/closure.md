# Dependency Closure Delta (`#399`, `trim-runtimecatalog`)

Measured on `darwin/arm64` (`go1.25.0`, default build tags) against frozen `github.com/c360studio/semstreams v1.0.0-beta.162.0.20260930150212-8b99efe9c66a` (`8b99efe9c66a4faa4fa509f9f62cc6bad8392128`), starting from base commit `4028b1e3951e7a1c7b488e0ed9f98049190632e7`.

## Summary Table

| Target & Command | Pre-trim Total | Post-trim Total | Total Delta | Pre-trim `semstreams` | Post-trim `semstreams` | `semstreams` Delta |
| --- | ---: | ---: | ---: | ---: | ---: | ---: |
| `go list -deps ./cmd/semteams` | 578 | 553 | -25 | 98 | 74 | -24 |
| `go list -deps ./internal/runtimecatalog` | 534 | 509 | -25 | 93 | 69 | -24 |
| `go list -deps -test ./internal/runtimecatalog` | 554 | 527 | -27 | 95 | 69 | -26 |

Direct repository AST import paths (`go run ./scripts/measure-semengine-port-ledger.go`):
- Active `github.com/c360studio/semstreams/...` import paths across repository `.go` files: **56 → 35** (**21** Group 4 paths pruned: 18 uncomposed component packages + `componentregistry` + `payloadbuiltins` + `governance`; only `#401` targets `pkg/logging` and `vocabulary/builtins` remain in Group 4).

## Governance Verification (`semstreams/governance`)

- Direct SemTeams imports of `github.com/c360studio/semstreams/governance` are now **0** (dropped from `internal/runtimecatalog/payloads.go` and `internal/runtimecatalog/payloads_test.go`).
- Verified against `configs/rules/**` and `configs/{flow-bootstrap,e2e-flow-bootstrap}.json`: rule action types across all live and parked packs are strictly `add_triple`, `lifecycle_transition`, `publish_agent`, `reconcile_predicates`, `remove_triple`, and `replace_owned` (zero `deny` or `approve` actions, and neither bootstrap configures loop governance).
- `github.com/c360studio/semstreams/governance` still appears transitively in `go list -deps ./cmd/semteams` at frozen `8b99efe9c66a` because `rule-processor` self-wires `newVerdictAuditor(rp)` via `SetVerdictAuditor` whenever `rp.natsClient != nil` (`processor/rule/processor.go:712-718`), causing `executeDeny` / `executeApprove` (`processor/rule/actions.go:18,2109,2123,2183`, `processor/rule/verdict_auditor.go:55-67`) to marshal and publish `governance.verdict.v1` `BaseMessage` envelopes without consulting `payloadregistry`. Dropping `governance` from `RegisterPayloads` is safe because zero live or parked rules use `deny` or `approve` and nothing in SemTeams subscribes to or decodes `governance.verdict.>` / `GOVERNANCE_VERDICT_AUDIT` (a rule hot-loaded via the rule-config KV using `deny`/`approve` would still emit verdict envelopes readable only by external decoders registering `governance`); upstream `C360Studio/semengine#28` removes the verdict hook in `setup-04a-06`.

## 25 Packages Removed from `go list -deps ./cmd/semteams` and `go list -deps ./internal/runtimecatalog`

24 `github.com/c360studio/semstreams/...` packages plus 1 indirect third-party module (`github.com/gorilla/websocket`, also pruned from `go.mod` by `go mod tidy`):

1. `github.com/c360studio/semstreams/gateway/http`
2. `github.com/c360studio/semstreams/gateway/lifecycle-gateway`
3. `github.com/c360studio/semstreams/graph/geo/geojson`
4. `github.com/c360studio/semstreams/input/file`
5. `github.com/c360studio/semstreams/input/udp`
6. `github.com/c360studio/semstreams/input/websocket`
7. `github.com/c360studio/semstreams/output/file`
8. `github.com/c360studio/semstreams/output/httppost`
9. `github.com/c360studio/semstreams/output/websocket`
10. `github.com/c360studio/semstreams/pkg/buffer`
11. `github.com/c360studio/semstreams/pkg/gateddag`
12. `github.com/c360studio/semstreams/pkg/revlag`
13. `github.com/c360studio/semstreams/processor/agentic-governance`
14. `github.com/c360studio/semstreams/processor/agentic-governance/injection_corpus`
15. `github.com/c360studio/semstreams/processor/gated-dag`
16. `github.com/c360studio/semstreams/processor/graph-clustering`
17. `github.com/c360studio/semstreams/processor/graph-embedding`
18. `github.com/c360studio/semstreams/processor/graph-index`
19. `github.com/c360studio/semstreams/processor/graph-index-spatial`
20. `github.com/c360studio/semstreams/processor/graph-index-temporal`
21. `github.com/c360studio/semstreams/processor/json_filter`
22. `github.com/c360studio/semstreams/processor/json_generic`
23. `github.com/c360studio/semstreams/processor/json_map`
24. `github.com/c360studio/semstreams/vocabulary/governance`
25. `github.com/gorilla/websocket`

`go list -deps -test ./internal/runtimecatalog` additionally dropped:
- `github.com/c360studio/semstreams/componentregistry`
- `github.com/c360studio/semstreams/payloadbuiltins`

Zero packages were added to either closure.

## Pre-trim `github.com/c360studio/semstreams/...` Closure (`./cmd/semteams`, 98 packages)

```text
github.com/c360studio/semstreams/agentic
github.com/c360studio/semstreams/agentic/agentrun
github.com/c360studio/semstreams/component
github.com/c360studio/semstreams/component/flowgraph
github.com/c360studio/semstreams/composition
github.com/c360studio/semstreams/config
github.com/c360studio/semstreams/gateway
github.com/c360studio/semstreams/gateway/graph-gateway
github.com/c360studio/semstreams/gateway/http
github.com/c360studio/semstreams/gateway/lifecycle-gateway
github.com/c360studio/semstreams/governance
github.com/c360studio/semstreams/graph
github.com/c360studio/semstreams/graph/clustering
github.com/c360studio/semstreams/graph/embedding
github.com/c360studio/semstreams/graph/geo/geojson
github.com/c360studio/semstreams/graph/inference
github.com/c360studio/semstreams/graph/llm
github.com/c360studio/semstreams/graph/query
github.com/c360studio/semstreams/graph/readiness
github.com/c360studio/semstreams/graph/structural
github.com/c360studio/semstreams/health
github.com/c360studio/semstreams/input/file
github.com/c360studio/semstreams/input/udp
github.com/c360studio/semstreams/input/websocket
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
github.com/c360studio/semstreams/output/file
github.com/c360studio/semstreams/output/httppost
github.com/c360studio/semstreams/output/websocket
github.com/c360studio/semstreams/payloadregistry
github.com/c360studio/semstreams/persona
github.com/c360studio/semstreams/pkg/acme
github.com/c360studio/semstreams/pkg/buffer
github.com/c360studio/semstreams/pkg/cache
github.com/c360studio/semstreams/pkg/dispatch
github.com/c360studio/semstreams/pkg/errs
github.com/c360studio/semstreams/pkg/gateddag
github.com/c360studio/semstreams/pkg/graphview
github.com/c360studio/semstreams/pkg/lifecycle
github.com/c360studio/semstreams/pkg/logging
github.com/c360studio/semstreams/pkg/platform
github.com/c360studio/semstreams/pkg/projection
github.com/c360studio/semstreams/pkg/projection/contract
github.com/c360studio/semstreams/pkg/resource
github.com/c360studio/semstreams/pkg/retry
github.com/c360studio/semstreams/pkg/revlag
github.com/c360studio/semstreams/pkg/rulepack
github.com/c360studio/semstreams/pkg/security
github.com/c360studio/semstreams/pkg/timestamp
github.com/c360studio/semstreams/pkg/tlsutil
github.com/c360studio/semstreams/pkg/types
github.com/c360studio/semstreams/pkg/worker
github.com/c360studio/semstreams/processor/agentic-dispatch
github.com/c360studio/semstreams/processor/agentic-governance
github.com/c360studio/semstreams/processor/agentic-governance/injection_corpus
github.com/c360studio/semstreams/processor/agentic-loop
github.com/c360studio/semstreams/processor/agentic-loop/internal/loopbucket
github.com/c360studio/semstreams/processor/agentic-loop/internal/looprequest
github.com/c360studio/semstreams/processor/agentic-loop/lessonmatch
github.com/c360studio/semstreams/processor/agentic-loop/prompt
github.com/c360studio/semstreams/processor/agentic-model
github.com/c360studio/semstreams/processor/agentic-tools
github.com/c360studio/semstreams/processor/agentic-tools/executors
github.com/c360studio/semstreams/processor/agentic-tools/runner
github.com/c360studio/semstreams/processor/gated-dag
github.com/c360studio/semstreams/processor/graph-clustering
github.com/c360studio/semstreams/processor/graph-embedding
github.com/c360studio/semstreams/processor/graph-index
github.com/c360studio/semstreams/processor/graph-index-spatial
github.com/c360studio/semstreams/processor/graph-index-temporal
github.com/c360studio/semstreams/processor/graph-ingest
github.com/c360studio/semstreams/processor/graph-query
github.com/c360studio/semstreams/processor/json_filter
github.com/c360studio/semstreams/processor/json_generic
github.com/c360studio/semstreams/processor/json_map
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
github.com/c360studio/semstreams/vocabulary/governance
github.com/c360studio/semstreams/vocabulary/rulepacks
```

## Post-trim `github.com/c360studio/semstreams/...` Closure (`./cmd/semteams`, 74 packages)

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

## Non-Destructive Mutation Checks (`8ccab98544ce9e8a0926094ad1d4524b888f7ede`)

5/5 `task mutate:check` (`ateam mutate` via `go test -overlay` against external mutant copies) reported `detection`:

1. `task mutate:check -- -pkg ./internal/runtimecatalog -test TestComponentsMatchComposedCatalog -file internal/runtimecatalog/components.go -mutant <ext>/mutant_components.go -expect-text 'registered factory set mismatch'` (omitting `graph-gateway`, mutant SHA-256 `91b1f1e6e4682976e7bd30b1c42a2082c7d70931a7590ac7d054578f70aafc7f`) → **`detection`**
2. `task mutate:check -- -pkg ./internal/runtimecatalog -test TestComponentsMatchShippedBootstraps -file internal/runtimecatalog/components.go -mutant <ext>/mutant_components.go -expect-text 'references unregistered factory'` → **`detection`**
3. `task mutate:check -- -pkg ./internal/runtimecatalog -test TestPayloadsMatchRetainedOwners -file internal/runtimecatalog/payloads.go -mutant <ext>/mutant_payloads.go -expect-text 'payload metadata/profile/projection-contract drift'` (omitting `graph/inference`, mutant SHA-256 `69f653c23b7073f415237b87b1f8f774dd1f6f9aa67b3521cb738f8e3e7fe62e`) → **`detection`**
4. `task mutate:check -- -pkg ./internal/runtimecatalog -test TestPayloadOwnersRoundTrip -file internal/runtimecatalog/payloads.go -mutant <ext>/mutant_payloads.go -expect-text graph.hierarchy_container.v1` → **`detection`**
5. `task mutate:check -- -pkg ./cmd/openapi-generator -test TestPruneStaleSchemas -file cmd/openapi-generator/main.go -mutant <ext>/mutant_openapi_main.go -expect-text 'stale schema'` (skipping `os.Remove`, mutant SHA-256 `50b09107e6b12fe4bc30f7caaad25ca53ec809299ff587346f743d6980c45900`) → **`detection`**

Cross-provider review (`PR #416`, `issuecomment-6727961568`) also independently verified `go test -count=1 -overlay` `detection` for dropping `graph-gateway`, re-adding uncomposed `udp`, re-adding dropped `governance`, and inverting the `pruneStaleSchemas` keep-check.

