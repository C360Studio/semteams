# Smoke #7 — OSH-Meshtastic real-LLM playbook

R3.7.2.l′ of ADR-034: first end-to-end real-LLM run of the
architect → builder → qa-reviewer chain. Designed to surface
concrete shape requirements for the open evidence-summary
integration plumbing (`project_smoke7_open_plumbing.md` §2)
that R3.7.2.k′ stubbed.

## Why this exists

The mock-LLM e2e (`test:e2e:agentic:dev-via-spec-qa`) proves the
chain wires correctly. It does not prove the personas converge on
real prompts, the rule chain handles real-LLM tool-call
variability, or the qa-reviewer reads stubbed evidence the way
the j′ persona contract intends. Smoke #7 is the forcing function
for those questions.

## What this is NOT

- A regression test. Real-LLM runs are too slow + expensive for
  CI. Run manually when a slice's correctness depends on real-LLM
  behaviour.
- A substantive proof of evidence grading. The spawn-rule for
  the qa-reviewer (`07-builder-decide-to-qa-reviewer.json`) embeds
  a literal `(stub)` evidence block per R3.7.2.k′. The qa-reviewer's
  persona-correct verdict against stubbed evidence is
  `decide(needs_clarification)`. That outcome is success for this
  smoke — it tells us the chain plumbed end-to-end. Real evidence
  grading lands in a follow-on slice once the integration plumbing
  shape is decided (see "Findings" below).

## Prerequisites

- `ANTHROPIC_API_KEY` exported in the shell that boots the stack.
  The compose file forwards it to the backend container; the
  dev-via-spec chain (`general` capability) routes through
  claude-sonnet-4-6 with claude-haiku-4-5 fallback.
- Docker running with at least 8 GB free (qwen3-8b alone is ~5 GB).
- First run pulls `ghcr.io/c360studio/seminstruct:qwen3-8b` and
  `ghcr.io/c360studio/semembed:latest` — allow ~5 minutes for
  initial boot.
- Sandbox image built locally from `docker/sandbox.Dockerfile`
  (the dev-via-spec-builder routes `bash` + `bootstrap_workspace`
  through the sandbox container). Build it with:

  ```bash
  docker compose -f ui/docker-compose.agentic-e2e.yml --profile sandbox build sandbox
  ```

  Or verify a working sandbox by running `task test:e2e:sandbox:dood:smoke`
  before the smoke — same image, faster signal.

## Estimated cost

Rough estimate: ~$2-5 per full run at current Anthropic pricing.
Anchor: only the `general` capability (architect, builder,
qa-reviewer) routes to claude-sonnet-4-6. Internal-graph
capabilities (summarization, query_classification,
intent_classification, etc.) route to local seminstruct/semembed
and don't touch the cloud. Of the ~60 LLM round-trips a full
chain produces, roughly 25-30 are billed. Pricing changes —
check the Anthropic console for actuals after a run. The first
pass is the most expensive; rerunning after a `down -v` reuses
the graph substrate but still pays for fresh dev-via-spec
prompts. Cap your spend by watching `probe:loops` for runaway
iteration.

## Procedure

### Recommended one-task path

If you just want to run the smoke and capture findings, the
single-task entry point handles boot + prompt + watcher + capture
+ teardown. **Run from the project root**:

```bash
cd /path/to/semteams         # if not already there
task ui:test:e2e:agentic:smoke7:run
# or to label the run for the evidence dump:
RUN_ID=ticket-123 task ui:test:e2e:agentic:smoke7:run
```

The project root invocation matters: the root `Taskfile.yml`
loads `.env` via its top-level `dotenv:` directive, which is
how `ANTHROPIC_API_KEY` flows through to docker-compose. Task
forbids `dotenv:` in included Taskfiles, so direct invocation
from `ui/` skips that load and the smoke7 precondition will
fail loud with remediation guidance.

Evidence lands in `/tmp/smoke7-<timestamp>/` (or `/tmp/$RUN_ID/`):

- `wedge-report.txt` — chain-shape diagnosis: per-loop terminal
  action table, final-role match against
  `WEDGE_EXPECTED_TERMINAL_ROLE` (default
  `dev-via-spec-qa-reviewer`), tool-issue tally. **Read this
  first** — it tells you whether the chain reached the
  expected terminal or wedged mid-flight, and at which loop.
- `watcher-trajectory.jsonl` + `watcher-events.log` — the
  auto-approve watcher's per-poll snapshot + role-transition log
  (relocated into the capture dir at exit).
- `loops.json` — final loops snapshot from
  `/teams-dispatch/loops`.
- `trajectory-<loop_id>.json` — one per loop.
- `messages.json` — message-logger entries (subject + payload).
- `triples.json` — graph triples snapshot (where
  `coordinator.decision_reason` and friends live).
- `tool-issues.log` — backend slog ERROR-level tool failures
  scanned at capture time. Empty file is clean.

To regenerate `wedge-report.txt` post-hoc against an existing
capture dir (no stack required):

```bash
RUN_ID=smoke7-run1 task ui:test:e2e:agentic:smoke7:wedge-report
WEDGE_EXPECTED_TERMINAL_ROLE=dev-via-spec-architect RUN_ID=... \
  task ui:test:e2e:agentic:smoke7:wedge-report   # for partial-chain expectations
```

Cleanup is deferred so an abort still tears the stack down.
The watcher is hard-capped at 30 minutes (~2× the upper-bound
estimate) to bound real-LLM spend on a wedged chain.

The rest of this section walks the same flow manually for
debugging, adding probe steps mid-run, or operating against a
stack that's already booted.

### 1. Boot the stack (manual)

```bash
task test:e2e:agentic:dev:osh-demo
```

The task exits after stack health-check passes (typically 60-90
seconds after image pulls; the `Stack up at http://localhost:3100`
banner is printed at exit, and your shell prompt returns).

### 2. Send the OSH prompt

In another terminal:

```bash
task test:e2e:agentic:probe:send -- 'create a driver for OpenSensorHub using OGC Connected Systems for Meshtastic devices'
```

The prompt is from ADR-031 §"R3.4 OSH demo" — the canonical
real-LLM exercise for the dev-via-spec chain. The phrasing
matters: it names the actor (OSH driver), the protocol (OGC
Connected Systems), and the integration target (Meshtastic).

### 3. Watch the chain settle

**Recommended:** in a third terminal, start the auto-approve
chain watcher — it polls every 5s, logs role transitions, and
auto-approves the source-acquisition gate (Step 4) without you
having to click through the UI:

```bash
task test:e2e:agentic:probe:auto-approve-watcher -- smoke7
```

Outputs land in `/tmp/smoke7-*` (trajectory, events log, dedup
state). The watcher exits when every loop is terminal and the
count's been stable for ~120s. Designed for exactly this
real-LLM-smoke shape.

**Manual fallback** (if you want to drive approvals yourself):
poll `task test:e2e:agentic:probe:loops` every 30-60 seconds and
approve via Step 4 below.

Expected progression — timing is a rough estimate; varies with
LLM latency, model load, and whether the substrate is warm or
cold:

| Phase | Loops | Wall time | Notes |
|---|---|---|---|
| Research arc | A-F (6 loops) | ~3-5 min | First add_source_repo pauses for approval — see Step 4 |
| dev-via-spec planner→architect | G-J (4 loops) | ~5-8 min | Architect reads challenger + planner + research-reviewer (3 reads) before emit |
| Builder | K (1 loop) | ~2-5 min | Calls bootstrap_workspace + bash iterations + builder_decide |
| QA-reviewer | L (1 loop) | ~30-60s | Reads builder result + decide(needs_clarification) |

Target: **all 12 loops in `complete` state** within ~25 minutes
total wallclock. Observe + update these estimates after a real
run lands.

### 4. Approve the source-acquisition gate

Loop C (researcher-with-source-acquisition) calls
`add_source_repo` for the OSH core repository. The
`approval_required` filter pauses the loop. In the UI at
http://localhost:3100:

1. Click on the awaiting-approval task card.
2. Verify the tool args show `https://github.com/sensorhub-tools/osh-core`.
3. Approve.

Without UI: use `task test:e2e:agentic:probe:approve -- <loop_id>`
(which posts to `POST /teams-dispatch/loops/<loop_id>/approval`
with the right `X-User-Id` header). Or, if you started the
auto-approve watcher in Step 3, this happens for you.

### 5. Verify per-phase

After Loop F (research arc complete):

```bash
task test:e2e:agentic:probe:triples -- coordinator.next_action
```

Expect `approved` from the third research-reviewer pass (Loop F).

After Loop J (architect emit):

```bash
task test:e2e:agentic:probe:triples -- dev_via_spec.artifact.path
```

Expect a path under `docs/specs/<date>-osh-meshtastic-driver.md`
(the architect's `emit_dev_via_spec_artifact` tool writes the
markdown there + mints the triple).

After Loop K (builder terminal):

```bash
task test:e2e:agentic:probe:triples -- coordinator.next_action
```

Expect `tests_passing` or `tests_failing` from the builder loop
(triggers rule 07). If `needs_clarification`, the builder hit a
boot-failure — capture the loop trajectory and stop.

After Loop L (qa-reviewer terminal):

```bash
task test:e2e:agentic:probe:triples -- coordinator.decision_reason
```

The qa-reviewer's verdict lives on `coordinator.decision_reason`
(the action itself goes onto `coordinator.next_action`, but
that predicate is shared with every other `decide`-emitting
role across all twelve loops; filter by the qa-reviewer's
loop_id if you want to isolate). Expected verdict:
`needs_clarification` — see "Why this is the expected verdict"
below. If `accept` or `reject`, the LLM is grading against
something other than the stub block; capture the loop trajectory
and note for findings.

**Why `needs_clarification` is the expected verdict.** The
spawn-rule prompt at `07-builder-decide-to-qa-reviewer.json` is
prompt-engineered to instruct the LLM to treat the `(stub)` block
as Empty Evidence per the persona's
`10-evaluation-contract.md` Rule 3, and emit
`decide(needs_clarification)`. This is *prompt-driven*
expected, not *persona-derivable* expected — the persona's
Rule 3 trigger conditions cover three other shapes (all results
UnknownKind, empty Image harness, target-test mismatch), none
of which structurally describe a literal `(stub)` block. If a
real LLM pattern-matches `(stub)` to "test data, accept" or to
"reject — no evidence," that's the open-plumbing question this
smoke is the forcing function for. Capture the verdict and the
trajectory either way.

## Capture findings

If you used `task test:e2e:agentic:smoke7:run`, the deferred
`smoke7:capture` step has already dumped the mechanical evidence
(loops snapshot, per-loop trajectories, message-logger entries)
to `/tmp/smoke7-<RUN_ID>/`. The synthesis step below is the
human-judgment layer over that evidence.

Save the following synthesis to a scratch file
(`/tmp/smoke7-<RUN_ID>/findings.md` or similar — do NOT commit):

1. **Loop list snapshot** — read `/tmp/<RUN_ID>/loops.json`
   (auto-captured) or run `task test:e2e:agentic:probe:loops`
   on a still-running stack.
2. **Loop trajectories for K and L** — read
   `/tmp/<RUN_ID>/trajectory-<loop_id>.json` (auto-captured).
3. **qa-reviewer terminal triples** —
   `task test:e2e:agentic:probe:triples -- coordinator.decision_reason`
   (run on the still-running stack before cleanup, or grep
   `messages.json` for `coordinator.decision_reason` payloads).
4. **Cost** — Anthropic console for token usage if you can
   correlate by time window.
5. **Anomalies** — any of:
   - Architect emit failed (slug mismatch, malformed artifact).
   - Builder iteration_budget exhausted before terminal.
   - Builder's `bootstrap_workspace` fails with a tool_result
     error (sandbox container not reachable; surfaces as a
     tool_result error not a `state=failed`, so the loops
     listing won't catch it — check the builder loop's
     trajectory directly).
   - qa-reviewer emitted `accept` or `reject` instead of
     `needs_clarification` (indicates the LLM is grading against
     prompt content other than the stub block — informs whether
     the stub marker is strong enough).
   - Any loop `state=failed` rather than `complete`.
   - Mid-chain compaction routes through `seminstruct`
     (qwen3-8b) per the config's `summarization` capability; if
     the builder loop suddenly stops citing architect
     commitments after a long run, suspect compaction-via-qwen3
     dropping load-bearing detail.

Write a one-page synthesis answering:

- Did the chain reach the qa-reviewer terminal? If not, where did
  it break?
- Which evidence-summary plumbing shape did the smoke surface as
  the right next step?
  - Rule action wrapping `publish_agent` to pre-render the gate
    output (lean per memory).
  - Tool the qa-reviewer calls (`read_evidence`) — adds tool
    surface, against fewer-rich-tools.
  - Pre-publish preprocessor that mints an `evidence.summary`
    triple on the builder entity.
- Any persona drift worth pinning (e.g. architect didn't read
  research artifact before emit, builder over-iterated)?

Land the synthesis as an addendum to ADR-034 §"What R3.7.2 work
is preserved" or as a new memory record under
`project_smoke7_findings.md`.

## Teardown

```bash
task test:e2e:agentic:cleanup
```

`down -v` is the right shape (NATS KV caches component config
per `feedback_nats_config_caching.md` — restart picks up stale
config; only `down -v` clears it).

## Failure-mode quick-reference

- **Stack won't boot** — check `ANTHROPIC_API_KEY` is set in
  the same shell that runs `task ...:dev:osh-demo`. Compose
  forwarding requires it at boot, not at probe time.
- **First researcher loop hangs** — seminstruct is still loading
  qwen3-8b weights. Allow 2-3 minutes after stack-up before
  sending the prompt on a cold boot.
- **No add_source_repo approval prompt** — check the Loop B
  reviewer actually emitted `decide(insufficient,
  recommend=add_source_repo)`. Real-LLM may approve directly if
  the corpus already has OSH content from a prior run; tear
  down with `down -v` for a clean slate.
- **Builder bash failures** — sandbox container can't reach the
  backend filesystem. Verify `task test:e2e:sandbox:dood:smoke`
  passes before re-running the smoke; the sandbox image may
  need rebuilding.
- **qa-reviewer never spawns** — confirm rule 07 loaded:
  `curl -sf http://localhost:3100/rule-processor/rules | jq '.[] | .id'`.
  If absent, the config wasn't picked up — `down -v` and reboot.
- **Anthropic 429 / max_concurrent saturation** — `osh-demo.json`
  caps `claude-sonnet` at 100 RPM and 10 max_concurrent. The
  full chain peaks at maybe 15 RPM during builder iteration; if
  another sonnet workload is running on the same key (e.g.
  parallel SemSpec smoke), the chain may serialize at the
  concurrency cap. Watch for any loop wedged in `iterations`
  count not advancing for >2 min — that's the rate-limit shape.
