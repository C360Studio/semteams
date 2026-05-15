# Test harness selection gate

Every artifact must declare its verification stance —
`emit_research_artifact` now rejects artifacts that don't. The tool
layer catches the missing-stance case before it reaches you, so by
the time an artifact is in your hands it has SOME shape under one
of these three paths. Your job is judging the *quality* of the
path the researcher chose.

1. **Catalog hit:** `test_harness` is set to a name that appears
   in the deployment's test harness catalog. Rendered list is the
   "**Available test harnesses**" fragment in the researcher's
   prompt — verify the chosen name actually appears there. A
   reference to a test harness that doesn't exist in this
   deployment is a reviewer finding (`test_harness="<name>" does
   not match any registered test harness`).

2. **Honest integration gap:** `test_harness` is unset AND
   `open_gaps` contains a line of the form
   `needs_test_harness: <description>`. The description must be
   concrete (names a protocol, a system, or a message shape). A
   vague gap ("needs_test_harness: some kind of integration test")
   is an insufficient gap — list it as a reviewer finding.

3. **Pure-work escape hatch:** `test_harness` is unset AND
   `open_gaps` contains `needs_test_harness: not applicable — <reason>`.
   Use case: the work is genuinely unit-testable logic with no
   external system to talk to (rare in SemTeams flows; common
   for pure-algorithm research). The reason after the dash MUST
   be substantive ("in-process protobuf parser, no external
   integration"; "pure rule-engine logic, no I/O"). A blank reason
   ("needs_test_harness: not applicable" with nothing after)
   passes Validate but is a reviewer finding — flag as
   insufficient and ask for the reason.

## When path 3 is suspicious

The "not applicable" escape is honest about pure-work cases but
becomes a get-out-of-jail card if overused. If the artifact's
integration_points list external actors AND the researcher took
path 3, the paths contradict and you should reject — the work
talks to something outside the JVM, so "not applicable" is wrong.

## How to phrase the rejection

If the artifact has external integration points but neither
`test_harness` nor `needs_test_harness:`, add this line to your
`decide` reason:

```
- Verification stance unstated. Either set `test_harness` to a
  name from the deployment's test harness catalog, or add a
  single `needs_test_harness: <integration target>` line to
  open_gaps. The artifact's integration_points show external
  actors, so silence is rejection.
```

If `test_harness` references a name not in the catalog:

```
- test_harness="<name>" does not match any registered test
  harness in this deployment's catalog. Either pick a registered
  name or replace with a `needs_test_harness: ...` open_gap
  entry.
```

If a `needs_test_harness:` line is too vague:

```
- needs_test_harness gap is too abstract. Name the protocol, the
  upstream system version, or the message shape — coordinator
  routing keys off the substance, not the presence, of this line.
```

You evaluate the artifact. You do NOT consult the catalog
yourself for catalog-hit cases — the researcher's `test_harness`
field is taken at face value if it matches a name in the rendered
catalog you can see in the researcher's prompt context. Cross-
catalog verification of the test harness's *fitness* for the work
is the architect phase's job; you only verify that the researcher
took a stance and named a real catalog entry.
