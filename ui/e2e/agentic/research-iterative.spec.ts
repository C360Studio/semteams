import { test, expect } from "@playwright/test";

/**
 * Journey: Iterative Research with Reviewer-as-Enumerator Gate
 *
 * R1 of the ADR-031 phasing. Validates the iterative-researcher
 * pattern: a researcher submits an artifact, a research-reviewer
 * evaluates against a checklist, and the rule layer drives a
 * retry-on-insufficient cycle until the reviewer approves.
 *
 * The bounded prompt is "identify the actor types in OSH's driver
 * framework, given OSH core repo is already indexed in our test
 * SemSource." No SemSource integration ships in R1 — the mock-llm
 * fixture stands in for the indexed corpus by returning OSH-shaped
 * tool-call responses.
 *
 * Sequence (matches the fixture, four loops, eight LLM round-trips):
 *   Loop 1: researcher → query_entity → completion (incomplete artifact)
 *           → outcome=success → rule 01 fires → spawn reviewer
 *   Loop 2: research-reviewer → read_loop_result → decide(insufficient)
 *           → rule 02 fires → spawn researcher retry
 *   Loop 3: researcher (retry) → read_loop_result → completion
 *           (complete artifact) → outcome=success → rule 01 fires →
 *           spawn reviewer
 *   Loop 4: research-reviewer → read_loop_result → decide(approved)
 *           → no rule fires → terminal success
 *
 * Validates:
 *   - Two distinct researcher loops (initial + retry) appear
 *   - Two distinct reviewer loops (insufficient + approved) appear
 *   - Final loop terminates without firing another rule
 *   - Backend state confirms terminal-success on the last reviewer
 *
 * Required fixture: test/fixtures/journeys/research-iterative.yaml
 *
 * Run via:
 *   FIXTURE=research-iterative.yaml \
 *     AGENTIC_CONFIG=e2e-research-iterative.json \
 *     npx playwright test --config playwright.agentic.config.ts \
 *     e2e/agentic/research-iterative.spec.ts
 */

test.describe("Iterative Research with Reviewer Gate", () => {
  test.beforeAll(async ({ request }) => {
    const health = await request.get("/health");
    expect(health.ok()).toBe(true);
  });

  test("research → review insufficient → research retry → review approved", async ({
    page,
    request,
  }) => {
    // -----------------------------------------------------------------
    // Step 1 — open the Board so the SSE activity stream is connected
    // before any loops appear. Required for the kanban-card assertions
    // in later steps.
    // -----------------------------------------------------------------
    await page.goto("/");

    await expect(page.getByTestId("connection-status")).toHaveAttribute(
      "data-summary",
      "healthy",
      { timeout: 10000 },
    );

    await expect(page.getByTestId("kanban-board")).toBeVisible();

    // -----------------------------------------------------------------
    // Step 2 — type the bounded R1 prompt. The dispatch's default_role
    // is `researcher`, so the loop spawns directly into the researcher
    // role without a coordinator front-door (we test that elsewhere).
    // -----------------------------------------------------------------
    const chatInput = page.getByTestId("chat-input");
    await chatInput.fill(
      "Identify the actor types in OSH's driver framework, given OSH core repo is already indexed in our test SemSource.",
    );
    await page.getByTestId("send-button").click();

    // -----------------------------------------------------------------
    // Step 3 — wait for ALL FOUR expected loops to appear. The rule
    // layer fires synchronously after each loop's outcome lands, so
    // four loops should accumulate in /teams-dispatch/loops.
    //
    //   Loop A: initial researcher
    //   Loop B: reviewer-1 (returns insufficient)
    //   Loop C: researcher retry
    //   Loop D: reviewer-2 (returns approved)
    //
    // The ordering in the response array matches the time order they
    // were created.
    // -----------------------------------------------------------------
    const loops = await pollUntil(async () => {
      const resp = await request.get("/teams-dispatch/loops");
      if (!resp.ok()) return null;
      const list = (await resp.json()) as Array<{
        loop_id: string;
        role: string;
        state: string;
      }>;
      return list.length >= 4 ? list : null;
    }, { timeoutMs: 60000 });

    expect(loops, "expected 4 loops (researcher, reviewer, researcher-retry, reviewer)").toBeTruthy();
    expect(loops!.length).toBeGreaterThanOrEqual(4);

    // -----------------------------------------------------------------
    // Step 4 — verify the role distribution proves both rules fired.
    //
    // Counting (not sequencing) is the right invariant here for two
    // reasons:
    //   (a) The /teams-dispatch/loops API does not guarantee creation
    //       order in the response array.
    //   (b) LoopInfo.Role is `omitempty` in the wire JSON; an early
    //       poll on the user-initiated loop can return undefined
    //       before TaskMessage.Role propagates onto LoopInfo.
    //
    // Normalise undefined → default_role ("researcher") and assert the
    // counts:
    //   - 2 researcher loops (the initial pass + the retry) → proves
    //     rule 02 (reviewer-rejected → spawn researcher) fired.
    //   - 2 research-reviewer loops (after each researcher) → proves
    //     rule 01 (researcher-success → spawn reviewer) fired twice.
    // -----------------------------------------------------------------
    const expectedDefaultRole = "researcher";
    const roles = loops!
      .slice(0, 4)
      .map((l) => l.role || expectedDefaultRole);
    const researcherCount = roles.filter((r) => r === "researcher").length;
    const reviewerCount = roles.filter((r) => r === "research-reviewer").length;
    expect(researcherCount, `expected 2 researcher loops, got roles=${JSON.stringify(roles)}`).toBe(2);
    expect(reviewerCount, `expected 2 research-reviewer loops, got roles=${JSON.stringify(roles)}`).toBe(2);

    // -----------------------------------------------------------------
    // Step 5 — wait for ALL FOUR loops to reach terminal state. The
    // approved decision matches no rule, so the loop count must
    // settle at 4 with every loop in `complete`. We poll the full
    // /loops list rather than per-loop GETs because the order in
    // /loops is not guaranteed (see Step 4 note).
    // -----------------------------------------------------------------
    const allTerminal = await pollUntil(async () => {
      const resp = await request.get("/teams-dispatch/loops");
      if (!resp.ok()) return null;
      const list = (await resp.json()) as Array<{ state: string }>;
      if (list.length !== 4) return null;
      return list.every((l) => l.state === "complete") ? list : null;
    }, { timeoutMs: 30000 });

    expect(allTerminal, "expected all 4 loops to reach terminal complete state").toBeTruthy();

    // -----------------------------------------------------------------
    // Step 6 — assert the loop count did not grow beyond 4 even after
    // a settle interval. If a fifth loop appeared it means the
    // approved decision somehow re-fired a retry rule (regression).
    // -----------------------------------------------------------------
    await new Promise((r) => setTimeout(r, 1500));
    const finalList = await request.get("/teams-dispatch/loops").then((r) => r.json()) as unknown[];
    expect(finalList.length, "approved decision must not spawn a fifth loop").toBe(4);
  });
});

async function pollUntil<T>(
  check: () => Promise<T | null>,
  options: { timeoutMs?: number; intervalMs?: number } = {},
): Promise<T | null> {
  const timeout = options.timeoutMs ?? 30000;
  const interval = options.intervalMs ?? 250;
  const deadline = Date.now() + timeout;
  while (Date.now() < deadline) {
    const result = await check();
    if (result !== null && result !== undefined) {
      return result;
    }
    await new Promise((r) => setTimeout(r, interval));
  }
  return null;
}
