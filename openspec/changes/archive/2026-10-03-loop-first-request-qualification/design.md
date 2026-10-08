# Composed loop first-request qualification

## Boundary

[Issue #294](https://github.com/C360Studio/semteams/issues/294) and
[PR #295](https://github.com/C360Studio/semteams/pull/295) qualify the gap between captured rule-family task emission
(#293) and model handling of an authored request (#289). Source is SemTeams
`a331910ee6ec297f86530c4a1d69ca84001a19f5`, frozen SemStreams `8b99efe9c66a4faa4fa509f9f62cc6bad8392128`.
The [evidence record](../../../../docs/migrations/loop-first-request/README.md) records exact composition, assertions,
commands and limits. Source feasibility is distinct from executed qualification.

## Implemented composition and observations

The integration-tagged `TestConfiguredLoopFirstRequestBoundary` selects only shipped `graph-ingest` and `teams-loop`
from both bootstraps and uses product registration/payload helpers, public managers and real NATS `2.14.4-alpine`.
Other services are omitted and the NATS URL targets the local fixture. Preserve all streams, model registry, declared
platform and selected component settings. The public manager mints the runtime platform instance from the shipped stem.
Real graph-ingest serves typed mutation and exact/prefix query APIs; no graph responders or state are fabricated.

Only the loop inputs `agent.task`, `agent.response` and `tool.result` gain truthful fixture-publisher External markers.
The test asserts original required/nonexternal values and JSON equality of the complete component parameters after
restoring those markers. No subjects, stream coverage, budgets or runtime parameters are altered. This qualifies an
isolated composition, not unchanged full-bootstrap execution.

Both components must be Started before installing the durable first-request capture and publishing canonical tasks.
Tasks are constructed from inherited action role/model/prompt/knobs with empty tools and no loop/run/parent identity.
No rule executor or tool registry is exercised. Captured requests must be canonical and valid, have the expected subject,
role/model, prompt and independent knob values, and belong to distinct roots. The first RequestID is asserted literally
as `<LoopID>:req:1:0`, independently of the framework's identity generator.

After each captured request, a public exact graph read establishes positive revision, the correct typed loop entity and
`agent.loop.task` equal to the fixture TaskID. Both loop and entity identities must differ from earlier roots. The test
observes persisted typed birth; source inspection separately identifies the component's birth-before-publication gate.
It does not provide an independent concurrency-ordering harness. At-least-once replay is acknowledged and skipped.

Required mode remains a selected live-source case; forced-function and response-format cases remain supplemental.
Their absent `fixture-model` registry entry produces expected default-context-limit fallback without changing the registry;
this proves literal model forwarding, not provider/capability resolution. The fourth case omits both knobs on a separate
fresh root within the same running component. It does not prove same-loop continuation or nil reset. Inherited #293
fixtures and primitive goldens remain unchanged.

## Evidence and review

Focused and focused-race runs passed both bootstraps and all four cases. Required lint, unit-race, integration, build and
unchanged schema checks passed. The final assertion/comment-only review fix passed the affected focused race rerun.
On that final reviewed source, changing the expected function name failed the intended case under both configurations;
exact-byte restoration returned the test to green. Independent Go/source review passed after the request-identity and
observation-ordering corrections. The evidence record supplies commands, final test/restored-golden hashes and limits;
temporary local logs are not retained artifacts. Final strict OpenSpec validation (5/5), queue fixtures (26/26), empty queue
and independent archive/content review passed. Operations use bounded contexts, durable capture-before-publish and joined Stop before owner cancellation.
These are cooperative budgets, not unconditional termination guarantees.

No runtime/config/dependency change, private policy copy, graph bypass, rule/tool/provider execution, continuation,
E3 inheritance, restart/durability, exactly-once, retained-state or future SemEngine qualification is claimed.
[SemEngine #77](https://github.com/C360Studio/semengine/issues/77) remains an unresolved rollback contract question; its
[owner update](https://github.com/C360Studio/semengine/issues/77#issuecomment-5970772317) schedules inventory after natsclient
3.7 and chooses no policy or API. Independent architecture, Go/source and final-content reviews are complete. The accepted runtime-composition
qualification is archived and synchronized. Upstream publication
is not a required task, and no merge or issue closure is authorized.
