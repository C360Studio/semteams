# Frozen SemStreams migration evidence

SemTeams moves from `v1.0.0-beta.160` at baseline `ce22c961d30014c463a09f8f8a2a90044ee1a1cf` to
`v1.0.0-beta.162.0.20260930150212-8b99efe9c66a`, frozen SHA `8b99efe9c66a4faa4fa509f9f62cc6bad8392128`.
[Issue #280](https://github.com/C360Studio/semteams/issues/280) owns the work;
[draft PR #281](https://github.com/C360Studio/semteams/pull/281) owns the claim. No replacement module is used.

The autoresearch terminal-delivery blocker is resolved by an independently reviewed SemTeams rule-pack repair.
All five focused mock browser cases pass: approved optimization, guardrail rejection, descended clarification,
limitation reply and involuntary failure. Each checks native task ancestry, authoritative run state, the original
HTTP channel/user and the delivered raw envelope in the existing UI. Full local Go gates and frontend gates pass.
The final stable 29-scenario browser matrix passes with **24 active passes, five explicit skips, no failures and no
source changes**. An earlier repair run was invalidated by a pack README edit inside the watched configuration tree;
its passing Playwright assertions were not accepted as qualification. Schema and frontend generated-type checks pass
after implementation commit `edf87d3104d4b5cc494623c89b17742539951380`.

The earlier `8656192c` qualification remains preserved as history: 20 passes, one autoresearch terminal-delivery
failure and five skips. No SemStreams change, response shim or new runtime primitive was needed for the repair.
This record does not assert that the migration has landed, SemSource is ready, or SemEngine is accepted.

## Evidence index

| Record | Purpose |
|---|---|
| [Contract inventory](contract-inventory.md) | Architect-approved bootstrap/config/rule/lifecycle/agentic/tool/UI contracts before adaptation |
| [Module identity](evidence/runtime/frozen-module.json) | Exact module/source SHA and checksum verification |
| [NATS identities](evidence/runtime/nats-identities.json) | Go helper 2.14.7 versus browser 2.14.4, with cached image digests |
| [Go validation](go-validation.md) | Baseline/target commands, retained logs, TDD evidence, coverage and final-gate limits |
| [Go review](go-review.md) | Independent code approval, repaired approval ordering, lint dispositions and qualification gaps |
| [Frontend review](frontend-review.md) | Independent F1–F4 code approval and browser-assertion/runner review |
| [Browser qualification](browser-baseline.md) | Final baseline/target matrix, overrides, attribution and qualification limits |
| [Final stable matrix](browser-evidence/repair-final-stable/results.json) | Accepted 24-pass/five-skip repair qualification with frozen source/image/config inputs |
| [Invalidated matrix](browser-evidence/repair-final/INVALID-source-mutation.json) | Preserved documentation-edit orchestration failure; no waived source guard |
| [Browser captures](browser-evidence/) | Machine-readable source/config/fixture hashes, exact compose/bootstrap files and results |
| [Browser difference excerpts](browser-evidence/differences.json) | Relevant states, tool errors and failing assertions |
| [Final warning ledger](browser-evidence/final/warning-ledger.json) | Exact composition warning severity, source, ownership and bounded capability effects |
| [Autoresearch terminal repair](autoresearch-terminal-blocker.md) | Historical typed ancestry failure, qualified repair and preserved boundaries |
| [Repair Go review](autoresearch-repair-review.md) | Independent approval of origin forwarding, run assertion and behavioral tests |
| [Repair frontend review](autoresearch-repair-frontend-review.md) | Independent review of typed delivery, run phase and raw UI evidence assertions |
| [Selected terminal envelopes](autoresearch-route-evidence.json) | Reduced durable wire evidence for that blocker |
| [Framework footprint](framework-footprint.md) | Measurement method, selected build contexts, limitations and SemEngine boundary |
| [Archived OpenSpec change](../../../openspec/changes/archive/2026-10-01-adopt-semstreams-frozen-8b99efe/) | Proposal, design, contract delta and conservative branch tasks |

The [historical 26-scenario matrix](browser-evidence/final/results.json) records the pre-repair outcomes.
The [five focused repair results](browser-evidence/repair-focused/results.json) record the repaired delivery cases.
Original failures and
corrected runs remain separately identified: a corrected assertion or stronger delivery gate changes what the test
proves. All scoped browser stacks were removed after that historical matrix. The current full rerun is recorded
separately.

## Baseline comparison and attribution

| Observed difference | Attribution | Evidence and consequence |
|---|---|---|
| Retired flow engine/store/template loader, seed flag and author/deploy endpoints | Intended upstream change | [Inventory](contract-inventory.md); UI retains graph exploration and read-only admitted composition, without flow-authoring claims |
| Canonical `system.domain` identity order and effective platform suffix | Intended upstream change | [Go identity tests](go-validation.md); owned rule/UI patterns changed; initial ops assertion used old diagnosis order and passed after correction |
| Root coordinator now has explicit role, durable authority and UUID identity | Intended upstream change | Initial research assertion expected an empty role; corrected query/role assertion passes, with final typed delivery separately checked |
| Rule configuration key family and caller-owned service lifecycle | Intended upstream change | [Go validation](go-validation.md); adapter readiness includes reconciliation, Stop uses fresh bounded context |
| Typed `user.response.*`, control command ingress, execution-correlated approvals and bounded approval timeout | Intended upstream change | [Inventory](contract-inventory.md); unsupported flat publishers/signals retired, owner/execution fences preserved |
| Research fan-out/join/review | Improved frozen behavior under intended typed routing | Existing corrected target journey passes; the same two-gather comparison has baseline structural completion but no final typed result, while target publishes exactly one; consolidated browser record retains exact fixture identity |
| Run-failed baseline assertion samples before ops observer appears | Reproduced baseline harness race | [Original failure](browser-evidence/differences.json) and [corrected baseline run](browser-evidence/baseline-race-fixed-results.json); waits for the observable state instead of immediate loop count |
| Old approval fixtures call unavailable/unadvertised `create_rule` | Existing fixture limitation | [Baseline failures](browser-evidence/differences.json); no product capability is inferred from those failed fixtures |
| Fresh sandbox-request approval comparison cannot observe pending baseline loop, but frozen approve/reject/cancel cases pass | Intended durable dispatch-authority change, with baseline observation limit | [Baseline boundary runs](browser-evidence/baseline-boundary-results.json), [target boundary runs](browser-evidence/target-contract-fixes-results.json); does not qualify all run-level event orderings |
| Approval answer/pending order can poison a later gate or acknowledge another execution | Reproduced implementation defect, repair independently approved | [Go validation](go-validation.md) records revision-fenced pair-set projection and final passing gates; [Go review G1](go-review.md) records code approval; no new bucket or worker |
| SSE terminal state can regress, edited approval arguments can cross gates, typed cancellation errors are ignored | Reproduced frontend defects | [Frontend review F1–F3](frontend-review.md); fixes have regression tests and independent frontend code approval |
| Autoresearch final coordinator has no typed ancestry to initial HTTP route | Reproduced SemTeams pack defect, repaired with existing upstream primitives | [Historical failure and repair](autoresearch-terminal-blocker.md); run facts return through the actual origin loop, preserving native parent/run routing; five focused browser cases pass |
| A documentation edit changes a watched configuration-tree hash during a matrix run | Orchestration failure, not a product failure or a passing gate | [Exact invalidation](browser-evidence/repair-final/INVALID-source-mutation.json); preserved separately, followed by a fresh image and full 29-scenario rerun with unchanged source |
| Run cap above three stalls under the default per-action firing limit | Existing authored-rule limit exposed by the new behavioral test | [Go red/green](go-validation.md#autoresearch-terminal-repair); repeating clear/propose actions explicitly use `max_iterations: 0`, leaving the run cap authoritative |
| Descended clarification chooses `respond_direct` and leaves the run executing | Existing explicit lifecycle policy, preserved | [Repair review](autoresearch-repair-review.md#preserved-limitation-reply-lifecycle); the response is delivered, but does not claim completed optimization or qualify subsequent continuation |
| Rule03 best-value prompt token and rule05 journal `.length` prose remain unresolved/sentinel-valued | Existing prompt-grounding defects at both pins | [Paired warning evidence](browser-evidence/repair-focused/prompt-warning-comparison.json); mocks qualify routing/tool outcomes, not live-model interpretation of those prompts |
| Autoresearch fixture omits `best_experiment_id` when calling artifact emitter | Existing fixture defect, reproduced at both pins | [Original paired errors](browser-evidence/autoresearch-fixture-defect.json); [corrected baseline](browser-evidence/baseline-autoresearch-corrected.json) proves actual emitter success/path and run best-value/experiment facts, then still fails strict final delivery; the [corrected target](browser-evidence/final/autoresearch-proof.json) proves those same artifact facts and still fails strict final delivery |
| Trajectory exposes facts/references but UI does not dereference full evidence bodies | Existing limitation #261 | [Claim boundary](../../demo-mvp-claims.md); do not claim ArtifactCard/context handoff, prose/tool bodies or rich narrative restored |
| Graph Chat pane is an unwired no-op at baseline and target | Existing UI limitation | [Frontend review](frontend-review.md); graph exploration and top-level product dispatch remain functional, no working graph-assistant claim |
| Typed terminal decision content remains structured rather than channel-ready reason prose | Existing upstream limitation | [SemStreams #1090](https://github.com/C360Studio/semstreams/issues/1090); typed publication is not proof of polished prose rendering |
| Ordinary client requests omit stable channel_id | Existing continuation limitation | [Frontend review](frontend-review.md); identity headers alone do not prove persistent conversation routing |
| Durable terminal observation can coexist with a live loop record after CAS conflict | Documented upstream residual | [Inventory](contract-inventory.md); UI preserves terminal evidence and reports disagreement, without claiming backend repair |
| Nine composition diagnostics appear at frozen admission and offline validation | Intended upstream diagnostics exposing existing topology limits | [Exact ledger](browser-evidence/final/warning-ledger.json); all nine are warnings, not errors. Optional storage ports, model chunks, discovery API and trajectory watcher are unconsumed; static graph lacks a user-response receiver although external typed replies are observed. No warning-free topology claim |
| Graph index routes and community generation lack configured owners | Existing capability gaps at both pins | [Browser diagnostic analysis](browser-baseline.md#runtime-diagnostics-and-qualification-limits); `graph-index` and `graph-clustering` are absent. Index routes, generated communities and full GraphRAG are not qualified; stored-summary absence uses statistical fallback |
| Initial `/loops` returns 503 while backend readiness is 200 | Intended fail-closed authoritative projection contract | [Cold-start evidence](browser-evidence/final/cold-start-authority.json); view-not-ready is explicit. Successful later admission does not qualify initial empty-loop-list availability |
| Empty duration warnings and run ancestry fallback appear in logs | Existing defaults and documented upstream fallback | [Warning ledger](browser-evidence/final/warning-ledger.json); both pins use the same buffer/cooldown defaults. Frozen research resolves through documented ancestry fallback and completes with a typed reply; no config expansion hides diagnostics |
| Go and browser gates use different NATS patch versions | Existing reproducibility limitation, measured explicitly | [Runtime identities](evidence/runtime/nats-identities.json); Go helper tag resolves locally to 2.14.7, browser pins 2.14.4; no cross-version behavior equality is inferred |
| Parked donor and artifact-handoff journeys remain skipped | Existing scope exclusions | [Baseline](browser-evidence/baseline-browser-results.json) and [target](browser-evidence/target-browser-results.json); skipped is not passed |

The original baseline matrix records 21 core scenarios (16 active, five explicit skips) plus two failing old approval
fixtures. Its run-failed assertion race was repaired and rerun explicitly. Initial target research/ops assertion
failures have separate corrected results. The historical full target matrix has 20 passes, one terminal-delivery
failure and five skips. The repaired terminal cases now pass in the
[focused results](browser-evidence/repair-focused/results.json) and the
[accepted full matrix](browser-evidence/repair-final-stable/results.json). All 24 active scenarios pass; five remain
explicitly outside scope.
The [actual run-scoped approval proof](browser-evidence/final/run-approval-boundary-proof.json) passes through graph
owner/projection, lifecycle rules, UI execution fencing and typed research delivery.

## Configuration and state boundary

Baseline and target use separate compose projects (`stmig160`, `stmig162`), ports (`33160`, `33162`), built images and
fresh volumes. [Baseline compose](browser-evidence/baseline-compose.json) and
[target compose](browser-evidence/target-compose.json), image digests and per-scenario manifests record exact settings.
The baseline Go source/module stay at `ce22c961`; later browser comparison specs/fixtures were deliberately amended
and separately hashed. The baseline directory is therefore not immutable as a whole. Per-run before/after manifests
distinguish controlled between-run corrections from changes during a run.

Scenario overrides include routing-only/team-spawn rule sets, autonomous clarification policy/persona overlay, and
sandbox-request approval policy. Mock LLM/attested sandbox fixtures do not qualify paid-model judgment or production
execution quality. The browser lane records effective configuration hashes and monitors concrete state during runs.

Browser qualification uses fresh NATS 2.14.4 and graph state. Existing Go integration gates use the unchanged
upstream `2.14-alpine` helper, locally resolved to 2.14.7; see [runtime
identities](evidence/runtime/nats-identities.json). Canonical identity and pending approval marker formats differ
from beta.160; the implemented repair uses loop/execution-correlated durable sets with revision-fenced reconciliation.
No retained-state reader/conversion, recovery
migration or production wipe is authorized or claimed. Existing storage must be assessed separately before deployment.

## Compatibility and product limits

The branch removes retired boot wiring; uses public loop/lesson projection contracts; shares one rule manager through
its config key family; reads post-start platform authority; preserves caller-owned shutdown; updates owned entity
patterns, typed reply ports, owner lookup, approval identity, cancellation, terminal SSE and composition UI consumers.
The live packs remain research/autoresearch with coordinator, agent-run and ops support; parked development packs stay
unwired. Historical ADRs describe their original releases and are not rewritten as current implementation guides.

SemTeams can complete the SemStreams upgrade independently of SemEngine. Framework references and compile closure
inform a later approved consumer contract. Full registrar closure and parked source/test obligations are not automatic
SemEngine first-release requirements. The [matched method-2 footprint](framework-footprint.md) includes
declaration/reference positions, import edges, module
graph and parked build diagnostics; its semantic and dynamic-runtime limits remain explicit.

SemSource-backed dogfooding remains held until SemSource is ready. Program Pulse remains the target product, not shipped
behavior. Evidence-body rendering limitation [#261](https://github.com/C360Studio/semteams/issues/261) remains separate
from the repaired autoresearch terminal route. Raw envelope inspection does not imply evidence-body rendering.

Historical post-content schema and frontend generated-type checks passed at `f70537c8`; see
[the exact record](evidence/go/postcommit-generated-checks.json). The [repair-content
checks](evidence/go/repair/postcommit-generated.json) also pass after `edf87d31`.
Hosted CI is tracked on the PR and is separate from these local qualification results.
