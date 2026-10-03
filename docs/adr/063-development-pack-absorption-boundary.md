# ADR-063: Development-pack absorption boundary

**Status:** Accepted design (2026-10-03), after independent architecture/source review for [issue #290](https://github.com/C360Studio/semteams/issues/290) and
[PR #291](https://github.com/C360Studio/semteams/pull/291). This defines admission for a later implementation; it neither
transfers code nor activates a development pack. Independent final-content review of the archive also passed.

**Refines:** [ADR-061](061-agent-runtime-ownership-and-observable-work.md)'s separately qualified SemDev absorption.
Its shared-runtime ownership, repository-artifact authority, read-only program-pulse target and live/parked fence remain.

## Context

The [pinned donor trace](../migrations/semdev-pack-boundary/README.md) examines committed SemDev
`7a22bc7422c978992977be6e168bca8c3dba138b` on SemStreams beta.160 against SemTeams foundation
`042193463e3b27fb1996ed4e99bf2b0cd4024a06` and frozen SemStreams `8b99efe9c66a4faa4fa509f9f62cc6bad8392128`.
SemDev brings eight domain components and fourteen tools, including deterministic workflow stations, intake and
conversation integration. A pack cannot be qualified by copying its prompts and JSON alone.

The committed donor has useful enforcement: authorized conversation decisions, CLI-validation freshness before task
projection, a fail-closed single-task projector, measured rather than model-declared outcomes, committed-artifact
clean-room verification, and PR adoption by recorded head. Those mechanisms do not establish every intended guarantee.
Human approval is not sealed to the complete reviewed revision and checker inputs. Sequential replay/opposite-decision
refusal is not atomic: concurrent authorized approval and rejection have an undefined winner, while single-valued
replacement prevents a permanent wedge. Graph fact contracts validate caller intent rather than exclusive ownership;
core-NATS station dispatch has a crash gap. Sealed-run, verification-sensitivity
and workspace-UI work are unmerged/in-progress inputs. An accepted decision in that work is not a merged implementation.

There is also an authority mismatch. The donor authors a canonical change document and revision in graph state, then
renders that document into workspace files. ADR-061 requires the real repository OpenSpec artifacts to be authoritative
for human edits and subsequent validation. Preserving both as mutable authorities would make a visible edit or approval
ambiguous even if the rest of the workflow survived unchanged.

## Decision

### One pack, runtime and operator experience

Preserve the opinionated issue → OpenSpec → approval → implementation → measurement/review → clean-room verification
→ PR journey on the shared SemTeams agent runtime. Development rules, personas, sandbox tools, deterministic domain
stations and verification policy belong to the pack. Loop, model/tool orchestration, agent approvals, context and
trajectory behavior belong to the shared runtime. Generic graph, transport, mutation, rule evaluation,
lifecycle/projection and generic durability belong to SemEngine.

This is an ownership boundary, not a prescribed package layout. Shared types, sentinels and substrate policy retain
one upstream authority until their integration is qualified. Private imports and missing shared access do not authorize
copies or local policy adaptations. Preserve the single-task donor refusal until a separately reviewed expansion proves
broader execution. Existing parked SemTeams packs remain parked.

### Separate source artifacts from execution facts

| Authority | Absorbed-pack requirement |
|---|---|
| GitHub issues and optional Projects | Authoritative repository-work and PM data; distinguish PM status from runtime execution status and unknown data from absence |
| Repository OpenSpec and code | Human-editable source artifacts with exact revision identity, validation and concurrency checks; a displayed or graph copy cannot become a competing mutable source |
| Runtime task projection and decision facts | Freeze the approved task inputs with the source artifact revision; record authorized actor, decision identity, reviewed revision and effect, including invalidation |
| Harness outcome facts and supporting evidence | Preserve actual producer, attempt, artifact revision and evidence reference/body; model prose cannot supply a measurement or verification result |
| Forge delivery state | Preserve recorded commit/PR identity, authorized effects and visible uncertain outcomes; acknowledgment alone does not prove application |

The authoring change is an explicit pack adaptation and admission hold. It is not a request for a new engine sync API.
Runtime graph state remains canonical for execution facts and frozen task projections. Conversation events may arrive
through the donor's authorized GitHub channel or a separately qualified UI action; future UI commands need not become
GitHub comments. No second PM database or independent developer UI is introduced.

Future spec controls must edit the actual repository artifact, validate it, show the affected revision and invalidate
stale approval or execution eligibility. The pending donor UI's read-only project/work-item/spec/evidence direction is
useful design input only. Its write controls remain deferred until authority, revision, idempotency and audit semantics
are proved; this ADR does not design screens or an API.

### Preserve guarantees and expose what remains unproved

Admission must carry the donor's harness outcomes, declared writers and real mutation paths, bounded task policy,
independent verifier, governed delivery and meaningful refusal tests. It must separately prove revision-bound human
authorization and protection of verification authority: environment manifests, checker commands, indirect scripts and
reports cannot silently change beneath an approval. Cloning a commit establishes isolation from the warm workspace;
it does not alone establish that the cloned checker is trustworthy.

Evidence must distinguish implemented enforcement, source-test assertions, historical journey claims and future
requirements. Caller-intent contracts and writer censuses are valuable, but are not exclusive engine ownership.
Recovery claims must retain the current core-NATS dispatch gap and unfinished station restart reconstruction. The
upstream lifecycle recovery-gate fix is already represented in donor tests and must not be listed as an unresolved gap.
Neither these tests nor planned durability establish exactly-once external effects.

### Integrate with actual upstream contracts

Donor beta.160 graph patterns use `agent.chain` and `agent.agentic-loop`; the frozen target uses `chain.agent` and
`agentic-loop.agent`. Requalify typed mutation, entity patterns, full-group reconcile behavior, ports, lifecycle and
rule actions before a transfer. Do not hide this mismatch with aliases or weakened checks.

Agent actions and shared tool seams map to existing SemEngine [#25](https://github.com/C360Studio/semengine/issues/25),
[#26](https://github.com/C360Studio/semengine/issues/26), [#27](https://github.com/C360Studio/semengine/issues/27),
[#28](https://github.com/C360Studio/semengine/issues/28) and [#29](https://github.com/C360Studio/semengine/issues/29).
Generic attempt/effect durability remains planned under
[#24's owner ruling](https://github.com/C360Studio/semengine/issues/24#issuecomment-5937829621), not supplied by this pack.
SETUP03B has chosen the E1–E3 action-family contract, including startup refusal for unknown pack actions. The public
implementation still requires qualification. Shared failed-start rollback access is the open contract question
[SemEngine #77](https://github.com/C360Studio/semengine/issues/77), separate from #24. It chooses no API and grants no
permission to copy generic policy to force admission.

Existing SemStreams [#1352](https://github.com/C360Studio/semstreams/issues/1352),
[#1353](https://github.com/C360Studio/semstreams/issues/1353) and
[#1354](https://github.com/C360Studio/semstreams/issues/1354) cover original-source binding, bounded large-entity reading
and prior-tool evidence handoffs. After migration these are agent-runtime responsibilities over engine graph/store
primitives. Pack admission must qualify the relevant handoffs and donor ports; their existence is not proof of delivered
behavior or grounds to duplicate the requests as new generic engine features.

## Consequences and acceptance evidence

A later implementation requires an exact donor/source/test map, provenance and MIT notice preservation, explicit pack
adaptations and independent review. Prove actual artifact edit → CLI validation → revision-bound approval → matching
frozen task projection, plus stale approval/execution refusal, competing authorized approve/reject outcomes and their
visible account, and visible unknown write outcomes. Sequential refusal does not establish an atomic winner; this
design chooses no CAS mechanism or API. Carry rejection tests
for unsupported tasks, missing or altered verification inputs, false measurement claims, independent verification
failure, exhausted budgets and ambiguous external effects. Qualify the public runtime/engine boundary rather than only
retesting domain functions.

Research and autoresearch remain the live product categories. No runtime copy, dependency switch, private-policy
workaround, new engine API, UI implementation, paid run, retained-state migration or external developer workflow is
authorized here. GitHub issues own subsequent choices and holds; this ADR is not their implementation backlog.

## Alternatives considered

- Preserve donor graph-authoritative authoring unchanged: conflicts with ADR-061's real-artifact edits and leaves human
  review vulnerable to divergent sources.
- Absorb only personas and rule files: drops domain stations, harness authority and verification/delivery behavior.
- Import pending sealed-run/UI work as a guarantee: confuses approved direction with merged, qualified implementation.
- Build a parallel workflow, journal or substrate adapter now: duplicates runtime/engine ownership against unsettled APIs.

## Related

- [Source and acceptance evidence](../migrations/semdev-pack-boundary/README.md)
- [OpenSpec admission design](../../openspec/changes/archive/2026-10-03-semdev-pack-boundary/design.md)
- [SemEngine contract at reviewed main](https://github.com/C360Studio/semengine/blob/2ec3bcf08bc1d2794d6cb1081ebd1982e08cf2c5/docs/contract.md)
