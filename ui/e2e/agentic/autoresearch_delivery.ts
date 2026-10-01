import { expect, type APIRequestContext, type Page, type TestInfo } from "@playwright/test";

export interface AutoresearchLoop {
  loop_id?: string;
  task_id?: string;
  role?: string | null;
  state?: string;
  result?: string;
}

interface Payload {
  loop_id?: string;
  task_id?: string;
  role?: string;
  run_id?: string;
  parent_loop_id?: string;
  channel_type?: string;
  channel_id?: string;
  user_id?: string;
  type?: string;
  content?: string;
  in_reply_to?: string;
  metadata?: {
    "agent.decide.action_allowlist"?: string[];
    "agent.related_loops"?: Record<string, string>;
  };
}
export interface MessageEntry {
  subject: string;
  message_type: string;
  raw_data?: { payload?: Payload };
}

export async function readMessages(request: APIRequestContext): Promise<MessageEntry[]> {
  // Filtering is deliberately client-side after a full journey-sized pull. The
  // old subject_prefix parameter was unsupported and counted unrelated traffic.
  const response = await request.get("/message-logger/entries?limit=500");
  expect(response.ok()).toBe(true);
  return response.json();
}

export async function assertAutoresearchTerminalDelivery(
  page: Page,
  request: APIRequestContext,
  testInfo: TestInfo,
  loops: AutoresearchLoop[],
  options: {
    content: string;
    sourceRole: string;
    relatedLoopKey: string;
    phase: "completed" | "failed" | "awaiting_approval" | "executing";
    action?: "respond_direct" | "ask_user";
    allowlist?: string[];
  },
): Promise<void> {
  const action = options.action ?? "respond_direct";
  const roots = loops.filter((loop) => loop.task_id?.startsWith("dispatch-")
    && loop.result && JSON.parse(loop.result).action === "autoresearch");
  expect(roots, "one actual front-door autoresearch coordinator").toHaveLength(1);
  const root = roots[0];
  const terminals = loops.filter((loop) => loop.role === "coordinator" && !loop.task_id?.startsWith("dispatch-")
    && loop.result && JSON.parse(loop.result).action === action);
  expect(terminals, "exactly one terminal coordinator, not an intermediate completion").toHaveLength(1);
  const terminal = terminals[0];
  const sources = loops.filter((loop) => loop.role === options.sourceRole);
  expect(sources, "one source loop for the terminal handoff").toHaveLength(1);
  const source = sources[0];
  const messages = await readMessages(request);
  const tasks = messages.filter((entry) => entry.message_type === "agentic.task.v1");
  const rootTasks = tasks.filter((entry) => entry.raw_data?.payload?.task_id === root.task_id);
  const terminalTasks = tasks.filter((entry) => entry.raw_data?.payload?.task_id === terminal.task_id);
  expect(rootTasks).toHaveLength(1);
  expect(terminalTasks, "no duplicate terminal task publish").toHaveLength(1);
  const rootTask = rootTasks[0].raw_data!.payload!;
  const terminalTask = terminalTasks[0].raw_data!.payload!;
  expect(rootTask.loop_id).toBe(root.loop_id);
  expect(rootTask.channel_type).toBe("http");
  expect(rootTask.channel_id).toBeTruthy();
  expect(rootTask.user_id).toBe("ui-anonymous");
  // The repair deliberately restores native typed lineage only at the terminal
  // handoff. related_loops cannot substitute for these dispatcher-owned fields.
  expect(terminalTask.parent_loop_id).toBe(root.loop_id);
  expect(terminalTask.run_id).toBe(root.loop_id);
  expect(terminalTask.metadata?.["agent.decide.action_allowlist"])
    .toEqual(options.allowlist ?? ["respond_direct"]);
  const related = terminalTask.metadata?.["agent.related_loops"];
  expect(related?.["autoresearch-run"]).toBe(root.loop_id);
  const sourceEntity = related?.[options.relatedLoopKey]?.split(".");
  expect(sourceEntity).toHaveLength(6);
  expect(sourceEntity?.slice(2)).toEqual(["agentic-loop", "agent", "execution", source.loop_id]);

  let replies: MessageEntry[] = [];
  await expect.poll(async () => {
    replies = (await readMessages(request)).filter((entry) => entry.subject.startsWith("user.response.")
      && entry.raw_data?.payload?.in_reply_to === terminal.loop_id);
    return replies.length;
  }, { timeout: 10_000, message: "terminal coordinator must publish exactly one actual response" }).toBe(1);
  const reply = replies[0];
  expect(reply.message_type).toBe("agentic.user_response.v1");
  const payload = reply.raw_data!.payload!;
  expect(payload).toMatchObject({
    type: action === "ask_user" ? "prompt" : "result",
    in_reply_to: terminal.loop_id,
    channel_type: rootTask.channel_type,
    channel_id: rootTask.channel_id,
    user_id: rootTask.user_id,
  });
  expect(reply.subject).toBe(`user.response.${rootTask.channel_type}.${rootTask.channel_id}`);
  expect(payload.content).toContain(options.content);
  const routedResults = (await readMessages(request)).filter((entry) =>
    entry.subject === reply.subject && ["result", "prompt"].includes(entry.raw_data?.payload?.type ?? ""));
  expect(routedResults, "no intermediate loop or duplicate result may leak onto the original channel").toHaveLength(1);
  expect(routedResults[0].raw_data?.payload?.in_reply_to).toBe(terminal.loop_id);
  let phases: Array<{ subject: string; object: unknown }> = [];
  await expect.poll(async () => {
    const response = await request.get("/graph/triples?predicate=agent.run.phase&limit=50");
    expect(response.ok()).toBe(true);
    phases = await response.json();
    return phases.map((phase) => phase.object);
  }, { timeout: 10_000 }).toEqual([options.phase]);
  expect(phases, "terminal handoff reuses the run instead of minting another anchor").toHaveLength(1);
  const runParts = phases[0].subject.split(".");
  expect(runParts).toHaveLength(6);
  expect(runParts.slice(2)).toEqual(["chain", "agent", "execution", root.loop_id]);
  const originResponse = await request.get("/graph/triples?predicate=agent.run.origin-entity-id&limit=50");
  expect(originResponse.ok()).toBe(true);
  const origins: Array<{ subject: string; object: unknown }> = await originResponse.json();
  expect(origins).toHaveLength(1);
  expect(origins[0]).toMatchObject({ subject: phases[0].subject,
    object: [...runParts.slice(0, 2), "agentic-loop", "agent", "execution", root.loop_id].join("."),
  });
  await testInfo.attach("autoresearch-terminal-route-proof", {
    contentType: "application/json",
    body: JSON.stringify({ root, terminal, source, rootTask, terminalTask, reply, phases, origins }, null, 2),
  });

  // This is the existing raw-evidence UI. It proves the actual typed payload is
  // inspectable, not that rich model/artifact body rendering (#261) is repaired.
  await page.goto(`/?task=${root.loop_id}`);
  await expect(page.getByTestId("task-detail-panel")).toBeVisible();
  const child = page.locator(`[data-testid="child-item"][data-loop-id="${terminal.loop_id}"]`);
  await expect(child).toBeVisible({ timeout: 10_000 });
  await child.click();
  await expect(child).toHaveAttribute("aria-pressed", "true");
  await page.getByRole("tab", { name: "Evidence", exact: true }).click();
  const panel = page.getByTestId("run-evidence-panel");
  await panel.locator("summary", { hasText: "Message log" }).click();
  const row = panel.getByTestId("trace-row").filter({ hasText: reply.subject });
  await expect(row).toHaveCount(1, { timeout: 10_000 });
  await row.getByRole("button").click();
  const body = row.getByTestId("trace-row-expanded");
  await expect(body).toBeVisible();
  const rendered = JSON.parse(await body.innerText()) as { payload: Payload };
  expect(rendered.payload).toEqual(payload);
  await testInfo.attach("autoresearch-terminal-response-ui", {
    contentType: "image/png", body: await panel.screenshot(),
  });
}
