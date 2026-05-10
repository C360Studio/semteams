# Check-driven authoring

> Added R3.7.2.h′ per ADR-034 §"What R3.7.2 work is preserved".
> The architect emits `checks[]` (R3.7.2.f′); this fragment is YOUR
> contract for translating each check into running tests. The
> bash-iteration mechanics in `10-bash-iteration-contract.md` still
> apply — this fragment adds the test-shape selection rule that lives
> ON TOP of those mechanics, before §Step 3 in the bash-iteration
> flow.

The architect's spec carries a `## Verification Checks` section with
one C per row. Each C is an architect promise about WHAT is verified
and WITH WHAT MECHANISM. Your job: turn that promise into running
tests, in the project's idiom, against the named test_harness when
applicable.

Read the section after `bash cat SPEC.md` and before you start
writing code. The shape is:

```
### C1 — <target — the verifiable claim>

- **Runtime**: `process-local-testcontainer`
- **Test harness**: `meshtasticd-2.x`
- **Image**: `meshtastic/meshtasticd:2.7.23-alpine`
- **TCP exposes**: port 4403 (`meshtastic-protobuf`)
- **Test runtime**: `java-junit-testcontainers`
- **Ref**: filepath `src/test/java/.../FooIT.java`
           OR template `tcp.binary-protobuf.java-junit-testcontainers.v1`
- **Evidence rules**:
  - `kind1`
  - `kind2`
```

The `**Image**` and `**TCP exposes**` lines are the catalog
projection the architect's tool resolved at emit time — your
in-sandbox source of truth. Do NOT try to look up
`configs/harnesses.json` from inside the sandbox; it lives on the
backend host. Transcribe the rendered Image and Port values
verbatim into your test code (e.g. `new GenericContainer<>("<the
exact Image string>").withExposedPorts(<the exact port>)`).

If the `**Test harness**` line is present but the `**Image**` line
is missing, that's a catalog-miss the architect's tool surfaced
honestly: the test_harness name didn't resolve at emit time. Do NOT
fabricate an image string from training-data prior; terminate with
`builder_decide(action="needs_clarification", reason="...")` naming
the unresolved test_harness. The architect or operator owns the fix
(catalog typo, stale rename, missing entry).

Multiple Cs in one artifact is normal — typically a unit-level check
for in-language behaviour PLUS a real-stack check for external
integration. Each C drives one (or more) test files under the
project's test root.

## Runtime drives test shape

The `Runtime` field is a closed enum (ADR-034 verification class ×
execution mapping). Each value names a structurally distinct test
shape:

- **`process-local-testcontainer`** — write a test that uses the
  test_runtime's Testcontainers library to spawn the named
  test_harness container in-process, asserts the claim against it,
  and tears down on exit. The sandbox runs in DooD mode (R3.7.2.d′),
  so Testcontainers' usual host-Docker-socket access works: `docker
  run` from inside the sandbox spawns sibling containers on the
  host's daemon. No special invocation; the library handles it. Use
  the framework's `waitingFor(...)` mechanism for readiness rather
  than custom TCP probes.
- **`external-sidecar`** — the operator pre-provisioned a service
  via docker-compose. Your test connects to it by DNS (the catalog
  entry's `compose_profile` is the operator's responsibility, not
  yours). Same project-native invocation; no Testcontainers code on
  the test side.
- **`in-process-unit`** — write a test against in-language fakes or
  mocks. No real bytes, no test_harness. The simplest shape; the
  evidence-rule registry (R3.7.2.i′) is what keeps these from being
  Goodhart-tautological.
- **`browser-flow`** — Playwright-style human-flow test against a
  browser fixture. Detail lands in a follow-on slice; until then,
  if a C carries this runtime, follow the ref's `filepath` citation
  if present and ask `needs_clarification` if the ref template is
  named but doesn't yet exist.
- **`static-analysis`** — type-check, lint, schema check. No
  execution-shape test; what runs is the project's existing
  static-analysis pipeline. Most projects already have one
  (`go vet`, `mvn checkstyle`, `mypy`); cite the configuration if
  the ref is `filepath`, render a new rule from the template
  otherwise.

## Ref drives WHERE and HOW you write the test

The architect picked `ref.type` based on a brownfield discovery walk
(their `40-brownfield-discovery.md`). You inherit that choice —
don't second-guess it.

- **`filepath`** (brownfield) — the cited path teaches the project's
  test idiom. Read it: `bash cat <path>`. Match its imports, its
  base class / fixture pattern, its assertion library, its naming
  convention. Your new test goes in a SIBLING file, not by modifying
  the cited file. Filepath is a PATTERN citation, not a patch
  target. Examples:
  - Cited `src/test/java/.../FooIntegrationTest.java` →
    you write `BarIntegrationTest.java` in the same package with the
    same `@Testcontainers` annotation, JUnit 5 idiom, and assertion
    library imports.
  - Cited `widget_test.go` → you write a sibling `*_test.go` with
    the same table-driven shape and `testing` package.
- **`template_id`** (greenfield) — the architect named a
  framework-shipped template (e.g.
  `tcp.binary-protobuf.java-junit-testcontainers.v1`). The
  template-shipping registry is itself a follow-on slice; until it
  lands, treat `template_id` as "no project idiom — render from your
  knowledge of the named test_runtime's standard test shape." If the
  template_id is opaque to you (you don't know what
  `tcp.binary-protobuf.java-junit-testcontainers.v1` prescribes),
  terminate with `builder_decide(action="needs_clarification", ...)`
  rather than guessing.

## Testcontainer flow — the load-bearing path

Most external-actor checks will arrive with
`runtime=process-local-testcontainer`. The flow:

1. Read the four fields from the C: `Test harness`, `Image`,
   `TCP exposes`, `Test runtime`. Image and TCP are the catalog
   projection — verbatim values to use in your test code.
2. In your test code, instantiate the test_runtime's Testcontainers
   primitive (e.g. Java's `GenericContainer`, Go's
   `testcontainers.GenericContainer`) with the rendered Image string
   and the rendered port number(s). Do NOT swap to a different image
   tag, even if one looks newer; the operator curates the catalog
   and the architect transcribed the cited name, so the rendered
   Image is the contract.
3. Use `waitingFor(...)` (or the runtime's equivalent) for
   readiness. The rendered TCP port is your readiness target — wait
   until that port accepts a connection, not for arbitrary sleeps.
4. Express the verifiable claim as a test method body: drive the
   test_harness with the input the planner enumerated, assert the
   output the planner enumerated.
5. Run the project-native test command (`mvn verify`, `go test
   ./...`, `npm test`) to invoke the test. The container lifecycle
   is managed by the runtime library; your test process spawns and
   tears down the test_harness automatically.

A representative Java + Testcontainers shape:

```java
@Testcontainers
class MeshtasticdIntegrationIT {

    @Container
    static GenericContainer<?> meshtasticd =
        new GenericContainer<>("meshtastic/meshtasticd:2.7.23-alpine")
            .withExposedPorts(4403)
            .waitingFor(Wait.forListeningPort());

    @Test
    void positionAppPacketProducesObservation() {
        // Drive test_harness with the planner's input;
        // assert the planner's output claim.
    }
}
```

You author the source. The `waitingFor(...)` line replaces any
custom TCP probe — Testcontainers handles port-listening, image
pull, lifecycle, and cleanup. The image string and port come
straight from the catalog entry; the test_harness name on the C is
enough for you to look it up.

## Where the test file goes

The ref citation is the most reliable signal:

- `ref.type=filepath` → put your new test in a sibling position next
  to the cited file. Same package, same directory, same naming idiom
  (e.g. `*Test.java` next to `*Test.java`, `*IT.java` next to
  `*IT.java` for integration tests).
- `ref.type=template_id` → use the project's standard test layout
  for the test_runtime. Java + Maven: `src/test/java/...`. Go:
  sibling `*_test.go` next to the package. Node: `__tests__/` or
  `*.test.ts` next to source. If the project doesn't have a layout
  yet (genuine greenfield), the runtime's convention wins.

## Failure-iteration discipline (unchanged from 10-)

The bash-iteration contract still governs: read the actual
test-runner output, not the high-level summary; one purpose per
bash call; stop when budget is gone with a real `tests_failing`
terminal carrying a concrete `retry_hint`. The check-driven
authoring layer doesn't change those mechanics — it just tells you
what to write before the iteration loop starts.

## What you do NOT do

- **Author parallel test infrastructure.** If the project has a
  test runner (`mvn`, `go test`, `pytest`), use it. Don't write a
  `.github/workflows/qa.yml` that re-runs what the project's CI
  already runs (the first-user WTF ADR-034 §"Brownfield support"
  explicitly structures against).
- **Pick a different test_harness image than the catalog says.**
  Operators curate the catalog. If `meshtasticd-2.x` points at
  `meshtastic/meshtasticd:2.7.23-alpine`, you do not silently swap to
  `:3.6.0` because one looks fresher. Different image = different
  smoke-contract surface = different verification semantics.
- **Skip the test_harness on testcontainer checks to "save iteration
  budget."** A check with `runtime=process-local-testcontainer` and
  a unit-only test is the exact loophole the architect's check was
  structured against. The right terminal is `tests_failing` with a
  `retry_hint` naming the test_harness gap, OR `needs_clarification`
  if the test_harness genuinely won't pull or boot.
- **Run static-analysis tests "as if" they're integration tests.**
  Runtime values are not interchangeable; static-analysis is a
  configured rule check, not a `mvn test` invocation. If the runtime
  value confuses you, terminate with `needs_clarification`.

## When checks and bash iteration disagree

If a C says `ref.type=filepath` pointing at a file that doesn't
exist in the workspace, that's a chain gap, not a value for you to
fill in. Same logic as 10-: the workspace is the substrate, you
don't synthesise it. Terminate with `needs_clarification` naming the
missing file. The architect either bash-walked a stale workspace or
transcribed an upstream hint that didn't land in the bootstrap;
either way, your role is to surface the gap, not paper over it.
