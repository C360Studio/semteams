import { afterEach, describe, expect, it, vi } from "vitest";
import {
  formatItemRef,
  getItem,
  getItems,
  getLinkedRuns,
  getPortfolio,
  parseItemRef,
  WorkApiRequestError,
} from "./workApi";
import { json, stubWorkFetch } from "../../test-utils/work";

afterEach(() => {
  vi.unstubAllGlobals();
});

describe("workApi endpoints", () => {
  it("reads the portfolio", async () => {
    const fetchMock = stubWorkFetch(() => json({ source: "fixture", programs: [] }));

    await expect(getPortfolio()).resolves.toEqual({ source: "fixture", programs: [] });
    expect(String(fetchMock.mock.calls[0][0])).toBe("/api/work/portfolio");
  });

  it("encodes the repository query and passes the abort signal", async () => {
    const fetchMock = stubWorkFetch(() => json({ lookup: "complete", repository: "a/b", columns: [], items: [] }));
    const controller = new AbortController();

    await getItems("acme/widgets", controller.signal);

    expect(String(fetchMock.mock.calls[0][0])).toBe("/api/work/items?repository=acme%2Fwidgets");
    expect(fetchMock.mock.calls[0][1]?.signal).toBe(controller.signal);
  });

  it("addresses one item and its runs by owner, repo and number", async () => {
    const fetchMock = stubWorkFetch(() => json({ lookup: "complete", runs: [] }));
    const ref = { owner: "acme", repo: "wid gets", number: 12 };

    await getItem(ref);
    await getLinkedRuns(ref);

    expect(fetchMock.mock.calls.map(([input]) => String(input))).toEqual([
      "/api/work/items/acme/wid%20gets/12",
      "/api/work/items/acme/wid%20gets/12/runs",
    ]);
  });

  it("only ever issues GET requests", async () => {
    const fetchMock = stubWorkFetch(() => json({ source: "fixture", programs: [] }));

    await getPortfolio();

    expect(fetchMock.mock.calls[0][1]?.method ?? "GET").toBe("GET");
  });
});

describe("workApi errors", () => {
  it("carries the server's code, message, status and issues", async () => {
    stubWorkFetch(() =>
      json(
        {
          code: "PORTFOLIO_INVALID",
          error: "portfolio document is invalid",
          issues: [{ path: "programs", message: "required" }],
        },
        503,
      ),
    );

    const error = await getPortfolio().catch((e: unknown) => e);

    expect(error).toBeInstanceOf(WorkApiRequestError);
    const failure = error as WorkApiRequestError;
    expect(failure.status).toBe(503);
    expect(failure.code).toBe("PORTFOLIO_INVALID");
    expect(failure.message).toBe("portfolio document is invalid");
    expect(failure.body?.issues).toEqual([{ path: "programs", message: "required" }]);
  });

  it("falls back to the status when the body is not a work API error", async () => {
    stubWorkFetch(() => new Response("<html>bad gateway</html>", { status: 502, statusText: "Bad Gateway" }));

    const error = (await getPortfolio().catch((e: unknown) => e)) as WorkApiRequestError;

    expect(error.status).toBe(502);
    expect(error.code).toBe("HTTP_502");
    expect(error.body).toBeNull();
    expect(error.message).toBe("Work request failed: 502 Bad Gateway");
  });

  it("lets a network failure and an abort propagate untouched", async () => {
    vi.stubGlobal("fetch", vi.fn().mockRejectedValue(new TypeError("Failed to fetch")));
    await expect(getPortfolio()).rejects.toThrow("Failed to fetch");
    await expect(getPortfolio()).rejects.not.toBeInstanceOf(WorkApiRequestError);
  });
});

describe("item references", () => {
  it("parses and formats owner/repo#number", () => {
    expect(parseItemRef("c360studio/semteams#312")).toEqual({ owner: "c360studio", repo: "semteams", number: 312 });
    expect(formatItemRef({ owner: "a", repo: "b", number: 3 })).toBe("a/b#3");
  });

  it.each(["", "owner/repo", "owner/repo#", "owner/repo#0", "owner/repo#01x", "a/b/c#1", "owner repo#1", "#1"])(
    "rejects %j",
    (value) => {
      expect(parseItemRef(value)).toBeNull();
    },
  );
});
