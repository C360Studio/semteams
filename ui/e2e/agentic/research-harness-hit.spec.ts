import { test, expect } from "@playwright/test";

/**
 * Journey: Research with harness-catalog-hit (R3.7.1.e of ADR-033)
 *
 * Single-pass researcher emits an artifact whose `harness` field
 * references the `stub` harness registered in
 * configs/harnesses-stub.json (mounted via SEMTEAMS_HARNESS_CATALOG_PATH).
 * Reviewer approves on first try.
 *
 * Validates:
 *   - boot-time harness manager loads the operator-curated catalog
 *   - persona-fragment auto-render path works under a real backend
 *   - emit_research_artifact accepts the new `harness` argument
 *   - the typed payload published on research.artifact.{loop_id}
 *     carries the harness field
 *   - the chain runs end-to-end without hanging
 *
 * The catalog-MISS path is structurally a strict subset of this
 * fixture (just omit the harness field) and is implicitly tested
 * by the existing research-iterative journey, whose fixture emits
 * artifacts without a harness field.
 *
 * Required fixture: test/fixtures/journeys/research-harness-hit.yaml
 *
 * Run via:
 *   task test:e2e:agentic:research-harness-hit
 */

test.describe("Research with harness-catalog-hit", () => {
  test.beforeAll(async ({ request }) => {
    const health = await request.get("/health");
    expect(health.ok()).toBe(true);
  });

  test("researcher → harness=stub artifact → reviewer approved", async ({
    page,
    request,
  }) => {
    await page.goto("/");

    await expect(page.getByTestId("connection-status")).toHaveAttribute(
      "data-summary",
      "healthy",
      { timeout: 10000 },
    );

    await expect(page.getByTestId("kanban-board")).toBeVisible();

    const chatInput = page.getByTestId("chat-input");
    await chatInput.fill(
      "Identify actor types for an OSH driver backed by a Meshtastic radio. Use the registered harness for verification scoping.",
    );
    await page.getByTestId("send-button").click();

    // -----------------------------------------------------------------
    // Wait for both loops to appear: initial researcher + reviewer
    // spawned by rule 01 on the researcher's success outcome.
    // -----------------------------------------------------------------
    const loops = await pollUntil(async () => {
      const resp = await request.get("/teams-dispatch/loops");
      if (!resp.ok()) return null;
      const list = (await resp.json()) as Array<{
        loop_id: string;
        role: string;
        state: string;
      }>;
      return list.length >= 2 ? list : null;
    }, { timeoutMs: 60000 });

    expect(loops, "expected 2 loops (researcher + reviewer)").toBeTruthy();
    expect(loops!.length).toBeGreaterThanOrEqual(2);

    const expectedDefaultRole = "researcher";
    const roles = loops!.slice(0, 2).map((l) => l.role || expectedDefaultRole);
    const researcherCount = roles.filter((r) => r === "researcher").length;
    const reviewerCount = roles.filter((r) => r === "research-reviewer").length;
    expect(researcherCount, `expected 1 researcher loop, got roles=${JSON.stringify(roles)}`).toBe(1);
    expect(reviewerCount, `expected 1 research-reviewer loop, got roles=${JSON.stringify(roles)}`).toBe(1);

    // -----------------------------------------------------------------
    // Both loops reach terminal complete. Approved decision matches no
    // rule, so the loop count must settle at 2.
    // -----------------------------------------------------------------
    const allTerminal = await pollUntil(async () => {
      const resp = await request.get("/teams-dispatch/loops");
      if (!resp.ok()) return null;
      const list = (await resp.json()) as Array<{ state: string }>;
      if (list.length !== 2) return null;
      return list.every((l) => l.state === "complete") ? list : null;
    }, { timeoutMs: 30000 });

    expect(allTerminal, "expected both loops to reach terminal complete").toBeTruthy();

    // -----------------------------------------------------------------
    // No rule should re-fire after approval. Settle interval guards
    // against a regression that would re-spawn a researcher.
    // -----------------------------------------------------------------
    await new Promise((r) => setTimeout(r, 1500));
    const finalList = await request.get("/teams-dispatch/loops").then((r) => r.json()) as unknown[];
    expect(finalList.length, "approved decision must not spawn additional loops").toBe(2);

    // -----------------------------------------------------------------
    // Verify the research.artifact.{loop_id} payload was published with
    // the harness field. Use the subject filter so the query isn't
    // crowded out by graph.mutation.triple.add entries (which produce
    // dozens per loop). The message-logger captures research.artifact.>
    // subjects per the config's monitor list.
    // -----------------------------------------------------------------
    // message-logger filter uses * wildcards (not NATS-style >). Note
    // that the subject filter applies AFTER the limit window — so a
    // small limit can miss the artifact publish (which lands early in
    // the chain) when graph.mutation.triple.add traffic dominates the
    // tail. Use a large enough window to cover the whole chain.
    const artifactResp = await request.get(
      "/message-logger/entries?limit=500&subject=research.artifact.*",
    );
    expect(artifactResp.ok(), "/message-logger/entries returned non-OK").toBe(true);
    // The message-logger entry parses JSON payloads into `raw_data`;
    // the literal bytes are not exposed as a string field.
    const artifactPayloads = (await artifactResp.json()) as Array<{
      subject: string;
      raw_data?: { harness?: string };
    }>;

    expect(
      artifactPayloads.length,
      `expected at least one research.artifact.{loop_id} payload`,
    ).toBeGreaterThanOrEqual(1);

    // The payload should carry the harness field round-trip.
    const haveHarness = artifactPayloads.some(
      (m) => m.raw_data?.harness === "stub",
    );
    expect(
      haveHarness,
      `expected at least one artifact payload with harness="stub"; subjects=${artifactPayloads.map((m) => m.subject).join(", ")}; harness_values=${artifactPayloads.map((m) => m.raw_data?.harness).join(" | ")}`,
    ).toBe(true);
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
