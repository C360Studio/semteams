import { test, expect } from "@playwright/test";

// Isolated qualification only: sandbox-mvp fixture, MockRunner, and a derived
// bootstrap with request_sandbox added to approval_required. Production keeps
// its existing policy. Each action gets a fresh stack/fixture cursor.
const action = process.env.APPROVAL_BOUNDARY_ACTION ?? "approve";
test(`pending sandbox request: ${action} preserves execution and identity boundaries`, async ({ page, request }) => {
  test.setTimeout(60_000);
  await page.goto("/");
  await expect(page.getByTestId("connection-status")).toHaveAttribute("data-summary", "healthy");
  await page.getByTestId("chat-input").fill("Prepare a restricted Go sandbox with task verification.");
  await page.getByTestId("send-button").click();
  let pending: { loop_id: string; state: string; user_id: string; pending_approval: { execution_id: string; tool_name: string } } | undefined;
  await expect.poll(async () => {
    const response = await request.get("/teams-dispatch/loops");
    if (!response.ok()) return false;
    pending = (await response.json()).find((loop: { state: string }) => loop.state === "awaiting_approval");
    return !!pending;
  }, { timeout: 15_000 }).toBe(true);
  expect(pending!.pending_approval.tool_name).toBe("request_sandbox");
  expect(pending!.pending_approval.execution_id).toBeTruthy();
  const id = pending!.loop_id;
  const identity = pending!.user_id;
  const url = `/teams-dispatch/loops/${id}/approval`;
  expect((await request.post(url, { data: { decision: "approve", user_id: identity } })).status()).toBe(400);
  expect((await request.post(url, { data: { decision: "approve", execution_id: "stale-execution", user_id: identity } })).status()).toBe(409);
  expect((await request.get(`/teams-dispatch/loops/${id}`).then((r) => r.json())).state).toBe("awaiting_approval");
  await page.goto(`/?task=${id}`);
  await expect(page.getByTestId("approval-tool-name")).toHaveText("request_sandbox");
  if (action === "cancel") {
    const refusal = await request.post("/teams-dispatch/message", { data: { content: `/cancel ${id}`, user_id: "migration-other-user" } });
    expect((await refusal.json()).type).toBe("error");
    await page.getByRole("button", { name: "Cancel", exact: true }).click();
  } else {
    await page.getByTestId(action === "reject" ? "approval-reject" : "approval-approve").click();
  }
  await expect.poll(async () => {
    const response = await request.get(`/teams-dispatch/loops/${id}`);
    return response.ok() ? (await response.json()).state : "unavailable";
  }, { timeout: 20_000 }).toBe(action === "cancel" ? "cancelled" : "complete");
  await expect(page.getByTestId("pending-approval-section")).toHaveCount(0);
  await expect.poll(async () => {
    const response = await request.post("/graphql", { data: { query: "query($loopId: String!) { trajectory(loopId: $loopId) { terminal_observed } }", variables: { loopId: id } } });
    return (await response.json()).data?.trajectory?.terminal_observed;
  }).toBe(true);
});
