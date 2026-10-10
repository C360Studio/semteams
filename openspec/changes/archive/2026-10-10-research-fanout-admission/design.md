# Authoritative planner fan-out admission design

## Scope and state

[Draft PR #312](https://github.com/C360Studio/semteams/pull/312) claims this design slice of
[#272](https://github.com/C360Studio/semteams/issues/272). [ADR-064](../../../../docs/adr/064-planner-fanout-admission.md)
records the proposed architectural choice. Production enforcement is unchanged. No new task action, tool name,
payload, storage, dependency version or component is introduced by this design.

The design was measured against SemTeams `41cfcc882c7e6bf51b927329cabf0411be6f8829` and SemStreams
`8b99efe9c66a4faa4fa509f9f62cc6bad8392128` (the pinned module). The existing explicit per-loop caps from #311 remain
qualified; fan-out, whole-run accounting and unattended scheduling do not become qualified by this PR.

## D1. Source survey and measurement

Paths below are relative to the frozen SemStreams tree unless prefixed SemTeams. Positions identify the surveyed
revision, not a promised future package API.

| Source | Established behavior |
|---|---|
| `processor/agentic-tools/decide.go:187–219,603–639` | Advertises optional string-array subtopics; parser has no cardinality check. |
| `processor/agentic-tools/decide.go:241–439` | Parse, resolve action, restrict action, emit success signals, append decision facts, return terminal. |
| `processor/agentic-tools/decide.go:141–158,472–639` | Private parsing/SAP helpers; references outside their file occur only in its tests. |
| `processor/agentic-tools/decide.go:641–721` | Mutation implementation stays behind public `TriplePublisher`/`NewNATSTriplePublisher`. |
| `processor/agentic-tools/executors/register.go:82–108,183` | `SkipBuiltins` explicitly supports `decide`. |
| `processor/agentic-tools/executor.go:53,93,159,212` | Duplicate registrations refused; global definitions; direct executor dispatch. |
| `agentic/tools.go:226,405–416` | Tool validation checks ID/name; defines runtime-owned `agent.role` metadata. |
| `processor/agentic-loop/handlers.go:2150–2161` | Shared dispatch replaces caller role with the owning loop's role. |
| `processor/agentic-loop/handlers.go:1079–1087` | Select task tools or global discovery, then cache per loop. |
| `processor/rule/actions.go:1898` | Rule task tools resolve by name against global definitions. |
| `processor/rule/actions.go:1707–1746` | Publish once per list item; missing/malformed references can fall back to one unbound dispatch. |
| SemTeams `cmd/semteams/tools/emitplan/executor.go:137` | Plan artifact is additive audit; not dispatch admission. |
| SemTeams `configs/rules/research/02-plan-to-gather.json` | Fans out from decision subtopics; rule 03b joins using that list's length. |

The [handoff comment](https://github.com/C360Studio/semteams/issues/272#issuecomment-6101060504) retains the complete
probe source and command. Real `DecideExecutor` output facts were fed to the real `ActionExecutor` with the unchanged
rule 02 action. N=1,4,5,15 each returned terminal success and N recorded gather publications without `emit_plan`.
The probe deliberately used a non-loop fixture entity and recording publishers; it establishes the executor bypass,
not transport, role authority, run membership, investigator execution or join completion. No maximum was approved
by that experiment. The proposed four-item policy is a design choice.

The schema survey found no public request-preprocessor or contextual registry hook. The historical
`setupToolsAndPreprocessor` name in SemTeams is not evidence of one. Per-task tool definitions are an existing data
shape; getting the shared policy into the authoritative intake path remains an integration requirement.

## D2. One policy and one effective identity

The proposed immutable policy selects effective role `researcher-research-plan`, resolved canonical action `gather`,
minimum 1 and maximum 4. The research pack declares it; the runtime receives one validated snapshot for schema and
execution. Future Go package names/config syntax are not declared public API here. Do not insert a second ceiling
into personas, rule metadata or `emit_plan`, and do not add a new per-spawn field merely to carry this constant.

The action is the existing resolver's output, so inputs it already normalizes to `gather` cannot evade the check.
Do not normalize again in a wrapper. Deployment action restrictions retain precedence and existing errors.
The role comes from runtime loop state and is bound to the effective LoopID at dispatch. Model arguments and a
`category` metadata string cannot select, erase or override policy. A direct in-process test call is not proof of
this transport authority. Qualify every admitted invocation/replay/approval path; if identity is unresolved or
inconsistent, return an internal/authority failure without effects, not an apparently out-of-scope success.
The proposed authority-failure behavior is an explicit fail-closed change for unqualified identities; preservation
of unrelated decisions applies to calls with established identities. Do not broaden external ingress authority.

There is no policy hot reload in this slice. Runtime restarts/recovery must reconstruct the same version of policy
for retained admitted work; changing policy across deployments needs a separate migration decision. This design
qualifies fresh isolated state and does not authorize retained-state conversion or wipes.

## D3. Admission and correction

The selected executor retains the frozen parse, allowlist and restriction sequence. After those succeed, before SAP
success telemetry and any decision mutation, consult the policy. For a protected `gather`, require a JSON array of
strings with 1–4 entries. Existing type failures retain their parser error; missing/null/empty/over-limit failures
use `agentic.ToolErrorInvalidArgs`, preserve call correlation and return `StopLoop=false` with no successful decision
metadata. The diagnostic states the allowed interval and actual count where meaningful; it does not disclose another
loop's data. Record no next-action, reason, subtopics or successful SAP fact and call no publisher method.

The tool result returns to the same loop for correction. It creates no fresh planner, recovery counter, retry rule
or durable record. Existing iteration exhaustion remains the failure boundary, including #311's response-side
provider-request overshoot limitation. Validation does not extend or reset a budget.

Valid calls pass the original ordered list to existing mutation/terminal behavior. Do not trim, deduplicate, clamp,
split or silently drop items. Blank/duplicate string entries remain admitted by the original type parser, count
against four and receive ordinary fan-out/join treatment; semantic content quality is outside this cardinality slice.
All unrelated role/action combinations retain their frozen argument, restriction, error and completion behavior.

`emit_plan` may succeed with a larger or different audit list; the later terminal list still rejects if invalid.
That can leave an audit artifact from a rejected pass. Do not describe it as an admitted plan or infer a decision
from its existence. Enforcing audit-to-decision equality or atomic artifact revision activation is outside this PR.

## D4. Selected executor transfer and telemetry

A later implementation may transfer only the executor core and private helpers/tests from `decide.go`, keeping the
mutation adapter upstream. Before committing source, enumerate exact file/symbol/test provenance and MIT notice.
Retain frozen concrete types, sentinels and vocabulary. Reuse public `NewNATSTriplePublisher` and the actual platform
identity; do not reimplement CAS/mutation or reconstruct authority suffixes. The private helper-reference survey
supports this candidate cut, but does not replace external-consumer compilation of the final source/test set.

Use `RegisterBuiltins` with the existing skips plus `decide`, and register exactly one executor under the canonical
name. Existing duplicate rejection is a startup guard, not an override API. Verify configured tools and schema
inventory still expose one `decide`; inactive frozen code remains a dependency, not a second selected implementation.

Copied `promauto` names would collide with initialized upstream collectors. Select
`semteams_decide_tool_action_allowlist_sap_coerced_total` and `semteams_decide_tool_action_restricted_total` for the
local implementation, preserving label/meaning semantics and existing log/result/fact signals. Document that the
upstream series are no longer live decision counters. No new metric is needed to prove width. Tests must import both
packages and actually register/start the selected implementation to expose a collision.

This exact ownership change is permitted as a design candidate by ADR-061/062, not by treating private shared policy
as copyable. No model/governance rollback helper, generic engine code or whole-loop component moves in this proposal.

## D5. Scoped schema integration and activation gate

Select the loop task-intake boundary before `CacheTools`, where effective role and selected tool definitions meet.
Project the research policy into a fresh per-loop definition; never mutate the shared registry's map/slice in place.
An unrelated loop started concurrently must continue to receive the base schema. Initial, retry, continuation and
recovered requests must use the same scoped definition and execution policy. The global registry remains discovery,
not the research-policy authority.

For this role, use an action-conditional structural schema: `gather` requires `subtopics` with `minItems=1`,
`maxItems=4`, and string items; other action branches preserve their previous argument contract. Existing action
normalization remains an executor behavior, so malformed/cased action inputs a provider emits still pass through
the existing resolver and the same canonical-action guard. The model-visible canonical action branch is not the
security boundary. For other roles, retain the base schema unchanged.

The intake transformation is a behavioral integration contract, not a shipped method or processor hook. Implementing
it requires a separately qualified agent-runtime seam/ownership change. It cannot be supplied by arbitrary task
metadata, a NATS interception subscriber or a new pack-specific component. A provider adapter must demonstrate that
its supported schema representation retains the conditional list constraint. An unsupported provider/deployment
cannot activate this contract with prose-only fallback. Test both provider request serialization and execution with
a schema-violating deterministic response; no paid research-quality benchmark is required for these mechanics.

The executor transfer and schema binding form one activation unit. Do not activate the local executor as a claimed
partial completion while the schema path is unresolved. No generic SemEngine #24 mechanism is required for array
admission. If the chosen implementation changes rule actions, qualify their existing #25/#26/#27 boundaries rather
than using this ADR as permission to copy them.

## D6. Required proving cases for later implementation

| Boundary | Required outcome |
|---|---|
| Width 1 and 4 | Real planner decision produces exactly N scoped investigators; all admitted findings join and synthesis runs once in the qualified journey. |
| Width 5 (also a larger list) | Invalid arguments, no terminal success, zero publisher calls/facts and zero investigators attributable to the rejected call. |
| Missing, null, empty, scalar, mixed types | Explicit nonterminal rejection; no unbound fallback dispatch or quietly completed planner. |
| Correction | Same planner rejects five, then accepts four within its unchanged iteration budget. |
| Audit bypass | Skipped `emit_plan`, failed audit and mismatched audit/decision lists cannot bypass the terminal check. |
| Fresh recovery planner | Each new planner identity gets the same width policy; this is not a bound on the count of planners. |
| Action resolution | Canonical and SAP-normalized gather values obey the same bound; restricted/off-allowlist actions preserve their errors. |
| Unrelated decisions | Established nonplanner identities and planner non-gather actions retain schema, arguments, metadata and terminal behavior. |
| Identity | Normal, queued, approval and recovery paths derive role from owning loop; forged arguments/category cannot erase the scope; unresolved identity has zero effects. |
| Schema | Actual selected-role requests carry the constraint through each supported adapter; shared schemas stay unchanged under concurrent roles. |
| Startup/provenance | One registered decide, carried source tests, retained type/sentinel identities, no collector collision; explicit metrics migration. |
| Failure | Cancellation/mutation failures retain frozen classification; repeated invalid calls exhaust the existing loop rather than starting unbounded recovery. |

Use a failing width-five regression against the unmodified boundary before implementing the guard. Separate recording
publisher tests from isolated NATS 2.14.4 graph/rule and rebuilt mock journey evidence. Synchronize event observations
and use bounded waits; an empty queue at one instant is not proof of no later dispatch. Do not claim replay-safe or
exactly-once behavior from first-terminal observations. Future implementation must carry source tests and report any
retained frozen limits. This design's validation is documentation/source review and existing baseline checks only.

## Delivery boundary

This PR claims and completes design, not #272. The proposed ceiling/ownership choice is reviewable in ADR-064; the
missing intake schema seam and actual implementation qualification stay explicit on #272. Archive only the
**design-admission requirements**: living specifications must not say that a four-investigator cap is running.
No merge, runtime activation or parent-issue closure is authorized by this design request.

## Design validation evidence

An independent architecture/Go-contract reviewer checked the frozen source claims, ownership cut, role binding,
schema hold, rejection boundary and proving matrix and found no blockers. The review corrected the private-helper
citation to include `newRestrictedActionSet`. This is review of a proposed contract, not owner approval of the ceiling
or a qualification of implementation.

All nine required local baseline gates passed on the unchanged production tree plus the initial proposal:
`task lint`, `task test:race`, `task test:integration`, `go build ./...`, `task schema:generate`,
`task schema:check-changes`, `task openspec:validate`, `task openspec:queue-test`, and `task publish:verify-test`.
Lint reported six existing warnings and zero errors; schema generation produced no tracked changes. The final
documentation receives fresh strict OpenSpec, link and diff validation. These existing-runtime checks establish a
baseline only; none proves the proposed fan-out guard, conditional schema delivery or provider compatibility.

Final archive/content review also passed with no blockers. The ADR-index table separator was corrected. Strict
OpenSpec validation, the empty active queue, archived-delta/spec agreement, relative links and diff checks passed
after archival. Task completion records only performed design/review/archive work, not implementation or post-merge facts.
