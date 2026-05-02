# ADR-032: R3.6 Builder Sandbox Design

## Status

**Proposed — 2026-05-02.** This ADR captures the planning conversation
around SemTeams's R3.6 sandbox before any code lands. The decision
space is mapped; defaults are proposed; open questions are flagged
explicitly. Implementation requires a follow-up ADR (or an addendum
to this one) once decisions are locked.

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

## Decision space

Each row below is a decision SemTeams's R3.6 sandbox needs to make.
Status is one of:

- **Decided**: default chosen; alternative ruled out with reason.
- **Proposed**: default proposed; alternative not ruled out; flag
  for the next conversation.
- **Open**: genuinely open; needs deliberate choice before R3.6
  starts.

### Decided

| # | Decision | Choice | Rationale |
|---|---|---|---|
| 1 | **Container model** | One long-lived shared container, like both sibling repos | Container-per-task is a blast-radius win we don't need at our scale. Single container is simpler and matches both upstream patterns. |
| 2 | **Workspace persistence** | **Per-chain named Docker volume** at `/workspace/{chain_id}/`. Mirrors semdragon. | Semspec's worktrees-in-repo model trades operational simplicity for a parallel-DAG capability we don't have. Docker volume = no orphan cleanup, no manual grooming. |
| 3 | **Git mutex strategy** | **Per-repo mutex** (`map[string]*repoMutex`), not global. Mirror semdragon. | Even with no parallelism today, per-repo is the simpler API and dodges the "what if we add a second concurrent chain later" concern. Costs ~10 lines vs ~zero. |
| 4 | **Reconciliation state machine** | **Skip.** Don't port from semspec. | Reconciliation exists to handle wedged-repo state under concurrent merges. Our serial chain produces zero merges (the builder writes to its own quest workspace; we don't merge to a shared `main` until the chain is fully terminal — and even then, manually-reviewed). |
| 5 | **`taskIDMain` special case** | **Skip.** Don't port. | semspec's read-only `main` access for plan/reviewer agents is parallel-DAG architecture cruft. Our planner / reviewer / challenger don't read from a workspace at all — they consume prior loop results via `read_loop_result`. Builder gets its own workspace. |
| 6 | **Resource limits** | **Hard-cap from compose**, mirroring semdragon: 2 CPU, 4 GB RAM, fd 4096/8192. | Production-ready posture. Cheap to land, expensive to retrofit. |
| 7 | **Security hardening** | **`cap_drop: ALL` + minimal `cap_add` for git ops; `no-new-privileges: true`; read-only root + tmpfs.** Mirror semdragon. | Same posture as semdragon's compose. Land it on day one. |
| 8 | **Toolchain in the image** | **Match semdragon's set: Go, Java (Maven + Gradle), Node, Python, plus protoc for OSH driver target.** | Both sibling repos converged on roughly this set. OSH adds Protobuf decode, so include `protoc` and `protoc-gen-go` (or the Java-specific equivalents) in the image. |
| 9 | **Cleanup model** | **Quest-scoped + explicit `DELETE /workspace/{chain_id}`.** No periodic orphan collector. | Aligns with semdragon. Named volume + explicit lifecycle = no manual grooming. |
| 10 | **Workspace forensics** | **`GET /workspace/{chain_id}` returns a zip download** (mirror semdragon's pattern). | Cleaner than host-filesystem inspection (which is what semspec uses); ergonomic for the demo's "what did the chain produce on this run" question. |

### Proposed (default chosen but worth a second look)

| # | Decision | Default | Alternative | When to revisit |
|---|---|---|---|---|
| 11 | **Test runner** | **`bash` only**, like both sibling repos. Builder calls `bash mvn test` (or `go test ./...` for a Go target). | Typed `run_tests` tool that constrains argument shape. | Revisit if we observe builder LLM consistently misusing bash for tests (e.g. forgetting `-e`, missing exit codes). Until then, `bash` is the simpler primitive. |
| 12 | **Approval gate on bash/write_file** | **No, auto-fire** (mirror both sibling repos). The builder's tools fire without per-call human approval. | Approval-gate every `bash` and `write_file`, mirroring `add_source_repo`'s ADR-030 pattern. | Revisit when (not if) the chain attempts something destructive. We need at minimum a deny-list — see #16 below. |
| 13 | **Network policy** | **No outbound restrictions on day one** (matches both sibling repos). | Egress allow-list (Maven Central, npm registry, GitHub, the configured SemSource); deny everything else. | This is the highest-priority "open" item. Track separately. See #17 below. |
| 14 | **Per-workspace disk quota** | **No quota on day one** (matches sibling repos). | Hard quota via `tmpfs size=` per workspace, or per-volume size limit. | Revisit if a runaway build fills the volume. Probably worth pre-empting — it's cheap. |
| 15 | **Builder terminal contract** | **`decide(action="tests_passing", artifact_summary=...)` or `decide(action="tests_failing", reason=..., retry_hint=...)`** with `max_iterations` cap (start at 3 retries on `tests_failing`). | Allow `decide(action="needs_clarification", reason=...)` from R3.5 the moment a real ambiguity surfaces. Pre-bake the slot in the contract. | This is exactly the case R3.5 was designed for. Recommend including the `needs_clarification` action in the builder's terminal enum from day one even though the rule that handles it is R3.5+. |

### Open (deliberate decisions required before R3.6 starts)

| # | Decision | Question | What it depends on |
|---|---|---|---|
| 16 | **Tool deny-list / safety boundary** | Are there commands or paths the builder must NEVER run? `rm -rf /`, `git push --force`, anything writing outside `/workspace/{chain_id}/`, anything reading from another chain's workspace? Both sibling repos trust the agent + tier-gating; neither has a deny-list. | Risk appetite. We probably need at least: refuse `rm -rf` against paths outside the workspace, refuse network operations to non-allow-listed hosts (#17), refuse writes outside the chain's own workspace. |
| 17 | **Network policy strategy** | Egress allow-list (most secure but operational pain), deny-list (some egress allowed, security surface), or open (matches sibling repos). Open is the easiest to ship; allow-list is the right end-state for production. | Whether we're production-targeting day one or demo-targeting. Demo can ship open; production must have at minimum a deny-list. |
| 18 | **Target choice: OSH-Java-Maven vs lighter target** | The OSH-class arc the spec produces is Java + OSGi + AbstractSensorModule. That's real toolchain weight (Maven dependency resolution, OSGi bundle metadata, Java ecosystem variability). A Go service that bridges MQTT to OGC CS REST has the same architectural shape but ~10× lighter toolchain. | Demo authenticity vs iteration speed. Coby's instinct on 2026-05-02: *"staying with OSH because the early-adopter is comparing us to BMAD/OpenSpec on real targets — switching to a Go toy would smell like dodging the hard case."* But Maven dependency resolution alone could turn into the kind of pain semspec hit on worktree merges. **This is the most important open question.** |
| 19 | **Builder model tier** | The builder writes code; that's substantively different from spec drafting. Sonnet at low effort handled the spec arc. Does code-writing want sonnet at higher reasoning_effort, or claude-opus, or local seminstruct:8b for cost? | What we observe in the first builder smoke. Default to sonnet at low effort to match the chain's existing tier; bump if drift. |
| 20 | **Spec-to-builder handoff format** | The architect emits a typed `dev_via_spec.artifact.v1` payload + the markdown. The builder needs to read that. Does it consume the typed payload via `query_entity` (typed, structured), the markdown via `read_loop_result` (prose, what the LLM produced), or both? | LLM ergonomics. Probably both: typed for routing decisions, markdown for human-comprehensible context. |
| 21 | **R3.5 `needs_clarification` slot** | If R3.5 isn't shipped before R3.6 starts, do we still bake the action into the builder's terminal enum (so the rule can land additively later)? | Yes — recommended in #15 above. Cost is one enum value the rule layer doesn't yet match on. Forward-compatibility is cheap. |
| 22 | **Test-pass criterion** | `mvn test` exit code 0 = "tests passing"? `go test ./...` returning success? An LLM-judged "this output looks like passing tests"? | Deterministic-vs-judgment dial we keep facing. Default: **deterministic exit code**. Reach for LLM judgment only when the deterministic check is genuinely ambiguous. |

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
- Endpoints: `POST /workspace/{chain_id}` (create), `POST /workspace/
  {chain_id}/exec` (bash), `POST /workspace/{chain_id}/files`
  (read/write), `POST /workspace/{chain_id}/merge-to-main` (the
  chain-terminal merge), `GET /workspace/{chain_id}` (zip download),
  `DELETE /workspace/{chain_id}`.
- Per-repo mutex; no reconciliation flag; no taskIDMain.
- Toolchain: Go, Java (Maven + Gradle), Node, Python, protoc.
- Tests: server-level integration tests under `test/integration/sandbox/`
  exercising create / exec / merge / cleanup.
- Decision lock: items #1–#10 (Decided table) + open-item #18 (target
  choice) — until we know OSH-Java vs Go target, the toolchain
  decision is half-pending.

### R3.6.2 — sandbox tool + builder persona stub

- Product-shell tools: `write_file`, `read_file`, `run_command` (or
  reuse upstream's `bash` and add a thin `write_file` we own).
  Allow-listed in the chain's `agentic-tools.allowed_tools`.
- New persona: `configs/personas/fragments/dev-via-spec-builder/*`.
  Reads the spec artifact (typed payload + markdown), iterates
  `read_file` / `write_file` / `bash` until tests pass or budget
  exhausted.
- New rule:
  `configs/rules/dev-via-spec/06-architect-emit-to-builder.json`
  fires on `coordinator.next_action="seed_requirements_emitted"`
  AND a marker triple from the artifact emit (e.g.
  `dev_via_spec.artifact.path` exists). Spawns the builder.
- Mock-llm fixture exercising the chain through builder terminal
  with a stubbed test run. Smoke #6 (real-LLM) follows.
- Decision lock: items #11–#15 (Proposed table) + open-items
  #19, #20, #22.

### R3.6.3 — production-readiness pass

- Decision lock: items #16, #17 (security boundary + network policy).
- Add deny-list, network allow-list, disk quota.
- ADR addendum capturing what the first real-LLM builder smoke
  produced and any pivots needed.

## What this ADR explicitly does NOT decide

- **Whether R3.5 ships before R3.6.** R3.5 was designed in
  ADR-031 §addendum 2026-05-02. Smoke #5 converged without it.
  Recommend deferring until R3.6 surfaces actual ambiguity (which is
  likely — the builder hits "is this scope creep or seed-requirement-
  implied?" more than the planner does). Pre-bake the
  `needs_clarification` action in the builder's terminal enum (#21)
  so R3.5 can land additively.
- **The OSH-vs-Go target choice (#18).** This is a deliberate
  conversation, not a default-and-forget item. It blocks R3.6.1 only
  on toolchain choice, but it shapes everything downstream — the
  builder persona's prompt, the test-runner expectations, the spec
  artifact's grounding fidelity, the "demo authenticity" claim.
- **The exact name and product-shell location of the sandbox
  component.** R3.6.1 design will pick this; survey the
  framework-alignment ADR-029 patterns first.

## Acceptance criteria for this ADR moving from Proposed → Accepted

1. Decision rows #1–#10 (Decided) reviewed and either confirmed or
   contested. Default: accept as proposed.
2. Open items #16–#22 each get either a "decide now" answer or an
   explicit "deliberately deferred to R3.6.X" assignment.
3. Phasing reviewed; nothing pulled forward (no PR that tries to do
   R3.6.1 + R3.6.2 in one shot).
4. Coby reads it; pushes back on what doesn't smell right.

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
