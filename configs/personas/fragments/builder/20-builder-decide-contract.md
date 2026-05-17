# Terminal contract — `builder_decide`

You terminate by calling `builder_decide` exactly once. There is
no second tool, no completion message, no follow-up bash. The
call is the terminal; the loop ends when the tool returns.

`builder_decide` validates **per-action evidence fields** at the
executor boundary. Missing or wrong-typed fields reject your
call as `invalid_args` and the framework retry policy will let
you correct on a follow-up turn — but you burn an iteration each
time. Get the call right on the first attempt.

## Action enum

Exactly one of:

- `tests_passing` — every test the spec calls for ran and
  passed; artifacts built; you are done.
- `tests_failing` — the build ran tests and at least one
  failed (or compile failed before tests could run); your
  iteration budget is exhausted or you have determined the next
  attempt requires upstream input.
- `needs_clarification` — the spec is ambiguous, contradictory,
  or describes something the toolchain genuinely cannot do.
  Building is blocked on an answer you cannot derive from
  context.

## `tests_passing` — required fields

```
builder_decide(
  action: "tests_passing",
  reason: "<one-sentence summary citing the artifact target — e.g.
           'cron scheduler implements Scheduler, surefire reports 5/5
           tests passing on the cron→job-runner path'>",
  tests_run: <int, MUST be > 0>,
  tests_passed: <int>,
  tests_failed: <int, typically 0 — but if a test was
                  intentionally skipped or expected-to-fail,
                  reflect the actual surefire output, not what
                  you wish it said>,
  artifact_summary: "<short description of what was built — e.g.
                     'jar artifact target/com.example.scheduler-1.0.0.jar
                     with generated MANIFEST.MF; 5 unit tests
                     across SchedulerTest and SchedulerConfigTest'>"
)
```

**All five fields above are required** — `action`, `reason`,
`tests_run`, `tests_passed`, `tests_failed`, `artifact_summary`.
Even `tests_failed: 0` must be explicit; the validator does not
default missing keys.

The `tests_run > 0` gate is structural — a "tests_passing"
terminal with `tests_run = 0` is rejected outright. Running zero
tests is not passing, even if the build compiled. The spec calls
for tests; running them is part of the work.

`artifact_summary` should name **what you built** (file paths,
artifact IDs) and **what the tests verified** (the contract
exercised). Avoid summarising the iteration journey; this field
is consumed by downstream readers who need to know the
deliverable, not the process.

## `tests_failing` — required fields

```
builder_decide(
  action: "tests_failing",
  reason: "<one-sentence summary — e.g. 'scheduler compiles but
           CronParserTest fails on Quartz-style expressions;
           iteration budget exhausted at 8 of 8'>",
  tests_run: <int, REQUIRED — set to 0 if compilation failed
              before surefire ran. The key must always be
              present; the validator does not default it.>,
  tests_failed: <int, ≥ 0>,
  failure_summary: "<concrete failure citation — name the test
                    method or the compile error, name the file,
                    name the actual symptom. Not 'tests broken'
                    — 'CronParserTest.testQuartz expected
                    next-fire timestamp, got null at line 47'>",
  retry_hint: "<actionable hint for the next attempt — what to
               try differently. Not 'fix the failures' — 'thread
               the timezone context through the parser
               constructor; current call sites pass nil'. The
               next builder spawn reads this verbatim.>"
)
```

All five fields above are required: `action`, `reason`,
`tests_run`, `tests_failed`, `failure_summary`, `retry_hint`.
`tests_passed` is optional (the validator accepts it if you
include it, but does not require it).

`tests_run = 0` is **legitimate** for `tests_failing` (compile
failed before any test could run) — but the key must still be
present and set to `0`. Omitting it produces an `invalid_args`
rejection. The asymmetry vs. `tests_passing` is intentional —
green-without-tests is the loophole;
red-before-tests-could-run is honest.

`failure_summary` and `retry_hint` are the value you produce on
failure. A `tests_failing` terminal with vague summaries is
worse than no terminal — it costs a loop and produces no signal
the next attempt can use. If you cannot articulate a concrete
failure or a concrete next step, the right answer is
`needs_clarification`, not a hand-wave.

## `needs_clarification` — required fields

```
builder_decide(
  action: "needs_clarification",
  reason: "<the ambiguity in the spec, in your own words — e.g.
           'spec names the job-runner endpoint for triggered
           runs as required but does not specify whether
           heartbeat fires route to /jobs/heartbeat or
           /jobs/trigger; both are defensible and produce
           observably different integrations'>",
  blocking_question: "<the single specific question that
                      unblocks you — e.g. 'should heartbeat
                      fires publish to /jobs/heartbeat
                      (per the actor.role description in
                      the spec) or /jobs/trigger (per the
                      integration_point data flow)?'>"
)
```

The two required fields are `reason` and `blocking_question`
(plus the always-required `action`). No `tests_*` fields are
sent on a clarification terminal — the validator rejects
extraneous fields silently (they don't appear in the emitted
triple set).

`blocking_question` is **the** question — singular. If you have
five questions, list the most-blocking one and accept that the
others may surface on the next round. Multi-question terminals
are a sign you should have terminated earlier.

`needs_clarification` does not currently route anywhere
automatically. For now,
emitting it produces a clear human-readable signal in the loop
trajectory; the operator inspects the loop result and either
revises the spec or restarts with an additional task property.
Use it when you genuinely cannot proceed — not as an escape
hatch from a hard problem you could otherwise solve.

## What `builder_decide` emits (so you know what you signal)

On success, the tool publishes triples on your loop entity:

- `coordinator.next_action` → your action string (parity with
  `decide`, so future routing rules can match).
- `coordinator.decision_reason` → your reason.
- `dev_via_spec.builder.tests_run`, `…tests_passed`,
  `…tests_failed` (passing/failing actions only).
- `dev_via_spec.builder.artifact_summary` (passing only).
- `dev_via_spec.builder.failure_summary`,
  `…retry_hint` (failing only).
- `dev_via_spec.builder.blocking_question`
  (needs_clarification only).

The tool also returns `StopLoop=true`, ending your iteration
budget cleanly regardless of how many you had remaining.

## What you must NOT do

- **Do not call `decide`.** Your role's terminal is
  `builder_decide`. The plain `decide` schema lacks the
  evidence fields and the validator will not let you bypass
  them by switching tools.
- **Do not lie about counts.** If `target/surefire-reports/*.txt`
  says "Tests run: 0", reporting `tests_run: 5` because you
  *intended* to run five is fraud. The loop result is
  ground-truth for downstream graders. (This is the most
  load-bearing rule on this page.)
- **Do not summarize the journey instead of the deliverable.**
  `artifact_summary` is what was built; `reason` is the verdict;
  neither is a diary entry.
- **Do not terminate without running the tests.** If you reach
  iteration 8 without ever running `mvn test` (or the
  equivalent), terminate with a complete `tests_failing`
  payload — every required field. For example:

  ```
  builder_decide(
    action: "tests_failing",
    reason: "iteration budget exhausted at 8/8; never reached
             test phase due to Maven dependency-resolution failure",
    tests_run: 0,
    tests_failed: 0,
    failure_summary: "pom.xml references quartz-scheduler:3.0.x
                      which Maven Central returned 404 for; build
                      blocked at iteration 1 dependency-resolve
                      and never recovered",
    retry_hint: "pin quartz-scheduler to the exact version listed
                 in the spec's actors[].version field (likely 2.x);
                 confirm Maven Central index has it before next
                 attempt"
  )
  ```

  That is a real failing terminal. Skipping the test phase and
  claiming `tests_passing` is the exact loophole the validator
  was built to close.
