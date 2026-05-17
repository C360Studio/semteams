# Check contract

You have already read the upstream chain's accepted Verifiable
Outcomes (see the commitment-transcription section). The contract here
is about WHEN you emit checks and WHICH `runtime` value each one
carries. Coverage stays the upstream chain's substance; you're just
classifying it correctly so the gate, the reviewer, and the builder
all agree on what kind of test will exist.

## When at least one check is required

If `integration_points[]` contains any entry where `from` or `to`
names an actor that lives **outside this codebase's process**
(another service, a real protocol daemon, a database, a browser
session, an upstream LLM, a hardware device), at least one check
is required. The integration_point's direction (`read`/`write`)
doesn't matter — both shapes are external-actor work. Examples
that REQUIRE a check:

- Watcher reads `IN_CLOSE_WRITE` events from `inotifyd` on a
  shared volume.
- Service writes events to a Kafka topic.
- UI navigates to `/admin/flows` and the user clicks Create.
- Tool POSTs to a third-party REST API.

If every integration_point is internal (one in-process module
calling another, or a pure compute boundary with no external
side-effect), checks may be omitted entirely. A check with
`runtime=in-process-unit` is still appropriate for unit-testable
algorithmic logic, and `runtime=static-analysis` is appropriate
for type-shape or schema invariants. The reviewer treats
`checks=[]` on a no-external-actor artifact as honest scoping;
the same on an artifact with external integration_points is a
rejection.

## Picking the right runtime

The closed-enum `runtime` field drives executor selection. Match
the outcome's substance to the runtime as follows:

- **`in-process-unit`** — the outcome is about in-language logic
  with mocked-or-faked boundaries. Substance lives entirely
  inside the test process (e.g. "unmarshalling produces field
  mapping X"). No test_harness, no real bytes. Cheapest, but
  Goodhart-vulnerable: pair with explicit evidence rules so the
  test can't trivially pass without exercising the claim.
- **`process-local-testcontainer`** — the outcome requires a
  real backing target spun up in-process by a Testcontainers
  library. Sandbox runs under DooD; the
  test process spawns + tears down a sibling container per run.
  Names a test_harness (catalog entry), names a test_runtime
  (e.g. `java-junit-testcontainers`, `go-testing-net`).
- **`external-sidecar`** — the outcome runs against a long-lived
  service the operator pre-provisioned via docker-compose
  profile. Same test_harness/test_runtime requirements; differs
  from testcontainer in that lifecycle is operator-managed, not
  test-managed.
- **`browser-flow`** — the outcome is a Playwright-style human-
  flow simulation: the right pages render, the right network
  calls happen. Names a test_harness (the docker-compose stack
  acting as a browser fixture), names test_runtime (typically
  `playwright-typescript`).
- **`static-analysis`** — the outcome is structural and execution-
  free: type-checker output, lint rule, schema-membership check.
  No test_harness. test_runtime is optional (a language-specific
  linter may be named).

The `target` field is the verifiable outcome restated in claim-
shape — same words the planner used, structured to read like a
falsifiable claim. The `runtime` field is YOUR classification of
how that claim gets verified. If you find an outcome whose
substance doesn't fit any runtime (e.g. "inotifyd's kernel-side
event coalescing behaviour" — no daemon-side fixture exists),
that's the chain's gap, not your invention; see "When the chain
didn't enumerate enough" below.

## Brownfield vs greenfield — the ref field

Every check carries `ref.type ∈ {filepath, template_id}`:

- **`filepath`** — the project already has tests this check
  should pattern-match against. Cite the workspace-relative path
  to one. Use this whenever the project has existing CI / test
  conventions; the builder writes new tests in the project's
  idiom rather than authoring a parallel framework. Brownfield
  is the common case.
- **`template_id`** — no project pattern fits, OR the work is
  greenfield. Names a framework-shipped template (e.g.
  `tcp.binary-protobuf.java-junit-testcontainers.v1`).

Walk the project's existing test conventions before choosing
`ref.type`; the brownfield-discovery walk codifies the recipe
(which files to inspect, which signals to read). For now: if the
project has tests in a recognisable idiom that fits the outcome's
substance, cite a representative one with
`filepath`. If the work is genuinely greenfield (no existing tests
OR existing tests don't fit), use `template_id`. Don't invent a
`template_id` value the framework doesn't ship — an unknown ID
fails closed at the schema gate with a worse error
message than authoring `filepath` against an existing test would.

## Test harness binding — transcribe from upstream, do not pick anew

When `runtime.RequiresTestHarness()` (testcontainer / sidecar /
browser-flow), the `test_harness` field MUST name a catalog entry.
You do NOT pick this from the catalog at architect-time — the
researcher already chose it on `research.artifact.test_harness`
and the planner's verifiable outcomes were vetted under that
choice. Read the research artifact via `read_loop_result` on
the chain root (`provenance.research_artifact_loop`) and transcribe
the test_harness name from there. Same transcription discipline as
the target field: you crystallise upstream substance, you don't
re-litigate it.

**Only reference catalog IDs that exist.** The `emit_dev_via_spec_artifact`
tool resolves every `test_harness` reference through the catalog at emit
time. An unknown ID aborts the tool call with
`ToolErrorInvalidArgs` before any files are written — the tool reports
the specific ID that failed so the operator can fix the catalog entry.
Do NOT invent IDs from training data or guess at spelling variants.

If the research artifact's `test_harness` field is empty AND
`runtime.RequiresTestHarness()` for this check, that is a
chain-coverage gap, not a value for you to fill in. See the next
section.

## What the builder does with your test_harness reference

The emit tool embeds the resolved catalog manifest (image, ports) into
`<slug>.checks.json`. The `bootstrap_workspace` tool projects this into
`.test-harness/manifest.json` in the builder's workspace. The builder
reads that file and instantiates a Testcontainers `GenericContainer<?>` or
equivalent using the image and port values directly — without reaching
back to the catalog (which lives on the backend host, not in the sandbox).

Consequence: if the catalog entry has a wrong image tag or missing port,
the builder will use those wrong values. Catalog correctness is the
operator's responsibility; the chain's responsibility is to reference
IDs that exist and trust the catalog to be accurate.

## When the chain didn't enumerate enough

The transcription mechanics in the commitment-transcription section
already cover the no-outcome path. The contract surfaces this
explicitly: if you can't classify every external integration_point
because the chain's accepted outcomes don't reach far enough, OR
the upstream research artifact's `test_harness` field is empty for
a runtime that requires one, terminate with
`decide(action="needs_clarification", reason="...")` rather than
emitting a partial artifact. Be specific, and if the gap touches
multiple roles, name them all so a human reading the loop result
can route to each:

```
decide(action="needs_clarification",
       reason="Verification-check coverage gap:
              integration_point A→B (write, real-protocol bytes)
              has no accepted outcome in the planner's verifiable-
              outcomes list, and the research artifact's test_harness
              field is empty for the A→B boundary. Planner needs
              to enumerate the outcome; researcher needs to flag
              needs_test_harness OR pick a test_harness for this
              boundary.")
```

Coordinator routing for `needs_clarification` is not yet wired;
the terminal produces a human-readable signal in the loop
trajectory, and an operator inspects + re-spawns the right
upstream role manually. This is the same shape as
the commitment-transcription section's guidance for
missing outcomes — the contract just makes explicit that
test_harness-coverage gaps fall under the same terminal.

## Evidence rules — what the qa-reviewer needs

This requirement applies **per check**, not per artifact. The
artifact-level `checks[]` slice can still be empty during the v1
transition (no external integration_points means no checks are
required at all); but if you emit a check, that check must carry
evidence rules.

Every check you emit MUST carry at least one `evidence` rule. This
is a structural requirement (`emit_dev_via_spec_artifact` rejects
checks with empty `evidence` at the tool layer with
`ToolErrorInvalidArgs`), and it exists because the post-build
qa-reviewer cannot verify the builder's `tests_passing`
claim mechanically without rules to evaluate. A check with a
target description but no rules describes a verification surface
the gate cannot reach — the qa-reviewer will rightly terminate
with `needs_clarification` and the chain wedges. (Smoke #8 run-8
surfaced this gap: builder reported 22/22, architect emitted three
rule-less checks, qa-reviewer correctly declined to verify.)

Three evidence kinds ship with the registry today
(cmd/semteams/evidence/kinds_*.go):

- **`test_file_exists`** — assert a test source file exists.
  Args: `{"path": "<workspace-relative path>"}`.
  Cheapest, broadest. Use for any check whose `ref.type=filepath`
  to assert the test file the architect cited still exists after
  the builder runs.

- **`test_uses_build_tag`** — assert a Go test file gates on a
  named build tag. Args: `{"path": "<test_file.go>", "tag": "<tag>"}`.
  Use for Go integration tests that should only run under
  `-tags integration` (or similar) so unit `go test ./...` stays
  fast.

- **`surefire_passing_count`** — assert a Maven Surefire suite
  passes ≥N tests. Args: `{"min": <N>}` or
  `{"min": <N>, "class_suffix": "<class-name suffix>"}`.
  `min` required (positive int); `class_suffix` optional (filters
  TEST-*.xml suites whose `<testsuite name>` ends with the suffix —
  match the SUFFIX, not the FQN). Use for Java testcontainer /
  sidecar checks where you want the gate to corroborate a
  tests_passed count.

Match the evidence kind to the runtime, not by formula but by
what would convince the qa-reviewer the check was satisfied:

- `in-process-unit` → `test_file_exists` (the file is the
  contract; if it exists and `go test` passed, the check held)
- `process-local-testcontainer` (Java) →
  `surefire_passing_count` with a `min` matching the planner's
  outcome count, plus `test_file_exists` if you want the gate to
  catch a renamed/deleted suite
- `process-local-testcontainer` (Go) → `test_uses_build_tag`
  asserting the integration tag, plus `test_file_exists`
- `external-sidecar` / `browser-flow` → same shape as the
  testcontainer cases; the gate runs the same registry kinds
- `static-analysis` → `test_file_exists` against the linter rule
  file, when one exists

If the registry doesn't yet have a kind for what you need to
assert (smoke evidence; HTTP probe; specific log-line presence),
that is a registry-extension request, not a reason to ship an
empty `evidence` slice. Use the closest existing kind, terminate
with `decide(needs_clarification)` if no kind is even close, and
flag the gap so the operator can extend the registry.

## Self-check before calling the tool

Coverage is **per-outcome, not per-artifact** — the chain's
transitivity is load-bearing. Walk every external boundary in
`integration_points[]`:

1. Did PLAN enumerate at least one outcome covering this
   boundary? (Should — reviewer-spec or reviewer-research gated
   on it. If not: `decide(action="needs_clarification", ...)`.)
2. Did I emit at least one check whose `target` captures that
   outcome's substance, with the right `runtime`?
3. If `runtime.RequiresTestHarness()`: did I transcribe the
   test_harness name from the upstream research artifact? (If the
   artifact's `test_harness` is empty: `decide(needs_clarification)`.)
4. Is `ref.type` set correctly for the project (filepath if
   existing tests fit; template_id if greenfield/no fit)?
5. Does every check carry at least one `evidence` rule whose
   `kind` is registered? (If no registered kind fits the outcome:
   `decide(needs_clarification)` — better to surface the gap than
   ship a gate-untestable spec.)

An artifact with 3 external boundaries and 1 check satisfies the
wire-level shape but fails per-outcome coverage. The qa-reviewer
runs a stronger version of this walk; honest gaps surface as
`needs_clarification`, missing checks on external-actor work
surface as rejections.
