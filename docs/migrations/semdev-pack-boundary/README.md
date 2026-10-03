# SemDev development-pack absorption evidence

This is the source trace for [ADR-063](../../adr/063-development-pack-absorption-boundary.md),
[issue #290](https://github.com/C360Studio/semteams/issues/290) and
[PR #291](https://github.com/C360Studio/semteams/pull/291). It qualifies an admission design, not a running SemTeams pack.

## Source boundary

- Consumer: SemTeams foundation `042193463e3b27fb1996ed4e99bf2b0cd4024a06`, frozen SemStreams
  `8b99efe9c66a4faa4fa509f9f62cc6bad8392128` (beta.162 pseudo-version).
- Committed donor: SemDev `7a22bc7422c978992977be6e168bca8c3dba138b`, SemStreams beta.160.
- Separate input: draft [SemDev #40](https://github.com/C360Studio/semdev/pull/40), commit
  `ca3956af2ed87d5fa5bdb8183cdb506f7beb7240`, and dirty verification-sensitivity/workspace-UI designs.
  [DR-0003](https://github.com/C360Studio/semdev/blob/ca3956af2ed87d5fa5bdb8183cdb506f7beb7240/docs/decisions/0003-run-contract-is-sealed-and-pre-resolved.md)
  is an accepted decision, not proof its implementation is in the donor baseline.

The source links below are pinned to the committed donor. Source and test assertions were inspected; this design did
not execute donor unit/integration/paid journeys or an absorbed pack. The donor's historical
[evidence ledger](https://github.com/C360Studio/semdev/blob/7a22bc7422c978992977be6e168bca8c3dba138b/docs/evidence-ledger.md#L66) records earlier M1 and live-forge claims; those are historical evidence,
not fresh qualification here. The [source manifest](source-manifest.json) records exact pins, reviewed source hashes and the selected unmerged input hashes.

## Workflow and authorities

| Step | Pinned source | Established mechanism and material limit |
|---|---|---|
| Intake and run birth | [admission record](https://github.com/C360Studio/semdev/blob/7a22bc7422c978992977be6e168bca8c3dba138b/internal/intake/record.go#L84); [mint-run rule](https://github.com/C360Studio/semdev/blob/7a22bc7422c978992977be6e168bca8c3dba138b/configs/rules/coordinator/01-issue-intake-mint-run.json) | Strict admission record (`admission-check`); rule `publish_agent` with `run_scope=new` creates the run. |
| OpenSpec authoring | [create/revision](https://github.com/C360Studio/semdev/blob/7a22bc7422c978992977be6e168bca8c3dba138b/internal/tools/createchange/createchange.go#L84); [workspace rendering](https://github.com/C360Studio/semdev/blob/7a22bc7422c978992977be6e168bca8c3dba138b/internal/tools/writechange/writechange.go#L124) | Canonical graph change document/revision (`create-change-author-tool`) is rendered to files. Repository-authoritative authoring is an absorption adaptation, not donor behavior. |
| Human decision | [approval adapter](https://github.com/C360Studio/semdev/blob/7a22bc7422c978992977be6e168bca8c3dba138b/internal/conversationchannel/approval.go#L156); [resume rule](https://github.com/C360Studio/semdev/blob/7a22bc7422c978992977be6e168bca8c3dba138b/configs/rules/run-lifecycle/02-resume-after-change-approval.json) | Authorized actor, current gate, fresh message, sequential replay handling and opposite-decision refusal; `approval-adapter` stamps the decision. The [read-then-write is non-atomic](https://github.com/C360Studio/semdev/blob/7a22bc7422c978992977be6e168bca8c3dba138b/internal/conversationchannel/approval.go#L316): concurrent opposite decisions have an undefined winner; single-valued replacement avoids a permanent wedge. Approval is not sealed to all reviewed revision/checker inputs. |
| Task projection | [projector](https://github.com/C360Studio/semdev/blob/7a22bc7422c978992977be6e168bca8c3dba138b/internal/tools/projecttasks/projecttasks.go#L58); [task policy](https://github.com/C360Studio/semdev/blob/7a22bc7422c978992977be6e168bca8c3dba138b/internal/devtask/project.go#L103) | Current revision must equal CLI-validated revision; more than one task is refused; required task fields and budget clamp 1–5 are enforced. |
| Implementation | [patcher](https://github.com/C360Studio/semdev/blob/7a22bc7422c978992977be6e168bca8c3dba138b/internal/runspace/patcher.go#L60) | Limits patch paths to approved task targets and commits the result. Pending sealed-run checker/environment guards are not inherited guarantees. |
| Measurement and review | [measurement](https://github.com/C360Studio/semdev/blob/7a22bc7422c978992977be6e168bca8c3dba138b/internal/tools/measuretask/measuretask.go#L111); [review verdict](https://github.com/C360Studio/semdev/blob/7a22bc7422c978992977be6e168bca8c3dba138b/internal/tools/submitreview/submitreview.go#L166) | Runs frozen task command, records actual exit (`measurement-harness`); approval requires `measurement.CanApprove` and no findings, rather than a model verdict. |
| Independent verification | [artifact verifier](https://github.com/C360Studio/semdev/blob/7a22bc7422c978992977be6e168bca8c3dba138b/internal/tools/verifyartifact/verifyartifact.go#L126); [pure verdict](https://github.com/C360Studio/semdev/blob/7a22bc7422c978992977be6e168bca8c3dba138b/internal/verify/verify.go#L119) | Clones recorded committed artifact, reads manifest from clone, cold-proves and records pass/fail/retry (`verify-harness`); infrastructure doubt is distinct from artifact failure. |
| Delivery | [delivery](https://github.com/C360Studio/semdev/blob/7a22bc7422c978992977be6e168bca8c3dba138b/internal/tools/openpr/delivery.go#L91) | Pushes recorded `attempt.commit.sha`; adopts existing PR by head before create; `open-pr` records the reference. This is not exactly-once external-effect proof. |

The [registry](https://github.com/C360Studio/semdev/blob/7a22bc7422c978992977be6e168bca8c3dba138b/internal/registry/registry.go#L36) lists six domain stations (delivery, projection, validation,
floors, verify and provision), issue intake and conversation: eight components. Its fourteen tools include authoring,
workspace, measurement, review, conversation and four SemSource read proxies. Registration is not universal exposure:
the proxies are advertised by a separate condition pack. This inventory shows why absorption needs domain Go integration;
it is not an instruction to copy the registry or a target package layout.

[Graph contracts](https://github.com/C360Studio/semdev/blob/7a22bc7422c978992977be6e168bca8c3dba138b/internal/graphown/contracts.go#L1) derive local writer intent from vocabulary and reconcile the full
selected predicate group. They do not reserve exclusive engine ownership. The
[typed mutation seam](https://github.com/C360Studio/semdev/blob/7a22bc7422c978992977be6e168bca8c3dba138b/internal/graphown/ports.go#L9) and real write paths must travel with their enforcement limits.
Graph execution facts and frozen task projections remain canonical runtime records. GitHub issues/optional Projects and
repository work/spec/code/PR artifacts have separate source authority; future UI controls must not replace either with
an editable display copy.

## Meaningful source tests

These are acceptance anchors to carry and adapt, not a claim that they ran in this change.

- [Lifecycle source census and bad fixture](https://github.com/C360Studio/semdev/blob/7a22bc7422c978992977be6e168bca8c3dba138b/test/conformance/g2_lifecycle_test.go#L151).
- [Tool schemas reject model-supplied outcome fields](https://github.com/C360Studio/semdev/blob/7a22bc7422c978992977be6e168bca8c3dba138b/test/conformance/g3_schema_test.go#L40).
- [Writer census, violations and sanctioned gate writers](https://github.com/C360Studio/semdev/blob/7a22bc7422c978992977be6e168bca8c3dba138b/test/conformance/g5_writers_test.go#L36).
- [Alternate validated slug and reauthored/unrevalidated change refused](https://github.com/C360Studio/semdev/blob/7a22bc7422c978992977be6e168bca8c3dba138b/internal/tools/projecttasks/projecttasks_test.go#L303).
- [Multiple tasks refused](https://github.com/C360Studio/semdev/blob/7a22bc7422c978992977be6e168bca8c3dba138b/internal/tools/projecttasks/projecttasks_test.go#L355).
- [Failing command, frozen command and timeout outcomes](https://github.com/C360Studio/semdev/blob/7a22bc7422c978992977be6e168bca8c3dba138b/internal/tools/measuretask/measuretask_test.go#L225).
- [False success, missing measurement and findings cannot approve](https://github.com/C360Studio/semdev/blob/7a22bc7422c978992977be6e168bca8c3dba138b/internal/tools/submitreview/submitreview_test.go#L174).
- [Verifier pass/fail/retry and prove-error stamps no result](https://github.com/C360Studio/semdev/blob/7a22bc7422c978992977be6e168bca8c3dba138b/internal/tools/verifyartifact/verifyartifact_test.go#L88).
- [Existing PR adoption, replay and missing commit refusal](https://github.com/C360Studio/semdev/blob/7a22bc7422c978992977be6e168bca8c3dba138b/internal/tools/openpr/openpr_test.go#L168).
- [Fixture-seeded mock bridge, retry, review reentry and budget park](https://github.com/C360Studio/semdev/blob/7a22bc7422c978992977be6e168bca8c3dba138b/test/e2e/journey_test.go#L247).

## Admission holds and adaptation evidence

| Boundary | Evidence and admission consequence |
|---|---|
| Real artifact authority | Donor graph document → file rendering conflicts with ADR-061's real repository edits. Adapt to repository-authoritative OpenSpec and freeze the approved task projection against that revision. Prove actual edit/CLI validation/approval/matching projection, stale approval and execution refusal, and visible unknown write outcomes. This is pack work, not an engine sync feature. |
| Approval and verification authority | Current actor/phase/freshness checks and validated-revision equality do not seal human approval to all checker inputs or decide concurrent approve/reject ordering. Admission must prove competing-decision outcomes and their visible account alongside revision binding. Pending sealed-run and sensitivity designs address environment files, indirect scripts and report authority; preserve them as requirements, not implemented guarantees. |
| Framework version | Donor [contract patterns](https://github.com/C360Studio/semdev/blob/7a22bc7422c978992977be6e168bca8c3dba138b/internal/graphown/contracts.go#L83) use `agent.chain`/`agent.agentic-loop`; frozen target uses `chain.agent`/`agentic-loop.agent`. Requalify entity classes, full-group mutations, payload identities, ports, lifecycle and rule actions before transfer. |
| Station recovery | [Station transport](https://github.com/C360Studio/semdev/blob/7a22bc7422c978992977be6e168bca8c3dba138b/internal/station/station.go#L23) is core NATS with a publish/handle crash gap; ordinary exhausted failures park, shutdown aborts do not. Restart reconstruction remains incomplete. The [OnRecovery tripwire](https://github.com/C360Studio/semdev/blob/7a22bc7422c978992977be6e168bca8c3dba138b/test/conformance/upstream_asks_test.go#L267) is now a regression guard for an already-fixed upstream gate. Do not confuse that historical fix with full reconstruction. |
| Shared substrate | Keep shared types, sentinels and failed-start rollback policy upstream; unavailable access is an integration hold, not permission for a local copy. [SemEngine #77](https://github.com/C360Studio/semengine/issues/77) asks for the rollback contract decision; no API is chosen. Existing engine action/tool work and planned durability are mapped in ADR-063. |
| Source/evidence handoffs | SemStreams #1352–1354 remain existing handoff work. Qualify original-source binding, bounded reads, prior-tool evidence and donor ports as agent-runtime behavior over engine graph/store primitives; do not duplicate them as generic engine requests. |
| Operator experience | Pending workspace-UI design is read-only first, separates PM status from execution and unknown from absent data, and defers writes. Use it as input to the single SemTeams UI; actual artifact/decision authority, concurrency, idempotency and audit remain admission requirements. |

The donor's [create-change spawn](https://github.com/C360Studio/semdev/blob/7a22bc7422c978992977be6e168bca8c3dba138b/configs/rules/coordinator/02-create-change-spawn.json)
(`run_scope`, forced `tool_choice`) and [retry route](https://github.com/C360Studio/semdev/blob/7a22bc7422c978992977be6e168bca8c3dba138b/configs/rules/dev-from-task/06c-route-retry.json)
(budget and ordered actions) are bounded E1–E3 consumer fixtures when the engine's family boundary is chosen. The
[conversation approval route](https://github.com/C360Studio/semdev/blob/7a22bc7422c978992977be6e168bca8c3dba138b/configs/rules/conversation/03a-route-intent-approve.json)
is ordinary publish to a validating adapter, not evidence for the E4 approve action family. These samples establish
neither a new API nor an implementation schedule.

A later transfer must retain MIT licensing/provenance and qualify success and refusal paths at the real shared-runtime
boundary. Neither source inspection nor historical bridge evidence proves new SemTeams runtime behavior, retained-state
migration, arbitrary-repository generality, restart safety or exactly-once effects. Independent source/architecture
review passed after correcting the concurrent-decision claim. Final-content review of the archive and synchronized
admission spec also passed.

## Design validation (2026-10-03)

Strict validation passed for all five OpenSpec items; the 26 queue fixture tests passed. All 38 pinned donor source
hashes, pinned README link targets/line bounds, local document links and documentation whitespace checks passed.
These checks validate the admission documents and their source references. Donor source/test inspection is not runtime
execution, and no donor journey, absorbed pack or paid workflow was run. Independent architecture/source review passed
after requiring the explicit concurrent-decision limit and future competing-decision evidence. It confirms the design
and source mapping, not runtime qualification. Final-content review passed: the archived and living requirements
match exactly (three requirements, nine scenarios), links resolve, and the evidence and task record remain design-only.
