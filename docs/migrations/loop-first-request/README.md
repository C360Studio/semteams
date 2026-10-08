# Loop first-request qualification

[Issue #294](https://github.com/C360Studio/semteams/issues/294) and
[PR #295](https://github.com/C360Studio/semteams/pull/295) qualify the boundary after #293's captured TaskMessage emission
and before #289's model handling of an authored AgentRequest. This is a test-only qualification, not extraction or a new API.

## Sources and composition

Consumer: SemTeams `a331910ee6ec297f86530c4a1d69ca84001a19f5`. Framework: SemStreams
`8b99efe9c66a4faa4fa509f9f62cc6bad8392128`. The integration-tagged
[TestConfiguredLoopFirstRequestBoundary](../../../cmd/semteams/loop_runtime_integration_test.go) uses real local
`nats:2.14.4-alpine`. The inherited [fixtures and primitive goldens](../../../test/fixtures/rule-family-consumer/expected.json)
remain unchanged; their [provenance](../../../test/fixtures/rule-family-consumer/provenance.json) preserves live versus
supplemental scope.

`loadLoopBoundaryConfig` selects `graph-ingest` and `teams-loop` from both
[production](../../../configs/flow-bootstrap.json) and [mock](../../../configs/e2e-flow-bootstrap.json) bootstraps,
clears other services and redirects NATS to the test server. All declared streams, model registry, platform declaration
and selected component settings remain. Public config setup mints the runtime platform instance from the shipped stem;
the test checks its organization and stem instead of inventing another authority.

`loopBoundaryExternalInputs` changes only the loop's required, originally nonexternal inputs `agent.task`, `agent.response`
and `tool.result` to `External=true`. Restoring each original marker in memory must produce JSON equal to the original
complete component parameters. Subjects, required flags, stream coverage and budgets are unchanged. This is an isolated
two-component composition, not unchanged full-bootstrap admission.

Product `setupRegistriesAndManager` and `buildPayloadRegistry` feed public `StreamsManager`, `ConfigManager` and
`ComponentManager`. Exactly two status entries must be `Started`. Real graph-ingest supplies typed mutation and queries;
there are no handwritten graph responders or direct state writes standing in for loop birth.

## Observed first-request boundary

`loopBoundaryTask` constructs canonical TaskMessage inputs from the selected action role/model/prompt/knobs. Tools are
explicitly empty; action metadata/run anchors are not copied. `LoopID`, `RunID` and `ParentLoopID` are empty, so each case
starts a fresh root. This does not execute the rule action or resolve tool names.

Before task publication, the test installs a durable `AGENT` consumer filtered to `agent.request.>`. Each task is published
to its original fixture subject. Captured bytes decode through the product payload catalog as `*agentic.AgentRequest`
and pass canonical validation. Checks cover the subject `agent.request.<LoopID>`, literal first-request identity
`<LoopID>:req:1:0`, distinct loop identity, role/model, prompt inclusion, empty tools and independently expected knob values. At-least-once replay of
an already observed RequestID is acknowledged and skipped; one observation is not an exactly-once claim.

After capture, `graph.NewExactEntityReader.ReadExactEntity` queries the loop ID resolved with the public runtime platform.
It must return a positive KV revision, the exact entity ID, `agentic.LoopExecutionMessageType()`, and `agent.loop.task`
equal to this fixture's TaskID. Loop and persisted entity IDs must differ from earlier roots. This observes real typed
persisted state for the captured request; it does not independently measure birth/publication ordering under concurrency.
The frozen [loop component's birth gate](https://github.com/C360Studio/semstreams/blob/8b99efe9c66a4faa4fa509f9f62cc6bad8392128/processor/agentic-loop/component.go#L1641) supplies the source-level
ordering guarantee. The bounded source path is:

| Source operation | Frozen symbol |
|---|---|
| Decode the canonical task and dispatch to the loop handler | [component.handleTaskMessage](https://github.com/C360Studio/semstreams/blob/8b99efe9c66a4faa4fa509f9f62cc6bad8392128/processor/agentic-loop/component.go#L1492) → `handler.HandleTask` |
| Publish only after typed spawn identity and loop persistence | `WriteSpawnIdentity` gate at component.go:1641, then [publishResults](https://github.com/C360Studio/semstreams/blob/8b99efe9c66a4faa4fa509f9f62cc6bad8392128/processor/agentic-loop/component.go#L1773); [graph writer](https://github.com/C360Studio/semstreams/blob/8b99efe9c66a4faa4fa509f9f62cc6bad8392128/processor/agentic-loop/graph_writer.go#L447) uses `LoopExecutionEntity` |
| Copy task knobs and prepare the first request envelope/subject | [buildTaskRequest](https://github.com/C360Studio/semstreams/blob/8b99efe9c66a4faa4fa509f9f62cc6bad8392128/processor/agentic-loop/handlers.go#L1192) |
| Generate request identity | [GenerateRequestID](https://github.com/C360Studio/semstreams/blob/8b99efe9c66a4faa4fa509f9f62cc6bad8392128/processor/agentic-loop/state.go#L1850) yields the independently asserted fresh-root `<LoopID>:req:1:0` |
| Resolve context limit or fall back | [resolveModelLimit](https://github.com/C360Studio/semstreams/blob/8b99efe9c66a4faa4fa509f9f62cc6bad8392128/processor/agentic-loop/context_manager.go#L102) reads the preserved registry, then uses `DefaultContextLimit` if absent |

| Case, run in fixed order per bootstrap | Independent expected first-request values | Scope |
|---|---|---|
| `required-live.json` | Mode `required`, no function name, response format absent; role `researcher-research-plan`, model `research` | Selected live action fields; tools deliberately empty |
| `forced-function.json` | Mode `function`, function `decide`, response format absent; role `fixture-function`, model `fixture-model` | Supplemental supported-contract case |
| `response-format.json` | Choice absent; `json_schema`, name `decision_with_evidence`, strict true, complete nested schema; role `fixture-format`, model `fixture-model` | Supplemental supported-contract case |
| `omitted-knobs` | Both fields absent; role `fixture-omitted`, model `fixture-model` | Separate fresh root in the same running components, after both knobs appeared elsewhere |

The independent schema requires decision `proceed|hold` and an evidence array of objects with required `entity_id:string`
and `accepted:boolean`, rejecting additional object properties. Expected values use test-local primitive types.
Supplemental `fixture-model` is deliberately absent from the preserved shipped registry, so default context-limit fallback
warnings are expected. Its literal forwarding is proved; endpoint, provider and model-capability resolution are not.
The omission case does not prove same-loop nil reset or continuation.

Readiness uses joined public Start and Started states, then a durable capture installed before publication. Each operation
has a 15-second budget within a 90-second owner context; exact graph reads use a 5-second timeout. Public component Stop
is joined with a 10-second context before owner cancellation, including assertion failures, and config Stop has a 5-second
budget. These are cooperative context budgets, not a proof of unconditional wall-clock termination.

## Verification and mutation

Both bootstraps passed all four cases with these focused commands, including race detection:

```sh
go test -tags=integration ./cmd/semteams -run '^TestConfiguredLoopFirstRequestBoundary$' -count=1 -v
go test -race -tags=integration ./cmd/semteams -run '^TestConfiguredLoopFirstRequestBoundary$' -count=1 -v
```

A controlled mutation changed only `forced-function.json.tool_choice.function_name` in `expected.json` from `decide` to
`different_expected_function`, after saving its exact bytes. Running the first command exited 1 at the `forced-function.json`
subtest under both `flow-bootstrap.json` and `e2e-flow-bootstrap.json`. Restoring the saved bytes and rerunning exited 0; the restored golden SHA-256 is
`9b9e71eb9ba637a82414d70e46b8c86a7d7014eeab31a0a826556261dc8887fe`.

Local results are `/tmp/semteams-loop-boundary-initial.log`, `/tmp/semteams-loop-boundary-race.log` and
`/tmp/semteams-loop-boundary-mutation-{red,restored}.log`, with `/tmp/semteams-loop-boundary-mutation-result.json`.
Those temporary logs are not committed artifacts; tracked source/fixtures and the commands above support reproduction.
Full `task lint` (six pre-existing warnings, zero errors), `task test:race`, `task test:integration`, `go build ./...`,
`task schema:generate` and `task schema:check-changes` passed. After the final assertion/comment-only review fix, the focused
integration-race command above passed all eight cases again; the final-source golden mutation also failed/restored as
recorded above. The shared fixture directory remains unchanged.

Independent Go/source review passed on test SHA-256
`ff5ed3548fc7a9493970811c35d94b05b43ea5e279430dc021dcb3ac2219ab62`. The reviewer inspected code, source and the post-fix
race output without independently rerunning Docker. The final local race log is `/tmp/semteams-loop-boundary-final-race.log`;
broad check logs are `/tmp/semteams-loop-boundary-{lint,all-race,all-integration,build,schema}.log`. These remain local logs,
not hosted CI or committed artifacts. Final strict OpenSpec validation passed all five specifications; queue fixtures passed
26/26 and the active queue is empty. Independent archive/content review passed. These final checks are recorded locally in
`/tmp/semteams-loop-boundary-{openspec,queue-test,queue}.log`.

## Limits and upstream hold

No production config/runtime/dependency change, graph-policy emulation, birth bypass, rule execution, tool-resolution or
provider proof, paid call, continuation, same-loop nil reset, E3 inheritance, recovery/durability, retained-state or future
SemEngine claim. The real graph owner remains authoritative. [SemEngine #77's update](https://github.com/C360Studio/semengine/issues/77#issuecomment-5970772317)
schedules an inventory after natsclient 3.7; it is not a rollback decision or permission to copy policy. No merge, issue
closure or upstream-publication task is part of this slice.
