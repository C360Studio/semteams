import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { render, screen, waitFor, within } from "@testing-library/svelte";
import userEvent from "@testing-library/user-event";
import { replaceState } from "$app/navigation";
import WorkPage from "./+page.svelte";
import {
  json,
  makeItem,
  makeItemsResponse,
  makePortfolio,
  makeRun,
  makeRunsResponse,
  stubWorkFetch,
} from "../../test-utils/work";
import type { Route } from "../../test-utils/work";

// A reactive URL stands in for SvelteKit's page.url so replaceState round-trips
// into the page the way shallow routing does.
const pageMock = vi.hoisted(() => ({ url: null as unknown as URL, state: {} }));
vi.mock("$app/state", () => ({ page: pageMock }));
vi.mock("$app/navigation", () => ({ replaceState: vi.fn() }));

const widgets = [
  makeItem({ ref: "acme/widgets#1", number: 1, title: "Todo widget", column: "Todo" }),
  makeItem({ ref: "acme/widgets#2", number: 2, title: "Done widget", column: "Done", state: "closed" }),
];

const boardRoutes: Route[] = [
  (url) => (url.pathname === "/api/work/portfolio" ? json(makePortfolio(["acme/widgets"])) : undefined),
  (url) =>
    url.pathname === "/api/work/items" ? json(makeItemsResponse("acme/widgets", widgets)) : undefined,
];

beforeEach(async () => {
  const { SvelteURL } = await import("svelte/reactivity");
  pageMock.url = new SvelteURL("http://localhost/work");
  vi.mocked(replaceState).mockImplementation((next) => {
    (pageMock.url as InstanceType<typeof SvelteURL>).search = new URL(String(next)).search;
  });
});

afterEach(() => {
  vi.unstubAllGlobals();
});

function openAt(search: string) {
  (pageMock.url as URL & { search: string }).search = search;
}

describe("/work page", () => {
  it("renders the board by default once the portfolio is read", async () => {
    stubWorkFetch(...boardRoutes);
    render(WorkPage);

    expect(screen.getByTestId("work-source-state")).toHaveAttribute("data-state", "loading");
    expect(await screen.findByTestId("work-board")).toBeInTheDocument();
    expect(screen.queryByTestId("work-table")).not.toBeInTheDocument();
    expect(screen.getByTestId("view-board")).toHaveAttribute("aria-pressed", "true");
    expect(screen.getByTestId("view-table")).toHaveAttribute("aria-pressed", "false");
    expect(await screen.findAllByTestId("work-item-card")).toHaveLength(2);
  });

  it("renders the table for ?view=table with the same items", async () => {
    openAt("?view=table");
    stubWorkFetch(...boardRoutes);
    render(WorkPage);

    expect(await screen.findByTestId("work-table")).toBeInTheDocument();
    expect(screen.queryByTestId("work-board")).not.toBeInTheDocument();
    expect(screen.getByTestId("view-table")).toHaveAttribute("aria-pressed", "true");
    const rows = await screen.findAllByTestId("work-item-row");
    expect(rows.map((row) => row.getAttribute("data-item"))).toEqual(["acme/widgets#1", "acme/widgets#2"]);
  });

  it("writes the view to the URL with replaceState and follows it", async () => {
    const user = userEvent.setup();
    stubWorkFetch(...boardRoutes);
    render(WorkPage);
    await screen.findByTestId("work-board");

    await user.click(screen.getByTestId("view-table"));

    expect(replaceState).toHaveBeenCalledTimes(1);
    expect(String(vi.mocked(replaceState).mock.calls[0][0])).toBe("http://localhost/work?view=table");
    expect(await screen.findByTestId("work-table")).toBeInTheDocument();

    await user.click(screen.getByTestId("view-board"));
    expect(String(vi.mocked(replaceState).mock.calls[1][0])).toBe("http://localhost/work");
    expect(await screen.findByTestId("work-board")).toBeInTheDocument();
  });

  it("toggles the view from the keyboard", async () => {
    const user = userEvent.setup();
    stubWorkFetch(...boardRoutes);
    render(WorkPage);
    await screen.findByTestId("work-board");

    screen.getByTestId("view-table").focus();
    await user.keyboard("{Enter}");

    expect(await screen.findByTestId("work-table")).toBeInTheDocument();
  });

  it("opens the overview from a card, writes ?item=, and reads no runs until asked", async () => {
    const user = userEvent.setup();
    const fetchMock = stubWorkFetch(...boardRoutes, (url) =>
      url.pathname === "/api/work/items/acme/widgets/1/runs"
        ? json(makeRunsResponse([makeRun({ coordinator_loop_id: "loop-9" })]))
        : undefined,
    );
    render(WorkPage);
    const cards = await screen.findAllByTestId("work-item-card");

    await user.click(cards[0]);

    expect(new URL(String(vi.mocked(replaceState).mock.calls[0][0])).searchParams.get("item")).toBe(
      "acme/widgets#1",
    );
    const overview = await screen.findByTestId("work-item-overview");
    expect(overview).toHaveAttribute("data-item", "acme/widgets#1");
    expect(screen.getAllByTestId("work-item-card")[0]).toHaveAttribute("aria-pressed", "true");
    const runCalls = () => fetchMock.mock.calls.filter(([input]) => String(input).endsWith("/runs"));
    expect(runCalls()).toHaveLength(0);

    await user.click(within(overview).getByTestId("load-linked-runs"));

    const section = await screen.findByTestId("work-linked-runs");
    await waitFor(() => expect(section).toHaveAttribute("data-lookup", "complete"));
    expect(runCalls()).toHaveLength(1);
    expect(within(section).getByTestId("work-drill-in")).toHaveAttribute("href", "/?task=loop-9");
    // The card now reports the run's facts; its column is untouched.
    expect(screen.getAllByTestId("work-item-card")[0]).toHaveAttribute("data-column", "Todo");
  });

  it("restores the open item from ?item= and closes it", async () => {
    const user = userEvent.setup();
    openAt("?item=acme%2Fwidgets%232");
    stubWorkFetch(...boardRoutes);
    render(WorkPage);

    const overview = await screen.findByTestId("work-item-overview");
    expect(overview).toHaveAttribute("data-item", "acme/widgets#2");

    await user.click(screen.getByTestId("work-overview-close"));

    expect(new URL(String(vi.mocked(replaceState).mock.calls[0][0])).searchParams.has("item")).toBe(false);
    await waitFor(() => expect(screen.queryByTestId("work-item-overview")).not.toBeInTheDocument());
  });

  it("says why an item in the URL cannot be shown", async () => {
    openAt("?item=acme%2Fwidgets%23404");
    stubWorkFetch(...boardRoutes, (url) =>
      url.pathname === "/api/work/items/acme/widgets/404"
        ? json({ code: "ITEM_NOT_FOUND", error: "acme/widgets#404 does not exist" }, 404)
        : undefined,
    );
    render(WorkPage);

    const state = await screen.findByTestId("work-item-state");
    await waitFor(() => expect(within(state).getByRole("alert")).toHaveTextContent("does not exist"));
  });

  it("shows the unconfigured source instead of an empty board", async () => {
    stubWorkFetch((url) =>
      url.pathname === "/api/work/portfolio" ? json({ source: "unconfigured", programs: [] }) : undefined,
    );
    render(WorkPage);

    await waitFor(() =>
      expect(screen.getByTestId("work-source-state")).toHaveAttribute("data-state", "unconfigured"),
    );
    expect(screen.queryByTestId("work-board")).not.toBeInTheDocument();
  });

  it("shows a configuration error and retries", async () => {
    const user = userEvent.setup();
    let failing = true;
    stubWorkFetch((url) => {
      if (url.pathname !== "/api/work/portfolio") return undefined;
      return failing
        ? json({ code: "PORTFOLIO_UNREADABLE", error: "portfolio document is unreadable" }, 503)
        : json({ source: "unconfigured", programs: [] });
    });
    render(WorkPage);

    await waitFor(() =>
      expect(screen.getByTestId("work-source-state")).toHaveAttribute("data-code", "PORTFOLIO_UNREADABLE"),
    );

    failing = false;
    await user.click(screen.getByRole("button", { name: "Try again" }));

    await waitFor(() =>
      expect(screen.getByTestId("work-source-state")).toHaveAttribute("data-state", "unconfigured"),
    );
  });

  it("refreshes on request and not on a timer", async () => {
    const user = userEvent.setup();
    const fetchMock = stubWorkFetch(...boardRoutes);
    render(WorkPage);
    await screen.findByTestId("work-board");
    const afterLoad = fetchMock.mock.calls.length;

    await user.click(screen.getByTestId("work-refresh"));

    await waitFor(() => expect(fetchMock.mock.calls.length).toBe(afterLoad * 2));
  });
});
