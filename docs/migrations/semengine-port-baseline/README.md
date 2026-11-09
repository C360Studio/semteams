# Post-`v0.3.0` SemEngine Port Acceptance Floor (`#403`)

This directory records the reproducible **"before" acceptance floor** measured on unmodified `main` at commit `e6cd847005833733a53343741a30248c45b25acf` (2026-11-09, closing Milestone 1 `semengine-port 1: ready before the pin` under Epic `#412`). Milestones 3 (`#408`) and 4 grade the SemEngine port against this live post-`v0.3.0` baseline—alongside [`baseline.json`](baseline.json)—rather than the pre-`v0.2.0` October 1 SemStreams upgrade snapshot ([`../semstreams-8b99efe/`](../semstreams-8b99efe/)).

 Since `#280` landed (`edf87d31`), `main` added `/work` (`#296`, `#314`, `#315`, `#319`, `#320`, `#323`, `#324`), scheduled/incremental Program Pulse (`#268`), research containment budgets (`#272`), per-loop cost accounting, `ateam` governance gates (`#362`, `#375`, `#413`), runtime boundary suites (`#289`, `#293`, `#295`), and Milestone 1 substrate decoupling (`#398`–`#402`: 9-factory `internal/runtimecatalog` trim, `internal/natstest` `2.15.0` digest fixture, `pkg/logging` + `vocabulary/builtins` removal, and the 21-unit source-to-destination manifest).

---

## 1. Pinned Runtime and Broker Identities

| Subject | Exact value | Verification source |
| --- | --- | --- |
| SemTeams `main` SHA | `e6cd847005833733a53343741a30248c45b25acf` | Clean worktree (`source_changed: false`, `source_tree_sha256: 7ef9b5ed...`) |
| Frozen SemStreams module | `github.com/c360studio/semstreams v1.0.0-beta.162.0.20260930150212-8b99efe9c66a` | `go.mod` (`sha256: 1ce49dd7...`), commit `8b99efe9c66a4faa4fa509f9f62cc6bad8392128`, sum `h1:WAlGLMjE/3EnYyeoqounCfYnVyIB3igMlG+FLP1Iq+o=`, `replace: none` |
| Go integration NATS broker (`internal/natstest`) | `nats-server: v2.15.0` | `internal/natstest/nats-image`: `nats:2.15.0-alpine@sha256:ac8f88a6494bffc2c2a5289a0ca61cb28a9145c11ba5677cf24265d07f46d8d4` |
| Playwright E2E NATS broker (`ui/docker-compose.agentic-e2e.yml`) | `nats-server: v2.14.4` | `nats:2.14.4-alpine` (`nats@sha256:f2123f533c2b0cada0a5c5ec434fb2b8cfe1cf220215ef9d7517e1372917ad66`) |
| Built E2E stack image IDs (`e6cd8470`) | `backend: sha256:d4f77fa6...` · `mock-llm: sha256:a1486aef...` · `sandbox: sha256:f46301b7...` · `ui: sha256:2bd87583...` · `caddy: sha256:78bd3502...` | Recorded per-run in `qualify-migration-browser.py` manifests (`baseline.json` `e2e_stack_image_digests`) |

> **Milestone 2/3 evolution & immutability note (`#404`, `#406`, `#408`):** The measurements recorded in `baseline.json` at `e6cd847005833733a53343741a30248c45b25acf` are immutable historical floor values and MUST NOT be overwritten when downstream port issues land. Specifically: (`#404`) once `semstreams` leaves `go.mod`, `TestSemEnginePortBaseline` skips the live `go list -m semstreams` check; (`#406`) when the trimmed SemTeams `graph-gateway` drops `graph_index_queries` (reducing warnings from `9` to `8`) and (`#408`) when `ui/docker-compose.agentic-e2e.yml` aligns its NATS broker pin to `2.15.0`, update `TestSemEnginePortBaseline` to enforce non-regression (subset of recorded warnings, `errors == 0`) or add additive `superseded_by` metadata alongside `docs/migrations/semengine-qualification/` rather than editing `e6cd8470` counts or pins in place.

---

## 2. Repository CI Parity, Integration, and Runtime Boundary Floor

All local Go, UI, and Governance gates pass with **zero failures** on `e6cd847005833733a53343741a30248c45b25acf`:

| Gate | Command | Result | Exact counts / timings |
| --- | --- | --- | --- |
| Full CI parity | `task validate` | **PASS** | `go: 149s`, `ui: 45s`, `governance: 6s`; `gofmt`: `258` Go files; `svelte-check`: `0` errors / `0` warnings; `test:browser-runner`: `1` passed; `openspec validate --all --strict`: `11/11` passed; `publish:verify-test`: `39` passed / `0` failed |
| Go unit & contract (`-race`) | `scripts/gopkgs.sh go test -race -count=1 -json ./...` | **PASS** | `40/42` packages with tests (`2` without tests): **756** top-level passed, **2** skipped, **0** failed; **750** subtests passed, **26** skipped, **0** failed |
| Go integration (`-p 1`) | `TESTCONTAINERS_RYUK_DISABLED=true go test -race -count=1 -tags=integration -p 1 -json ./...` | **PASS** | `41/43` packages with tests: **777** top-level passed, **4** skipped (`2` `cmd/openapi-generator` + `2` paid-LLM `test/integration`), **0** failed; **824** subtests passed, **26** skipped, **0** failed |
| UI Vitest | `npm --prefix ui run test:unit -- --run` | **PASS** | **201** test files passed + **1** skipped (`202` total); **1,330** suites passed; **4,197** tests passed + **12** skipped (`4,209` total), **0** failed |

### Post-`v0.2.0` Runtime Boundary Suites Verified Inside the Floor

- **`#289` Model retry boundary (`cmd/semteams`):** `TestConfiguredModelRetryBoundary/{production,mock}` (`1.14s` `-tags=integration`) — PASS.
- **`#293` Rule-family consumer contract (`test/contract`):** `TestRuleFamilyDecode/{forced-function.json,response-format.json,required-live.json}`, `TestRuleFamilyEmission/{required-live.json,forced-function.json,response-format.json}`, `TestRuleFamilyOracleRejectsDrift` (6 drift cases), and `TestRuleFamilyLiveSourceProjection` — PASS.
- **`#295` / `#272` Loop first-request, cost stamping, and research budget (`cmd/semteams`):** `TestConfiguredLoopFirstRequestBoundary` (`flow-bootstrap.json` + `e2e-flow-bootstrap.json` x 4 fixtures, `1.03s`), `TestLoopCostStampingAcrossBootstraps` (`7` capability/direct cases, `1.10s`), `TestStatefulToolsUseDeclaredLoopAuthorityBucket`, `TestInfrastructureUsesMintedPlatformForRunAndLoopIDs` (`0.30s`), and `TestResearchLoopBudgetTerminalBoundary/{exhaust=false,exhaust=true}` (`0.48s`) — PASS.
- **`#268` Scheduled Program Pulse containment (`cmd/semteams` + `test/contract`):** `TestScheduledCronInflightAndCooldownContainment` (`3.85s`), `TestScheduledCronPatternBKillSwitch` (`2.28s`), and `test/contract/program_pulse_contract_test.go` — PASS.

---

## 3. Startup Composition Warning Ledger and Cold-Start `/loops` `503` Behavior

### 3.1 Nine-Warning Composition Ledger (Post-`#399` 9-Factory Catalog Cut)

Even after `#399` trimmed `internal/runtimecatalog` from 23 factories to the 9 composed factories, both offline `composition.Validate` (`TestShippedBootstrapCompositions` across `configs/flow-bootstrap.json` and `configs/e2e-flow-bootstrap.json`) and live `ComponentManager.analyzeBootComposition` emit **`status: "warnings"`, `0` errors, and the exact same `9` `warning` findings**:

| # | Type | Component | Port | Message | Milestone 2/3 disposition |
| ---: | --- | --- | --- | --- | --- |
| 1 | `disconnected_node` | `objectstore` | *(none)* | `component has no connections` | Retained (`emit_artifact` writes via direct Go helper, not stream ports) |
| 2 | `orphaned_port` | `agentic-model` | `agent.stream` | `output port 'agent.stream' (stream): no_subscribers` | Retained unless streaming token consumer is wired |
| 3 | `orphaned_port` | `agentic-tools` | `tool.list` | `input port 'tool.list' (request): optional_api_unused` | Retained optional discovery port |
| 4 | `orphaned_port` | `graph-gateway` | `graph_index_queries` | `output port 'graph_index_queries' (request): optional_api_unused` | **Expected to drop in `#406`** when trimmed SemTeams `graph-gateway` removes `graph_index_queries` |
| 5 | `orphaned_port` | `objectstore` | `events` | `output port 'events' (stream): no_subscribers` | Retained optional storage output |
| 6 | `orphaned_port` | `objectstore` | `stored` | `output port 'stored' (stream): no_subscribers` | Retained optional storage output |
| 7 | `orphaned_port` | `objectstore` | `write` | `input port 'write' (stream): no_publishers` | Retained optional storage input |
| 8 | `orphaned_port` | `teams-dispatch` | `user.response` | `output port 'user.response' (stream): no_subscribers` | Retained (typed replies consumed externally over JetStream/SSE) |
| 9 | `orphaned_port` | `teams-loop` | `trajectories` | `output port 'trajectories' (watch): optional_index_unwatched` | Retained optional trajectory watch index |

`TestSemEnginePortBaseline` (`test/contract/semengine_port_baseline_test.go`) enforces exact parity between this 9-finding list and live `composition.Validate` across both shipped bootstrap configs.

### 3.2 Cold-Start `GET /teams-dispatch/loops` `503 -> 200` Readiness Window

Probing a cold Docker boot of `ui/docker-compose.agentic-e2e.yml` on `e6cd8470` confirmed that fail-closed loop projection readiness remains unchanged from `#280`:

1. **`715ms`–`794ms`:** Caddy reverse proxy returns HTTP `502` while `semteams` finishes HTTP listener bind.
2. **`822ms`–`895ms`:** `GET /teams-dispatch/loops` returns HTTP **`503`** (`NOT READY`, then `{"response_id":"...","type":"error","content":"loop state is not available right now; retry shortly"}`) **at the exact same moment `GET http://localhost:8080/readyz` inside `backend` already returns HTTP `200 READY`**, accompanied by backend `ERROR` log:
   `agentic-dispatch: loop listing refused while loop authority is unavailable` — `Component.currentLoopSnapshot: loop projection not caught up failed: graphview: view not ready`.
3. **`928ms`:** `GET /teams-dispatch/loops` transitions cleanly to HTTP **`200`** (`[]`) once `graphview` catches up.

---

## 4. Mock-LLM Playwright E2E and Boundary Matrix Floor

Paid real-LLM `smoke*` journeys were excluded per owner ruling. Because `Repository CI` (`scripts/validate.sh`) does not run Docker-backed Playwright stacks, executing the full mock-LLM inventory on unmodified `e6cd8470` separated **qualified passing product journeys** from **four pre-existing post-`v0.2.0` E2E harness/spec/fixture defects** (filed as [#420](https://github.com/C360Studio/semteams/issues/420) and deliberately left unedited here so `#403` records `main` as-is).

### 4.1 Summary by Execution Surface

| Surface | Runner | Total | Passed | Skipped | Failed | Notes |
| --- | --- | ---: | ---: | ---: | ---: | --- |
| `task ui:test:e2e` chain (as-shipped `stack:up`) | `ui/Taskfile.yml` | 23 subtasks | **9** (`13` tests) | **5** | **9** | **4** of the `9` failures are pure `stack:up` cold-start `/loops` `503` races that **PASS (`4/4`)** under wait-gated `qualify-migration-browser.py` (`#420`) |
| Wait-gated recheck of API-first subtasks | `scripts/qualify-migration-browser.py` (`up -d --wait`) | 4 scenarios | **4** | 0 | 0 | `coordinator-routing-matrix`, `coordinator-readiness-gate`, `coordinator-team-spawn`, `ops-agent` (`source_changed: false`) |
| Opt-in `autoresearch` journeys | `task ui:test:e2e:agentic:autoresearch{,-guardrails}` & `qualify-migration-browser.py` | 2 scenarios | **2** | 0 | 0 | Approved optimization (`16.1s`) + guardrail rejection (`16.0s`) both PASS |
| Approval & autoresearch boundary matrix | `scripts/qualify-migration-browser.py` (`scripts/semstreams-migration-matrix.json`) | 7 scenarios | **7** | 0 | 0 | `run-approval-boundary`, `approval-boundary-{approve,reject,cancel}`, `autoresearch-{descended-clarification,propose-failed,descended-status}` (`source_changed: false`) |
| Parked / un-repointed donor scaffolds | Isolated compose probe | 2 specs | 0 | 0 | 2 | `tool-approval-gate.spec.ts`, `clarification-resume-ui.spec.ts` (documented donor/ADR-058 exclusions, not wired in `task ui:test:e2e`) |

### 4.2 Qualified Passing Product Capabilities on `e6cd8470` (`22` Active Scenarios Passing)

 Combining `stack:up` passes (`9` subtasks / `13` Playwright tests), wait-gated API-first subtasks (`4` scenarios), opt-in `autoresearch` (`2` scenarios), and boundary matrix scenarios (`7` scenarios) qualifies **22 active scenarios** (`26` Playwright test cases) end to end on `e6cd8470`:

- **Coordinator intake, readiness, routing, and autonomous clarification:** `coordinator-routing-matrix` (wait-gated), `coordinator-readiness-gate` (wait-gated), `coordinator-team-spawn` (wait-gated), `clarification-autonomous`.
- **Research fan-out, sandbox execution, failure & drill-in, and ops terminal diagnosis:** `research-fanout`, `sandbox-mvp`, `real-time-activity-stream`, `chain-drill-in`, `run-failed`, `ops-agent` (`ops-run-terminal.spec.ts`, wait-gated).
- **Post-`v0.2.0` UI surfaces (`/pulse`, `/work`, 9-factory admin inventory):** `action-chips` (`action-chips-personas.spec.ts` on `/pulse`), `work-board` (`work-board.spec.ts`, 4/4 tests on `/work`), `admin-flows-inventory` (2/2 tests against the 9-factory catalog).
- **Run-scoped and loop-scoped tool approval gates:** `run-approval-boundary` (`emit_plan` gate -> `awaiting_approval` -> approve -> typed research completion) and `approval-boundary-{approve,reject,cancel}` (`request_sandbox` gate).
- **Autoresearch full 5-case matrix (`#280` parity preserved):** `autoresearch`, `autoresearch-guardrails`, `autoresearch-descended-clarification`, `autoresearch-propose-failed`, `autoresearch-descended-status`.

### 4.3 Pre-existing `main` E2E Harness/Spec/Fixture Defects Tracked in `#420` (Do Not Blame on SemEngine)

1. **`stack:up` cold-start `/teams-dispatch/loops` `503` race (`coordinator-routing-matrix`, `coordinator-readiness-gate`, `coordinator-team-spawn`, `ops-agent`):** `ui/Taskfile.yml` `stack:up` polls `/health` and `/teams-dispatch/commands` rather than `/teams-dispatch/loops`, so API-first specs hit the §3.2 `503` window; all 4 pass (`exit=0`) when started with `docker compose up -d --wait`.
2. **`research-mvp.spec.ts:348` path regex mismatch (PR `#328`, `22ea2530`):** Full 6-loop `research-mvp` completes cleanly and writes `/artifacts/research/<date>-mqtt-vs-nats-for-iot-edge.md` (`SEMTEAMS_RESEARCH_ARTIFACT_DIR=/artifacts/research`), but `22ea2530` added `/\.artifacts\/research\/.+\.md$/` expecting a leading dot.
3. **`task-story-trace.spec.ts:64` & `task-title-aliases.spec.ts:68` child-UUID selection:** Both specs pick `(await listLoops(request)).find((l) => !initialIds.has(l.loop_id))` without filtering `!l.parent_loop_id`; because `/teams-dispatch/loops` sorts lexicographically by UUIDv4 across all 7 spawned loops, any child loop with a smaller UUID than the root coordinator causes the top-level `[data-testid="task-card"]` locator to time out after all 7 backend loops reach `state: "complete"`.
4. **`approval-pause` & `approval-resume` unadvertised `create_rule` on `research/06` (PR `#330`):** Both fixtures target `configs/rules/research/06-needs-clarification-replan.json:38`, whose recovery coordinator advertises `tools: ["read_loop_result", "bash", "decide"]` (omitting `create_rule`). Frozen SemStreams `8b99efe9c66a` (`processor/agentic-tools/component.go:748-766`: `admitToolCall` per-loop advertised-set check runs before `:765` `approvalFilter.FilterToolCalls`) rejects unadvertised tool calls before `ApprovalFilter`, so Loop R exhausts `max_iterations: 8` (`runPhases: ["failed"]`). Runtime approval gating itself passes `4/4` via `run-approval-boundary` and `approval-boundary-{approve,reject,cancel}`.

### 4.4 Explicit Skips (`5`) and Parked Donor Scaffolds (`2`)

- **Explicit skips in `task ui:test:e2e` (`5`):** `artifact-context-handoff` (`#261` evidence-body dereference held), `run-failed-coordinator`, `run-failed-coordinator-inherit`, `ask-user-pause`, `clarification-resume` (`ADR-058` parked `dev_via_test` pack).
- **Unwired donor scaffolds (`2`):** `tool-approval-gate.spec.ts` (front-door unadvertised `create_rule`) and `clarification-resume-ui.spec.ts` (parked `dev_via_test` category) remain excluded from `task ui:test:e2e`.
