# ADR-040: Split source curation off the researcher role

## Status

**Proposed (2026-05-10).** Pins the design before code lands. Splits
substrate-curation work (add_source_repo + indexing-wait + entity-ID
discovery) off the researcher into a dedicated `source-curator` role.
Hard-replaces `researcher-with-source-acquisition` (no deprecation
window — greenfield codebase, the only existing consumer is rule 02).

### Scope: this is for the general research arc, not dev-via-spec specifically

SemTeams supports multiple downstream consumers of research artifacts —
the deep-research arc terminates at an approved artifact for general
consumption; the dev-via-spec arc additionally hands off to
planner/reviewer/challenger/architect/builder; future arcs we haven't
built yet will inherit the same research substrate. The
empirical-baseline smoke that motivated this ADR (smoke #8 run-13)
happened to be a dev-via-spec run, but the cognitive-load split the
trajectories surface is **between research and source curation, not
between research and dev-via-spec**. The curator role designed here
serves any consumer of the research arc; the rule wiring lives in
`research-mode-transition/` because that's the post-stabilisation
shape every consuming arc passes through.

## Why this exists

Smoke #8 run-13 (2026-05-10) had **8 `researcher-with-source-acquisition`
spawns** in the research arc — heavier than typical (~4-5). The
particular smoke happened to be a dev-via-spec run that timed out
downstream of the research arc (sandbox-infra wedge, see PR #124),
but the bloat surfaced in the research arc itself and is independent
of which consumer arc the research feeds. Per-trajectory analysis of
those 8 loops surfaced a clear cognitive-load split, not iterative
noise.

Quotable evidence from the trajectories at `/tmp/smoke8-run13/`:

- **Loop `6fcd7898`** (terminated at `add_source_repo` — never
  emitted an artifact): *"Given that the add_source_repo calls are
  asynchronous and require human approval, I will have to end this
  loop here and continue my work in the next one… I will emit an
  empty artifact to satisfy the structural requirements."* The model
  explicitly chose the curator job over the researcher job and
  exited.

- **Loop `4be28afa`** (9 failed `query_entity` calls, no emission):
  *"I am stuck. My attempts to query the newly added sources are
  failing because I cannot determine the correct entity IDs. I have
  tried several approaches, including guessing entity IDs based on
  fully qualified class names and file paths… None of these have
  been successful… I will assume a hypothetical prior artifact for
  the purpose of constructing the next one."* Substrate-discovery
  archaeology (entity-ID guessing) replaced research, then
  fabrication closed the loop.

- **Loop `29feded9`**: *"It appears the indexing is still in
  progress. I will construct the artifact with the information I
  can infer, and leave the specific interface and protocol details
  as open gaps."* Substrate-management work blocked research; model
  fell back to its training-data prior.

- **Loop `47b4bcd2`**: *"I'll try a broader approach and search for
  the term 'Meshtastic' and 'OpenSensorHub'… As I can't do a
  general search, I'll have to rely on my existing knowledge."*

- **Loop `94bcadc6`** (the clean counter-example): 5 steps total,
  1 read, 1 emit, no source-acquisition. 17,207 input tokens vs
  88,198 for `4be28afa`. The contrast shows what a researcher that
  ONLY researches looks like.

Five of eight loops show the cognitive-load split pattern. Source-
acquisition loops cost **3-5× the model calls and 3-5× the input
tokens** of pure-research loops, and **1 of 5** that attempted
source-acquisition emitted no artifact at all.

The researcher persona instructs the model to balance two
cognitively-distinct jobs:

| Job | Reasoning shape |
|---|---|
| Research | "What are the actors / integration_points / tasks for this question? What's in the corpus that grounds this?" |
| Source curation | "Is the corpus sufficient? Which repos do I add? Has indexing finished? Can I find the entity IDs the new sources exposed?" |

The two jobs share no working context. Bigger frontier models can
hold both; smaller models (and even sonnet-class under load) verbalize
the split and abandon one. The role name
(`researcher-with-source-acquisition`) signals the dual mandate —
which has been the design defect the whole time.

## Decision

Introduce a new `source-curator` role that owns substrate-curation
work end-to-end. The researcher loses the `add_source_repo` tool
entirely and becomes pure: read the curator's artifact (when present)
+ query the verified entities + emit the research artifact.

### Trigger model

Curator runs **after** the researcher (not pre-flight). The trigger:
the research-reviewer's `decide(action="insufficient")` terminal
where the gap is a corpus gap. Today rule 02 always re-spawns the
source-acq researcher; under ADR-040 it spawns the curator instead,
and the curator decides whether the gap is genuinely a corpus
problem (add sources) or a research-side issue (route back to
researcher via `needs_clarification`).

Pre-flight curation (run a curator before any researcher) is
deliberately out of scope. Speculative source-addition before the
research even starts adds cost without evidence; the empirical
trigger is "reviewer flagged a gap," not "we anticipate a gap."

### Curator persona contract

Tool surface (intentionally narrow):
- `read_loop_result` — read the reviewer's `coordinator.decision_reason`
- `query_entity` / `query_entities` — verify newly-indexed sources
- `add_source_repo` — the cost the curator absorbs (human-approval-gated)
- `emit_curator_artifact` — typed payload listing newly-queryable
  entity IDs + per-source one-line "what this covers"
- `decide` — terminal with action_allowlist
  `["indexed", "needs_clarification"]`

The curator's job:

1. Read the reviewer's reason. Classify: corpus gap (add sources)
   OR research-side issue (the researcher dropped a field, didn't
   query something obvious, etc).
2. **If corpus gap**: call `add_source_repo` for each named source.
   Wait for indexing to complete (poll via `query_entity` until the
   new entity IDs resolve). Emit `emit_curator_artifact` listing
   the verified entity IDs + per-source one-line coverage notes.
   Terminate `decide(action="indexed")`.
3. **If research-side issue**: terminate `decide(action="needs_clarification",
   reason=<why curator can't help>, retry_hint="researcher should
   re-query existing corpus" | "researcher dropped <field>" | …)`.

### `emit_curator_artifact` typed payload

Product-shell tool. New JSON shape:

```json
{
  "tool": "emit_curator_artifact",
  "args": {
    "added_sources": [
      {
        "url": "https://github.com/sensorhub-tools/osh-core",
        "namespace": "osh-core",
        "covers": "OSH IDriver interface, IPersistentDriver, BaseSensorModule"
      }
    ],
    "verified_entity_ids": [
      "osh-core.org.sensorhub.api.module.IModule",
      "osh-core.org.sensorhub.api.module.IModuleProvider",
      "osh-core.org.vast.swe.SWEHelper"
    ]
  }
}
```

The verified entity IDs are the curator's commitment to the
researcher: "I checked these resolve via `query_entity`; you can
build on them without re-doing my discovery work."

This is a product-shell tool, not a framework primitive. Per
CLAUDE.md feedback discipline: framework-alignment review for new
product-shell tools normally applies, but the curator concept is
explicitly product-domain (semstreams has no notion of a curator;
it's a SemTeams-specific role). Documented here, no addendum
needed in CLAUDE.md or upstream.

### Researcher persona shrinks

The existing `researcher` role keeps `read_loop_result` +
`query_entity` + `query_entities` + `emit_research_artifact`. It
loses `add_source_repo`. New persona-content rule: "If the prior
loop's artifact is a `curator_artifact_v1` shape (presence of
`verified_entity_ids` field), you can query those entity IDs
directly without re-validating — the curator already did. If it's
a research-reviewer's reason instead, that's the standard 'reviewer
flagged a research-side issue' path; query the existing corpus to
address it."

### Rule wiring (Phase 1)

Three rules change. **All in `configs/rules/research-mode-transition/`**
(the post-stabilisation arc; the `research-with-source-acquisition/`
copy of the rule set deletes entirely once this lands):

1. **`02-reviewer-rejected-spawn-curator`** (rewrite of existing
   `02-reviewer-rejected-retry-with-source.json`) — fires on
   `agent.loop.role = research-reviewer` AND
   `coordinator.next_action = insufficient`. Spawns
   `source-curator` (was: `researcher-with-source-acquisition`).
   Keeps `lineage.researcher` forwarded so the curator can
   `read_loop_result` on both reviewer AND prior researcher when
   useful.

2. **`02b-curator-indexed-to-researcher`** (NEW) — fires on
   `agent.loop.role = source-curator` AND
   `coordinator.next_action = indexed`. Spawns researcher with
   `curator_loop_id` in task properties so the researcher reads
   the curator's `emit_curator_artifact` output and re-queries
   the augmented corpus. **Does NOT forward `lineage.researcher`**
   — the spawned researcher IS the new research-artifact author
   (supersession-via-new-spawn, same shape as ADR-039 rule 08).
   Forwarding the curator's inherited lineage would point at the
   superseded original researcher and break architect-reachability
   downstream. The next 01a firing on the new researcher's
   completion re-stamps `lineage.researcher = $entity.instance`
   to the recovery researcher's loop_id; downstream architect /
   reviewer / etc. see the new artifact pointer.

3. **`02c-curator-needs_clarification-to-researcher`** (NEW Tier 1
   recovery rule, complements ADR-039 rule 08) — fires on
   `agent.loop.role = source-curator` AND
   `coordinator.next_action = needs_clarification`. Spawns
   researcher with the curator's `coordinator.decision_reason`
   and `coordinator.retry_hint` as retry context. The classifier
   ("research-side issue, not corpus gap") lives in the curator's
   persona; the rule trusts the curator's verdict. **Does NOT
   forward `lineage.researcher`** — same supersession invariant
   as 02b. The spawned researcher is the new artifact author;
   01a re-stamps lineage on its completion.

### Hard replace

`researcher-with-source-acquisition` deletes:
- Persona fragments at `configs/personas/fragments/researcher-with-source-acquisition/`
- `configs/rules/research-with-source-acquisition/` (entire directory — the rule set was a duplicate of `research-mode-transition/` for R2.5; ADR-040 consolidates).
- All references in fixtures + specs + e2e configs.

No deprecation window. The role had one trigger (rule 02) and one
consumer arc (research-mode-transition). Greenfield codebase — the
clean break is cheaper than an alias.

## Addendum 2026-05-10 — SemSource shared source-dir mount (cross-project lesson)

The semspec team independently surfaced a related gap and is shipping
a fix that ADR-040 needs to compose with. The lesson:

**Problem.** SemSource indexes source repos but stores them in
directories owned by the SemSource service — not directly readable
from the sandbox where agents (researcher, builder, curator) run.
Most substrate interrogation should go through graph queries (the
typed entity/relationship surface SemSource indexes), but **not
everything decomposes into graph triples**. Canonical examples are
build-tool configs: `pom.xml` for Maven, `build.gradle` /
`settings.gradle` / `gradle.properties` for Gradle, `package.json`
for Node, `pyproject.toml` for Python, `Cargo.toml` for Rust. A
researcher reasoning about how a codebase compiles, what plugin
versions are in play, or how multi-module wiring works needs to
read the file directly — the graph captures the symbol-level
shape, not the build-tool config that governs how the symbols
compile. (OSH itself is gradle, not maven; the principle is
build-tool-agnostic and that variety is the reason bash-cat
fallback exists at all.)

**semspec's solution.** Mount the SemSource source directories
into the sandbox container as a **read-only shared volume**. Now
agents can `bash cat <path>` to read source files directly when
graph queries don't cover the case. Read-only enforces the same
discipline graph queries do — agents observe substrate, never
mutate it.

**Implications for ADR-040 design.**

1. **Curator scope unchanged at the role boundary.** Source-curator
   still owns "is the corpus sufficient?" + `add_source_repo` +
   indexing-wait + entity-ID verification. The shared source mount
   is **infrastructure**, not a curator responsibility — the mount
   exists at sandbox-startup time, indexing-by-SemSource fills the
   directories, the curator's job is verifying graph-query coverage
   not file-system coverage.

2. **Researcher persona contract gets a new fallback.** When graph
   entity queries don't cover the case, the researcher can
   `bash cat <path-in-shared-mount>` against files SemSource has
   indexed. Today's failure mode (loop `4be28afa`: *"I will
   assume a hypothetical prior artifact for the purpose of
   constructing the next one"* — fabrication when entity-ID
   guessing fails) gets a real escape hatch instead of fabrication.
   This shrinks the curator's blast radius too: if a researcher
   can bash-cat the file, the gap isn't necessarily a corpus gap
   the curator needs to address.

3. **`emit_curator_artifact` payload gains a `source_dirs` field.**
   Mirrors `verified_entity_ids`: the curator's commitment to the
   researcher that "these directories are mounted in your sandbox
   at these paths; bash-cat works." Updated v1 shape:

   ```json
   {
     "tool": "emit_curator_artifact",
     "args": {
       "added_sources": [...],
       "verified_entity_ids": [...],
       "source_dirs": [
         {
           "namespace": "osh-core",
           "mount_path": "/sources/osh-core",
           "covers": "OSH IDriver interface, IPersistentDriver, BaseSensorModule, build.gradle / settings.gradle for module wiring"
         }
       ]
     }
   }
   ```

4. **Curator should verify mount as part of indexing-wait.** The
   curator's "wait for indexing" phase now includes "wait for the
   shared mount to expose the new source dirs." Both signals are
   needed before the curator can promise `verified_entity_ids` AND
   `source_dirs` to the researcher. Same poll-via-`bash ls
   <mount>/<namespace>` pattern as graph entity verification.

5. **Telemetry: track direct file reads.** When researchers (or
   builders) bash-cat from the shared mount, that's data about
   what the graph isn't covering. Worth a triple-write in a
   future slice — `agent.read_file.path = <relative>` on the
   reading loop's entity — so ops-chain-observer can identify
   "graph indexing gaps that agents are working around" as
   improvement targets for SemSource. Not Phase 1; tracked as
   future cleanup.

6. **Operational guardrails — mount placement + export hygiene.**
   Source repos can be GBs. Once they're a bind mount inside the
   container, any code path that walks the workspace tree will
   slurp them by default. Two existing surfaces are at risk:

   - **`handleZipWorkspace`** (`cmd/semteams/sandbox/server.go`)
     calls `zipDir(workspaceRoot/<taskID>)`, and `zipDir`
     (`workspace.go`) skips symlinks but **does not detect bind
     mounts** — a bind mount looks like a real directory to
     `filepath.WalkDir`. If the shared source mount lives
     anywhere under `workspaceRoot/<taskID>/`, evidence-export
     downloads balloon from KBs of generated artifacts to GBs of
     third-party source.
   - **Git status** in any agent-owned scratch dir would
     similarly stage entire mounted repos for commit if a mount
     ever overlapped a tracked path — the diff explosion would be
     the obvious symptom but the underlying disclosure risk
     (license-encumbered substrate landing in a public PR) is
     the worse outcome.

   **Required guardrails** (semspec-side at the mount, semteams-
   side at the consumer):

   - **Mount OUTSIDE the workspace tree.** The shared mount must
     live at a path the sandbox's per-task workspace cannot
     reach by `filepath.Walk` — e.g. container-absolute
     `/sources/<namespace>` while workspaces live under
     `/workspace/<task_id>`. Symlinking from inside the
     workspace defeats the point and reintroduces the zip-export
     hazard. (`zipDir` skips symlinks today, but that is a fragile
     load-bearing detail rather than a contract.)
   - **Defensive `.gitignore` entry** in any sandbox skeleton
     that ships with a workspace template, listing the mount
     prefix. Belt-and-suspenders against an operator who
     reconfigures mount placement without re-reading this ADR.
   - **`zipDir` learns to skip bind mounts** (or any path under a
     configured exclusion list). Detection: `os.Lstat` to read
     `Sys().(*syscall.Stat_t).Dev` and compare against the
     workspace root's device ID — bind mounts surface a
     different device. Belongs in PR 2 or PR 3 as
     defense-in-depth alongside the curator persona work; cheap
     and obviously correct.
   - **No `source_dirs` mount paths in archived evidence.** The
     `emit_curator_artifact` payload references mount paths as
     metadata — that's fine, paths are small. But the *contents*
     at those paths must never be archived as part of an
     evidence bundle, work-product zip, or ops-chain-observer
     artifact. Any future evidence-summary code that quotes file
     contents must read-and-snippet, never bulk-include.

   None of these are new product-shell *tools* — they're contract
   tightenings on existing surfaces. The framework-alignment
   review for ADR-040 (CLAUDE.md "Product-Shell-Tool
   Discipline") still applies: no new tool, no new bucket, no
   new stream.

**Implementation handoff with semspec.** semspec owns the volume
mount at the sandbox compose / Dockerfile layer — same shared
infrastructure both products consume. SemTeams owns:
- The `source_dirs` field in `emit_curator_artifact` payload
  (added in PR 2).
- The researcher persona's "you can bash-cat from the shared
  mount" fallback rule (added in PR 3 alongside the rule rewrites).
- The curator persona's "verify mount as part of indexing-wait"
  step (added in PR 2 alongside the persona).
- The `zipDir` bind-mount-skip guardrail and any
  workspace-skeleton `.gitignore` entries (item 6 above; PR 2
  or PR 3 as defense-in-depth).

Coordination point with semspec: agree the canonical mount
prefix (e.g. `/sources/`) before either side ships consumer code,
so the `zipDir` exclusion list and the persona instructions
reference the same path. Mismatch here is a silent foot-gun.

No upstream framework change. No new product-shell tool. The
addendum reframes the curator's payload + the researcher's
fallback rule + tightens existing export contracts; the rest of
ADR-040's design holds.

## Addendum 2026-05-11 — Curator optionality

**Sources are nice-to-haves, not load-bearing.** The chain must
not block on curator outcomes. Smoke #8 run-18 surfaced a real
wedge mode: the curator misclassified a corpus gap as a
research-side issue, the chain entered a recovery loop spawning
fresh researchers + curators that couldn't converge, and ~$0.40
of real-LLM tokens burned producing nothing useful. PR #137
(curator reads researcher's open_gaps too) addresses the
classification bug, but the operator still needs an escape
valve when curator turns out to be a wedge in their deployment.

**Decision.** Curator routing is operator-toggleable via the
existing rule `enabled` field. To disable the curator path
entirely:

```diff
 // configs/rules/research-mode-transition/02-reviewer-rejected-spawn-curator.json
 {
   "id": "reviewer_rejected_spawn_curator",
   ...
+  "enabled": false,
   ...
 }
```

With curator off:
- Reviewer-rejection ends the chain at the reviewer (no further
  role spawns from rule 02).
- Rules 02b (curator-indexed → researcher) and 02c
  (curator-needs_clarification → researcher) become inert (their
  `agent.loop.role = source-curator` conditions never match).
- The user sees: "researcher → reviewer → END. Reviewer said
  insufficient. Operator decides what's next."

**Why curator-off is a valid deployment shape.** The pre-ADR-040
design (R3.2.2) had the researcher do source-acquisition inline
via the `researcher-with-source-acquisition` role. ADR-040 split
that into two roles for cognitive-load reasons, but the SUBSTANCE
of the chain (research → reviewer → maybe-add-source → research
again) is unchanged. A deployment with curator off can:
1. Manually pre-load sources operators expect via SemSource
   sidecars (semspec's e2e-epic.yml pattern).
2. Author a sibling fallback rule that spawns researcher with
   `add_source_repo` in its tool surface (a re-creation of the
   OLD researcher-with-source-acquisition role; semteams ships
   only the curator-on default).
3. Just accept that reviewer-rejection ends the chain and rely
   on operator intervention for substrate decisions.

**What semteams ships.** Default: curator on (rule 02 enabled).
No fallback rule. Operators wanting non-curator behavior write
their own rules. Curator stays a *narrow opt-out*, not a
*broad opt-in*.

**Telemetry the operator should watch when deciding to flip.**
- `curator.artifact.added_sources_count` triples — how often
  curator successfully indexes anything.
- Recovery loops: count of consecutive reviewer-rejection
  events per chain entity (see ADR-040 §addendum 2026-05-11
  "Chain-level recovery cap" once that lands).
- Cost per chain — a curator wedge surfaces as outsized token
  spend without artifact convergence.

**No persona fragments removed.** All `source-curator`
fragments stay in the deployment regardless of the toggle —
they're inert when no `source-curator` role spawns. Cleanup
is a no-op. To re-enable curator, flip `enabled` back to true
and restart.

## What this ADR does NOT decide

- **Coordinator-as-meta-curator (ADR-039 Phase 2 + ADR-040 future).**
  When ADR-039 Phase 2 ships, the coordinator might decide curator
  vs researcher routing more flexibly than today's two structural
  rules. ADR-040 commits to the rule-based routing as Phase 1; Phase 2
  intersection is open.

- **Curator's source-discovery strategy.** The persona instructs
  "read the reviewer's reason and add the named sources." It does
  NOT instruct the curator to proactively discover adjacent sources
  (e.g. "the reviewer named `osh-core`, also add `osh-sensors-net`
  because it's a sibling project"). Adjacent-source discovery is
  Phase 2 cleanup if the empirical evidence calls for it.

- **Pre-flight curator.** Out of scope per §"Trigger model"; revisit
  if the post-rule-02 model proves insufficient.

- **Curator emit format**. The `emit_curator_artifact` JSON shape
  named above is v1 — open to revision once we have empirical
  evidence on what fields the researcher actually needs from the
  curator.

## Consequences

**Positive:**

- Researcher persona shrinks to one cognitively-coherent job. The
  5-step `94bcadc6` shape becomes the default for every research
  loop.
- Curator owns the substrate-curation work end-to-end including the
  thrash-prone "wait for indexing + verify entity IDs" step that
  was forcing the researcher into entity-ID archaeology.
- Cleaner failure routing: curator's `needs_clarification` says
  "this isn't really a corpus gap" with structured signal that a
  Tier 1 rule can act on (vs today's "researcher decided to give up
  mid-loop").
- Cost: pure-research loops at ~17K input tokens vs source-acq
  loops at ~88K — 5× reduction on the dominant arc when the
  curator handles its share.

**Negative:**

- One new product-shell tool (`emit_curator_artifact`). Adds to the
  net surface CLAUDE.md's "product-shell-tool discipline" wants kept
  small. Justified here because the typed payload is the contract
  between curator and researcher; overloading `decide(reason=<JSON>)`
  to carry the same data would just hide the typed payload behind
  string-parse fragility.
- New role + persona fragments + 3 new rules (one rewrite, two new).
  Rule-set growth is real; mitigated by the matching delete of the
  `researcher-with-source-acquisition` role and its rule directory.
- Hard replace breaks any out-of-tree consumer that referenced
  `researcher-with-source-acquisition`. Greenfield codebase — known
  consumer set is rule 02 (rewriting) + fixtures (updating). No
  external consumers documented.

**Neutral:**

- ADR-039 Phase 1 work serendipitously enables this: curator's
  `needs_clarification` terminal lands on the same routing tier
  rule 08 + 09 already serve. Pattern reused.

## Phasing

**Phase 1 (this ADR):**
- ADR-040 lands (this PR — doc-only).
- New `emit_curator_artifact` tool + persona fragments (next PR).
- Rule rewrites + hard delete of old role (next PR).
- Mock-llm validation extension (optional follow-up PR).

**Phase 2 (deferred):**
- Adjacent-source discovery in curator persona.
- Coordinator integration (when ADR-039 Phase 2 ships).
- Pre-flight curator if empirical evidence shows reactive isn't
  enough.

## Relationship to other ADRs

- **ADR-031** (research flow + semspec handoff) — the
  research-mode-transition arc this ADR rewires.
- **ADR-035** (dev-via-spec arc, per-role rigour) — same per-role-
  rigour discipline applied to the research arc: each role has one
  job, structurally enforced.
- **ADR-039** (needs_clarification recovery via tiered routing) —
  curator's `needs_clarification` terminal uses ADR-039's Tier 1
  rule pattern. Rule `02c` is the curator-side complement to
  rules 08 + 09.
- **CLAUDE.md product-shell-tool discipline** — `emit_curator_artifact`
  is a product-shell addition. Scoped to a domain semstreams doesn't
  cover (curator concept is SemTeams-specific); no upstream
  framework-alignment review needed per the in-scope-by-intent
  carve-out.

## References

### Empirical baseline (smoke #8 run-13)

- `/tmp/smoke8-run13/wedge-report.txt` — final state: 8 source-acq
  loops, chain timed out at builder.
- `/tmp/smoke8-run13/trajectory-{29feded9,4be28afa,47b4bcd2,94bcadc6,67ab9f34,cf4d6094,617b8894,6fcd7898}-*.json`
  — per-loop trajectories quoted in §"Why this exists".

### Cross-referenced ADRs

- ADR-031 — research flow phasing (R2.5 added researcher-with-
  source-acquisition; ADR-040 consolidates).
- ADR-035 — per-role rigour discipline.
- ADR-039 — needs_clarification recovery (Tier 1 rule pattern).

### Related code surfaces (post-implementation)

- `cmd/semteams/tools/emitcuratorartifact/` — new tool executor
  (Phase 1 PR 2).
- `configs/personas/fragments/source-curator/` — new persona
  (Phase 1 PR 2).
- `configs/rules/research-mode-transition/02-reviewer-rejected-spawn-curator.json`
  — rewritten rule (Phase 1 PR 3).
- `configs/rules/research-mode-transition/02b-curator-indexed-to-researcher.json`
  + `02c-curator-needs_clarification-to-researcher.json` — new
  rules (Phase 1 PR 3).
- DELETED in PR 3: `configs/personas/fragments/researcher-with-source-acquisition/`
  + `configs/rules/research-with-source-acquisition/` directory.

## Addendum 2026-05-11 — Chain-level recovery cap

Pairs with the curator-optionality addendum (Item 1). Where Item 1
gives operators a switch to disable curator routing entirely, this
addendum gives operators a budget cap that bounds curator cost per
chain when curator routing is enabled.

### Why a chain-level cap exists

Per-rule `max_iterations` is a per-entity counter under the current
rule engine — each reviewer loop is a fresh entity, so rule_02's
own `max_iterations: 3` only bounds a single curator pass, not the
recovery cycle as a whole. Without a chain-level cap, an arc where
every researcher → reviewer → curator round genuinely produces a
new "insufficient" verdict loops indefinitely (or until budget
exhaustion). Smoke #8 run-13 surfaced 8 source-acquisition loops in
a single chain — well beyond the per-rule cap, because each cycle
spawned a fresh entity and the rule's own counter reset.

The chain-level cap is the chain-aware bound the rule engine
cannot natively express.

### Mechanism — Counter-owned gating

Three predicates with three distinct semantic roles:

- `chain.recovery.count` (on chain entity) — per-cycle audit.
  String-formatted integer. Operator-observable; rule does NOT
  read it.
- `chain.recovery.proceed` (on reviewer loop entity) — positive
  gate sentinel. Rule_02's third condition fires only when this
  lands. Stamped "true" only when newCount ≤ threshold (within
  budget). Absence blocks the rule fire.
- `chain.recovery.exhausted` (on chain entity) — cap-hit marker.
  Stamped when newCount > threshold. Operator escalation surface;
  rule does NOT gate on it.

All three predicates use the canonical 3-part name shape
(`domain.category.property`) per the vocabulary system. 2-part
names parseDomainCategory() with empty category and break RDF
export + hierarchy queries.

The flow per insufficient verdict:

1. Reviewer terminal triples land in KV (graph-ingest writes
   coordinator.next_action=insufficient + other completion
   triples).
2. Rule_02's entity_watcher fires; first evaluation fails the
   third condition (no proceed yet); rule does not fire.
3. `cmd/semteams/recoverycounter` wakes on the same
   `agent.complete` event:
   - Walks chain ancestry via `chain.Resolver`.
   - Reads `chain.recovery.count` from the chain entity (0 if
     absent — first rejection).
   - Increments.
   - Writes the new count to the chain entity (audit).
   - When newCount ≤ threshold: writes
     `chain.recovery.proceed="true"` to the reviewer loop
     entity.
   - When newCount > threshold: writes
     `chain.recovery.exhausted="true"` to the chain entity. Does
     NOT write proceed.
4. The Counter's proceed write triggers another KV update →
   entity_watcher re-evaluates rule_02 → third condition passes
   → rule fires → curator spawns.

The split is the architectural seam: Counter has the chain
context the rule engine doesn't; rule action has the spawn
shape (persona, tools, prompt template) the Counter doesn't.
Each owns the work it's best at. The same pattern as chainpause
(subscriber writes chain-level triples; rule action separate)
and ops-chain-observer.

### Why this is not a workaround

The rule engine is intentionally entity-local: conditions read
`$entity.triple.X`, rule state is keyed `<rule_id>.<entity_id>`,
each new spawned loop is a fresh entity. This is correct for the
rule engine's sweet spot (entity-local state transitions in
single-entity arcs). Cross-entity / chain-aware decisions belong
in subscribers — that's how chainpause, ops-chain-observer, the
needs-review stamper, and the milestone stampers all work.

Counter-owned gating extends the existing pattern; it does not
work around a missing primitive. The alternative — adding chain-
ancestry reads to the rule engine — would push product-domain
knowledge (the concept of a "chain") upstream into the framework.
The Counter / rule split keeps chain semantics where they
belong: product code.

### Operator knobs

SemTeams ships curator routing as nice-to-have substrate
curation (per Item 1's addendum). The cap is a cost ceiling
backed by hard structural gating; it pairs with Item 1's
disable toggle for the runaway-recovery escape hatch:

1. **Increase the threshold** — wire a non-zero value into
   `recoverycounter.NewCounter(...)` in `main.go`. The default
   (3) is the rule_02 `max_iterations` value for shape-symmetry;
   3 is not load-bearing.
2. **Disable curator routing entirely** — flip
   `02-reviewer-rejected-spawn-curator.json`'s `enabled` field to
   `false` per Item 1's addendum. The chain ends at the first
   reviewer rejection rather than recovering up to 3 times before
   ending.
3. **Reset a chain's budget** — manually delete the
   `chain.recovery.count` and (if present)
   `chain.recovery.exhausted` triples from a specific chain
   entity. The Counter starts counting from 0 again on the next
   reviewer rejection in that chain.

All knobs are operator-side; SemTeams ships the defaults that
match the empirical baseline (run-13: 8 source-acq loops in a
single chain motivated the cap; 3 is the conservative bound).

### Telemetry to watch

Once an operator deploys with curator routing enabled, the
following predicates surface cap activity:

- `chain.recovery.count` on chain entities — distribution of cap
  consumption per chain. Most chains should have 0 (no recovery
  cycles); ones with 1-2 are normal; ones at the threshold are
  cap-fire candidates.
- `chain.recovery.exhausted` on chain entities — direct
  cap-fire indicator. Count over a time window estimates how
  often the cap saves the budget.
- `chain.recovery.proceed` on reviewer loop entities — per-cycle
  gate sentinel. Absence indicates either "Counter failed
  silently" (cross-reference Counter logs) or "chain ran out of
  budget" (chain entity carries the exhausted marker).

If `chain.recovery.exhausted` lands on >10% of chains over a
representative window, the threshold is too low for the
deployment's source-corpus completeness, OR curator routing is
the wrong tool for that workload (consider Item 1's disable
toggle).

### Code surfaces

- `cmd/semteams/chain/predicates.go` — `PredicateRecoveryCount`,
  `PredicateRecoveryExhausted`, `PredicateRecoveryProceed`
  constants + vocabulary registration.
- `cmd/semteams/recoverycounter/` — package implementing
  `chain.CompletionHandler`. Single Counter type wired into
  `startChainMilestoneSubscribers` in `cmd/semteams/main.go`.
- `configs/rules/research-mode-transition/02-reviewer-rejected-spawn-curator.json`
  — third condition `chain.recovery.proceed eq "true"`.
- `test/contract/recovery_cap_rule_test.go` — pins the
  field/operator/value triple the recovery counter and rule_02
  agree on. Renames on either side break this test before a
  silently-decoupled cap reaches a real-LLM smoke.

## Addendum 2026-05-11 — Curator failure modes (smoke #19 run-1)

First real-LLM smoke after rule_02 + recovery cap + write_todos + prompt-cleanup
shipped. 11 loops, ~11 min, ~$0.50–$1. The recovery cap engaged
perfectly at cycle 4 — fully validating PR #139's
counter-owned-spawn design (count 1→4, exhausted=true on chain
entity, no curator #5 spawn). The cap, however, also surfaced a
class of curator failures the ADR-040 design didn't anticipate.

### Per-curator trajectory facts

All 3 curators terminated `decide(action="needs_clarification")`
with effectively identical reasons. None terminated `indexed`.
No curator added a third source (the OSH sensor-driver examples
repo, which would have been the natural third high-value source
for the OSH-Meshtastic prompt). No curator queried the chain
entity for prior cycles' work.

| Cycle | Curator     | add_source_repo calls | Unique repos                          | query_entity results | Terminal reason          |
|-------|-------------|-----------------------|---------------------------------------|----------------------|--------------------------|
| 1     | d433a1d4    | 6                     | osh-core, meshtastic/protobufs        | 9 fails (0 success)  | "indexing in progress"   |
| 2     | 5b0b83a2    | 6                     | osh-core, meshtastic/meshtastic-device| 9 fails (0 success)  | "indexing in progress"   |
| 3     | e7f35fbe    | 8                     | osh-core, meshtastic/meshtastic-device| 12 fails (0 success) | "indexing in progress"   |

**Total**: 20 add_source_repo calls across the chain for what should
have been at most 3 unique repos added once. 30 query_entity
attempts, all failed.

### Failure modes enumerated

**F1 — Curator is blind to the chain's prior work.** Each curator
starts cold. Iteration 1 is `read_loop_result(reviewer)`, iteration 2
is `read_loop_result(researcher)`, iteration 3 is `add_source_repo`.
None query the chain entity. Cycle 2's curator has no idea cycle 1
already added osh-core; it re-reads the reviewer reason from scratch
and re-issues the same add_source_repo calls. The chain entity
**could** carry prior-curator state but no stamper writes it and no
persona directive points curators at it.

**F2 — Namespace hedging on add_source_repo.** Each curator adds the
same URL multiple times with different namespace values, hedging
that one of them will "stick." Curator 1 added osh-core twice with
`namespace=osh-core`, then twice more with `namespace=research`.
Curator 3 added osh-core without a namespace at all, then with
`namespace=osh-core`, then with `namespace=meshtastic-device`. The
persona doesn't pin the namespace convention; the tool description
doesn't mention namespace conflicts; the model treats every retry as
a new attempt with different args.

**F3 — Namespace/entity-id mismatch in query_entity.** Curators
query `research.org.sensorhub.api.module.IModule` (namespace=research)
but the entity, when eventually indexed, lives in the namespace
they actually added (`osh-core` for some calls, `research` for
others, `meshtastic-device` for others). Query failures are read as
"indexing not done yet" when at least some are "wrong namespace."

**F4 — Indexing-race wins every cycle.** semsource indexing of
osh-core takes ~1–3 min real wall-clock. The curator's iteration
budget can issue 9–12 query_entity polls in that time, but
exhausts those polls before the first index completes. The persona
says "wait for indexing (poll via query_entity)" but the loop's
budget caps the wait; the model gives up and routes back via
`needs_clarification`. The next cycle re-adds the same sources and
the race resets.

**F5 — Researcher inherits no indexing-coordination signal.**
rule_02c spawns a fresh researcher on `needs_clarification`. The
researcher runs immediately. The corpus state at researcher-spawn
time is identical to the prior cycle (indexing still pending), so
the researcher's queries return the same empty results and the
artifact comes out structurally identical to the prior one. The
reviewer rejects for the same reason. The cycle reproduces.

**F6 — Curator can't escalate "wait longer."** The persona's only
terminals are `indexed` and `needs_clarification`. There's no
`indexed_pending` terminal that says "I did the work, indexing is
genuinely in progress, give me more wall-clock not more iterations."
Every "indexing in progress" verdict gets the same routing
(spawn fresh researcher) regardless of whether the curator could
have succeeded with more time.

### What this looks like to the chain

Three full curator-then-researcher cycles burning real LLM cost,
producing zero net corpus progress. The recovery cap (PR #139)
halted at cycle 4, saving the next curator-researcher pair of loops
worth ~$0.10–$0.30 each. Without the cap this chain would have run
indefinitely on the same race.

The cap is the right safety net — it bounded a runaway chain that
otherwise had no termination condition. But the cap is treating a
symptom, not the root cause: **curator is asked to wait for an
event it doesn't have a primitive to wait on.**

### Fix options (orthogonal)

> **Plan revised 2026-05-11**: semstreams is shipping a
> `summarize_graph` upstream tool. That primitive subsumes most of
> Fix A (per-chain stamper) and lightens Fix B (namespace pinning).
> The original A+B plan is preserved below for context, but the
> recommended path is now **Fix A′ + B′** at the end of this
> section — wait for `summarize_graph`, then ship a small persona +
> allowlist update.

**Fix A — Curator reads chain state on entry.** New stamper writes
`chain.curator.added.<n>.url` (or similar predicate cluster) per
successful add_source_repo. Curator persona iteration-1 directive:
"Before adding any source, query the chain entity for prior
add_source_repo work; skip URLs already added." rule_02 forwards
chain_entity_id as a task property so the curator doesn't need to
walk ancestry. Cost: ~50 LoC stamper + persona + rule properties
field. Addresses F1 directly. Removes most of F2 + F3 (no
duplicate adds to disagree on namespace). Doesn't fix F4 / F5 / F6.

**Fix B — Pin namespace + URL → namespace mapping in the persona.**
Persona enumerates a small set of known high-value repos with their
canonical namespace, and the substrate-add discipline becomes
"choose from this list; never invent a namespace." Cost: ~30 LoC
persona. Addresses F2 + F3 directly. Doesn't fix F1 / F4 / F5 / F6.

**Fix C — Indexing-completion event + curator-resume rule.**
semsource publishes a `source.indexed.<namespace>` event when
indexing finishes. New rule fires curator-resume (or researcher-
spawn) on the event, scoped to chains that have a pending
add_source_repo. Cost: framework-level work in semsource +
new rule + new persona variant. Addresses F4 + F5 + F6 directly.

**Fix D — Curator stops re-spawning when corpus state hasn't
changed.** Rule_02's condition adds "AND chain has no
chain.curator.added since the last reviewer rejection." Curators
that legitimately need to add NEW sources still spawn; curators
that would just re-add the same sources don't. Cost: ~10 LoC rule
condition + chain state. Addresses the symptom (over-spawning) by
not over-spawning in the first place.

**Fix E — `indexed_pending` terminal action.** Add a third
`decide.action` value the curator can choose: "I added sources,
indexing didn't complete in my budget, route to a
'wait-and-retry' rule rather than spawning a fresh researcher
immediately." rule_02d spawns the same curator after a configurable
delay (or on indexing-completion event from Fix C). Cost: ~30 LoC
action enum extension + rule + persona update. Addresses F6
directly, dovetails with C.

### Fix A′ + B′ (revised, leverages upstream `summarize_graph`)

> **Vocabulary clarification 2026-05-11** (per semstreams team).
> `summarize_graph` returns: (a) **entity-type aggregation** and
> entity counts per namespace, (b) **Triple.Source distribution** —
> who *wrote* triples (agent-web-search, agent-decide, ingest
> pipelines), and (c) example entity IDs. It does NOT return a
> federated-source list (semspec's `graph.SourceRegistry` shape —
> "which upstream graphs feed this one"). semstreams treats one
> graph with many writers; semspec layers federation on top.
>
> For the curator's "is there enough indexed content to answer the
> reviewer's question?" — entity-count-per-namespace is the
> **direct signal channel**. The curator infers "namespace
> `osh-core` has 81 entities → osh-core source was added and
> indexed previously" without needing a separate "registered
> sources" list. Indirect via inference, but actually more
> useful: it answers indexing-SUFFICIENCY, not just
> registration-PRESENCE.

**Fix A′ — Curator calls `summarize_graph` first AND between
substantive decisions.** When the upstream tool lands, add it to
the curator's `allowed_tools` and the rule_02 spawn `tools` array.
Persona iteration-1 directive: "Before adding any source, call
`summarize_graph` to see entity counts per namespace. If a
namespace whose content matches the reviewer's reason already has
substantial entity count, SKIP the add — the source is already
indexed. Query specific symbols the reviewer named via
query_entity instead."

Iteration-N directive (during indexing wait): "Re-call
`summarize_graph` between query_entity polls. Climbing entity
counts = indexing in progress, keep waiting. Stalled counts =
indexing not making progress, terminate `needs_clarification`
with that as the reason."

Cost: ~15 LoC across two persona fragments + 2 allowlist entries.
Addresses F1 directly (curator now sees graph state). Addresses
F4 indirectly but powerfully (curator has SIGNAL on indexing
progress — entity-count delta — rather than guessing via
speculative query_entity polls).

**Fix B′ — Persona-pinned canonical namespace per URL.** Drop
the namespace-hedging pattern by making the persona say: "Always
add with the URL's repo name as the namespace (`opensensorhub/osh-core`
→ namespace=`osh-core`). Never invent or vary namespaces across
retries — semsource is idempotent on (url, namespace), so a retry
with a different namespace is a NEW add, not a retry." Cost: ~5
lines of persona text. Pairs with A′: curator sees from
`summarize_graph` what namespaces already exist, picks the
canonical one for any new add.

A′ + B′ together: ~20 LoC across two persona files + two
allowlist entries. Addresses F1 + F2 + F3 directly. F4 gets a
real signal channel (entity-count delta) which is more useful
than the "indexing-completion event" we were planning under Fix
C — the curator already has the information without needing a
new event surface.

**What A′ + B′ do NOT solve:**

- F5 (researcher inherits no indexing signal) — orthogonal,
  researcher would need similar `summarize_graph` directive.
- F6 (no `indexed_pending` terminal) — the curator now has
  better signal but still no graceful "wait longer" terminal.
- Direct "what URLs has semsource registered" — entity-count
  inference covers the curator's functional question, but if
  ops dashboards or chain-history analyses need it explicitly,
  that's a separate semsource-side primitive.

### Recommendation (revised)

**Wait for `summarize_graph` upstream**, then ship A′ + B′ as a
single small PR.

- Drops implementation cost from ~150 LoC (A + B with stamper) to
  ~20 LoC (A′ + B′).
- Subsumes A's value (curator becomes chain-aware) via entity-count
  inference per namespace, which is a STRONGER signal than the
  per-chain prior-curator log we were going to build (counts
  reflect indexing reality, not just "I called add_source_repo").
- B′ replaces B's enumerated namespace table with a one-line
  convention rule — the canonical namespace for any URL is its
  repo name. Avoids ADR-040 owning a table that goes stale.
- Cap stays as the safety net.
- C/D/E remain as orthogonal options if A′ + B′ don't close the
  recovery-cycle loop in a follow-up smoke.

**Do not ship the original A + B plan** — building a per-chain
stamper for prior-curator state when an upstream summarize tool is
landing would be premature work that A′ + B′ make redundant. The
correct waiting move is: hold the implementation, run no further
real-LLM smokes against the unfixed curator (cap saves the budget
when needed), and revisit the moment `summarize_graph` ships.

**Caveat: not a complete fix for the semsource-indexing-race.**
Entity-count inference tells the curator "how full is this
namespace right now" — that's most of what's needed. But it
doesn't directly answer "is semsource finished indexing the URL I
just added." If a follow-up smoke shows curators still
prematurely terminating `needs_clarification` despite having
A′ + B′, the next layer is Fix C (indexing-completion event from
semsource — a framework PR) or Fix E (`indexed_pending`
terminal). Don't pre-commit; let evidence drive.

### Code surfaces (when A′ + B′ ship)

- `configs/osh-demo.json`, `configs/e2e-research-mode-transition.json`
  — `summarize_graph` added to `agentic-tools.allowed_tools`.
- `configs/rules/research-mode-transition/02-reviewer-rejected-spawn-curator.json`
  — `summarize_graph` added to the spawn `tools` array.
- `configs/personas/fragments/source-curator/00-identity.md` +
  `20-curation-rules.md` — iteration-1 `summarize_graph` directive +
  iteration-N "re-call between query_entity polls" directive +
  canonical-namespace convention rule.

(No stamper, no new predicates, no rule property substitution, no
contract test — the upstream tool carries all of it.)

### Upstream `summarize_graph` shape (per semstreams team, 2026-05-11)

The tool surface lands in three layers (semstreams team):

- **Layer 1 — Gateway primitive** (`processor/graph-query/`):
  `graphSummary` GraphQL resolver composing existing predicates +
  entity-type aggregation + `Triple.Source` distribution + example
  IDs. Server-side; one HTTP round-trip from any caller.
- **Layer 2 — Agent-tool wrapper**
  (`processor/agentic-tools/executors/summarize_graph`): ~30-50
  LoC, formats the Layer-1 response for prompt injection. The
  curator + researcher + reviewer all consume this surface.
- **Layer 3 — MCP** (later): exposes Layer 1 to external agents
  (Claude Code, etc.) without re-implementing.

semstreams's `Triple.Source` is "who wrote the triple"
(`agent-decide`, `agent-web-search`, ingest pipelines), NOT
"which federated graph" — the latter is semspec-specific
(`graph.SourceRegistry`) and stays semspec-side. For the curator
this means the Source distribution facet shows triple authorship
(useful for audit; not load-bearing for the source-curation
decision). Entity-count-per-namespace is the load-bearing facet
for curator decisions.

A companion `search_graph` resolver is also planned —
server-side fallback from `globalSearch` (GraphRAG) to
`semanticSearch` when communities aren't populated. Not directly
in scope for the curator's recovery-cycle problem but worth
knowing about: it gives the researcher a stronger primitive than
`query_entity` for "find me entities matching this concept" even
when entity IDs aren't predictable.

### Evidence

- `memory/project_smoke19_run1.md` — full smoke writeup.
- `/tmp/smoke19-run1/trajectory-*.json` — per-curator full
  trajectories.
- `/tmp/smoke19-run1/triples.json` — chain entity state showing
  zero `chain.curator.*` writes.
