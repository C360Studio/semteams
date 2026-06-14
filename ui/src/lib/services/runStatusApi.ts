// Run-status triple reader
// Reads raw triples from /graph/triples for run-level pause detection.
// Separate from graphApi.ts (which uses /graphql) — the triple endpoint
// is a lighter REST path used by the polling store.

import { AgentApiError } from "./agentApi";

/**
 * A raw triple from the /graph/triples REST endpoint.
 * Wire shape matches the backend handler (go-side TriplesResponse).
 */
export interface RawTriple {
  subject: string;
  predicate: string;
  object: string;
  confidence?: number;
  source?: string;
  timestamp?: string;
}

const TRIPLES_ENDPOINT = "/graph/triples";

/**
 * Fetch raw triples matching the given filter params.
 * All params are optional; at minimum pass `predicate` for practical use.
 * Throws AgentApiError on non-2xx responses.
 */
export async function getTriples(params: {
  subject?: string;
  predicate?: string;
  object?: string;
  limit?: number;
  signal?: AbortSignal;
}): Promise<RawTriple[]> {
  const qs = new URLSearchParams();
  if (params.subject !== undefined) qs.set("subject", params.subject);
  if (params.predicate !== undefined) qs.set("predicate", params.predicate);
  if (params.object !== undefined) qs.set("object", params.object);
  if (params.limit !== undefined) qs.set("limit", String(params.limit));

  const url = qs.toString()
    ? `${TRIPLES_ENDPOINT}?${qs.toString()}`
    : TRIPLES_ENDPOINT;

  const response = await fetch(url, params.signal ? { signal: params.signal } : undefined);
  if (!response.ok) {
    const detail = await response.json().catch(() => ({}));
    throw new AgentApiError(
      `Failed to fetch triples: ${response.statusText}`,
      response.status,
      detail,
    );
  }
  return response.json() as Promise<RawTriple[]>;
}
