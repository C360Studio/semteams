# Autoresearch repair independent Go review

Reviewer: `go_reviewer`, independent of the implementation authors. Scope: the SemTeams-only terminal handoff repair
on `codex/semstreams-frozen-8b99efe`, against frozen SemStreams `8b99efe9c66a`.

## Design decision

**Approved for implementation, subject to the checks below.** The existing run-local iteration driver and accumulator
remain authoritative. A category-specific, append-only terminal fact can cross from the completed run to its actual
origin coordinator loop through ordinary `add_triple` rules. That loop can publish the final coordinator using the
framework's native parent and run ancestry. This uses existing primitives and requires no SemStreams change, response
publisher shim, fabricated framework predicate, new subscriber, new tool, queue, stream, or storage bucket.

The review rejected merely changing rule 05 to fire on transient child loops: that would reset its iteration counter
and remove access to the run-local cap and journal. The review also rejected plain run inheritance at the origin:
a recovery coordinator can carry an inherited run anchor before its own new anchor, and first-value inheritance would
choose the older run.

Using `run_scope: new` at the actual origin is an idempotent assertion of the existing origin-rooted run. Frozen
`agentic/agentrun/agentrun.go:248-345` explicitly implements create-or-retrieve and compares the stored origin before
returning an existing record. `pkg/lifecycle/manager.go` returns `ErrAlreadyExists` for a lifecycle-managed entity;
there is no transition back to `dispatched`. This preserves the run identity and current phase while assigning the
child's typed `RunID` from the firing origin, even when its graph contains inherited-first anchors.

## Required implementation and qualification checks

- The run bridge requires exactly one origin and one terminal fact. The origin must equal the canonical local loop
  entity at the run's instance; malformed, foreign-authority, or different-instance origins must not forward a fact.
- The run bridge must use the appropriate lifecycle phase. Approved delivery waits for `completed`. Failure reporting
  waits for `failed` and excludes budgeted execute failures. Clarification preserves the existing active-run semantics.
- The origin rule is scoped to the coordinator's `autoresearch` decision and one unambiguous category terminal fact.
  It must not publish through another run's origin or resurrect cancelled work.
- Real action execution must prove `ParentLoopID` and `RunID` name the intended origin despite an inherited-first run
  anchor. Existing `completed`, `failed`, and `awaiting_approval` run identity, origin, and phase remain unchanged.
- Direct `agentrun.Mint` tests must prove stored-origin mismatch is refused. This must not be misrepresented as a
  stronger task-publication guarantee from the rule action; see the existing limitation below.
- Exercise duplicate facts, stale revisions and a recreated state tracker against durable state. State the observed
  restart guarantee precisely. Do not introduce automatic replay of terminal publication without idempotency.
- Preserve cap exhaustion, clean and failed execute accounting, kept-best promotion, late completion suppression,
  cancellation, and approval boundaries. Qualify the final typed user response in the real mock browser journey.
- Record tests before and after the change and obtain final independent review of the resulting rules and tests.

## Existing framework boundaries

The frozen `publish_agent` action logs a Mint failure, clears the task's `RunID`, and still publishes with its native
`ParentLoopID` (`processor/rule/actions.go:1987-1993`). The authored bridge must validate the origin before forwarding,
so a mismatched origin cannot arise from a well-formed configured handoff. A direct Mint refusal does not establish
that the action refuses publication under arbitrary corrupted or transient lifecycle state. Parent ancestry remains
the existing terminal resolver's supported fallback. No product shim or upstream repair is required by this design.

The stateful evaluator persists match state after executing actions. Successfully persisted state suppresses duplicate
and stale replay, including after restart. It does not provide a transaction spanning task publication and state
persistence; a process crash in that interval is not an exactly-once guarantee. The repair must not claim otherwise.

A clarification action selected before cancellation has the existing rule-action ordering boundary; a run-phase guard
is not a transaction across entities. Completed-success delivery is gated by the terminal completed phase. The repair
must not claim that a delayed active-phase marker is a new cancellation synchronization primitive.

## Implementation review

The implementation review is performed by the architect acting in the Go-reviewer role. This reviewer authored
contracts and documentation, but did not implement the rules, vocabulary or tests. The independent initial design
review above remains part of the evidence. Scope is the repair relative to `8656192c`, including the three reply
facts, source rules, six relay/publication rules, explicit repeating-action cap, narrow registration fences and the
cosmetic approval-projection local-variable rename.

The review requested exact-one run-anchor guards on all new source-fact writers, matching the approved reviewer
source. Rules 10b/12/13 now refuse ambiguous anchors rather than selecting their first value; the focused negative
regression covers each. The registration fence permits `run_scope: new` only at the original category mint points
and the three named origin-return rules. Coordinator-spawn classification separately checks the origin role,
category decision, explicit self-run assertion and lack of conflicting inherited lineage. Downstream rules retain
the existing inheritance fence.

### Admission of reporting children after a terminal run

The frozen loop admission path permits the reporting coordinator to be associated with a completed or failed run.
`processor/agentic-loop/component.go:1929-1956` validates the task and lineage batch without reading run lifecycle.
`handlers.go:881` (`HandleTask`) checks context cancellation, depth and task/loop duplication, creates the fresh
loop and builds its model request. `configureLoopMetadata` at lines 548-564 assigns native parent/run fields;
`state.go:1692-1703` (`SetRunID`) stores that association without a run-phase gate. The run is an association here,
not a request to transition its lifecycle. The added admission regression calls the real frozen `HandleTask` on
published approved/failed reporting tasks and verifies fresh work is admitted while the run phase remains terminal.
This complements the actual browser journey; a `Mint` assertion alone would not prove loop admission.

### Preserved limitation reply lifecycle

An autoresearch clarification coordinator choosing `respond_direct` still leaves its run `executing`. This is
pre-existing, explicit product policy, not a new branch introduced by the repair. Agent-run rules 03 and 04 reserve
completion for an approved success outcome and failure for involuntary failed/truncated work. Rule 04 explicitly
excludes limitation replies from failure; coordinator rule 03b only records reply prose. ADR-053's existing outcome
policy and `configs/rules/agent-run/README.md` already describe the non-terminal direct reply. The new origin return
preserves the old 10b allowlist and lifecycle behavior while restoring typed routing and current-run association.
`ask_user` continues through the existing pause/resume rules. Changing the direct-limitation disposition requires
a separate product decision; silently marking it completed, failed or cancelled would change that contract.

### Review validation

**Approved for the scoped Go/config implementation. No unresolved correctness finding remains in this review.**
The final independent race run passed after the anchor-ambiguity correction and admission regression. It includes
native origin/current-run identity, phase/origin guards, idempotent Mint, cap above three, persisted/stale replay,
source terminal boundaries, ambiguous source-anchor refusal, actual terminal-run child admission, and the narrow
registration/coordinator-spawn fences.

```sh
go test -race -tags integration -p 1 ./test/contract \
  -run 'TestAutoresearch(Reply|IterationCapAboveDefaultActionLimit)|TestRunScopeMintPointsAndLifecycleTransitions|TestAgentRunPack_CoordinatorSpawnCoverage|TestAnchorSplitSiblingsKeepContractInSync' \
  -count=1
```

Environment: Go 1.26.4, darwin/arm64; isolated testcontainers NATS server explicitly pinned to 2.14.4. Result:
`ok github.com/c360studio/semteams/test/contract 4.772s`. The [retained log](evidence/go/autoresearch-independent-review.log)
records completion. `git diff --check` also passed. The integration fixture uses the real frozen matcher, action
executor, Mint, HandleTask and durable NATS StateTracker; graph/lifecycle storage and task publication are fixtures.
It does not substitute for the separately required actual graph-owner/browser delivery evidence.

The approval-projection change is a local variable rename only. The review did not change production code or tests.
Browser delivery and the complete migration gates remain separately owned qualification evidence; this approval
must not be represented as a passing browser result or authorization to close the issue.
