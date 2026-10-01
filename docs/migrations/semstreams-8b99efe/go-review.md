# Independent Go review of the frozen SemStreams migration

Review date: 2026-10-01. Review lane: independent `go-reviewer`; no Go/config implementation authored by this reviewer.
Scope: working-tree changes from `ce22c961` on `codex/semstreams-frozen-8b99efe`, issue #280, draft PR #281.
Framework: `v1.0.0-beta.162.0.20260930150212-8b99efe9c66a` at SHA
`8b99efe9c66a4faa4fa509f9f62cc6bad8392128`.

**Current status: scoped Go code approval, including the autoresearch repair; no unresolved Go code findings.**
The approval-ordering findings and the later SemTeams-only origin return repair are approved. Historical Q1 below
records the original routing failure; it is superseded by the [approved repair review](autoresearch-repair-review.md)
and the focused browser evidence linked from [Go validation](go-validation.md#autoresearch-terminal-repair).
The [repair full gates](go-validation.md#final-repair-go-gates) and hosted implementation CI passed; current lint
reports six warnings and zero errors. Earlier seven-warning gates and source/image hashes below are historical.
The expanded browser matrix and final documentation/archive checks remain separate qualifications; this code review
is not an overall migration-readiness assertion. SemSource-backed dogfooding stays held. A later SemEngine switch
requires its own approved consumer contract.

## Closed findings and framework-alignment decision

### G1 — P1 approval ordering: repaired

The initial loop-only pending/resumed reflection allowed an answer before pending to block later pauses. A delayed
first answer for A could resume B, and predicate-wide cleanup could erase a new gate. The reviewer independently
reproduced those failures against the exported Pauser before the repair. The durable
[ordering red](evidence/go/approval-order-red.log) covers the event-order failures.

The attempted set-cardinality rule also failed the frozen matcher: right-hand `.length` substitution counts the first
triple object's list elements, while `length_*` evaluates matching triples. The
[count-design red](evidence/go/approval-count-design-red.log) remains red evidence regardless of its original temporary
filename. It is not a passing alternative.

The accepted replacement is one product-specific `RecordApproval` operation on an existing run. It exact-reads the
entity and same-entry revision, computes pending/answered JSON pair sets and their outstanding count, then reconciles
only these three predicates with that exact revision. Answers insert both pairs; existing tuple annotations remain
unchanged. Competing writers recompute after a definite revision mismatch, bounded to eight attempts and five seconds.
No callback ordering or local mutex is treated as cross-process authority.

The design uses the existing public typed framework protocol:

- `graph/mutation_requests.go` exposes `ReconcilePredicatesRequest.ExpectedRevision`; frozen ADR-091 and
  `processor/graph-ingest/spec.md` document the canonical mutation operation.
- `processor/graph-ingest/canonical_mutations.go` remains the mutation authority and owns atomic revision checks/CAS.
- `graph.NewExactEntityReader` supplies validated entity bytes and their same-entry revision. The adapter also checks
  effective local authority, canonical run identity and `lifecycle.harness.v1` before mutation.
- `pkg/projection.MutationClient.Reconcile` reads a revision after receiving Desired; it cannot fence a Desired set
  computed from the caller's earlier snapshot. This public convenience-API gap does not remove the public wire contract.
- The fixed `projection.Contract` declares exactly the three approval predicates. It validates intent, not exclusive
  ownership. The operation enforces its fixed group and never writes lifecycle, loop admission or entity-birth fields.

No generic internal client was copied and no raw KV access, bucket, stream, worker, payload type or new rule action was
added. The approved framework survey and migration posture are recorded in the
[ADR-029 addendum](../../adr/029-product-shell-wiring.md#addendum-2026-10-01-execution-correlated-approval-projection).
Adopt an upstream equivalent when it can preserve the caller's exact revision and these ordering guarantees.

Rules 12 and 13 each execute one framework lifecycle transition. Resume requires zero outstanding gates, the exact
reserved pause reason, source `rule`, previous phase `executing`, and no clarification marker. Phase and audit are
written together; no marker cleanup can erase B. A selected resume may briefly precede B's pause, but retained history
allows convergence. Terminal phases cannot be resurrected by these rules. The approval request still uses the live
loop's exact `pending_approval.execution_id`; run projection facts never authorize tool execution.

### P2 protocol refusals and revision receipts: repaired

Review identified definite server/no-responder refusals incorrectly classified as unknown commits, and success replies
accepted without checking their revision relation to the caller's fence. The
[behavioral red](evidence/go/approval-projection-review-red.log) precedes the repairs.

The final adapter preserves classified error code/class and marks definite refusals as `CommitNotCommitted`.
It rechecks cancellation before sending. Applied receipts must advance the expected revision; unchanged receipts must
match it exactly. Identity, type, request ID and selected facts must also match. Invalid replies and ambiguous transport
errors remain commit-unknown and are not retried or inferred successful from matching stored content. A legitimate
unchanged answered replay is covered and accepted.

### Boundary evidence gaps: closed for the changed paths

The unused interim `ReadPredicateValues` helper was removed. Live NATS tests now exercise the actual production
constructor, frozen exact reader and typed mutation boundary. They fail closed on missing/wrong envelopes, zero
revision, malformed history, classified query failures, invalid mutation receipts and cancellation. These tests use a
protocol graph-owner fixture; the browser proof below separately exercises the configured graph owner.

Selected-run command tests retain frozen `errs.ClassifiedError` codes for `invalid_loop_id`, `loop_not_found`,
`loop_owner_absent`, `loop_record_invalid` and transient `loop_state_unavailable`. Nil lookup, a different returned loop
and missing owner refuse without graph read/write. Error wrapping preserves `errors.As`, code and classification.
This qualifies owner lookup without activating the parked implementation lane.

## Initial compatibility scope reviewed — historical snapshot

- Bootstrap constructs one public rule ConfigManager, registers its key family before configuration startup and shares
  it with tool executors. The product rule-config service mirrors the frozen composition-root adapter through public
  APIs. It starts after components and stops before them; readiness includes initial rule reconciliation. Tests cover
  rollback, nil/repeated lifecycle calls, bounded Stop, processor hot reload and explicit synchronization.
- Runtime context remains live through StopAll; shutdown receives a fresh bounded context. Effective Platform.ID after
  configuration startup is supplied to local writers. Canonical entity system/domain ordering replaces the old ordering.
- Retired flow engine/store/template dependencies, manager wiring, seed loader and CLI seed flag are removed. Schema
  generation expects composition Result/Graph and rejects the retired flow service. No substitute flow framework is
  introduced.
- Public loop/lesson projection contracts replace copied definitions. Builtin payload registration remains injected;
  reserved user-response output is typed and flat publishers are removed. Vocabulary numbers use int/float declarations.
- Both shipped bootstraps retain the live 42-file rule set; parked packs remain unwired. Loop KV output and the tools'
  declared loop-read port are used. Composition contract tests cover both shipped bootstrap graphs.
- Selected-run authorization uses current-owner lookup. Sandbox attestation IDs are canonical; there is no new host
  execution fallback. Empirical sandbox qualification remains a browser evidence responsibility.

Frozen approval timeout persists the timed-out result before publishing its execution-correlated response. Omission
means a 12-hour timeout, not indefinite waiting. This follows the same answer projection and needs no uncorrelated
resume marker. It does not establish restart recovery: Core-NATS event loss remains an existing limitation.

The new receipt format requires fresh NATS/graph state. Retained beta.160 loop-reference markers are incompatible;
no dual reader, retained-state conversion or production wipe is authorized or claimed.

## Pre-repair validation and source identity — historical snapshot

This section retains the initial approval-projection qualification before the autoresearch return repair. Its
seven-warning count, 42-rule composition and image/source identities are not the current repair snapshot. Use
[final repair Go gates](go-validation.md#final-repair-go-gates) for the six-warning result and current evidence.

The reviewer inspected recorded full-suite evidence and independently ran focused regressions instead of duplicating
all infrastructure runs. Historical gate manifest:
`/tmp/semteams-migration-8b99efe/evidence/final-go-gates.json`.

| Historical local gate | Result |
| --- | --- |
| `task lint` | Exit 0; 0 errors, 7 warnings (baseline: 5 warnings) |
| `task test:race` | Exit 0 |
| `task test:integration` | Exit 0; atomic coverage profile recorded |
| `go build ./...` | Exit 0 |
| `task schema:generate` | Exit 0 |

Integration used `GOFLAGS=-coverprofile=/tmp/semteams-migration-8b99efe/evidence/final-go.cover -covermode=atomic`.
Other final gates had empty GOFLAGS. Their sibling `final-go-*.log` files record output. The backend build manifest
`final-backend-build.json` reports unchanged before/after source SHA-256
`92ba491c1199aad6fc7b08bdd969047513a6fb2ef107e3d6003850793c122f28` and image
`sha256:6aba6492ddd7da32387ffd4fbb46c3414f2fb2949b02afe89fa3f2defab88ed7`.

Focused [race/integration evidence](evidence/go/approval-projection-race.log) and its exact command/environment are in
[Go validation](go-validation.md#approval-follow-up-evidence). Deterministic tests execute the frozen rule matcher and
action executor with lifecycle persistence fixtures, including answer-first, failed pending then answer, late A while B
waits, selected pause/resume races, distinct concurrent Pauser instances, replay, terminal guards and unrelated-field
preservation. Subscription readiness uses flush and concurrency tests use explicit channels rather than sleeps.

The reviewer independently ran these focused commands successfully with `-count=1`:

```bash
go test ./cmd/semteams/commands/implementspec \
  -run 'TestExecute_(PreservesClassifiedOwnerLookupFailures|RefusesUnavailableOrMalformedOwner)' -count=1
go test -race ./cmd/semteams/approvalpause \
  -run 'TestProjectionConcurrentWritersPreserveBothGates|TestApprovalRules' -count=1
go test -race ./cmd/semteams/approvalpause \
  -run 'TestProjection(BadMutationReceipts|DefiniteRefusals|DuplicateAccepts|ConcurrentWriters)|TestApprovalRules' \
  -count=1
```

Focused coverage is 80.5% for approvalpause and 85.0% for implementspec. RecordApproval is 88.5%, tuple computation
92.0%, anchor-to-projection dispatch 94.1% and selected-run authorization 100%. Broad boot/subscriber coverage is not
asserted by those figures. The [function report](evidence/go/approval-projection-functions.txt) retains the limits.
The new live boundary pins NATS 2.14.4; the existing chain helper uses 2.14-alpine, resolved to 2.14.7 in that run.
The browser uses pinned 2.14.4. This distinction is documented, not treated as an identical environment.

The actual-owner browser proof is
`/tmp/semteams-migration-8b99efe/evidence/target-final/run-approval-boundary/`: one Chromium test passed, zero retries,
source unchanged. Its spec and attachments prove a real planner `emit_plan` gate after run birth, awaiting phase with
one exact pending tuple and outstanding=1, a UI answer for that same execution, retained answered/pending tuples with
outstanding=0, a completed research run and one typed final user response. The manifest records the image above,
`stmig162`, port 33162 and exact derived configuration/compose hashes. It closes the deployed graph-owner/rule boundary
gap; the rest of the final browser matrix was still running at this review's signoff.

## Q1 — historical adoption blocker, superseded by the approved origin return repair

Before the repair, the empirical autoresearch run could complete without delivering the final coordinator reply
to the initiating channel. This was distinct from evidence-body rendering limitation #261. The observations below
are retained red evidence, not a current unresolved code finding.

The reviewer inspected `/tmp/semteams-migration-8b99efe/evidence/target-browser/autoresearch/final-state.json`:
sequence 157's synthesize task lacks both `parent_loop_id` and `run_id`, though related-loops metadata survives.
Sequence 182's reviewer and sequence 199's coordinator have descendant parents but no run. Only the initial user
acknowledgement is captured. See the [blocker](autoresearch-terminal-blocker.md) and
[reduced wire evidence](autoresearch-route-evidence.json).

Frozen `processor/rule/actions.go:1799–1886` derives ParentLoopID only from a loop-execution firing entity and inherits
RunID through `agent.loop.run`. Rule 05 fires from the run entity. Frozen
`processor/agentic-dispatch/terminal_settlement.go:393–397` explicitly classifies severed non-loop ancestry as
`route_less_settled`; the typed route resolver does not use arbitrary related-loops metadata. No supported explicit
parent/run override preserving this run-triggered product protocol was found.

The subsequent SemTeams-only repair preserves the run-local iteration state and uses existing rule facts to return
through the authoritative origin coordinator. It supplies native ancestry without stamping `agent.loop.run` onto a
run, adding a response publisher shim, changing the frozen SHA or requiring a SemStreams change. The
[repair review](autoresearch-repair-review.md) records current approval and bounded guarantees. The
[historical blocker record](autoresearch-terminal-blocker.md) links the reproduced failure to its repair evidence;
completed loops alone were never accepted as proof of user delivery.

## Initial lint warning disposition — historical snapshot

The pre-repair lint run reported zero errors and seven warnings; the beta.160 baseline had five. Its two additions were in
`cmd/semteams/approvalpause/projection.go`. They are reviewed low-priority maintainability findings, not preexisting
warnings or unresolved behavioral defects. Scoped code approval remains valid with these explicit dispositions,
tracked with [migration #280](https://github.com/C360Studio/semteams/issues/280):

- `redefines-builtin-id`, line 202: the local `copy` variable shadows the builtin only within the outstanding-fact
  branch. No builtin call occurred there, and taking its address preserved the existing fact's annotations. The
  approved autoresearch repair renamed it to `existingCount`; the warning is resolved in the current six-warning gate.
  Its initial deferral preserved the earlier source/image qualification snapshot, which remains historical evidence.
- `function-length`, line 65: `RecordApproval` has 52 statements against a threshold of 50. Accepted without a required
  refactor: the function keeps the exact read, revision fence, bounded retry and receipt classification together.
  Extracting a generic mutation helper solely to meet the count would obscure this domain boundary. Revisit structure
  only if behavior grows or the upstream public projection API removes the adapter.

Neither warning was suppressed. The function-length disposition remains current; the shadowed-variable finding is
resolved. These dispositions waive no failed gate or behavioral defect.

## Initial framework footprint method 2 review — historical snapshot

The table below records the earlier method-2 snapshot. The repair adds rule references and tests; the current
matched measurements and their method limits are in [framework footprint](framework-footprint.md). Do not treat
the historical table as the current dependency total.

The script and both final method-2 JSON reports were independently inspected. Declaration file/line/column prevents
same-named fields from merging. Direct import sites, production/test reference positions, package/module closures,
full module graph edges and a separate parked build context are recorded. Runtime, default-test and integration
contexts have no compiler errors in either report.

| Comparable measure | beta.160 | Frozen target |
| --- | ---: | ---: |
| Runtime referenced declarations | 296 | 326 |
| Runtime defining framework packages | 31 | 29 |
| Runtime framework package closure | 95 | 100 |
| Runtime complete package closure | 577 | 576 |
| Runtime module closure | 68 | 65 |
| Runtime direct import sites | 481 | 485 |
| Default/test referenced declarations | 421 | 467 |
| Integration referenced declarations | 424 | 513 |
| Module requirement graph edges | 724 | 718 |

Both parked contexts reproduce `recordingPublisher` lacking `Append` in the existing create-change fixture. That lane's
output is partial diagnostic evidence, not a successful compiled obligation set. The measurement does not enable or
repair parked packs. Script exit zero alone is insufficient; inspect every scope's errors and module graph error.

These are macOS arm64, Go 1.26.4 selected build-context results, not linker reachability or all-platform closure.
`Call=true` denotes a syntactic call target, including conversions; it misses later indirect function-value calls and
is not an exact API-call census. Import counts are source declaration sites, not unique dependencies. Registrar closure
and compiled donor code do not prove active product requirements or expand SemEngine's first release. Use the matched
[baseline](framework-footprint-beta160-v2.json) and [target](framework-footprint-frozen-v2.json) reports.
Earlier method-1 counts are historical and must not be used for the final comparison.


## Autoresearch return repair review

The follow-up SemTeams-only origin return rules, declared reply-source facts, cap-above-three compatibility change
and associated Go contract tests received scoped implementation approval. The reviewer authored the design but did
not implement the reviewed production rules or tests. The [repair review](autoresearch-repair-review.md) records
resolved exact-one anchor guards, native child admission for terminal runs, the narrow idempotent-Mint exception,
independent race/integration results and the preserved limitation-reply lifecycle policy. Actual browser delivery
and final full gates remain separate qualification requirements; the earlier blocker history is not erased.


## Final API consistency check: result lookup accepts full loop entity IDs

The new origin-return prompts pass the full canonical source loop entity ID to `read_loop_result`. This matches
its frozen public contract. `processor/agentic-tools/loop_result.go:70-72` explicitly advertises both bare UUIDs and
full six-part entity IDs. The executor normalizes the input before reading `COMPLETE_<uuid>`; lines 164-186 define
that normalization. `loop_result_test.go:343-374` exercises a canonical full entity ID through the executor and
asserts both returned content and normalized loop metadata.

The reviewer independently ran that existing frozen regression from the product's pinned module graph:

```bash
go test github.com/c360studio/semstreams/processor/agentic-tools \
  -run '^TestReadLoopResultExecutor_FullEntityIDLoopArg$' -count=1
```

It passed in 0.342 seconds on Go 1.26.4, darwin/arm64. The
[retained output](evidence/go/repair/full-entity-loop-result-review.log) records the result. This is executor-contract
proof using the upstream test fixture; it does not claim the final-only-`decide` mock coordinator exercised a real
result-lookup tool call. No rule change or SemStreams change is needed. The earlier replay, Mint fallback,
cancellation-ordering and limitation-reply qualifications remain in force.
