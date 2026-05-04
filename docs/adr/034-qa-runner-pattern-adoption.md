# ADR-034: Verification execution via qa-runner pattern

## Status

Proposed — 2026-05-04. Pivots R3.7.2 line away from bespoke
family/runtime/template orchestration toward the qa-runner pattern
already extant in semspec. R3.7.2.a/b/c remain valid; R3.7.2.d
(closed in PR #59) and the planned R3.7.2.e/f/g/h/i are reshaped.

## Context

R3.7.2's working design was a (family × runtime) matrix:

- **Protocol family** (sidecar-side, language-agnostic): defines wire-
  level contracts, ships per-family JSON-shape validators.
- **Test runtime** (driver-side, per-language): owns invocation
  command (`mvn verify`, `go test`, `npx playwright test`) and
  per-family rendering templates that turn smoke contracts into
  real test code.
- **Harness catalog**: operator-curated `configs/harnesses.json`
  with `compose_profile` field referencing docker-compose services.

R3.7.2.d (PR #59, closed before merge) shipped the family + runtime
registries with one entry each (`tcp.binary-protobuf.v1`,
`java-junit-testcontainers`).

A pro-devops review surfaced that the `compose_profile` field bakes
the orchestrator into the wire format, that we have no GitOps or
multi-tenancy story, that we'd reinvented orchestration from scratch.
While walking through the gaps, surveying our sibling repo
**semspec** turned up a `qa-runner` component that solves nearly
all of them — and is the pattern a senior devops person would have
designed if they'd been on this from day one.

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

1. **Event-driven decoupling**. Chain publishes a job, runner
   consumes, chain consumes the result. Chain doesn't know how or
   where tests ran.
2. **`act` as execution engine** (GitHub Actions emulator). Local
   runs of `.github/workflows/qa.yml` against `catthehacker/ubuntu:
   act-latest` images. Universal CI vernacular — every devops
   person already knows the YAML shape.
3. **Workspace bind-mount** via Docker socket. Runner mounts the
   chain's workspace into act's container; tests see the
   implementation under test.
4. **Standardized result envelope**: `QAFailure` (job/step/test
   name + log excerpt), `QAArtifactRef` (workspace-relative path +
   type + purpose), duration, runner error distinguished from test
   failures.
5. **Mode-based routing**: `unit` → sandbox (process-local,
   cheap), `integration|full` → act (real-stack, sidecars
   declared in workflow `services:`).
6. **ARC-compatible swap path** for k8s. `act` is the local
   backend; a future k8s deployment swaps qa-runner's executor for
   GitHub Actions Runner Controller (or Tekton, or Argo Workflows)
   without changing the event shape.

### Honest assessment of qa-runner maturity

The qa-runner is **DESIGNED + BUILT but mostly UNUSED in semspec**.
Six weeks old (April 2026). A few smoke tests have run through it;
no production load. The pattern is right; the implementation is
fresh and likely needs SemTeams-specific refinement.

This ADR is NOT proposing we adopt a battle-tested production
component. It IS proposing we adopt a structurally-correct
pattern that already exists in our ecosystem, vs. building a
parallel bespoke solution.

## Decision

**Pivot R3.7.2's execution boundary to the qa-runner pattern.**

Specifically:

1. Verification execution moves from "builder runs `mvn verify`
   directly inside sandbox against operator-curated compose
   sidecar" to "builder publishes a `qa.requested`-shaped event
   with workspace + workflow path; qa-runner consumes; result
   event drives `decide`."

2. The verification commitment's `target` and `convention` fields
   point at a workflow YAML the architect/builder author
   (`workflow_path: .github/workflows/qa.yml` by convention).
   The workflow's `services:` block declares sidecars; its
   `runs-on:` declares the runner image; its `jobs.*.steps`
   declare the test invocation. **All in one file, in a format
   every devops person already reads.**

3. SemTeams ports qa-runner from semspec. Subtree-import vs
   reimplement is an open question (see §Open questions). Either
   way the event types (`QARequestedEvent`, `QACompletedEvent`,
   `QAVerdictEvent`) move into shared territory — likely
   upstreamed to semstreams as Pattern-B verification primitives.

4. The bespoke (family × runtime) matrix is **withdrawn**.
   `verification.Commitment` (R3.7.2.a, merged) and the artifact
   widening (R3.7.2.b, merged) remain valid. Upstream verifiability
   personas (R3.7.2.c, merged) remain valid. The harness catalog
   (R3.7.1, merged) survives in modified shape — declares
   "available services to reference in workflows" rather than
   "compose profiles to provision."

## Consequences

### What R3.7.2 work is preserved

- **R3.7.2.a `verification.Commitment` primitive (PR #56)** —
  unchanged. `target`/`approach`/`convention`/`evidence` are all
  still meaningful in a workflow-based world.
- **R3.7.2.b `dev_via_spec.artifact.VerificationCommitments[]`
  widening (PR #57)** — unchanged. The artifact still carries the
  architect's commitments.
- **R3.7.2.c upstream verifiability personas (PR #58)** —
  unchanged. Planner enumerates outcomes; reviewer gates;
  challenger probes; architect transcribes. The transcription
  target is now a workflow YAML scenario rather than a JUnit
  Given/When/Then — but the discipline is identical.
- **Harness catalog (R3.7.1, merged in PR #55)** — partially
  preserved. Catalog still declares "what's available to this
  deployment"; the `compose_profile` field becomes a `services_yaml`
  fragment or workflow-include reference.

### What changes / is replaced

- **R3.7.2.d (closed in PR #59)** — `families.Registry` /
  `runtimes.Registry` interfaces and registrations. Gone. The
  matrix is implicit in workflow YAML's `runs-on` + `services:`
  shape, which act handles natively.
- **R3.7.2.e (planned: first sidecar harness `meshtasticd-3.x`)**
   — reshaped. Becomes "first workflow YAML referencing a
   meshtasticd service." The catalog entry is the operator's
   declaration of "meshtasticd is available"; the workflow file
   is the chain's declaration of "I'm using it like this."
- **R3.7.2.f (planned: evidence-rule registry + architect
  contract)** — reshaped. Evidence rules become structural checks
  on the workflow YAML (e.g. `workflow_uses_services`,
  `workflow_runs_real_command`) rather than checks on Go/Java
  test source code. The architect contract still requires emitting
  commitments; the convention's `template_id` points at a workflow
  template instead of a JUnit template.
- **R3.7.2.g (planned: builder + sidecar TCP probe + mvn)** —
  replaced. Builder writes the workflow YAML and code; chain
  publishes `qa.requested`; qa-runner handles execution. No
  sandbox-level TCP probe needed because act manages service
  readiness via the workflow's `services.<n>.options.--health-cmd`
  directives.
- **R3.7.2.h (planned: evidence gate as separate rule)** —
  reshaped. The evidence gate consumes `qa.completed` events and
  walks the structured `Failures` / `Artifacts` envelope. Goodhart
  resistance is built in: the runner's verdict is mechanical
  (test pass/fail, real bytes), not LLM-judged.
- **R3.7.2.i (planned: reviewer reads evidence report)** —
  becomes the qa-reviewer pattern semspec already has. LLM role
  consumes `QACompletedEvent`, judges, publishes `QAVerdictEvent`.

### New work

- **Port qa-runner from semspec.** Standalone binary; needs
  SemTeams-specific event subjects (or shared with semspec; TBD).
- **Workspace bind-mount story for SemTeams sandbox.** Today our
  sandbox is hardened via cap_drop + read-only root + tmpfs. The
  qa-runner pattern needs the workspace path to be reachable from
  act's sibling-container Docker socket. Reconcile the security
  model.
- **Workflow YAML template registry.** Replaces the JUnit-template
  registry from R3.7.2.d. One template per language ecosystem
  (Java/Maven, Go, Node, Python). Templates are simpler than the
  JUnit class shape was — they're just `.yml` snippets the
  architect populates with smoke contract scenarios.
- **Compose stack updates**. Add qa-runner service alongside
  sandbox / mock-llm / nats / backend. `act` toolchain in the
  qa-runner image.
- **Evidence-rule kinds for workflow shape.** Examples: 
  `workflow_has_services`, `workflow_step_invokes_real_command`,
  `workflow_artifacts_uploaded`. These check the workflow YAML
  before submission, in the same vein as R3.7.2.e's evidence
  registry but operating on a different artifact type.

### What's deferred (NOT decided in this ADR)

- **k8s backend**. The pattern supports it (swap qa-runner's
  executor for ARC); when we actually need it, a follow-on ADR
  ships the swap. Not now.
- **Multi-tenancy isolation specifics**. Multiple runners in
  multiple namespaces is the obvious shape; per-tenant
  authentication, secrets, network policy — separate ADR when
  we have a tenant.
- **Secrets primitives**. GitHub Actions `secrets:` syntax exists;
  what backs it (Vault, k8s Secrets, env vars) is operator-policy.
  Catalog gains a `secret_refs` field in a future slice.
- **Subtree-import vs reimplement**. Ports semspec's qa-runner to
  semteams; design choice deferred to first slice of the resumed
  R3.7.2 line.
- **Event subject namespace.** Whether `workflow.events.qa.*` is
  shared between semspec and semteams or per-product is an
  upstream-coordination question.

## Cost / benefit

### Cost

- **2-3 weeks scope shift on R3.7.2.** R3.7.2.d is gone. R3.7.2.e
  through .i are reshaped. Smoke #7 timeline pushes out by a
  similar interval.
- **Net-new code to port + adapt.** semspec qa-runner is ~600 LOC
  including tests; porting + adapting to SemTeams's NATS surface
  + sandbox model is non-trivial.
- **Coordinate event subject shape with semspec**. Either
  upstream the events to semstreams (best long-term) or
  duplicate with namespace prefix (faster). Open question.

### Benefit

- **Devops vernacular**. `.github/workflows/qa.yml` is the most
  widely-recognized CI format on the planet. Onboarding any
  devops engineer drops from "explain custom orchestration" to
  "you know GitHub Actions, right?"
- **k8s migration path is real**. ARC + GitHub Actions Runner
  Controller is a one-line config change to qa-runner; the chain
  remains agnostic. Today's bespoke-orchestration design has no
  k8s story.
- **Reuse vs reinvent**. semspec already sunk the design effort
  on event shapes and runner protocol. We get to apply that
  thinking instead of redoing it.
- **Multi-language by design**. GitHub Actions YAML supports
  every language ecosystem with the same shape. Java, Go, Node,
  Python, Rust — all `runs-on:` + `steps:` invocations. Our
  bespoke (family × runtime) matrix would have needed per-language
  template work for each new ecosystem.
- **Goodhart resistance is mechanical**. qa-runner returns the
  raw act exit code + structured failure parsing. The chain
  cannot fake a test pass; act either ran the workflow or didn't.
- **Multi-tenancy is achievable** without a major redesign.
  Multiple qa-runner instances in multiple namespaces.

## Open questions

1. **Subtree-import vs reimplement vs upstream.** Three
   options:
   - **Subtree-import** semspec's `cmd/qa-runner/` directly into
     semteams. Cheap; couples our runner to semspec's event
     namespace.
   - **Reimplement** the runner in semteams from semspec's
     design. Independent codebases; more freedom; risk of drift.
   - **Upstream to semstreams** as a shared component
     (`semstreams/processor/qa-runner` or similar). Best long-
     term; requires upstream coordination + ADR there.

   Lean: upstream. The pattern is generic, both products will use
   it, framework is the right home. But timeline depends on
   upstream's R3.7 priorities.

2. **Workspace bind-mount + sandbox security model.** Today's
   sandbox container is hardened (cap_drop, read-only, tmpfs).
   qa-runner needs Docker socket access to spawn `act`'s sibling
   container. Two possible shapes:
   - **qa-runner is its own container, NOT sandbox-hardened.** Has
     Docker socket access; runs act; mounts workspace from the
     sandbox's volume. Sandbox stays hardened for builder bash;
     qa-runner is a separate trust boundary.
   - **Sandbox runs act directly.** Higher-trust sandbox. Worse
     security posture but fewer moving parts.

   Lean: separate container with separate trust boundary. The
   sandbox's cap_drop ALL is load-bearing for the bash code-
   execution surface; qa-runner doesn't need to share it.

3. **Event subject collision with semspec.** semspec uses
   `workflow.events.qa.*`. Two products on the same NATS cluster
   would collide. Options:
   - Per-product namespace prefix: `semteams.workflow.events.qa.*`
   - Per-deployment cluster (already common pattern).
   - Upstream the events to semstreams with a per-product `domain`
     field on the envelope.

   Lean: upstream with `domain` field once we upstream the runner
   itself.

4. **`act` image footprint**. `catthehacker/ubuntu:act-latest`
   is ~1GB pulled. SemTeams's compose stack already pulls the
   sandbox toolchain image (Java 21 + Maven + Gradle + Go + Node
   + Python). Adding act doubles the disk footprint. Acceptable
   for dev and smoke testing; production deployments may want a
   smaller act image variant.

## What this ADR explicitly does NOT decide

- The exact event subject namespace
- The exact qa-runner port path (subtree / reimplement / upstream)
- k8s backend implementation
- Multi-tenancy / per-tenant runner pools
- Secrets management
- Workflow YAML template content (deferred to resumed R3.7.2 line)
- Whether to upstream qa-runner to semstreams (likely yes, but
  timing depends on upstream coordination)

## Relationship to prior ADRs

- **ADR-031** (research-flow + dev-via-spec): the chain shape
  upstream of verification. Architect emits artifact; this ADR
  reshapes what comes after.
- **ADR-032** (R3.6 sandbox): the sandbox stays as the bash
  code-execution surface for the builder. This ADR adds qa-runner
  as a separate trust boundary for verification execution.
- **ADR-033** (harness-anchored verification + coordinator
  authority): the §1 harness catalog primitive survives in
  modified shape. The §2 smoke-contract-execution design is
  largely replaced by the qa-runner pattern (workflow YAML IS
  the smoke contract, mechanically).

## References

- semspec qa-runner: `/cmd/qa-runner/{main,runner,qa_subscriber}.go`
  (semspec repo)
- semspec ADR-031 (qa-test-execution): `/docs/adr/ADR-031-qa-test-execution.md`
  (semspec repo)
- Event types: `semspec/workflow/subjects.go` —
  `QARequestedEvent`, `QACompletedEvent`, `QAFailure`,
  `QAArtifactRef`, `QAVerdictEvent`
- act (GitHub Actions emulator): https://github.com/nektos/act
- ARC (Actions Runner Controller for k8s):
  https://github.com/actions/actions-runner-controller
- ADR-033 §addendum 2026-05-04 (R3.7.1 framework-alignment review):
  upstream-survey precedent; this ADR follows the same
  before/decision/migration structure.
