# ADR-034: Verification execution via verification-runner pattern

## Status

Proposed — 2026-05-04 (revised same day for verification-class
boundaries + brownfield support + naming).

Pivots R3.7.2 line away from bespoke (family × runtime) orchestration
toward an event-driven verification-runner pattern, with sandbox+
Testcontainers handling unit/integration cases and the runner reserved
for genuine browser-flow / e2e / greenfield-no-CI cases. R3.7.2.a/b/c
remain valid; R3.7.2.d (closed in PR #59) and the planned R3.7.2.e/f/g/h/i
are reshaped.

## Context

R3.7.2's working design was a (family × runtime) matrix:

- **Protocol family** (sidecar-side, language-agnostic): defines wire-
  level contracts, ships per-family JSON-shape validators.
- **Test runtime** (driver-side, per-language): owns invocation
  command (`mvn verify`, `go test`, `npx playwright test`) and
  per-family rendering templates.
- **Harness catalog**: operator-curated `configs/harnesses.json`
  with `compose_profile` field referencing docker-compose services.

R3.7.2.d (PR #59, closed before merge) shipped the family + runtime
registries with one entry each (`tcp.binary-protobuf.v1`,
`java-junit-testcontainers`).

A pro-devops review surfaced the design's gaps: the `compose_profile`
field bakes the orchestrator into the wire format; we have no GitOps
or multi-tenancy story; we'd reinvented orchestration. While walking
through gaps, surveying our sibling repo **semspec** turned up a
`qa-runner` component that's structurally well-shaped — and is the
pattern a senior devops person would have designed if they'd been on
this from day one.

A second-pass review of that proposal (this revision) surfaced two
sharpenings: lean on the language ecosystem's standard (Testcontainers)
for integration tests rather than routing everything through act; and
treat brownfield projects (existing CI, existing test runner, existing
conventions) as the common-case rather than the edge case. Both
narrow the runner's scope and improve the design.

### What semspec's qa-runner is

A standalone Docker-containerized binary that consumes
`workflow.events.qa.requested` events from NATS JetStream and
publishes `workflow.events.qa.completed`. The execution model:

```
plan-manager ──qa.requested──> NATS ──> qa-runner consumer
                                            │
                                            ├─ unit mode → sandbox executor
                                            └─ integration|full mode → act runs
                                                                       .github/workflows/qa.yml

qa-runner ──qa.completed──> NATS ──> qa-reviewer (LLM)
                                          │
                                          └─ qa.verdict ──> plan-manager
```

Key properties:

1. **Event-driven decoupling**: chain publishes a job; runner
   consumes; chain consumes the result. Chain doesn't know how or
   where tests ran.
2. **`act` as execution engine** (GitHub Actions emulator). Local
   runs of `.github/workflows/qa.yml` against
   `catthehacker/ubuntu:act-latest` images. Universal CI vernacular.
3. **Workspace bind-mount** via Docker socket.
4. **Standardized result envelope**: `QAFailure`, `QAArtifactRef`,
   duration, runner error vs test failure distinction.
5. **Mode-based routing**: unit → sandbox; integration|full → act.
6. **ARC-compatible swap path** for k8s without changing event shape.

### Honest assessment of qa-runner maturity

semspec's qa-runner is **DESIGNED + BUILT but mostly UNUSED** —
six weeks old (April 2026), fresh implementation, NOT battle-tested
production-grade. The pattern is right; the implementation needs
SemTeams-specific refinement during the port.

### Naming

semspec calls it `qa-runner` because semspec has a "qa" role
controlling it. SemTeams has no qa role; the runner is mechanical
infrastructure that responds to events. We name our component
**`verification-runner`** — matches our ADR-033 vocabulary
(verification-commitment, evidence-gate, verification-runner) and
matches GitHub Actions / CI vernacular every devops engineer
recognizes. References to semspec's component keep its `qa-runner`
name (that's what they call it).

## Decision

**Pivot R3.7.2's execution boundary to a verification-runner
pattern, with carefully-bounded scope:**

1. The architect's `Approach` enum (R3.7.2.a) drives executor
   selection. The runner is invoked only when act's workflow-level
   orchestration buys something the sandbox can't deliver.

2. **Most verification execution stays in the sandbox**, augmented
   with DinD (Docker-in-Docker) so Testcontainers can manage sidecar
   lifecycle from in-process. This is the language-ecosystem-standard
   pattern and works across Java, Go, Python, Node, Rust, .NET.

3. **verification-runner is invoked specifically for**:
   - `browser-flow` Approach (Playwright/Cypress against an
     orchestrated stack)
   - Greenfield e2e where no existing CI exists to extend
   - Cases where workflow YAML's job-level orchestration is genuinely
     needed (multi-job pipelines, artifact uploads, CI-parity
     verification)

4. **Brownfield is a first-class case**: when the project has
   existing CI / test runner / conventions, the chain extends them
   rather than authoring parallel verification surfaces. The
   architect discovers the project's conventions before emitting
   commitments; the builder writes test code matching the project's
   idiom; sandbox runs the project-native invocation.

5. The bespoke (family × runtime) matrix is **withdrawn**.
   `verification.Commitment` (R3.7.2.a, merged) and the artifact
   widening (R3.7.2.b, merged) remain valid. Upstream verifiability
   personas (R3.7.2.c, merged) remain valid. The harness catalog
   (R3.7.1, merged) survives in modified shape — declares
   "available services" rather than "compose profiles."

## Verification class → execution mapping

This table is the central contract. The architect's `Approach`
choice — combined with the brownfield/greenfield discrimination
already encoded in `ConventionRef.type` — determines the executor.

| Approach | ConventionRef.type | Executor | Mechanism |
|---|---|---|---|
| `in-process-unit` | filepath (brownfield) | sandbox | builder runs project-native command (`mvn test -DskipITs`, `go test ./...`, `pytest -m unit`, etc.) |
| `in-process-unit` | template_id (greenfield) | sandbox | builder writes test from template; runs language-native test runner |
| `process-local-testcontainer` | filepath (brownfield) | sandbox + DinD | builder writes test code matching existing conventions; project-native command (e.g. `mvn verify`); Testcontainers manages sidecar in-process |
| `process-local-testcontainer` | template_id (greenfield) | sandbox + DinD | builder writes test from template using Testcontainers library; runs language-native command |
| `external-sidecar` | filepath (brownfield) | sandbox + project-managed sidecar | tests connect to operator-/CI-managed sidecar via DNS; project-native test invocation |
| `external-sidecar` | template_id (greenfield) | sandbox + harness-catalog sidecar | operator pre-provisioned the sidecar via R3.7.1 harness catalog; tests connect via declared port |
| `browser-flow` | filepath (brownfield) | sandbox runs project's existing Playwright invocation | new spec file in project's `e2e/` dir; existing `npx playwright test` finds it; works against project's stack |
| `browser-flow` | template_id (greenfield) | **verification-runner via act** | chain authors `.github/workflows/qa.yml` with browser + backend services; act runs it; result event drives chain |
| `static-analysis` | either | sandbox | project-native linter/type-check (`go vet`, `mvn checkstyle`, `mypy`, etc.) |

Two boundaries set the runner's scope:

- **unit ↔ integration**: does the test need real Docker infra? If
  yes, integration. Sandbox grows DinD; everything stays local.
- **integration ↔ e2e**: multiple components OR browser
  orchestration? If yes, e2e — and only the greenfield-no-existing-
  CI case routes to verification-runner.

Most real adoption ships under the brownfield rows. The runner
appears in maybe 20–30% of cases over the project lifetime —
specifically OSH-Meshtastic-style smoke #7 (greenfield from-scratch
project) and any multi-component browser-flow work. Bias the scope
of work this ADR generates accordingly.

## Brownfield support

**Brownfield is the common case.** Most adoption targets are:
- Repositories with existing test runners (`mvn`, `go test`, `npm
  test`, `pytest`, `cargo test`)
- Established CI workflows (`.github/workflows/ci.yml`,
  `.gitlab-ci.yml`, `Jenkinsfile`)
- Project-specific conventions (test directory layout, fixture
  patterns, mock infrastructure)
- Project-specific helpers (database fixtures, test data factories,
  custom matchers)

The chain must DISCOVER and EXTEND these — not author parallel
verification surfaces. The first-user WTF is "the chain wrote a
new `.github/workflows/qa.yml` that runs the same tests my
existing CI runs, but reports to a different dashboard."

### Discovery is the architect's job (persona discipline)

Before emitting commitments, the architect explores the workspace
via bash:

- `cat pom.xml` / `cat go.mod` / `cat package.json` — language and
  build system
- `ls -la .github/workflows .gitlab-ci.yml Jenkinsfile` — existing
  CI
- `find . -path "*/test/*" -o -name "*_test.go" -o -name "*Test.java"
  | head -10` — test convention
- `cat Makefile | grep -E "^test|^check|^ci"` — common project
  test targets
- Read 1–2 existing test files to learn the convention pattern

The commitment then references the existing convention via
`ConventionRef.type=filepath`. The architect transcribes the planner's
verifiable outcomes into the project's idiom — not a foreign one.

### When verification-runner is NOT used in brownfield

Almost always. The runner is event-driven by design; for projects
that already have `mvn test` or equivalent, the chain calls bash
in sandbox to run the project's native command. Sandbox returns
the output; builder reads and acts. No qa.requested round-trip.

### When verification-runner IS used in brownfield

Rare cases: the project has e2e tests but no CI configured for
them, or the new verification needs orchestration the project's CI
doesn't yet provide. In those cases the chain may author a small
workflow YAML the runner executes — but it's an exception path,
not the default.

### Open: discovery tool vs persona discipline

Should we ship a `discover_test_setup` tool that returns
structured project conventions? Pro: consistent surface; LLM
doesn't need to bash-walk. Con: more tool sprawl (Coby's 2026-05-03
fewer-rich-tools principle), and the discovery is genuinely
project-specific in ways structured tools struggle to capture.

**Lean: persona discipline now; tool only if real adoption shows
the bash-walk is the bottleneck.** Doc fragments per architect role
will spell out the discovery sequence; bash is the primitive.

## Consequences

### What R3.7.2 work is preserved

- **R3.7.2.a `verification.Commitment` primitive (PR #56)** —
  unchanged. `target`/`approach`/`convention`/`evidence` all still
  meaningful. The `Approach` enum becomes the routing-classifier
  per the mapping table above.
- **R3.7.2.b `dev_via_spec.artifact.VerificationCommitments[]`
  widening (PR #57)** — unchanged.
- **R3.7.2.c upstream verifiability personas (PR #58)** —
  unchanged. The transcription target (commitment.target) is now a
  workflow scenario / a project-native test description, but the
  discipline is identical.
- **Harness catalog (R3.7.1, PR #55)** — partially preserved.
  `compose_profile` field becomes `services_yaml` fragment or
  workflow-include reference; remains optional.

### What changes / is replaced

- **R3.7.2.d (closed in PR #59)** — `families.Registry` /
  `runtimes.Registry` interfaces. Withdrawn. The matrix is implicit
  in workflow YAML (`runs-on:` + `services:`) for greenfield
  browser-flow cases; for everything else, the language ecosystem's
  standard (Testcontainers + native test runner) is the matrix.
- **R3.7.2.e (planned: first sidecar harness `meshtasticd-3.x`)**
   — reshaped for greenfield browser-flow OR external-sidecar
   testcontainer cases. The catalog declares "meshtasticd is
   available"; chain consumes via Testcontainers (greenfield) or
   workflow `services:` (when using act).
- **R3.7.2.f (planned: evidence-rule registry + architect
  contract)** — reshaped. Evidence rules check the appropriate
  artifact for the executor: project-native test source for
  brownfield, workflow YAML for verification-runner cases.
- **R3.7.2.g (planned: builder + sidecar TCP probe + mvn)** —
  largely subsumed. The sandbox running the project's native test
  command IS the builder verification path; the TCP probe becomes
  Testcontainers' built-in `waitingFor(...)` mechanism.
- **R3.7.2.h (planned: evidence gate)** — unchanged in spirit.
  Consumes structured envelopes (sandbox output for in-sandbox
  routes; `verification.completed` event for runner routes).
- **R3.7.2.i (planned: reviewer reads evidence)** — reshaped to
  the qa-reviewer pattern.

### New work

- **Port qa-runner from semspec**, rename to `verification-runner`.
  Subtree-import vs reimplement vs upstream-extract → open question
  (see §Open questions).
- **Sandbox DinD support**: sandbox grows Docker-in-Docker so
  Testcontainers can manage sidecar lifecycle from in-process tests.
  Trust-boundary work: tests can spawn arbitrary containers; DinD
  isolates per-sandbox-instance vs sharing host's Docker daemon.
- **Brownfield discovery persona fragments** for architect
  (1 fragment) explaining the bash-walk for project conventions.
- **Compose stack updates**: add verification-runner service
  alongside sandbox / mock-llm / nats / backend. `act` toolchain
  in the runner image.
- **Evidence-rule kinds for sandbox-route output**: parse
  language-native test runner output (mvn surefire reports, go test
  output, pytest junit XML, npm test JSON) into the evidence
  envelope. R3.7.2.f scope.

### What's deferred (NOT decided in this ADR)

- **k8s backend**. The runner pattern supports it (swap executor for
  ARC); when actually needed, follow-on ADR ships the swap.
- **Multi-tenancy isolation specifics**. Multiple runners +
  sandboxes in multiple namespaces is the obvious shape; per-tenant
  authentication, secrets, network policy — separate ADR when
  driven by a tenant.
- **Secrets primitives**. GitHub Actions `secrets:` syntax exists;
  what backs it is operator policy. Catalog gains a `secret_refs`
  field in a future slice when needed.
- **Subtree-import vs reimplement vs upstream**. Open question
  below.
- **Event subject namespace**. Inherit semspec's
  `workflow.events.qa.*` (and align names) vs adopt our own
  (`verification.events.run.*`)? Coordinate with semspec.
- **Discovery tool vs persona-only**. Lean persona-only; revisit
  if real adoption shows bash-walk friction.
- **`act` image footprint** in production deployments.

## Cost / benefit

### Cost

- **2-3 weeks scope shift on R3.7.2.** R3.7.2.d is gone. R3.7.2.e
  through .i are reshaped. Smoke #7 timeline pushes out by similar.
- **Net-new code to port + adapt.** semspec verification-runner is
  ~600 LOC; SemTeams adaptation includes renaming + sandbox DinD
  reconciliation + brownfield-discovery persona work.
- **Coordinate event subject shape with semspec**. Best done by
  upstreaming to semstreams.

### Benefit

- **Devops vernacular**. Workflow YAML for the cases where the
  runner runs; project-native commands for everything else. Both
  immediately recognizable to any devops engineer.
- **k8s migration path is real**. Runner backend swap (ARC), no
  event-shape changes. Sandbox already runs in containers.
- **Testcontainers leverage**. Mature library across Java, Go,
  Python, Node, Rust, .NET. Cross-language integration testing
  comes for free.
- **Brownfield support is structural**, not retrofit. The
  ConventionRef discriminator already drives it; persona fragments
  do the discovery.
- **Multi-language by design**. Each language's test runner +
  Testcontainers library handles its own integration story.
- **Goodhart resistance is mechanical**. Project-native test
  runner exit codes + structured output parsing for sandbox routes;
  act exit + structured failure parsing for runner routes. Chain
  cannot fake a pass.
- **Multi-tenancy is achievable**. Runner pools per tenant;
  sandbox per builder loop; standard isolation patterns.
- **Smaller verification-runner footprint** than the original
  ADR. Fewer cases to handle; sharper boundary.

## Open questions

1. **Subtree-import vs reimplement vs upstream-to-semstreams.**
   Three options:
   - **Subtree-import** semspec's `cmd/qa-runner/` directly,
     rename to verification-runner. Cheap; couples our runner to
     semspec's event namespace.
   - **Reimplement** the runner in semteams from semspec's
     design. Independent codebases; freedom to adapt; risk of drift.
   - **Upstream to semstreams** as a shared component. Best long-
     term; requires upstream coordination + ADR there.

   Lean: **upstream**. Pattern is generic, both products will use
   it, framework is the right home. Timeline depends on upstream
   priorities.

2. **Sandbox DinD trust-boundary reconciliation.** Today's
   sandbox is hardened (cap_drop ALL, read-only root, tmpfs). DinD
   needs Docker daemon access from inside the sandbox. Two shapes:
   - **DinD inside sandbox**: sandbox runs its own dockerd in a
     nested container. Slower; properly isolated.
   - **DooD via Docker socket**: sandbox shares host's Docker via
     /var/run/docker.sock mount. Faster; weaker isolation; the
     verification-runner pattern uses this for act.

   Lean: **DinD for sandbox** (per-test isolation), **DooD for
   verification-runner** (single trust boundary at runner level
   already). Two trust boundaries; clear separation.

3. **Event subject namespace.** semspec uses
   `workflow.events.qa.*`. Options for SemTeams:
   - Align: use the same subjects (requires upstream coordination
     or shared cluster).
   - Per-product prefix: `semteams.workflow.events.qa.*` →
     `semteams.verification.events.run.*` (rename + namespace).
   - Upstream the events to semstreams with a `domain` field on
     the envelope.

   Lean: upstream when we upstream the runner itself; until then,
   per-product namespace prefix to avoid cluster collisions.

4. **Discovery tool vs persona-only for brownfield**. Lean
   persona-only initially; revisit if real adoption shows the
   bash-walk is friction.

5. **`act` image footprint.** ~1GB pulled. Acceptable for dev
   and smoke; production deployments may want a smaller variant.
   Operator concern, not framework.

## What this ADR explicitly does NOT decide

- The exact event subject namespace (open question 3)
- The exact verification-runner port path (open question 1)
- k8s backend implementation
- Multi-tenancy / per-tenant runner pools
- Secrets management
- Workflow YAML template content (deferred to resumed R3.7.2 line)
- Whether to upstream verification-runner to semstreams (likely
  yes, but timing depends on upstream coordination)
- Brownfield discovery tool shape (lean persona-only for now)

## Relationship to prior ADRs

- **ADR-031** (research-flow + dev-via-spec): the chain shape
  upstream of verification. Architect emits artifact; this ADR
  reshapes what comes after.
- **ADR-032** (R3.6 sandbox): the sandbox stays as the bash
  code-execution surface for the builder AND becomes the primary
  verification execution surface (via project-native test
  invocation + Testcontainers DinD). verification-runner is a
  separate trust boundary for the genuine e2e/browser-flow case.
- **ADR-033** (harness-anchored verification + coordinator
  authority): the §1 harness catalog primitive survives in
  modified shape. The §2 smoke-contract-execution design is
  largely replaced — for greenfield browser-flow it's a workflow
  YAML; for everything else it's the project's native test runner.

## References

- semspec qa-runner: `/cmd/qa-runner/{main,runner,qa_subscriber}.go`
  (semspec repo)
- semspec ADR-031 (qa-test-execution):
  `/docs/adr/ADR-031-qa-test-execution.md` (semspec repo)
- Event types: `semspec/workflow/subjects.go` —
  `QARequestedEvent`, `QACompletedEvent`, `QAFailure`,
  `QAArtifactRef`, `QAVerdictEvent`
- act (GitHub Actions emulator): https://github.com/nektos/act
- ARC (Actions Runner Controller for k8s):
  https://github.com/actions/actions-runner-controller
- Testcontainers (multi-language): https://testcontainers.com/
- ADR-033 §addendum 2026-05-04 (R3.7.1 framework-alignment review):
  upstream-survey precedent.

## Addendum 2026-05-04 #2 — Sandbox DinD vs DooD: DooD-first

Resolves Open Question 2 ("Sandbox DinD trust-boundary
reconciliation") with a phased answer.

### Decision

**Ship DooD (Docker-out-of-Docker) first as `--docker-mode=dood`,
add DinD (`--docker-mode=dind`) as a follow-on slice when
multi-tenant deployments materialize. Default mode follows the
deployment's threat model.**

- `--docker-mode=none` (current state): sandbox has no Docker
  access. Existing R3.6 builder bash work is unaffected.
- `--docker-mode=dood` (R3.7.2.d′ first): sandbox mounts host
  `/var/run/docker.sock`. Tests use Testcontainers libraries which
  spawn sibling containers on the host's Docker daemon. Cleanup
  via `--rm` discipline + Testcontainers' Ryuk reaper. Hardening
  (cap_drop ALL, read_only root, tmpfs) preserved; the socket
  mount is the ONLY relaxation.
- `--docker-mode=dind` (R3.7.2.d′-dind follow-on): sandbox runs
  its own dockerd nested. Requires `--privileged` or sysbox runc.
  Per-sandbox-instance isolation; multi-tenant-safe. Slower boot
  (~30s for nested daemon).

Default in compose stack: **dood for dev/smoke; dind in any
deployment running multiple chains concurrently per sandbox
service.**

### Why DooD first

1. **Smoke #7 is single-tenant dev work.** OSH-Meshtastic smoke
   target runs on Coby's laptop / a dev cluster. DooD's weaker
   isolation isn't a real threat in that context.
2. **DooD is cheap to ship.** Mount the socket, set the env var,
   test that Testcontainers boots a known image. ~1-2 days vs
   3-5 for DinD.
3. **DinD complexity isn't bleeding-edge but isn't free.** Need
   privileged containers (or sysbox), nested dockerd lifecycle
   (boot wait + cleanup), storage management for the nested
   daemon's containers. Deferred until a real driver requires it.
4. **The decision is reversible by config.** Both modes coexist;
   operators choose at deploy time. No wire-format lock-in.

### Trust boundary delta

DooD weakens the sandbox's isolation in one specific way: tests
running inside the sandbox can spawn arbitrary containers on the
host's Docker daemon, including privileged ones. For a single-
tenant dev environment, this is equivalent to running tests
locally — same trust boundary as `mvn verify` on a developer
laptop. For a multi-tenant production deployment, this would let
tenant A spawn containers visible to tenant B, which is
unacceptable. DinD or sysbox-runc closes that gap.

**Mitigation today**: deployment-level network policies + the
existing sandbox cap_drop ALL hardening still apply to the
sandbox process itself. The socket mount only widens what the
process can do via Docker API; the process's own kernel-level
capabilities remain dropped.

### Implementation summary for R3.7.2.d′

The slice ships:
- `SANDBOX_DOCKER_MODE` env var with values `none|dood`
- Compose stack updates: socket mount + env passthrough,
  optionally remove `read_only: true` only when mode != none
- Smoke test from inside sandbox: testcontainers-go boots
  `nats:2.10-alpine`, opens TCP, asserts subject responds
- Existing R3.6 bash builder work unaffected (default stays
  `none` until persona work in R3.7.2.h′ ships testcontainer
  flow)

DinD support is a separate slice (R3.7.2.d′-dind) that adds:
- `SANDBOX_DOCKER_MODE=dind` value
- Sandbox image grows dockerd
- `privileged: true` (or sysbox-runc runtime selection)
- dockerd lifecycle in sandbox container entrypoint
- Storage volume for nested daemon

### Re-renders one open question as resolved

Open Question 2 from the original ADR (DinD vs DooD trust-
boundary reconciliation): **resolved by addendum.** Answer is
"both, but DooD first; DinD follows when multi-tenant deployments
land." The other open questions remain.

## Revision history

- 2026-05-04 — initial proposal: full pivot to qa-runner pattern
  for all verification execution.
- 2026-05-04 (same day) — revised after team review:
  - Added §"Verification class → execution mapping" sharpening
    when sandbox vs verification-runner is the right executor.
  - Added §"Brownfield support" treating projects with existing
    CI / conventions as the common case.
  - Renamed our component qa-runner → verification-runner (semspec
    name reflects their qa role; we have no qa role).
  - Narrowed verification-runner scope to greenfield browser-flow
    and genuine multi-component e2e; sandbox + Testcontainers
    handles unit/integration via project-native invocation.
- 2026-05-04 §addendum #2 — DooD-first decision for sandbox
  Docker mode. DinD follows when multi-tenant deployments
  materialize. Resolves Open Question 2.
