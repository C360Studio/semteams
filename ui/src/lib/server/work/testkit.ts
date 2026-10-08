// Test support for the work API: a settable stand-in for `$env/dynamic/private`
// and a fake backend that answers the two reads the overlays make with the same
// filter semantics as the real GET /graph/triples (exact string match on
// subject, predicate and object; limit applied).
//
// Test-only: nothing in production imports this module.

import { vi } from "vitest";

export const mockEnv: Record<string, string | undefined> = {};

/** Replace the whole environment for one test. */
export function setEnv(values: Record<string, string | undefined>): void {
  for (const key of Object.keys(mockEnv)) delete mockEnv[key];
  Object.assign(mockEnv, values);
}

export interface FakeTriple {
  subject: string;
  predicate: string;
  object: unknown;
}

export function triple(subject: string, predicate: string, object: unknown): FakeTriple {
  return { subject, predicate, object };
}

export interface FakeWorld {
  triples?: FakeTriple[];
  /** loop id -> loop body, or an HTTP status number for a failing read. */
  loops?: Record<string, Record<string, unknown> | number>;
}

function jsonResponse(body: unknown, status = 200): Response {
  return new Response(JSON.stringify(body), {
    status,
    headers: { "Content-Type": "application/json" },
  });
}

export function fakeBackend(world: FakeWorld): (url: URL) => Response {
  return (url) => {
    if (url.pathname === "/graph/triples") {
      const subject = url.searchParams.get("subject");
      const predicate = url.searchParams.get("predicate");
      const object = url.searchParams.get("object");
      const limit = Number(url.searchParams.get("limit") ?? "100");
      const matches = (world.triples ?? [])
        .filter((t) => !subject || t.subject === subject)
        .filter((t) => !predicate || t.predicate === predicate)
        .filter((t) => !object || String(t.object) === object)
        .slice(0, limit);
      return jsonResponse(matches);
    }
    const loopMatch = /^\/teams-dispatch\/loops\/([^/]+)$/.exec(url.pathname);
    if (loopMatch) {
      const entry = world.loops?.[decodeURIComponent(loopMatch[1])];
      if (entry === undefined) return jsonResponse({ error: "loop not found" }, 404);
      return typeof entry === "number" ? jsonResponse({ error: "unavailable" }, entry) : jsonResponse(entry);
    }
    return jsonResponse({ error: "not found" }, 404);
  };
}

/** Install a fetch stub over a handler; returns the mock so tests can inspect calls. */
export function stubBackend(handler: (url: URL) => Response | Promise<Response>) {
  const fetchMock = vi.fn(async (input: Parameters<typeof fetch>[0]) => handler(new URL(String(input))));
  vi.stubGlobal("fetch", fetchMock);
  return fetchMock;
}

export function requestedUrls(fetchMock: ReturnType<typeof stubBackend>): URL[] {
  return fetchMock.mock.calls.map(([input]) => new URL(String(input)));
}
