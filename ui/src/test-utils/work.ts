// Builders and a fetch router for the work lens tests. Unknown overlays always
// carry a reason, as the server emits them.

import { vi } from "vitest";
import type {
  ItemsResponse,
  LinkedRun,
  PortfolioResponse,
  Program,
  RunsResponse,
  WorkItem,
} from "$lib/types/work";

export function makeItem(overrides: Partial<WorkItem> = {}): WorkItem {
  return {
    ref: "acme/widgets#1",
    repository: "acme/widgets",
    number: 1,
    title: "Add a widget",
    body_excerpt: "Explain why widgets matter.",
    state: "open",
    labels: [],
    assignees: [],
    column: "Todo",
    dependencies: [],
    delivery: { state: "unknown", reason: "source does not supply linked pull requests" },
    ...overrides,
  };
}

export function makeRun(overrides: Partial<LinkedRun> = {}): LinkedRun {
  return {
    run_entity_id: "acme.platform.chain.agent.execution.run-1",
    coordinator_loop_id: "loop-1",
    lookup: "complete",
    execution_stage: { state: "known", value: "completed" },
    needs_you: { state: "known", value: false },
    verification: { state: "unknown", reason: "no verification fact for research runs" },
    ...overrides,
  };
}

export function makePortfolio(repositories: string[]): PortfolioResponse {
  const program: Program = {
    id: "acme",
    name: "Acme",
    projects: [
      {
        id: "platform",
        name: "Platform",
        repositories: repositories.map((id) => {
          const [owner, name] = id.split("/");
          return { owner, name };
        }),
      },
    ],
  };
  return { source: "fixture", programs: [program] };
}

export function makeItemsResponse(
  repository: string,
  items: WorkItem[],
  overrides: Partial<ItemsResponse> = {},
): ItemsResponse {
  return {
    lookup: "complete",
    repository,
    columns: ["Todo", "In Progress", "Done"],
    items,
    ...overrides,
  };
}

export function makeRunsResponse(runs: LinkedRun[], overrides: Partial<RunsResponse> = {}): RunsResponse {
  return { lookup: "complete", runs, ...overrides };
}

export function json(body: unknown, status = 200): Response {
  return new Response(JSON.stringify(body), {
    status,
    headers: { "Content-Type": "application/json" },
  });
}

type FetchInput = Parameters<typeof fetch>[0];
type FetchInit = Parameters<typeof fetch>[1];

export type Route = (
  url: URL,
  init: FetchInit,
) => Response | undefined | Promise<Response | undefined>;

/**
 * Stubs global fetch with the first route that answers; anything unrouted is a
 * 404 WorkApiError so a missed route fails loudly instead of hanging.
 */
export function stubWorkFetch(...routes: Route[]) {
  const fetchMock = vi.fn(async (input: FetchInput, init?: FetchInit) => {
    const url = new URL(String(input), "http://localhost");
    for (const route of routes) {
      const response = await route(url, init);
      if (response) return response;
    }
    return json({ code: "TEST_UNROUTED", error: `unrouted ${url.pathname}${url.search}` }, 404);
  });
  vi.stubGlobal("fetch", fetchMock);
  return fetchMock;
}

/** A promise settled by hand, for holding a request in flight. */
export function deferred<T = void>() {
  let resolve!: (value: T) => void;
  const promise = new Promise<T>((r) => {
    resolve = r;
  });
  return { promise, resolve };
}

/** Rejects like fetch does when its signal aborts. */
export function abortable<T>(signal: AbortSignal | null | undefined, work: Promise<T>): Promise<T> {
  return new Promise<T>((resolve, reject) => {
    if (signal?.aborted) {
      reject(new DOMException("aborted", "AbortError"));
      return;
    }
    signal?.addEventListener("abort", () => reject(new DOMException("aborted", "AbortError")), {
      once: true,
    });
    work.then(resolve, reject);
  });
}
