# User Journey Specs

User-facing capability journeys for the semteams agentic superpowers surface.
Each journey is a **Playwright test file** under `ui/e2e/agentic/` — the test
IS the spec. No custom schema, no parallel markdown prose, no drift risk.

## Where things live

| Artifact | Location | Purpose |
|---|---|---|
| Journey specs (executable) | `ui/e2e/agentic/<slug>.spec.ts` | Playwright `describe`/`test` — readable AND runnable |
| Shared mock-llm fixtures | `test/fixtures/journeys/<slug>.yaml` | Deterministic LLM responses, loaded by both Playwright and Go observer tests |
| Go observer tests (optional) | `test/e2e/scenarios/journeys/<slug>/` | Backend-only assertions for journeys that benefit from a non-browser observer |

## Why Playwright is the spec

The journey "what we claim to support" claim lives in the test file itself:

- `test.describe('Tool approval gate', () => { ... })` — the journey name
- JSDoc at the top — the goal, preconditions, rationale
- `test('user proposes high-risk tool', async ({ page, request }) => { ... })` — each step
- Assertions inside the test — backend state (via `request.get('.../loops/{id}')`) AND UI state (via `expect(page.locator(...))`) in the same function

One source of truth. No "update the markdown, then update the spec" drift.

## Planned journeys

Mapped from the superpowers in `docs/proposals/agentic-superpowers.md`:

| Slug | Superpower | Status |
|---|---|---|
| `tool-approval-gate` | Human-in-the-loop approval gates | **landed** (Phase C.2) |
| `real-time-activity-stream` | Real-time agent activity streaming | planned |
| `self-programming-rule-creation` | Self-programming agents (rule writes) | planned |
| `multi-agent-hierarchy` | Parent/child loop chains | planned |
| `graph-backed-memory` | Semantic memory recall | planned |
| `stock-workflow-deep-research` | Pre-built deep-research flow | planned |

Initial specs land in Phase C.2 (`tool-approval-gate`) and Phase C.3
(`real-time-activity-stream`) as tracer-bullet journeys. The rest follow
as each capability stabilizes.

## Adding a new journey

1. Write the Playwright spec at `ui/e2e/agentic/<slug>.spec.ts`. Use
   `test.describe()` for the journey name, `test()` per step. Include a
   JSDoc block at the top explaining goal / preconditions / rationale.
2. If the journey needs deterministic LLM responses, drop a YAML fixture at
   `test/fixtures/journeys/<slug>.yaml` and point the mock-llm container
   at it via the Playwright fixture helper.
3. Backend-state assertions go inline in the test via
   `await request.get('http://localhost:8080/agentic-dispatch/loops/{id}')`
   — no separate test file needed.
4. Run it locally: `task ui:test:e2e` (or `cd ui && npm run test:e2e`).
5. If the journey also benefits from a non-browser Go observer (e.g. heavy
   backend-only state assertions, or a backend CI tier that doesn't run
   Playwright), add a sibling scenario under
   `test/e2e/scenarios/journeys/<slug>/`. Optional — many journeys will
   only need the Playwright side.

## How to read a journey spec

A well-structured Playwright journey spec is self-documenting. Example
skeleton:

```typescript
/**
 * Journey: Tool approval gate
 *
 * Goal: Agent proposes a high-risk tool, user approves, loop resumes.
 *
 * Preconditions:
 *   - semteams stack running with agentic-dispatch, agentic-loop,
 *     agentic-tools, agentic-governance enabled
 *   - Mock-llm loaded with test/fixtures/journeys/tool-approval-gate.yaml
 *   - User has an active chat session
 *
 * Validates: Phase 4 HITL gate, ApprovalFilter, RequiresApproval enforcement
 */

import { test, expect } from "@playwright/test";

test.describe("Tool approval gate", () => {
  test("agent proposes high-risk tool and pauses for approval", async ({
    page,
    request,
  }) => {
    // Step 1: user sends message that triggers the tool
    await page.goto("/");
    // ...assert AgentLoopCard appears with state=executing

    // Step 2: loop transitions to awaiting_approval
    // ...assert ApprovalPrompt renders

    // Step 3: user clicks approve
    // ...assert loop resumes

    // Step 4: assert final backend state via direct HTTP
    const loop = await request
      .get("http://localhost:8080/agentic-dispatch/loops/...")
      .then((r) => r.json());
    expect(loop.state).toBe("complete");
  });
});
```

Anyone reading this file understands the journey without having to read
any other document.

## Rationale for this structure

The original question that started this directory — *"who owns Playwright
E2E for the agentic superpowers?"* — initially led to a custom markdown
schema + bash validator + YAML frontmatter design. That was over-engineered:
Playwright already has a journey DSL (`describe`/`test`), readable structure
(JSDoc + nested calls), and a built-in fixture system. Inventing a second
layer on top meant two sources of truth, a custom validator to maintain,
and a new format for contributors to learn.

Keeping Playwright as the single source avoids all of that. See the plan
at `.claude/plans/linked-finding-starlight.md` (local) for the full history.
