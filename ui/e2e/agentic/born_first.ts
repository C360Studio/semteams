import type { APIRequestContext } from "@playwright/test";
import { expect } from "@playwright/test";

/**
 * Born-first anchor assertion — the ADR-055/056 must-exist-flip gate (semteams#222).
 *
 * semstreams is landing the BREAKING flip where a `triple.add` referencing an
 * entity that does not yet exist becomes an `entity_not_found` error instead of
 * silently auto-vivifying a stub. semteams' whole exposure is the bare-triple
 * lane: rule packs stamping MARKER triples onto ANCHOR entities. The prediction
 * is zero rejects by causal ordering (the anchor is always born before any
 * marker targets it) — but that must be PROVEN, not assumed.
 *
 * THE PRE-FLIP TRAP: auto-vivify is still present pre-flip, so "did the marker
 * land?" passes today even when ordering is wrong. Every born-first assertion
 * MUST be written so it FAILS under simulated auto-vivify. We do that by
 * deriving the anchor from the MARKER's own subject, then asserting that SAME
 * entity also carries its born-first semantic ENVELOPE — a lifecycle/spawn
 * identity predicate the framework stamps at birth, which a marker-vivified bare
 * stub would never carry:
 *   - chain.execution run anchors  → `agent.run.phase` (lifecycle Manager, at Create)
 *   - agentic-loop.execution loop/plan anchors → `agent.loop.role` (graph writer, at spawn)
 *
 * Reads graph state via GET /graph/triples (the same endpoint the journeys
 * already use). Triples carry no revision; envelope-PRESENCE is the auto-vivify-
 * failing proof the issue accepts (a bare stub has only the marker).
 */

export interface BornFirstTriple {
  subject?: string;
  predicate?: string;
  object?: unknown;
  timestamp?: string;
}

async function fetchTriples(
  request: APIRequestContext,
  params: { subject?: string; predicate?: string; limit?: number },
): Promise<BornFirstTriple[]> {
  const query = new URLSearchParams();
  if (params.subject) query.set("subject", params.subject);
  if (params.predicate) query.set("predicate", params.predicate);
  if (params.limit) query.set("limit", String(params.limit));
  const resp = await request.get(`/graph/triples?${query.toString()}`);
  if (!resp.ok()) {
    throw new Error(
      `GET /graph/triples?${query.toString()} returned ${resp.status()}: ${await resp.text()}`,
    );
  }
  return (await resp.json()) as BornFirstTriple[];
}

async function pollFor<T>(
  fn: () => Promise<T | null>,
  timeoutMs: number,
): Promise<T | null> {
  const deadline = Date.now() + timeoutMs;
  while (Date.now() < deadline) {
    const result = await fn();
    if (result != null) return result;
    await new Promise((r) => setTimeout(r, 300));
  }
  return null;
}

/** Born-first envelope predicate by anchor entity-id segment. */
export const RUN_ANCHOR = {
  substr: "agent.chain.execution.",
  envelope: "agent.run.phase",
} as const;
export const LOOP_ANCHOR = {
  substr: "agent.agentic-loop.execution.",
  envelope: "agent.loop.role",
} as const;

export interface BornFirstOpts {
  /** The marker predicate a rule stamps onto the anchor (e.g. "agent.run.outcome"). */
  markerPredicate: string;
  /** Optional: only consider markers whose object equals this (e.g. "failed"). */
  markerObject?: string;
  /** The born-first identity predicate the anchor must carry (RUN_ANCHOR.envelope / LOOP_ANCHOR.envelope). */
  envelopePredicate: string;
  /** Substring the anchor entity-id must contain (RUN_ANCHOR.substr / LOOP_ANCHOR.substr). */
  anchorSubstr: string;
  /** Human label for failure messages (e.g. "autoresearch/04c best.value"). */
  label: string;
  timeoutMs?: number;
}

/**
 * Assert the entity a marker landed on was BORN-FIRST (carries its lifecycle/
 * spawn identity envelope), not auto-vivified by the marker. Fails under
 * simulated auto-vivify.
 */
export async function assertAnchorBornFirst(
  request: APIRequestContext,
  opts: BornFirstOpts,
): Promise<void> {
  expect(
    opts.envelopePredicate,
    "born-first envelope predicate must DIFFER from the marker predicate (else the assertion is tautological)",
  ).not.toBe(opts.markerPredicate);

  // 1. Find the anchor: the subject the marker stamped (filtered to the right
  //    anchor entity-id shape, and optionally the right object).
  const anchor = await pollFor(async () => {
    const markers = await fetchTriples(request, {
      predicate: opts.markerPredicate,
      limit: 50,
    });
    const subj = markers
      .filter((m) =>
        opts.markerObject == null ? true : String(m.object) === opts.markerObject,
      )
      .map((m) => String(m.subject ?? ""))
      .find((s) => s.includes(opts.anchorSubstr));
    return subj && subj.length > 0 ? subj : null;
  }, opts.timeoutMs ?? 30_000);

  expect(
    anchor,
    `${opts.label}: no ${opts.markerPredicate}${opts.markerObject ? `=${opts.markerObject}` : ""} marker landed on a ${opts.anchorSubstr} anchor — cannot assert born-first ordering`,
  ).toBeTruthy();

  // 2. The marker's target MUST carry its born-first envelope. A marker-vivified
  //    bare stub would carry ONLY the marker, never this birth-stamped predicate.
  const triples = await fetchTriples(request, { subject: anchor!, limit: 200 });
  const predicates = new Set(triples.map((t) => String(t.predicate ?? "")));
  expect(
    predicates.has(opts.envelopePredicate),
    `${opts.label}: anchor ${anchor} carries the marker ${opts.markerPredicate} but NOT its born-first envelope ` +
      `${opts.envelopePredicate} — the marker auto-vivified a bare stub instead of landing on an entity born by the ` +
      `lifecycle Manager / agentic-loop graph writer. ADR-055/056 must-exist flip WILL reject this (semteams#222). ` +
      `Predicates present: ${[...predicates].sort().join(", ")}`,
  ).toBeTruthy();
}
