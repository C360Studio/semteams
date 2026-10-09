import { afterEach, describe, expect, it, vi } from "vitest";
import { createWorkStore, ITEM_CONCURRENCY, SOURCE_UNCONFIGURED_CODE } from "./workStore.svelte";
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

afterEach(() => {
  vi.unstubAllGlobals();
});

const portfolioRoute =
  (repositories: string[]): Route =>
  (url) =>
    url.pathname === "/api/work/portfolio" ? json(makePortfolio(repositories)) : undefined;

const itemsRoute =
  (byRepository: Record<string, ReturnType<typeof makeItemsResponse>>): Route =>
  (url) => {
    if (url.pathname !== "/api/work/items") return undefined;
    const response = byRepository[url.searchParams.get("repository") ?? ""];
    return response ? json(response) : undefined;
  };

describe("workStore source state", () => {
  it("is loading before anything has been read", () => {
    stubWorkFetch();
    expect(createWorkStore().sourceState).toEqual({ kind: "loading" });
  });

  it("reports an unconfigured source from the portfolio and reads no items", async () => {
    const fetchMock = stubWorkFetch((url) =>
      url.pathname === "/api/work/portfolio" ? json({ source: "unconfigured", programs: [] }) : undefined,
    );
    const store = createWorkStore();

    await store.load();

    expect(store.sourceState).toEqual({ kind: "unconfigured" });
    expect(fetchMock).toHaveBeenCalledTimes(1);
  });

  it("treats the unconfigured code on a later read as an unconfigured source, not an error", async () => {
    stubWorkFetch(portfolioRoute(["acme/widgets"]), (url) =>
      url.pathname === "/api/work/items"
        ? json({ code: SOURCE_UNCONFIGURED_CODE, error: "work source is not configured" }, 503)
        : undefined,
    );
    const store = createWorkStore();

    await store.load();

    expect(store.sourceState).toEqual({ kind: "unconfigured" });
  });

  it("surfaces a 503 configuration code with its message and issues", async () => {
    stubWorkFetch((url) =>
      url.pathname === "/api/work/portfolio"
        ? json(
            {
              code: "PORTFOLIO_INVALID",
              error: "portfolio document is invalid",
              issues: [{ path: "programs[0].id", message: "required" }],
            },
            503,
          )
        : undefined,
    );
    const store = createWorkStore();

    await store.load();

    expect(store.sourceState).toEqual({
      kind: "error",
      code: "PORTFOLIO_INVALID",
      message: "portfolio document is invalid",
      issues: [{ path: "programs[0].id", message: "required" }],
    });
  });

  it("reports a network failure as an error state", async () => {
    vi.stubGlobal("fetch", vi.fn().mockRejectedValue(new TypeError("Failed to fetch")));
    const store = createWorkStore();

    await store.load();

    expect(store.sourceState).toEqual({
      kind: "error",
      code: "NETWORK_ERROR",
      message: "Failed to fetch",
    });
  });
});

describe("workStore repositories", () => {
  it("keeps each repository's lookup, reason and ordered columns", async () => {
    stubWorkFetch(
      portfolioRoute(["acme/widgets", "acme/gadgets"]),
      itemsRoute({
        "acme/widgets": makeItemsResponse("acme/widgets", [makeItem()], {
          lookup: "partial",
          reason: "project board read timed out",
          columns: ["Todo", "In Review", "Done"],
        }),
        "acme/gadgets": makeItemsResponse("acme/gadgets", [], { columns: ["Todo", "Done"] }),
      }),
    );
    const store = createWorkStore();

    await store.load();

    expect(store.sourceState).toEqual({ kind: "ready" });
    expect(store.boards["acme/widgets"]).toMatchObject({
      loading: false,
      lookup: "partial",
      reason: "project board read timed out",
      columns: ["Todo", "In Review", "Done"],
    });
    expect(store.boards["acme/widgets"].items).toHaveLength(1);
    expect(store.boards["acme/gadgets"]).toMatchObject({ lookup: "complete", columns: ["Todo", "Done"] });
  });

  it("records a failed read as a failed lookup for that repository only", async () => {
    stubWorkFetch(
      portfolioRoute(["acme/widgets", "acme/gadgets"]),
      (url) =>
        url.pathname === "/api/work/items" && url.searchParams.get("repository") === "acme/gadgets"
          ? json({ code: "REPOSITORY_NOT_CONFIGURED", error: "repository acme/gadgets is not in the portfolio" }, 404)
          : undefined,
      itemsRoute({ "acme/widgets": makeItemsResponse("acme/widgets", [makeItem()]) }),
    );
    const store = createWorkStore();

    await store.load();

    expect(store.sourceState).toEqual({ kind: "ready" });
    expect(store.boards["acme/widgets"].lookup).toBe("complete");
    expect(store.boards["acme/gadgets"]).toMatchObject({
      lookup: "failed",
      reason: "repository acme/gadgets is not in the portfolio",
      items: [],
    });
  });

  it("reads at most four repositories at a time", async () => {
    const repositories = Array.from({ length: 7 }, (_, i) => `acme/repo-${i}`);
    const gate = deferred();
    let inFlight = 0;
    let peak = 0;
    stubWorkFetch(portfolioRoute(repositories), async (url) => {
      if (url.pathname !== "/api/work/items") return undefined;
      inFlight += 1;
      peak = Math.max(peak, inFlight);
      await gate.promise;
      inFlight -= 1;
      const repository = url.searchParams.get("repository") ?? "";
      return json(makeItemsResponse(repository, []));
    });
    const store = createWorkStore();

    const loading = store.load();
    await vi.waitFor(() => expect(inFlight).toBe(ITEM_CONCURRENCY));
    expect(ITEM_CONCURRENCY).toBe(4);
    gate.resolve();
    await loading;

    expect(peak).toBe(4);
    expect(Object.values(store.boards).every((board) => !board.loading)).toBe(true);
  });

  it("refresh re-reads the portfolio and items", async () => {
    let titles = ["First"];
    const fetchMock = stubWorkFetch(portfolioRoute(["acme/widgets"]), (url) =>
      url.pathname === "/api/work/items"
        ? json(makeItemsResponse("acme/widgets", titles.map((title) => makeItem({ title }))))
        : undefined,
    );
    const store = createWorkStore();
    await store.load();
    expect(store.boards["acme/widgets"].items[0].title).toBe("First");

    titles = ["Second"];
    await store.refresh();

    expect(store.boards["acme/widgets"].items[0].title).toBe("Second");
    expect(store.refreshing).toBe(false);
    expect(fetchMock).toHaveBeenCalledTimes(4);
  });
});

describe("workStore linked runs", () => {
  const widgets = makeItem();
  const gadget = makeItem({ ref: "acme/widgets#2", number: 2, title: "Gadget" });

  function stubBoard(runsRoute: Route) {
    return stubWorkFetch(
      portfolioRoute(["acme/widgets"]),
      itemsRoute({ "acme/widgets": makeItemsResponse("acme/widgets", [widgets, gadget]) }),
      runsRoute,
    );
  }

  it("does not read linked runs when the board loads or an item is selected", async () => {
    const fetchMock = stubBoard(() => json(makeRunsResponse([makeRun()])));
    const store = createWorkStore();

    await store.load();
    store.select("acme/widgets#1");

    const runCalls = fetchMock.mock.calls.filter(([input]) => String(input).endsWith("/runs"));
    expect(runCalls).toHaveLength(0);
    expect(store.linkedRunsFor("acme/widgets#1")).toEqual({ status: "idle" });
  });

  it("loads the selected item's runs on request and keeps the lookup reason", async () => {
    stubBoard((url) =>
      url.pathname === "/api/work/items/acme/widgets/1/runs"
        ? json(
            makeRunsResponse([makeRun({ lookup: "partial", reason: "run entity has no facts" })], {
              lookup: "partial",
              reason: "run entity has no facts",
            }),
          )
        : undefined,
    );
    const store = createWorkStore();
    await store.load();
    store.select("acme/widgets#1");

    await store.loadLinkedRuns();

    const state = store.linkedRunsFor("acme/widgets#1");
    expect(state).toMatchObject({ status: "loaded", lookup: "partial", reason: "run entity has no facts" });
    expect(state.status === "loaded" && state.runs).toHaveLength(1);
  });

  it("cancels the in-flight run read when another item is selected", async () => {
    const signals: AbortSignal[] = [];
    stubBoard((url, init) => {
      if (!url.pathname.endsWith("/runs")) return undefined;
      const signal = init?.signal;
      if (signal) signals.push(signal);
      return abortable(signal, new Promise<Response>(() => {}));
    });
    const store = createWorkStore();
    await store.load();
    store.select("acme/widgets#1");

    const pending = store.loadLinkedRuns();
    await vi.waitFor(() => expect(signals).toHaveLength(1));
    expect(store.linkedRunsFor("acme/widgets#1")).toEqual({ status: "loading" });

    store.select("acme/widgets#2");
    await pending;

    expect(signals[0].aborted).toBe(true);
    expect(store.linkedRunsFor("acme/widgets#1")).toEqual({ status: "idle" });
    expect(store.linkedRunsFor("acme/widgets#2")).toEqual({ status: "idle" });
  });

  it("records a failed run read as an error with the server code", async () => {
    stubBoard((url) =>
      url.pathname.endsWith("/runs")
        ? json({ code: "ITEM_NOT_FOUND", error: "acme/widgets#1 does not exist" }, 404)
        : undefined,
    );
    const store = createWorkStore();
    await store.load();
    store.select("acme/widgets#1");

    await store.loadLinkedRuns();

    expect(store.linkedRunsFor("acme/widgets#1")).toEqual({
      status: "error",
      code: "ITEM_NOT_FOUND",
      message: "acme/widgets#1 does not exist",
    });
  });

  it("refresh re-reads the open item's runs only if they had been requested", async () => {
    const fetchMock = stubBoard(() => json(makeRunsResponse([makeRun()])));
    const store = createWorkStore();
    await store.load();
    store.select("acme/widgets#1");

    await store.refresh();
    const afterIdleRefresh = fetchMock.mock.calls.filter(([input]) => String(input).endsWith("/runs"));
    expect(afterIdleRefresh).toHaveLength(0);

    await store.loadLinkedRuns();
    await store.refresh();
    const afterLoadedRefresh = fetchMock.mock.calls.filter(([input]) => String(input).endsWith("/runs"));
    expect(afterLoadedRefresh).toHaveLength(2);
    expect(store.linkedRunsFor("acme/widgets#1").status).toBe("loaded");
  });
});

describe("workStore selection", () => {
  it("resolves a listed item without reading the item endpoint", async () => {
    const fetchMock = stubWorkFetch(
      portfolioRoute(["acme/widgets"]),
      itemsRoute({ "acme/widgets": makeItemsResponse("acme/widgets", [makeItem()]) }),
    );
    const store = createWorkStore();
    store.select("acme/widgets#1");
    expect(store.selection).toEqual({ kind: "loading", ref: "acme/widgets#1" });

    await store.load();

    expect(store.selection).toMatchObject({ kind: "item", item: { title: "Add a widget" } });
    expect(fetchMock.mock.calls.some(([input]) => String(input).includes("/items/acme/"))).toBe(false);
  });

  it("falls back to the item endpoint for a ref the lists do not carry", async () => {
    stubWorkFetch(
      portfolioRoute(["acme/widgets"]),
      itemsRoute({ "acme/widgets": makeItemsResponse("acme/widgets", []) }),
      (url) =>
        url.pathname === "/api/work/items/acme/widgets/9"
          ? json({ lookup: "complete", item: makeItem({ ref: "acme/widgets#9", number: 9, title: "Deep link" }) })
          : undefined,
    );
    const store = createWorkStore();
    store.select("acme/widgets#9");

    await store.load();
    await vi.waitFor(() => expect(store.selection.kind).toBe("item"));

    expect(store.selection).toMatchObject({ item: { title: "Deep link" } });
  });

  it("says why an unknown item cannot be shown", async () => {
    stubWorkFetch(
      portfolioRoute(["acme/widgets"]),
      itemsRoute({ "acme/widgets": makeItemsResponse("acme/widgets", []) }),
      (url) =>
        url.pathname === "/api/work/items/acme/widgets/9"
          ? json({ code: "ITEM_NOT_FOUND", error: "acme/widgets#9 does not exist" }, 404)
          : undefined,
    );
    const store = createWorkStore();
    store.select("acme/widgets#9");

    await store.load();
    await vi.waitFor(() => expect(store.selection.kind).toBe("unavailable"));

    expect(store.selection).toMatchObject({ message: "acme/widgets#9 does not exist" });
  });

  it("rejects a malformed ref without a request", async () => {
    const fetchMock = stubWorkFetch(
      portfolioRoute(["acme/widgets"]),
      itemsRoute({ "acme/widgets": makeItemsResponse("acme/widgets", []) }),
    );
    const store = createWorkStore();
    store.select("not-a-ref");

    await store.load();

    expect(store.selection.kind).toBe("unavailable");
    expect(fetchMock).toHaveBeenCalledTimes(2);
  });

  it("dispose aborts a load in flight", async () => {
    const signals: AbortSignal[] = [];
    stubWorkFetch((url, init) => {
      if (init?.signal) signals.push(init.signal);
      return url.pathname === "/api/work/portfolio"
        ? abortable(init?.signal, new Promise<Response>(() => {}))
        : undefined;
    });
    const store = createWorkStore();

    const loading = store.load();
    await vi.waitFor(() => expect(signals).toHaveLength(1));
    store.dispose();
    await loading;

    expect(signals[0].aborted).toBe(true);
    expect(store.sourceState).toEqual({ kind: "loading" });
  });

  it("dispose aborts an in-flight linked-runs read and leaves its state at loading, not a stale error", async () => {
    const signals: AbortSignal[] = [];
    stubWorkFetch(
      portfolioRoute(["acme/widgets"]),
      itemsRoute({ "acme/widgets": makeItemsResponse("acme/widgets", [makeItem()]) }),
      (url, init) => {
        if (!url.pathname.endsWith("/runs")) return undefined;
        if (init?.signal) signals.push(init.signal);
        return abortable(init?.signal, new Promise<Response>(() => {}));
      },
    );
    const store = createWorkStore();
    await store.load();
    store.select("acme/widgets#1");
    const pending = store.loadLinkedRuns();
    await vi.waitFor(() => expect(signals).toHaveLength(1));

    store.dispose();
    await pending;

    expect(signals[0].aborted).toBe(true);
    // An aborted read is not a failure to report.
    expect(store.linkedRunsFor("acme/widgets#1").status).not.toBe("error");
  });

  it("dispose aborts an in-flight item read for a deep link", async () => {
    const signals: AbortSignal[] = [];
    stubWorkFetch(
      portfolioRoute(["acme/widgets"]),
      itemsRoute({ "acme/widgets": makeItemsResponse("acme/widgets", []) }),
      (url, init) => {
        if (url.pathname !== "/api/work/items/acme/widgets/9") return undefined;
        if (init?.signal) signals.push(init.signal);
        return abortable(init?.signal, new Promise<Response>(() => {}));
      },
    );
    const store = createWorkStore();
    store.select("acme/widgets#9");
    await store.load();
    await vi.waitFor(() => expect(signals).toHaveLength(1));

    store.dispose();

    await vi.waitFor(() => expect(signals[0].aborted).toBe(true));
    expect(store.selection).toEqual({ kind: "loading", ref: "acme/widgets#9" });
  });
});
