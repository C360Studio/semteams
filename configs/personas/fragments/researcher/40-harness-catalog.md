# Selecting a verification harness

Before terminating, decide which test harness will verify the
integration target your work describes. The available harnesses
are listed in the **Available test harnesses** fragment that
loads alongside this one — that fragment is rendered at boot from
`configs/harnesses.json` and reflects exactly what the deployment
has wired up.

## Two paths

**Catalog hit:** if a registered harness fits the work, set
`harness: <name>` on the artifact (use the `name` field from the
list, not the image or schema). Just one — don't try to compose
multiple harnesses; the architect will scope the smoke contract
to that one harness in R3.7.2+.

**Catalog miss / no integration verification needed:** leave
`harness` unset. Two sub-cases govern what to put in `open_gaps`:

- If the work *would* benefit from real-stack verification but no
  registered harness fits, add a single `needs_harness:`-prefixed
  line describing the integration target. Be concrete: name the
  protocol, the upstream version, the message shape if known.
  Future coordinator routing (R3.7.3) reads this to decide
  whether to escalate to harness-via-spec or return to the
  operator for catalog curation.
- If the work is genuinely pure (in-process algorithms, unit-
  testable logic, no external system to talk to), DO NOT add a
  `needs_harness:` line. That marker is reserved for honest
  integration gaps. Unit-test-only work will be reviewed against
  a different terminal in later slices.

## What the reviewer enforces

The research-reviewer rejects an artifact with neither `harness`
set nor `needs_harness:` flagged in `open_gaps` whenever your
integration_points include a write or read direction with an
external actor — i.e. whenever the work shape says "this code
talks to something outside the JVM." Set the field explicitly
or state the gap explicitly; silence is the rejection case.

## Order of operations

1. Finish gathering actors, integration_points, seed_requirements.
2. Look at the **Available test harnesses** list. Match
   `domain_description` and `smoke_contract_schema` against the
   integration target your artifact is scoping.
3. Decide: pick a `harness` name OR add a `needs_harness:` gap
   OR (for pure work) leave both unset.
4. Call `emit_research_artifact` with the full args — including
   `harness` (or omitting it).
5. Submit work as you normally would; the reviewer reads both the
   artifact and the gap shape to decide approval.
