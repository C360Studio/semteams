# Browser and frontend migration qualification

Baseline: SemTeams `ce22c961`, SemStreams `v1.0.0-beta.160`. Target: frozen SHA
`8b99efe9c66a4faa4fa509f9f62cc6bad8392128`, module
`v1.0.0-beta.162.0.20260930150212-8b99efe9c66a`. All journeys used the mock LLM.
Final frozen matrix: **20 passed, one failed, five explicit skips** across 26 scenarios. The failure is the
retained autoresearch typed terminal-reply blocker; successful artifact emission is now proved before that assertion.
These are development qualification results, not evidence of live-model research quality or a shipped program pulse.
[Final results](browser-evidence/final/results.json) preserve every scenario, assertion error and effective identity.

## Reproducible infrastructure

[Baseline Compose](browser-evidence/baseline-compose.json) and
[target Compose](browser-evidence/target-compose.json) record the complete resolved configurations.
[Baseline bootstrap](browser-evidence/baseline-bootstrap.json) and
[target bootstrap](browser-evidence/target-bootstrap.json) preserve actual initial test configs.
Image identities are recorded in [baseline images](browser-evidence/baseline-image-digests.txt) and
[initial target images](browser-evidence/target-image-digests.txt), which predate the final approval repair.
The final backend image `stmig162-backend:8b99efe` has ID
`sha256:6aba6492ddd7da32387ffd4fbb46c3414f2fb2949b02afe89fa3f2defab88ed7`.
[Final build record](browser-evidence/final/final-backend-build.json) and
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
| autoresearch | Old checks passed; corrected artifact proof passed, typed reply failed | Corrected artifact proof passed; strict final typed reply failed. |
| autoresearch-guardrails | Passed | Passed; mock bounds and refusal paths only. |
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

- **Autoresearch route blocker:** see [terminal-route evidence](autoresearch-terminal-blocker.md).
  RUN-triggered iterative spawns lack typed ancestry required by frozen terminal delivery. Loop completion is
  insufficient.
- **Autoresearch fixture defect:** both pins returned `invalid_args`:
  `best_experiment_id is required (use literal 'baseline' when iterations_kept == 0)` from `emit_autoresearch_artifact`.
  Canned subsequent success prose is not an emitted artifact. Exact [paired
  errors](browser-evidence/autoresearch-fixture-defect.json)
  are retained. The stronger [baseline assertion failed](browser-evidence/baseline-artifact-red.json) before correction.
  The corrected fixture uses `mock-provenance-second-kept-iteration`, a schema-valid static token explicitly distinct
  from generated loop identity. The [corrected beta.160 run](browser-evidence/baseline-autoresearch-corrected.json)
  proves actual successful ToolResult, matching artifact-path triple, and a running-best graph identity that refers
  to a real kept execute loop with the promoted value. Its strict terminal-reply assertion remains red.
  The [final frozen proof](browser-evidence/final/autoresearch-proof.json) passes those same artifact assertions and
  retains the terminal-reply failure. [Paired hashes](browser-evidence/final/autoresearch-paired-fixture.json) prove
  identical corrected fixture/spec at both pins. Final-state capture separately reports ten completed loops and a
  completed run; assertions after the failed delivery check did not execute. Static token and metric fixtures do not
  prove dynamic artifact lineage or live empirical provenance.
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

Raw local logs and videos are under `/tmp/semteams-migration-8b99efe/evidence`; durable manifests, exact configs,
structural proofs and failure excerpts above preserve reviewable evidence without relying only on that temporary path.
The final qualification run completed with 20 passes, one explicit autoresearch delivery blocker and five skips.
Independent frontend review is approved, including the final fixture/runner assertions; see
[final disposition](frontend-review.md#final-frontend-re-review-disposition). The known delivery blocker and product
holds remain explicit; this is not an all-green release claim.
