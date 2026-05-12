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
| `emit_dev_via_spec_artifact` | [`emitspecartifact/`](emitspecartifact/) | Architect role's terminal action in the dev-via-spec arc. Renders `dev_via_spec.artifact.v1` as markdown to `docs/specs/<slug>.md` (env-overrideable), mints marker triples on the calling loop, publishes the typed payload on `dev_via_spec.artifact.{loop_id}`. | [ADR-031 §R3.3](../../../docs/adr/031-research-flow-and-semspec-handoff.md) | Same migration target as `emit_research_artifact`: upstream's planned `write_artifact` suite (ADR-028 §What's not built here). Verified absent in semstreams beta.36. |
| `builder_decide` | [`builderdecide/`](builderdecide/) | dev-via-spec-builder role's terminal validator. Sibling to upstream `decide` — same wire shape (emits `coordinator.next_action` + `coordinator.decision_reason` triples) plus per-action evidence triples (`tests_run`, `tests_passed`, `tests_failed`, `artifact_summary` / `failure_summary` / `retry_hint` / `blocking_question`) enforced at the executor boundary. Closed action enum: `tests_passing` \| `tests_failing` \| `needs_clarification`. **Note**: unlike the `emit_*` siblings, `builder_decide` does NOT publish a typed payload — full args round-trip via the tool result Content for `read_loop_result` consumers. The terminal is decision metadata, not a content artifact. | [ADR-032 §15](../../../docs/adr/032-r36-sandbox-design.md) | Stays product-local until upstream ships either (a) a per-role terminal-tool primitive, or (b) a `decide` extension hook that lets product code register validators against existing tool names. Neither shipped in semstreams beta.39. The framework's wrapping pattern (see `processor/agentic-tools/wrapping_pattern_test.go`) is for transforming results, not adding distinct evidence schemas to canonical tools. |
| `bootstrap_workspace` | [`bootstrapworkspace/`](bootstrapworkspace/) | dev-via-spec-builder role's iteration-1 setup hook. Reads the rendered spec markdown from the host filesystem (path supplied via the rule's `$entity.triple.dev_via_spec.artifact.path` substitution), creates the sandbox worktree at the builder's loop_id via upstream `sandbox.Client`, and PUTs `SPEC.md` into the workspace root. Path traversal rejected; spec_path must resolve under `SEMTEAMS_DEVVIASPEC_ARTIFACT_DIR` (default `docs/specs`). Idempotent on retry (worktree create returns "exists"; PUT overwrites). Skipped at boot when `SANDBOX_URL` is unset — the entire builder slice is non-functional without a sandbox. | [ADR-032 §addendum 2026-05-03 R3.6.2.d](../../../docs/adr/032-r36-sandbox-design.md) | Stays product-local until upstream ships an `http_request` rule action **plus** a substitution variable for `publish_agent`'s generated task_id within the same on_enter sequence. Neither shipped in beta.39. Together they would let the rule POST `/worktree` + PUT `/file` inline, then publish_agent the builder; this tool can then be deleted. Until then, the chicken-and-egg between rule action timing and publish_agent's internal task_id generation makes a product-shell tool the cheaper structural fit (per the package doc's framework-alignment review). |

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
