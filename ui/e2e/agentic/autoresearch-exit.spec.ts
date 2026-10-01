import { test, expect } from "@playwright/test";
import { assertAutoresearchTerminalDelivery, readMessages, type AutoresearchLoop } from "./autoresearch_delivery";

interface Loop extends AutoresearchLoop {
  iterations?: number;
  max_iterations?: number;
  outcome?: string;
  error?: string;
}
const failure = process.env.FIXTURE === "autoresearch-propose-failed.yaml";
const statusOnly = process.env.FIXTURE === "autoresearch-descended-status.yaml";

test(`autoresearch ${failure ? "nonbudgeted failure" : statusOnly ? "descended limitation status" : "descended clarification"} returns to the original user route`, async ({
  page, request,
}, testInfo) => {
  test.setTimeout(120_000);
  expect(["autoresearch-propose-failed.yaml", "autoresearch-descended-clarification.yaml", "autoresearch-descended-status.yaml"])
    .toContain(process.env.FIXTURE);
  await page.goto("/");
  await expect(page.getByTestId("connection-status")).toHaveAttribute("data-summary", "healthy");
  await page.getByTestId("chat-input").fill(
    "Optimize go test ./... wallclock, lower is better, cap 2, bounded surface test/helpers. Keep all tests passing.",
  );
  await page.getByTestId("send-button").click();

  let loops: Loop[] = [];
  await expect.poll(async () => {
    const response = await request.get("/teams-dispatch/loops");
    if (!response.ok()) return false;
    loops = await response.json();
    // Wait for the required observer on actual failure; reaching failed and
    // starting its notification/observer are separate asynchronous events.
    return loops.length === (failure ? 5 : 4)
      && loops.every((loop) => ["complete", "failed"].includes(loop.state ?? ""))
      && loops.filter((loop) => loop.role === "coordinator").length === 2;
  }, { timeout: 80_000 }).toBe(true);
  expect(loops.filter((loop) => loop.role === "autoresearch-baseline")).toHaveLength(1);
  const proposes = loops.filter((loop) => loop.role === "autoresearch-propose");
  expect(proposes).toHaveLength(1);
  const propose = proposes[0];
  for (const role of ["autoresearch-execute", "autoresearch-synthesize", "reviewer-autoresearch"]) {
    expect(loops.filter((loop) => loop.role === role), `no ${role} after this exit`).toHaveLength(0);
  }
  if (failure) {
    expect(propose.state).toBe("failed");
    expect(propose.outcome).toBe("failed");
    expect(propose.max_iterations).toBe(8);
    expect(propose.iterations).toBe(propose.max_iterations);
    expect(propose.error).toContain("max iterations reached");
    expect(loops.filter((loop) => loop.state === "failed")).toHaveLength(1);
    expect(loops.find((loop) => loop.role === "ops-chain-observer")?.state).toBe("complete");
  } else {
    expect(propose.state).toBe("complete");
    expect(JSON.parse(propose.result!).action).toBe("needs_clarification");
    expect(loops.filter((loop) => loop.state === "failed")).toHaveLength(0);
  }

  const sourceMessages = (await readMessages(request)).filter((entry) =>
    entry.subject.startsWith("tool.result.") && entry.raw_data?.payload?.loop_id === propose.loop_id);
  expect(sourceMessages.length).toBeGreaterThan(0);
  for (const entry of sourceMessages) {
    const payload = entry.raw_data!.payload as { name?: string; error?: string };
    expect(payload.error ?? "", "an off-policy tool refusal is not the intended exit").toBe("");
    expect(payload.name).toBe(failure ? "scratchpad" : "decide");
  }
  const graph = async (predicate: string) => {
    const response = await request.get(`/graph/triples?predicate=${predicate}&limit=50`);
    expect(response.ok()).toBe(true);
    return response.json() as Promise<Array<{ subject: string; object: unknown }>>;
  };
  const best = await graph("autoresearch.best.value");
  expect(best).toHaveLength(1);
  expect(Number(best[0].object)).toBe(1.2);
  expect(await graph("autoresearch.measurement.value")).toHaveLength(0);
  expect(await graph("autoresearch.artifact.path")).toHaveLength(0);
  await testInfo.attach("autoresearch-exit-proof", {
    contentType: "application/json", body: JSON.stringify({ loops, sourceMessages, best }, null, 2),
  });
  await assertAutoresearchTerminalDelivery(page, request, testInfo, loops, {
    content: failure ? "Autoresearch could not continue" : statusOnly
      ? "Autoresearch stopped before an experiment because the current surface is too narrow"
      : "Which additional test directories may the optimizer change?",
    sourceRole: "autoresearch-propose", relatedLoopKey: failure ? "failed" : "rejecting",
    // Existing lifecycle policy: a limitation reply does not claim completion
    // or stamp failure. The run remains executing; ask_user instead waits.
    phase: failure ? "failed" : statusOnly ? "executing" : "awaiting_approval",
    action: failure || statusOnly ? "respond_direct" : "ask_user",
    allowlist: failure ? ["respond_direct"] : ["respond_direct", "ask_user"],
  });
});
