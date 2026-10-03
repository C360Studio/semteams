# Rule-family consumer contribution

[Issue #292](https://github.com/C360Studio/semteams/issues/292) and
[PR #293](https://github.com/C360Studio/semteams/pull/293) contribute bounded consumer evidence to SemEngine
[#26](https://github.com/C360Studio/semengine/issues/26) and [#27](https://github.com/C360Studio/semengine/issues/27).
E1–E3 already have a chosen contract. These fixtures characterize the frozen consumer, not delivery of the future
public family implementation. Production configs, runtime policy and the dependency remain unchanged.

## Source and current corpus

SemTeams foundation: `042193463e3b27fb1996ed4e99bf2b0cd4024a06`. Frozen SemStreams:
`8b99efe9c66a4faa4fa509f9f62cc6bad8392128`. All source links below name those revisions unless explicitly marked SemDev.
Recursively enumerating `configs/rules/**/*.json` at the foundation yields the following source inventory:

| Source group | Rule files | Actions | `publish_agent` | `tool_choice` |
|---|---:|---:|---:|---:|
| Loaded product: research + autoresearch | 30 | 41 | 17 | 15 |
| Loaded support: coordinator + agent-run + ops | 18 | 20 | 1 | 0 |
| Parked: create-change + proof-readiness + dev-from-task + dev-via-test | 36 | 62 | 23 | 20 |
| All checked-in rules | 84 | 123 | 41 | 35 |

The counter examines only each rule [Definition](https://github.com/C360Studio/semstreams/blob/8b99efe9c66a4faa4fa509f9f62cc6bad8392128/processor/rule/rule_factory.go#L16) action slot:
`on_enter`, `on_exit`, `while_true`, `on_recovery` and cron `actions`. It does not recurse into action payloads and count
nested objects as rule actions. The two shipped bootstrap lists—[production](https://github.com/C360Studio/semteams/blob/042193463e3b27fb1996ed4e99bf2b0cd4024a06/configs/flow-bootstrap.json#L345)
and [mock](https://github.com/C360Studio/semteams/blob/042193463e3b27fb1996ed4e99bf2b0cd4024a06/configs/e2e-flow-bootstrap.json#L314)—name the same 48 rule paths, totaling 61 actions and 18
`publish_agent` actions. Loaded-list membership and source counts do not prove that every action executes.

All 35 configured tool choices are `required`: research 5, autoresearch 10, parked create-change 2, dev-from-task 1 and
dev-via-test 17. Configured forced-function and response-format counts are both zero. The historical SemEngine SETUP03B
inventory reported 119 actions, 40 `publish_agent` and 31 tool-choice cases; those are not current foundation counts.
Neither inventory is a claim of full-corpus behavioral qualification by these fixtures.

## Fixture and execution boundary

[Tests](../../../test/contract/rule_family_consumer_test.go) exercise the frozen public boundary with three portable cases.
[expected.json](../../../test/fixtures/rule-family-consumer/expected.json) contains literal expectations independent of the
production decoder/executor. Test-local primitive expectation structs are separate from the public agentic types;
[provenance.json](../../../test/fixtures/rule-family-consumer/provenance.json) records pins,
source hash, retained/omitted fields and limits.

| Fixture | Independent expected values | Classification |
|---|---|---|
| [required-live.json](../../../test/fixtures/rule-family-consumer/required-live.json) | `tool_choice.mode=required`, no function name, response format absent; subject `agent.task.researcher-research-plan`, role `researcher-research-plan`, model `research` | Selected live research action projection with a test-only rule envelope |
| [forced-function.json](../../../test/fixtures/rule-family-consumer/forced-function.json) | `mode=function`, `function_name=decide`, response format absent; subject `agent.task.fixture-function`, role `fixture-function`, model `fixture-model` | Supplemental supported-contract case |
| [response-format.json](../../../test/fixtures/rule-family-consumer/response-format.json) | Choice absent; format `type=json_schema`, `name=decision_with_evidence`, `strict=true`; subject `agent.task.fixture-format`, role `fixture-format`, model `fixture-model` | Supplemental supported-contract case |

The response schema independently requires `decision` and `evidence`, rejects additional top-level properties, and
constrains decision to `proceed` or `hold`. Evidence is an array of objects with required `entity_id:string` and
`accepted:boolean`, also rejecting additional properties. These nested values are part of the oracle.

The live projection retains `type`, `subject`, `role`, `model`, `prompt`, `tools`, `tool_choice` and `action_allowlist`;
it omits exactly `run_scope` and `properties`. The fixture envelope is test-only. The supplemental cases do not invent
a current SemTeams production dependency. Tool registry/name resolution and E3 run behavior are outside this proof.

The relevant source path is [rule.Definition](https://github.com/C360Studio/semstreams/blob/8b99efe9c66a4faa4fa509f9f62cc6bad8392128/processor/rule/rule_factory.go#L16) / `[]Action`, with cold JSON
decoding in [rule loading](https://github.com/C360Studio/semstreams/blob/8b99efe9c66a4faa4fa509f9f62cc6bad8392128/processor/rule/rule_loader.go#L72) and hot definition mapping in
[definitionFromMap](https://github.com/C360Studio/semstreams/blob/8b99efe9c66a4faa4fa509f9f62cc6bad8392128/processor/rule/config_validation.go#L449). The test directly uses `encoding/json.Unmarshal`
into `rule.Definition`, followed by a marshal/unmarshal round trip. Neither the cold file loader nor hot configuration
service is executed by these tests. The frozen action fields are
[ResponseFormat and ToolChoice](https://github.com/C360Studio/semstreams/blob/8b99efe9c66a4faa4fa509f9f62cc6bad8392128/processor/rule/actions.go#L191).

Public execution follows [NewActionExecutorFull / Execute](https://github.com/C360Studio/semstreams/blob/8b99efe9c66a4faa4fa509f9f62cc6bad8392128/processor/rule/actions.go#L843) →
[executePublishAgent](https://github.com/C360Studio/semstreams/blob/8b99efe9c66a4faa4fa509f9f62cc6bad8392128/processor/rule/actions.go#L1683) → `publishAgentOnce` →
[stampPerSpawnLLMKnobs](https://github.com/C360Studio/semstreams/blob/8b99efe9c66a4faa4fa509f9f62cc6bad8392128/processor/rule/actions.go#L1601) →
[BaseMessage publication](https://github.com/C360Studio/semstreams/blob/8b99efe9c66a4faa4fa509f9f62cc6bad8392128/processor/rule/actions.go#L2023). Decoder assertions must independently name choice
mode/function name and response-format type/name/strict/nested schema content. Executor evidence must separately check
those fields on a concrete canonical `*agentic.TaskMessage` at the captured public output boundary. The real executor
uses an in-memory publisher, a canonical non-loop trigger entity and no graph/lifecycle dependencies. The emitted
bytes are decoded through `runtimecatalog.RegisterPayloads` and `message.NewDecoder`, followed by `TaskMessage.Validate`.
The assertion of one captured message is per synchronous invocation; it proves neither NATS delivery nor exactly-once
behavior. Rule conditions, tool resolution, E3 run inheritance, loop consumption, provider forwarding and the future
core/family implementation remain outside this result.

## Held E3 run anchors

These original production rules are the retained fixtures; no additional copies or execution are implied.

| Original rule | JSON pointer and value | Frozen interpretation |
|---|---|---|
| [Research spawn](https://github.com/C360Studio/semteams/blob/042193463e3b27fb1996ed4e99bf2b0cd4024a06/configs/rules/research/01-coordinator-research-spawn.json#L28) | `/on_enter/0/run_scope` = `new` | Starts a new run |
| [Plan to gather](https://github.com/C360Studio/semteams/blob/042193463e3b27fb1996ed4e99bf2b0cd4024a06/configs/rules/research/02-plan-to-gather.json) | `/on_enter/0` omits `run_scope` | Default inheritance path |
| [Terminal observer](https://github.com/C360Studio/semteams/blob/042193463e3b27fb1996ed4e99bf2b0cd4024a06/configs/rules/ops/01-run-terminal-observe.json#L29) | `/on_enter/0/run_scope` = `none` | Observer does not join the observed run |

The field/predicate map is source inventory only; none of these held inheritance/fallback behaviors is newly executed
by the three fixture tests.

| Field or predicate | Writer | Reader |
|---|---|---|
| `TaskMessage.RunID` | [publishAgentOnce](https://github.com/C360Studio/semstreams/blob/8b99efe9c66a4faa4fa509f9f62cc6bad8392128/processor/rule/actions.go#L1826) selects new/inherited/no run. | [configureLoopMetadata](https://github.com/C360Studio/semstreams/blob/8b99efe9c66a4faa4fa509f9f62cc6bad8392128/processor/agentic-loop/handlers.go#L537) passes it to `LoopManager.SetRunID`. |
| Graph `agent.loop.run` (`LoopRun`) | [stampRunAnchors](https://github.com/C360Studio/semstreams/blob/8b99efe9c66a4faa4fa509f9f62cc6bad8392128/processor/rule/actions.go#L731) stamps the owned firing loop; [LoopExecutionEntity.Triples](https://github.com/C360Studio/semstreams/blob/8b99efe9c66a4faa4fa509f9f62cc6bad8392128/agentic/loop_execution_entity.go#L133) emits spawned-loop anchors through [WriteSpawnIdentity](https://github.com/C360Studio/semstreams/blob/8b99efe9c66a4faa4fa509f9f62cc6bad8392128/processor/agentic-loop/graph_writer.go#L447). | [publishAgentOnce](https://github.com/C360Studio/semstreams/blob/8b99efe9c66a4faa4fa509f9f62cc6bad8392128/processor/rule/actions.go#L1884) reads inheritance; [NATSLoopTripleReader.GetLoopRunID](https://github.com/C360Studio/semstreams/blob/8b99efe9c66a4faa4fa509f9f62cc6bad8392128/agentic/agentrun/nats_reader.go#L37) serves [ResolveRun](https://github.com/C360Studio/semstreams/blob/8b99efe9c66a4faa4fa509f9f62cc6bad8392128/agentic/agentrun/agentrun.go#L395). |
| Graph `agent.run.entity-id` (`LoopRunEntityID`) | The same `stampRunAnchors` and spawned-loop `Triples`/`WriteSpawnIdentity` paths record the resolved entity ID. | [DecisionHandler.HandleDecision](https://github.com/C360Studio/semteams/blob/042193463e3b27fb1996ed4e99bf2b0cd4024a06/cmd/semteams/chainpause/decision_handler.go#L165) and [approvalpause.resolveRunAnchor](https://github.com/C360Studio/semteams/blob/042193463e3b27fb1996ed4e99bf2b0cd4024a06/cmd/semteams/approvalpause/pauser.go#L160) read the graph predicate. |
| Tool metadata `agent.run_id` / `agent.run_entity_id` | [dispatchToolCall](https://github.com/C360Studio/semstreams/blob/8b99efe9c66a4faa4fa509f9f62cc6bad8392128/processor/agentic-loop/handlers.go#L2074) writes the keys defined by [MetadataKeyRunID / MetadataKeyRunEntityID](https://github.com/C360Studio/semstreams/blob/8b99efe9c66a4faa4fa509f9f62cc6bad8392128/agentic/tools.go#L388). | [runanchor.Anchor](https://github.com/C360Studio/semteams/blob/042193463e3b27fb1996ed4e99bf2b0cd4024a06/cmd/semteams/runanchor/runanchor.go#L33) reads metadata; [ChainEntityID](https://github.com/C360Studio/semteams/blob/042193463e3b27fb1996ed4e99bf2b0cd4024a06/cmd/semteams/runanchor/runanchor.go#L76) uses that resolved anchor after the explicit related-loop override. |
| `LoopFailedEvent.RunEntityID` | [buildFailureEvent](https://github.com/C360Studio/semstreams/blob/8b99efe9c66a4faa4fa509f9f62cc6bad8392128/processor/agentic-loop/handlers.go#L3381) resolves and writes the event field. | [chainpause.Pauser.HandleFailed](https://github.com/C360Studio/semteams/blob/042193463e3b27fb1996ed4e99bf2b0cd4024a06/cmd/semteams/chainpause/pauser.go#L65) reads it; [MilestoneSubscriber.resolveRunForEvent](https://github.com/C360Studio/semstreams/blob/8b99efe9c66a4faa4fa509f9f62cc6bad8392128/agentic/agentrun/agentrun.go#L778) uses the event anchor before `ResolveRun` fallback. |
| `related_loops` and `agent.lineage.<key>` | [stampRelatedLoops](https://github.com/C360Studio/semstreams/blob/8b99efe9c66a4faa4fa509f9f62cc6bad8392128/processor/rule/actions.go#L1488) carries explicit role-keyed links; [WriteLineageTriples / buildLineageTriples](https://github.com/C360Studio/semstreams/blob/8b99efe9c66a4faa4fa509f9f62cc6bad8392128/processor/agentic-loop/graph_writer.go#L347) produces lineage predicates. | `approvalpause.resolveRunAnchor` falls back to `agent.lineage.run-loop-entity-id`; `runanchor.ChainEntityID` first reads `related_loops['chain-entity-id']`. These are distinct keys and lookup paths. |

The underscore metadata spellings are distinct from graph predicates. There is no frozen `ChainID` Go identifier in
this map; `ContextRequestID` is request/context correlation, not a run anchor. Existing
[agent-run contracts](https://github.com/C360Studio/semteams/blob/042193463e3b27fb1996ed4e99bf2b0cd4024a06/test/contract/agent_run_pack_test.go#L115) and [autoresearch reply integration tests](https://github.com/C360Studio/semteams/blob/042193463e3b27fb1996ed4e99bf2b0cd4024a06/test/contract/autoresearch_reply_integration_test.go#L197)
concern the current runtime and do not qualify future E3 extraction.

## Retry ownership and exact readers

| Example | Reading/executing symbols | Classification and limit |
|---|---|---|
| [Research reviewer retry](https://github.com/C360Studio/semteams/blob/042193463e3b27fb1996ed4e99bf2b0cd4024a06/configs/rules/research/05-reviewer-rejected-retry.json) | `/on_enter/0/type=publish_agent`, `/on_enter/0/properties/retry=true`; `/max_iterations=3` maps to `Definition.MaxIterations` | Pack workflow re-entry; the top-level value is available to explicit guards, but this rule has no `When`/`$state` guard. It is not an enforced rule-entry or chain-wide budget, nor a `retry` action; the separate per-action cap can coexist. |
| Human-requested retry | [DecisionHandler.retry](https://github.com/C360Studio/semteams/blob/042193463e3b27fb1996ed4e99bf2b0cd4024a06/cmd/semteams/chainpause/decision_handler.go#L200) → `NATSTaskPublisher.PublishTask` (line 339) | Product policy emits a fresh task; not substrate operation retry. |
| Merge-style `update_kv` action | [executeUpdateKV](https://github.com/C360Studio/semstreams/blob/8b99efe9c66a4faa4fa509f9f62cc6bad8392128/processor/rule/actions.go#L2276) → [KVWriter.UpdateJSON](https://github.com/C360Studio/semstreams/blob/8b99efe9c66a4faa4fa509f9f62cc6bad8392128/processor/rule/kv_writer.go#L112) → [KVStore.UpdateJSON / UpdateWithRetryRev](https://github.com/C360Studio/semstreams/blob/8b99efe9c66a4faa4fa509f9f62cc6bad8392128/natsclient/kv.go#L428) → [retry.Do](https://github.com/C360Studio/semstreams/blob/8b99efe9c66a4faa4fa509f9f62cc6bad8392128/pkg/retry/retry.go#L67) | Real core operation retry on CAS conflicts. `KVOptions/getRetryConfig` translates additional retries to total attempts with delay/backoff/jitter. No rule action retry field is needed. |
| Agent request retry after compaction | [MessageHandler.emitRetryRequest](https://github.com/C360Studio/semstreams/blob/8b99efe9c66a4faa4fa509f9f62cc6bad8392128/processor/agentic-loop/handlers.go#L2322) | Agent-loop behavior emits a fresh `AgentRequest` without incrementing the iteration. |
| Provider retry | [Client.runRetryLoop](https://github.com/C360Studio/semstreams/blob/8b99efe9c66a4faa4fa509f9f62cc6bad8392128/processor/agentic-model/client.go#L498) | Agent-model transient/rate-limit policy, distinct from pack convergence and core mutation retries. |
| SemDev donor retry route | [06c-route-retry.json](https://github.com/C360Studio/semdev/blob/7a22bc7422c978992977be6e168bca8c3dba138b/configs/rules/dev-from-task/06c-route-retry.json#L11) reads `route.task.budget` with `length_lt`, records the routed marker/`task.attempt.instance`, then publishes a new developer task | Pack development-attempt/convergence budget. This donor rule has neither `max_iterations` nor `loop_max_iterations`; it is not core operation retry. |

Three max-iteration settings must remain distinct: [Definition.MaxIterations](https://github.com/C360Studio/semstreams/blob/8b99efe9c66a4faa4fa509f9f62cc6bad8392128/processor/rule/rule_factory.go#L39) is copied into `MatchState`/`stateFields` by
[stateful evaluation](https://github.com/C360Studio/semstreams/blob/8b99efe9c66a4faa4fa509f9f62cc6bad8392128/processor/rule/stateful_evaluator.go#L205) and exposed through
[`ExecutionContext.SubstituteVariables`](https://github.com/C360Studio/semstreams/blob/8b99efe9c66a4faa4fa509f9f62cc6bad8392128/processor/rule/execution_context.go#L303) /
[typed substitution](https://github.com/C360Studio/semstreams/blob/8b99efe9c66a4faa4fa509f9f62cc6bad8392128/processor/rule/typed_substitution.go#L160) for explicit guards; it does not itself stop execution.
 [Action.MaxIterations](https://github.com/C360Studio/semstreams/blob/8b99efe9c66a4faa4fa509f9f62cc6bad8392128/processor/rule/actions.go#L323) limits action firings per rule/entity match cycle
(default 3, zero unlimited), read by [the stateful evaluator](https://github.com/C360Studio/semstreams/blob/8b99efe9c66a4faa4fa509f9f62cc6bad8392128/processor/rule/stateful_evaluator.go#L398);
`Action.LoopMaxIterations` bounds a spawned loop through
[stampLoopMaxIterations](https://github.com/C360Studio/semstreams/blob/8b99efe9c66a4faa4fa509f9f62cc6bad8392128/processor/rule/actions.go#L1628). The donor's task budget is another domain policy.
There is no frozen `ActionRetry`/`type: retry`, but that does not mean core retry is absent. `Processor.waitForStream`
separately retries startup lookups; generic `errs.RetryConfig` helpers are not directly called by the rule processor.
No retry policy is moved or exercised as a new behavior in this slice.

## Validation and limits

Focused commands passed, including the race run:

```sh
go test ./test/contract -run '^TestRuleFamily' -count=1 -v
go test -race ./test/contract -run '^TestRuleFamily' -count=1 -v
```

- `TestRuleFamilyDecode`: three direct decode cases and three Definition JSON round trips.
- `TestRuleFamilyEmission`: three real public-executor captured emissions, decoded as canonical task payloads.
- `TestRuleFamilyOracleRejectsDrift`: six valid-input changes rejected at both decode and emission boundaries—choice
  omission, required-mode drift, function-name drift, format omission, strict-flag drift and nested `accepted.type` drift.
- `TestRuleFamilyLiveSourceProjection`: pinned source hash and exact retained/omitted field projection.

Local validation passed: `task lint` (six warnings in unchanged files, zero errors), `task test:race`, `task test:integration`,
`go build ./...`, `task schema:generate` and `task schema:check-changes` (generated outputs unchanged), strict OpenSpec
validation of all five items, and the 26 OpenSpec queue fixture tests. These are local checks, not hosted CI evidence.

The controlled golden mutation is reproducible from the tracked fixtures:

1. Save the exact bytes of `test/fixtures/rule-family-consumer/expected.json`; change only
   `forced-function.json.tool_choice.function_name` from `decide` to `different_expected_function`.
2. Run `go test ./test/contract -run '^TestRuleFamily(Decode|Emission)$' -count=1 -v`.
   The recorded run exited 1, failing both `TestRuleFamilyDecode/forced-function.json` and
   `TestRuleFamilyEmission/forced-function.json` on the changed expectation.
3. Restore the saved bytes and run the same command. The recorded run exited 0; restored SHA-256 was
   `9b9e71eb9ba637a82414d70e46b8c86a7d7014eeab31a0a826556261dc8887fe`.

Local red/restored logs and the result record were kept at `/tmp/semteams-rule-family-mutation-{red,restored}.log` and
`/tmp/semteams-rule-family-mutation-result.json`. These temporary files are not committed artifacts; the tracked fixture,
source provenance and command above are the reproduction inputs. Independent Go/source review passed after the
primitive-oracle and source-claim corrections. The reviewer separately reran the focused race tests successfully and
checked test SHA-256 `d799edc0adf0e78044d7a469d13041beb962e45ca19b9112418782f3f44f5829` and the restored golden hash.
That review confirms the bounded test/source evidence, not broker delivery or future family behavior. Independent
final-content review of the archive and synchronized specification passed, with reviewed hashes unchanged.

No production activation, runtime extraction, engine API, paid model calls, full historical or current corpus
qualification, or future E1–E3 implementation qualification is supplied. Upstream comments require the reviewed final
commit SHA and exact paths; publication is PR/issue progress, not an OpenSpec pre-commit completion claim. Merge and
issue closure remain outside this contribution.
