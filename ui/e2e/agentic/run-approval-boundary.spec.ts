import { test, expect, type APIRequestContext } from "@playwright/test";

type Triple = { subject: string; predicate: string; object: unknown };
type Loop = {
  loop_id: string; task_id: string; role: string; state: string; result?: string;
  pending_approval?: { execution_id: string; tool_name: string; arguments: Record<string, unknown> };
};
async function triples(request: APIRequestContext, params: Record<string, string>): Promise<Triple[]> {
  const response = await request.get(`/graph/triples?${new URLSearchParams({ ...params, limit: "100" })}`);
  expect(response.ok()).toBe(true);
  return response.json();
}

// Target contract qualification only. The isolated derived config gates the
// planner's advertised emit_plan, after the research run anchor exists.
test("run-scoped approval joins exact execution receipt, resumes and delivers research", async ({ page, request }, testInfo) => {
  test.setTimeout(120_000);
  await page.goto("/");
  await expect(page.getByTestId("connection-status")).toHaveAttribute("data-summary", "healthy");
  await page.getByTestId("chat-input").fill("Compare MQTT vs NATS for IoT edge deployments.");
  await page.getByTestId("send-button").click();
  let loops: Loop[] = [];
  let planner: Loop | undefined;
  await expect.poll(async () => {
    const response = await request.get("/teams-dispatch/loops");
    if (!response.ok()) return false;
    loops = await response.json();
    planner = loops.find((loop) => loop.role === "researcher-research-plan" && loop.state === "awaiting_approval");
    return Boolean(planner?.pending_approval?.execution_id);
  }, { timeout: 20_000 }).toBe(true);
  const gate = planner!.pending_approval!;
  expect(gate.tool_name).toBe("emit_plan");
  const tuple = JSON.stringify([planner!.loop_id, gate.execution_id]);
  const root = loops.find((loop) => loop.task_id.startsWith("dispatch-"));
  expect(root).toBeDefined();
  let runEntity = "";
  let waiting: Triple[] = [];
  await expect.poll(async () => {
    const pending = await triples(request, { predicate: "agent.run.approval-pending" });
    runEntity = pending.find((triple) => triple.object === tuple)?.subject ?? "";
    if (!runEntity) return false;
    waiting = await triples(request, { subject: runEntity });
    return waiting.some((triple) => triple.predicate === "agent.run.phase" && triple.object === "awaiting_approval")
      && waiting.some((triple) => triple.predicate === "agent.run.approval-outstanding" && Number(triple.object) === 1);
  }, { timeout: 20_000, message: "the actual run owner/projection/rules must pause for this execution" }).toBe(true);
  expect(runEntity.endsWith(`.${root!.loop_id}`)).toBe(true);
  expect(waiting.filter((triple) => triple.predicate === "agent.run.approval-pending" && triple.object === tuple)).toHaveLength(1);
  expect(waiting.filter((triple) => triple.predicate === "agent.run.approval-answered")).toHaveLength(0);
  await testInfo.attach("run-approval-pending", { contentType: "application/json", body: JSON.stringify({ planner, runEntity, waiting }, null, 2) });

  await page.goto(`/?task=${root!.loop_id}`);
  const section = page.getByTestId("run-waiting-section");
  await expect(section).toBeVisible();
  await expect(section).toContainText("emit_plan");
  await expect(section).toContainText(String(gate.arguments.title));
  await expect(page.getByTestId("run-approve")).toBeEnabled();
  const responsePromise = page.waitForResponse((response) => new URL(response.url()).pathname
    === `/teams-dispatch/loops/${planner!.loop_id}/approval` && response.request().method() === "POST");
  await page.getByTestId("run-approve").click();
  const approvalResponse = await responsePromise;
  expect(approvalResponse.ok()).toBe(true);
  const submitted = approvalResponse.request().postDataJSON();
  expect(submitted).toMatchObject({ decision: "approve", execution_id: gate.execution_id });

  let finished: Triple[] = [];
  await expect.poll(async () => {
    finished = await triples(request, { subject: runEntity });
    return finished.some((triple) => triple.predicate === "agent.run.phase" && triple.object === "completed")
      && finished.some((triple) => triple.predicate === "agent.run.approval-outstanding" && Number(triple.object) === 0)
      && finished.some((triple) => triple.predicate === "agent.run.approval-answered" && triple.object === tuple);
  }, { timeout: 40_000, message: "answered receipt must reduce outstanding gates and allow the research run to complete" }).toBe(true);
  expect(finished.filter((triple) => triple.predicate === "agent.run.approval-pending" && triple.object === tuple)).toHaveLength(1);
  expect(finished.filter((triple) => triple.predicate === "agent.run.approval-answered" && triple.object === tuple)).toHaveLength(1);
  await expect(section).toHaveCount(0);
  await expect.poll(async () => {
    const response = await request.get("/teams-dispatch/loops");
    loops = await response.json();
    return loops.length === 7 && loops.every((loop) => loop.state === "complete" && !loop.pending_approval);
  }, { timeout: 15_000 }).toBe(true);
  const finalCoordinator = loops.find((loop) => loop.role === "coordinator" && loop.result?.includes('"action":"respond_direct"'));
  expect(finalCoordinator).toBeDefined();
  type Entry = { message_type: string; raw_data?: { payload?: { type?: string; in_reply_to?: string; content?: string } } };
  let results: Entry[] = [];
  await expect.poll(async () => {
    const response = await request.get("/message-logger/entries?limit=500&subject=user.response.*");
    const entries: Entry[] = await response.json();
    results = entries.filter((entry) => entry.message_type === "agentic.user_response.v1"
      && entry.raw_data?.payload?.type === "result" && entry.raw_data.payload.in_reply_to === finalCoordinator!.loop_id);
    return results.length;
  }, { timeout: 10_000 }).toBe(1);
  expect(results[0].raw_data?.payload?.content).toContain("For IoT edge deployments");
  await testInfo.attach("run-approval-settled", { contentType: "application/json", body: JSON.stringify({
    submitted, runEntity, finished, loops, results,
  }, null, 2) });
});
