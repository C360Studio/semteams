# ADR-042: Coordinator-instantiated flows via template inventory

## Status

**Proposed (2026-05-17).** Reframes the next phase of harness
work after the deep-research → dev-research rename (PR #166) and the
2026-05-17 harness extensibility audit surfaced that **the framework
already ships a flow-template substrate that semteams has never
populated**. Chain-mode machinery (`chainmode.actionToMode`,
`phasevalidator.allowedEdges`, `chainstall.detectRejection`) is the
hardcoded workaround for the empty template inventory.

> **2026-05-17 §Phase 2 redesign — see the addendum at the end of this ADR.**
> After Phase 1/4/2a shipped (PRs #168/#169/#170), a deeper
> re-investigation revealed the template-driven-flow framing was
> the wrong unit for MVP. The corrected design uses
> **category-keyed rule packs + named persona bundles** running
> through the framework's existing singleton substrate — no
> runtime flow construction needed for MVP. The Phase 1/4/2a
> work stays as **post-MVP scaffolding** for runtime category
> creation. The original Decision and Phasing sections below
> reflect pre-redesign thinking; the addendum is the active
> direction. Read the addendum first.

**Break/fix migration, not additive.** Greenfield posture: we
build the template-driven path, validate it end-to-end, then
delete the chain-mode machinery + concrete configs in the same
migration PR. No vestigial dual-path. Context pollution from
running two paths concurrently is the failure mode this ADR
explicitly avoids.

This ADR keeps [[ADR-029]] (product-shell wiring), [[ADR-038]]
(chain entity), [[ADR-030]] (approval seam), [[ADR-027]] (ops
agent), and [[ADR-041]] (4-role MVP roster) intact — chain
entity stays as lineage substrate; the 4-role roster stays as
the chain participants; approval and ops surfaces are
flow-shape-agnostic.

This ADR **supersedes**:

- The chain-mode invention from coordinator Slice 1b/1c (PRs
  #157, #158, #159, #162) — `chainmode/`, `phasevalidator/`,
  `chainstall/` retire entirely in the migration PR.
- [[ADR-037]] (chain failure handling) — the chainstall recovery
  primitive retires; recovery becomes flow-deploy-level under
  templates. Failure-handling lessons preserved in §"What we
  learned from chain machinery" below.
- [[ADR-039]] (needs-clarification recovery) — same retirement;
  the LLM-side `needs_clarification` terminal stays, but the
  chain-level recovery rule retires.
- Concrete chain configs (`configs/dev-research.json`,
  `configs/osh-demo.json`, `configs/agentic.json`,
  `configs/agentic-claude.json`, `configs/onboarding.json`, all
  `configs/e2e-*.json`) — all replaced by templates +
  bootstrap config.

## Why this exists

### The framework ships a template system that nothing uses

Upstream `semstreams@v1.0.0-beta.72` ships:

- `flowtemplate.Manager` — KV-backed Pattern-B CRUD per ADR-029 step 4.
  Stores parameterized flow definitions. `Template.Instantiate(params)`
  renders the body (text/template substitution) into a concrete
  `flowstore.Flow`.
- Six flow-template tools registered by
  `processor/agentic-tools/executors/flow_templates.go` and gated by
  `RegisterBuiltins(deps.FlowTemplateManager != nil)`:
  - `create_flow_template`, `update_flow_template`, `delete_flow_template`
  - `get_flow_template`, `list_flow_templates`
  - `instantiate_flow_template` (renders, does NOT persist)
- Five flow-CRUD tools (`processor/agentic-tools/executors/flows.go`):
  `create_flow`, `update_flow`, `delete_flow`, `get_flow`, `list_flows`.

Semteams **does** wire the manager
(`cmd/semteams/main.go:238 — FlowTemplateManager: buildFlowTemplateManager(...)`).
The tools auto-register because `RegisterBuiltins` is called with the
non-nil manager. But:

1. **Zero templates are seeded.** `grep -rn 'flow_template' configs/`
   returns nothing. The KV bucket boots empty every time. There is no
   directory loader analogous to `persona.LoadFromDirectory`.
2. **No coordinator persona uses these tools.** Every chain config's
   `allowed_tools` list omits the flow-template + flow-CRUD tools.
   The coordinator works in a `decide(action=delegate_research |
   delegate_dev_chain | ...)` taxonomy that bottoms out at
   `chainmode.actionToMode` (a closed Go map; see
   [[project_coordinator_slice1b_pickup]]).

The substrate exists. The inventory is empty.

### Chain machinery is the workaround for the empty inventory

The 2026-05-17 audit (this session) classified the harness's
task-type extensibility at three touchpoints:

| Touchpoint | Status | What it would be if templates were populated |
|---|---|---|
| `chainmode.actionToMode` (Go map) | hardcoded | Coordinator calls `list_flow_templates`, selects one, calls `instantiate_flow_template` + `create_flow`. The selected template *is* the mode. |
| `phasevalidator.allowedEdges` (Go map keyed by mode) | hardcoded | The instantiated flow's `Nodes` + `Connections` *is* the allowed-edges graph. Validator derives from the running flow. |
| `chainstall.detectRejection` (per-mode rejection routing) | hardcoded | Rejection routing is template-attached metadata (or rule-table per template). |

Each piece of chain.mode hardcoding emerged because **the coordinator
had no way to construct a flow at runtime**. The closed-set of modes
became a stand-in for what should have been "the coordinator picked a
flow from inventory."

### The audit-of-personas + the rename PR are symptoms of the same gap

The 2026-05-17 trilogy of PRs (#163 OSH cleanup, #164 harness reframe,
#165 domain audit) and the dev-research rename (#166) all addressed
the same underlying issue: **the shared persona corpus accreted
domain flavor because chain-shape was hardcoded outside it**. With
template-driven flows, each template carries its own persona-overlay
choice (via the flow body that loads specific fragment dirs), and the
shared corpus naturally stays harness-level.

### Empirical baseline

- Coordinator Slice 1b smoke (2026-05-16, real-LLM, ~$0.20):
  classification GREEN. Synthesize-drift-to-architect detected by
  phasevalidator with `chain.stall.reason = mode_mismatch`. Chain
  wedged fail-safe.
- Finding 1 smoke (PR #162, 2026-05-16, real-LLM, ~$0.50): mode-aware
  synthesize allowlist GREEN. Surfaced osh-demo reviewer
  domain-lock — resolved in #163-#165 via persona-corpus cleanup.
- These smokes prove the **hardcoded** chain-mode machinery works.
  They do not exercise the template-driven path because there is no
  template-driven path to exercise.

## Decision

### High-level: templates are the only unit of prompt-class definition

A prompt class is a **flow template** under
`configs/flow-templates/<name>.json`. Concrete chain configs go
away. The harness boots from a minimal bootstrap config that wires
just the substrate (NATS, agentic-dispatch, agentic-loop,
agentic-tools, agentic-memory, persona-loader, rule-engine,
template-loader); prompt-class flows are instantiated by the
coordinator at runtime from the template inventory.

The coordinator's job is:

1. Read user request.
2. `list_flow_templates` — see what's in inventory.
3. `get_flow_template(id=<picked>)` — read parameters and required
   fields.
4. `decide(action=instantiate, template_id=<id>, parameters={...})` —
   terminal action.
5. Rule fires on the decision: `instantiate_flow_template` then
   `create_flow`. The new flow is persisted at `not_deployed`.
6. Phase 4 closure (open question) routes the user's task to the
   newly-created flow.

There is no parallel "concrete config" path. The chain.mode/
phasevalidator/chainstall machinery is deleted in the migration PR
once the template path is validated. Operators who want to inspect
"what the harness can do today" run `list_flow_templates`, not
`ls configs/`.

### Templates parameterize chain shape, not just data

A template's body is a full flow JSON: `components`, `streams`,
`platform`, `rules_files`, persona-loader configs. Parameters can
substitute anywhere a `{{.Name}}` placeholder is valid in text/template.

Concrete examples:

- `research-pipeline` template: parameters substitute `topic`,
  `max_iterations`, `model`, `tools_allowlist`. The rule-files list and
  persona-fragment dirs are *fixed* in the template body (researcher
  + reviewer + their rule packs).
- `dev-via-spec-pipeline` template: parameters substitute
  `target_framework`, `sandbox_image`, plus the researcher /
  reviewer / builder knobs.
- `web-research` template: parameters substitute `topic`,
  `max_iterations`. Rule-files load a `configs/rules/web-research/`
  pack and persona-fragment dirs include
  `configs/personas/fragments/web-research/`.
- `decision-memo` template (future): same structure with different
  rule pack and persona overlay.

This is the **chain shape encoded in JSON** — the rule pack + persona
overlay + tool allowlist together define what the chain does. The
template is the runtime-instantiable version of what concrete
`configs/*.json` files do today.

### What the coordinator instantiates is a flow, not a chain

A "chain" is the runtime concept: a sequence of loops linked by
`agent.loop.parent` lineage, rolling up into a chain entity (ADR-038).
The chain machinery's role is **lineage and recovery**, not shape
definition.

The **flow** defines:
- Which components run (`agentic-dispatch`, `agentic-loop`, etc.)
- Which rules fire (rule pack)
- Which persona fragments are loaded
- Which tools are allowed at each role

The **template** is a parameterized flow.

The **chain** is what the flow produces at runtime (multi-loop work
with lineage).

This separation means the chain-entity (ADR-038) keeps its full value
under templates — every template-instantiated flow that produces
multi-loop work still rolls up into a chain entity. Lineage doesn't
care whether the flow was authored as a concrete config or
instantiated from a template.

### Open question (load-bearing): the deploy gap

The flow CRUD tools persist a flow at `RuntimeState = not_deployed`.
**Lifecycle (Deploy / Start / Stop / Undeploy) is exposed only via
HTTP (`/flowbuilder/flows/{id}/deploy` etc.) — NOT as agent tools.**

`processor/agentic-tools/categories.go:65` references `deploy_flow`
as a meta-category but no executor registers under that name in
beta.72.

Three options to investigate in Phase 4:

1. **Add a `deploy_flow` agent tool.** Framework gap — upstream PR.
   Requires a Pattern-A `FlowEngineManager` dependency in
   `executors.ToolDependencies` plus a registration gate. Approval
   filter applies — deploys go through human review by default.
2. **Auto-deploy via rule on flow-created event.** Rule listens for
   `agent.tool.create_flow.result` (or KV watch on
   `FLOW_DEFINITIONS`) and calls `flowengine.Deploy(flowID)`
   internally. Product-shell rule action, not an agent-exposed tool.
3. **Single-flow process with template-as-config-rewrite.** Semteams
   process restarts with the new template-instantiated flow as its
   active config. Simpler but breaks the runtime-flow-changes story
   the framework was designed for.

Phase 4 investigates the framework's multi-flow runtime support and
picks the right closure. Option 1 is the cleanest — it matches the
agent-tools discipline and approval-flow seam — but requires an
upstream PR.

### Framework-alignment review

Per [[feedback_framework_alignment_review]], before any product-shell
additions:

| Surface | Upstream status | Action |
|---|---|---|
| `flowtemplate.Manager` | exists | use it |
| 6 flow-template tools | exist, auto-register | allowlist in coordinator config |
| 5 flow-CRUD tools | exist | allowlist `create_flow` in coordinator config |
| Template directory-loader | does **not** exist | product-shell `loadFlowTemplates()` in `cmd/semteams/main.go`, mirror `persona.LoadFromDirectory` pattern. **Upstream issue**: `flowtemplate.LoadFromDirectory` follows the persona-loader pattern. File before merge. |
| `deploy_flow` agent tool | does **not** exist | **Upstream issue**: register a `deploy_flow` executor with `FlowEngineManager` dependency. Phase 4 picks closure. |

The product-shell additions in this ADR are:

- One new function `loadFlowTemplates()` in `cmd/semteams/main.go`
  (~50 LoC, mirrors `loadPersonaFragments`).
- Optionally one new product-shell rule action for Phase 4 option 2
  if we don't wait for upstream.

That's it. No new tools, no new payloads, no new buckets, no new
streams. The chain-mode/phasevalidator/chainstall Go code is
unchanged in this ADR — it keeps running on legacy configs.

## Phasing

The first five phases build the new path. Phase 6 is the
migration cut: validate end-to-end, then delete legacy in a single
focused PR. No "vestigial mode running on the side" — the legacy
path either works (during build) or is gone (after migration).

### Phase 1: Template-loader infrastructure (~50 LoC, product-shell)

- Add `loadFlowTemplates(ctx, natsClient, dir, logger)` to
  `cmd/semteams/main.go`. Scans `<dir>/*.json` (default
  `configs/flow-templates/`), unmarshals to `flowtemplate.Template`,
  calls `mgr.Create()` (or `Update()` if exists — idempotent
  re-seed).
- CLI flag: `--flow-templates-path` (defaults to
  `configs/flow-templates/`).
- Boot order: after `buildFlowTemplateManager`, before
  `setupToolsAndPreprocessor`. Templates need to be in KV before
  the tool registry comes online so `list_flow_templates` returns
  them on the first call.
- Validation: `Template.Validate()` runs per-file; boot fails on
  malformed templates (consistent with persona-loader behavior).
- Contract test: `test/contract/flow_templates_seed_test.go` —
  asserts every `configs/flow-templates/*.json` parses + validates
  + instantiates with declared defaults.

**Exit criteria:** Boot semteams with `configs/flow-templates/`
populated; `list_flow_templates` (via tool or via HTTP) returns the
seeded set.

### Phase 2: Author first 2-3 templates

- `research-pipeline.json` — parameterized version of `dev-research.json`.
  Parameters: `topic`, `max_iterations`, `model` (defaulting to
  `claude-haiku`), `tools_allowlist` (defaulting to research minimal).
- `dev-via-spec-pipeline.json` — parameterized version of `osh-demo.json`.
  Parameters: `target_framework`, `max_iterations`, `sandbox_image`,
  `model`.
- One **non-software** template to validate generality. Two
  candidates:
  - `web-research` — pure source-grounded research over web
    substrate, OSINT discipline (cited sources, graded confidence,
    corroboration). Parameters: `topic`, `max_sources`,
    `confidence_threshold`.
  - `decision-memo` — frame question → enumerate options → recommend.
    Parameters: `question`, `option_count_target`.
  - **Pick one for first cut.** Web-research has the most prior
    investment (deep-research rename was about clearing namespace
    for it) — start there.

Each template's body is a full flow JSON. Parameter substitution
points are limited to LLM-author-controllable knobs — components,
streams, payload-registry, and persona-loader paths are **not**
parameterized (they're constraints of the harness, not knobs).

**Exit criteria:** Three templates seed cleanly; mock-LLM journey
exercises `list_flow_templates` → `get_flow_template` →
`instantiate_flow_template` (preview only, no persist).

### Phase 3: Coordinator persona + tool surface

- Add `list_flow_templates`, `get_flow_template`,
  `instantiate_flow_template`, `create_flow` to coordinator
  config's `allowed_tools`. Mark `create_flow` and (for now)
  `instantiate_flow_template` as `approval_required` — every
  coordinator instantiation goes through human approval until trust
  is built.
- New coordinator persona fragment:
  `configs/personas/fragments/coordinator/40-flow-instantiation.md`.
  Teaches the new `decide(action=instantiate, ...)` terminal.
- Coordinator rule:
  `configs/rules/coordinator/06-instantiate-template.json`. Fires
  on `coordinator.decision.next_action = "instantiate"`. Calls
  `instantiate_flow_template` then `create_flow` (via two
  publish-tool actions, since rule-engine doesn't yet support
  chained tool calls).
  - Open: does the rule engine support sequencing two tool calls
    in one rule firing, or do we need two rules with a state
    transition? Investigate in Phase 3.

**Exit criteria:** Mock-LLM journey: user prompt → coordinator
classifies → coordinator calls `list_flow_templates` →
`instantiate_flow_template` → `create_flow`. Assert the new flow
appears in the FLOW_DEFINITIONS KV bucket. Don't deploy yet.

### Phase 4: Deploy / dispatch closure

**Closure picked: Option 1 (upstream `deploy_flow` tool family).**
Phase 4 investigation 2026-05-17 resolved this — see addendum below.

Semteams-side work after upstream lands:

- Add `buildFlowEngine(configMgr, flowMgr, componentRegistry,
  natsClient, metricsRegistry, logger) *flowengine.Engine` next to
  `buildFlowManager` (~25 LoC).
- Refactor `buildFlowManager` to return `*flowstore.Manager`
  (concrete) so the same instance threads into both
  `ToolDependencies.FlowManager` and `flowengine.NewEngine`. Mirrors
  the `*flowtemplate.Manager` refactor from Phase 1.
- Thread `metricsRegistry` and the engine through
  `setupToolsAndPreprocessor` into the deps struct's
  `FlowEngineManager` field.
- Coordinator config: allowlist `deploy_flow` and `start_flow` under
  `approval_required` so coordinator-authored flows route through
  human approval before going live. `stop_flow` and `undeploy_flow`
  follow the same gating posture but may not be needed in v1.

**Exit criteria:** Real-LLM smoke (Phase 5 below) shows a user
prompt produces a deployed running flow that handles the task
end-to-end, instantiated from a template the coordinator picked.

#### Addendum (2026-05-17): Phase 4 investigation findings

The investigation surfaced three load-bearing facts that resolved
the closure without ambiguity:

1. **Multi-flow runtime works today.** `service/component_manager.go`
   already watches `semstreams_config` KV
   (`OnChange("components.*")`) and dynamically creates + starts new
   components when new keys appear with `Enabled: true`
   (`handleComponentConfigUpdate` → `createAndStartComponent`).
   `flowengine.Engine.Deploy` writes per-component configs to that
   bucket; `Engine.Start` flips `Enabled: true`. No process
   restart needed. **Option 3 (process restart) ruled out.**

2. **`flowengine.Engine` is constructible from product-shell.** All
   six deps (`*config.Manager`, `*flowstore.Manager`,
   `*component.Registry`, `*natsclient.Client`, `*slog.Logger`,
   `*metric.MetricsRegistry`) are already in scope at
   `setupToolsAndPreprocessor`. ~10 LoC to construct.

3. **Rule engine doesn't sequence tool calls — agents do.** The
   rule engine's `Actions []Action` union does not include a
   `publish_tool` action; tool dispatch goes through
   `publish_agent` (which spawns a loop that itself calls N tools
   in sequence). So the coordinator's `decide` loop handles
   `instantiate_flow_template` → `create_flow` → `deploy_flow` →
   `start_flow` as four LLM-driven tool calls within one loop,
   not as four rule fires. This **resolves ADR-042 open question
   2** in §"Open questions".

The upstream PR (~300 LoC: `flow_lifecycle.go` executor +
`FlowEngineManager` interface in `ToolDependencies` + gate in
`RegisterBuiltins`) was filed against semstreams and shipped in
**beta.76**. Tool surface: `deploy_flow`, `start_flow`,
`stop_flow`, `undeploy_flow`, each taking `flow_id: string`.
`*flowengine.Engine` satisfies the new interface by duck typing.

**Option 2 (product-shell tactical bridge) ruled out** because
the upstream PR landed on a fast cycle — no need to ship throwaway
code.

**Pre-Phase-2 prereq surfaced**: templates may want an
`auto_start: bool` parameter (default `true`). After `deploy_flow`,
the flow sits at `RuntimeState = deployed_stopped` until
`start_flow` lands it at `running`. Default `true` lets the
coordinator stitch the four-call sequence mechanically. `false`
puts a human gate between deploy and start (e.g. for templates that
need approval before live traffic). Mirror upstream's
`flowstore.RuntimeState` vocabulary rather than inventing a new one.

### Phase 5: Real-LLM validation on the template path

- Bootstrap config + 3 templates from Phase 2 + coordinator path
  from Phase 3 + deploy closure from Phase 4 must all work
  together on real LLM.
- Smoke 1 — research prompt: coordinator picks `research-pipeline`
  → instantiates → deploys → researcher loop → reviewer loop →
  user reply. ~$0.30-$0.50.
- Smoke 2 — dev-via-spec prompt: coordinator picks
  `dev-via-spec-pipeline` → researcher → architect-phase →
  builder → reviewer-qa. ~$0.50-$1.00.
- Smoke 3 — non-software prompt: coordinator picks `web-research`
  → researcher (web-grounded, OSINT) → reviewer-research → user
  reply. ~$0.30-$0.50.

**Exit criteria:** All three smokes converge to user-reply
terminal states under the template path. No reliance on chain.mode/
phasevalidator/chainstall code paths — these can be confirmed
unreached via instrumentation (log a warning on entry; absence of
warnings on a clean smoke = unreached).

### Phase 6: Migration cut — delete the legacy path

Single focused PR after Phase 5 green. Deletes the chain-mode
machinery and concrete configs in one atomic change. Reviewer-pass
required.

**Code deletions:**

- `cmd/semteams/chainmode/` (~120 LoC + tests)
- `cmd/semteams/phasevalidator/` (~600 LoC including tests)
- `cmd/semteams/chainstall/` (~400 LoC including tests)
- Any `cmd/semteams/chain/` code specific to mode-keyed routing
  (chain entity stays per ADR-038; only mode-specific code goes)

**Config deletions:**

- `configs/dev-research.json` (renamed in PR #166 — namespace
  cleanup carries forward into the template; file itself goes)
- `configs/osh-demo.json`
- `configs/agentic.json`, `configs/agentic-claude.json`
- `configs/onboarding.json`
- All `configs/e2e-*.json` that exercise the chain machinery
- `configs/rules/coordinator/*.json` rules that key on
  `actionToMode` outputs (kept rules: anything mode-agnostic)
- Mode-specific rule dirs: `configs/rules/research-mode-transition/`,
  `configs/rules/dev-via-spec/`, `configs/rules/research-iterative/`,
  etc.

**Test deletions / rewrites:**

- `test/contract/coordinator_slice1_test.go` — pins persona-action
  allow-list. Delete the closed-set portion; new tests pin the
  template-instantiation contract.
- `test/contract/config_dispatch_test.go`,
  `test/contract/config_graph_pairing_test.go` — pin concrete
  config filenames. Replace with `test/contract/flow_templates_*.go`
  pinning the template inventory.
- Chain-mode-specific contract tests
  (`test/contract/adr041_persona_dirs_test.go` shape pins, etc.)
  — rewrite to pin template-rendered flow shapes.

**Doc updates:**

- CLAUDE.md — remove chain-mode framing, replace with template-
  driven framing.
- README.md, docs/getting-started.md — update "fastest path" to
  use the bootstrap-config + coordinator-picks-template flow.
- Mark superseded ADRs explicitly (ADR-037, ADR-039, and the
  partial-supersession of ADRs 030/038/041 sections).

**Migration memory entry**: capture which legacy artifacts went
where (deleted vs. ported into templates) so future audits don't
have to reconstruct the move.

**Exit criteria:** Repo builds, all tests pass, real-LLM smoke
#5 reruns green under the new path only. `grep -rn 'chainmode\|
phasevalidator\|chainstall' cmd/ configs/` returns nothing.

## What this does NOT change

- **Persona corpus structure** — the 4-role MVP roster from ADR-041
  stays. Templates load persona fragments via the existing
  `persona.Manager` infrastructure. Chain-scoped persona overlays
  (cleaned up in PRs #163-#165) compose with the shared corpus per
  template body.
- **Chain entity (ADR-038)** — lineage rollup, milestone tracking,
  chain.* triples all keep working. Every template-instantiated flow
  that produces multi-loop work rolls up into a chain entity the
  same way. The chain *entity* survives; the chain *mode-keyed
  machinery* doesn't.
- **Approval flow (ADR-030)** — `approval_required` on `create_flow`
  + `deploy_flow` is the seam. Human approves every coordinator-
  authored flow until trust is built.
- **Ops agent (ADR-027)** — queries graph entities for completed
  loops; doesn't care whether they came from a concrete config or a
  template-instantiated flow. Objective specs (under
  `docs/objectives/`) carry forward as-is.
- **Emit-tool domain discipline (ADR-031)** — each template can
  reference its own emit tool (`emit_research_artifact`,
  `emit_web_research_artifact` if added, etc.). Domain-fit
  artifacts stay domain-fit; templates compose them.

## What we learned from chain machinery (preserved as design constraints)

Phases 1b/1c/2 of the coordinator work (PRs #157-#162, late
2026-05) produced empirical lessons that the template-driven path
must honor. None of them justify keeping the hardcoded mode
machinery, but the templates need to encode equivalent guarantees:

- **Structural gates beat LLM judgment** ([[feedback_structural_over_llm_judgment]]).
  Phasevalidator caught synthesize → architect drift structurally
  (smoke 2026-05-16, real-LLM, GREEN). The template path needs the
  same: each instantiated flow's edges define what transitions are
  allowed. The validator becomes flow-graph-derived rather than
  mode-keyed.
- **Recovery cap is load-bearing** (PR #139, smoke #19). Looping
  recovery without a cap burns tokens; templates that produce
  multi-loop chains need a per-flow `recovery_count` cap. The
  primitive lives in flow-config, not in `chainstall/recoverycounter`.
- **Per-reject-reason routing is the right grain**
  ([[feedback_rejected_to_coordinator]]). Framing-fixable rejects
  route to coordinator for re-instantiation; budget/structural
  rejects escalate to human. The routing table moves from
  `chainstall/routing.go` into flow-template metadata.
- **Approval gate on write tools must stay** (ADR-030).
  `create_flow` + `deploy_flow` get gated; coordinator can author
  but cannot ship without approval.
- **Coordinator persona-text alone is insufficient as a contract**
  (smoke #155 lesson — classification GREEN but
  synthesize-drift recurred until structural gate landed). The
  template-driven path replaces persona-text-as-contract with
  flow-graph-as-contract.

## What this changes about previous direction

- **The audit's "FRICTION: small Go PRs to add a new mode"
  framing** (this session, 2026-05-17) was correct under the
  assumption that we'd keep adding `chain.mode` values. Under this
  ADR, **we stop adding chain.mode values *and we delete the ones
  we have***. New prompt classes land as templates; the audit's
  friction points get deleted along with the legacy path.
- **The "is the shared persona corpus actually domain-neutral?"
  question** ([[project_persona_osh_audit]] §pickup) was framed as
  a single smoke against the existing chain. Under templates, the
  question reframes to "do template-scoped persona overlays
  compose cleanly with the shared corpus?" — a different (and
  better) test exercised by Phase 5 smoke 3 (web-research, the
  non-software template).
- **The "build web-research as a config-only variant" proposal**
  (this session, 2026-05-17) was correct in shape but the *unit*
  was wrong. Web-research becomes a **template**, not a concrete
  config. The work scope is similar; the artifact is different.
- **PR #166 (dev-research rename, in flight)** — the rename
  cleared the namespace and pulled OSH-Meshtastic content out of
  the shared corpus. Both outcomes carry forward into the
  template-driven path: web-research is a template name with no
  collision, and the cleaned corpus composes with chain-scoped
  template overlays. The `configs/dev-research.json` file itself
  is deleted in Phase 6 — replaced by `configs/flow-templates/research-pipeline.json`.
  The rename was not waste; the persona-corpus cleanup it
  carried is the load-bearing piece.
- **Coordinator Slices 1/1b/1c/2 (PRs #155-#162)** — the
  *classification* work (Slice 1, PR #155) preserves into the
  template path: coordinator still classifies, just outputs a
  template selection instead of a closed mode enum. The
  *structural-gate* work (Slices 1b/1c, PRs #157/#158)
  preserves as **design constraint**: validators must be
  structural, but they're now flow-graph-derived. The
  *chainstall-recovery* work (Slice 2, PRs #159/#160/#162) gets
  deleted as code but **preserves as lesson** (recovery cap,
  per-reason routing) — these become flow-template metadata.

## References

- [[ADR-029]] — product-shell wiring. This ADR adds one new
  function to `cmd/semteams/main.go` (`loadFlowTemplates`)
  following the same Pattern-B discipline.
- [[ADR-038]] — chain entity. Unchanged; templates produce flows
  whose loops still roll up into chain entities.
- [[ADR-041]] — 4-role MVP roster. Unchanged; templates use the
  existing roles.
- [[feedback_framework_alignment_review]] — applied above in the
  framework-alignment review section. Two upstream issues to file.
- [[project_persona_osh_audit]] — the persona corpus cleanup
  trilogy (#163-#165) sets the stage for templates by stripping
  hardcoded chain flavor from the shared corpus.
- [[project_semteams_is_the_harness]] — the 2026-05-17 reframe.
  Templates are the *mechanism* that makes the reframe operational:
  bundled chains become bundled templates, illustrative of what the
  harness can run.
- Upstream `processor/agentic-tools/executors/flow_templates.go` —
  the six template tools.
- Upstream `processor/agentic-tools/executors/flows.go` — the five
  flow-CRUD tools.
- Upstream `flowtemplate/template.go` — `Template` shape.
- Upstream `flowstore/flow.go` — `Flow` shape.
- Upstream `engine/engine.go` — `Deploy / Start / Stop / Undeploy`
  methods (HTTP-only today).

## Open questions

1. **Phase 4 deploy closure.** Which of the three options does the
   framework support cleanly? Investigation precedes Phase 4
   implementation.
2. **Rule-engine chained tool calls.** Does the rule engine support
   sequencing `instantiate_flow_template` + `create_flow` in one
   firing, or do we need two rules with an intermediate state
   transition?
3. **Multi-flow process.** Can one semteams binary run multiple
   concurrent flows? If yes, template-instantiated flows are routing
   targets alongside legacy concrete configs. If no, the deploy
   story involves a more complex handoff.
4. **Parameter typing.** Templates declare params as
   `{name, description, default, required}` — all defaults are
   strings. Numeric / enum / list parameters require text/template
   marshalling tricks. Acceptable for v1?
5. **Approval-loop UX.** What does the human-approval surface look
   like for "the coordinator wants to create this flow" — Kanban
   card with the rendered flow JSON? UI work scope.

These are not blockers for the ADR — they're the investigation
agenda for Phases 1-4.

---

## 2026-05-17 §Phase 2 redesign — substrate-plus-overlays, not template instantiation

After Phases 1, 4, and 2a shipped (PRs #168, #169, #170), the
"5-20KB of escaped JSON per template" framing for Phase 2b
triggered Coby's pushback ("i do not like the sounds of this").
A second investigation — re-reading the framework's primitives
with a different question — revealed that **the template-driven
flow instantiation model was the wrong unit of work for MVP**.
This addendum captures the corrected design. The Decision and
Phasing sections above reflect pre-redesign thinking and are
preserved for the historical record; this addendum is the active
direction.

### What changed in the framework reading

The original ADR assumed: "to add a task class, the coordinator
authors a flow template, instantiates it at runtime, deploys it
into a fresh component set." That assumes per-task-class
isolation via separate component instances. The re-investigation
asked: **why do we need separate component instances at all?**

The answer turned out to be "we mostly don't." The framework
supports running multiple task classes through a **singleton
substrate**:

- **One `agentic-loop` instance** drives all task classes.
  Subscribes to a broad subject (`agent.task.>`); per-task
  `task.Role` selects the persona; per-task `task.Tools`
  narrows the LLM-visible tool list; per-task `task.Model`
  selects the LLM.
- **One `agentic-tools` instance** with a broad
  `allowed_tools` ceiling enforces the executor-side safety
  floor; per-task `task.Tools` (set by rules via
  `publish_agent` actions) narrows what the LLM actually
  sees.
- **One `rule-processor` instance** loads all rule packs via
  the existing hot-reload `rule.ConfigManager` (KV-watched).
  Rules pattern-match on event shape and chain context to
  fire only for their target task class.
- **One `persona.Manager` instance** reads the global
  `PERSONAS` KV bucket. Fragments are tagged with `Roles`
  — which the framework treats as **named addressable
  bundles**, not just "cognitive role labels." A task with
  `task.Role = "researcher-web"` pulls exactly the fragments
  tagged with that name. Bespoke personas are first-class
  invokable units under this addressing model.

The persona-scoping "gap" from the first investigation pass was
answering the wrong question. The original framing was "can two
different `agentic-loop` instances have different persona
scopes?" — which is moot when there's only one instance.
Bespoke per-task-class personas are achieved by **creating
named persona-fragment dirs and addressing them via the role
field on the task** — exactly the convention the framework's
addressing system already supports.

### What an MVP task class actually is

A task class = **{ rule pack + named persona bundle + tool
allowlist + initial role }**. None of these require new
component instances. Adding a task class = adding rule files +
persona files + a coordinator-routing entry. Removing one =
deleting them.

```
configs/
  flow-bootstrap.json        # single substrate config: agentic-* + graph-* + http + rule-processor + coordinator's agentic-loop
  rules/
    coordinator/             # classifier rules (existing, post-#157-#162)
    research/                # research-class rule pack
    web-research/            # web-research-class rule pack
    dev-via-spec/            # dev-via-spec-class rule pack
  personas/fragments/
    coordinator/             # shared classifier (post-#163-#165 cleanup)
    researcher-research/     # bespoke researcher persona for the research class
    researcher-web/          # bespoke researcher persona for the web-research class
    reviewer-research/       # shared reviewer scope (or split per-class as needed)
    builder-dev/             # dev-via-spec builder persona
    ...
```

The framework's `Roles` field on each Persona is the
addressing key. `task.Role = "researcher-web"` pulls only the
`researcher-web/*.md` fragments. Coordinator routes by setting
the role on the spawned task.

### Coordinator's job under the corrected model

Single responsibility: **classify the user prompt into a task
category, emit `decide(action=<category>)`**. The rule layer
takes it from there:

1. User prompt arrives → coordinator agentic-loop spawns
2. Coordinator emits `decide(action=research | web_research | dev_via_spec | ...)`
3. Category-keyed rule (e.g. `configs/rules/research/01-spawn-researcher.json`) pattern-matches the action, publishes the next agent task with the right role + tools + model
4. The chain progresses through rule-driven hand-offs until terminal
5. Reviewer's `decide(action=approved | insufficient | needs_clarification)` terminates the chain or kicks off recovery

This is **essentially what coordinator Slices 1/2 (PRs
#155-#162) shipped** — minus the chain.mode/phasevalidator/
chainstall Go machinery that was layered on top. Rules
pattern-matching on `coordinator.decision.next_action`
already discriminates task class; we don't need a Go enum for
it. The chain-mode machinery was the workaround for not
trusting the rule layer to do its own pattern-matching with a
clean rule pack per class.

### MVP scope (the new authoritative scope)

| Phase | Work | Touchpoints |
|---|---|---|
| **MVP-1** | Bootstrap config wiring the substrate singletons. Replaces `configs/dev-research.json` + `configs/osh-demo.json` as the single boot config. Authored once; runtime adds task classes around it. | `configs/flow-bootstrap.json` |
| **MVP-2** | Category-keyed rule packs. 2-3 task classes ship in the inventory: `research`, `web-research`, `dev-via-spec`. Each pack is the classic Slice 1/2 shape — spawn rules, terminal handlers, recovery routes — without chain.mode gating. | `configs/rules/<category>/*.json` |
| **MVP-3** | Named persona inventory. Bespoke researcher / reviewer / builder bundles per category. Builds on the post-#163-#165 cleaned-up shared corpus. | `configs/personas/fragments/<persona-name>/*.md` |
| **MVP-4** | Coordinator persona teaching the classifier the closed category taxonomy. `decide(action=<category>)` is the terminal. | `configs/personas/fragments/coordinator/*.md` (extend) |
| **MVP-5** | Mock-LLM journey validating coordinator → category-keyed rule → spawn → terminal end-to-end. | `ui/e2e/agentic/coordinator-category-*.spec.ts` |
| **MVP-6** | Real-LLM smoke per category (~$0.30-$0.50 per category). | Manual run |
| **MVP-7** (was original Phase 6) | Migration cut: delete `cmd/semteams/{chainmode,phasevalidator,chainstall}/`; delete the legacy concrete configs; rewrite docs. Single atomic PR. | Reviewer-pass required. |

### MVP debt assessment

**Effectively zero on the framework axis.** The corrected design
uses framework primitives as intended:

- `Persona.Roles` as the named-bundle addressing key (correct
  use; just nomenclature is overloaded with "cognitive role")
- `task.Role` / `task.Tools` / `task.Model` on the task envelope
  (existing per-task scoping mechanisms, used as designed)
- `rule.ConfigManager` hot-reload (existing, used as designed)
- Rule pattern-matching on `coordinator.decision.next_action`
  triples (existing, used as designed)

**Authoring discipline only**: persona dir names must match
the role tokens the coordinator emits and the rules expect.
A single contract test pins this three-way consistency.

### Post-MVP path (what the original ADR was building toward)

Runtime category creation — coordinator authors a new task class
when no existing one matches a prompt — IS still the long-term
direction. It requires:

- `flowtemplate.Manager` substrate + tool surface (already
  available, primed by **PR #168** Phase 1 loader)
- `deploy_flow` / `start_flow` agent tools (already available,
  primed by **PR #169** Phase 4 wiring)
- Reference templates for the inventory (primed by **PR #170**
  Phase 2a skeletons; full bodies are post-MVP work)
- Approval-loop UX for "coordinator wants to create this new
  task class" (post-MVP UI work)
- Possibly: `persona.Config.BucketName` upstream so per-flow
  persona scopes can be cleanly isolated (post-MVP if at all
  — role-suffixing is the workable convention until the
  inventory exceeds ~10 task classes)

**All three Phase 1/4/2a PRs stay as post-MVP scaffolding.**
They cost zero MVP friction — the loaders + tools sit primed
but unused until the post-MVP work activates them.

### What the original ADR text gets right

- **Supersession set** (ADR-037, ADR-039, chain-mode work) —
  still correct. The migration cut still happens; the
  replacement is cleaner under the corrected design (rules
  encode their own valid transitions per category, no global
  Go gate).
- **What we learned from chain machinery** — still relevant.
  Structural-gate discipline, recovery cap, per-reject-reason
  routing — all become per-category rule-pack design
  constraints rather than chain.mode Go code.
- **ADR-038 chain entity** stays — lineage substrate is
  flow-shape-agnostic and remains useful for any task class
  that spans multiple loops.
- **ADR-041 4-role roster** stays — researcher / reviewer /
  builder / coordinator are the cognitive functions; named
  persona bundles (`researcher-web`, `reviewer-research`)
  are addressed under those role names with chain-suffixes.

### What the original ADR text gets wrong

- **"Templates are the only unit of prompt-class definition"
  (§Decision High-level)** — wrong for MVP. The unit is
  category-keyed rule packs + named persona bundles. Templates
  are the post-MVP unit for runtime category creation.
- **"Each template's body is a full flow JSON"
  (§Decision Templates parameterize chain shape)** — wrong
  for MVP. Skeleton templates from PR #170 stand as
  post-MVP exploration; MVP doesn't need full bodies.
- **Phases 2b through 5 as originally framed** —
  superseded by the MVP-1 through MVP-7 table above. The
  original phases are still useful as the post-MVP
  expansion sequence.

### Open questions closed by this redesign

- **Original OQ-1 (Phase 4 deploy closure)** — resolved
  in the §Phase 4 addendum above. Closure stays available
  for post-MVP.
- **Original OQ-2 (rule-engine chained tool calls)** —
  resolved by the §Phase 4 investigation. Not blocking;
  agents (not rules) sequence tool calls when needed.
- **Original OQ-3 (multi-flow process)** — confirmed
  supported but **not needed for MVP**. The corrected
  design uses singleton substrate, not multi-flow.
- **Original OQ-4 (parameter typing)** — moot for MVP;
  becomes a real question post-MVP if/when runtime
  template instantiation is wired.
- **Original OQ-5 (approval-loop UX)** — post-MVP UI work.

### New open questions

1. **Recovery routing per category** — coordinator Slice 2
   shipped chainstall + per-reject-reason routing. Under
   the redesign, each category's rule pack owns its own
   recovery routes. Smoke-test that a category's reject
   handlers compose cleanly without the chainstall
   primitive. May need a small recovery-rule pattern in
   each pack.
2. **Persona naming convention** — `researcher-web` vs
   `researcher.web` vs `web-research-researcher`? Settle
   on one before authoring MVP-3 to avoid renaming later.
   Recommend `<role>-<category>` (e.g. `researcher-web`)
   so the role prefix sorts together.
3. **Contract test scope** — assert (persona dir names) ∪
   (role tokens emitted by coordinator) ∪ (role names
   referenced in rule actions) are mutually consistent.
   Single test, three sets. Catches the "added a category
   but forgot to update the coordinator persona" class of
   bug at commit time.

### Sequencing for the next session

1. Draft MVP-1 (bootstrap config). Should it derive from
   `configs/dev-research.json` or be authored fresh from the
   substrate-singletons design? Either approach works;
   fresh authoring keeps the boot config small and explicit.
2. Pick the first MVP category to ship end-to-end. Lean:
   `research` (closest to existing chain shape; easiest to
   validate the redesign against the smoke we already have).
3. After MVP-1 + one category lands and validates on
   mock-LLM, parallelize MVP-2/3 for the remaining
   categories.

The migration cut (MVP-7) waits for all categories +
real-LLM validation. Same posture as the original ADR's
Phase 6 — atomic deletion PR with reviewer pass.

## §addendum 2026-05-18 — MVP-7 migration cut (PR #178)

MVP-7 landed as a single atomic PR. Deletions:

- **Go packages**: `cmd/semteams/{chainmode,phasevalidator,chainstall,recoverycounter}/`.
  All four pinned the legacy chain.mode-keyed gates; the
  research-pack rule layer reaches the same recovery + terminal
  behavior without them (pack identity replaces mode classification;
  rule `max_iterations` replaces per-entity recovery counting).
- **Concrete configs**: `configs/{agentic.json, agentic-claude.json,
  dev-research.json, onboarding.json, osh-demo.json, e2e-agentic.json,
  e2e-coordinator.json, e2e-dev-research.json, e2e-dev-via-spec.json,
  e2e-ops-observer.json, e2e-research-harness-hit.json,
  e2e-research-iterative.json, e2e-research-mode-transition.json,
  e2e-research-with-source.json}`. The surviving pair is
  `flow-bootstrap.json` (production) + `e2e-flow-bootstrap.json`
  (mock-LLM clone).
- **Rule dirs**: `configs/rules/{research-mode-transition,
  research-iterative, dev-via-spec}/` deleted entirely.
  `configs/rules/coordinator/{01-delegate-research, 02-delegate-dev-chain,
  04a-research-terminal-to-coordinator, 04b-qa-terminal-to-coordinator,
  05-chain-stall-to-coordinator}.json` + `configs/rules/chain/00-loop-failed-pause.json`
  deleted. Surviving coordinator rules: `03-ask-user.json`,
  `03b-respond-direct.json`. Surviving research rules:
  `configs/rules/research/01..08-*.json` (the MVP-2 pack).
- **Persona dirs**: `configs/personas/fragments/{researcher, researcher-plan,
  researcher-gather, researcher-synthesize, researcher-architect,
  reviewer-spec, reviewer-qa, source-registrar, research-reviewer,
  builder}/` deleted. Surviving roles:
  `coordinator`, `researcher-research-{plan,gather,synthesize}`,
  `reviewer-research`, `ops`, `ops-chain-observer`, `ops-progress-observer`.
- **Fixtures + Playwright specs**: all journey YAML and spec files
  paired with deleted configs. Surviving: `research-mvp.yaml`,
  `ops-agent-baseline.yaml`, `real-time-activity-stream.yaml`, plus
  the spec files for the orthogonal UI tests (task-story-trace,
  action-chips-personas, task-title-aliases, admin-flows-inventory)
  which were repointed to the surviving fixture + config.
- **Contract tests**: `chain_stall_routing_test.go`,
  `chainstall_handler_order_test.go`, `chain_pause_rule_test.go`,
  `recovery_cap_rule_test.go`, `dev_via_spec_rules_test.go`,
  `lineage_threading_test.go`, `adr041_persona_dirs_test.go`.
  Replacement: `chain_agents_no_graph_test.go` preserves the
  ADR-041 chain-agents-no-graph-tools invariant + ops-persona
  presence check.
- **main.go subscribers**: chainmode.Stamper, phasevalidator.{Phase,SpecMode,QAMode},
  chainstall.Subscriber, recoverycounter.Counter, plus the
  `extractStallConfig` helper. Surviving completion handlers:
  `dispatched`, `terminalStamper`, `research` (milestone),
  `needsReview`.
- **chain.terminal**: `reviewerQARole` + `acceptAction` constants
  retired; `roleToApprovalAction` is single-entry
  (`reviewer-research → approved`). A future dev-via-spec category
  pack reintroducing reviewer-qa would extend the table.

What stays out of scope (deferred to follow-up cleanup PR or future
MVP):

- `cmd/semteams/devviaspec/` Go package — still needed by
  `tools/emitplan` (Plan payload type is shared with research arc).
  `artifact.go` is dead under MVP-7 but couples to MVP-1 build via
  payload registration; cleanup deferred to avoid widening this PR.
- `tools/emitspecartifact/`, `tools/builderdecide/`, `verification/`,
  `sandbox/` — all dev-via-spec-arc supporting code. Tool registry
  still includes them; they're inert under bootstrap (no rule spawns
  a builder, no persona names them in tool allowlists).

MVP-7 closes the substrate-plus-overlays migration. Future MVPs
introduce new category packs (dev-via-spec re-introduction,
web-research, decision-memo, etc.) as additive rule + persona
inventory without further migration steps.
