import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { render, screen, waitFor, within } from "@testing-library/svelte";
import userEvent from "@testing-library/user-event";
import { goto } from "$app/navigation";
import WorkPage from "./+page.svelte";
import {
  abortable,
  deferred,
  json,
  makeItem,
  makeItemsResponse,
  makePortfolio,
  makeRun,
  makeRunsResponse,
  stubWorkFetch,
} from "../../test-utils/work";
import type { Route } from "../../test-utils/work";

// A reactive URL stands in for SvelteKit's page.url, and goto() writes the search
// into it the way a real client-side navigation does. (The shallow replaceState()
// does not update page.url in a real browser, which is why the page uses goto.)
const pageMock = vi.hoisted(() => ({ url: null as unknown as URL, state: {} }));
vi.mock("$app/state", () => ({ page: pageMock }));
vi.mock("$app/navigation", () => ({ goto: vi.fn() }));

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
  vi.mocked(goto).mockImplementation(async (next) => {
    (pageMock.url as InstanceType<typeof SvelteURL>).search = new URL(String(next)).search;
  });
});

afterEach(() => {
  vi.unstubAllGlobals();
});

function openAt(search: string) {
  (pageMock.url as URL & { search: string }).search = search;
}

// fetch rejects with an AbortError when its signal aborts. These hold a request open,
// record its signal, and let a test settle it by hand.
function holdOpen(pathname: string) {
  const held = deferred<Response>();
  const signals: AbortSignal[] = [];
  const route: Route = (url, init) => {
    if (url.pathname !== pathname) return undefined;
    if (init?.signal) signals.push(init.signal);
    return abortable(init?.signal, held.promise);
  };
  return { route, signals, settle: held.resolve };
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

  it("writes the view to the URL with a replacing navigation and follows it", async () => {
    const user = userEvent.setup();
    stubWorkFetch(...boardRoutes);
    render(WorkPage);
    await screen.findByTestId("work-board");

    await user.click(screen.getByTestId("view-table"));

    expect(goto).toHaveBeenCalledTimes(1);
    expect(String(vi.mocked(goto).mock.calls[0][0])).toBe("http://localhost/work?view=table");
    // In place: no history entry, no focus reset, no scroll to the top.
    expect(vi.mocked(goto).mock.calls[0][1]).toEqual({ replaceState: true, keepFocus: true, noScroll: true });
    expect(await screen.findByTestId("work-table")).toBeInTheDocument();

    await user.click(screen.getByTestId("view-board"));
    expect(String(vi.mocked(goto).mock.calls[1][0])).toBe("http://localhost/work");
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

    expect(new URL(String(vi.mocked(goto).mock.calls[0][0])).searchParams.get("item")).toBe(
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

    expect(new URL(String(vi.mocked(goto).mock.calls[0][0])).searchParams.has("item")).toBe(false);
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

  describe("focus", () => {
    it("keeps the focused runs button in place while its read is in flight and announces the result", async () => {
      const user = userEvent.setup();
      const runs = holdOpen("/api/work/items/acme/widgets/1/runs");
      openAt(`?item=${encodeURIComponent("acme/widgets#1")}`);
      stubWorkFetch(...boardRoutes, runs.route);
      render(WorkPage);
      const button = await screen.findByTestId("load-linked-runs");
      button.focus();

      await user.keyboard("{Enter}");
      await waitFor(() => expect(runs.signals).toHaveLength(1));

      expect(screen.getByTestId("load-linked-runs")).toBe(button);
      expect(button).toHaveFocus();
      expect(button).toHaveAttribute("aria-busy", "true");
      expect(screen.getByTestId("linked-runs-status")).toHaveTextContent(/loading linked runs/i);

      runs.settle(json(makeRunsResponse([makeRun({ coordinator_loop_id: "loop-9" })])));

      await waitFor(() =>
        expect(screen.getByTestId("linked-runs-status")).toHaveTextContent("1 linked run read."),
      );
      expect(screen.getByTestId("load-linked-runs")).toBe(button);
      expect(button).toHaveFocus();
      expect(button).toHaveAccessibleName("Reload linked runs");
    });

    it("keeps the focused Refresh button in place while the read is in flight and ignores a second press", async () => {
      const user = userEvent.setup();
      const second = deferred<Response>();
      let portfolioReads = 0;
      stubWorkFetch((url, init) => {
        if (url.pathname !== "/api/work/portfolio") return undefined;
        portfolioReads += 1;
        return portfolioReads === 1
          ? json(makePortfolio(["acme/widgets"]))
          : abortable(init?.signal, second.promise);
      }, ...boardRoutes.slice(1));
      render(WorkPage);
      await screen.findByTestId("work-board");
      const button = screen.getByTestId("work-refresh");
      // The first load is a refresh too; wait it out so the press below starts a new one.
      await waitFor(() => expect(button).toHaveTextContent("Refresh"));
      await waitFor(() => expect(button).not.toHaveTextContent("Refreshing"));
      button.focus();

      await user.keyboard("{Enter}");
      await waitFor(() => expect(portfolioReads).toBe(2));

      expect(screen.getByTestId("work-refresh")).toBe(button);
      expect(button).toHaveFocus();
      expect(button).toHaveAttribute("aria-busy", "true");
      expect(button).toHaveAttribute("aria-disabled", "true");
      expect(button).not.toBeDisabled();

      await user.keyboard("{Enter}");
      expect(portfolioReads).toBe(2);

      second.resolve(json(makePortfolio(["acme/widgets"])));

      await waitFor(() => expect(button).toHaveAttribute("aria-busy", "false"));
      expect(button).not.toHaveAttribute("aria-disabled");
      expect(button).toHaveFocus();
    });

    it("lands on the overview heading when a card opens it, and returns to that card on Close", async () => {
      const user = userEvent.setup();
      stubWorkFetch(...boardRoutes);
      render(WorkPage);
      const [card] = await screen.findAllByTestId("work-item-card");

      card.focus();
      await user.keyboard("{Enter}");

      const overview = await screen.findByTestId("work-item-overview");
      expect(within(overview).getByRole("heading", { name: "acme/widgets#1" })).toHaveFocus();

      await user.tab();
      expect(within(overview).getByTestId("work-overview-close")).toHaveFocus();
      await user.keyboard("{Enter}");

      await waitFor(() => expect(screen.queryByTestId("work-item-overview")).not.toBeInTheDocument());
      expect(screen.getAllByTestId("work-item-card")[0]).toHaveFocus();
    });

    it("returns focus to the table row's button on Close", async () => {
      const user = userEvent.setup();
      openAt("?view=table");
      stubWorkFetch(...boardRoutes);
      render(WorkPage);
      const [first] = await screen.findAllByRole("button", { name: /^acme\/widgets#/ });

      await user.click(first);
      await user.click(await screen.findByTestId("work-overview-close"));

      await waitFor(() => expect(screen.queryByTestId("work-item-overview")).not.toBeInTheDocument());
      expect(screen.getAllByRole("button", { name: /^acme\/widgets#/ })[0]).toHaveFocus();
    });

    it("returns focus to the opener for a deep link, where no click opened it", async () => {
      const user = userEvent.setup();
      openAt("?item=acme%2Fwidgets%232");
      stubWorkFetch(...boardRoutes);
      render(WorkPage);

      await user.click(await screen.findByTestId("work-overview-close"));

      await waitFor(() => expect(screen.queryByTestId("work-item-overview")).not.toBeInTheDocument());
      const doneCard = screen
        .getAllByTestId("work-item-card")
        .find((card) => card.getAttribute("data-item") === "acme/widgets#2");
      expect(doneCard).toHaveFocus();
    });

    it("falls back to the page heading when Close is pressed on an item that cannot be shown", async () => {
      const user = userEvent.setup();
      openAt("?item=acme%2Fwidgets%23404");
      stubWorkFetch(...boardRoutes, (url) =>
        url.pathname === "/api/work/items/acme/widgets/404"
          ? json({ code: "ITEM_NOT_FOUND", error: "acme/widgets#404 does not exist" }, 404)
          : undefined,
      );
      render(WorkPage);
      const state = await screen.findByTestId("work-item-state");
      await waitFor(() => expect(within(state).getByRole("alert")).toBeInTheDocument());

      await user.click(within(state).getByRole("button", { name: "Close" }));

      await waitFor(() => expect(screen.queryByTestId("work-item-state")).not.toBeInTheDocument());
      expect(screen.getByRole("heading", { level: 1, name: "Work" })).toHaveFocus();
    });
  });

  describe("partial run lookups on the board and table", () => {
    const partialRuns = () =>
      json(
        makeRunsResponse(
          [
            makeRun({
              execution_stage: { state: "known", value: "completed" },
              needs_you: { state: "known", value: false },
            }),
          ],
          { lookup: "partial", reason: "prompt binding matched no coordinator loop" },
        ),
      );
    const runsRoute: Route = (url) =>
      url.pathname === "/api/work/items/acme/widgets/1/runs" ? partialRuns() : undefined;
    const unknownWithReason = /at least 1 run; lookup partial: prompt binding matched no coordinator loop/;

    it("shows the card's run badges as unknown, never needs-you no, while the overview keeps the run's own facts", async () => {
      const user = userEvent.setup();
      openAt(`?item=${encodeURIComponent("acme/widgets#1")}`);
      stubWorkFetch(...boardRoutes, runsRoute);
      render(WorkPage);

      await user.click(await screen.findByTestId("load-linked-runs"));

      const overview = within(await screen.findByTestId("work-item-overview"));
      await waitFor(() => expect(overview.getByTestId("work-linked-runs")).toHaveAttribute("data-lookup", "partial"));
      // The run itself resolved, so its row reports what it knows...
      expect(overview.getByTestId("overlay-needs-you")).toHaveAttribute("data-state", "known");
      expect(overview.getByTestId("overlay-execution-stage")).toHaveTextContent("completed");
      // ...but the item-level summary on the card cannot say "no", or how many.
      const card = within(
        screen.getAllByTestId("work-item-card").find((c) => c.getAttribute("data-item") === "acme/widgets#1")!,
      );
      for (const name of ["needs-you", "execution-stage", "verification"]) {
        expect(card.getByTestId(`overlay-${name}`)).toHaveAttribute("data-state", "unknown");
      }
      expect(card.getByTestId("overlay-needs-you").getAttribute("title")).toMatch(unknownWithReason);
      expect(card.getByTestId("overlay-needs-you")).not.toHaveAttribute("data-value");
    });

    it("shows the table row's run cells as unknown with the lookup reason, including the run count", async () => {
      const user = userEvent.setup();
      openAt(`?view=table&item=${encodeURIComponent("acme/widgets#1")}`);
      stubWorkFetch(...boardRoutes, runsRoute);
      render(WorkPage);

      await user.click(await screen.findByTestId("load-linked-runs"));
      await waitFor(() =>
        expect(screen.getByTestId("work-linked-runs")).toHaveAttribute("data-lookup", "partial"),
      );

      const row = within(
        screen.getAllByTestId("work-item-row").find((r) => r.getAttribute("data-item") === "acme/widgets#1")!,
      );
      for (const name of ["needs-you", "execution-stage", "verification", "linked-runs"]) {
        expect(row.getByTestId(`overlay-${name}`)).toHaveAttribute("data-state", "unknown");
      }
      expect(row.getByTestId("overlay-linked-runs").getAttribute("title")).toMatch(unknownWithReason);
      expect(row.getByTestId("overlay-linked-runs")).not.toHaveAttribute("data-value");
    });

    it("keeps a known needs-you of yes on the card even when the lookup was partial", async () => {
      const user = userEvent.setup();
      openAt(`?item=${encodeURIComponent("acme/widgets#1")}`);
      stubWorkFetch(...boardRoutes, (url) =>
        url.pathname === "/api/work/items/acme/widgets/1/runs"
          ? json(
              makeRunsResponse(
                [makeRun({ needs_you: { state: "known", value: true, reason: "approval outstanding" } })],
                { lookup: "partial", reason: "one run unreadable" },
              ),
            )
          : undefined,
      );
      render(WorkPage);

      await user.click(await screen.findByTestId("load-linked-runs"));

      await waitFor(() => {
        const card = screen.getAllByTestId("work-item-card").find((c) => c.getAttribute("data-item") === "acme/widgets#1")!;
        expect(within(card).getByTestId("overlay-needs-you")).toHaveAttribute("data-state", "known");
        expect(within(card).getByTestId("overlay-needs-you")).toHaveTextContent("yes");
      });
    });
  });

  describe("leaving the page", () => {
    it("aborts an in-flight linked-runs read when the page unmounts", async () => {
      const user = userEvent.setup();
      const runs = holdOpen("/api/work/items/acme/widgets/1/runs");
      openAt(`?item=${encodeURIComponent("acme/widgets#1")}`);
      stubWorkFetch(...boardRoutes, runs.route);
      const { unmount } = render(WorkPage);

      await user.click(await screen.findByTestId("load-linked-runs"));
      await waitFor(() => expect(runs.signals).toHaveLength(1));
      expect(runs.signals[0].aborted).toBe(false);

      unmount();

      expect(runs.signals[0].aborted).toBe(true);
    });

    it("aborts an in-flight item read for a deep link when the page unmounts", async () => {
      const item = holdOpen("/api/work/items/acme/widgets/404");
      openAt(`?item=${encodeURIComponent("acme/widgets#404")}`);
      stubWorkFetch(...boardRoutes, item.route);
      const { unmount } = render(WorkPage);

      await waitFor(() => expect(item.signals).toHaveLength(1));
      expect(item.signals[0].aborted).toBe(false);

      unmount();

      expect(item.signals[0].aborted).toBe(true);
    });
  });
});
