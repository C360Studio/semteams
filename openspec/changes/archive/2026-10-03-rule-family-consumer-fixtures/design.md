# Rule-family consumer fixture design

## Scope

[Issue #292](https://github.com/C360Studio/semteams/issues/292) and
[PR #293](https://github.com/C360Studio/semteams/pull/293) supply evidence for SemEngine's chosen SETUP03B E1–E3 contract
([#25](https://github.com/C360Studio/semengine/issues/25), [#26](https://github.com/C360Studio/semengine/issues/26),
[#27](https://github.com/C360Studio/semengine/issues/27)). This is a consumer contribution, not a proposed family API.
The baseline is SemTeams foundation `042193463e3b27fb1996ed4e99bf2b0cd4024a06`, with SemStreams
`8b99efe9c66a4faa4fa509f9f62cc6bad8392128` frozen through its module dependency.

Portable fixtures live in `test/fixtures/rule-family-consumer/`; consumer contract tests live in
`test/contract/rule_family_consumer_test.go`. The compact
[evidence record](../../../../docs/migrations/rule-family-consumer/README.md) will map every case to its actual source,
expected values, exercised symbols and evidence level. Production rule packs remain unchanged.

## Evidence boundaries

| Case | Contribution and limit |
|---|---|
| Existing SemTeams tool-choice cases | Pin real source/action selectors and preserve live/support/parked classification. Current choices are 15 live and 20 parked required-mode actions; a parked source remains inactive. |
| Forced-function tool choice | Supplemental supported-contract characterization with explicit expected mode and function name; do not invent a current forced-function need in SemTeams packs. |
| Response format | Supplemental supported-contract characterization: explicit type, name, strict flag and nested schema values. Current SemTeams configuration has no response-format case. |
| Run-inheritance anchors | Pin the original production rules, exact JSON paths, values and reading symbols for E3 agentic-family work; no extra copies or execution are required. Inventory is not qualification of future inheritance/recovery behavior. |
| Retry examples | Name the reading symbol and classify core operation retry, agent-loop behavior or pack workflow policy. Similar field names do not establish identical owners or behavior. |

## Decoder and executor proof

Use frozen public decoding and execution boundaries rather than a local imitation of the future family extension.
State expected field values independently of the input parser and actual output. Decoder assertions must cover both
`ToolChoice.Mode` and `ToolChoice.FunctionName`, and `ResponseFormat.Type`, `Name`, `Strict` and nested schema content.
Checking only non-nil fields, remarshal equality or values derived from the same parser is insufficient evidence.

Report decoding separately from actual public executor → canonical `agentic.TaskMessage` emission. The executor test
must exercise the real action path and assert the concrete canonical payload and independently expected fields at its
public output boundary. Its exact supporting dependencies and capture boundary must be stated. This does not prove
agent-loop consumption, provider forwarding, paid-model behavior, the future engine/family implementation, or the full
source corpus. Do not label decoder-only evidence as an executed consumer path or broker delivery.

Run-anchor and retry mappings remain a small source inventory tied to symbols. The original new/inherit/none rules
serve as held E3 material, not a locally implemented family. A missing `retry` action type does not imply that the core
executor has no retry policy. `Definition.MaxIterations` is a configured value copied into state and available to explicit guards, not an automatic
stop. The selected research retry has no such guard; the actual per-action `effectiveMaxIterations` cap is separate.
No production configuration, module, shared policy, runtime API or schema changes are authorized.

## Review and contribution

The independent architecture plan is approved. The three cases now pass direct `json.Unmarshal` into `rule.Definition`,
Definition JSON round trips and actual executor emission. Six valid-input omission/value changes are rejected by the
shared oracle at decode and emission boundaries. This is not execution of the cold file loader or hot configuration
service. The controlled expected-function-name mutation failed both decode and emission tests; exact-byte restoration
returned both to green. Required local lint, unit-race, integration, build, unchanged schema and OpenSpec checks passed.
The evidence record preserves the reproduction command and limits. Independent Go/source review passed after the primitive-oracle and source-claim corrections; the reviewer separately
reran the focused race tests successfully. Independent final-content review of the archive and synchronized specification passed. After review and checks, upstream comments on
#26/#27 carry the reviewed commit SHA and exact fixture/test/evidence paths. Comments do not claim merge, engine readiness,
full 119/40 corpus qualification or completion of held E3 behavior. Publication needs the final reviewed SHA and is
tracked in the PR/issue, outside OpenSpec task completion. GitHub owns subsequent coordination and holds.
