import { test, expect, type APIRequestContext } from "@playwright/test";

type Loop = { loop_id: string; task_id: string; role?: string; state: string; result?: string };
type Entry = {
  subject: string;
  message_type: string;
  raw_data?: { payload?: {
    task_id?: string; role?: string; prompt?: string; parent_loop_id?: string;
    in_reply_to?: string; type?: string; content?: string;
  } };
};
type Triple = { subject: string; object: unknown };
const subtopics = [
  "MQTT vs NATS latency on constrained ARM",
  "MQTT vs NATS memory footprint on constrained ARM",
];

async function entries(request: APIRequestContext): Promise<Entry[]> {
  const response = await request.get("/message-logger/entries?limit=500");
  expect(response.ok()).toBe(true);
  return response.json();
}

// One identical contract runs against beta.160 and the frozen version. A baseline
// failure at terminal delivery is retained: a flat empty rule publish is not a reply.
test("two gather identities join once before synthesis, review and terminal delivery", async ({
  page, request,
}, testInfo) => {
  test.setTimeout(180_000);
  await page.goto("/");
  await expect(page.getByTestId("connection-status")).toHaveAttribute("data-summary", "healthy");
  await page.getByTestId("chat-input").fill("Compare MQTT vs NATS latency and memory on constrained ARM devices.");
  await page.getByTestId("send-button").click();

  let loops: Loop[] = [];
  await expect.poll(async () => {
    const response = await request.get("/teams-dispatch/loops");
    if (!response.ok()) return false;
    loops = await response.json();
    return loops.length === 8 && loops.every((loop) => loop.state === "complete");
  }, { timeout: 120_000, message: "both gather branches and the joined research run must complete" }).toBe(true);

  const forRole = (role: string) => loops.filter((loop) => (loop.role || "coordinator") === role);
  expect(forRole("coordinator")).toHaveLength(2);
  expect(forRole("researcher-research-plan")).toHaveLength(1);
  expect(forRole("researcher-research-gather")).toHaveLength(2);
  expect(forRole("researcher-research-synthesize")).toHaveLength(1);
  expect(forRole("reviewer-research")).toHaveLength(1);
  expect(forRole("ops-chain-observer")).toHaveLength(1);
  const planner = forRole("researcher-research-plan")[0];
  const gatherers = forRole("researcher-research-gather");
  const gatherIDs = gatherers.map((loop) => loop.loop_id).sort();
  expect(new Set(gatherIDs).size).toBe(2);
  expect(JSON.parse(planner.result!).subtopics).toEqual(subtopics);
  const synthesis = forRole("researcher-research-synthesize")[0];
  const reviewer = forRole("reviewer-research")[0];
  const finalCoordinator = forRole("coordinator").find((loop) =>
    JSON.parse(loop.result!).action === "respond_direct");
  expect(finalCoordinator).toBeDefined();

  const graph = await request.get("/graph/triples?predicate=research.gather.completed-subtopic&limit=20");
  expect(graph.ok()).toBe(true);
  const joined: Triple[] = await graph.json();
  expect(joined).toHaveLength(2);
  expect(new Set(joined.map((triple) => triple.subject)).size).toBe(1);
  expect(joined[0].subject.endsWith(`.${planner.loop_id}`)).toBe(true);
  // Compare returned identities without assuming historical domain/system order.
  expect(joined.map((triple) => String(triple.object).split(".").at(-1)).sort()).toEqual(gatherIDs);

  const messages = await entries(request);
  const taskEntries = messages.filter((entry) => entry.message_type === "agentic.task.v1");
  const gatherTasks = taskEntries.filter((entry) => entry.raw_data?.payload?.role === "researcher-research-gather");
  expect(gatherTasks).toHaveLength(2);
  expect(gatherTasks.map((entry) => entry.raw_data!.payload!.task_id).sort())
    .toEqual(gatherers.map((loop) => loop.task_id).sort());
  for (const subtopic of subtopics) {
    expect(gatherTasks.filter((entry) => entry.raw_data!.payload!.prompt?.includes(`**Your subtopic:** ${subtopic}`)))
      .toHaveLength(1);
  }
  for (const entry of gatherTasks) expect(entry.raw_data!.payload!.parent_loop_id).toBe(planner.loop_id);
  const synthTasks = taskEntries.filter((entry) => entry.raw_data?.payload?.task_id === synthesis.task_id);
  expect(synthTasks).toHaveLength(1);
  const synthPayload = synthTasks[0].raw_data!.payload!;
  expect(synthPayload.parent_loop_id).toBe(planner.loop_id);
  const siblingJSON = synthPayload.prompt!.match(/Sibling gather loop IDs \(inlined here as a JSON array\): (\[[^\n]*?\])/);
  expect(siblingJSON).not.toBeNull();
  expect((JSON.parse(siblingJSON![1]) as string[]).sort()).toEqual(joined.map((triple) => String(triple.object)).sort());
  expect(taskEntries.find((entry) => entry.raw_data?.payload?.task_id === reviewer.task_id)?.raw_data?.payload?.parent_loop_id)
    .toBe(synthesis.loop_id);
  expect(taskEntries.find((entry) => entry.raw_data?.payload?.task_id === finalCoordinator!.task_id)?.raw_data?.payload?.parent_loop_id)
    .toBe(reviewer.loop_id);
  const phases = await request.get("/graph/triples?predicate=agent.run.phase&limit=20");
  const runPhases: Triple[] = await phases.json();
  expect(runPhases.map((triple) => triple.object)).toContain("completed");

  // Save structural proof even if the next, stronger delivery assertion fails.
  await testInfo.attach("fanout-join-proof", { contentType: "application/json", body: JSON.stringify({
    loops, joined, runPhases, tasks: taskEntries.map((entry) => ({ subject: entry.subject,
      ...entry.raw_data?.payload, })),
  }, null, 2) });
  let responses: Entry[] = [];
  await expect.poll(async () => {
    responses = (await entries(request)).filter((entry) => entry.subject.startsWith("user.response."));
    return responses.filter((entry) => entry.message_type === "agentic.user_response.v1"
      && entry.raw_data?.payload?.type === "result"
      && entry.raw_data?.payload?.in_reply_to === finalCoordinator!.loop_id).length;
  }, { timeout: 10_000, message: "terminal coordinator must deliver exactly one typed result" }).toBe(1);
  expect(responses.find((entry) => entry.raw_data?.payload?.in_reply_to === finalCoordinator!.loop_id)
    ?.raw_data?.payload?.content).toContain("For IoT edge deployments");
});
