import { test, expect } from "@playwright/test";

/**
 * Frozen migration: the retained admin route shows admitted composition and
 * validation. Graph exploration uses returned canonical entity identities;
 * neither surface may request the retired runtime flow authoring API.
 */

test.describe("Admin flows inventory", () => {
  test.beforeAll(async ({ request }) => {
    const health = await request.get("/health");
    expect(health.ok(), "Backend not healthy — stack not running?").toBe(true);
  });

  test("/admin → Flows card → read-only inventory at /admin/flows", async ({
    page,
  }) => {
    await page.goto("/admin");

    await expect(page.getByTestId("admin-page")).toBeVisible();

    const flowsCard = page.getByTestId("admin-card-flows");
    await expect(flowsCard).toBeVisible();
    await expect(flowsCard).toContainText("Flows");

    await flowsCard.click();
    await expect(page).toHaveURL(/\/admin\/flows$/);

    const flowsPage = page.getByTestId("flows-page");
    await expect(flowsPage).toBeVisible();

    // Read-only contract is named in the page subtitle. The exact
    // wording is a stable user-facing claim — if it changes, the
    // editor regression guard below loses meaning.
    await expect(flowsPage).toContainText(
      "Read-only inventory",
    );
    await expect(flowsPage).toContainText(
      "Configured components and their admitted connections",
    );

    // A working frozen bootstrap must expose its admitted component inventory.
    await expect(page.getByTestId("error-banner")).toHaveCount(0);
    await expect(page.getByTestId("composition-validation")).toBeVisible();
    await expect(page.getByTestId("flow-list")).toContainText("teams-loop");
    await expect(page.getByTestId("flow-list")).toContainText("agentic-loop");

    // Regression guard — these affordances were removed in bffa800.
    // None of them must reappear without an explicit product-shape
    // decision.
    await expect(
      page.getByRole("button", { name: /create.*flow/i }),
    ).toHaveCount(0);
    await expect(
      page.getByRole("button", { name: /^new flow$/i }),
    ).toHaveCount(0);
    await expect(
      page.getByRole("link", { name: /create.*flow/i }),
    ).toHaveCount(0);
    await expect(
      page.getByRole("button", { name: /^delete$/i }),
    ).toHaveCount(0);
    await expect(
      page.getByRole("button", { name: /^edit$/i }),
    ).toHaveCount(0);
  });
  test("graph inspector preserves canonical entity identity without flowbuilder", async ({ page, request }) => {
    test.setTimeout(120_000);
    const retiredRequests: string[] = [];
    page.on("request", (outgoing) => {
      if (new URL(outgoing.url()).pathname.startsWith("/flowbuilder/")) retiredRequests.push(outgoing.url());
    });
    await page.goto("/");
    await expect(page.getByTestId("connection-status")).toHaveAttribute("data-summary", "healthy");
    await page.getByTestId("chat-input").fill("Compare MQTT vs NATS on constrained ARM devices.");
    await page.getByTestId("send-button").click();
    await expect.poll(async () => {
      const response = await request.get("/teams-dispatch/loops");
      if (!response.ok()) return false;
      const loops = await response.json() as Array<{ state: string }>;
      return loops.length === 7 && loops.every((loop) => loop.state === "complete");
    }, { timeout: 60_000 }).toBe(true);

    const graphResponse = page.waitForResponse((response) => new URL(response.url()).pathname === "/graphql"
      && (response.request().postData() ?? "").includes("GetEntitiesByPrefix"));
    await page.goto("/graph");
    const graph = await (await graphResponse).json();
    expect(graph.errors).toBeUndefined();
    const entities = graph.data.entitiesByPrefix.entities as Array<{ id: string }>;
    const entity = entities.find((item) => item.id.includes(".agentic-loop.agent.execution."));
    expect(entity, "graph must return an actual loop entity").toBeDefined();
    const [org, platform, system, domain, type, instance] = entity!.id.split(".");
    expect(system).toBe("agentic-loop");
    expect(domain).toBe("agent");
    expect(instance).toMatch(/^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/);
    await expect(page.getByTestId("data-view")).toBeVisible();
    await expect(page.getByTestId("sigma-canvas")).toContainText(`${entities.length} entities`);
    await expect(page.getByTestId("data-view").getByRole("alert")).toHaveCount(0);
    await expect(page.getByTestId("domain-filter-agent")).toBeVisible();
    await expect(page.getByTestId("domain-filter-agentic-loop")).toHaveCount(0);
    // Existing deterministic selection seam: no synthetic graph state or data.
    await page.evaluate((id) => window.__e2eSelectEntity!(id), entity!.id);
    const detail = page.getByTestId("graph-detail-panel");
    await expect(detail).toBeVisible();
    for (const [label, value] of Object.entries({ org, platform, system, domain, type, instance })) {
      await expect(detail.locator(".id-part").filter({ has: page.locator(".id-label", { hasText: new RegExp(`^${label}$`) }) })
        .locator(".id-value")).toHaveText(value);
    }
    await page.getByTestId("domain-filter-agent").click();
    await expect(page.getByTestId("domain-filter-agent")).toHaveClass(/active/);
    await page.getByTestId("reset-filters").click();
    await expect(page.getByTestId("sigma-canvas")).toContainText(`${entities.length} entities`);
    expect(retiredRequests).toEqual([]);
    for (const endpoint of ["/api/ai/chat", "/api/ai/generate-flow"]) {
      const response = await request.post(endpoint, { data: { prompt: "Create a flow" } });
      expect(response.status()).toBe(410);
    }
  });

});
