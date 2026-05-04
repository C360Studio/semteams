# ADR-032: R3.6 Builder Sandbox Design

## Status

**Accepted — 2026-05-02.** First review pass with Coby on 2026-05-02
closed all open items: target locked to OSH-Java-Maven bare seed,
security boundary set via structural path confinement, network policy
decided as egress allow-list day-one, scaffolding seeds rejected as
demo-gaming. The Proposed/Open buckets are empty modulo two
deliberate deferrals (model tier, handoff format) called out below.
R3.6.1 implementation begins on branch `plan/r36-sandbox`.

## Context

ADR-031 R3.4 shipped the dev-via-spec arc terminating at a structured
spec artifact (`docs/specs/<slug>.md`, `dev_via_spec.artifact.v1`
typed payload). The OSH-class chain converged in 6 loops on real LLM,
producing a Meshtastic / OGC CS bridge spec.

But the spec is **input** to the demo, not the demo. The demo target
is *driver written that compiles and passes tests*. R3.6 is the
builder slice: a `dev-via-spec-builder` agent reads the spec
artifact, writes code, runs tests, iterates until passing.

That requires a **sandboxed code-execution environment.** Both
sibling products in the c360 ecosystem already have one — semspec
and semdragon. Both have been a serious pain. This ADR is the
deliberate planning conversation Coby flagged on 2026-05-02:
*"semspec and semdragon both have one. and it's been a serious pain
in the ass… i would say finish out 3.4 and maybe 3.5 if it makes
sense and then we need a serious plan for sandbox."*

## Survey of upstream sandbox implementations

### Semspec

Lives at `~/Code/c360/semspec/cmd/sandbox/`. ~2250 lines in
`server.go`. Designed to support **parallel DAG task decomposition**:
multiple concurrent tasks merging branches into a shared repo's
`main`. The core complexity comes from that parallelism:

- **Global `repoMu sync.Mutex`** serialises all branch / merge /
  HEAD-mutating operations. Not per-repo — global.
- **`needsReconciliation` atomic flag**: when a merge fails AND the
  self-heal (`merge --abort`, `reset --hard`) also fails, the flag is
  set; all merge endpoints return HTTP 503 until an operator hits
  `POST /admin/reconcile`. Concurrent task merges to a wedged repo
  are blocked by design.
- **Per-task worktrees** at `.semspec/worktrees/{task_id}/`,
  scoped to the task lifecycle. Worktrees outlive their parent
  task if cleanup is delayed (orphan collection runs hourly with a
  24h TTL).
- **`taskIDMain = "main"`** special-cases read-only access for plan
  / reviewer agents who shouldn't see the worktree state.

### Semdragon

Lives at `~/Code/c360/semdragon/cmd/sandbox/`. ~700 lines in
`server.go` — about 3.2× smaller. Designed for **quest-scoped serial
agency**: each quest is owned by one agent (or a party with directed
sub-quest assignment via `partycoord.AssignTask()`):

- **Per-repo `repoMutexes map[string]*repoMutex`** allows merges on
  different repos to proceed in parallel; merges on the same repo
  serialize. No global lock contention.
- **Named Docker volume** for workspaces. Workspaces live at
  `/workspace/{quest_id}/`; Docker handles cleanup at volume
  destruction; no orphan-collector cron needed.
- **Quest-scoped lifecycle**: workspace created on quest entering
  agentic loop; deleted on quest completion (`DELETE /workspace/
  {questID}`). On failure, workspace persists for forensics with
  `GET /workspace/{questID}` returning a zip download.
- **Production-hardened compose**: `cap_drop: ALL` + minimal
  `cap_add` for git, `no-new-privileges: true`, `read-only: true`
  root filesystem with tmpfs at `/tmp` (512M) and `/var/tmp` (256M),
  hard caps `2 CPU / 4 GB RAM`, file descriptors 4096/8192.
- **Tier-gated tool access** via `questtools` processor (Apprentice
  read-only, Journeyman+ `bash`, Master+ `decompose_quest`).

### Why semdragon is cleaner — the load-bearing reason

**It's not just code-size.** Semspec's complexity is paid for
parallel DAG correctness: concurrent tasks targeting the same repo's
`.git`, merge reconciliation across overlapping branches, race-free
worktree creation and cleanup. The reconciliation state machine
(~100 lines) and global `repoMu` (every branch op blocks) are
correctness costs of that parallelism.

Semdragon dodged it by deciding agency is **directed, not
pull-based**. The DAG executor knows which agent works on which
sub-quest before any worktree is created. There's no "race to claim"
logic; every quest has at most one active worker.

### Implication for SemTeams's serial chain

SemTeams's dev-via-spec chain is **serial by design**: planner →
reviewer → challenger → architect → builder. Each role waits on the
previous role's terminal `decide`. There's no concurrent merge
scenario.

**We can use semdragon's simpler model directly.** The reconciliation
state machine, global `repoMu`, and orphan-collection cron are
overhead we don't need. Per-repo mutexes are sufficient (and even
those barely earn their keep — at one builder per chain, contention
is ~zero).

That's the load-bearing finding from the survey. **Semdragon's
design is the right starting shape.** Below are the decisions
where we still need to choose.

## Why per-product, not upstream (yet)

semspec, semdragon, and now semteams will each have their own
sandbox. We are deliberately NOT extracting a shared `semsandbox`
upstream now. Three reasons:

1. **Three implementations diverge on load-bearing concerns.**
   semspec is parallel-DAG; semdragon is serial-quest; semteams is
   serial-chain-with-different-concerns. The shared shape isn't
   obvious until all three are running with real workloads.
2. **Per ADR-029, when upstream lacks a primitive, product-local
   with a documented migration target is the discipline.** Premature
   extraction across the parallel-DAG/serial gap would ship the
   wrong API.
3. **Copying patterns across products is cheaper than the wrong
   abstraction.** Three sandboxes that share ~80% structure but
   differ on the load-bearing 20% is the right place to be until we
   have data on what's shared.

Migration trigger: when (a) all three products' sandbox APIs
converge to ~80% identical, OR (b) a fourth product needs one.
Until then, no upstream extraction.

## Decision space

Each row below is a decision SemTeams's R3.6 sandbox needs to make.
Status is one of:

- **Decided**: locked; alternative ruled out with reason.
- **Open**: deliberately deferred to a downstream phase; default
  proposed but final lock waits on observed data.

After the 2026-05-02 review pass with Coby, the Proposed bucket is
empty — every item there was either confirmed-as-default and moved
to Decided, or pushed into Open with an explicit deferral target.
A few decision numbers were reused (e.g. old Open #17 collapsed
into #13; old Open #18 became Decided #17) so reading the
provenance in the Rationale column is recommended.

### Decided

| # | Decision | Choice | Rationale |
|---|---|---|---|
| 1 | **Container model** | One sandbox service container with per-workspace volume subdirs (mirrors semdragon). | Container-per-task is a blast-radius win we don't need at our scale. Single-service container with per-chain volume subdirs is simpler and matches semdragon. |
| 2 | **Workspace persistence** | **Per-chain named Docker volume** at `/workspace/{chain_id}/`. Mirrors semdragon. | Semspec's worktrees-in-repo model trades operational simplicity for parallel-DAG capability we don't have. Docker volume = no orphan cleanup, no manual grooming. |
| 3 | **Git mutex strategy** | **Per-repo mutex** (`map[string]*repoMutex`), not global. Mirror semdragon. | Even with no parallelism today, per-repo is the simpler API and dodges "what if we add a second concurrent chain later." Costs ~10 lines vs ~zero. |
| 4 | **Reconciliation state machine** | **Skip.** Don't port from semspec. | Reconciliation handles wedged-repo state under concurrent merges. Our serial chain produces zero merges (builder writes to its own quest workspace; no merge to a shared `main`). |
| 5 | **`taskIDMain` special case** | **Skip.** Don't port. | semspec's read-only `main` access is parallel-DAG cruft. Our planner / reviewer / challenger consume prior loop results via `read_loop_result`, not workspace state. Builder gets its own workspace. |
| 6 | **Resource limits** | **Hard-cap from compose**, mirroring semdragon: 2 CPU, 4 GB RAM, fd 4096/8192. | Production-ready posture. Cheap to land, expensive to retrofit. |
| 7 | **Security hardening** | **`cap_drop: ALL` + minimal `cap_add` for git ops; `no-new-privileges: true`; read-only root + tmpfs.** Mirror semdragon. | Same posture as semdragon's compose. Land it day one. |
| 8 | **Toolchain in the image** | **Java (Maven + Gradle), Go, Node, Python, protoc + protoc-gen-go.** | OSH-Java-Maven is the locked target (#17). Toolchain-in-sandbox + bare-seed responsibility means product provides the environment; LLM produces every artifact (pom.xml, OSGi bundle metadata, sensor logic, test harness). Toolchain never ships boilerplate. |
| 9 | **Cleanup model** | **Quest-scoped + explicit `DELETE /workspace/{chain_id}`.** No periodic orphan collector. | Aligns with semdragon. Named volume + explicit lifecycle = no manual grooming. |
| 10 | **Workspace forensics** | **`GET /workspace/{chain_id}` returns a zip download** (mirror semdragon). | Cleaner than host-filesystem inspection (semspec's pattern); ergonomic for "what did the chain produce on this run." |
| 11 | **Test runner** | **`bash` only**, like both sibling repos. Builder calls `bash mvn test` (or `go test ./...`). | Typed `run_tests` tool ruled out as premature constraint. Revisit only if we observe builder LLM consistently misusing bash for tests. |
| 12 | **Approval gate on bash/write_file** | **No, auto-fire** (mirror both sibling repos). | Path confinement (#16) + egress allow-list (#13) + deny-list residual (#23) cover the safety story structurally. Per-call approval would gate every iteration; unworkable for a 5–8 retry loop. |
| 13 | **Network policy** | **Egress allow-list day-one + full request logging.** Allow: Maven Central (`repo1.maven.org`, `repo.maven.apache.org`), GOPROXY (`proxy.golang.org`, `sum.golang.org`), npm (`registry.npmjs.org`), GitHub (`github.com`, `api.github.com`, `raw.githubusercontent.com`, `objects.githubusercontent.com`), configured SemSource host. | Open day-one is cheaper to ship but expensive to retrofit. With OSH-Java-Maven locked, the LLM hits Maven Central on iteration 1 — we want every fetch logged from the start so we can see dep drift across runs and tighten if needed. Mechanism (Docker network policy or per-container iptables) is the load-bearing piece; policy is config. |
| 14 | **Per-workspace disk quota** | **10 GB hard cap via tmpfs `size=` per workspace.** | Cheap to land day-one. Maven dependency download + build artifacts could fill an unbounded volume in one runaway loop. |
| 15 | **Builder terminal contract** | **`decide(action, ...)` with action ∈ {tests_passing, tests_failing, needs_clarification}** and required evidence fields. `max_iterations` cap **30** (recalibrated from initial 8 per the §addendum 2026-05-03 R3.6.2.f smoke; ceiling 50 matching semspec). 8 was empirically too low by ~4× — even at 15 the builder force-failed before reaching `mvn`. Per-rule override of max_iterations isn't supported in beta.39's `rule.Action`; bump applies at the agentic-loop component-level config. | Validator rejects terminal payloads missing required fields:<br>• `tests_passing`: `tests_run` (must be >0), `tests_passed`, `tests_failed`, `artifact_summary`<br>• `tests_failing`: `tests_run`, `tests_failed`, `failure_summary`, `retry_hint`<br>• `needs_clarification`: `reason`, `blocking_question` — slot baked in for R3.5 rule landing additively.<br>Closes the "exit 0 with no tests" loophole; pre-bakes R3.5. |
| 16 | **Path confinement (safety boundary)** | **Structural at API boundary, day-one.** `write_file` accepts only workspace-relative paths after `filepath.Clean` + prefix check; rejects absolute paths and `..` traversal. `bash` exec runs with `chdir` set to workspace and rejects `..` in args. | Stronger than pattern-match deny-list. ~20 lines of validation closes the bulk of the safety story. Residual deny-list (e.g., `rm -rf` patterns hitting workspace-internal paths) deferred to R3.6.3 — see Open #23. |
| 17 | **Target choice** | **OSH-Java-Maven, bare seed.** | Toolchain-in-sandbox is the responsibility line: product provides Java/Maven/OSGi/protoc tooling pre-installed; LLM produces every artifact (pom.xml, OSGi bundle metadata, abstract base class wiring, surefire harness, sensor adapter logic) from the spec. Pre-built scaffolding seeds rejected as demo-gaming — the early-adopter comparison ("agents wrote a real OSH driver, not a hello-world") only holds if the agents wrote the boilerplate too. |
| 18 | **Shared cache volume** | **Named Docker volume mounted r/w across all workspaces** at `~/.m2`, `$GOPATH/pkg/mod`, `~/.npm`. Caches treated as immutable-by-convention; nuke the volume if poisoned. | Without this, every workspace's first `mvn install` re-downloads the world (60–120s). With OSH-Java-Maven + bare seed locked, this is the load-bearing iteration-speed lever. Verify semdragon's pattern matches before locking exact mount paths. |

### Open (deliberately deferred)

| # | Decision | Default | Defer to | Lock criterion |
|---|---|---|---|---|
| 19 | **Builder model tier** | Sonnet at low effort (matches the chain's existing tier). | First builder smoke (smoke #6) | Convergence rate / drift on real-LLM smoke. Bump to higher reasoning_effort or claude-opus if drift. |
| 20 | **Spec-to-builder handoff format** | Both: typed payload via `query_entity` for routing decisions, markdown via `read_loop_result` for human-comprehensible context. | R3.6.2 persona design | LLM ergonomics — confirm with persona fragment design. |
| 23 | **Deny-list (residual)** | None day-one; path confinement (#16) + egress allow-list (#13) cover the structural cases. | R3.6.3 production-readiness | What patterns surface as actually-dangerous in workspace-internal scope after smoke #6 observation. |

## Phasing

We do **not** try to land R3.6 in one PR. The user's 2026-05-02
framing was explicit: *"Don't try to do builder + sandbox in one
PR. Land the sandbox abstraction (with a stub builder driver,
possibly a 'hello world' test) before adding LLM-driven code
synthesis on top."* Apply that.

Three slices:

### R3.6.1 — sandbox primitive (no LLM yet)

- New product-shell component or sidecar at `cmd/semteams/sandbox/`
  (or wherever fits the ADR-029 wiring discipline). Mirror
  semdragon's `cmd/sandbox/server.go` shape.
- Compose service `sandbox` with the production-hardened posture
  from semdragon (resource limits, read-only root, tmpfs, cap_drop).
  Profile-gated under `local-models` or a new `sandbox` profile.
- **Endpoints**: `POST /workspace/{chain_id}` (create),
  `POST /workspace/{chain_id}/exec` (bash),
  `POST /workspace/{chain_id}/files` (read/write — workspace-relative
  paths only, structural confinement),
  `GET /workspace/{chain_id}` (zip download),
  `DELETE /workspace/{chain_id}`. **Drop**: `merge-to-main` (no
  shared-main merge in our serial chain — see #4).
- **Per-repo mutex**; no reconciliation flag; no taskIDMain (#3, #4, #5).
- **Toolchain in image**: Java (Maven + Gradle), Go, Node, Python,
  protoc, protoc-gen-go (#8).
- **Shared cache volume** (#18): named Docker volume mounted r/w
  across all workspaces at `~/.m2`, `$GOPATH/pkg/mod`, `~/.npm`.
  Managed by sandbox service, not per-workspace.
- **Egress filter mechanism + allow-list** (#13): Docker network
  policy or per-container iptables; allow Maven Central, GOPROXY,
  npm registry, GitHub, configured SemSource host; full request
  logging to backend logs.
- **Path confinement** structural at API boundary (#16):
  `write_file` rejects absolute paths and `..` traversal; `bash`
  runs with `chdir` to workspace and rejects `..` in args.
- **Per-workspace disk quota** (#14): 10 GB hard cap via tmpfs
  `size=`.
- **Tests**: server-level integration tests under
  `test/integration/sandbox/` exercising create / exec /
  read-write-file / cleanup / forensics-zip; egress filter tests
  verifying allow-listed hosts succeed and others fail; path
  confinement tests verifying traversal/absolute paths rejected.
- **Decision lock**: items #1–#14, #16–#18 (all Decided rows except
  #15, which is R3.6.2 surface).

### R3.6.2 — sandbox tool + builder persona stub

- **Product-shell tools**: `write_file`, `read_file`, `run_command`
  (or reuse upstream's `bash` and add a thin `write_file` we own).
  Allow-listed in the chain's `agentic-tools.allowed_tools`. Tool
  implementations enforce path confinement at the call site.
- **Builder terminal validator** (#15): rejects `decide` payloads
  missing required evidence fields; specifically rejects
  `tests_passing` with `tests_run == 0`. Lives in product-shell
  decide-handler, not LLM persona.
- **New persona**:
  `configs/personas/fragments/dev-via-spec-builder/*`. Reads the
  spec artifact (typed payload via `query_entity` + markdown via
  `read_loop_result` per #20). **Bare-seed responsibility line
  spelled out explicitly**: "you are responsible for `pom.xml`,
  OSGi bundle metadata, abstract base class wiring, surefire test
  harness, and sensor adapter logic — the spec describes the
  *what*, you produce the *how* end-to-end." Iterates
  `read_file` / `write_file` / `bash` until tests pass or
  `max_iterations` (8) exhausted.
- **New rule**:
  `configs/rules/dev-via-spec/06-architect-emit-to-builder.json`
  fires on `coordinator.next_action="seed_requirements_emitted"`
  AND a marker triple from the artifact emit (e.g.
  `dev_via_spec.artifact.path` exists). Spawns the builder.
- **Mock-llm fixture** exercising the chain through builder
  terminal with a stubbed test run. Smoke #6 (real-LLM,
  OSH-Java-Maven bare seed) follows.
- **Decision lock**: item #15 (terminal contract) + Open #19
  (model tier — observe and lock after smoke #6) + Open #20
  (handoff format — confirm in persona design).

### R3.6.3 — production-readiness pass

- **Decision lock**: Open #23 (residual deny-list) — define
  dangerous-command pattern matching for workspace-internal scope
  beyond what path confinement and egress allow-list catch.
- **ADR addendum** capturing what the first real-LLM builder smoke
  (smoke #6) produced and any pivots needed: model-tier observation
  (Open #19), iteration-cap calibration, egress allow-list
  adjustments, scaffolding regrets if any (the bare-seed posture's
  first encounter with reality).

Most production-readiness items (allow-list, disk quota, path
confinement, security hardening) were pulled forward into R3.6.1.
R3.6.3 is now smaller, focused on residual deny-list and
post-smoke calibration.

## What this ADR explicitly does NOT decide

- **Whether R3.5 ships before R3.6.** R3.5 was designed in
  ADR-031 §addendum 2026-05-02. Smoke #5 converged without it.
  Recommend deferring until R3.6 surfaces actual ambiguity (which
  is likely — the builder hits "is this scope creep or
  seed-requirement-implied?" more than the planner does). The
  `needs_clarification` slot is baked into the builder's terminal
  enum (#15) so the R3.5 rule lands additively.
- **The exact name and product-shell location of the sandbox
  component.** R3.6.1 design will pick this; survey the
  framework-alignment ADR-029 patterns first.
- **Builder persona prompt details.** R3.6.2 work — informed by
  smoke #6 observations once the sandbox primitive lands.

## Acceptance criteria for this ADR moving from Proposed → Accepted

1. **Done**: Decision rows #1–#10 reviewed and confirmed in
   2026-05-02 review pass with Coby. Wording on #1 (container
   model) and #8 (toolchain) updated for clarity.
2. **Done**: Open items #16–#22 each closed or deliberately deferred:
   - #16 (path confinement) → Decided as #16 structural
   - #17 (network policy) → Decided as #13 allow-list day-one
   - #18 (target) → Decided as #17 OSH-Java-Maven bare seed
   - #19 (model tier) → Open, deferred to smoke #6 observation
   - #20 (handoff format) → Open, deferred to R3.6.2 persona design
   - #21 (R3.5 slot) → Decided, baked into #15 terminal contract
   - #22 (test-pass criterion) → Decided, evidence fields in #15
3. **Done**: Phasing reviewed; R3.6.1 absorbed cache volume +
   egress filter + path confinement + disk quota; R3.6.3 trimmed
   accordingly. No PR attempts R3.6.1 + R3.6.2 in one shot.
4. **Done**: Coby Accept on this rev (2026-05-02).

## Addendum 2026-05-02 — egress filter + disk quota deferral

Implementation of R3.6.1.d surfaced two incompatibilities with the
original phasing. Both items are deferred to R3.6.3 with explicit
rationale so future readers see the why-deferred trail:

### Per-workspace disk quota (Decision #14)

ADR §14 specified "10 GB hard cap via tmpfs `size=` per workspace."
This is incompatible with the named-cache-volume decision in §18:
named volumes do not support per-subdirectory size caps. Per-workspace
quota on a single named volume requires either:

- **XFS project quotas** on the underlying volume — couples sandbox
  ops to host filesystem choice; brittle across deployment targets.
- **Per-workspace tmpfs mount** dynamically created — Docker compose
  doesn't support dynamic per-chain mounts; would need a sidecar
  workspace-manager that does the mounts via Docker API.
- **Accept "no per-workspace quota"** with the cache-sharing benefit.
  Mitigation: monitor total volume usage; nuke + recreate if a
  runaway chain fills it.

R3.6.1.d ships **option 3** (no per-workspace quota). R3.6.3 revisits
with real workload data from smoke #6 — if a builder iteration
genuinely fills 10 GB, project quotas justify the host-OS coupling.
Until then, the cache-sharing iteration-speed lever (§18) outweighs
the bounded-blast-radius benefit of per-workspace caps.

### Egress filter (Decision #13)

ADR §13 specified "egress allow-list day-one + full request logging"
via "Docker network policy or per-container iptables." Implementation
revealed three constraints:

- **iptables inside the container** is precluded by `cap_drop: ALL`
  (Decision #7) — adding NET_ADMIN to support iptables would
  significantly weaken the security posture for an enforcement
  mechanism the LLM toolchain doesn't need.
- **Docker network policy alone** doesn't support per-host allow-
  lists on bridge networks. Requires either an L7-aware mesh
  (Cilium/Istio — heavy) or a sidecar HTTP/HTTPS-CONNECT proxy
  bridging an internal-only sandbox network and an egress network.
- **Sidecar proxy** (~250 lines: Go binary + tests + Dockerfile +
  compose plumbing + Maven settings.xml + Gradle/npm proxy env)
  is the right *shape* — privilege-separated container, agent's
  bash can't kill or reconfigure it — but premature before we've
  observed what the real builder agent actually reaches for in
  smoke #6.

R3.6.1.d ships **logging-only egress** (toolchain stdout/stderr
already captures every URL mvn/npm/go fetches, exposed via the
exec endpoint and forensics zip). R3.6.3 revisits with real smoke
#6 data: if the chain only ever hits the §13 allow-list (Maven
Central, GOPROXY, npm, GitHub, configured SemSource), the proxy
shape sketched above is the obvious next slice. If the chain
reaches for unexpected hosts, that's product/security signal worth
discussing before the mechanism lands.

The "expensive to retrofit" concern in the original §13 rationale
turns out to be about the *mechanism*, not the *policy*. The
mechanism is significant work; landing it before we know what to
allow-list is the wrong order. R3.6.3's egress slice will land the
proxy as drafted, with the empirical allow-list from smoke #6.

### R3.6.1.d revised scope

With both items deferred, R3.6.1.d collapses to a close-out slice:
final go-reviewer pass over the assembled R3.6.1 (lifecycle + file
ops + path confinement + exec + toolchain + cache), full lint /
race / e2e smoke, and PR.

## Addendum 2026-05-03 — R3.6.2.b builder_decide as sibling tool

R3.6.2.b ships the builder terminal validator from §15. The
framework-alignment review (ADR-029 discipline) settled on a
**sibling tool** (`builder_decide`) rather than a wrapping
replacement of upstream `decide`. The evidence trail:

### What §15 left open

The decision row says "Lives in product-shell decide-handler, not
LLM persona. ADR-032 §15 has the full contract. Shape: a thin
product-shell tool wrapper or a `decide` pre-validator." Both
shapes were explicitly authorised; the addendum picks one.

### Why not wrap upstream `decide`

semstreams beta.39 surfaces:

- `agentictools.DecideExecutor` with parameter schema `{action,
  reason, subtopics, retry_hint}` — the builder evidence fields
  (`tests_run`, `tests_passed`, `tests_failed`,
  `artifact_summary`, `failure_summary`, `blocking_question`)
  aren't in the schema, so the LLM has no path to supply them
  through the canonical tool.
- `agentictools.ExecutorRegistry.RegisterTool` returns an error
  on duplicate name. There is no `Unregister` /
  `Replace` surface. `executors.RegisterBuiltins` (which the
  product shell calls in `cmd/semteams/main.go` step 9c)
  registers `decide` *before* `registerProductTools` fires (step
  9d), so the product shell cannot register a wrapper under
  `decide`.
- The framework's `wrapping_pattern_test.go` demonstrates a
  wrapping executor — but only when the product shell owns the
  initial registration. Once the framework's own `RegisterBuiltins`
  has registered `decide`, that path is closed.

A `decide` pre-validator hook in upstream would close this — but
adding one is a non-trivial framework change, premature without a
second product (semspec / semdragon) wanting role-specific
validators on `decide`. Not worth the upstream cost for one slice
of one product.

### Why a sibling tool is correct here

`builder_decide` mirrors the canonical agent-terminal-tool
emission shape (`decide`, `emit_diagnosis`,
`emit_research_artifact`, `emit_dev_via_spec_artifact`):

- emits `coordinator.next_action` + `coordinator.decision_reason`
  triples on the loop entity (parity with `decide` so downstream
  rules can route on the same predicates),
- adds per-action evidence triples under
  `dev_via_spec.builder.{tests_run,...}` (counts and references
  only — never free-text content beyond LLM-authored summaries),
- returns `StopLoop=true` with the full args in `Content` for
  `read_loop_result` consumers.

The builder persona's `allowed_tools` lists `builder_decide`
instead of `decide`. The canonical `decide` remains untouched and
available for any role that doesn't need evidence validation.

### Migration posture

Stays product-local until upstream ships either (a) a per-role
terminal-tool primitive, or (b) a `decide`-extension hook that
lets product code register validators against existing tool
names. Neither shipped in beta.39. When one ships, revisit:

- If (a): replace `builder_decide` with the upstream primitive,
  configure with the per-action evidence contract.
- If (b): drop `builder_decide`, register a validator under
  `decide` for the builder role.

Either way, the migration is name-only — the action enum, the
evidence fields, and the triple set are stable design decisions.

### What R3.6.2.b ships

- `cmd/semteams/tools/builderdecide/{doc.go,executor.go,executor_test.go}`
  — executor, predicates, full TDD test suite covering the action
  enum, per-action evidence requirements, the
  `tests_passing`-with-zero-tests gate from §15, JSON-number
  coercion, negative-count rejection, and triple-publish failure.
- `cmd/semteams/product_tools.go` — `registerBuilderDecide`
  wired after `registerEmitSpecArtifact` (always-on when NATS is
  reachable, mirrors the other product-shell tools).
- `cmd/semteams/tools/README.md` — tool-table row + migration
  posture pointer to this addendum.

R3.6.2.c (builder persona fragments), R3.6.2.d (rule that fires
on builder loop spawn → POST /worktree), R3.6.2.e (mock-LLM
fixture), and R3.6.2.f (smoke #6) remain on the roadmap.

## Addendum 2026-05-03 — R3.6.2.d spawn rule + bootstrap_workspace tool

R3.6.2.d ships the rule that fires when the architect emits a
spec artifact, plus the product-shell tool that bootstraps the
builder's sandbox workspace. The framework-alignment review
(ADR-029 discipline) revealed an architectural gap that pulled
the design away from this ADR's original §R3.6.2.d framing.

### What §R3.6.2.d originally proposed

The R3.6.2.d row in the §Phasing section read:

> **.d** — rule that fires on builder loop spawn (matching marker
> triple from architect's artifact emit): POST /worktree on
> sandbox via `http_request` rule action.

Two assumptions in that sentence don't survive contact with
beta.39:

1. **No `http_request` rule action exists.** Upstream beta.39
   ships these rule actions: `publish`, `add_triple`,
   `remove_triple`, `update_triple`, `publish_agent`,
   `trigger_workflow`, `publish_boid_signal`, `update_kv`,
   `deny`. A generic outbound-HTTP primitive would be useful for
   any product but isn't in scope upstream.

2. **`publish_agent` generates the spawned task_id internally.**
   `processor/rule/actions.go:587` computes
   `taskID := fmt.Sprintf("rule-%s-%d", entityID,
   time.Now().UnixNano())` at action-execution time and embeds
   that into the published TaskMessage. The rule cannot
   pre-create a sandbox worktree at the new loop's task_id
   because the task_id doesn't exist until publish_agent runs;
   even with `http_request` available, you'd need a substitution
   variable (e.g. `${rule.spawned_task}`) that exposes the
   to-be-generated task_id to subsequent on_enter actions in the
   same firing.

### Options considered

The framework-alignment review weighed three paths:

1. **Land both upstream changes** (`http_request` action +
   `${rule.spawned_task}` substitution). Heaviest path; blocks
   R3.6.2.d on multiple upstream slices; the `http_request`
   action is generally useful but requires careful design (auth,
   timeouts, response handling) that's premature without a
   second product wanting it.

2. **Add a dispatcher pre-loop hook** at `agentic-loop` that the
   product shell can install for specific roles. Generic but
   intrusive; requires a framework-side extension point that
   doesn't exist.

3. **A product-shell `bootstrap_workspace` tool the builder
   calls as iteration 1.** Self-contained; no upstream
   dependency; documented migration target. Costs one iteration
   of the 8-budget and one extra tool surface (3 → 4: bash,
   read_loop_result, builder_decide, bootstrap_workspace).

We picked option 3.

### Why bash cannot subsume bootstrap_workspace

Per Coby's "fewer rich tools" principle (feedback memory
2026-05-03), net-new product-shell tools must clear a
"bash-genuinely-insufficient" bar. Bash inside the sandbox
cannot:

- Reach the host filesystem to read the rendered spec markdown
  at `$SEMTEAMS_DEVVIASPEC_ARTIFACT_DIR/<slug>.md` — sandbox
  workspaces are isolated from the host fs.
- POST `/worktree` to create the workspace itself — upstream
  `BashExecutor` requires an existing worktree (the integration
  test in `cmd/semteams/sandbox/integration_test.go:54-58`
  documents this contract: "Upstream's BashExecutor doesn't
  auto-create; the framework's rule layer (R3.6.2.d) will fire
  CreateWorktree on builder spawn.").
- PUT `/file` from outside the sandbox network without the
  `SANDBOX_URL` credential and HTTP client.

All three operations need host-side execution before bash inside
the sandbox is even reachable. bootstrap_workspace closes that
gap; bash takes over from iteration 2 onward.

### What R3.6.2.d ships

- `cmd/semteams/tools/bootstrapworkspace/{doc.go,executor.go,executor_test.go}`
  — single-tool executor with the `spec_path` arg, path-traversal
  guard against `SEMTEAMS_DEVVIASPEC_ARTIFACT_DIR`, idempotent
  worktree create + SPEC.md seed via upstream `sandbox.Client`.
  Full TDD coverage (missing args, traversal, lookalike-prefix,
  file-not-found, sandbox failures, idempotent re-create).
- `cmd/semteams/product_tools.go` — `registerBootstrapWorkspace`
  skipped when `SANDBOX_URL` is unset (the dev-via-spec-builder
  loop is non-functional without a sandbox; same env var the
  upstream BashExecutor already consumes, so an unset value
  disables the entire builder slice consistently).
- `configs/rules/dev-via-spec/06-architect-emit-to-builder.json`
  — fires on `agent.loop.role=dev-via-spec-architect` AND
  `coordinator.next_action=seed_requirements_emitted` AND
  `dev_via_spec.artifact.path` exists. Spawns the builder via
  `publish_agent` with tools `[bootstrap_workspace, bash,
  read_loop_result, builder_decide]` and the rule's prompt
  substitutes `$entity.triple.dev_via_spec.artifact.path` so the
  LLM sees the literal path.
- `configs/e2e-dev-via-spec.json` + `configs/osh-demo.json` —
  rules_files appended; allowed_tools extended with the three
  builder-side tools.
- `configs/personas/fragments/dev-via-spec-builder/{00-identity,
  10-bash-iteration-contract}.md` — updated to reflect the new
  4-tool surface (bootstrap_workspace added) and the
  iteration-1 setup contract; the missing-SPEC.md boot-failure
  terminal is reframed as a missing-bootstrap-workspace failure.
- `cmd/semteams/tools/README.md` — tool-table row +
  migration-target pointer back to this addendum.

### Migration target

`bootstrap_workspace` is product-local with a documented exit
condition: when upstream ships `http_request` rule action **plus**
`${rule.spawned_task}` substitution (or equivalent — anything
that lets a rule sequence "POST /worktree → PUT /file →
publish_agent" inline with the new task_id known to the first
two actions), this tool deletes:

- The rule's on_enter array gains two `http_request` entries
  before publish_agent.
- The persona's iteration 1 becomes its real first iteration.
- The 8-iteration budget recovers one slot.

When that day comes, this addendum is the historical record of
why we shipped product-local first.

R3.6.2.e (mock-LLM fixture exercising the chain through
builder terminal) and R3.6.2.f (smoke #6, real-LLM
OSH-Java-Maven bare seed) remain on the roadmap.

## Addendum 2026-05-03 — R3.6.1.1 wire-format conformance + read_only_paths

R3.6.1.1 is a follow-up slice driven by two findings made AFTER R3.6.1
merged:

### Framework-alignment finding

semstreams beta.36 ships `processor/agentic-tools/sandbox.Client` and
`BashExecutor`. The BashExecutor routes through the client when
`SANDBOX_URL` is configured. ADR-029 framework-alignment review fires:
**use upstream's pattern, do not fork a parallel one in the product
shell.**

The wire format upstream's client expects differs from R3.6.1's
REST-style endpoints:

| Operation | R3.6.1 (REST) | R3.6.1.1 (upstream-conformant) |
|---|---|---|
| Create | `POST /workspace/{chainID}` | `POST /worktree` (body has task_id) |
| Delete | `DELETE /workspace/{chainID}` | `DELETE /worktree/{taskID}` |
| Exec | `POST /workspace/{chainID}/exec` | `POST /exec` (body has task_id) |
| Write | `PUT /workspace/{chainID}/files` | `PUT /file` (body has task_id+path+content) |
| Read | `GET /workspace/{chainID}/files?path=…` | `GET /file?task_id=…&path=…` |
| Archive (zip) | `GET /workspace/{chainID}` | `GET /worktree/{taskID}/archive` (product-shell extension; upstream client doesn't use it) |

R3.6.1.1 conforms to the upstream shape so upstream's `BashExecutor`
reaches our sandbox via `SANDBOX_URL` without an adapter. Internal
vocabulary tracks the wire (`taskID`, `taskMutex`, `resolveTaskPath`,
`isValidTaskID`).

### Karpathy-Loop autoresearch feature: read_only_paths

A parallel design conversation surfaced this feature (timely overlap
with the conformance pass). The autoresearch use case: declare a
"frozen baseline" the agent's iterations cannot mutate.

**Design (lives in sandbox; bash tool stays simple per "fewer rich
tools" principle):**

- `POST /worktree` body accepts optional `read_only_paths: []string`
  (workspace-relative; `..`/abs/empty rejected at create).
- Sandbox stores `read_only_paths` as in-memory metadata per task.
- After every successful `PUT /file` and `POST /exec`, sandbox
  applies a chmod sweep: existing read_only_paths get `555` (dirs)
  and `444` (files), recursively. Idempotent.
- `PUT /file` to a chmod-frozen target returns `403` with a clear
  "read-only" message (mapped from EACCES).
- Bash exec hitting a chmod-frozen target returns non-zero with
  EACCES in stderr. Bash naturally surfaces the OS error — no
  command-string parsing required.

**Why filesystem-level instead of pre-execution refusal:** parsing
arbitrary bash for "would this write to read_only_paths" is fragile
with false negatives (sed -i, python writes, here-docs, eval). chmod
catches every category structurally. The autoresearch threat model
is "experimental iterations don't accidentally mutate the baseline,"
not "actively malicious agent escaping" — chmod is the right level.

**Lifecycle:**
- Empty workspace + read_only_paths at create: paths recorded; no
  immediate chmod (paths don't exist).
- Bash seeds the baseline (e.g., `cp -r /seed/* baseline/`); after
  exec, sandbox chmods the now-existing baseline tree.
- Subsequent iterations see frozen baseline; writes inside fail.

**DELETE handling:** `removeWorkspace` runs `chmodTreeWritable` first
so `os.RemoveAll` succeeds across chmod-locked subtrees. Without this,
delete would fail EACCES on read_only_paths.

### Git init at create time

Workspaces now run `git init -b main` + `git config user.{email,name}`
+ `git commit --allow-empty -m initial` at create. Required for the
upstream PR's planned `verify_clean` flag to have a meaningful baseline
(`git status --porcelain` against the initial commit).

### Upstream PR scope (separate from this slice)

Two additive changes proposed to semstreams:

1. `sandbox.Client.CreateWorktree` accepts `read_only_paths []string`
   in the request body — pure pass-through to the sandbox.
2. `BashExecutor` adds `verify_clean: bool` argument; when true, the
   tool runs `git status --porcelain` via a precondition exec and
   errors if the tree has uncommitted changes outside read_only_paths.

Both are net-additive; existing callers see no behavior change.

### Why no product-shell write_file/read_file tools

Bash subsumes file ops (`echo > file`, `cat file`, `sed -i …`). The
LLM is trained heavily on bash; small models degrade with more tools.
Per the "fewer rich tools" principle, we don't expose `write_file` /
`read_file` to the LLM. Sandbox still exposes them at the HTTP layer
for non-bash callers (rule actions, future product-shell tools that
genuinely need typed wire shapes).

## Addendum 2026-05-03 — R3.6.2.f smoke #6 outcome

Smoke #6 ran the full R3.6 chain on real LLM (claude-sonnet at low
effort, no cloud-Gemini due to SemSpec saturation). Prompt: *"create
a driver for OpenSensorHub using OGC Connected Systems for Meshtastic
devices"*. The chain ran end-to-end through the new builder slice
(R3.6.2.b through .e); this addendum captures the calibration
findings the rest of R3.6 was waiting on.

### Convergence

15 loops in ~10 minutes wall-clock:

| # | Time | Role | Notes |
|---|---|---|---|
| 1 | 21:20:26 | researcher (dispatch) | 20×http_request + 5×web_search; emitted artifact rev=1 |
| 2 | 21:22:37 | research-reviewer | decided `insufficient` (corpus gap) |
| 3 | 21:23:12 | researcher-with-source-acquisition | add_source_repo + emit rev=2 |
| 4 | 21:23:55 | research-reviewer | decided `approved` — **no stabilisation pass needed** |
| 5 | 21:24:08 | dev-via-spec-planner | `planned` |
| 6 | 21:24:37 | dev-via-spec-reviewer | `approved` |
| 7 | 21:25:00 | dev-via-spec-challenger | **`concerns_raised` (rule 04)** |
| 8 | 21:25:25 | dev-via-spec-planner (retry) | `planned` |
| 9 | 21:26:01 | dev-via-spec-reviewer | **`insufficient` (rule 02)** |
| 10 | 21:26:30 | dev-via-spec-planner (retry) | `planned` |
| 11 | 21:26:53 | dev-via-spec-reviewer | `approved` |
| 12 | 21:27:15 | dev-via-spec-challenger | `accept` |
| 13 | 21:27:39 | dev-via-spec-architect | emit_dev_via_spec_artifact + decide(seed_requirements_emitted) |
| 14 | 21:28:11 | **dev-via-spec-builder** | bootstrap_workspace + 13×bash; force-failed at iter 15 |

The chain exercised retry paths the mock fixture skipped: rule 02
(reviewer-rejected → planner retry) AND rule 04 (challenger-concerns
→ planner retry) both fired once. Real LLM is more rigorous than the
mock.

The architect's emit_dev_via_spec_artifact rendered
`docs/specs/2026-05-03-meshtastic-lora-transport-driver.md`. Rule
06's `$entity.triple.dev_via_spec.artifact.path` substitution
resolved correctly; bootstrap_workspace seeded SPEC.md into the
builder's sandbox worktree on first call. End-to-end wiring verified.

### What the builder produced (in 15 iterations)

```
pom.xml
SPEC.md
src/main/java/com/semstreams/meshtastic/
  codec/{ProtobufCodec,DefaultProtobufCodec}.java
  config/MeshtasticDriverConfig.java
  connection/{ConnectionException,ConnectionHandle,ConnectionProvider}.java
  envelope/{MessageEnvelopeLayer,DefaultMessageEnvelopeLayer}.java
  transport/{TransportContract,InboundMessage,OutboundMessage,
             MeshtasticTransportDriver}.java
src/test/java/com/semstreams/meshtastic/
  MeshtasticTransportDriverTest.java   (17 unit tests)
```

12 production sources + 1 test class with 17 unit tests. Reasonable
package decomposition (codec/config/connection/envelope/transport).
**`mvn compile` against this tree returns BUILD SUCCESS.** **`mvn
test` reports `Tests run: 17, Failures: 1, Errors: 0`** — one test
asserts `IllegalArgumentException` for null inputs while the
implementation throws `NullPointerException` via
`Objects.requireNonNull`. Trivial test-vs-impl mismatch the
builder would have fixed on its next iteration.

### Calibration: max_iterations is empirically too tight

This is the load-bearing finding. **§15 locks `max_iterations` at 8;
osh-demo's agentic-loop default is 15; the builder needed >15 just
to finish writing source + tests, before any verification.** Each
bash heredoc (`cat > path << 'EOF' … EOF`) consumes one iteration
per file. With 13 files needed, the budget exhausts before
`mvn compile` even runs. The builder force-failed at iter 15 with
`state=failed`; no `builder_decide` was emitted because the
framework killed the loop first.

Workspace persistence held — we ran `mvn compile` and `mvn test`
manually post-failure to learn what the LLM had produced. That
forensics path is invaluable; smoke #6 would have been much
harder to read without it.

**Calibration recommendation (amends §15):**

- Bump default `max_iterations` for the dev-via-spec-builder role
  from 8 → **30**.
- Allow per-rule override up to **50** for OSH-class workloads,
  matching semspec's empirical ceiling.
- Implementation: rule 06 (`06-architect-emit-to-builder.json`)
  should set `max_iterations: 30` explicitly on the publish_agent
  action so the builder gets a budget independent of the
  agentic-loop component default. (Rule action support for
  per-spawn `max_iterations` may need an upstream change — verify
  beta.39's publish_agent payload accepts it; if not, raise as a
  small upstream PR.)

§15's original "8 (bumped from initial 3 default — bare seed means
more iteration on Maven/OSGi config)" rationale was directionally
right but quantitatively too low by ~4×. Smoke #6 surfaced the gap
the §15 author couldn't have without observation.

### Calibration: persona could batch file-writes

Iteration drain is dominated by one-file-per-bash-call. The builder
persona doesn't currently encourage batching. A heredoc can carry
multiple `cat > path << 'EOF' … EOF` blocks in one bash invocation
— the persona's
`configs/personas/fragments/dev-via-spec-builder/10-bash-iteration-contract.md`
shows a single-file example. Updating the example to demonstrate
multi-file batching could halve iteration consumption, decoupling
budget from file count.

This is a persona refinement, not a sandbox or rule change. Defer
to a follow-up R3.6.2.g (or fold into the §15 calibration PR).

### Calibration: Open #19 (model tier)

`claude-sonnet` at low effort produced **compilable Java** with
**16/17 passing tests on first attempt** — without seeing test
results, without iterating. Architectural decomposition was sound.
**No upgrade to opus or higher reasoning_effort needed for OSH-class
workloads.** Open #19 closes: lock claude-sonnet, low effort.

### Calibration: Open #13 (egress allow-list)

Researcher hit (in order):
- `github.com`, `raw.githubusercontent.com` (OSH source files)
- `meshtastic.org`, `python.meshtastic.org`, `buf.build`
  (Meshtastic protocol docs + protobuf schemas)
- `opengeospatial.github.io`, `docs.ogc.org` (OGC Connected
  Systems specs)

All hosts are on §13's day-one allow-list (or `github.com`
subdomains). **No unexpected egress.** §13's allow-list draft
holds for OSH-class research; smoke #6 doesn't argue for tightening
or expanding it.

### Calibration: Open #14 (per-workspace disk quota)

Builder workspace post-failure: 13 source files + Maven `target/`
directory. Total <1 MB. `mvn compile` populated `target/classes`
with 12 `.class` files. **No quota concern.** §14's deferral holds
— if a future smoke produces multi-GB artifacts (e.g. heavy
generated code), revisit; OSH-Java-Maven bare seed doesn't.

### Cost

Approximately 15 loops × ~10K tokens average = ~150K tokens at
claude-sonnet pricing (~$3/M input, $15/M output). Total run
cost: estimated **$2–3**. Acceptable for a calibration smoke.

### Drift signals captured (summary)

1. **Iteration budget too tight** (loadbearing): bump 8 → 30
   default, 50 ceiling. Either calibrate §15 or override on rule 06.
2. **Persona batching opportunity**: multi-file heredocs reduce
   iteration drain.
3. **Mock fixture under-tests retry paths**: real LLM hit rule 02
   AND rule 04; mock fixture skipped both. Either expand the mock
   fixture or accept that smoke #6 is the canonical retry-path
   coverage.
4. **Forensics workflow proven**: post-failure workspace inspection
   via `docker exec` + manual `mvn` is essential and worked. The
   sandbox's worktree persistence (until DELETE) is what makes this
   possible; do not change that lifecycle.
5. **Wire-format end-to-end verified**: rule 06 →
   bootstrap_workspace → bash → architect's spec content reaches
   the builder workspace correctly. No bugs in the
   architect→builder handoff.

### What R3.6.2.f does NOT decide

- Whether the calibration above (`max_iterations: 30`) gets
  applied as a follow-up slice (R3.6.2.g) or a §15 amendment in
  this ADR. **Recommend** a follow-up slice so the persona, rule,
  and ADR change land coherently with a single re-run smoke for
  verification.
- Whether the persona should be updated for batching. **Recommend**
  yes; bundle with the calibration follow-up.
- Whether to add a builder-retry rule (analogous to planner retry on
  reject) when the framework force-fails. **Defer** until we see how
  often the budget bump alone gets the chain to terminal.

R3.6 is functionally complete with the budget caveat captured.
Closeout follow-up: budget bump + persona batching + smoke #6 re-run
to confirm `tests_passing` terminal.

## Addendum 2026-05-03 — R3.6.2.g closeout: smoke #6 re-run, tests_passing confirmed

R3.6.2.g applied the calibration recommendations from R3.6.2.f and
re-ran smoke #6 against real LLM. **The chain converged to
`builder_decide(tests_passing)` with 18/18 unit tests green.** R3.6
fully closed.

### Calibration applied

- `configs/osh-demo.json`: `agentic-loop.max_iterations` 15 → **30**;
  `agentic-loop.timeout` 300s → **1200s** (and `consumer.ack_wait`
  matched). The timeout bump was a lesson from a first re-run attempt
  that hit the 300s wall during `add_source_repo` approval delay
  (paid LLM is fast, but the human-in-the-loop approval gate makes
  per-loop wall-clock unpredictable; 1200s gives operators headroom).
- `configs/personas/fragments/dev-via-spec-builder/10-bash-iteration-contract.md`:
  iteration budget callout updated 8 → 30; Step 3 example rewritten
  to demonstrate **multi-file heredoc batching** in a single bash
  call (one `mkdir` + N `cat > path << 'EOF' … EOF` blocks). Each
  bash invocation costs one iteration; batching trades a long heredoc
  for many separate calls.
- ADR §15 (Decided table): `max_iterations` cap recalibrated 8 → 30
  with rationale + ceiling 50 matching semspec; explicit note that
  beta.39's `rule.Action` doesn't expose per-rule
  `max_iterations` override (bump applies at agentic-loop component).

### Smoke #6 re-run convergence

11 loops in ~12 minutes wall-clock (same chain shape as R3.6.2.f
but converging cleanly):

| # | Time | Role | State / outcome |
|---|---|---|---|
| 1 | 21:56:23 | researcher (dispatch) | http_request + web_search + emit rev=1 |
| 2 | 21:58:22 | research-reviewer | `insufficient` (corpus gap) |
| 3 | 21:58:50 | researcher-with-source-acquisition | add_source_repo (auto-approved) + emit rev=2 |
| 4 | 22:01:21 | research-reviewer | `approved` (no stabilisation pass) |
| 5 | 22:01:46 | dev-via-spec-planner | `planned` |
| 6 | 22:02:24 | dev-via-spec-reviewer | `insufficient` (rule 02) |
| 7 | 22:02:56 | dev-via-spec-planner (retry) | `planned` |
| 8 | 22:03:29 | dev-via-spec-reviewer | `approved` (rule 03) |
| 9 | 22:03:57 | dev-via-spec-challenger | `accept` (rule 05) |
| 10 | 22:04:23 | dev-via-spec-architect | emit_dev_via_spec_artifact + decide |
| 11 | 22:04:49 | **dev-via-spec-builder** | **builder_decide(tests_passing) at iter 9 of 30** |

### Builder's actual trajectory (iter 1–9)

```
22:04:51 bootstrap_workspace
22:04:53 bash cat SPEC.md
22:05:02 bash mkdir + write source files (batch 1)
22:06:05 bash write more source files (batch 2)
22:06:52 bash write test files (batch)
22:06:55 bash mvn -B -ntp compile         ← BUILD SUCCESS
22:07:15 bash mvn -B -ntp test            ← Tests run: 18, Failures: 0
22:07:27 bash cat target/surefire-reports/*.txt
22:07:37 builder_decide(tests_passing, tests_run=18, tests_passed=18)
```

**Persona batching cut iteration drain by ~75%.** R3.6.2.f burned
13 bash calls writing one file per call before failing at iter 15
without reaching `mvn`. R3.6.2.g wrote everything in 3 batched bash
calls and reached `mvn test` at iter 7. Net: 9 iterations to a
compiling, fully-tested driver.

### What the builder produced

`builder_decide` payload:

```json
{
  "action": "tests_passing",
  "reason": "MeshtasticSensorModule (OSH AbstractSensorModule subclass)
             implements all 5 SRs — plugin manifest (SR1),
             MeshtasticTransport serial/TCP connect/reconnect loop (SR2),
             module lifecycle init/start/stop orchestration (SR3),
             IProcessChain protobuf-to-O&M DataBlock wiring + SensorML
             schema registration (SR4), and CS API publish-path
             smoke/integration test (SR5); surefire reports 18/18
             tests passing across 4 test classes.",
  "tests_run": 18,
  "tests_passed": 18,
  "tests_failed": 0,
  "artifact_summary": "Maven JAR osh-meshtastic-driver-1.0.0.jar; 11
                      main-source files (MeshtasticSensorModule,
                      MeshtasticTransport, MeshPacketProcessChain,
                      IProcessChain, AbstractSensorModule,
                      MeshtasticConfig, MeshPacket, DataBlock,
                      DataComponent, PacketListener, TransportException)
                      + META-INF/osh-module.properties; 4 JUnit 5 test
                      classes covering SR1–SR5 integration points."
}
```

11 production-source files + manifest + 4 test classes (18 tests
total: MeshPacketTest 1, MeshPacketProcessChainTest 4,
MeshtasticTransportTest 6, MeshtasticSensorModuleTest 7). All
passing on first build. The test/impl null-handling mismatch from
R3.6.2.f didn't recur — the LLM produced more careful contract
matching this run.

### Drift signals captured (R3.6.2.g additions to R3.6.2.f's list)

1. **Per-loop timeout × approval-gate latency** (from the failed
   first re-run attempt). 300s timeout fires while researcher is
   blocked waiting for human approval of `add_source_repo`. Bumped
   to 1200s; for production, a coordinator-as-source-approver
   pattern (see #4 below) is the right structural fix.
2. **Researcher LLM called wrong tools** — the dispatch researcher
   in this run made 1 `bootstrap_workspace` call and 1 `bash` call
   despite those being builder-only tools. `agentic-tools.allowed_tools`
   is global (all tools available to all roles); the persona is
   what tells the role what it should/shouldn't use. Real LLMs
   sometimes pick from the global set anyway. Calls failed
   silently (bootstrap_workspace got no spec_path; bash got an
   echo) and the chain recovered. Consider per-role
   `allowed_tools` enforcement at the framework level (or simply
   drop builder-only tools from the global allowed_tools and rely
   on rule-level tool injection per role).
3. **add_source_repo namespace mismatch on first attempt**. LLM
   first called with `namespace=default` which isn't in the
   per-deployment allowlist (`SEMTEAMS_SEMSOURCE_NAMESPACES=research`).
   Tool rejected without surfacing the allowlist hint. LLM
   self-corrected on retry with `namespace=research`. Consider
   surfacing the allowed-namespace list in the tool's error
   response so the LLM doesn't waste an iteration discovering it.
4. **Coordinator-as-source-approver** (Coby's framing during the
   monitoring session). In a fully autonomous deployment, the
   coordinator role should hold the approval policy for
   `add_source_repo` (and other gated tools). Coordinator inspects
   URL host, namespace, prior corpus state, and the originating
   prompt's intent; approves or rejects with a documented reason.
   Replaces the current "human in the loop" pattern AND replaces
   ad-hoc bash auto-approvers used for smokes. Future ADR-027
   Phase 2 extension or a focused R3.7 slice. Logged for follow-up.

### What remains in R3.6 scope

Nothing. R3.6 is closed. R3.6.2.b (validator) + .c (persona) +
.d (rule + bootstrap) + .e (mock fixture) + .f (smoke + calibration
findings) + .g (calibration applied + smoke confirmed) — all
shipped. Drift signals 2–4 above are out-of-scope for R3.6 and
become independent slices.

### Cost

~$2–3 for ~12 minutes of real-LLM activity at claude-sonnet/low
effort. 11 loops × ~10K tokens average. Same as R3.6.2.f. Persona
batching had no cost impact (same total LLM work, fewer round-trips
on the builder side).

## References

- ADR-031 — research-flow + dev-via-spec; R3.4 closeout addendum
  defines what R3.4 produces vs what R3.6 must produce.
- `project_sandbox_planning_open.md` (memory) — the framing
  conversation 2026-05-02 captured before this ADR existed.
- `project_r35_coordinator_meta_reviewer.md` (memory) — R3.5 design
  the builder's `needs_clarification` slot anticipates.
- `feedback_format_compliance_goodhart.md` (memory) — the
  substance-over-format pivot from R3.4b. Same shape applies to
  builder persona contracts: don't ask for output in a particular
  format; ask for substance, let deterministic tools enforce
  structure.
- Semspec sandbox: `~/Code/c360/semspec/cmd/sandbox/server.go`,
  `docker/sandbox.Dockerfile`, ADR-008 (worktree DAG execution).
- Semdragon sandbox: `~/Code/c360/semdragon/cmd/sandbox/server.go`,
  `docker/sandbox.Dockerfile`, `docker/compose.yml`,
  `docs/08-SANDBOX-REPOS.md`, ADR-002 (party quest DAG execution).
