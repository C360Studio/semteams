# SemTeams Agent Profiles

The files in `contracts/` are the tracked, platform-neutral behavioral authority. Platform adapters are intentionally
thin and must point to exactly one canonical contract.

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

## Model routing

Adapters separate **model tier** from **reasoning effort** (`low -> medium -> high`). Neither the architect nor the
reviewers run on a weaker model tier than the developers.

| Role | Claude | Codex | Gemini (Antigravity) | Target effort |
| --- | --- | --- | --- | --- |
| `architect` | `opus` (read-only tools) | `gpt-6-astra`, `medium` default | `inherit` or `pro` (`enable_write_tools = false`) | `medium` default; escalate to `high` for cross-cutting concurrency/boundary designs |
| `go-developer` | `opus` | `gpt-6-sol`, `medium` default | `inherit` or `pro` (`enable_write_tools = true`) | `medium` default for bounded work; escalate to `high` for complex/failed-review rounds |
| `go-reviewer` | `opus` | `gpt-6-astra`, `high` | `inherit` or `pro` (`enable_write_tools = false`) | `high` (single-pass audit of record against review-only rules) |
| `svelte-developer` | `opus` | `gpt-6-sol`, `medium` default | `inherit` or `pro` (`enable_write_tools = true`) | `medium` default for bounded UI work; escalate to `high` for complex stores/failed-review rounds |
| `svelte-reviewer` | `opus` | `gpt-6-astra`, `high` | `inherit` or `pro` (`enable_write_tools = false`) | `high` (single-pass audit of record for UI, accessibility, and contract truth) |
| `technical-writer` | `opus` (`sonnet` eligible) | `gpt-6-sol`, `low`/`medium` | `flash` or `inherit` (`enable_write_tools = true`) | `low` or `medium` (mechanical doc and OpenSpec task-truth sync) |

- **Model tier:** `architect`, `go-developer`, `go-reviewer`, `svelte-developer`, and `svelte-reviewer` run on the
  frontier model (`opus` on Claude, `gpt-6-astra` / `gpt-6-sol` on Codex, `inherit` / `pro` on Gemini).
  `technical-writer` is non-decision-making (durable doc sync and conservative OpenSpec task ticking) and may run on a
  lighter tier (`sonnet` on Claude, `flash` on Gemini).
- **Reasoning effort (`low -> medium -> high`):** Start `architect`, `go-developer`, and `svelte-developer` at `medium`
  effort by default and escalate to `high` only when a change crosses module/runtime boundaries, designs a concurrency
  or durable-execution contract, or fails a review round. Keep `go-reviewer` and `svelte-reviewer` at `high` effort
  because they run as the independent audit against the repository's review-only rules. Run `technical-writer` at `low`
  or `medium` effort.

On Gemini (Antigravity), role subagents are registered with `define_subagent` pointing at the matching
`.agents/contracts/<role>.md` contract and invoked with `invoke_subagent`; `Model` defaults to `"inherit"` (for example,
Gemini 4 Argon High) unless `"pro"` or `"flash"` is explicitly requested.

## Orchestrating role agents

These rules bind the session that intakes owner requests and spawns/briefs role agents:

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

## Shared work protocol

[`.agents/protocol.md`](protocol.md) is the canonical shared work protocol (state homes, rituals, worktree hygiene, and
cross-agent review). `CLAUDE.md` and `AGENTS.md` point to it and carry the summary table and rituals inline, enforced
for parity and the `< 24,000`-byte per-file rule cap by `test/contract/shared_work_protocol_contract_test.go`.

## Shared skills

Canonical skills live in `skills/`. Read the relevant `SKILL.md` fully; the platform adapters in `.claude/skills/` add
discovery metadata and argument handling, not another rule set. Antigravity (Gemini) automatically discovers
`.agents/skills/<name>/SKILL.md` from the workspace root; Codex can invoke canonical names with `$` or read their paths
directly; Claude Code uses `.claude/skills/<name>/SKILL.md`.

- [kv-or-stream](skills/kv-or-stream/SKILL.md): choose a KV watch or a JetStream stream. Claude command `/kv-or-stream`.
- [new-payload](skills/new-payload/SKILL.md): add a payload type to the registry. Claude command `/new-payload`.
- [orchestration-check](skills/orchestration-check/SKILL.md): decide whether logic belongs in a reactive rule, workflow,
  or component. Claude command `/orchestration-check`.
- [query-pattern](skills/query-pattern/SKILL.md): choose how code reads the graph (GraphQL, MCP, or NATS Direct).
  Claude command `/query-pattern`.

## Manual read-only parity smoke

Run `go test ./test/contract/... -run TestSharedWorkProtocol` after changing a contract, adapter, skill, or repository
routing rule:

1. Confirm all six canonical contracts (`.agents/contracts/*.md`), six Claude adapters (`.claude/agents/*.md`), and six
   Codex adapters (`.codex/agents/*.toml`) exist, and that `.agents/protocol.md` exists.
2. Confirm each adapter names its matching `.agents/contracts/<role>.md` path and says to read it fully first.
3. Confirm the read-only roles (`architect`, `go-reviewer`, `svelte-reviewer`) restrict write/edit tools in Claude and
   set `sandbox_mode = "read-only"` in Codex.
4. Confirm `AGENTS.md` and `CLAUDE.md` stay under `24,000` bytes (`wc -c AGENTS.md CLAUDE.md`) and share an identical
   `## Shared work protocol (Claude, Codex, and Gemini)` section.
5. Confirm every skill in `.agents/skills/<name>/SKILL.md` has a matching `.claude/skills/<name>/SKILL.md` adapter that
   points to the canonical file.
