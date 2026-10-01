# Browser and frontend migration qualification

Baseline: SemTeams `ce22c961`, SemStreams `v1.0.0-beta.160`. Target: frozen SHA
`8b99efe9c66a4faa4fa509f9f62cc6bad8392128`, module
`v1.0.0-beta.162.0.20260930150212-8b99efe9c66a`. All journeys used the mock LLM.
Final stable repair matrix: **24 active scenarios passed, zero failed, five explicit skips** across 29 scenarios
(25 passing Playwright tests, five skipped tests). The historical 26-scenario result remains **20/1/5** below,
including the autoresearch delivery defect that motivated the approved SemTeams-only repair.
These are development qualification results, not evidence of live-model research quality or a shipped program pulse.
[Final stable results](browser-evidence/repair-final-stable/results.json) preserve each scenario, effective identity
and exact assertions; [pre-repair results](browser-evidence/final/results.json) retain the original failure.

## Approved autoresearch repair qualification

The historical matrix below remains intact: its 20/1/5 outcome identifies the original delivery defect.
The approved SemTeams-only repair adds a category terminal handoff through the authoritative origin loop, using
existing frozen framework rules and typed task lineage; it leaves the iteration driver unchanged.
[Five focused repair scenarios](browser-evidence/repair-focused/results.json) now pass on image
`sha256:d96116ba2b09643bb02fad97b2d0c2e069c3e8d96d2a19b11c77fa96c41cdca1`:

- Success: cap 2, actual artifact emission, kept best value and original-channel terminal result; run completed.
- Guardrails: kept 1.00, rejected crashed 0.10, actual typed final result; run completed.
- Descended clarification `ask_user`: actual original-channel prompt; same run awaiting approval.
- Nonbudgeted propose failure: actual max-iteration failure, no experiment/artifact, typed failure status; run failed.
- Descended clarification `respond_direct`: typed limitation status; same run remains executing by existing policy.

Every case proves one terminal task with native `ParentLoopID` and `RunID` naming the original dispatch loop, one run
with its unchanged authoritative origin, and one typed response on the original channel/user. The browser opens the
existing Evidence message log and checks the expanded raw payload exactly. Screenshots have existing right-rail
clipping; they are not full-content visual proof. Rich prose/artifact rendering (#261) remains outside this proof.
[Independent E2E review](autoresearch-repair-frontend-review.md) approved this scope. Lint/check/unit/build and direct
E2E type checks passed; [gate excerpts](browser-evidence/repair-focused/ui-gates.txt) retain the result.

The focused runtime source hash was unchanged before/after:
`79ad0a9e17faa8f3d3c4473808da9faa2d34353323612013f10e5fd3b0fd9ba8`.
Build and runtime manifests, exact derived configurations, logs and proofs are recorded alongside the focused results.
The first complete repair run passed all 24 active Playwright scenarios and skipped five, but is **invalid** as a
qualification run: a configuration README changed during `clarification-autonomous`, producing runner exit 125.
[Invalidation evidence](browser-evidence/repair-final/INVALID-source-mutation.json) proves the only changed scanned
file was `configs/rules/autoresearch/README.md`; it preserves both contents and hashes. This is an orchestration
failure, not a product failure. The guard was not relaxed or waived. The entire matrix was repeated after a
new build with all inputs frozen; this history is not described as an unbroken first-pass run.

[Paired prompt-warning evidence](browser-evidence/repair-final-stable/prompt-warning-comparison.json) records two existing
limitations at both pins: rule03's explanatory best-value token resolves on the wrong entity, and rule05's prose
plateau count applies `.length` to a scalar completion object. The real emitter comparison and state-based iteration
cap passed; these fixture results do not qualify live prompt grounding or plateau detection. The repair removes the
old terminal prompt's separate unresolved artifact-path token. Other configured capability warnings retain their
attribution below.

The valid final run used image `stmig162-backend:autoresearch-repair-stable`, ID
`sha256:83ddf378cef73029a014bb0f90225f2a166e9538a37ebfac1f62329f0934b23c`.
Its [752-file build manifest](browser-evidence/repair-final-stable/backend-build.json) was unchanged at
`f1d65f14c609217259d1c5636e862404180ec77c68ca5d12089b320969fd43f6`.
All 747 scanned runtime files [matched the image build](browser-evidence/repair-final-stable/browser-build-source-link.json);
the complete matrix's before/after hash was
`29fc06e6426d24a7b9364356dce3af929104dc69dcf405a97eb2a3b623f45552`.
[Source comparison](browser-evidence/repair-final-stable/source-comparison.json) and every scenario confirm no mutation.
The [only rebuild delta](browser-evidence/repair-final-stable/previous-build-delta.json) was the final README content.
Exact [configurations](browser-evidence/repair-final-stable/configs/),
[Compose files](browser-evidence/repair-final-stable/compose/),
[typed route and artifact proofs](browser-evidence/repair-final-stable/proofs/),
[runner output](browser-evidence/repair-final-stable/runner.log), image digests and fixture hashes are durable.
[Scoped cleanup](browser-evidence/repair-final-stable/scoped-cleanup.json) confirms no remaining containers, volumes or
network for the migration project. Playwright retries remained zero; the invalid whole-run repeat is disclosed above.

## Reproducible infrastructure

[Baseline Compose](browser-evidence/baseline-compose.json) and
[target Compose](browser-evidence/target-compose.json) record the complete resolved configurations.
[Baseline bootstrap](browser-evidence/baseline-bootstrap.json) and
[target bootstrap](browser-evidence/target-bootstrap.json) preserve actual initial test configs.
Image identities are recorded in [baseline images](browser-evidence/baseline-image-digests.txt) and
[initial target images](browser-evidence/target-image-digests.txt), which predate the final approval repair.
The historical pre-repair backend image `stmig162-backend:8b99efe` has ID
`sha256:6aba6492ddd7da32387ffd4fbb46c3414f2fb2949b02afe89fa3f2defab88ed7`.
[Pre-repair build record](browser-evidence/final/final-backend-build.json) and
[build-to-runtime source linkage](browser-evidence/final/final-browser-build-source-link.json) identify the qualified
code. The 741-file build manifest was unchanged; all 736 runtime-scanned files matched that build at launch.
Every scenario reports `source_changed: false`; the complete matrix's before/after source digest is
`bdbd34b5ad26c5253a65617bc58ffab2c49e46820b23ddb1c2659ac532df2d0f`.
[Source comparison](browser-evidence/final/source-comparison.json), complete
[derived configs](browser-evidence/final/configs/) and [Compose documents](browser-evidence/final/compose/)
are durable. Final scoped containers, volumes and network were removed.

Isolation changes from the checked-in Compose:

- Projects `stmig160` and `stmig162`; removed every fixed `container_name`; namespaced networks and all volumes.
- Host Caddy ports `33160` and `33162`; per-project tenant directories under `/tmp/semteams-migration-8b99efe`.
- Built/tagged separate backend, mock, sandbox and UI images; fresh project NATS volumes for every scenario.
- NATS `2.14.4-alpine`; Gemini, Anthropic and Brave credentials blank. No SemSource profile or paid models.
  Go integration helpers separately used cached `2.14-alpine` resolving to 2.14.7 at both pins;
  [runtime identities](evidence/runtime/nats-identities.json) record that distinct configuration.
- Baseline source exported before the module bump. Target source is the migration worktree.
  UI source is bind-mounted, so its source hash matters independently of its base image identity.

The runner is [qualify-migration-browser.py](../../../scripts/qualify-migration-browser.py), with the exact
[scenario matrix](../../../scripts/semstreams-migration-matrix.json). It captures test output, backend logs,
loop state and message-logger entries every 25 seconds, and removes only the scoped stack. It explicitly disables
retries.
Existing test deadlines were retained; the run-failed test received event/state synchronization, not a longer timeout.
The final runner records source hashes before/after, effective derived configs, fixture/spec hashes and image digests;
a source mutation invalidates that run. It reads `/readyz` inside the backend container because Caddy does not route it.
The original `/readyz` attempt through Caddy returned Svelte 404; that observation is a harness routing limitation.
Initial frozen `/loops` can return 503 with `loop state is not available right now; retry shortly` before submission;
it did not prevent subsequent admission and successful journeys. Final evidence pairs this with backend `/readyz`
HTTP 200 `READY`. The precise log is `Component.currentLoopSnapshot: loop projection not caught up failed:
graphview: view not ready`. This is the intended fail-closed authoritative-view contract while graph projection catches
up. Readiness 200 does not qualify empty-loop-list availability: that initial request was unavailable.
This source-backed contract and the paired HTTP/log evidence are retained in
[cold-start authority evidence](browser-evidence/final/cold-start-authority.json) and the
[warning ledger](browser-evidence/final/warning-ledger.json). Post-submission assertions await admitted state and pass.

Scenario-specific config derivations mirror Taskfile: routing loads coordinator rules 03/03b only; team-spawn loads
research/autoresearch rule 01 only; autonomous clarification restricts `ask_user` and loads its persona overlay.
The supplemental approval-boundary runs alone replace `approval_required` with `request_sandbox`, an advertised tool.
The additional run-approval-boundary scenario gates `emit_plan`, advertised by the research planner, after the run
anchor exists. It checks exact pending/answered tuples, outstanding 1→0, UI execution identity, lifecycle and final
reply.
The normal research/autoresearch paths remain ungated. These overrides qualify boundaries, not a changed production
policy.

## Existing gates and behavioral comparison

Initial frontend gates: lint passed, svelte-check 0 errors/0 warnings, 168 test files passed + 1 skipped,
3,720 tests passed + 12 skipped, production build passed. After initial compatibility plus approval tuple changes:
171 files passed + 1 skipped, 3,725 tests passed + 12 skipped, production build passed.
Final review fixes retire obsolete authoring-route tests and add approval settlement/terminal cases.
Final lint, check, test and build all passed: 171 test files passed + one skipped, 3,704 tests passed + 12 skipped;
svelte-check reported zero errors/warnings. Later fixture/journey edits also passed lint and check.
Durable excerpts: [initial gate evidence](browser-evidence/frontend-gates.txt) and
[final gate evidence](browser-evidence/frontend-final-gates.txt).

The original 21-task aggregate yielded 15 passes, one failure and five explicit skips at beta.160.
The frozen initial aggregate yielded 13 passes, three failures and the same five skips.
Manifests and exact result rows: [baseline](browser-evidence/baseline-browser-results.json),
[target initial](browser-evidence/target-browser-results.json),
[target corrections](browser-evidence/target-contract-fixes-results.json).
Original failures are retained in [difference excerpts](browser-evidence/differences.json).

| Journey / surface | Beta.160 | Frozen observation and attribution |
|---|---|---|
| research-mvp (N=1) | Old checks passed | Explicit root role; adapted identity and strict reply passed. |
| research-fanout (N=2) | Join passed; typed reply failed | All checks passed; intended delivery improvement. |
| autoresearch | Old checks passed; corrected artifact proof passed, typed reply failed | Original typed reply failed; approved repair now passes artifact, cap/best, native ancestry and exact UI reply. |
| autoresearch-guardrails | Passed | Strict typed reply and real kept/crashed metric bounds passed; mock evidence only. |
| Descended clarification, ask_user | New repair contract | Typed root-channel prompt passed; same run awaiting approval. |
| Descended clarification, respond_direct | New repair contract | Typed limitation passed; same run remains executing by existing policy. |
| Nonbudgeted propose failure | New repair contract | Actual max-iterations8 failure and typed root-channel status passed; run remains failed. |
| coordinator routing / readiness / spawn | Passed | Passed; route decisions and rule task admission. |
| sandbox-mvp | Passed | Passed; MockRunner, not empirical devcontainer attestation. |
| ops-agent | Passed | Old segment-order assertion failed; canonical system/domain correction passed. |
| activity / task story / chips / aliases | Passed | Final post-review repeat passed. |
| admin-flows-inventory / graph | Old inventory passed | Two final tests passed: composition and real graph identity/filter/inspector behavior; authoring retired. |
| chain-drill-in | Passed | Passed navigation/metadata; rich evidence bodies remain limited. |
| run-failed | Observer-arrival race failed | Explicit observer synchronization passed on both pins. |
| clarification-autonomous | Passed | Passed strict reply; interactive donor paths remain unavailable. |
| request_sandbox approval / reject / cancel | Old pending-state assertion failed | Three final boundary scenarios passed with isolated policy override. |
| run-approval-boundary | Target contract only | Actual run graph, projection, rules, UI execution identity and final reply passed. |

Two-gather proof compares exact IDs: two distinct gather tasks scoped to distinct subtopics, two completion objects on
one planner, those same IDs in the single synthesize task prompt, typed parent chain through reviewer and final
coordinator,
and completed run. It does not infer fan-out correctness from loop count alone.
See [baseline fan-out proof](browser-evidence/baseline-fanout.json) and
[final frozen fan-out proof](browser-evidence/final/research-fanout-proof.json).
Static responses prove orchestration, not evidence quality.
The final graph test used returned UUIDs and the full runtime platform suffix, verified canonical system/domain
segments in the inspector, and filtered domain `agent`. Its selection uses the existing graph selection test seam;
it does not qualify WebGL pointer hit-testing or evidence-body rendering. Both retired AI authoring routes returned
410, and no `/flowbuilder` request was made.

The run-failed synchronization correction was applied to the preserved beta.160 checkout and rerun once:
[corrected baseline](browser-evidence/baseline-race-fixed-results.json). This is an explained harness correction,
not retry-to-green of an unchanged failing test.

## Approval, cancellation, and terminal evidence

Supplemental frozen approve/reject/cancel journeys passed with the advertised `request_sandbox` gated in isolation.
They assert missing execution ID HTTP 400, stale ID HTTP 409, current UI action, foreign-user cancellation refusal,
terminal state, pending gate removal, and graph terminal-observed evidence. The UI no longer exposes loop pause/resume;
cancellation goes through the owned-user `/cancel` message command and treats HTTP-200 `type:error` as refusal.
A successful acknowledgement does not itself prove terminal state.

Beta.160 supplemental runs exposed a different old presentation contract: `/loops` remained `pending` while
`pending_approval` was populated. Their initial `awaiting_approval` assertion failed; these runs do not prove the
approval action itself failed. See [baseline boundary results](browser-evidence/baseline-boundary-results.json).

Review required preserving canonical `loop_completed` markers arriving before the loop record, including
failed/cancelled
outcomes, and preventing stale snapshots from undoing terminal evidence. UI reports record disagreement explicitly.
Approval forms reset on execution replacement and ignore settlements for earlier executions. Run approval history uses
exact JSON `[loopID, executionID]` pending-minus-answered tuples; historical tuples cannot make a terminal run
actionable.
The loop record's pending execution must agree before approval controls enable.
Final backend/UI qualification includes a real planner `emit_plan` gate after run anchoring:
[run approval proof](browser-evidence/final/run-approval-boundary-proof.json) records the exact current loop/execution,
run phase `awaiting_approval`, pending tuple and outstanding count 1; UI approval submits that execution identity,
then both retained pending/answered tuples, outstanding 0, completed run and final typed reply are verified.
This exercises the actual graph authority, projection adapter and rules as well as the browser controls.

Legacy `/api/ai/chat` and `/api/ai/generate-flow` are flow-authoring-only entrypoints; both now return HTTP 410 before
parsing the body or constructing model/backend clients. Donor libraries remain unreachable through these routes.

## Existing limitations, reproduced defects, and holds

- **Historical autoresearch route defect:** see [terminal-route evidence](autoresearch-terminal-blocker.md).
  RUN-triggered iterative spawns lacked typed ancestry required by frozen terminal delivery. The approved repair
  restores the terminal handoff through the authoritative origin; loop completion alone remains insufficient.
- **Autoresearch fixture defect:** both pins returned `invalid_args`:
  `best_experiment_id is required (use literal 'baseline' when iterations_kept == 0)` from `emit_autoresearch_artifact`.
  Canned subsequent success prose is not an emitted artifact. Exact [paired
  errors](browser-evidence/autoresearch-fixture-defect.json)
  are retained. The stronger [baseline assertion failed](browser-evidence/baseline-artifact-red.json) before correction.
  The corrected fixture uses `mock-provenance-second-kept-iteration`, a schema-valid static token explicitly distinct
  from generated loop identity. The [corrected beta.160 run](browser-evidence/baseline-autoresearch-corrected.json)
  proves actual successful ToolResult, matching artifact-path triple, and a running-best graph identity that refers
  to a real kept execute loop with the promoted value. Its strict terminal-reply assertion remains red.
  The [pre-repair frozen proof](browser-evidence/final/autoresearch-proof.json) passes those same artifact assertions
  and retains the terminal-reply failure. [Paired hashes](browser-evidence/final/autoresearch-paired-fixture.json) prove
  identical corrected fixture/spec at both pins. Final-state capture separately reports ten completed loops and a
  completed run; assertions after the failed delivery check did not execute. Static token and metric fixtures do not
  prove dynamic artifact lineage or live empirical provenance. The repaired final proof now passes all subsequent
  terminal delivery, phase and iteration assertions; the historical failed run is unchanged.
- **Donor approval scaffolds:** `tool-approval-gate` tried `create_rule`, which was not advertised to the loop:
  `tool "create_rule" is not permitted for this loop (advertised tool set)`. Its canned “created” prose is not a bypass.
  `approval-pause` reached only one complete coordinator because its category is parked. `approval-resume` was aborted
  after the identical impossible prerequisite; `clarification-resume-ui` was not run for that reason. No donor pack
  rewired.
- **Five explicit skips:** artifact-context-handoff (#261), run-failed-coordinator, run-failed-coordinator-inherit,
  ask-user-pause and clarification-resume. These remain skips, never counted as successful coverage.
- **Evidence bodies (#261):** graph metadata, storage references and navigation work; rich evidence/model body rendering
  remains incomplete. No claim that artifact-body handoff or a complete evidence-backed product pulse now works.
- **Continuation:** client requests still omit a persistent channel ID; frozen dispatch creates channels per request.
  This existing continuity limitation was not silently expanded into a migration feature.
- **External qualification:** SemSource-backed dogfooding stays held; a future SemEngine switch needs its own approved
  consumer contract. MockRunner and fixture metrics do not replace live attestation or empirical optimization evidence.

## Runtime diagnostics and qualification limits

The [paired warning ledger](browser-evidence/final/warning-ledger.json) preserves exact severity, code, message,
owner, effect and frozen source excerpts for the same research fixture at both pins. The nine new composition logs
are all **warning** severity and occur identically in offline validation of both shipped configs and live admission.
`composition.AssertValid` and boot admission reject errors while permitting warnings. Thus these passing gates do not
claim a warning-free topology. The findings mean:

| Finding | Attribution and bounded effect |
|---|---|
| `objectstore` disconnected; `write` no publishers; `events` and `stored` no subscribers | Existing optional message ports are unused; direct registered storage capability is separate. Stream ingestion/notification integration is not qualified. |
| `agentic-model/agent.stream` no subscribers | Existing optional model-chunk output is newly reported. No configured chunk consumer; activity/final response tests do not prove token-by-token prose streaming. |
| `agentic-tools/tool.list` optional API unused | Existing unconsumed discovery API. Actual advertised tool execution passed; discovery requests are unqualified. |
| `graph-gateway/graph_index_queries` optional API unused | Both bootstraps omit `graph-index`, which owns these handlers. Existing index-route integration gap; request orphans remain warnings even if marked required. Other graph journey success does not qualify these routes. |
| `teams-dispatch/user.response` no subscribers | Static component graph has no receiver; external message logger/browser observes actual typed replies. The finding does not establish the separate autoresearch lineage defect. |
| `teams-loop/trajectories` optional index unwatched | No configured downstream watcher; request-based trajectory access passed. Watch-based indexing is unqualified. |

Both pins also lack `graph-clustering`, the owner of `COMMUNITY_INDEX` and `COMMUNITY_SUMMARIES`.
Community generation stays unavailable with periodic retries, and missing stored summaries use the statistical
fallback; no complete community/GraphRAG capability is claimed. Both pins emit the same empty-duration warnings and
use the same 10-minute buffer and 2-minute cooldown defaults. Frozen research logs additionally record the documented
run-resolution ancestry fallback; that path's completed run/reply passed. No config was silently expanded to erase
these diagnostics. Existing optional capabilities would require separate configuration and qualification if claimed.

[Final runtime warning ledger](browser-evidence/repair-final-stable/runtime-warning-ledger.json) also retains exact
ToolResult errors. Approval gates and autonomous `ask_user` refusal are intentional policy checks. The activity
fixture calls unadvertised `query_entity` at both pins; its UI event assertions do not prove that tool succeeded.
All five autoresearch repair scenarios have no failed ToolResults. The propose failure is the actual configured
max-iteration failure, not canned success hiding a tool error. Prompt defects described above remain explicit.

Raw local logs and videos are under `/tmp/semteams-migration-8b99efe/evidence` and its sibling `repair/`;
durable manifests, exact configs,
structural proofs and failure excerpts above preserve reviewable evidence without relying only on that temporary path.
The historical pre-repair matrix completed with 20 passes, one autoresearch delivery blocker and five skips.
Independent frontend review is approved, including the repair fixture assertions; see the
[initial final disposition](frontend-review.md#final-frontend-re-review-disposition) and
[repair review](autoresearch-repair-frontend-review.md). The final stable repair run completed with 24 active scenario
passes and five explicit skips. Product holds and
coverage limitations remain applicable; these mock results do not qualify live research or rich evidence rendering.
