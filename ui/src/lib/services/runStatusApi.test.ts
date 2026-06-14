import { describe, it, expect, beforeEach, afterEach, vi } from "vitest";
import { getTriples } from "./runStatusApi";
import { AgentApiError } from "./agentApi";

describe("runStatusApi.getTriples", () => {
  const mockFetch = vi.fn();
  const originalFetch = globalThis.fetch;

  beforeEach(() => {
    mockFetch.mockClear();
    globalThis.fetch = mockFetch;
  });

  afterEach(() => {
    globalThis.fetch = originalFetch;
  });

  // -------------------------------------------------------------------------
  // Querystring assembly
  // -------------------------------------------------------------------------

  it("builds the querystring from all params", async () => {
    mockFetch.mockResolvedValueOnce({
      ok: true,
      json: async () => [],
    });

    await getTriples({ subject: "s1", predicate: "p1", object: "o1", limit: 50 });

    const [url] = mockFetch.mock.calls[0];
    expect(url).toBe("/graph/triples?subject=s1&predicate=p1&object=o1&limit=50");
  });

  it("omits absent params from the querystring", async () => {
    mockFetch.mockResolvedValueOnce({ ok: true, json: async () => [] });

    await getTriples({ predicate: "agent.run.approval_pending" });

    const [url] = mockFetch.mock.calls[0];
    expect(url).toBe("/graph/triples?predicate=agent.run.approval_pending");
    expect(url).not.toContain("subject=");
    expect(url).not.toContain("object=");
    expect(url).not.toContain("limit=");
  });

  it("calls bare /graph/triples when no params passed", async () => {
    mockFetch.mockResolvedValueOnce({ ok: true, json: async () => [] });

    await getTriples({});

    const [url] = mockFetch.mock.calls[0];
    expect(url).toBe("/graph/triples");
  });

  // -------------------------------------------------------------------------
  // OK path — parsed response
  // -------------------------------------------------------------------------

  it("returns parsed RawTriple array on ok response", async () => {
    const triples = [
      { subject: "sub1", predicate: "pred1", object: "obj1", confidence: 0.9 },
      { subject: "sub2", predicate: "pred2", object: "obj2" },
    ];
    mockFetch.mockResolvedValueOnce({ ok: true, json: async () => triples });

    const result = await getTriples({ predicate: "pred1" });

    expect(result).toEqual(triples);
  });

  it("returns an empty array when the server returns []", async () => {
    mockFetch.mockResolvedValueOnce({ ok: true, json: async () => [] });

    const result = await getTriples({ predicate: "missing" });

    expect(result).toEqual([]);
  });

  // -------------------------------------------------------------------------
  // Non-ok throws AgentApiError
  // -------------------------------------------------------------------------

  it("throws AgentApiError with status code on non-ok response", async () => {
    mockFetch.mockResolvedValueOnce({
      ok: false,
      status: 503,
      statusText: "Service Unavailable",
      json: async () => ({ error: "down" }),
    });

    await expect(getTriples({ predicate: "p" })).rejects.toMatchObject({
      name: "AgentApiError",
      statusCode: 503,
    });
  });

  it("throws AgentApiError even when error body is not valid JSON", async () => {
    mockFetch.mockResolvedValueOnce({
      ok: false,
      status: 500,
      statusText: "Internal Server Error",
      json: async () => { throw new Error("not json"); },
    });

    await expect(getTriples({ predicate: "p" })).rejects.toBeInstanceOf(AgentApiError);
  });
});
