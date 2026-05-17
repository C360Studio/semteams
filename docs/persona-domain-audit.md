# Persona Domain Audit (2026-05-17)

## Purpose

Classify every fragment in `configs/personas/fragments/` as
**harness-level** (domain-neutral; reusable across any chain) vs
**software-domain** (assumes software-shaped substrate, tools, or
artifacts) vs **mixed** (one fragment that combines both).

Why: SemTeams is a configurable agentic harness (per CLAUDE.md
opener, 2026-05-17). The shared persona corpus should be
harness-level by design; domain-flavor belongs in chain-scoped
overlays. Today the shared corpus has drifted toward software
domain. This audit is the first step toward separating them so
non-software prompt classes (web research, comparative analysis,
decision memo, etc.) can opt into a clean harness layer.

This document is the artifact of Phase 2a (classification only —
no file moves, no loader changes). Phase 2b (structural overlay
loading + file moves) is downstream work, scoped only after this
classification is reviewed.

## Methodology

For each fragment file:

1. Count software-domain telltale tokens:
   `test_harness`, `surefire`, `builder_decide`, `IDriver`,
   `emit_research_artifact`, `emit_dev_via_spec_artifact`,
   `emit_plan`, `source_repo`, `semsource`, `add_source_repo`,
   `bootstrap_workspace`, `/artifacts/specs`, `/artifacts/plans`,
   `/artifacts/research`, `checks[]`, `process-local-testcontainer`,
   `external-sidecar`, `in-process-unit`, `browser-flow`,
   `static-analysis`, `test_runtime`, `verifiable outcomes`,
   `src/test/java`, `src/main/java`, `pom.xml`, `mvn `, `go test`,
   `junit`.

2. Read the fragment in full where the token count is borderline
   (1–6).

3. Decide: harness / software-domain / mixed. Rationale captured
   in the table.

The audit reflects the persona corpus as of branch
`chore/classify-personas-by-domain` (stacked on the PR #163
OSH-cleanup branch).

## Classification taxonomy

- **harness** — describes a persona's job (decide-action contract,
  output-contract structure, tool-usage discipline) without
  assuming software-shaped substrate. Usable as-is for web
  research, comparative analysis, decision memo, etc.
- **software-domain** — assumes software-shaped substrate (source
  repos, test_harness, checks[]), or invokes software-flavored
  emission tools (emit_research_artifact, emit_plan,
  emit_dev_via_spec_artifact, builder_decide), or names software-
  specific artifacts (Maven, surefire, Java).
- **mixed** — single fragment that combines both; would need
  splitting before chain-scoped overlay separation can be clean.

## Role-by-role summary

| Role | Fragments | Verdict | Notes |
|------|-----------|---------|-------|
| `coordinator/` | 3 | **fully harness** | The only role purely about classification + routing; no software coupling. |
| `ops/` | 3 | **mostly harness** | 05-semteams-identity names "deep-research" as a deployment fact; otherwise harness-level. |
| `ops-chain-observer/` | 3 | **mixed** | Observation pattern is general; framing binds to "dev-via-spec chain." |
| `ops-progress-observer/` | 2 | **mixed** | Same as above. |
| `source-registrar/` | 2 | **software-domain** | Role's purpose is registering source repos. |
| `researcher-plan/` | 5 | **mixed** | Plan concept is harness-level; emit_plan tool + harness-catalog are software-flavored. |
| `researcher-gather/` | 3 | **mostly harness** | Workflow (web_search + scratchpad) is general; one forward reference to emit_research_artifact. |
| `researcher-synthesize/` | 5 | **software-domain** | Composes the structured research artifact (SDD shape) and invokes emit_research_artifact + emit_dev_via_spec_artifact. |
| `researcher-architect/` | 5 | **software-domain** | Emits the dev_via_spec artifact; whole role is software-purpose. |
| `reviewer-research/` | 5 | **mixed** | Plan-conformance check (new in PR #163) is harness; 40-harness-gate is software. |
| `research-reviewer/` (legacy mirror) | 5 | **mixed** | Same shape as reviewer-research. |
| `reviewer-spec/` | 4 | **software-domain** | Grades the dev_via_spec artifact's checks[]. |
| `reviewer-qa/` | 3 | **software-domain** | Grades builder output against checks[] evidence gate. |
| `builder/` | 5 | **software-domain** | Role's purpose is building software per a spec. |
| `researcher/` (legacy) | 5 | **software-domain** | Pre-MVP-split researcher; emit_research_artifact + harness-catalog. |

## Per-fragment classification

### `coordinator/` — fully harness

| Fragment | Verdict | Rationale |
|---|---|---|
| 00-identity.md | harness | Classification + routing front door; "every user message arrives at you first." |
| 10-decision-contract.md | harness | Decide-action contract for routing. |
| 20-delegation-rules.md | harness | Routing taxonomy. |

### `ops/` — mostly harness

| Fragment | Verdict | Rationale |
|---|---|---|
| 05-semteams-identity.md | harness, with leak | Deployment-overlay fragment. Names "deep-research" by name (line ~9). Should reference "the bundled chain templates" instead. |
| 10-objective-grounding.md | harness | General objective-spec grounding. |
| 20-diagnostic-rules.md | harness | Diagnosis emission shape. |

### `ops-chain-observer/` — mixed

| Fragment | Verdict | Rationale |
|---|---|---|
| 00-identity.md | mixed | "Per-chain detailed fused observation … wake at the moment a dev-via-spec chain reaches its post-build verdict." Pattern general; framing software-coupled. |
| 10-chain-walk.md | mixed | Chain-walk algorithm general; assumes dev-via-spec chain shape (plan → architect → builder → qa). |
| 20-diagnosis-rules.md | mixed | Diagnosis emission shape general; examples software-flavored. |

### `ops-progress-observer/` — mixed

| Fragment | Verdict | Rationale |
|---|---|---|
| 00-identity.md | mixed | "In-flight chain progress observation"; framing binds to dev-via-spec chain. |
| 10-progress-rules.md | mixed | Progress-rule emission general; examples software-flavored. |

### `source-registrar/` — software-domain

| Fragment | Verdict | Rationale |
|---|---|---|
| 00-identity.md | software-domain | Registers GitHub repo URLs via add_source_repo. |
| 10-tool-contract.md | software-domain | Tool contract for add_source_repo. |

### `researcher-plan/` — mixed

| Fragment | Verdict | Rationale |
|---|---|---|
| 00-identity.md | harness | PLAN phase concept: define scope/goal/epics before corpus reading. Domain-neutral. |
| 10-output-contract.md | mixed | Output contract structure is harness; rendered-markdown coupling to emit_plan is software-flavored. |
| 15-emit-plan.md | software-domain | Wraps emit_plan tool call; the tool itself is software-flavored. |
| 20-revision-rules.md | harness | Revision discipline; domain-neutral. |
| 30-verifiable-outcomes.md | harness | Post-PR-#163 cleanup, examples are generic. Concept (falsifiable claim = input + output) is harness-level. |

### `researcher-gather/` — mostly harness

| Fragment | Verdict | Rationale |
|---|---|---|
| 00-identity.md | harness | GATHER phase: web_search + scratchpad evidence collection. Post-PR-#163 cleanup is domain-neutral. One forward reference to emit_research_artifact (in "what you do NOT do" §). |
| 10-output-contract.md | harness | Scratchpad-as-working-memory contract; domain-neutral after PR #163. |
| 20-iteration-rules.md | harness | Iteration discipline. |

### `researcher-synthesize/` — software-domain

| Fragment | Verdict | Rationale |
|---|---|---|
| 00-identity.md | software-domain | "Compose the structured research artifact from GATHER's evidence and commit it via emit_research_artifact." Tool coupling is the substrate. |
| 10-output-contract.md | software-domain | Artifact JSON shape: actors / integration_points / tasks / addressed_gaps / open_gaps — SDD-flavored. |
| 20-iteration-rules.md | harness | Iteration discipline; could move shared. |
| 30-emit-artifact.md | software-domain | Direct tool wrapper for emit_research_artifact. |
| 40-harness-catalog.md | software-domain | Test harness catalog rules — exclusively software. |

### `researcher-architect/` — software-domain

| Fragment | Verdict | Rationale |
|---|---|---|
| 00-identity.md | software-domain | "ARCHITECT phase: extract typed commitments + verification checks[]." Whole role is dev-via-spec. |
| 10-output-contract.md | software-domain | emit_dev_via_spec_artifact wrapper with full schema. |
| 20-commitment-transcription.md | software-domain | Transcribes verifiable outcomes into checks[] entries. |
| 30-commitment-contract.md | software-domain | When checks[] is required + runtime selection rules. |
| 40-brownfield-discovery.md | software-domain | Brownfield bash-walks of repo tests. |

### `reviewer-research/` — mixed

| Fragment | Verdict | Rationale |
|---|---|---|
| 00-identity.md | mixed | Reviewer pattern (enumerator) is harness; the artifact shape it reads is SDD-flavored. |
| 10-evaluation-contract.md | harness | Evaluation pattern (insufficient/insufficient-but-progressing/approved). |
| 15-stabilisation-check.md | harness | Stabilisation gate; post-PR-#163 generic. |
| 20-plan-conformance.md | harness | New in PR #163; harness-level by design. |
| 40-harness-gate.md | software-domain | Test harness presence check. |

### `research-reviewer/` (legacy mirror) — mixed

Same classification as `reviewer-research/`; legacy mirror dir
retained per ADR-041 Phase 3 retention until
`research-iterative/01-research-to-reviewer.json` rule sunsets.
Per-fragment verdicts mirror the above.

### `reviewer-spec/` — software-domain

| Fragment | Verdict | Rationale |
|---|---|---|
| 00-identity.md | software-domain | Grades the dev_via_spec artifact. |
| 10-evaluation-contract.md | software-domain | Verdict shape (approve / insufficient) on the spec artifact. |
| 20-completeness-checklist.md | software-domain | Substance grading on actors / integration_points / tasks / checks[]. |
| 30-verifiable-outcomes-gate.md | software-domain | checks[] coverage gate. |

### `reviewer-qa/` — software-domain

All three fragments grade builder output against checks[] evidence
gate; purely software-flavored.

### `builder/` — software-domain

All five fragments coach the role of building software per a spec.

### `researcher/` (legacy) — software-domain

Pre-MVP-split researcher. Mirrors researcher-synthesize's
software-flavored output contract + emit_research_artifact wrapper.

## Tally

- **Harness-level fragments:** 18 of 57 (~32%) — mostly in
  `coordinator/`, `researcher-gather/`, `researcher-plan/`,
  `reviewer-research/`, with one each in `ops/`,
  `researcher-synthesize/`, plus the new plan-conformance
  fragments.
- **Software-domain fragments:** 32 of 57 (~56%).
- **Mixed fragments:** 7 of 57 (~12%) — in `ops-chain-observer/`,
  `ops-progress-observer/`, the legacy `research-reviewer/`, and
  one each in `researcher-plan/` (10-output-contract) and
  `reviewer-research/` (00-identity).

## Findings

### 1. Only `coordinator/` is fully harness today

Every other role couples to either software-shaped emission tools
or software-shaped artifacts. This is not a content drift problem
(PR #163 already cleaned content-level drift like
OSH-Meshtastic-driver examples); it is a **structural fact**: the
research-and-build chain shape is the only chain shape the corpus
encodes.

### 2. Standing up a non-software chain today requires a fresh corpus

If we want a web-research chain (source-grounded research over
web substrate, OSINT-influenced discipline applied as the quality
bar), we cannot inherit `researcher-plan` / `researcher-gather` /
`researcher-synthesize` / `reviewer-research` from the shared
corpus and add chain-specific fragments on top. Most of those
roles' fragments either invoke software-flavored emission tools
or describe software-flavored artifact shapes that don't fit
research findings (`actors`, `integration_points`, `tasks`,
`checks[]` are wrong for "summary, key findings, sources with
confidence, open questions").

### 3. The harness-level fragments are real but thin

The fragments that ARE harness-level cluster around:
- decide-action contracts
- iteration / revision discipline
- scratchpad-as-working-memory pattern
- web_search grounding (post-PR-#163)
- evaluation-as-enumerator pattern
- stabilisation gate
- plan-conformance (new in PR #163)

These are valuable building blocks but they are scattered across
8+ role directories. A clean Phase 2b layout would consolidate
them into a `harness/` overlay that ANY chain can opt into, with
role-specific harness-level fragments living under their role
within that overlay.

### 4. Mixed fragments need surgical splits before Phase 2b

The 7 mixed fragments contain harness-level paragraphs and
software-flavored paragraphs in the same file. Mechanical file
moves can't separate them; they need editing. Split candidates:

- `researcher-plan/10-output-contract.md` — separate "what an
  output contract is" (harness) from "rendered as markdown via
  emit_plan" (software-domain).
- `reviewer-research/00-identity.md` — separate "reviewer-as-
  enumerator role" (harness) from "reads research artifact"
  (software-domain coupling).
- `ops-chain-observer/00-identity.md` + `10-chain-walk.md` +
  `20-diagnosis-rules.md` — separate "per-chain observation
  pattern" (harness) from "dev-via-spec chain shape" (software).
- `ops-progress-observer/00-identity.md` + `10-progress-rules.md`
  — same.

### 5. The `ops/` deployment-overlay leak is minor and fixable now

`ops/05-semteams-identity.md` names "deep-research" as a specific
chain. Change to "the bundled chain templates" or "the chains
configured in this deployment" — one-line edit, no structural
impact.

## Recommendation for Phase 2b

When Phase 2b lands (likely concurrent with the web-research
chain config), the structural target is:

```
configs/personas/
├── fragments/                  # harness-level only (default)
│   ├── coordinator/
│   ├── ops/                    # post-leak-fix
│   ├── researcher-gather/      # post-split (harness portion)
│   ├── researcher-plan/        # post-split (harness portion)
│   ├── reviewer-research/      # post-split (harness portion)
│   └── ...
├── overlays/
│   ├── software/               # current software-domain content
│   │   ├── builder/
│   │   ├── source-registrar/
│   │   ├── researcher-architect/
│   │   ├── researcher-synthesize/
│   │   ├── reviewer-qa/
│   │   ├── reviewer-spec/
│   │   ├── researcher-plan/    # post-split (software portion: emit_plan)
│   │   ├── researcher-gather/  # post-split (software portion: forward refs)
│   │   ├── reviewer-research/  # post-split (software portion: harness-gate)
│   │   └── ops-chain-observer/ # post-split (software-coupled framing)
│   └── web-research/           # new, when web-research chain ships
│       └── ...
```

Chain configs opt into overlays via a new field
(`persona_overlay_paths: ["overlays/software"]` or similar). The
loader is called multiple times: once for the shared root, then
once per overlay path. Last-writer-wins semantics already
supported by `persona.LoadFromDirectory`.

This is a meaty change (file moves + product-shell wiring + chain
config schema), worth scoping when there is concrete
web-research chain work to justify it. Pre-web-research, this
audit alone is the deliverable: it tells future contributors
what's harness vs software and pre-empts the next OSH-style drift.

## Defending against future drift (interim)

Until Phase 2b lands, two cheap signals would catch drift early:

1. **Contract test** (`test/contract/TestPersonaDomainAudit.go`):
   walk the fragment tree, classify each file by token-count
   heuristic, fail CI if a fragment classified as harness in this
   audit doc gains software-domain tokens. The doc itself is the
   spec; the test enforces drift detection at PR time.
2. **Audit-doc reference in CLAUDE.md.** Already linked from
   the post-PR-#164 opener via "shared persona corpus … stays
   domain-neutral by design"; add an explicit pointer to this
   doc once it merges.

Both are follow-ups; neither blocks the audit's value today.

## Pickup for Phase 2b

1. Decide whether overlay path is `configs/personas/overlays/<domain>`
   or `configs/personas/templates/<chain-id>` — the choice matters
   because overlay groups by domain (one overlay reusable across
   multiple chains) and template groups by chain (one chain, one
   directory). Domain grouping is more reusable; chain grouping is
   more discoverable. Lean: domain.
2. Add `persona_overlay_paths` field to the chain config schema.
3. Refactor `cmd/semteams/main.go:loadPersonaFragments` to accept
   a list of roots; iterate `persona.LoadFromDirectory` over them
   in order.
4. Execute the file moves per §"Recommendation" above.
5. Execute the splits per §"Findings #4" above.
6. Update contract tests to read the new layout.
7. Add a domain-leak contract test per §"Defending against future
   drift #1."
