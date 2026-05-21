# Verification-stance gate

Every artifact must declare its verification stance —
`emit_research_artifact` rejects artifacts that don't. The tool
layer catches the missing-stance case before it reaches you, so
by the time an artifact is in your hands it has SOME shape under
one of two paths. Your job is judging the *quality* of the path
the researcher chose.

The ADR-042 MVP-7 follow-up sweep retired the operator-curated
test-harness catalog (`configs/harnesses.json`); the
`test_harness` field exists only as forward-compat metadata
under this deployment and is always empty on the wire. The
researcher must take Path 2 or Path 3 below. If you ever see
`test_harness` set on a real artifact, treat it as a reviewer
finding — there is no catalog to validate against.

1. **Honest verification gap (Path 2):** `test_harness` is unset
   AND `open_gaps` contains a line of the form
   `needs_test_harness: <description>`. The description must be
   concrete — name the thing that would need to be verified
   (a protocol, a system boundary, a market dynamic, a policy
   relationship, a measurement). A vague gap
   ("needs_test_harness: some kind of verification") is an
   insufficient gap — list it as a reviewer finding.

2. **No-verification path (Path 3):** `test_harness` is unset
   AND `open_gaps` contains
   `needs_test_harness: not applicable — <reason>`. Use case:
   the work has no external boundary that a future verification
   step would test — informational research, comparative
   analysis, market or policy review, decision substrate, or
   in-process pure-logic work. The reason after the dash MUST
   be substantive ("informational research on policy trends, no
   concrete verification surface"; "comparative analysis of
   market positioning, no integration target"; "in-process
   pure-logic work, no external boundary"). A blank reason
   (`needs_test_harness: not applicable` with nothing after)
   passes Validate but is a reviewer finding — flag as
   insufficient and ask for the reason.

## When path 3 is suspicious

The "not applicable" escape is honest for genuinely
boundary-less work but becomes a get-out-of-jail card if
overused. If the artifact's `integration_points` describe
real external boundaries (system-to-system data flows,
verifiable market interactions, named policy enforcement
relationships) AND the researcher took path 3, the paths
contradict — pick path 2 instead. Reject as `insufficient`.

## How to phrase the rejection

If the artifact has external integration points but no
`needs_test_harness:` line:

```
- Verification stance unstated. Add a single
  `needs_test_harness: <what would need verifying>` line to
  open_gaps. The artifact's integration_points describe
  real external boundaries, so silence is rejection.
```

If `test_harness` is non-empty (no catalog exists under this
deployment):

```
- test_harness is set but this deployment retired its harness
  catalog under the ADR-042 MVP-7 follow-up sweep. Leave
  test_harness unset and use a `needs_test_harness: ...`
  open_gap entry instead.
```

If a `needs_test_harness:` line is too vague:

```
- needs_test_harness gap is too abstract. Name the protocol,
  market dynamic, regulatory relationship, or measurement —
  coordinator routing keys off the substance, not the
  presence, of this line.
```

You evaluate the artifact. You do NOT consult an external
catalog — under MVP-7 there is none. Cross-catalog verification
of any future verification target's *fitness* for the work is
the architect phase's job in a future dev-via-spec
re-introduction; you only verify that the researcher took a
stance and that the stance is substantive.
