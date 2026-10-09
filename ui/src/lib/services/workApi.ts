// Work API client (design D1/D5). Browser side of GET /api/work/*.
//
// The server answers every non-2xx with a WorkApiError body ({code, error,
// issues?}); that body is carried on WorkApiRequestError so callers can tell a
// configuration problem (503) from an unknown repository (404) without
// re-parsing. Nothing here writes: every call is a GET.

import type {
  ItemResponse,
  ItemsResponse,
  PortfolioResponse,
  RunsResponse,
  WorkApiError,
} from "$lib/types/work";

const WORK_BASE = "/api/work";

export class WorkApiRequestError extends Error {
  /** The server's {code, error, issues?} body, when the response carried one. */
  readonly body: WorkApiError | null;

  constructor(
    message: string,
    public readonly status: number,
    body: WorkApiError | null = null,
  ) {
    super(message);
    this.name = "WorkApiRequestError";
    this.body = body;
  }

  /** Machine-readable server code, or a status-derived fallback. */
  get code(): string {
    return this.body?.code ?? `HTTP_${this.status}`;
  }
}

function isWorkApiError(value: unknown): value is WorkApiError {
  if (typeof value !== "object" || value === null) return false;
  const candidate = value as Record<string, unknown>;
  return typeof candidate.code === "string" && typeof candidate.error === "string";
}

async function getJson<T>(path: string, signal?: AbortSignal): Promise<T> {
  const response = await fetch(path, {
    headers: { Accept: "application/json" },
    ...(signal && { signal }),
  });
  if (!response.ok) {
    const detail: unknown = await response.json().catch(() => null);
    if (isWorkApiError(detail)) {
      throw new WorkApiRequestError(detail.error, response.status, detail);
    }
    throw new WorkApiRequestError(
      `Work request failed: ${response.status} ${response.statusText}`.trim(),
      response.status,
    );
  }
  return (await response.json()) as T;
}

// ---------------------------------------------------------------------------
// Item references: `owner/repo#number`
// ---------------------------------------------------------------------------

export interface ItemRef {
  owner: string;
  repo: string;
  number: number;
}

/** Parses `owner/repo#number`; null for anything else (including `?item=` junk). */
export function parseItemRef(ref: string): ItemRef | null {
  const match = /^([^/\s#]+)\/([^/\s#]+)#([1-9]\d*)$/.exec(ref);
  return match ? { owner: match[1], repo: match[2], number: Number(match[3]) } : null;
}

export function formatItemRef({ owner, repo, number }: ItemRef): string {
  return `${owner}/${repo}#${number}`;
}

function itemPath({ owner, repo, number }: ItemRef): string {
  return `${WORK_BASE}/items/${encodeURIComponent(owner)}/${encodeURIComponent(repo)}/${number}`;
}

// ---------------------------------------------------------------------------
// Endpoints
// ---------------------------------------------------------------------------

export function getPortfolio(signal?: AbortSignal): Promise<PortfolioResponse> {
  return getJson<PortfolioResponse>(`${WORK_BASE}/portfolio`, signal);
}

/** `repository` is `owner/name` as configured in the portfolio. */
export function getItems(repository: string, signal?: AbortSignal): Promise<ItemsResponse> {
  return getJson<ItemsResponse>(
    `${WORK_BASE}/items?repository=${encodeURIComponent(repository)}`,
    signal,
  );
}

export function getItem(ref: ItemRef, signal?: AbortSignal): Promise<ItemResponse> {
  return getJson<ItemResponse>(itemPath(ref), signal);
}

export function getLinkedRuns(ref: ItemRef, signal?: AbortSignal): Promise<RunsResponse> {
  return getJson<RunsResponse>(`${itemPath(ref)}/runs`, signal);
}
