# Product-shell tools

This directory holds the product-shell-local tool executors registered
on the framework's tool registry from `cmd/semteams/main.go` (after
`executors.RegisterBuiltins`). Tools here are SemTeams-specific —
they're not in upstream `semstreams/processor/agentic-tools/executors/`
because their behaviour, config, or scope is product policy.

## What lives here

| Tool | Package | Purpose | ADR | Migration target |
|---|---|---|---|---|
| `add_source_repo` | [`addsource/`](addsource/) | Bridges the agent loop to SemSource's `graph.ingest.add.{namespace}` contract. Approval-gated via `agentic-tools.approval_required`. | [ADR-031 §R2](../../../docs/adr/031-research-flow-and-semspec-handoff.md) | Stays product-local. The namespace allowlist is per-deployment policy; SemSource is a sibling product, not framework. |
| `emit_research_artifact` | [`emitartifact/`](emitartifact/) | Writes marker triples + publishes typed `research.artifact.v1` payload on a stable subject. Researcher persona calls it before `submit_work`. | [ADR-031 §R3.2 + addendum 2026-04-30](../../../docs/adr/031-research-flow-and-semspec-handoff.md) | Re-evaluate when upstream ships the planned generic `write_artifact` tool ([ADR-028 §What's not built here](https://github.com/c360studio/semstreams/blob/main/docs/adr/028-orchestration-architecture.md)). Verified absent in semstreams beta.36. |
| `emit_plan` | [`emitplan/`](emitplan/) | Planner role's milestone tool. Renders `dev_via_spec.plan.v1` markdown to `docs/plans/<slug>.md`, mints chain-entity triples, publishes the typed payload. Called from `researcher-research-plan` before `decide(action="gather")`. | [ADR-038 PR C Phase C2](../../../docs/adr/038-research-arc-emissions.md) | Same migration target as `emit_research_artifact` — upstream's planned generic `write_artifact` suite. |
| `bash` (chain-scoped wrapper) | [`chainbash/`](chainbash/) | Wraps upstream `executors.BashExecutor` with a chain-id resolver. Rewrites `Metadata["task_id"]` to the chain root so every role in a chain shares one sandbox worktree (ADR-038 D1). Always-warm sandbox under ADR-042 MVP-7 follow-up (2026-05-19): the framework reads `SANDBOX_URL` once at boot, so the sandbox container is brought up unconditionally by the compose stack. | [ADR-041 Phase 4](../../../docs/adr/041-mvp-role-compression-and-graph-as-substrate.md) | Stays product-local until upstream ships a per-tool middleware hook. The wrap-then-delegate pattern is structurally bounded. |
| `request_sandbox` | [`requestsandbox/`](requestsandbox/) | Coordinator's ADR-043 Layer 1 entry point. Accepts typed `SandboxRequirements` (languages, tools, services, network, secrets, mounts, privileges, verification probes) and delegates to `sandboxmanager.Manager.Request`: profile-match → admission → `devcontainer up` → probes → attest → stamp `sandbox.attestation.*` triples on the chain entity. Returns the Attestation synchronously; Coordinator routes on the attestation shape (`ready`/`degraded`/`terminal`/`admission_outcome`). | [ADR-043](../../../docs/adr/043-devcontainer-as-sandbox-spec.md) | Stays product-local. The three-layer model + canonical-profile catalog are SemTeams policy; upstream has no equivalent (verified semstreams beta.86, 2026-05-29). |
| `query_sandbox_attestation` | [`querysandboxattestation/`](querysandboxattestation/) | Read-only: looks up whether a fresh `sandbox.attestation.*` triple set on the chain entity already covers the requested `SandboxRequirements`. Returns `{found, fresh, staleness_seconds, attestation?}`. Coordinator calls this BEFORE `request_sandbox` to skip re-attesting when the same chain has a recent attestation for the same `(profile, requirements_hash)` signature. | [ADR-043](../../../docs/adr/043-devcontainer-as-sandbox-spec.md) | Stays product-local alongside `request_sandbox`. |
| `emit_dev_via_test_plan` | [`emitdevviatestplan/`](emitdevviatestplan/) | Lisa's terminal commit in the dev-via-test category arc. Validates a Karpathy-shaped plan (assumptions, non_goals, target_files≥1, test_command required at plan + per-task level) and stamps `plan.*` triples on the run entity (coordinator's loop entity) for downstream Ralph + CBG consumption. The schema rejects payloads missing the Karpathy-required fields — discipline lives structurally, not in persona prose. | [ADR-044](../../../docs/adr/044-dev-via-test-pack.md) | Same migration target as `emit_research_artifact` / `emit_plan` / `emit_autoresearch_*` — upstream's planned generic `write_artifact` suite ([ADR-028 §What's not built here](https://github.com/c360studio/semstreams/blob/main/docs/adr/028-orchestration-architecture.md)). Verified absent in semstreams beta.96. |
| `emit_dev_via_test_measurement` | [`emitdevviatestmeasurement/`](emitdevviatestmeasurement/) | Ralph's per-iteration audit stamp inside the dev-via-test inner convergence loop. Stamps `dev_via_test.measurement.{pass,value,stdout_tail,stderr_tail,stamped_at}` on Ralph's loop entity. Rules 04a (converged) + 04b (failed) condition on these for coordinator wake-up routing. v1 binary semantics: pass=true ⇒ terminal converged; no kept/reverted machinery (unlike `emit_autoresearch_measurement`). Deferred fractional convergence with empirical-reviewer logic to v2 when richer test results surface a need. | [ADR-044](../../../docs/adr/044-dev-via-test-pack.md) §addendum 2026-06-03 Slice 2 | Same migration target. If v2 needs fractional convergence with kept/reverted, evaluate consolidating with `emit_autoresearch_measurement` at that point. |
| `emit_change` | [`emitchange/`](emitchange/) | The create_change journey author's terminal commit (ADR-057 §D5). Parses a prompt + ingested living-spec context into a structured OpenSpec change and stamps `change.<slug>.*` triples on the run entity for the reviewer gate + the P4 dev-from-task hand-off. Reuses the pure `openspec.Change.Facts()` for markdown-representable content (proposal/design/delta/thin tasks) and ADDS the writer-only graph-state the model does not carry — `status`, `acceptance_command`, `generated_at`, and the §D6 execution-rich per-task fields (goal + target_files≥1 + test_command + assumptions + non_goals), index-aligned with the content half. The schema enforces ADR-057's §D3 (every requirement carries an RFC-2119 SHALL + ≥1 Given/When/Then scenario) and §D6 discipline structurally, not in persona prose. | [ADR-057](../../../docs/adr/057-openspec-graph-spec-model-and-create-change.md) | Same migration target as `emit_research_artifact` / `emit_plan` / `emit_dev_via_test_*` — upstream's planned generic `write_artifact` suite ([ADR-028 §What's not built here](https://github.com/c360studio/semstreams/blob/main/docs/adr/028-orchestration-architecture.md)). Verified absent in semstreams beta.114. The pure format layer is extraction-ready (`pkg/openspec/` target) on a 2nd consumer — see ADR-057 §Framework-alignment 5a. |

## Discipline: framework-alignment review before adding a tool here

The semspec lesson — and the load-bearing claim of ADR-031's R3.2.1
framework-alignment addendum — is that **products become bespoke by
accretion**. Each tool added here is individually defensible. The
risk is the cumulative drift away from framework idiom.

Before adding a new tool to this directory:

1. **Survey upstream.** Does
   `~/go/pkg/mod/github.com/c360studio/semstreams@<current>` already
   ship a tool that does this, or a near-equivalent pattern (`decide`,
   `emit_diagnosis`, `read_loop_result`, `read_entity`,
   `query_entities`, `submit_work`)? If yes: use it. If "near": port
   to it; do not fork.

2. **Check the upstream roadmap.** Read the latest `ROADMAP.md` and
   the `What's not built here` sections of the most recent ADRs
   (especially [ADR-028](https://github.com/c360studio/semstreams/blob/main/docs/adr/028-orchestration-architecture.md)
   for agentic infrastructure). If the pattern is *planned* but not
   shipped, you may land a domain-specific instance here — but
   document the migration target explicitly in this README.

3. **Document the alignment review** in the relevant ADR. The
   evidence trail is what protects future agents from re-litigating
   the decision in a vacuum. ADR-031's addendum 2026-04-30
   ("Framework-alignment review for R3.2 emission shape") is the
   working template: documents the survey, lists the alternatives
   considered and ruled out with upstream evidence for each
   rejection, and pins the migration posture.

4. **Pattern-match against the canonical shapes.** If your tool:
   - Emits structured terminal output → mirror `decide` and
     `emit_diagnosis`. Don't invent a new emission shape.
   - Calls an external service → mirror `add_source_repo`'s
     request/reply via `RequestWithRetry`.
   - Reads from the graph → use `query_entities` / `read_entity`;
     don't re-implement KV traversal.

If you cannot point at an upstream pattern your design implements
(or a planned one in the roadmap), **that is a stop signal.** Either
the design is wrong, or the framework is missing a primitive that
should be raised upstream rather than worked around here.

## Anti-patterns the codebase has explicitly rejected

These have come up in design conversations and been rejected with
recorded reasoning. Future agents tempted to re-introduce them
should read the cited reasoning before re-opening the question.

- **Per-loop KV bucket for artifact state** (semspec's plan-manager
  shape). Rejected at ADR-031 §addendum 2026-04-30 (R3.1):
  "SemSpec's per-Plan bespoke KV-bucket ownership is exactly the
  friction SemTeams was architected to avoid." Use the existing
  `payloadregistry` + `graph.ingest` + `rule` primitives.
- **Rule action that parses agent output to synthesise typed
  payloads.** Rejected at ADR-031 §addendum 2026-04-30 (R3.2.1
  framework-alignment review): rules don't quality-judge
  unstructured content (ADR-028 §Layer 2); rule action object
  substitution is string-only (cannot compose nested data).
- **Cross-product runtime contract for `ResearchArtifact`.**
  Rejected at ADR-031 (the original ADR) for versioning cost,
  velocity coupling, and boundary-placement reasons. The artifact
  is SemTeams-local by intent.
- **Stuffing free-text content into rule-substituted predicates.**
  Rejected at upstream ADR-028 §Layer 2 (referenced by our
  ADR-031 §addendum). Truncation risk + small-context-window
  context explosion. Rules carry references; agents fetch
  content via `read_loop_result` / `read_entity`.

## When upstream ships a generalising primitive

The framework's roadmap explicitly anticipates an artifact-tool
suite (ADR-028 §What's not built here). When `write_artifact` /
`read_artifact` / `list_artifacts` ship in upstream's
`processor/agentic-tools/`:

1. Open a tracking issue / ADR addendum in this repo evaluating
   migration of `emit_research_artifact` onto the generic primitive.
2. The migration is replacing a concrete executor with a configured
   one; the tool name + triple/payload contract stay in product code
   if the artifact-shape semantics are research-domain-specific.
3. If the generic primitive subsumes our shape entirely: delete
   `emitartifact/`, register the upstream tool from
   `product_tools.go`, update the persona fragments. Keep the ADR
   addendum as the historical record of why we shipped the
   product-local version first.

This is the commission-not-omission posture: we shipped the
domain-specific tool because the generic one wasn't ready, and we
explicitly contract to migrate when it is.
