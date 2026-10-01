# Go migration validation snapshot

Final local Go gates passed against the stable approval-projection implementation. The intermediate red/green
snapshots below are retained as development history. [Independent Go code review](go-review.md) is approved with no
remaining blocking code findings. The [final browser matrix](browser-baseline.md) records 20 passes, one autoresearch
delivery failure and five explicit skips. Post-commit generated-output checks remain pending. Passing local Go tests
does not qualify the unresolved autoresearch final typed delivery.

## Baseline and environment

- Baseline commit: `ce22c961d30014c463a09f8f8a2a90044ee1a1cf`, SemStreams `v1.0.0-beta.160`.
- Frozen target: `v1.0.0-beta.162.0.20260930150212-8b99efe9c66a`, SHA
  `8b99efe9c66a4faa4fa509f9f62cc6bad8392128`; no replacement or upstream shim.
- Dedicated worktree: `/Users/coby/.codex/worktrees/semstreams-frozen-migration/semteams`, branch
  `codex/semstreams-frozen-8b99efe`. Proposal commit: `4300f6c8`; target results include uncommitted implementation.
- Baseline Go source preserved at `/tmp/semteams-migration-8b99efe/baseline-source`.
  Later browser comparisons deliberately amended selected E2E specs/fixtures in that directory (observer synchronization,
  two-gather proof and corrected autoresearch emitter inputs). It is not an immutable whole-directory archive.
  Baseline Go source/module remain unchanged; each browser run's manifest records its exact amended inputs.
- macOS arm64, Go `1.26.4` (module minimum `1.26.3`), Task `3.51.1`, local Docker Desktop.
- Integration uses fresh testcontainers and test cleanup, `TESTCONTAINERS_RYUK_DISABLED=true`, sequential
  package execution (`-p 1`). The unchanged upstream `NewTestClient` defaults to `nats:2.14-alpine`
  (beta.160 `natsclient/test_client.go:589`; frozen line 598). The cached local image reports **2.14.7**.
  The browser lane explicitly uses **2.14.4-alpine**. These are different runtime lanes; the Go gate results
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

## Final local Go gates

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

Final lint has zero errors and seven warnings. Five are retained from the baseline; two new non-failing warnings
are approval projection function length (52 statements against the 50-statement rule) and a local variable named
`copy`. The [independent review disposition](go-review.md#new-lint-warning-disposition) accepts the cohesive
function length and defers the cosmetic rename to the next approved source edit under #280 to preserve the final
qualification source identity. Neither warning is suppressed or treated as a behavioral blocker. The final gate is
green, but its warning count is not the earlier five-warning snapshot.

`task schema:check-changes` and the frontend generated-types dirty-tree check remain pending the content commit.
Generation success is not a claim that those checks passed. Independent code review is approved; final hosted checks remain owned by the PR lane.

## Final coverage

The full integration coverage profile reports 72.1% overall. The durable
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
| `task schema:check-changes` | exit 0 | pending after commit | `evidence/go/semteams-frozen-schema-check.log` |

`task test:race` executes `go test -race -count=1 ./...`. `task test:integration` executes
`go test -race -count=1 -tags=integration -p 1 ./...` with the environment above. The final lint, race and build
were repeated after adapter edge-case tests; the focused integration coverage run below includes those tests.
The full integration gate had already passed before those test-only additions and the timeout-metadata correction.

Lint retains five existing warnings, with no errors. The first frozen full race run failed on old
`flow-service` generator assertions. These were changed to assert retired service absence and supported
component/composition response types; the full race rerun passes. The schema command generated changed
framework schemas/OpenAPI, and the orphan retired workflow schema was removed. The dirty-tree schema gate
currently exits Task status 201 (inner status 1) because these generated changes are not committed. It must
be run after committing the generated output; this report does not call that gate green at the target.

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
2. **Autoresearch terminal delivery:** the final coordinator completes but lacks typed ancestry back to the
   root HTTP route after the non-loop run-triggered action. See
   [terminal blocker](autoresearch-terminal-blocker.md) and [reduced wire evidence](autoresearch-route-evidence.json).
   This is a product lineage limitation exposed by stricter qualification, not a claim that closed upstream
   issue #1094 is still broken. No publisher shim or unsupported run predicate is added.
3. **Coverage and final gates:** focused regression/coverage evidence below supplements the final full Go gates
   recorded above. Both independent code reviews are approved. Schema/frontend generated-output dirty-tree
   verification remains pending after the content commit; hosted CI remains a separate PR gate.
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
[test-before-code output](evidence/go/approval-projection-red.log) is a compile red for the absent adapter, not a separate
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
`loop_owner_absent`, `loop_record_invalid`, and transient `loop_state_unavailable`, with no graph read/write on rejection.
Nil lookup, a different returned loop ID, and absent owner fail closed. Canonical UUID inputs are used. These tests
verify SemTeams wrapping of the frozen lookup contract, not upstream's private lookup implementation.

The accepted framework-alignment decision is recorded in
[ADR-029](../../adr/029-product-shell-wiring.md#addendum-2026-10-01-execution-correlated-approval-projection).
The [inventory](contract-inventory.md#approval-ordering-review-and-accepted-projection-contract) records format,
lifecycle and future upstream public-client migration boundaries. Final full gates, actual-owner browser evidence,
and independent final Go review are recorded separately; this focused result does not claim complete migration approval.
