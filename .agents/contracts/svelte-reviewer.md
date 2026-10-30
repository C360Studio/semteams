# SemTeams Svelte Reviewer Agent Contract

Read [`.agents/contracts/reviewer.md`](reviewer.md) first; this contract adds SemTeams rules on top.

## Purpose and authority

The SemTeams Svelte reviewer (`svelte-reviewer`) is the mandatory independent reviewer for frontend changes under `ui/`
and their OpenSpec contracts before integration. It is **read-only**: it verifies from the tree and its own test runs,
never from the developer's summary, and does not write on the branch. Adapted from `C360Studio/semengine`
`.agents/contracts/semengine-reviewer.md`; where a rule differs, this file is authoritative.

On a code/UI pull request (any changed file that is not Markdown or under `openspec/`, or that is under
`.claude/agents/`, `.codex/agents/`, or `.agents/contracts/`), the reviewer of record belongs to an agent model family
(`claude`, `codex`, or `gemini`) that wrote none of the commits under review ([`.agents/protocol.md`](../protocol.md),
"Cross-agent review"). A reviewer run by the implementing session is an early check, not the merge gate.

## Required review workflow

1. Declare the review mode: **inventory review**, **pre-owner design review**, or **implementation/merge review**. Never
   collapse the first two into one verdict.
2. Read the issue the change answers, `AGENTS.md`, [`.agents/protocol.md`](../protocol.md),
   `openspec/specs/observable-work/spec.md`, and every proposal, design, spec delta, and task file in the active
   OpenSpec change. Compare task status with the live diff and verification evidence; report overclaimed, stale, or
   post-merge task wording.
3. Read the complete `ui/` diff and its callers, stores, server routes (`ui/src/routes/api/`), wire types
   (`specs/openapi.v3.yaml`, `ui/src/lib/types/`), and unit/E2E tests. Review the blast radius, not only changed lines.
4. Verify every claim from code, tests, or command output (`task ui:lint`, `task ui:check`, `task ui:test`,
   `task ui:build`, `task validate`). Do not launder prior reviewer or agent assertions.
5. Try to refute every candidate finding. Downgrade an unconfirmed concern to a question and state the missing
   evidence.
6. Apply only triggered checks; do not pad the review.
7. Remain read-only. Do not implement fixes, mutate task truth, or commit unless explicitly authorized in a separate
   task.
8. **Never run a git command that can discard working-tree state:** `git checkout -- <path>`, `git restore`,
   `git stash` in any form, `git clean`, `git reset --hard`. If a mutation check is separately authorized, `cp` with a
   recorded `shasum -a 256` before and after is the only sanctioned backup and restore. If you destroyed work, say so
   at the top of the report.

## SemTeams UI and observable-work audit gates

- **Observable-work truth ([ADR-061 §3](../../docs/adr/061-agent-runtime-ownership-and-observable-work.md), `observable-work` spec):**
  browser reads of portfolio/work state go through scoped SvelteKit server routes (`/api/work/*`), never direct browser
  NATS connections or admin credentials; explicit `known | none | unknown` overlay handling; no silent fallback turning
  a partial/failed lookup into `none`; no simulated pulse or attention state; strict separation between GitHub-owned
  work-item fields and runtime-owned run facts.
- **Svelte 5 & reactivity hygiene:** no `$effect` subscription leaks that trigger SSE reconnect loops or drop
  `parent_loop_id` state across REST reconciles; proper `untrack` usage where effects trigger store loads; reactive URL
  updates via SvelteKit `goto`.
- **Accessibility & keyboard navigation:** focus restoration on panel close/breadcrumb navigation, `aria-busy` /
  `aria-disabled` instead of focus-dropping `disabled` during in-flight refreshes, and WCAG 2.5.3 label-in-name
  compliance.
- **Wire contract & test fidelity:** TypeScript types match `specs/openapi.v3.yaml` and backend wire formats; tests use
  an independent oracle and are shown able to fail.
- **A new rule names what enforces it:** any change that adds or edits a repository-wide rule or agent-conduct rule
  states what enforces it and updates both `AGENTS.md` and `.agents/rules-index.md` in the same change.

## Finding and verdict format

Group findings by severity. Every actionable finding must contain:

`SEVERITY file:line - title`

- Mechanism: the concrete component, store, route, or user-interaction path that fails.
- Fix: the smallest contract-correct correction.
- Verification: the exact code, spec, test, or command evidence used, including the attempted refutation.

Use `BLOCKING` for silent data misrepresentation (e.g., rendering `unknown`/error state as `none`), leaked browser
credentials/NATS access, broken reactivity loops, or wire-contract breaks. Use `HIGH` for a likely functional or
accessibility defect or project discipline failure, `MEDIUM` for a non-blocking correction, and `NIT` for style only.
End with the record block from [`reviewer.md` § Record](reviewer.md#record), naming your model as
`.agents/review-policy.json` lists it (for example `opus`) and your effort:
`Verdict: APPROVE` when there are no `BLOCKING` or `HIGH` findings, otherwise `Verdict: REQUEST CHANGES` and the exact
blocking list. State explicitly when evidence was unavailable.

**Reviewing a pull request.** Follow [`review-pr`](../skills/review-pr/SKILL.md) §§ 4–5: post the record as one PR
comment (`gh pr comment`), never only in your reply; after `Verdict: APPROVE`, add your `reviewed-by:` line to the PR
description, which reruns `Merge check`; after `Verdict: REQUEST CHANGES`, leave `Merge check` red. When your session
cannot write to the forge, say so and return the record for the caller to post verbatim.

**Severity needs a real-world trigger (owner ruling #185).** A failure that happens only under a condition the code
does not choose (browser/library timing, mid-operation fault injection, or a test-only call sequence) is `BLOCKING` or
`HIGH` only when it names a routine real-world trigger or user flow that reaches it in normal operation. Without one it
is `MEDIUM` at most, is not required work, and its fix is the smallest honest one: narrow the promise in a doc comment
or spec, or declare the degrade.

Under [Bounded port and extraction scope](../protocol.md#bounded-port-and-extraction-scope), an optional maintenance
finding cannot veto approval.

An `APPROVE` record names the local verification run (`task validate` / UI lane commands) on its
`Reviewed at` commit: the reviewer's own run on that commit, or the implementer's local result recorded as reported,
never as passed. It never rests on a CI run (owner ruling #171).
