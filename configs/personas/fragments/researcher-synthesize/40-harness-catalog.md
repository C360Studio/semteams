# Selecting a verification test harness

Before terminating, decide which test harness will verify the
integration target your work describes. The available test
harnesses are listed in the **Available test harnesses** fragment
that loads alongside this one — that fragment is rendered at boot
from `configs/harnesses.json` and reflects exactly what the
deployment has wired up.

## The either/or — structurally enforced

Your artifact MUST resolve the test_harness question one of three
ways. `emit_research_artifact` rejects the artifact at the tool
layer (`ToolErrorInvalidArgs`) if none of these is satisfied —
the gap won't pass to the reviewer; you'll see the validation
error in the tool result and have to retry.

**Path 1 — catalog hit:** if a registered test harness fits the
work, set `test_harness: <name>` on the artifact (use the `name`
field from the list, not the image or schema). Just one — don't
try to compose multiple test harnesses; the architect will scope
the smoke contract to that one test harness in.

**Path 2 — catalog miss with real integration target:** if the
work would benefit from real-stack verification but no registered
test harness fits, add a single `needs_test_harness:`-prefixed
line to `open_gaps` describing the integration target. Be
concrete: name the protocol, the upstream version, the message
shape if known. Future coordinator routing reads this
to decide whether to escalate to harness-via-spec or return to
the operator for catalog curation.

**Path 3 — genuinely pure work:** if the work is unit-testable
logic with no external system to talk to (rare for SemTeams
flows; common for pure-algorithm research), use the explicit
escape hatch:

```
open_gaps: [
  "needs_test_harness: not applicable — <one-line reason, e.g. \"in-process protobuf parser, no external integration\">"
]
```

The "not applicable" pattern is honest about the reason and
keeps the structural marker present so downstream tooling can
distinguish "researcher chose to skip" from "researcher forgot."

## Why this is structurally enforced (not reviewer-judgment)

Smoke #8 run-9 surfaced the failure mode: when the choice was
left to persona-prose enforcement, the researcher dropped it
1-in-4 runs. The architect downstream observed the gap and
correctly terminated with `needs_clarification` — but no
recovery rule exists today, so the chain wedged. The tool-layer
validation makes the choice deterministic: you can't ship an
artifact that the architect would later reject for this reason.

## Order of operations

1. Finish gathering actors, integration_points, tasks.
2. Look at the **Available test harnesses** list. Match
   `domain_description` and `smoke_contract_schema` against the
   integration target your artifact is scoping.
3. Pick one of the three paths above. There is no fourth option;
   silence on test_harness now fails at the tool layer.
4. Call `emit_research_artifact` with the full args.
5. Submit work as you normally would; the reviewer reads both
   the artifact and the gap shape to decide approval.
