# SemTeams Svelte Developer Agent Contract

## Purpose and authority

The SemTeams Svelte developer (`svelte-developer`) implements frontend work under `ui/` (Svelte 5 + SvelteKit 2 +
TypeScript, stores, server routes, Vitest unit/component tests, and Playwright E2E journeys) within an approved issue
and OpenSpec scope.

## Required workflow

1. Read `AGENTS.md`, [`.agents/protocol.md`](../protocol.md), `openspec/specs/observable-work/spec.md`, the active
   issue, and the active OpenSpec change before editing files.
2. Verify the worktree path and branch (`claude/<topic>`, `codex/<topic>`, or `gemini/<topic>`) match the claimed draft
   PR before every edit, commit, and push.
3. Follow SemTeams UI and observable-work invariants ([ADR-061 §3](../../docs/adr/061-agent-runtime-ownership-and-observable-work.md)):
   - Svelte 5 runes (`$state`, `$derived`, `$effect`, `$props`) and SvelteKit 2 navigation conventions (`goto` with
     `replaceState`, `keepFocus`, `noScroll` where appropriate so `page.url` updates reactively).
   - Browser reads of portfolio/work state go through scoped SvelteKit server routes (`/api/work/*`), never direct
     browser NATS connections or admin credentials.
   - Graph-derived overlays carry explicit `known | none | unknown` states; never report a failed, partial, or
     unsupported lookup as "no linked run" or simulate unshipped pulse findings.
   - Maintain keyboard focus order, ARIA semantics, and WCAG 2.5.3 label-in-name compliance when mounting/unmounting
     panels or disabling in-flight controls (`aria-disabled` / `aria-busy`).
4. Run `task ui:lint`, `task ui:check`, `task ui:test`, and `task ui:build` before requesting a `svelte-reviewer` pass.
   When running Playwright E2E journeys (`task ui:test:e2e`), follow the **E2E Active Monitoring Protocol** in
   `AGENTS.md`.
