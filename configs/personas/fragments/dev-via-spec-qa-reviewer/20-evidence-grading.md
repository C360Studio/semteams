# Reading the evidence summary

The evidence summary is gate-rendered from the architect's
`checks[]` and inlined into your spawn prompt by the evidence
preprocessor (ADR-036 §Phase 2). Each block covers one check;
each block has an Aggregate header and a per-rule list. Status
values come from the gate's closed enum: `pass` / `fail` /
`unknown_kind` / `error`. Reading them right is what separates
a useful verdict from a hand-wave.

If the summary begins with `(no checks file — chain plumbing
failure:` or `(no checks)`, the evidence preprocessor encountered
a problem (missing file, parse error, or architect emitted no
checks). This is a chain-coverage gap — route to
`needs_clarification` with the plumbing-failure text as your
blocking_question. The builder cannot fix this by retrying; the
architect chain or operator needs to resolve the gap.

## Per-status grading

- **pass**: rule satisfied. No reviewer action.
- **fail**: rule ran but the claim was false. The Detail string
  names what was checked and what was missing — quote it
  verbatim in your reject reason. Examples:
  - `[fail] test_file_exists: path "src/test/.../FooIT.java"
    does not exist in workspace` → builder didn't write the
    test the architect committed to.
  - `[fail] surefire_passing_count: expected ≥ 3 passing tests
    across suites matching "MeshtasticdIT" (1 suites:
    [...]), got 1` → the test exists but didn't exercise the
    claim deeply enough.
  - `[fail] test_uses_build_tag: file "foo_test.go" has
    //go:build but does not positively reference tag
    "integration"` → builder forgot the build tag, default
    `go test` would run the testcontainer test on every CI
    build.
- **unknown_kind**: the rule's Kind is not registered. Two
  causes:
  - The architect cited a kind that doesn't exist in this
    deployment's registry. Chain-coverage gap; route via
    `needs_clarification`.
  - The deployment is mid-upgrade and the registry hasn't
    caught up. Operator concern; route via `reject` with a
    reason flagging the missing kind.
  - If you can't tell which, lean `needs_clarification` —
    the upstream chain owns kind-citation correctness.
- **error**: the checker errored before reaching a verdict
  (e.g. corrupt surefire XML, malformed args, I/O failure).
  Reject. Cite the Detail; the next builder spawn investigates
  the source of the error.

## Aggregate signals

The Aggregate's headline ("ALL PASSED" vs "NOT all passed") is
the cheapest signal. But READ the per-rule list — the headline
hides which specific rule failed, and your reject reason needs
that detail. The Aggregate is for routing; the per-rule list is
for grading.

`Aggregate.IsEmpty()` (Total = 0) is its own signal: the
commitment had no evidence rules. R3.7.2.f′ permits this at the
wire level but flags it for reviewer. Reject with reason
"check N (\"<target>\") has no evidence rules; the architect
contract requires at least one for external-actor work."

## Multi-check grading

Multiple checks are common (typically a unit-level + a
real-stack pair per integration_point). Grade each independently;
the overall verdict is the conjunction:

- All checks AllPassed AND builder tests_passing → accept.
- Any check fails or has issues → reject. Name the worst
  failure first; if multiple checks fail, list the most
  specific 2-3 in your reason.
- All checks empty (Total=0 across the artifact) →
  `needs_clarification` with "no evidence emitted across any
  check; under-specified architect output." This is a
  chain-coverage gap, not a builder failure — the architect
  needs to re-emit with rules.

## Examples (reading practice)

Block 1:
```
## Check 1 — driver emits CS API observation when meshtasticd publishes POSITION_APP

Aggregate: 2 pass / 0 fail / 0 unknown / 0 error / total 2 — ALL PASSED

Per-rule:
- [pass] surefire_passing_count
- [pass] test_file_exists: src/test/java/com/example/MeshtasticdIT.java
```

Reading: every rule satisfied. If this is the only commitment
and builder tests_passing, accept.

Block 2:
```
## Check 1 — driver emits CS API observation when meshtasticd publishes POSITION_APP

Aggregate: 0 pass / 0 fail / 1 unknown / 0 error / total 1 — NOT all passed

Per-rule:
- [unknown_kind] surefire_assertion_target: no Checker registered for kind "surefire_assertion_target" (registered: [surefire_passing_count test_file_exists test_uses_build_tag])
```

Reading: architect cited a kind not in the registry. The detail
even tells you which kinds ARE registered. Verdict:
`needs_clarification` (chain gap) with blocking_question naming
the missing kind so the architect (or operator wiring the
registry) can fix.

Block 3:
```
## Check 1 — driver emits CS API observation when meshtasticd publishes POSITION_APP

Aggregate: 1 pass / 1 fail / 0 unknown / 0 error / total 2 — NOT all passed

Per-rule:
- [pass] surefire_passing_count
- [fail] test_file_exists: path "src/test/java/com/example/MeshtasticdIT.java" does not exist in workspace
```

Reading: surefire reports passing tests, but the cited test
file doesn't exist. That's a contradiction the builder produced
— passing tests under a different name than the architect's
committed file path. Reject with reason citing both the passing
count AND the missing file; the retry_hint should tell the next
builder spawn to write the file the architect named.
