import { describe, it, expect, vi, beforeEach } from "vitest";
import { createGraphQLClient } from "./client";

const mockFetch = vi.fn();
vi.stubGlobal("fetch", mockFetch);

beforeEach(() => {
  vi.resetAllMocks();
});

// ---------------------------------------------------------------------------
// Malformed JSON response — server returns non-JSON body
// ---------------------------------------------------------------------------

describe("client.attack — malformed JSON response", () => {
  it("throws when response body is not valid JSON", async () => {
    mockFetch.mockResolvedValueOnce(
      new Response("<html>502 Bad Gateway</html>", {
        status: 200,
        headers: { "Content-Type": "text/html" },
      }),
    );

    const client = createGraphQLClient({ baseUrl: "http://backend:8082" });
    await expect(client.query("query { ok }")).rejects.toThrow();
  });

  it("throws when response body is an empty string", async () => {
    mockFetch.mockResolvedValueOnce(
      new Response("", {
        status: 200,
        headers: { "Content-Type": "application/json" },
      }),
    );

    const client = createGraphQLClient({ baseUrl: "http://backend:8082" });
    await expect(client.query("query { ok }")).rejects.toThrow();
  });

  it("throws when response is truncated JSON", async () => {
    mockFetch.mockResolvedValueOnce(
      new Response('{"data": {"entities": [', {
        status: 200,
        headers: { "Content-Type": "application/json" },
      }),
    );

    const client = createGraphQLClient({ baseUrl: "http://backend:8082" });
    await expect(client.query("query { ok }")).rejects.toThrow();
  });

  it("throws when response body is null literal string", async () => {
    // { data: null } is handled by the null-data check — it must throw
    mockFetch.mockResolvedValueOnce(
      new Response("null", {
        status: 200,
        headers: { "Content-Type": "application/json" },
      }),
    );

    const client = createGraphQLClient({ baseUrl: "http://backend:8082" });
    // null parses fine but data will be missing — must throw
    await expect(client.query("query { ok }")).rejects.toThrow();
  });
});

// ---------------------------------------------------------------------------
// Very large response — must not hang or crash
// ---------------------------------------------------------------------------

describe("client.attack — very large response", () => {
  it("handles a very large JSON response without crashing", async () => {
    // 10 000 entities
    const entities = Array.from({ length: 10_000 }, (_, i) => ({
      id: `c360.ops.robotics.gcs.drone.${String(i).padStart(6, "0")}`,
      triples: [],
    }));

    mockFetch.mockResolvedValueOnce(
      new Response(
        JSON.stringify({
          data: {
            globalSearch: {
              entities,
              count: 10_000,
              duration_ms: 500,
              community_summaries: [],
              relationships: [],
            },
          },
        }),
        { status: 200, headers: { "Content-Type": "application/json" } },
      ),
    );

    const client = createGraphQLClient({ baseUrl: "http://backend:8082" });
    const result = await client.query<{ globalSearch: { count: number } }>(
      "query { globalSearch { count } }",
    );
    expect(result.globalSearch.count).toBe(10_000);
  });

  it("handles response with a very long string value", async () => {
    const longString = "x".repeat(1_000_000);
    mockFetch.mockResolvedValueOnce(
      new Response(JSON.stringify({ data: { description: longString } }), {
        status: 200,
        headers: { "Content-Type": "application/json" },
      }),
    );

    const client = createGraphQLClient({ baseUrl: "http://backend:8082" });
    const result = await client.query<{ description: string }>(
      "query { description }",
    );
    expect(result.description.length).toBe(1_000_000);
  });
});

// ---------------------------------------------------------------------------
// HTTP redirect — fetch follows by default; 3xx that doesn't resolve → error
// ---------------------------------------------------------------------------

describe("client.attack — HTTP redirect / unusual status codes", () => {
  it("throws on 401 Unauthorized", async () => {
    mockFetch.mockResolvedValueOnce(
      new Response("Unauthorized", { status: 401, statusText: "Unauthorized" }),
    );
    const client = createGraphQLClient({ baseUrl: "http://backend:8082" });
    await expect(client.query("query { ok }")).rejects.toThrow(/401/);
  });

  it("throws on 404 Not Found", async () => {
    mockFetch.mockResolvedValueOnce(
      new Response("Not Found", { status: 404, statusText: "Not Found" }),
    );
    const client = createGraphQLClient({ baseUrl: "http://backend:8082" });
    await expect(client.query("query { ok }")).rejects.toThrow(/404/);
  });

  it("throws on 429 Too Many Requests", async () => {
    mockFetch.mockResolvedValueOnce(
      new Response("Too Many Requests", {
        status: 429,
        statusText: "Too Many Requests",
      }),
    );
    const client = createGraphQLClient({ baseUrl: "http://backend:8082" });
    await expect(client.query("query { ok }")).rejects.toThrow(/429/);
  });
});

// ---------------------------------------------------------------------------
// Concurrent queries — AbortControllers must be independent
// ---------------------------------------------------------------------------

describe("client.attack — concurrent queries do not interfere", () => {
  it("two simultaneous queries can both succeed independently", async () => {
    mockFetch
      .mockResolvedValueOnce(
        new Response(JSON.stringify({ data: { result: "first" } }), {
          status: 200,
          headers: { "Content-Type": "application/json" },
        }),
      )
      .mockResolvedValueOnce(
        new Response(JSON.stringify({ data: { result: "second" } }), {
          status: 200,
          headers: { "Content-Type": "application/json" },
        }),
      );

    const client = createGraphQLClient({ baseUrl: "http://backend:8082" });

    const [r1, r2] = await Promise.all([
      client.query<{ result: string }>("query { result }"),
      client.query<{ result: string }>("query { result }"),
    ]);

    expect(r1.result).toBe("first");
    expect(r2.result).toBe("second");
  });

  it("one failing query does not abort another in-flight query", async () => {
    // First call rejects with network error, second succeeds
    mockFetch
      .mockRejectedValueOnce(new TypeError("Failed to fetch"))
      .mockResolvedValueOnce(
        new Response(JSON.stringify({ data: { ok: true } }), {
          status: 200,
          headers: { "Content-Type": "application/json" },
        }),
      );

    const client = createGraphQLClient({ baseUrl: "http://backend:8082" });

    const results = await Promise.allSettled([
      client.query("query { fail }"),
      client.query<{ ok: boolean }>("query { ok }"),
    ]);

    expect(results[0].status).toBe("rejected");
    expect(results[1].status).toBe("fulfilled");
    if (results[1].status === "fulfilled") {
      expect((results[1].value as { ok: boolean }).ok).toBe(true);
    }
  });

  it("AbortControllers are per-request — aborting one does not leak to another", async () => {
    // Capture signals via mockImplementation (replaces all previous stubs)
    const capturedSignals: AbortSignal[] = [];
    let callCount = 0;
    mockFetch.mockImplementation((_url: string, init: RequestInit) => {
      if (init.signal) capturedSignals.push(init.signal as AbortSignal);
      callCount++;
      return Promise.resolve(
        new Response(JSON.stringify({ data: { n: callCount } }), {
          status: 200,
          headers: { "Content-Type": "application/json" },
        }),
      );
    });

    const client = createGraphQLClient({ baseUrl: "http://backend:8082" });
    await client.query("query { n }");
    await client.query("query { n }");

    expect(capturedSignals).toHaveLength(2);
    // Each request got its own signal instance
    expect(capturedSignals[0]).not.toBe(capturedSignals[1]);
  });
});

// ---------------------------------------------------------------------------
// GraphQL errors array edge cases
// ---------------------------------------------------------------------------

describe("client.attack — GraphQL errors array edge cases", () => {
  it("throws when errors array is present even with partial data", async () => {
    mockFetch.mockResolvedValueOnce(
      new Response(
        JSON.stringify({
          data: { partial: true },
          errors: [{ message: "partial failure" }],
        }),
        { status: 200, headers: { "Content-Type": "application/json" } },
      ),
    );

    const client = createGraphQLClient({ baseUrl: "http://backend:8082" });
    await expect(client.query("query { partial }")).rejects.toThrow(
      /partial failure/,
    );
  });

  it("concatenates multiple error messages", async () => {
    mockFetch.mockResolvedValueOnce(
      new Response(
        JSON.stringify({
          errors: [{ message: "error one" }, { message: "error two" }],
        }),
        { status: 200, headers: { "Content-Type": "application/json" } },
      ),
    );

    const client = createGraphQLClient({ baseUrl: "http://backend:8082" });
    await expect(client.query("query { ok }")).rejects.toThrow(/error one/);
  });

  it("throws when errors array is empty and data is null", async () => {
    mockFetch.mockResolvedValueOnce(
      new Response(JSON.stringify({ errors: [], data: null }), {
        status: 200,
        headers: { "Content-Type": "application/json" },
      }),
    );

    const client = createGraphQLClient({ baseUrl: "http://backend:8082" });
    await expect(client.query("query { ok }")).rejects.toThrow();
  });
});
