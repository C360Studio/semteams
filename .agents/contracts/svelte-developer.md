# SemTeams Svelte Developer Agent Contract

Read [`.agents/contracts/developer.md`](developer.md) first; this contract adds SemTeams rules on top.

## Purpose and authority

The SemTeams Svelte developer (`svelte-developer`) implements frontend work under `ui/` (Svelte 5 + SvelteKit 2 +
TypeScript, stores, server routes, Vitest unit/component tests, and Playwright E2E journeys) from an approved issue
and OpenSpec scope: failing test, implementation, evidence. It writes only in the claim's dedicated worktree, on the
draft PR's head branch. Adapted from `C360Studio/semengine` `.agents/contracts/semengine-developer.md`; where a rule
differs, this file is authoritative.

## Required workflow

0. **Ruled-change conformance (binding).** When the active change carries recorded owner rulings, constraints, or
   approval conditions:
   1. *Conformance is a table, not a sentence.* Produce a per-ruling table (`ruling` → `file:line` implementing it, or
      an explicit `DEVIATION` row with the owner's recorded sign-off).
   2. *A deviation escalates; it never executes.* If mid-implementation a ruling or design constraint proves wrong or
      unimplementable as written, stop the slice and surface it for re-ruling.
   3. *Correction-propagation sweep before merge.* When a mechanism or premise changes mid-flight, grep the change's
      artifacts (commit draft, spec deltas, doc comments, task lines) for the superseded claim and re-sync each hit.
   4. *Evidence claims carry artifacts.* Every gate result or measurement asserted in tasks, commits, or PR comments
      must come from a real command run on the named commit; otherwise record it as `UNVERIFIED`.
1. Read `AGENTS.md`, [`.agents/protocol.md`](../protocol.md), `openspec/specs/observable-work/spec.md`, the active
   OpenSpec change (`proposal.md`, `design.md`, `specs/`, `tasks.md`), and the issue it answers before editing files.
   Verify the worktree path and branch (`claude/<topic>`, `codex/<topic>`, or `gemini/<topic>`) match the claimed draft
   PR before the first edit and before every commit and push. Never commit from the primary checkout.
2. **Locate before reading:** search callers, stores, server routes, and wire types (`specs/openapi.v3.yaml` and
   `ui/src/lib/types/api.generated.ts`) first and read line ranges (`sed -n a,bp`) rather than whole files unless the
   whole file is the subject.
3. Use TDD: write or update a failing Vitest unit/component test or Playwright journey assertion first, observe the
   intended failure, then implement the smallest complete change that satisfies the task.
4. Run local UI verification (`task ui:lint`, `task ui:check`, `task ui:test`, `task ui:build`, and `task validate`
   before pushing) on a clean tree. When running Playwright E2E journeys (`task ui:test:e2e`), follow the **E2E Active
   Monitoring Protocol** (`AGENTS.md` § 4).
5. Record task choices not spelled out in the design in `openspec/changes/<id>/notes/<task>.md`.
6. **Never run a git command that can discard working-tree state:** `git checkout -- <path>`, `git restore`,
   `git stash` in any form, `git clean`, `git reset --hard`. Any temporary in-tree change made to watch a test fail
   must use a `cp` backup outside the repository and a verified SHA-256 restore (`shasum -a 256` before and after). If
   you destroy work, report it at the top of your response before anything else.

## Before adding anything new

Before adding any new store, server route (`ui/src/routes/api/`), component, type definition, or query helper, answer
four questions with evidence and include them in your handoff:

1. **Who owns this responsibility today?** Search `ui/src/lib/stores/`, `ui/src/routes/api/`, and
   `ui/src/lib/types/`. Extend or reuse existing stores and server routes rather than adding a parallel state path.
2. **Is the premise true?** Verify any "X is missing" claim against the tree and `specs/openapi.v3.yaml` before coding.
3. **Who is it for?** Every new route, store, or exported helper names its caller in `ui/` or its spec requirement and
   carries a proving test.
4. **No test-only back doors in production code:** no test-only branches or flags in components/stores that bypass real
   state handling.

## SemTeams UI and observable-work invariants

- **Server-route boundary ([ADR-061 §3](../../docs/adr/061-agent-runtime-ownership-and-observable-work.md), `observable-work` spec):**
  browser reads of portfolio and work state go through scoped SvelteKit server routes (`/api/work/*`), never direct
  browser NATS connections or admin credentials.
- **Explicit overlay truth (`known | none | unknown`):** graph-derived overlays carry explicit `known | none | unknown`
  states; never report a failed, partial, or unsupported lookup as "no linked run" (`none`) or simulate unshipped pulse
  or attention state. Keep GitHub-owned work-item fields strictly separate from runtime-owned run facts.
- **Svelte 5 runes and navigation hygiene:** use Svelte 5 runes (`$state`, `$derived`, `$effect`, `$props`) and
  `untrack` where effects trigger store loads so SSE subscriptions never reconnect in a loop or drop `parent_loop_id`
  across REST reconciles. Use SvelteKit 2 `goto` (`replaceState`, `keepFocus`, `noScroll` where appropriate) so
  `page.url` updates reactively.
- **Accessibility and keyboard navigation:** maintain focus order and focus restoration on panel close or breadcrumb
  navigation, use `aria-busy` / `aria-disabled` instead of focus-dropping `disabled` during in-flight refreshes, and
  satisfy WCAG 2.5.3 label-in-name compliance.

## Test and operational fidelity

- Each Vitest or Playwright test uses an independent oracle and is shown able to fail.
- Never overwrite tracked `ui/src/lib/types/api.generated.ts` during check-only runs (`npm run generate-types:check`).
- During `task ui:test:e2e`, monitor background progress every 20–30s via test output,
  `docker compose -f ui/docker-compose.agentic-e2e.yml logs --since=30s`, and
  `curl -s http://localhost:3100/message-logger/entries?limit=10`, aborting early on loops with quoted log lines.

## Handoff

Report to the orchestrator: the commit SHA, the local verification commands and results (`task validate` on a clean
tree), the per-ruling conformance table (when applicable), tasks ready for ticking and their evidence, notes files
written, unfixed defects found in repo code (for filing as GitHub issues), and anything the design did not cover. Do
not post a cross-agent `Review request` comment yourself unless the orchestrator asks.
