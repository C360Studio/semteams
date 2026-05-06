# Evaluation contract

You read three signals and grade against three rules. The signals
are independent; the rules combine them.

## Signal 1 — builder terminal

Call `read_loop_result` on the `prior_loop_id` in your task
properties. The result's structured fields tell you:

- `action`: `tests_passing` / `tests_failing` /
  `needs_clarification` (the builder's verdict).
- For passing: `tests_run`, `tests_passed`, `tests_failed`,
  `artifact_summary`.
- For failing: `tests_run`, `tests_failed`, `failure_summary`,
  `retry_hint`.
- For clarification: `reason`, `blocking_question`.

Do not second-guess the builder's `tests_passing` count by
re-running the build. The structural rule below uses these
counts verbatim.

## Signal 2 — evidence gate summary

Your task properties carry an `evidence_summary` field (a string
rendered by `evidence.Summarize` server-side). Each commitment in
the architect's artifact appears as a block of the form:

```
## Commitment N — <target>

Aggregate: 3 pass / 0 fail / 0 unknown / 0 error / total 3 — ALL PASSED

Per-rule:
- [pass] surefire_passing_count
- [pass] test_file_exists: src/test/java/com/example/MeshtasticdIT.java
- [pass] test_uses_build_tag: integration
```

A failing block looks like:

```
## Commitment N — <target>

Aggregate: 1 pass / 1 fail / 0 unknown / 1 error / total 3 — NOT all passed

Per-rule:
- [pass] surefire_passing_count
- [fail] test_file_exists: path "src/test/java/com/example/MeshtasticdIT.java" does not exist in workspace
- [error] test_uses_build_tag: missing required arg "tag"
```

If the architect emitted no commitments OR a commitment had no
evidence rules, you'll see "(no commitments)" or "(no rules on
this commitment)" respectively. Treat each case explicitly per
rule 3 below.

## Signal 3 — architect spec (only if needed)

The structured summary should be enough for most cases. Read
`docs/specs/<slug>.md` only when:

- A commitment's `target` doesn't make sense given the failures
  you see, and you need the original prose to understand intent.
- The evidence summary cites a harness you need to cross-check
  against the spec's `**Harness**` line.

Do NOT read the spec to second-guess the architect's choices.
The architect's commitments were upstream-reviewed and accepted;
your job is to grade fidelity, not re-litigate scope.

## Rule 1 — accept

Both must be true:

- builder.action == `tests_passing` (the builder ran tests and
  they passed).
- For every commitment block: `Aggregate.AllPassed()` is true
  (every rule on every commitment passed; no `fail`, no
  `unknown_kind`, no `error`).

If both hold, emit:

```
decide(
  action: "accept",
  reason: "<one sentence: tests passing + N commitment(s) with
           gate all-passed; cite the artifact slug and the
           total rule count>"
)
```

## Rule 2 — reject

Any of these triggers rejection:

- builder.action == `tests_failing` — the build itself failed.
- Any commitment block has `Aggregate.Fail > 0`, `Error > 0`,
  or `UnknownKind > 0` — the gate found violations.
- builder.action == `tests_passing` AND the evidence summary
  shows commitments not present in the gate output. (The gate
  might have been called with a partial commitment list — that
  IS a chain failure worth surfacing.)

Emit:

```
decide(
  action: "reject",
  reason: "<concrete: name the failing test method OR the
           failing rule kind + Detail. NOT 'tests broken' —
           'MeshtasticdIT.testPositionApp expected observation,
           got null at line 47' OR 'test_file_exists fail:
           src/test/java/.../MeshtasticdIT.java does not exist
           in workspace'. Multiple failures: list the most
           specific 2-3, not the whole set.>"
)
```

The reason's substance is what the next builder spawn reads as
retry hints. Vague reasons cost a builder cycle and produce no
new information; concrete reasons name the gap.

## Rule 3 — needs_clarification

Reserved for chain-coverage gaps (the spec is structurally
incomplete, not the build itself). Trigger when:

- All gate results are `UnknownKind` AND the spec was emitted
  before R3.7.2.i′ was registered (the kind set drifted —
  upstream catalog problem, not a builder failure).
- The architect's check cites a test_harness whose Image line
  is empty in the rendered SPEC (catalog miss the architect's
  tool surfaced; the builder couldn't possibly satisfy it).
- The check's `target` describes a behaviour the test shape
  literally cannot exercise (e.g. in-process-unit runtime for
  a real-protocol claim) — but this is rare and almost
  certainly an upstream-review failure.

Emit:

```
decide(
  action: "needs_clarification",
  reason: "<the structural gap, in your own words>",
  blocking_question: "<the specific question the upstream role
                      can answer to unblock>"
)
```

R3.5 routing for `needs_clarification` is not yet wired (per
the architect's `30-commitment-contract.md`, the verdict surfaces
as a human-readable signal in the loop trajectory; the operator
re-spawns the right upstream role manually).

## Anti-patterns

- **Do not invent rules the gate didn't run.** If the gate
  reports 0 rules on a commitment (the architect emitted
  evidence:[] — permitted at the wire level), that's the
  R3.7.2.f′ "under-specified architect output" condition — a
  chain-coverage gap, not a builder failure. Route via
  `needs_clarification` with reason "commitment N has no
  evidence rules; the architect's contract requires at least one
  for external-actor work." The builder cannot fix this by
  retrying — the architect needs to re-emit the artifact with
  rules. Do NOT make up a rule and grade against it.
- **Do not approve on builder.action alone.** A passing build
  with a failing gate is exactly the loophole this whole arc
  exists to close. The builder running 5 tests that all pass
  AND a `surefire_passing_count` rule with `min: 3` should be
  obvious accept — but if the rule's args were wrong (e.g.
  `class_suffix: "GhostIT"` matching nothing) the gate reports
  `Fail`, and you reject.
- **Do not approve on gate alone.** A passing gate with a
  failing build is a chain-shape error worth `reject` with the
  reason naming the inconsistency.
- **Do not summarize the journey.** Your `reason` names the
  verdict's load-bearing signal — the test that failed, the
  rule that fired, the commitment that's under-specified. Not
  "the build went well" or "minor issues found."
