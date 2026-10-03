# Development-pack absorption admission design

## Scope and evidence

[Issue #290](https://github.com/C360Studio/semteams/issues/290) and
[PR #291](https://github.com/C360Studio/semteams/pull/291) own this design-only admission contract.
[ADR-063](../../../../docs/adr/063-development-pack-absorption-boundary.md) records its rationale; the
[pinned donor trace](../../../../docs/migrations/semdev-pack-boundary/README.md) maps actual source and refusal tests.
SemTeams starts from foundation `042193463e3b27fb1996ed4e99bf2b0cd4024a06`, with frozen SemStreams
`8b99efe9c66a4faa4fa509f9f62cc6bad8392128`. Donor SemDev is
`7a22bc7422c978992977be6e168bca8c3dba138b`, on SemStreams beta.160.

Use the pinned donor for implementation claims. Draft sealed-run-contract and dirty-checkout verification/UI work are
unmerged/in-progress inputs; DR-0003's accepted decision status does not establish implemented or merged guarantees.
Source and tests were inspected, not executed as a new donor or absorbed-pack qualification in this design.

## Ownership and adaptation

The journey remains issue → OpenSpec → approval → implementation → measurement/review → clean-room verification → PR.
The development pack brings domain policy, tools, sandboxing and deterministic stations to one shared agent runtime.
Agent orchestration, context, approvals and trajectories belong to that runtime; generic graph, mutation, transport,
rule evaluation, lifecycle/projection and durability belong to SemEngine. The donor inventory has eight components
(six stations, intake and conversation) and fourteen tools. It is not a target layout or proof every registered tool
is advertised in every configuration.

GitHub/repository artifacts own repository work: issues and optional Projects supply PM data, and real repository
OpenSpec/code revisions supply human-editable artifacts. Runtime decision facts, frozen task projections and
harness-produced outcomes retain their own canonical execution authority and provenance. Trajectory/store evidence
must preserve producer, attempt/revision and accessible supporting material. A future approved UI action need not route
through a GitHub comment.

The donor instead authors its canonical OpenSpec document/revision in graph state and renders workspace files from it.
Absorption must adapt this: the actual repository artifact is the authoring authority; approved execution projections
reference and freeze its exact validated revision. Do not preserve a second mutable graph authoring copy or invent an
engine synchronization API. This is a pack admission hold, not a generic engine request.

SemTeams provides one operator UI. Pending donor workspace-UI designs contribute read-only project/work-item/spec/evidence
ideas, separate PM status/execution badges and explicit unknown states; they are not implemented features. Future spec
CRUD edits the real artifact and requires authorization, revision/concurrency checks, invalidation, command identity,
idempotency and audit/outcome semantics before activation. Detailed screens and API signatures remain unsettled.

## Accepted donor behavior and limits

- Projection requires the current revision to equal the CLI-validated revision and rejects more than one task. It does
  not bind human approval to the complete reviewed revision/checker inputs. Preserve single-task refusal pending a
  separately qualified behavioral change.
- Conversation decisions enforce actor authorization, current gate phase, freshness and sequential replay/opposite-decision
  refusal. Their read-then-write is non-atomic: concurrent opposite decisions have an undefined winner, although
  single-valued replacement prevents a permanent wedge. Revision-bound approval, competing-decision outcome handling
  and checker-authority protection remain admission requirements; approval display and validation freshness do not prove them.
- Measurement records the frozen task command's actual result; review approval derives from measurement and no findings.
  Verification clones the recorded artifact, loads its manifest from that clone and separates failure from infrastructure
  doubt. Isolation alone cannot establish the authority of mutable checker inputs or reports.
- Fact contracts constrain caller intent/full selected reconcile groups; writer censuses do not establish exclusive
  engine ownership. Distinguish actual writers and harness facts from model claims.
- Delivery uses the recorded commit and adopts an existing PR by head. Core-NATS station dispatch still has a crash gap,
  and station restart reconstruction is incomplete. The upstream recovery-gate fix is already covered by donor tests.
  No exactly-once or fully restart-safe delivery claim follows.

## Integration holds

The beta.160 donor's `agent.chain` / `agent.agentic-loop` entity patterns differ from the frozen target's `chain.agent` /
`agentic-loop.agent`. Requalify patterns, public typed mutation, full-group reconciliation, ports, lifecycle and rule
actions. Preserve one upstream authority for shared types, sentinels and policy; no private-policy copy or local
adaptation to force an extraction.

Existing engine work covers action registration/fields and agent action/verdict extraction (#25–28), removal of shared
tool dependencies (#29), and planned lifecycle/projection-based generic durability (#24). Public action-family shape
remains unsettled, while the pinned contract already requires unknown pack actions to fail startup. Public shared
failed-start rollback access is the separate open contract question
[SemEngine #77](https://github.com/C360Studio/semengine/issues/77), with no chosen API or policy-copy permission. Map to
actual delivered contracts when available.

Existing SemStreams #1352–1354 cover original-source binding, bounded large-entity reads and prior-tool evidence.
These belong to agent-runtime handoffs over engine graph/store primitives after migration, rather than wholesale engine
ownership. Their pack-facing behavior and donor ports require qualification. See ADR-063 for direct issue links.
The evidence record identifies bounded donor E1–E3 fixtures to offer when the action-family boundary is chosen;
ordinary conversation publish is not E4 approval-action proof. Supported uncovered needs may become deduped upstream
issues; unresolved product choices remain concise PM questions.

## Implementation admission and proving evidence

A later implementation claim must preserve source/test/fixture provenance and MIT notices, identify every adaptation,
and pass independent review. Require both success and refusal evidence for:

- actual repository artifact edit, CLI validation, human approval bound to that revision and matching frozen task
  projection; stale approval or execution must be refused, competing authorized approve/reject outcomes and the decision
  that actually governs execution must be evidenced, and unknown write outcomes remain visible;
- harness-owned outcomes with producer, attempt/artifact revision and supporting evidence; missing measurements or
  model-declared success cannot authorize progress;
- declared and actual writers with enforcement limits, complete reconciliation and canonical payload identities;
- protected verification inputs, independent committed-artifact proof, fail/retry distinction and rejection of absent,
  stale or altered evidence;
- bounded tasks and retries, explicit parks/failures, authorized delivery identity and recovery/uncertainty limits;
- actual pack/runtime/engine handoffs, including source/evidence availability, beyond isolated domain-unit tests.

Source-test assertions and historical journeys are not freshly executed absorbed-pack evidence. This change implements
admission only. Research/autoresearch remain live; Program Pulse and development activation remain targets. No copying,
module switch, private-policy workaround, engine API, UI, paid run or retained-state conversion is authorized.
