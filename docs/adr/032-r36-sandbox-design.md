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
| 15 | **Builder terminal contract** | **`decide(action, ...)` with action ∈ {tests_passing, tests_failing, needs_clarification}** and required evidence fields. `max_iterations` cap **8** (bumped from initial 3 default — bare seed means more iteration on Maven/OSGi config). | Validator rejects terminal payloads missing required fields:<br>• `tests_passing`: `tests_run` (must be >0), `tests_passed`, `tests_failed`, `artifact_summary`<br>• `tests_failing`: `tests_run`, `tests_failed`, `failure_summary`, `retry_hint`<br>• `needs_clarification`: `reason`, `blocking_question` — slot baked in for R3.5 rule landing additively.<br>Closes the "exit 0 with no tests" loophole; pre-bakes R3.5. |
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
