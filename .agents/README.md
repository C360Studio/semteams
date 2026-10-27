# SemTeams Agent Profiles

The files in `contracts/` are the tracked, platform-neutral behavioral authority. Platform adapters are intentionally
thin and must point to exactly one canonical contract. Adapted from SemEngine (`C360Studio/semengine`, `.agents/`),
SemVocab, and SemSparql; where a rule differs, this tree is authoritative.

## Role and platform mapping

- SemTeams architect (read-only)
  - Canonical: `.agents/contracts/architect.md`
  - Claude: `.claude/agents/architect.md`
  - Codex: `.codex/agents/architect.toml`
  - Gemini (Antigravity): `define_subagent` (`name = "architect"`, `enable_write_tools = false`)
- SemTeams Go developer
  - Canonical: `.agents/contracts/go-developer.md`
  - Claude: `.claude/agents/go-developer.md`
  - Codex: `.codex/agents/go-developer.toml`
  - Gemini (Antigravity): `define_subagent` (`name = "go-developer"`, `enable_write_tools = true`)
- SemTeams Go reviewer (independent; read-only)
  - Canonical: `.agents/contracts/go-reviewer.md`
  - Claude: `.claude/agents/go-reviewer.md`
  - Codex: `.codex/agents/go-reviewer.toml`
  - Gemini (Antigravity): `define_subagent` (`name = "go-reviewer"`, `enable_write_tools = false`)
- SemTeams Svelte developer
  - Canonical: `.agents/contracts/svelte-developer.md`
  - Claude: `.claude/agents/svelte-developer.md`
  - Codex: `.codex/agents/svelte-developer.toml`
  - Gemini (Antigravity): `define_subagent` (`name = "svelte-developer"`, `enable_write_tools = true`)
- SemTeams Svelte reviewer (independent; read-only)
  - Canonical: `.agents/contracts/svelte-reviewer.md`
  - Claude: `.claude/agents/svelte-reviewer.md`
  - Codex: `.codex/agents/svelte-reviewer.toml`
  - Gemini (Antigravity): `define_subagent` (`name = "svelte-reviewer"`, `enable_write_tools = false`)
- SemTeams technical writer
  - Canonical: `.agents/contracts/technical-writer.md`
  - Claude: `.claude/agents/technical-writer.md`
  - Codex: `.codex/agents/technical-writer.toml`
  - Gemini (Antigravity): `define_subagent` (`name = "technical-writer"`, `enable_write_tools = true`)

SemTeams carries six cross-stack specialist roles (`architect`, `go-developer`, `go-reviewer`, `svelte-developer`,
`svelte-reviewer`, `technical-writer`) alongside ATeam's base [`.agents/contracts/developer.md`](contracts/developer.md)
and [`.agents/contracts/reviewer.md`](contracts/reviewer.md) contracts and [`.agents/review-policy.json`](review-policy.json);
SemStreams' explorer and judge contracts are not carried. Add a role only with a recorded owner decision.

## Model routing

Adapters separate **model tier** from **reasoning effort** (`low -> medium -> high`). Neither the architect nor the
reviewers run on a weaker model tier than the developers.

| Role | Claude | Codex | Gemini (Antigravity) | Target effort |
| --- | --- | --- | --- | --- |
| `architect` | `opus`, `medium` | `gpt-6-astra`, `medium` | `inherit` or `pro` (`enable_write_tools = false`) | `medium` default; escalate to `high` for cross-cutting concurrency/boundary designs |
| `go-developer` | `opus`, `medium` | `gpt-6-sol`, `medium` | `inherit` or `pro` (`enable_write_tools = true`) | `medium` default for bounded work; escalate to `high` for complex/failed-review rounds |
| `go-reviewer` | `opus`, `high` | `gpt-6-astra`, `high` | `inherit` or `pro` (`enable_write_tools = false`) | `high` (single-pass audit of record against review-only rules) |
| `svelte-developer` | `opus`, `medium` | `gpt-6-sol`, `medium` | `inherit` or `pro` (`enable_write_tools = true`) | `medium` default for bounded UI work; escalate to `high` for complex stores/failed-review rounds |
| `svelte-reviewer` | `opus`, `high` | `gpt-6-astra`, `high` | `inherit` or `pro` (`enable_write_tools = false`) | `high` (single-pass audit of record for UI, accessibility, and contract truth) |
| `technical-writer` | `sonnet`, `medium` | `gpt-6-sol`, `medium` | `inherit` (or `flash` when requested; `enable_write_tools = true`) | `low` or `medium` (mechanical doc and OpenSpec task-truth sync) |

- **Model tier:** `architect`, `go-developer`, `go-reviewer`, `svelte-developer`, and `svelte-reviewer` run on the
  frontier model (`opus` on Claude, `gpt-6-astra` / `gpt-6-sol` on Codex, `inherit` / `pro` on Gemini).
  `technical-writer` is non-decision-making (durable doc sync and conservative OpenSpec task ticking) and runs on a
  lighter tier where supported (`sonnet` on Claude, `flash` on Gemini when explicitly requested).
- **Reasoning effort (`low -> medium -> high`):** Start `architect`, `go-developer`, and `svelte-developer` at `medium`
  effort by default and escalate to `high` only when a change crosses module/runtime boundaries, designs a concurrency
  or durable-execution contract, or fails a review round. Keep `go-reviewer` and `svelte-reviewer` at `high` effort
  because they run once per ready diff as the independent audit against the repository's review-only rules. Run
  `technical-writer` at `low` or `medium` effort. The tracked `.claude/agents/*.md` (`model` and `effort`) and
  `.codex/agents/*.toml` (`model` and `model_reasoning_effort`) adapter files pin these defaults and are checked by
  `TestAgentSetupParity` (`test/contract/shared_work_protocol_contract_test.go`).

### Gemini (Antigravity) capability and effort parity

Unlike Claude (`.claude/agents/*.md`) and Codex (`.codex/agents/*.toml`), Antigravity registers role subagents at
runtime via `define_subagent` and launches them via `invoke_subagent`. To keep Gemini in capability and effort parity
with Claude and Codex:

1. **Register thin role adapters with `define_subagent`:** For each role invoked in a session, register it with
   `name = "<role>"`, `enable_write_tools = false` for read-only roles (`architect`, `go-reviewer`, `svelte-reviewer`)
   or `enable_write_tools = true` for write roles (`go-developer`, `svelte-developer`, `technical-writer`), and a
   `system_prompt` instructing: `Your first action is to read .agents/contracts/<role>.md fully and follow it as the
   behavioral authority for this role.`
2. **Keep `Model = "inherit"` by default:** In `invoke_subagent`, always pass `Model = "inherit"` (inheriting the
   active frontier session model, such as Gemini 4 Argon Medium/High) unless the user explicitly requests `"pro"` or
   `"flash"`. Never self-downgrade `architect`, `*-developer`, or `*-reviewer` to `"flash"` or `"flash_lite"`.
3. **Carry target reasoning effort in the subagent brief:** Because `invoke_subagent` has no separate
   `model_reasoning_effort` field and the parent session may run at `Medium`, state the target effort posture
   explicitly in `Prompt`:
   - For `go-reviewer` and `svelte-reviewer` (`high` effort): require a full single-pass audit of record (read the
     complete diff and blast radius via `gopls`/callers, verify claims from the tree and local verification commands,
     attempt refutation of every candidate finding, and emit `APPROVE` or `CHANGES REQUESTED`).
   - For `architect`, `go-developer`, and `svelte-developer`: state `medium` effort for bounded work, or escalate to
     `high` in the brief when crossing runtime/module boundaries, designing concurrency/durability contracts, or
     resolving a failed review round.
4. **Model-family identity and attestation:** Antigravity can host both Gemini and Claude models; for `implemented-by:`
   and `reviewed-by:` the agent is the **model family** (`claude`, `codex`, `gemini`), not the IDE or desktop shell, and
   each line records the resolved model and reasoning effort (`implemented-by: gemini (gemini-4-argon, high)`), never
   unresolved `inherit`, for comparison under [`.agents/review-policy.json`](review-policy.json)
   ([`.agents/protocol.md`](protocol.md), "Cross-agent review").

The table does not rank one platform's models against another's; across agents the pairing is the owner's decision.
For a code/UI pull request the reviewer of record is another agent's reviewer from the table above; a documents-only
pull request keeps any agent's reviewer ([`.agents/protocol.md`](protocol.md), "Cross-agent review").

## Orchestrating role agents

These rules bind the session that intakes owner requests and spawns/briefs role agents. Every review round finds
something real, so the review loop is not where to cut; the avoidable cost is in unclarified scope and paraphrased
briefs.

- **Clarify Definition of Done before starting or delegating work.** Before mutating state or spawning a role agent on
  an underspecified prompt, do a quick read-only check of any named issue, PR, or file (never asking what the tree
  already answers), then verify four boundaries are clear: (1) **Deliverable** (answer/audit, issue, OpenSpec design,
  docs-only PR, or code/UI PR), (2) **Scope and non-goals** (in-bounds packages/files and what must not change),
  (3) **Definition of Done and verification** (observable completion criteria and `task` gates), and (4) **Stop
  boundary** (pause after inventory/design for an owner decision vs. run through to a ready PR). If any of the four is
  missing or ambiguous, stop, propose a concise 3–4 bullet recommended Definition of Done, and ask at most 1–3 targeted
  questions; once confirmed, run autonomously to that finish line without stopping for routine steps.
- **A brief carries the owner's intent, not only the approved document.** State what the product is for
  (`AGENTS.md`, [`docs/product/program-manager.md`](../docs/product/program-manager.md), and
  [ADR-061](../docs/adr/061-agent-runtime-ownership-and-observable-work.md)) and any owner ruling that bears on the
  question.
- **An orchestrator's paraphrase is an unreviewed claim.** Hand an agent the artifact itself (a path, an issue comment
  URL, a `file:line`), not a summary of it. Where a summary is unavoidable, mark it as the orchestrator's so the
  reviewer checks it like any other claim.
- **Resume for continuity, start fresh for mechanics.** A resumed agent re-reads its own transcript on every resume.
  For a mechanical follow-up, a fresh agent with a precise brief costs less.
- **A design gets three review rounds.** Findings still open after the third go to the owner on the issue, with the
  choice stated, instead of a fourth round.
- **Scope an inventory before it starts.** An inventory names its question and the repositories (and pinned commits) it
  reads first: this repository, the frozen SemStreams pin (`8b99efe9c66a4faa4fa509f9f62cc6bad8392128`), and SemEngine's
  admitted contracts.

## Shared work protocol and rule index

[`.agents/protocol.md`](protocol.md) is the canonical shared work protocol (state homes, rituals, worktree hygiene, and
cross-agent review). `CLAUDE.md` imports `AGENTS.md`, which points to `.agents/protocol.md` and carries the four gates
(clarify, claim, merge, close) inline; edit the protocol only in `.agents/protocol.md`.
[`.agents/rules-index.md`](rules-index.md) is the enforcement audit matrix backing the domain-grouped rule summary in
`AGENTS.md`.

## Shared skills

Canonical skills live in `skills/`. Read the relevant `SKILL.md` fully; the platform adapters in `.claude/skills/` and
`.gemini/commands/*.toml` add discovery metadata and argument handling, not another rule set. Repository protocol and
role contracts remain authoritative.

Canonical ATeam workflow skills:

- [pickup](skills/pickup/SKILL.md): reconcile live git/worktree, GitHub, and `.ateam/state.json` state and verify write
  ownership (`ateam state pickup`). Claude `/pickup`; Gemini `.gemini/commands/pickup.toml`.
- [handoff](skills/handoff/SKILL.md): checkpoint unfinished work across Claude, Codex, and Gemini sessions and emit the
  5-field pickup prompt (`ateam state handoff`). Claude `/handoff`; Gemini `.gemini/commands/handoff.toml`.
- [preflight](skills/preflight/SKILL.md): scope SemTeams verification gates (`task validate` / `task verify`, Go, UI,
  Governance, Playwright E2E) and record portable evidence. Claude `/preflight`; Gemini `.gemini/commands/preflight.toml`.
- [ponytail](skills/ponytail/SKILL.md): smallest complete change discipline ladder, `// shortcut:` tracking, and
  1–2 line skipped/risk summary. Claude `/ponytail`; Gemini `.gemini/commands/ponytail.toml`.
- [review-pr](skills/review-pr/SKILL.md): independent read-only cross-agent PR review and commit-bound `reviewed-by:` +
  `Reviewed at <sha>` + `Verdict:` comment record. Claude `/review-pr`; Gemini `.gemini/commands/review-pr.toml`.

SemTeams architecture and runtime-capability skills (`adopter_skills` in `ateam doctor`):

- [kv-or-stream](skills/kv-or-stream/SKILL.md): choose a KV watch or a JetStream stream. Claude `/kv-or-stream`.
- [new-payload](skills/new-payload/SKILL.md): add a payload type to the registry. Claude `/new-payload`.
- [orchestration-check](skills/orchestration-check/SKILL.md): decide whether logic belongs in a reactive rule, workflow,
  or component. Claude `/orchestration-check`.
- [query-pattern](skills/query-pattern/SKILL.md): choose how code reads the graph (GraphQL, MCP, or NATS Direct).
  Claude `/query-pattern`.

Codex can invoke canonical names with `$` or read their paths directly. Antigravity (Gemini) automatically discovers
`.agents/skills/<name>/SKILL.md` from the workspace root, and Gemini CLI loads `.gemini/commands/*.toml`. Claude Code
uses `.claude/skills/<name>/SKILL.md`. A private memory reference cannot be required to interpret a shared rule.

## Read-only parity smoke

Steps 1–8 are enforced automatically by `task ateam:doctor` (`ateam doctor --check`) and `TestAgentSetupParity` /
`TestAgentSetupParitySensitivity` (`test/contract/shared_work_protocol_contract_test.go`, run by `task test:race` and
`task validate`). Run Step 9 after changing a contract, adapter, or repository routing rule.

1. Confirm all six specialist contracts (`.agents/contracts/{architect,go-developer,go-reviewer,svelte-developer,svelte-reviewer,technical-writer}.md`),
   ATeam's `.agents/contracts/{developer,reviewer}.md`, `.agents/review-policy.json`, six Claude adapters
   (`.claude/agents/*.md`), and six Codex adapters (`.codex/agents/*.toml`) exist, and that `.agents/protocol.md` and
   `.agents/rules-index.md` exist.
2. Confirm each adapter names-and-links only its matching `.agents/contracts/<role>.md` path and says to read it fully
   first.
3. Confirm the Claude read-only adapters (`architect`, `go-reviewer`, `svelte-reviewer`) list tools
   `Read, Bash, Grep, Glob, Skill`, and that write adapters (`go-developer`, `svelte-developer`, `technical-writer`)
   add `Edit` and `Write`, with no `Task`, `LSP`, or other delegation tool (`gopls` is reached through `Bash`; an
   unresolved tool name makes Claude Code refuse to launch the agent).
4. Confirm the Codex read-only adapters (`architect`, `go-reviewer`, `svelte-reviewer`) set
   `sandbox_mode = "read-only"`; write adapters (`go-developer`, `svelte-developer`, `technical-writer`) have no
   sandbox override and inherit the parent workspace permissions.
5. Confirm `CLAUDE.md` only imports `@AGENTS.md` and carries no rules of its own, that `AGENTS.md` stays strictly under
   `24,000` bytes (`wc -c AGENTS.md`), and that every rule row in `AGENTS.md` has its corresponding audit row in
   `.agents/rules-index.md` (`TestAgentSetupParity` checks numbered domain headings and per-domain row counts; semantic
   correspondence of individual rows is review only).
6. Confirm role adapters stay within 20 lines and contain no copied checklist (`- [ ]` or `- [x]`).
7. For every skill in `.agents/skills/<name>/SKILL.md`, confirm the canonical file and Claude adapter exist, the
   adapter points to that canonical file and says to read it fully, stays within 25 lines, and contains no copied
   checklist.
8. Confirm the Claude (`.claude/agents/*.md` `model` and `effort`) and Codex (`.codex/agents/*.toml` `model` and
   `model_reasoning_effort`) settings match the table above.
9. Validate changed skill frontmatter and relative links, then walk the applicable scenarios below without private
   memory.

Use these semantic fixtures when reading the routing text:

- "Implement a nontrivial backend or pack change" routes first to `go-developer` and then `go-reviewer`.
- "Implement a nontrivial `ui/` change" routes first to `svelte-developer` and then `svelte-reviewer`.
- "Implement a cross-stack backend + `ui/` change" routes through both developer/reviewer lanes before landing.
- "Review a nontrivial SemTeams change" routes to `go-reviewer` and/or `svelte-reviewer` in read-only mode.
- "Design an API, runtime extraction slice, pack contract, or OpenSpec target" routes to `architect` (ownership and
  seam inventories first, drafts as text); binding rulings and approval stay with the owner session.
- "Update durable docs or reconcile OpenSpec task truth" routes to `technical-writer`.
- "Prepare a docs-only PR" retains protocol review/CI gates and selects checks for the changed artifacts.
- "The previous revision was green" preserves that evidence as historical, not current-head proof.
- "Continue without private memory" resolves shared state and verifies ownership through `pickup` and
  `.agents/protocol.md`.

The smoke passes only when Claude, Codex, and Gemini resolve the same logical role and canonical contract for every
fixture.
