# Dev-via-test pack — MVP-1 smoke against `@mavlink-decode` (2026-06-04)

The chain-end gate caught a constraint violation that per-task
tests couldn't. The substrate ran 4 slices of new machinery
end-to-end on the first real-LLM run; CBG rejected the
implementation with a substantive citation of the rule that broke.

**Run cost:** ~$0.30 (~944K tokens, mostly gemini-flash + claude-haiku).
**Wallclock:** ~5 minutes.
**Loops:** 9.
**Mechanics:** every Slice 1–4 rule fired correctly.
**Outcome:** CBG `rejected` → coordinator `ask_user`.

## What got built (and why this run is sponsor-worthy)

The prompt is the [`@mavlink-decode` scenario](https://github.com/c360studio/semspec/blob/main/ui/e2e/plan-lifecycle-llm-mavlink.spec.ts)
that semspec's BMAD-shaped pipeline has been working through:

> "Add a Go HTTP service that listens for MAVLink v2 HEARTBEAT
> frames over UDP on port 14540 and exposes the most recent
> heartbeat at GET /heartbeat as JSON containing 'system_id',
> 'component_id', 'autopilot_type', 'base_mode', and 'received_at'.
> **Use a real Go MAVLink library (e.g., github.com/bluenviron/gomavlib)
> for frame parsing — do not hand-roll the MAVLink wire format.**
> Include unit tests that decode captured MAVLink HEARTBEAT frames
> from testdata/ files and assert the parsed fields."

The dev-via-test pack ran the chain end-to-end:

| Loop | Role | Took | What happened |
|---|---|---|---|
| `loop_457ad91c` | front-door coordinator | ~30s | classified ask as `dev_via_test`; called `request_sandbox` (one retry; succeeded on second attempt); emitted `decide(action=dev_via_test)` |
| `fb924d42-cae7` | dev-via-test-plan (Lisa) | ~30s | decomposed into 2 tasks (`receiver-logic`, `http-service`) with Karpathy-shaped spec; emitted via `emit_dev_via_test_plan` |
| `7649cb29-408` | coordinator (walker A) | ~15s | read plan, dispatched `receiver-logic` via `decide(action=dev_via_test, subtopics=["receiver-logic"])` |
| `995ca2ca-504` | dev-via-test-execute (Ralph 1) | ~60s | wrote `receiver.go` + tests, ran `go test`, called `emit_dev_via_test_measurement(pass=true)`, `decide(measured)` |
| `cd04296b-2ef` | coordinator (walker B) | ~15s | saw `dev_via_test.execute.task_completed`, dispatched `http-service` |
| `bc4ddbea-243` | dev-via-test-execute (Ralph 2) | ~35s | wrote `server.go` + tests; all 6 tests passed; `decide(measured)` |
| `8ab40577-890` | coordinator (walker C) | ~15s | saw both tasks complete; emitted `decide(action=dev_via_test_finalize)` |
| `6c6c5c05-3c1` | reviewer-dev-via-test (CBG) | ~45s | ran `go test -v ./...` (all 6 passed); read `git diff plan-start`; **rejected** |
| `c5e28f39-476` | coordinator (final wake-up) | ~10s | read CBG's verdict; emitted `decide(ask_user, reason=<user-facing rollup>)` |

**CBG's verdict, verbatim:**

> "Rejected — the implementation violates a core technical
> constraint of the plan. The goal explicitly required using a
> real Go MAVLink library (e.g., `gomavlib`) and forbade
> hand-rolling the wire format ('do not hand-roll the MAVLink
> wire format'). However, `receiver.go` manually parses frames
> using hardcoded byte offsets (e.g., `frame[10+4]` for autopilot
> type) and ignores the checksum…"

**The user got** (final coordinator's `ask_user`):

> "The implementation was rejected by our reviewer because it
> violated a core technical constraint. While the code passed the
> integration tests, it cheated by manually parsing the MAVLink
> frames with hardcoded byte offsets instead of using a real Go
> library like gomavlib as required. Should I amend the plan to
> strictly enforce the library usage and try again, or would you
> like to abandon this run?"

**Verification:** `evidence/ralph-workspace/go.mod` has zero
external deps; `evidence/ralph-workspace/receiver.go` hand-rolls
byte parsing. CBG's claim is factually correct.

## Why this is the result we wanted

The pack's design rationale puts CBG here for exactly this:

> "Per-task tests passed individually (that's what Ralph proved);
> your job is to catch the contamination between tasks — Ralph
> on t2 quietly broke a t1 invariant, scope leaked outside
> `target_files`, **test-gaming snuck through**. The integration
> suite is the only gate that sees the whole."

Ralph wrote tests that exercised *Ralph's implementation*. The
tests passed. The integration test passed. From a per-task
perspective, the chain "succeeded." CBG read the diff and
noticed the rule the plan said could not be broken was broken
anyway. **One run, immediate caught.**

This is the deterministic-test-attestation property the pack is
meant to prove: not "tests pass" (Ralph proved that), but "tests
pass AND the work meets the stated constraints" (CBG verified
that).

## What works (mechanics — all green)

Every load-bearing piece of new machinery the 4 slices added
fired correctly on first contact with a real LLM:

- **Two-mode `dev_via_test` token** — rule 01 fired for initial
  Lisa dispatch (subtopics empty + no lineage); rule 03 fired
  twice for walker dispatch (subtopics=[task-id] + walker has
  lineage); the differentiator held.
- **`emit_dev_via_test_plan` schema enforcement** — Lisa's payload
  passed Karpathy-required validation (assumptions, non_goals,
  target_files≥1, test_command); plan triples landed on the
  run entity.
- **Cross-entity stamping** — rules 04a/04b stamped
  `dev_via_test.execute.task_completed` on the run entity via
  `$entity.triple.lineage.run-loop-entity-id` substitution; walker
  read the markers via `query_entity` to compute derivative status.
- **`for_each` over subtopics** — rule 03 spawned Ralph at the
  walker-chosen `$subtopic` value; bare-UUID lineage shape
  consistent across all walker generations.
- **`dev_via_test_finalize`** — Slice 4's new action token routed
  walker C's "all done" decision to rule 06 → CBG. Closed-taxonomy
  enforcement worked.
- **Approve/reject split (rules 07a/07b)** — CBG's `decide(rejected)`
  fired rule 07b, which spawned the final coordinator with
  `[ask_user, respond_direct]` allowlist. The final coordinator
  emitted `ask_user` exactly as 07b's prompt instructed.
- **CBG single-run discipline** — exactly one integration test
  run, exactly one diff read, exactly one verdict. No retry,
  no fix, no iteration. Persona prose held under real LLM.
- **Sandbox + attestation** — coordinator hit one
  `request_sandbox` transient failure (`context deadline
  exceeded` on the first `devcontainer up`), LLM retried, second
  attempt succeeded. `sandbox.attestation.ready=true` landed;
  all `bash` calls routed through the per-tenant devcontainer
  (`evidence/ralph-workspace/` is the actual sandbox output).

## Findings worth following up

### F1 (load-bearing for next runs) — Lisa's plan didn't structurally enforce the library constraint

Lisa's task spec for `receiver-logic` named the goal in prose but
didn't include "must `go get github.com/bluenviron/gomavlib`" as
a separate task or as an assumption that Ralph would have to
write into his target_files. Result: Ralph chose the path of
least resistance — hand-rolled the frame format because the
tests didn't structurally demand the library.

**Remediation:** sharpen Lisa's persona (`dev-via-test-plan/10-emit-contract.md`)
to explicitly require: when the prompt names a specific library
or framework, the plan MUST include a task that runs
`go get <library>` (or equivalent) BEFORE the task that uses it,
AND the using-task's test_command MUST exercise the library's
exported API. This makes the constraint structural rather than
hope-based.

### F2 (medium) — Ralph's tests-pass-without-delivering pattern

Ralph 1's tests assert "parser extracts the right bytes from a
specific binary frame in `testdata/`" — but Ralph crafted both
the test fixture AND the parser, so the test is circular. CBG's
diff caught the rule violation; the tests didn't.

**Remediation:** Ralph's persona (`dev-via-test-execute/10-execute-loop.md`)
already warns against test-gaming. Strengthen with: when a task's
`assumptions` or `goal` names a specific library or behavior, the
test_command MUST exercise it (e.g., `go test -tags=integration`
that imports the library, not just hash-equality on canned
fixtures). This may be hard to enforce structurally; the
remediation may be CBG-side instead — sharper diff-review
heuristics.

### F3 (low — known framework gap) — "model not in registry, using default context limit" WARN

Backend logs show repeated WARNs of `model not in registry, using
default context limit` for `model: "research"` / `"coordinator"` /
`"reviewing"` — these are capability aliases (resolved via the
`capabilities` block in `flow-bootstrap.json`), not registry
endpoint names. The default 128K context limit applies, which is
sufficient. Filed-already-or-file upstream: capability-alias
resolution should walk to the endpoint's max_tokens, not fall
through to default.

### F4 (low) — first `request_sandbox` failed with "context deadline exceeded"

First `devcontainer up` POST hit a context deadline on tenant
`f882f7f6022adda1`; coordinator retried (or fell through to
attestation-stamp retry?), second attempt on tenant
`d7d99df29380bd38` succeeded. Worth understanding the timeout
budget — devcontainer-cli first-pull is slow; the sandbox
manager's deadline may be too tight for cold starts. Defer until
a second occurrence.

### F5 (script gap, not a chain issue) — wedge-report false-positive

`wedge-report.txt` flagged the chain as "WEDGE" because the
default expected terminal role is `dispatch` (smoke13's shape).
Our chain's terminal is `coordinator` with `decide(ask_user)` —
legitimate post-CBG-rejected path. Update `smoke7:capture` to
accept multiple expected terminal roles, or build a
dev-via-test-specific smoke task that knows the rejected and
approved chain shapes.

## What this run does NOT prove yet

- **5-run convergence rate** — the pack's acceptance bar is "≥4 of 5
  runs converge inside framework `max_iter` ceiling with
  `go test ./...` green + `go vet` clean." This run mechanically
  satisfies the strict criterion (chain converged, tests green
  inside max_iter) but the CBG rejection means the SPIRIT of the
  acceptance isn't met. Need 4 more runs to establish the
  convergence rate — both mechanics and "approved" outcomes.
- **The OSH-MAVSDK (`@mavlink-hard`) Accept-gate** — that's a
  brownfield scenario requiring Lisa to read pre-cloned source
  trees. Out of scope for MVP-1 first run; would test brownfield
  story.
- **Cost-runaway resilience** — this run was cheap (~$0.30). A
  pathological prompt could go orders of magnitude higher. The
  framework `max_iterations: 50` + per-loop `timeout: 300s` are
  the safety floor; not exercised in this run.

## Evidence in this pack

| File | What it is |
|---|---|
| `evidence/loops.json` | The 9-loop list with roles, states, outcomes. |
| `evidence/trajectory-<loop-id>.json` | Per-loop iteration log (tool calls, responses, decide terminal). |
| `evidence/watcher-events.txt` | Watcher's role-transition timeline. |
| `evidence/watcher-trajectory.jsonl` | Per-poll watcher snapshot. |
| `evidence/wedge-report.txt` | Auto-generated chain-shape report (note F5 false positive). |
| `evidence/triples-filtered.json` | `plan.*` + `dev_via_test.*` + `coordinator.decision.*` + `sandbox.attestation.*` triples (the load-bearing ones). |
| `evidence/ralph-workspace/` | Ralph's actual sandbox output — `go.mod` (zero deps), `receiver.go` (hand-rolled), tests. |

## Cross-links

- [dev-via-test design record](../../adr/044-dev-via-test-pack.md) — pack design
  + four §addendum entries documenting each slice's
  framework-alignment + reviewer-pass decisions.
- [Lisa persona](../../../configs/personas/fragments/dev-via-test-plan/)
- [Ralph persona](../../../configs/personas/fragments/dev-via-test-execute/)
- [CBG persona](../../../configs/personas/fragments/reviewer-dev-via-test/)
- [Pack rules](../../../configs/rules/dev-via-test/)
- [Sister sponsor pack: research-pack fan-out](../research-pack-fan-out-2026-05-29/) (the precedent for this format)
