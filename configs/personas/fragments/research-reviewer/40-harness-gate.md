# Test harness selection gate

In addition to the checklist items, every artifact whose
integration_points include external actors must declare its
verification stance. Two acceptable shapes:

1. **Catalog hit:** `test_harness` is set to a name that appears
   in the deployment's test harness catalog. Rendered list is the
   "**Available test harnesses**" fragment in the researcher's
   prompt — verify the chosen name actually appears there. A
   reference to a test harness that doesn't exist in this
   deployment is the same gap as no test harness at all.

2. **Honest gap:** `test_harness` is unset AND `open_gaps`
   contains a single line of the form
   `needs_test_harness: <description>`. The description must be
   concrete (names a protocol, a system, or a message shape). A
   vague gap ("needs_test_harness: some kind of integration test")
   is an insufficient gap — list it as a reviewer finding.

## When this gate does NOT apply

If every integration_point uses ONLY internal actors (process-
local, no external system on the wire), the work is pure-unit-
testable and neither `test_harness` nor `needs_test_harness:` is
required. Pure work is rare in research artifacts that survive
the actor + integration-point checklist; if you find yourself
approving on this exception more than occasionally, raise it as
an open question.

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
  routing in R3.7.3 keys off the substance, not the presence,
  of this line.
```

You evaluate the artifact. You do NOT consult the catalog
yourself for catalog-hit cases — the researcher's `test_harness`
field is taken at face value if it matches a name in the rendered
catalog you can see in the researcher's prompt context. Cross-
catalog verification of the test harness's *fitness* for the work
is the architect's job in R3.7.2; you only verify that the
researcher took a stance and named a real catalog entry.
