# Go migration validation snapshot

The final local Go gates pass after the autoresearch origin-return repair. Both the
[original migration review](go-review.md) and [repair review](autoresearch-repair-review.md) approve their scoped code.
All five focused repaired browser cases and all 24 active scenarios in the expanded full matrix pass; five explicit
skips remain. Browser evidence is recorded separately in the
[browser report](browser-baseline.md). Intermediate red/green and pre-repair gate snapshots remain development history.

## Baseline and environment

- Baseline commit: `ce22c961d30014c463a09f8f8a2a90044ee1a1cf`, SemStreams `v1.0.0-beta.160`.
- Frozen target: `v1.0.0-beta.162.0.20260930150212-8b99efe9c66a`, SHA
  `8b99efe9c66a4faa4fa509f9f62cc6bad8392128`; no replacement or upstream shim.
- Dedicated worktree: `/Users/coby/.codex/worktrees/semstreams-frozen-migration/semteams`, branch
  `codex/semstreams-frozen-8b99efe`. Proposal commit: `4300f6c8`; initial qualified source is in `f70537c8`, historical
blocked qualification in
  `8656192c`, and the repaired implementation in `edf87d3104d4b5cc494623c89b17742539951380`.
- Baseline Go source preserved at `/tmp/semteams-migration-8b99efe/baseline-source`.
  Later browser comparisons deliberately amended selected E2E specs/fixtures in that directory (observer
synchronization,
  two-gather proof and corrected autoresearch emitter inputs). It is not an immutable whole-directory archive.
  Baseline Go source/module remain unchanged; each browser run's manifest records its exact amended inputs.
- macOS arm64, Go `1.26.4` (module minimum `1.26.3`), Task `3.51.1`, local Docker Desktop.
- Integration uses fresh testcontainers and test cleanup, `TESTCONTAINERS_RYUK_DISABLED=true`, sequential
  package execution (`-p 1`). The unchanged upstream `NewTestClient` defaults to `nats:2.14-alpine`
  (beta.160 `natsclient/test_client.go:589`; frozen line 598). The cached local image reports **2.14.7**.
  The browser lane explicitly uses **2.14.4-alpine**; new approval and autoresearch behavioral fixtures explicitly
  pin NATS **2.14.4**. These are different runtime lanes; the complete Go gate results
  must not be described as NATS 2.14.4 qualification. No paid LLM or production NATS storage was used.
- Both `configs/flow-bootstrap.json` and `configs/e2e-flow-bootstrap.json` are decoded and composition-validated.
  Runtime behavior uses the browser lane's isolated mock configuration; see that lane for exact compose settings.

Initial restricted Go-cache/Docker access required an environment-permission retry. That retry is not a
product failure. The original baseline commands completed before changing the dependency.

## Exact module and runtime identities

[Module download identity](evidence/runtime/frozen-module.json) records the complete source SHA, module sum
`h1:WAlGLMjE/3EnYyeoqounCfYnVyIB3igMlG+FLP1Iq+o=` and go.mod sum
`h1:C3/wpNLTLCifyqkNx5j8cpxvQiCIqD8/YAOlKtkFv2o=`; both match `go.sum`.
The origin is `https://github.com/c360studio/semstreams` at
`8b99efe9c66a4faa4fa509f9f62cc6bad8392128`. No replacement is present in `go.mod`.

[Runtime image/version evidence](evidence/runtime/nats-identities.json) retains requested tags, immutable image IDs,
repository digests, architecture and reported server versions. Go's cached `2.14-alpine` resolves to
`sha256:4063edae0717ba5f7501bfde75f97fd9b57f5b93597b92c70b6a6fbbf6a74e06` (2.14.7); browser
`2.14.4-alpine` resolves to `sha256:f2123f533c2b0cada0a5c5ec434fb2b8cfe1cf220215ef9d7517e1372917ad66`.
The floating Go helper tag was not changed for this migration. A later machine must check its resolved digest/version
rather than assuming this cached-image result. This documents a preexisting reproducibility limit, not a proven
behavioral defect or a reason to relabel the existing gate.

## Pre-repair full Go snapshot

[Machine-readable commands and exits](evidence/go/final-go-gates.json) records these outcomes after the approval
projection and final tests were complete:

| Command | Exit | Durable evidence |
|---|---:|---|
| `task lint` | 0 | [Lint log](evidence/go/final-go-lint.log) |
| `task test:race` | 0 | [Race log](evidence/go/final-go-race.log) |
| `task test:integration` | 0 | [Integration log](evidence/go/final-go-integration.log) |
| `go build ./...` | 0 | [Build log](evidence/go/final-go-build.log) |
| `task schema:generate` | 0 | [Schema generation](evidence/go/final-schema-generate.log) |

The integration gate adds `GOFLAGS=-coverprofile=/tmp/semteams-migration-8b99efe/evidence/final-go.cover
-covermode=atomic` to the unchanged `go test -race -count=1 -tags=integration -p 1 ./...` task. Other listed commands
have empty `GOFLAGS`. The unchanged testcontainer helper and local NATS 2.14.7 identity are described above.

That historical lint snapshot had zero errors and seven warnings. Five were baseline warnings; the two additions
were the cohesive approval projection function length and a local variable named `copy`. Independent review accepted
the 52-statement projection function. The subsequent approved repair renamed the local to `existingCount`, so the
current gate has six warnings. No warning was suppressed. Historical generated-output checks at `f70537c8` passed.

## Coverage snapshot before the autoresearch repair

The pre-repair full integration coverage profile reports 72.1% overall. The repair full gates did not request a new
coverage profile; the values below describe that measured snapshot, not coverage of the new rule handoff tests. The
durable
[function summary](evidence/go/final-go-functions.txt) avoids committing the large raw profile.

| Package | Final statement coverage |
|---|---:|
| `cmd/semteams` | 17.0% |
| `commands/implementspec` | 85.0% |
| `approvalpause` | 80.5% |
| `chain` | 76.0% |
| `chainpause` | 55.3% |
| `sandboxmanager` | 81.0% |
| `sandboxruntime` | 100.0% |

The changed approval projection's `RecordApproval` reaches 88.5%, approval fact validation 92.0%, pair validation
80.0%, and run/entity validation 100%. Selected-run authorization, effective-platform extraction, rule-service
Start/Stop and shutdown are 100%; rule-service registration is 83.3%. Approval response wrapper coverage remains
75%, and subscriber Start is not exercised in this Go profile. The complete approval package meets 80%, but these
numbers do not mean every function or all product-shell wiring meet that threshold. Deterministic ordering/concurrency
regressions and the real isolated browser gate supplement statement coverage; browser execution is not included in
this Go profile.

## Early required Go gates (superseded target snapshot)

| Command | beta.160 | Frozen snapshot | Evidence |
|---|---|---|---|
| `task lint` | exit 0 | exit 0 | `evidence/go/semteams-{beta160,frozen}-go-lint.log` |
| `task test:race` | exit 0 | exit 0 | `evidence/go/semteams-{beta160,frozen}-go-race.log` |
| `task test:integration` | exit 0 | exit 0 | `evidence/go/semteams-{beta160,frozen}-go-integration.log` |
| `go build ./...` | exit 0 | exit 0 | `evidence/go/semteams-{beta160,frozen}-go-build.log` |
| `task schema:generate` | exit 0 | exit 0 | `evidence/go/semteams-{beta160,frozen}-schema-generate.log` |
| `task schema:check-changes` | exit 0 | historical pending; final check passed | `evidence/go/semteams-frozen-schema-check.log` |

`task test:race` executes `go test -race -count=1 ./...`. `task test:integration` executes
`go test -race -count=1 -tags=integration -p 1 ./...` with the environment above. The final lint, race and build
were repeated after adapter edge-case tests; the focused integration coverage run below includes those tests.
The full integration gate had already passed before those test-only additions and the timeout-metadata correction.

Lint retains five existing warnings, with no errors. The first frozen full race run failed on old
`flow-service` generator assertions. These were changed to assert retired service absence and supported
component/composition response types; the full race rerun passes. The schema command generated changed
framework schemas/OpenAPI, and the orphan retired workflow schema was removed. The initial dirty-tree schema gate
exited Task status 201 (inner status 1) before those generated changes were committed. The later committed checks
below passed; the early failure is retained as history.

## Behavioral tests and attribution

| Contract | Observed red / baseline difference | Adaptation and green evidence |
|---|---|---|
| Typed user response | Two flat coordinator publishers and untyped dispatch ports fail new contract tests | Remove flat publishers; declare `agentic.user_response/v1`; frozen race gate passes |
| Strict composition | Both configs reject retired `platform.instance_id`; next validation rejects `loops_bucket` and undeclared external input | Single platform authority, named KV ports, external `user.message` input; composition passes |
| Loop authority | Existing helper returns empty bucket for declared `agent_loops` KV input | Resolve the typed port's bucket; custom named bucket regression passes |
| Caller-owned shutdown | New lifecycle probe initially fails before helper implementation | Start context remains live; Stop receives fresh bounded context; errors propagate; probes pass |
| Effective identity | Frozen config generates the effective platform suffix during Start | Integration reads the post-Start config snapshot and checks canonical loop/run IDs |
| Rule admission/reload | Upstream ConfigManager API changed to config key-family ownership | Adapter seeds/reconciles inside StartAll readiness; real NATS CRUD reload and bounded Stop tests pass |
| Authority lookup | Removed in-memory LoopTracker API | Durable `LookupLoopOwner` authorization fails closed; implementspec tests pass |
| Approval replay | Repeated answered execution can re-pause a resumed run | Revision-fenced paired sets plus atomic outstanding projection pass ordering/race/live boundary tests; browser qualification tracked separately |
| Sandbox | Canonical run IDs change literals; sandbox policy unchanged | Existing admission, attestation and execution-boundary tests pass |

Red logs are preserved under [evidence/go](evidence/go): `response-red`, `composition-red`,
`composition-step2`, `loops-bucket-red`, and `shutdown-red`. Green logs include `composition-step3`,
`bootstrap-green`, `bootstrap-integration`, `rule-service-green` and the final full race/integration runs.
The shutdown red is a missing-helper compile failure before implementation; it is not proof of a preexisting
runtime defect. Identity and rule lifecycle integration tests supply behavioral evidence, without claiming
that their initial introduction was an independently captured semantic-red run.

Flow loader removal, public projection contracts, canonical entity position order, strict config fields,
caller-owned lifecycle, rule key-family management, and typed responses are intended upstream changes.
Vocabulary `number` was replaced with supported `float`/`int`. Parked packs remain unwired. Full registrar
compilation is not proof that every compiled capability is part of the live product contract.

## Earlier critical-path coverage (superseded snapshot)

Initial pre-repair snapshot command (exit 0):

```bash
TESTCONTAINERS_RYUK_DISABLED=true go test -race -tags=integration -p 1 \
  -coverprofile=/tmp/semteams-frozen-critical-integration.cover \
  ./cmd/semteams ./cmd/semteams/commands/implementspec \
  ./cmd/semteams/approvalpause ./cmd/semteams/chain ./cmd/semteams/chainpause \
  ./cmd/semteams/sandboxmanager ./cmd/semteams/sandboxruntime
```

| Package | Statement coverage |
|---|---:|
| `cmd/semteams` | 17.1% |
| `commands/implementspec` | 84.1% |
| `approvalpause` | 68.7% |
| `chain` | 62.0% |
| `chainpause` | 55.3% |
| `sandboxmanager` | 81.0% |
| `sandboxruntime` | 100.0% |

Changed rule-service Start/Stop and shutdown helpers reach 100%; rule-service registration is 83.3%,
effective-platform extraction 100%, implementspec authority lookup 87.5%, sandbox admission 89.4%.
The initial ordering bug invalidated that snapshot as approval qualification; repaired boundary evidence follows below.
The aggregate critical-path standard is not claimed met merely from passing tests or these selected functions.
Broad boot wiring and subscriber paths remain under-covered. Full function coverage is preserved in
[critical-functions.txt](evidence/go/critical-functions.txt).

## Findings from the initial review

1. **Approval ordering, P1 repaired:** independent review reproduced answer-before-pending and delayed first-answer
   defects in the initial receipt implementation. The replacement uses exact-revision graph reconciliation of
   execution-specific paired sets plus their outstanding count. Deterministic ordering, concurrent writers, terminal
   guards and live NATS boundary tests now pass. Independent code review is approved, and the
   [actual graph-owner browser proof](browser-evidence/final/run-approval-boundary-proof.json) passes through the
   configured projection, lifecycle rules, exact UI execution fence and typed research delivery.
2. **Autoresearch terminal delivery, repaired:** the historical final coordinator lacked typed ancestry back to
   the root HTTP route after the non-loop run-triggered action. See the
   [historical failure and qualified repair](autoresearch-terminal-blocker.md) and original
   [wire evidence](autoresearch-route-evidence.json). The SemTeams-only origin return restores delivery in all five
   focused browser cases. No publisher shim, unsupported run predicate or upstream change was added.
3. **Coverage and final gates:** focused regression/coverage evidence below supplements the final full Go gates
   recorded in the final repair section below. Both independent code reviews are approved. Schema/frontend
generated-output dirty-tree
   verification passes after the content commit; hosted CI remains a separate PR gate.
4. **Product truth:** existing evidence-body rendering is still limited; evidence availability in graph or
   trajectory APIs is not a claim that the UI renders its full body. SemSource dogfooding stays held, and a
   later SemEngine adoption needs its own approved consumer contract.

The frozen sweeper persists a timeout response and publishes matching ApprovalResponse. A timeout answer therefore
uses the same execution-correlated projection as an explicit response; no separate resume marker is required.

## Approval follow-up evidence

The [ordering red](evidence/go/approval-order-red.log) reproduces answer-before-pending, failed pending mutation then
answer, delayed A while B waits, and gates arriving during selected pause/resume actions. The attempted multivalue
`.length` comparison also remained [red](evidence/go/approval-count-design-red.log): frozen substitution counts a first
object's list length, not all predicate values. Both mechanisms were replaced. The domain projection's first
[test-before-code output](evidence/go/approval-projection-red.log) is a compile red for the absent adapter, not a
separate
runtime reproduction. The ordering red provides the behavioral reproduction.

Independent review then identified definite refusal classification and response-revision validation gaps. Their
[behavioral red](evidence/go/approval-projection-review-red.log) precedes fixes. The final adapter preserves classified
refusals as definite noncommits, treats ambiguous transport/malformed receipts as unknown without retry, and accepts
only advancing applied revisions or exactly equal unchanged revisions. The legitimate unchanged replay case also passes.

Final focused [green race/integration output](evidence/go/approval-projection-race.log):

```bash
go test -race -tags integration -p 1 \
  -coverprofile=/tmp/semteams-approval-projection.cover \
  ./cmd/semteams/approvalpause ./cmd/semteams/commands/implementspec \
  ./cmd/semteams/chain ./test/contract -count=1
```

Environment: macOS arm64, workspace Go toolchain from `go.mod`, local Docker testcontainers, no LLM or external service
calls, no extra environment overrides. The new `TestNATSProjectionLiveBoundary` explicitly pins NATS `2.14.4` and uses
an isolated dynamically assigned port. Existing `chain` integration retains its test helper's `2.14-alpine` image, which
resolved to `2.14.7`; the broader Go baseline uses that same existing helper. Browser infrastructure separately pins
`2.14.4`. The new boundary exercises the frozen exact reader and documented mutation request/reply over real NATS;
its graph owner is a protocol fixture. The new browser run-approval journey tests the actual configured graph owner.

The final suite proves two preserved receipts, duplicate and answer-first delivery, subscriber recreation, independent
concurrent Pauser instances, revision conflicts/recomputation, stable retry provenance, no cleanup loss of gate B,
operator/clarification disambiguation, immutable terminal phases, authority/type refusal, malformed/inconsistent tuples,
definite versus unknown mutation failures, valid unchanged receipts, and cancellation. NATS subscription readiness uses
flush, not sleeps. The unused interim `ReadPredicateValues` API was removed when the adapter adopted the frozen
`graph.ExactEntityReader`; its intermediate red/green logs are historical only. Current fail-closed evidence lives at
the actual projection boundary, including missing/wrong entity envelopes, zero revision and classified query failures.

| Final focused package | Statement coverage |
|---|---:|
| `approvalpause` | 80.5% |
| `commands/implementspec` | 85.0% |
| `chain` | 76.0% |

The new `RecordApproval` path is 88.5%, tuple projection computation 92.0%, run-anchor-to-projection dispatch 94.1%,
and implementspec selected-run authorization 100%. Broader boot/subscriber lifecycle coverage is not asserted by these
numbers. Full function output is in [approval-projection-functions.txt](evidence/go/approval-projection-functions.txt).

The implementspec tests preserve frozen `errs.ClassifiedError` codes for `invalid_loop_id`, `loop_not_found`,
`loop_owner_absent`, `loop_record_invalid`, and transient `loop_state_unavailable`, with no graph read/write on
rejection.
Nil lookup, a different returned loop ID, and absent owner fail closed. Canonical UUID inputs are used. These tests
verify SemTeams wrapping of the frozen lookup contract, not upstream's private lookup implementation.

The accepted framework-alignment decision is recorded in
[ADR-029](../../adr/029-product-shell-wiring.md#addendum-2026-10-01-execution-correlated-approval-projection).
The [inventory](contract-inventory.md#approval-ordering-review-and-accepted-projection-contract) records format,
lifecycle and future upstream public-client migration boundaries. Final full gates, actual-owner browser evidence,
and independent final Go review are recorded separately; this focused result does not claim complete migration approval.

## Committed generated-output checks

Content commit `f70537c8` contains the reviewed implementation. Regenerating schemas and running
`task schema:check-changes` both exited 0 after that commit. `task ui:generate-types:check` also exited 0;
the tracked schema/OpenAPI/type outputs remain unchanged. Exact commands and the full content commit are in
[evidence/go/postcommit-generated-checks.json](evidence/go/postcommit-generated-checks.json), with adjacent logs.
These are historical local checks, separate from hosted CI and the later repair-content checks below.

## Autoresearch terminal repair

The production source stayed at `8656192c` while the new behavior tests captured the failure. The
[exact red command](evidence/go/repair/go-red-command.txt),
[red output](evidence/go/repair/go-red.log) and
[test-source snapshot](evidence/go/repair/autoresearch_reply_integration_red_test.go.txt) preserve that state.
The old reviewer rule published an orphan final coordinator, the origin bridges were absent, and the cap-five
case stalled clearing pending at iteration four under the default-three action limit. A later
[ambiguity red](evidence/go/repair/ambiguous-anchor-red.log) preceded exact-one source-anchor guards.

```bash
TESTCONTAINERS_RYUK_DISABLED=true go test -race -tags=integration ./test/contract \
  -run '^TestAutoresearch(Reply|IterationCap)' -count=1 -v
```

The [focused green](evidence/go/repair/go-green.log) uses fresh NATS 2.14.4 JetStream and the real frozen matcher,
stateful evaluator, action executor, Mint, HandleTask and durable StateTracker. Graph/lifecycle persistence and task
publication are fixtures. Assertions cover native root parent/run fields despite inherited-first graph anchors,
phase/origin preservation, terminal child admission, malformed/ambiguous origin rejection, cancellation/approval
phase selection, duplicate/stale/restarted persisted state and five experiments above the former hidden limit.
Direct Mint mismatch refusal is tested separately; the action's parent-only publication fallback is not hidden.
This fixture does not claim crash-window transactional publication or actual graph-owner integration.

[Existing contracts and emitter/approval tests](evidence/go/repair/existing-contracts.log) also pass with the
[recorded command](evidence/go/repair/go-green-command.txt). The
[independent review run](evidence/go/autoresearch-independent-review.log) passes after source-anchor correction and
real child-admission coverage. Actual graph-owner/browser execution is supplied by the
[five focused results](browser-evidence/repair-focused/results.json), including both clarification decisions and
involuntary failure. The original failed browser evidence remains historical red.

## Final repair Go gates

[Exact commands, exits and UTC timings](evidence/go/repair/full-results.json) record all seven gates passing against
the repair source, later committed as `edf87d3104d4b5cc494623c89b17742539951380`:

| Command | Exit | Durable evidence |
|---|---:|---|
| `task lint` | 0 | [Lint](evidence/go/repair/full-go-lint.log), six warnings, zero errors |
| `task test:race` | 0 | [Race](evidence/go/repair/full-go-race.log) |
| `task test:integration` | 0 | [Integration](evidence/go/repair/full-go-integration.log) |
| `go build ./...` | 0 | [Build](evidence/go/repair/full-go-build.log) |
| `task schema:generate` | 0 | [Schema generation](evidence/go/repair/full-schema-generate.log) |
| `task openspec:validate` | 0 | [OpenSpec](evidence/go/repair/full-openspec-validate.log) |
| `task openspec:queue-test` | 0 | [Queue fixtures](evidence/go/repair/full-openspec-queue.log) |

The existing helper still resolves to NATS 2.14.7, while the new boundary fixtures pin 2.14.4. Both are explicitly
accounted for above. These gate invocations add no coverage-profile claim. The focused browser build/run manifests
establish unchanged source identity. A subsequent full-matrix attempt was
invalidated by a documentation edit to `configs/rules/autoresearch/README.md` inside its watched source tree, despite
passing Playwright assertions. The [invalidation record](browser-evidence/repair-final/INVALID-source-mutation.json)
preserves that orchestration failure. A fresh build and [full stable
rerun](browser-evidence/repair-final-stable/results.json)
then passed all 24 active scenarios with five skips and no source mutation. No guard or test failure was waived.

Schema and frontend generated-type dirty-tree checks both pass after the repair implementation commit; the
[new post-commit record](evidence/go/repair/postcommit-generated.json) records exact commands and outcomes.
The final documentation/archive commit and hosted checks are tracked by the PR lane, not asserted as future facts.

[Hosted Repository CI run 36876354628](https://github.com/C360Studio/semteams/actions/runs/36876354628) succeeded
on implementation commit `edf87d31`, including all four jobs. That observed result is separate from the accepted
expanded browser matrix and from checks for the final documentation/archive commit.
