// Run overlays for a work item (design D2), read server-side so the browser
// cannot use the work API to walk the graph.
//
// Reads, all GET, all with a per-fetch timeout:
//   GET /graph/triples?...        run facts, loop identity, product run markers
//   GET /teams-dispatch/loops/{id} coordinator loop state / pending_approval
//
// The loops endpoints carry no prompt, metadata or parent_loop_id on the frozen
// runtime, so a coordinator is found through its graph facts
// (agent.loop.description, agent.loop.role, agent.run.entity-id) instead.
//
// A failed read degrades that lookup to `partial` or `failed`; it is never
// reported as `none`. The same holds for a read that cannot be trusted to be
// whole: the frozen /graph/triples returns [] when it cannot read and stops
// silently at `limit`, so an empty answer proves nothing and a full one may be
// cut short. Both degrade to `partial` with a reason.

import type {
  LinkedRun,
  LookupStatus,
  Overlay,
  RunPhase,
} from "$lib/types/work";
import type { LinkedRunBinding } from "./source";

export const READ_TIMEOUT_MS = 5000;

const RUN_INFIX = ".chain.agent.execution.";
const LOOP_INFIX = ".agentic-loop.agent.execution.";
const RUN_PHASES: readonly RunPhase[] = [
  "dispatched",
  "executing",
  "awaiting_approval",
  "completed",
  "failed",
  "cancelled",
];

// Read limits. A result at the limit is treated as truncated (see truncation()).
const RUN_FACTS_LIMIT = 200;
const LOOP_FACTS_LIMIT = 200;
const PROMPT_LISTING_LIMIT = 50;

export const PROMPT_BINDING_UNMATCHED_REASON = "prompt binding matched no coordinator loop";

function truncation(limit: number): string {
  return `read truncated at ${limit} triples`;
}

export const VERIFICATION_UNKNOWN_REASON = "no verification fact for research runs";
export const NOT_YET_STAMPED_REASON = "run entity not yet stamped";

interface Triple {
  subject: string;
  predicate: string;
  object: unknown;
}

type Read<T> = { ok: true; value: T } | { ok: false; error: string };

function isObject(value: unknown): value is Record<string, unknown> {
  return typeof value === "object" && value !== null && !Array.isArray(value);
}

/** Triple objects arrive as strings or numbers depending on the writer; normalize. */
function text(value: unknown): string {
  if (typeof value === "string") return value.trim();
  return value === undefined || value === null ? "" : String(value);
}

// ---------------------------------------------------------------------------
// Backend reads
// ---------------------------------------------------------------------------

interface Reader {
  triples(params: Record<string, string>): Promise<Read<Triple[]>>;
  loop(loopId: string): Promise<Read<Record<string, unknown>>>;
}

async function getJson(url: string, label: string): Promise<unknown> {
  const controller = new AbortController();
  const timer = setTimeout(() => controller.abort(), READ_TIMEOUT_MS);
  try {
    const response = await fetch(url, {
      signal: controller.signal,
      headers: { Accept: "application/json" },
    });
    if (!response.ok) throw new Error(`${label} answered ${response.status}`);
    return await response.json();
  } catch (err) {
    if (err instanceof Error && err.name === "AbortError") {
      throw new Error(`${label} timed out after ${READ_TIMEOUT_MS} ms`);
    }
    throw err;
  } finally {
    clearTimeout(timer);
  }
}

async function attempt<T>(fn: () => Promise<T>): Promise<Read<T>> {
  try {
    return { ok: true, value: await fn() };
  } catch (err) {
    return { ok: false, error: err instanceof Error ? err.message : String(err) };
  }
}

function createReader(backendHost: string): Reader {
  const base = `http://${backendHost}`;
  return {
    triples: (params) =>
      attempt(async () => {
        const body = await getJson(
          `${base}/graph/triples?${new URLSearchParams(params).toString()}`,
          "GET /graph/triples",
        );
        if (!Array.isArray(body)) throw new Error("GET /graph/triples returned a non-array body");
        return body.filter(
          (t): t is Triple =>
            isObject(t) && typeof t.subject === "string" && typeof t.predicate === "string",
        );
      }),
    loop: (loopId) =>
      attempt(async () => {
        const path = `/teams-dispatch/loops/${encodeURIComponent(loopId)}`;
        const body = await getJson(`${base}${path}`, `GET ${path}`);
        if (!isObject(body)) throw new Error(`GET ${path} returned a non-object body`);
        return body;
      }),
  };
}

// ---------------------------------------------------------------------------
// Overlay derivation
// ---------------------------------------------------------------------------

function valuesOf(triples: Triple[], predicate: string): string[] {
  return triples.filter((t) => t.predicate === predicate).map((t) => text(t.object)).filter(Boolean);
}

function lastValue(triples: Triple[], predicate: string): string {
  return valuesOf(triples, predicate).at(-1) ?? "";
}

function bareLoopId(ref: string): string {
  const idx = ref.indexOf(LOOP_INFIX);
  return idx === -1 ? ref.trim() : ref.slice(idx + LOOP_INFIX.length);
}

function stageOverlay(facts: FactsState): Overlay<RunPhase> {
  if (facts.kind !== "ok") return { state: "unknown", reason: facts.reason };
  const phase = lastValue(facts.triples, "agent.run.phase");
  if (!phase) return { state: "unknown", reason: "no agent.run.phase fact on the run entity" };
  if (!(RUN_PHASES as readonly string[]).includes(phase)) {
    return { state: "unknown", reason: `unrecognised agent.run.phase "${phase}"` };
  }
  return { state: "known", value: phase as RunPhase };
}

function needsYouOverlay(facts: FactsState, loop: LoopState): Overlay<boolean> {
  const reasons: string[] = [];
  if (loop.kind === "ok") {
    if (loop.info.state === "awaiting_approval") reasons.push("coordinator loop is awaiting approval");
    else if (isObject(loop.info.pending_approval)) reasons.push("coordinator loop has a pending approval");
  }
  // A marker found in a truncated read still holds (true is definitive); only
  // its absence is unprovable, which is why `false` below needs an `ok` read.
  const triples = readTriples(facts);
  if (triples) {
    if (triples.some((t) => t.predicate === "agent.run.clarification-pending")) {
      reasons.push("run is waiting for a clarification answer");
    }
    const outstanding = Number(lastValue(triples, "agent.run.approval-outstanding"));
    if (outstanding > 0) reasons.push(`${outstanding} approval gate(s) outstanding`);
  }
  if (reasons.length > 0) return { state: "known", value: true, reason: reasons.join("; ") };
  if (facts.kind === "ok" && loop.kind === "ok") return { state: "known", value: false };

  const gaps: string[] = [];
  if (facts.kind !== "ok") gaps.push(facts.reason);
  if (loop.kind !== "ok") gaps.push(loop.reason);
  return { state: "unknown", reason: gaps.join("; ") };
}

// ---------------------------------------------------------------------------
// One run
// ---------------------------------------------------------------------------

type FactsState =
  | { kind: "ok"; triples: Triple[] }
  /** Read returned `limit` triples: what is here is true, what is absent is unproven. */
  | { kind: "truncated"; triples: Triple[]; reason: string }
  | { kind: "missing" | "error" | "unstamped"; reason: string };

function readTriples(facts: FactsState): Triple[] | undefined {
  return facts.kind === "ok" || facts.kind === "truncated" ? facts.triples : undefined;
}

type LoopState =
  | { kind: "ok"; info: Record<string, unknown> }
  | { kind: "unknown" | "error"; reason: string };

interface RunTarget {
  /** Full run entity id; null while the run is not stamped. */
  runEntityId: string | null;
  /** Bare coordinator loop id when already known; else read from the run's handoff. */
  coordinatorLoopId: string | null;
}

async function resolveRun(reader: Reader, target: RunTarget): Promise<LinkedRun> {
  let attempted = 0;
  let succeeded = 0;
  const tally = <T>(read: Read<T>): Read<T> => {
    attempted += 1;
    if (read.ok) succeeded += 1;
    return read;
  };

  const readLoop = async (id: string): Promise<LoopState> => {
    const read = tally(await reader.loop(id));
    return read.ok
      ? { kind: "ok", info: read.value }
      : { kind: "error", reason: `coordinator loop ${id} unreadable: ${read.error}` };
  };

  let loopPromise: Promise<LoopState> | undefined = target.coordinatorLoopId
    ? readLoop(target.coordinatorLoopId)
    : undefined;

  let facts: FactsState;
  if (target.runEntityId === null) {
    facts = { kind: "unstamped", reason: NOT_YET_STAMPED_REASON };
  } else {
    const read = tally(await reader.triples({ subject: target.runEntityId, limit: String(RUN_FACTS_LIMIT) }));
    if (!read.ok) {
      facts = { kind: "error", reason: `run facts for ${target.runEntityId} unreadable: ${read.error}` };
    } else if (read.value.length === 0) {
      facts = { kind: "missing", reason: `run entity ${target.runEntityId} has no facts` };
    } else if (read.value.length >= RUN_FACTS_LIMIT) {
      facts = {
        kind: "truncated",
        triples: read.value,
        reason: `run facts for ${target.runEntityId}: ${truncation(RUN_FACTS_LIMIT)}`,
      };
    } else {
      facts = { kind: "ok", triples: read.value };
    }
  }

  let coordinatorLoopId = target.coordinatorLoopId;
  const runTriples = readTriples(facts);
  if (coordinatorLoopId === null && runTriples) {
    const handoff = lastValue(runTriples, "agent.run.handoff");
    if (handoff) {
      coordinatorLoopId = bareLoopId(handoff);
      loopPromise = readLoop(coordinatorLoopId);
    }
  }
  const loop: LoopState = loopPromise
    ? await loopPromise
    : { kind: "unknown", reason: "coordinator loop id unknown (no agent.run.handoff on the run entity)" };

  const execution_stage = stageOverlay(facts);
  const needs_you = needsYouOverlay(facts, loop);

  const resolved = execution_stage.state === "known" && needs_you.state === "known";
  const lookup: LookupStatus = resolved
    ? "complete"
    : attempted > 0 && succeeded === 0
      ? "failed"
      : "partial";
  // One reason per unreadable source, not per overlay, so a single failed read
  // is not reported twice.
  const reasons: string[] = [];
  if (facts.kind !== "ok") reasons.push(facts.reason);
  else if (execution_stage.reason) reasons.push(execution_stage.reason);
  if (needs_you.state === "unknown" && loop.kind !== "ok") reasons.push(loop.reason);

  return {
    run_entity_id: target.runEntityId,
    coordinator_loop_id: coordinatorLoopId,
    lookup,
    ...(lookup !== "complete" && { reason: reasons.join("; ") }),
    execution_stage,
    needs_you,
    verification: { state: "unknown", reason: VERIFICATION_UNKNOWN_REASON },
  };
}

// ---------------------------------------------------------------------------
// Bindings
// ---------------------------------------------------------------------------

interface Part {
  lookup: LookupStatus;
  reason?: string;
}

interface BindingResolution {
  runs: LinkedRun[];
  /** Gaps with no run to carry them: a failed or truncated listing, or a binding that matched nothing. */
  parts: Part[];
}

/** A coordinator found by prompt: its graph facts name the run it minted. */
async function resolvePromptBinding(reader: Reader, equals: string): Promise<BindingResolution> {
  const listing = await reader.triples({
    predicate: "agent.loop.description",
    object: equals,
    limit: String(PROMPT_LISTING_LIMIT),
  });
  if (!listing.ok) {
    return {
      runs: [],
      parts: [{ lookup: "failed", reason: `loop lookup by prompt failed: ${listing.error}` }],
    };
  }

  const loopEntities = [
    ...new Set(
      listing.value
        .map((t) => t.subject)
        .filter((s) => s.includes(LOOP_INFIX) && bareLoopId(s) !== ""),
    ),
  ];

  const parts: Part[] = [];
  if (listing.value.length >= PROMPT_LISTING_LIMIT) {
    parts.push({ lookup: "partial", reason: `loop lookup by prompt: ${truncation(PROMPT_LISTING_LIMIT)}` });
  }

  const resolved = await Promise.all(
    loopEntities.map(async (loopEntity): Promise<{ run: LinkedRun } | { part: Part } | null> => {
      const loopId = bareLoopId(loopEntity);
      const read = await reader.triples({ subject: loopEntity, limit: String(LOOP_FACTS_LIMIT) });
      if (!read.ok) {
        return {
          run: {
            run_entity_id: null,
            coordinator_loop_id: loopId,
            lookup: "failed",
            reason: `loop facts for ${loopId} unreadable: ${read.error}`,
            execution_stage: { state: "unknown", reason: "loop facts unreadable" },
            needs_you: { state: "unknown", reason: "loop facts unreadable" },
            verification: { state: "unknown", reason: VERIFICATION_UNKNOWN_REASON },
          },
        };
      }
      // A cut-short read can hide the role or the run anchor, so this loop can
      // be neither claimed nor ruled out. It is a part, not a run: inventing a
      // run for a loop that may be a researcher would be wrong the other way.
      if (read.value.length >= LOOP_FACTS_LIMIT) {
        return { part: { lookup: "partial", reason: `loop facts for ${loopId}: ${truncation(LOOP_FACTS_LIMIT)}` } };
      }
      // Only a coordinator's run is the work item's run; a researcher loop can
      // share a prompt with it.
      if (!valuesOf(read.value, "agent.loop.role").includes("coordinator")) return null;

      // The coordinator's own run is the anchor whose instance is its agent.loop.run;
      // a recovery coordinator can carry an inherited anchor as well.
      const anchors = valuesOf(read.value, "agent.run.entity-id");
      const ownRun = lastValue(read.value, "agent.loop.run");
      const runEntityId =
        (ownRun && anchors.find((a) => a.endsWith(`${RUN_INFIX}${ownRun}`))) || anchors[0] || null;
      return { run: await resolveRun(reader, { runEntityId, coordinatorLoopId: loopId }) };
    }),
  );
  const runs: LinkedRun[] = [];
  for (const entry of resolved) {
    if (entry === null) continue;
    if ("run" in entry) runs.push(entry.run);
    else parts.push(entry.part);
  }

  // A binding that was declared but resolved to nothing is not "no linked run":
  // the frozen /graph/triples answers [] when it cannot read, so an empty
  // listing proves nothing. Only an empty binding list is a complete "none".
  if (runs.length === 0 && parts.length === 0) {
    parts.push({ lookup: "partial", reason: PROMPT_BINDING_UNMATCHED_REASON });
  }
  return { runs, parts };
}

async function resolveBinding(reader: Reader, binding: LinkedRunBinding): Promise<BindingResolution> {
  if (binding.by === "coordinator_prompt") return resolvePromptBinding(reader, binding.equals);
  const run = await resolveRun(reader, { runEntityId: binding.value, coordinatorLoopId: null });
  return { runs: [run], parts: [] };
}

export interface RunsResolution {
  lookup: LookupStatus;
  reason?: string;
  runs: LinkedRun[];
}

/**
 * Resolve an item's declared run bindings against the backend. An empty
 * binding list is a `complete` lookup with no runs: the one legal "no linked
 * run". A declared binding that resolves to nothing is `partial`, never that.
 */
export async function resolveLinkedRuns(
  bindings: LinkedRunBinding[],
  backendHost: string,
): Promise<RunsResolution> {
  const reader = createReader(backendHost);
  const resolutions = await Promise.all(bindings.map((b) => resolveBinding(reader, b)));

  const seen = new Set<string>();
  const runs: LinkedRun[] = [];
  for (const run of resolutions.flatMap((r) => r.runs)) {
    if (run.run_entity_id !== null) {
      if (seen.has(run.run_entity_id)) continue;
      seen.add(run.run_entity_id);
    }
    runs.push(run);
  }

  const parts: Part[] = [...runs, ...resolutions.flatMap((r) => r.parts)];
  const incomplete = parts.filter((p) => p.lookup !== "complete");
  const lookup: LookupStatus =
    incomplete.length === 0
      ? "complete"
      : incomplete.every((p) => p.lookup === "failed") && incomplete.length === parts.length
        ? "failed"
        : "partial";
  const reason = [...new Set(incomplete.map((p) => p.reason).filter((r): r is string => Boolean(r)))].join("; ");
  return { lookup, ...(reason && { reason }), runs };
}
