# User Journey Specs

User-facing capability journeys for the semteams UI. Each journey
is a **Playwright test file** under `ui/e2e/agentic/` — the test
IS the spec. No custom markdown schema, no parallel prose, no
drift risk.

## Where things live

| Artifact | Location | Purpose |
|---|---|---|
| Journey specs (executable) | `ui/e2e/agentic/<slug>.spec.ts` | Playwright `describe`/`test` — readable AND runnable |
| Shared mock-llm fixtures | `test/fixtures/journeys/<slug>.yaml` | Deterministic LLM responses for the mock-llm container |

The complete inventory of landed journeys is whatever is in
`ui/e2e/agentic/`. There is no parallel "planned vs landed" table
here on purpose — that table drifts the moment a spec lands.

## Why Playwright is the spec

The "what we claim to support" claim lives in the test file itself:

- `test.describe('…', () => { ... })` — the journey name
- JSDoc at the top — the goal, preconditions, wire-shape
  assumptions, and citations to relevant design or architecture notes
- `test('…', async ({ page, request }) => { ... })` — each step
- Assertions inside the test — backend state (via
  `request.get('/teams-dispatch/loops/{id}')`) AND UI state (via
  `expect(page.locator(...))`) in the same function

One source of truth. No "update the markdown, then update the
spec" drift.

A solid Playwright spec also (per
[`feedback_solid_playwright_specs`](../../README.md)):

- Comments the **why** at the top — what's the user journey, what
  invariants hold, what fixture/config it needs, what wire-shape
  assumptions are baked in.
- Asserts the *cause* of the bug class, not just the symptom.
- Carries clear failure messages on every `expect` that has a
  non-obvious cause — failure messages are the only doc future-us
  reads.

`ui/e2e/agentic/chain-drill-in.spec.ts` (2026-06-03) is the
template.

## Adding a new journey

1. Write the Playwright spec at `ui/e2e/agentic/<slug>.spec.ts`.
   Use `test.describe()` for the journey name, `test()` per step.
   Include a JSDoc block at the top.
2. If the journey needs deterministic LLM responses, drop a YAML
   fixture at `test/fixtures/journeys/<slug>.yaml`.
3. Add a `test:e2e:agentic:<slug>` task to `ui/Taskfile.yml` that
   wires the fixture + the e2e bootstrap config.
4. Run it: `task ui:test:e2e:agentic:<slug>`.

Component instance names are `teams-dispatch` / `teams-loop` so
HTTP prefixes match; factories are upstream `agentic-dispatch` /
`agentic-loop`.

## Running journeys

```bash
# Single journey (iteration loop)
task ui:test:e2e:agentic:<slug>

# Bring up stack but don't run anything (poking around)
task ui:test:e2e:agentic:dev:<scenario>

# Tear down + wipe volumes
task ui:test:e2e:agentic:cleanup
```

`task --list` shows the full surface. Mock-llm fixture state is
held in memory; if a chain doesn't dispatch on a fresh prompt,
the fixture index probably exhausted — tear the stack down with
`-v` and re-up.

## Deliberate-break verification

A spec that silently tolerates broken behavior is worse than no
spec at all. When the agentic surface changes meaningfully (and
at least once after any major dependency bump), pick a couple of
landed specs and:

1. Run the journey against a clean tree — confirm it passes.
2. Introduce a targeted break to something the spec claims to
   validate (rename a testid, swap a fixture turn, drop a query
   parameter, etc.).
3. Re-run. It **must** fail, and the failure message should point
   clearly at the broken claim. If the spec passes with the
   deliberate break in place, the spec is too loose.
4. Revert. Confirm green again.

Recording the specific break in the spec's JSDoc as a known-good
canary is a worthwhile bonus when the spec has a non-obvious
load-bearing assertion — the next person breaking it can read
why it's there.
