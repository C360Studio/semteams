// The /work page against the real board-mvp fixture, through the real route
// handlers, with only the runtime backend faked. This is the check that the
// components handle every shape the server produces: Project columns, the
// GitHub-field fallback, delivery context, and the three linkage cases.
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

vi.mock("$env/dynamic/private", async () => ({
  env: (await import("$lib/server/work/testkit")).mockEnv,
}));
const pageMock = vi.hoisted(() => ({ url: null as unknown as URL, state: {} }));
vi.mock("$app/state", () => ({ page: pageMock }));
vi.mock("$app/navigation", () => ({ replaceState: vi.fn() }));

import { render, screen, waitFor, within } from "@testing-library/svelte";
import userEvent from "@testing-library/user-event";
import WorkPage from "./+page.svelte";
import * as portfolioRoute from "../api/work/portfolio/+server";
import * as itemsRoute from "../api/work/items/+server";
import * as runsRoute from "../api/work/items/[owner]/[repo]/[number]/runs/+server";
import { fakeBackend, setEnv, triple } from "$lib/server/work/testkit";

const PROMPT =
  "Compare MQTT vs NATS for IoT edge deployments — which has lower latency on constrained ARM devices?";
const LOOP_ID = "loop_7b3b867e";
const LOOP_ENTITY = `c360.semteams.agentic-loop.agent.execution.${LOOP_ID}`;
const RUN = `c360.semteams.chain.agent.execution.${LOOP_ID}`;

const backend = fakeBackend({
  triples: [
    triple(LOOP_ENTITY, "agent.loop.description", PROMPT),
    triple(LOOP_ENTITY, "agent.loop.role", "coordinator"),
    triple(LOOP_ENTITY, "agent.loop.run", LOOP_ID),
    triple(LOOP_ENTITY, "agent.run.entity-id", RUN),
    triple(RUN, "agent.run.phase", "completed"),
  ],
  loops: { [LOOP_ID]: { loop_id: LOOP_ID, state: "complete" } },
});

function serve(input: Parameters<typeof fetch>[0]): Promise<Response> | Response {
  const raw = String(input);
  // The runtime backend is read by the server with an absolute URL.
  if (!raw.startsWith("/")) return backend(new URL(raw));

  const url = new URL(raw, "http://ui.test");
  if (url.pathname === "/api/work/portfolio") return portfolioRoute.GET({} as never);
  if (url.pathname === "/api/work/items") return itemsRoute.GET({ url } as never);
  const runs = /^\/api\/work\/items\/([^/]+)\/([^/]+)\/(\d+)\/runs$/.exec(url.pathname);
  if (runs) return runsRoute.GET({ params: { owner: runs[1], repo: runs[2], number: runs[3] } } as never);
  return new Response(JSON.stringify({ code: "TEST_UNROUTED", error: raw }), { status: 404 });
}

beforeEach(async () => {
  setEnv({ WORK_SOURCE: "fixture", WORK_FIXTURE_SET: "board-mvp" });
  vi.stubGlobal("fetch", vi.fn(async (input: Parameters<typeof fetch>[0]) => serve(input)));
  const { SvelteURL } = await import("svelte/reactivity");
  pageMock.url = new SvelteURL("http://localhost/work");
  const { replaceState } = await import("$app/navigation");
  vi.mocked(replaceState).mockImplementation((next) => {
    (pageMock.url as InstanceType<typeof SvelteURL>).search = new URL(String(next)).search;
  });
});

afterEach(() => {
  setEnv({});
  vi.unstubAllGlobals();
});

const open = (search: string) => {
  (pageMock.url as URL & { search: string }).search = search;
};
const itemParam = (ref: string) => `?item=${encodeURIComponent(ref)}`;

describe("/work with the board-mvp fixture", () => {
  it("renders both repositories with their own columns and every item", async () => {
    render(WorkPage);

    const semteams = within(await screen.findByTestId("work-repository-c360studio-semteams"));
    await waitFor(() => expect(semteams.getAllByTestId("work-item-card")).toHaveLength(7));
    expect(
      semteams.getAllByTestId(/^work-column-/).map((column) => column.getAttribute("data-testid")),
    ).toEqual(["work-column-todo", "work-column-in-progress", "work-column-in-review", "work-column-done"]);

    const semsource = within(screen.getByTestId("work-repository-c360studio-semsource"));
    await waitFor(() => expect(semsource.getAllByTestId("work-item-card")).toHaveLength(4));
    expect(
      semsource.getAllByTestId(/^work-column-/).map((column) => column.getAttribute("data-testid")),
    ).toEqual(["work-column-todo", "work-column-in-progress", "work-column-done"]);
    // GitHub-field fallback: closed -> Done, assigned -> In Progress.
    const done = within(semsource.getByTestId("work-column-done"));
    expect(done.getByTestId("work-item-card")).toHaveAttribute("data-item", "c360studio/semsource#53");
    expect(screen.queryByTestId("work-lookup")).not.toBeInTheDocument();
  });

  it("renders the same items in the table view", async () => {
    open("?view=table");
    render(WorkPage);

    await waitFor(() => expect(screen.getAllByTestId("work-item-row")).toHaveLength(11));
    expect(screen.getByTestId("work-table")).toBeInTheDocument();
  });

  it("shows an item that declares no runs as 'no linked run' once its runs are loaded", async () => {
    const user = userEvent.setup();
    open(itemParam("c360studio/semteams#311"));
    render(WorkPage);

    await user.click(await screen.findByTestId("load-linked-runs"));

    expect(await screen.findByTestId("no-linked-run")).toHaveTextContent("no linked run");
    expect(screen.getByTestId("work-linked-runs")).toHaveAttribute("data-lookup", "complete");
  });

  it("shows an item bound to a missing run entity as a partial lookup with unknown overlays", async () => {
    const user = userEvent.setup();
    open(itemParam("c360studio/semteams#313"));
    render(WorkPage);

    await user.click(await screen.findByTestId("load-linked-runs"));

    const section = await screen.findByTestId("work-linked-runs");
    await waitFor(() => expect(section).toHaveAttribute("data-lookup", "partial"));
    expect(within(section).queryByTestId("no-linked-run")).not.toBeInTheDocument();
    const row = within(within(section).getByTestId("linked-run-row"));
    expect(row.getByTestId("overlay-execution-stage")).toHaveAttribute("data-state", "unknown");
    expect(row.getByTestId("overlay-execution-stage").getAttribute("title")).toMatch(/has no facts/);
    expect(row.getByTestId("work-drill-in-unavailable")).toBeInTheDocument();
    expect(row.queryByTestId("work-drill-in")).not.toBeInTheDocument();
    // The card keeps its Project column: a run never moves it.
    expect(
      within(screen.getByTestId("work-repository-c360studio-semteams"))
        .getAllByTestId("work-item-card")
        .find((card) => card.getAttribute("data-item") === "c360studio/semteams#313"),
    ).toHaveAttribute("data-column", "In Progress");
  });

  it("drills into the run bound by coordinator prompt", async () => {
    const user = userEvent.setup();
    open(itemParam("c360studio/semteams#312"));
    render(WorkPage);

    await user.click(await screen.findByTestId("load-linked-runs"));

    const link = await screen.findByTestId("work-drill-in");
    expect(link).toHaveAttribute("href", `/?task=${LOOP_ID}`);
    const row = within(screen.getByTestId("linked-run-row"));
    expect(row.getByTestId("overlay-execution-stage")).toHaveTextContent("completed");
    expect(row.getByTestId("overlay-needs-you")).toHaveTextContent("no");
    expect(row.getByTestId("overlay-verification")).toHaveAttribute("data-state", "unknown");
  });

  it("shows delivery context for the item with linked pull requests and dependencies for another", async () => {
    open(itemParam("c360studio/semteams#314"));
    const first = render(WorkPage);
    const delivery = await screen.findByTestId("overview-delivery");
    expect(delivery).toHaveTextContent("c360studio/semteams#320");
    expect(delivery).toHaveTextContent("c360studio/semteams#321");
    first.unmount();

    open(itemParam("c360studio/semteams#310"));
    render(WorkPage);
    expect(await screen.findByTestId("overview-dependencies")).toHaveTextContent(
      "c360studio/semteams#314 (blocked by)",
    );
    expect(screen.getByTestId("overview-delivery")).toHaveAttribute("data-state", "unknown");
  });
});
