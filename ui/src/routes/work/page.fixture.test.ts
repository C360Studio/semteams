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
import * as itemRoute from "../api/work/items/[owner]/[repo]/[number]/+server";
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

// The board-mvp set has no item with both a resolved and an unresolved binding, which
// is the one case where the server answers `partial` with a run in it. A test can ask
// for that shape on top of the real run: the real handler's run, lookup downgraded the
// way the service does for an unmatched sibling binding.
let downgradeRunsToPartial = false;
async function runsResponse(params: { owner: string; repo: string; number: string }): Promise<Response> {
  const real = await runsRoute.GET({ params } as never);
  if (!downgradeRunsToPartial) return real;
  const body = (await real.json()) as Record<string, unknown>;
  return new Response(
    JSON.stringify({ ...body, lookup: "partial", reason: "prompt binding matched no coordinator loop" }),
    { status: real.status, headers: { "Content-Type": "application/json" } },
  );
}

function serve(input: Parameters<typeof fetch>[0]): Promise<Response> | Response {
  const raw = String(input);
  // The runtime backend is read by the server with an absolute URL.
  if (!raw.startsWith("/")) return backend(new URL(raw));

  const url = new URL(raw, "http://ui.test");
  if (url.pathname === "/api/work/portfolio") return portfolioRoute.GET({} as never);
  if (url.pathname === "/api/work/items") return itemsRoute.GET({ url } as never);
  const runs = /^\/api\/work\/items\/([^/]+)\/([^/]+)\/(\d+)\/runs$/.exec(url.pathname);
  if (runs) return runsResponse({ owner: runs[1], repo: runs[2], number: runs[3] });
  // The single-item read the page falls back to for a ?item= the lists do not carry.
  const single = /^\/api\/work\/items\/([^/]+)\/([^/]+)\/(\d+)$/.exec(url.pathname);
  if (single) return itemRoute.GET({ params: { owner: single[1], repo: single[2], number: single[3] } } as never);
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
  downgradeRunsToPartial = false;
  setEnv({});
  vi.unstubAllGlobals();
});

const open = (search: string) => {
  (pageMock.url as URL & { search: string }).search = search;
};
const itemParam = (ref: string) => `?item=${encodeURIComponent(ref)}`;
const cardOf = (ref: string) =>
  within(screen.getAllByTestId("work-item-card").find((card) => card.getAttribute("data-item") === ref)!);
const rowOf = (ref: string) =>
  within(screen.getAllByTestId("work-item-row").find((row) => row.getAttribute("data-item") === ref)!);

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

  describe("badges after an item's runs are loaded", () => {
    it("shows 'no linked run' on the card for an item that declares none, and leaves other cards unknown", async () => {
      const user = userEvent.setup();
      open(itemParam("c360studio/semteams#311"));
      render(WorkPage);

      await user.click(await screen.findByTestId("load-linked-runs"));
      await screen.findByTestId("no-linked-run");

      const card = cardOf("c360studio/semteams#311");
      for (const name of ["execution-stage", "needs-you", "verification"]) {
        expect(card.getByTestId(`overlay-${name}`)).toHaveAttribute("data-state", "none");
        expect(card.getByTestId(`overlay-${name}`)).toHaveTextContent("no linked run");
      }
      // Only the opened item's runs were read.
      expect(cardOf("c360studio/semteams#310").getByTestId("overlay-needs-you")).toHaveAttribute(
        "data-state",
        "unknown",
      );
      expect(cardOf("c360studio/semteams#310").getByTestId("overlay-needs-you")).toHaveAttribute(
        "title",
        "linked runs are read when an item is opened",
      );
    });

    it("shows unknown with the reason on the card for a binding whose run entity does not exist", async () => {
      const user = userEvent.setup();
      open(itemParam("c360studio/semteams#313"));
      render(WorkPage);

      await user.click(await screen.findByTestId("load-linked-runs"));
      await waitFor(() =>
        expect(screen.getByTestId("work-linked-runs")).toHaveAttribute("data-lookup", "partial"),
      );

      const card = cardOf("c360studio/semteams#313");
      for (const name of ["execution-stage", "needs-you", "verification"]) {
        expect(card.getByTestId(`overlay-${name}`)).toHaveAttribute("data-state", "unknown");
        expect(card.getByTestId(`overlay-${name}`).getAttribute("title")).toMatch(/has no facts/);
      }
    });

    it("shows the bound run's stage and needs-you on the card, with verification unknown", async () => {
      const user = userEvent.setup();
      open(itemParam("c360studio/semteams#312"));
      render(WorkPage);

      await user.click(await screen.findByTestId("load-linked-runs"));
      await screen.findByTestId("work-drill-in");

      const card = cardOf("c360studio/semteams#312");
      expect(card.getByTestId("overlay-execution-stage")).toHaveTextContent("completed");
      expect(card.getByTestId("overlay-needs-you")).toHaveTextContent("no");
      expect(card.getByTestId("overlay-verification")).toHaveAttribute("data-state", "unknown");
    });

    it("shows the same three cases in the table, run count included", async () => {
      const user = userEvent.setup();

      // Declares no runs.
      open(`?view=table&item=${encodeURIComponent("c360studio/semteams#311")}`);
      const none = render(WorkPage);
      await user.click(await screen.findByTestId("load-linked-runs"));
      await screen.findByTestId("no-linked-run");
      expect(rowOf("c360studio/semteams#311").getByTestId("overlay-linked-runs")).toHaveAttribute("data-state", "none");
      expect(rowOf("c360studio/semteams#311").getByTestId("overlay-execution-stage")).toHaveTextContent(
        "no linked run",
      );
      none.unmount();

      // Bound to a run entity that does not exist.
      open(`?view=table&item=${encodeURIComponent("c360studio/semteams#313")}`);
      const missing = render(WorkPage);
      await user.click(await screen.findByTestId("load-linked-runs"));
      await waitFor(() =>
        expect(screen.getByTestId("work-linked-runs")).toHaveAttribute("data-lookup", "partial"),
      );
      for (const name of ["execution-stage", "needs-you", "verification", "linked-runs"]) {
        expect(rowOf("c360studio/semteams#313").getByTestId(`overlay-${name}`)).toHaveAttribute(
          "data-state",
          "unknown",
        );
      }
      missing.unmount();

      // Bound to a real run.
      open(`?view=table&item=${encodeURIComponent("c360studio/semteams#312")}`);
      render(WorkPage);
      await user.click(await screen.findByTestId("load-linked-runs"));
      await screen.findByTestId("work-drill-in");
      const bound = rowOf("c360studio/semteams#312");
      expect(bound.getByTestId("overlay-execution-stage")).toHaveTextContent("completed");
      expect(bound.getByTestId("overlay-linked-runs")).toHaveTextContent("1 run");
    });

    it("does not report no-needs-you or a run count when the lookup was partial but a run resolved", async () => {
      downgradeRunsToPartial = true;
      const user = userEvent.setup();
      open(itemParam("c360studio/semteams#312"));
      const board = render(WorkPage);

      await user.click(await screen.findByTestId("load-linked-runs"));
      await waitFor(() =>
        expect(screen.getByTestId("work-linked-runs")).toHaveAttribute("data-lookup", "partial"),
      );

      const card = cardOf("c360studio/semteams#312");
      for (const name of ["execution-stage", "needs-you", "verification"]) {
        expect(card.getByTestId(`overlay-${name}`)).toHaveAttribute("data-state", "unknown");
        expect(card.getByTestId(`overlay-${name}`).getAttribute("title")).toMatch(
          /^at least 1 run; lookup partial: prompt binding matched no coordinator loop$/,
        );
      }
      // The run itself is still shown with what is known of it.
      expect(within(screen.getByTestId("linked-run-row")).getByTestId("overlay-execution-stage")).toHaveTextContent(
        "completed",
      );
      board.unmount();

      open(`?view=table&item=${encodeURIComponent("c360studio/semteams#312")}`);
      render(WorkPage);
      await user.click(await screen.findByTestId("load-linked-runs"));
      await waitFor(() =>
        expect(screen.getByTestId("work-linked-runs")).toHaveAttribute("data-lookup", "partial"),
      );
      const row = rowOf("c360studio/semteams#312");
      expect(row.getByTestId("overlay-needs-you")).toHaveAttribute("data-state", "unknown");
      expect(row.getByTestId("overlay-linked-runs")).toHaveAttribute("data-state", "unknown");
      expect(row.getByTestId("overlay-linked-runs").getAttribute("title")).toMatch(/^at least 1 run; lookup partial/);
    });
  });

  describe("?item= deep links the lists do not carry", () => {
    it("reads the single-item endpoint and shows the server's own not-found message", async () => {
      open(itemParam("c360studio/semteams#999"));
      render(WorkPage);

      const state = await screen.findByTestId("work-item-state");
      await waitFor(() =>
        expect(within(state).getByRole("alert")).toHaveTextContent("c360studio/semteams#999 does not exist"),
      );
      const itemReads = vi
        .mocked(fetch)
        .mock.calls.filter(([input]) => String(input) === "/api/work/items/c360studio/semteams/999");
      expect(itemReads).toHaveLength(1);
    });

    it("says an unconfigured repository is not in the portfolio", async () => {
      open(itemParam("elsewhere/other#1"));
      render(WorkPage);

      const state = await screen.findByTestId("work-item-state");
      await waitFor(() =>
        expect(within(state).getByRole("alert")).toHaveTextContent("repository elsewhere/other is not in the portfolio"),
      );
    });
  });
});
