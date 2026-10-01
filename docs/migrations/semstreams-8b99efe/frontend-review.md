# Independent frontend migration review

## Status and scope

**2026-10-01 final frontend code status: approved after F1–F4 corrections.** This is independent frontend code
approval, not migration adoption approval. Independent Go review also approves the repaired approval projection.
The final browser matrix records 20 passes, one autoresearch delivery failure and five explicit skips.
The initial findings below are retained as review history; the final disposition and browser addendum follow.
The reviewer made no frontend implementation changes; all blocking frontend findings were fixed and re-reviewed.

Reviewed the working frontend diff in PR #281 against baseline `ce22c961` and the frozen SemStreams commit
`8b99efe9c66a4faa4fa509f9f62cc6bad8392128`, with the [approved contract inventory](contract-inventory.md).
Scope includes identity parsing, run projections, dispatch ownership/cancellation, approval execution correlation,
terminal SSE handling, composition inventory, active graph routes, relevant component tests, and browser assertions.
Generated TypeScript is included in the final passing frontend gates. PM follow-through after code review:
`task ui:generate-types:check` passed at content commit `f70537c8` with no tracked output drift.

## Initial blocking findings (resolved)

### F1 — P1: terminal observations are misread and can regress to active state

Locations at initial review: `ui/src/lib/types/agent.ts:127`, `ui/src/lib/types/agent.ts:160`, and
`ui/src/lib/stores/agentStore.svelte.ts:33` / `:86`.

The frozen dispatch emits terminal SSE envelopes with `type: "loop_completed"`, a **bare** `loop_id`, and no
`data.state`. See frozen `processor/agentic-dispatch/http.go:579` and `:1012`; the completion payload's outcome
identifies success, failure, or cancellation. The UI only recognizes an envelope whose ID starts with `COMPLETE_`.
A real frozen terminal envelope therefore falls through to `normalizeWireLoop`, which supplies `exploring` when
state is absent. The old prefix-only branch also drops a completion received before its loop, and `mergeLoop`
overwrites a prior terminal outcome/state with later live records or snapshots.

This violates the approved marker-before-record-CAS contract. A terminal-record CAS conflict is an explicitly
retained upstream residual, so an eventual live-record update cannot be presumed to repair the UI.

Required correction: consume the actual terminal envelope; retain durable terminal evidence when it arrives before
loop metadata; prevent a later nonterminal record/snapshot from erasing it; and expose record/terminal disagreement
honestly rather than inventing backend repair. Regression tests must use the frozen envelope shape, cover all three
outcomes, marker-before-loop ordering, and stale SSE/snapshot ordering.

### F2 — P1: edited approval arguments can be applied to a different execution

Location at initial review: `ui/src/lib/components/board/PendingApprovalSection.svelte:33` / `:71`.

The modify editor seeds `argsDraft` from pending execution A, but submission reads the current
`pendingApproval.execution_id`. If SSE or a parent update replaces A with execution B while the editor remains
mounted, A's edited arguments are posted with B's identity. The server correctly validates B, so its stale-execution
fence cannot distinguish this unreviewed decision from a real review of B.

Required correction: bind local draft state to the execution that was reviewed, or reset/remount it when loop or
execution identity changes. Add a component regression that enters modify for A, replaces the pending request with
B, and proves A's draft cannot be submitted as B. Check that an old asynchronous submission cannot clear B's draft.
This is part of the frozen `execution_id` approval boundary, not a request for a new approval capability.

### F3 — P1: typed cancellation refusals are silently treated as success

Location at initial review: `ui/src/lib/services/agentApi.ts:297` / `:349`, with callers in
`ui/src/lib/components/board/TaskDetailPanel.svelte` and `ui/src/lib/components/layout/ChatBar.svelte`.

Cancellation now uses the correct `/teams-dispatch/message` command ingress and requesting identity. However,
`sendMessage` only rejects non-2xx HTTP responses. Frozen `handleCancelCommand` returns a typed user response with
`type: "error"` for invalid/absent/foreign loop IDs; `processCommandSync` similarly returns typed permission errors.
These are HTTP 200. Both UI callers discard returned content, so a refused cancellation produces no error feedback.
See frozen `processor/agentic-dispatch/commands.go:113` and `http.go:250`.

Required correction: surface typed error responses as actionable failures and retain input where appropriate. Add
an HTTP-200 `type: "error"` cancellation regression. Successful command acknowledgement must remain distinct from
observing the final cancelled loop/run state.

## Other observations and qualification limits

- Composition JSON interfaces match frozen `composition/graph.go` and `composition/findings.go`. The retained
  `/components/flowgraph` and `/components/validate` operations expose admission at boot, not current liveness.
  Change the inventory's “Running components” claim to describe admitted/configured components. Add empty-result
  and independently failing endpoint coverage; a 503 must not become a successful empty inventory.
- `/graph` now uses entity exploration directly and no longer fetches a saved flow. `/admin/flows` no longer calls
  retired flowbuilder endpoints or exposes author/deploy controls. Unreachable donor services/components remain in
  source; their presence does not establish a shipped capability.
- `PendingApproval.timeout` still describes omitted/zero values as indefinite, contrary to the frozen 12-hour
  default/cap. `ApprovalAcceptResponse` does not yet declare the returned `execution_id`. Several LoopTracker and
  old state-alias comments describe retired internals. Update these contract comments/types.
- Ordinary `sendMessage` still omits `channel_id`. Frozen HTTP assigns a new channel when absent, while implicit
  continuation matches user, channel type, and channel ID. This was also absent in the baseline client. Qualify it
  as an existing continuation limitation unless the approved adaptation explicitly supplies stable routing;
  do not claim persistent conversational continuation from identity headers alone.
- Identity segment order is corrected in the graph parser, AI entity lookup, and run/loop infixes, preserving the
  effective platform segment instead of hard-coding its framework suffix. This pass does not certify all dynamic
  graph births; those require backend and isolated browser evidence.
- Approval controls now carry a pending execution identity and the run-level tool gate waits for request details.
  The product-owned clarification/tool-gate distinction remains. Removal of unsupported paused controls is correct.
- Trajectory remains bounded fact metadata with storage references. No evidence-body dereferencing was added;
  [issue #261](https://github.com/C360Studio/semteams/issues/261) remains an existing limitation. Typed response
  publication in message-logger and successful fact retrieval do not prove prose/tool-body rendering.
- The updated research/autoresearch browser assertions distinguish typed publication and final-loop correlation
  from user-visible rendering. Final target results are not yet available for this review. Approval-resume's old
  direct POST fixture still lacked `execution_id` when inspected; the frontend developer owns its replacement.
- The reviewed temporary matrix runner writes per-scenario derived configuration and hashes, uses separate compose
  project/port and fresh volume teardown, and captures loops/message state plus backend logs every 25 seconds.
  This is useful infrastructure evidence, not itself a passing journey or proof of readiness/terminal attribution.

## Evidence inspected

Developer-supplied focused logs were read, not replaced by a duplicate full-suite run:

- `/tmp/semteams-migration-8b99efe/evidence/ui-contract-green.log`: initial dispatch/component focused gate.
- `/tmp/semteams-migration-8b99efe/evidence/ui-identity-green.log`: 6 files / 167 tests passed, including composition
  service, graph identity, run status/health, slash commands, and AI entity lookup.
- `/tmp/semteams-migration-8b99efe/evidence/baseline-browser/research-mvp/manifest.json`: baseline source, compose
  project `stmig160`, port `33160`, source hashes, and derived configuration hash.

These focused green results predate F1–F3 corrections and did not exercise the failing boundary cases. Full frontend
check/lint/unit/build, regenerated types, isolated target journeys, and an independent review of the fixes are still
required. No SemSource dogfooding or SemEngine consumer switch is approved by this review.

## Intermediate re-review (superseded)

A second independent Svelte reviewer (the backend/technical-writing agent, with no UI implementation authorship)
reviewed the F1–F3 corrections and the new approval-history consumer on 2026-10-01. No UI code was edited by this
reviewer. Approval was withheld at this intermediate point; the final disposition below records their resolution.

F1 now accepts the frozen bare-ID `loop_completed` envelope and retains a completion before its loop metadata.
Stored terminal patches take precedence over later nonterminal SSE/snapshot records; the detail panel exposes
`record_state` disagreement. The existing new store regression covers success and stale ordering; explicit failure
and cancellation envelope regressions were requested to substantiate all three outcomes. F3 now turns HTTP-200
`type:error` command replies into rejected API calls; both UI callers already surface errors, and the focused
boundary test verifies the refusal.

### F2 follow-up — stale asynchronous settlement remains uncorrelated

Resetting the editor on execution replacement fixes posting A's draft as B in the ordinary rerender path. However,
both `PendingApprovalSection.submit` and `RunWaitingSection.submitGateDecision` still apply success/error/finally
state updates without comparing the submitted loop/execution with the currently displayed gate. If A is in flight
when B replaces it, A's eventual response can clear B's state or display “Approval submitted” for B. Capture the
submitted tuple/generation and ignore stale settlements. Deterministic deferred-promise replacement tests must cover
both success and rejection. The reviewer also requested terminal-run history coverage: durable unmatched pending
receipts must not make a cancelled/completed/failed run appear to await an operator forever.

### F4 — P1: legacy flow-generation server endpoints remain reachable

The page routes `/flows` and `/flows/[id]` correctly redirect to read-only composition inventory. Route/layout import
inspection found no active flow editor navigation. However, `POST /api/ai/generate-flow` still constructs a Claude
client, calls the paid model, and then calls the removed `/flowbuilder/flows/{id}/validate` endpoint through MCP.
`POST /api/ai/chat` is also a legacy flow-generation endpoint (requires `currentFlow` and supplies flow-generation
tools). These are reachable server routes, not merely donor components.

Retire the entrypoints explicitly before model/backend work; use route tests proving that no model/network call
occurs. Keep disconnected donor helpers if useful, but do not expose a partial authoring API or replace the retired
framework surface. This finding was sent to the frontend developer and PM; no paid request was executed during review.

The tuple consumer correctly computes pending-minus-answered by exact pair, merges per-subject history, and only
renders/enables a run gate when its loop record carries the matching execution. The final terminal-history behavior,
stale async settlement and retired endpoint fixes remain due. Full unit/build logs currently show 171 files passed,
3725 tests passed and 12 intentionally skipped contract tests, but those outputs predate this follow-up and do not
qualify the missing cases.

## Final frontend re-review disposition

**Approved for the reviewed frontend code on 2026-10-01.** The second reviewer authored no UI implementation.
F1, F2, F3 and F4 are resolved in the inspected source; no further blocking frontend code finding remains.

- F1: canonical bare UUID `loop_completed` envelopes for success, failed and cancelled outcomes are tested before
  loop records and against stale SSE/snapshots. Terminal evidence stays authoritative in the UI; record disagreement
  is visible. This does not claim repair of the framework's terminal-record CAS residual.
- F2: both approval forms capture the submitted loop/execution pair. Success, failure and finally updates check that
  pair; replacement resets/re-enables the form. Deferred A-resolve/A-reject after B arrives preserves B's draft,
  confirmation and error state. Run gates require the graph tuple to match current pending loop execution.
- Approval history: exact pair set subtraction preserves later gates; per-subject reads include durable receipts.
  Completed/failed/cancelled graph phases suppress actionable waits while preserving health facts. Tests cover all
  three terminal phases, unanswered history and cross-execution replacement. This is a UI contract approval, not
  a substitute for the separately approved Go projection/rule review and actual browser boundary proof.
- F3: typed HTTP-200 command errors reject instead of silently acknowledging cancellation. Task and chat callers
  surface the failure. Command acknowledgement remains separate from observed cancellation.
- F4: both flow-generation POST routes return 410 before parsing or model/backend construction. Four route tests
  assert no network/client call. `/flows` and `/flows/[id]` redirect to read-only inventory; route/layout imports do
  not expose donor editors. The old `/api/ai/chat` required `currentFlow` and only supplied flow-generation tools.
  Baseline and current `DataView.handleChatSubmit` are the same no-op; `chatApi.streamChat` and generic server tool
  selection have no non-test callers. Thus the retirement removes no working graph assistant. Graph exploration and
  product `ChatBar` dispatch are retained, while the unwired graph-chat pane is an existing limitation.

Reviewed red-to-green evidence includes seven failing async/terminal-history cases and four failing retired-route
cases, followed by 116 passing focused tests across six files. Root's final sequential frontend gates all exit 0:
ESLint, svelte-check (0 errors/0 warnings), unit tests (171 files passed, one skipped; 3704 tests passed, 12 skipped),
and production build. The reduced test count reflects removal of retired authoring-success tests, replaced by 410/no
network contract tests. [Final gate excerpts](browser-evidence/frontend-final-gates.txt) and focused red/green logs in
`browser-evidence/` preserve the evidence. The reviewer inspected the code and logs without duplicating the final suite.
An accidentally launched duplicate suite was interrupted after root identified the active final run; its interrupted
exit is an orchestration event, not a product gate result.

The final consolidated browser matrix is complete with 20 passes, one autoresearch final-delivery failure and five
explicit skips. The corrected autoresearch emitter proof and actual run-scoped approval boundary pass; final typed
autoresearch delivery remains blocked. #261 evidence-body rendering, stable-channel continuation, SemSource readiness
and SemEngine consumer acceptance remain separate limits. This scoped code approval does not waive them.

## Final browser assertion and runner addendum

The independent frontend reviewer inspected the final E2E-only changes and runner after product source stabilized.
**Approved for the reviewed assertion/harness scope; final matrix outcomes remain separate.** No product/UI source
was edited during this pass.

- `autoresearch.spec.ts` now requires exactly one correlated successful artifact ToolResult, parses its returned
  path/revision/entity, and matches the persisted graph artifact path. The fixture supplies the previously missing
  field. Its artifact provenance token is explicitly static mock data; a separate assertion proves the graph's running
  best experiment names a real kept execute loop whose measured value equals best.value. This does not assert the
  static artifact token is that dynamic graph reference. Strict final typed reply count and terminal-loop correlation
  remain in place and are expected to expose the unresolved delivery blocker. Later run-phase assertions in the test
  are not reached after that failure; retained final-state captures provide separate phase evidence.
- `run-approval-boundary.spec.ts` gates the planner's advertised `emit_plan` after the run exists. It reads the actual
  run owner's exact pending tuple, `awaiting_approval` phase and outstanding count, clicks the UI approval, verifies the
  exact execution sent, then requires durable answered/pending history, zero outstanding, terminal run/loops, removed
  waiting UI and a typed final research response. It writes through public UI/HTTP only. This validates the actual
  projection/rule handoff; deterministic Go tests cover callback ordering/concurrency cases it does not induce.
- `admin-flows-inventory.spec.ts` rejects inventory errors, requires configured component/validation evidence and
  exercises graph navigation on an actual returned entity. It checks canonical identity segments and domain filtering,
  uses the existing selection seam only to select that real ID, observes no `/flowbuilder/` request, and requires 410
  from retired authoring routes. It injects no graph facts. This is graph/identity behavior, not evidence-body rendering.
- The runner checks the exclusively owned project/resource names and blank paid credentials; captures effective
  config, compose, fixture/spec/source/image identities and runner/matrix hashes; uses zero retries; samples concrete
  state/logs/readiness every 25 seconds; tears down only its scoped stack; and marks source mutation as an invalid run.
  Readiness is captured directly inside the backend because the UI proxy lacks that route. Declared skips remain
  explicit. The selected paths in the source hash guard and prebuilt image identities are recorded, not a claim of
  reproducible binary builds or empirical model/sandbox quality.

The final [run-scoped approval proof](browser-evidence/final/run-approval-boundary-proof.json) passes. The
[final matrix](browser-evidence/final/results.json) has 26 rows: 20 passed, one failed and five explicitly skipped; every
row records unchanged source hashes. The sole failure is strict final autoresearch delivery after successful emitter,
artifact-path and running-best proof. The [corrected baseline](browser-evidence/baseline-autoresearch-corrected.json)
also passes emitter proof and fails strict delivery. The frontend-owned [browser report](browser-baseline.md) retains
the exact configurations and attribution. Completed execution of the matrix is not passing migration qualification.
Existing mock/static provenance, #261, graph-chat placeholder and external ecosystem holds remain unchanged.

The final report's nine composition diagnostics are individually attributed in the
[warning ledger](browser-evidence/final/warning-ledger.json). Their warning severity permits boot and offline
validation; it does not prove optional graph-index, community, streaming or watcher capabilities. The graph browser
assertions qualify the routes they exercise only. Cold-start authoritative-view unavailability is retained as an
observed 503 even when backend readiness is 200; no empty-loop-list availability claim is inferred.
