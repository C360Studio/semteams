// On-demand reads that explain a control (design D3). Not polled: the caller
// reads once per control when the story is shown and again when a control's
// loop state changes. Each read is bounded, and a result that fills the bound
// is treated as truncated rather than complete.

import { getTriples } from "./runStatusApi";
import type { RawTriple } from "./runStatusApi";
import { deriveControlEvidence } from "$lib/types/control";
import type { Control, ControlEvidence, ControlReads } from "$lib/types/control";

export const RUN_READ_LIMIT = 200;
export const LOOP_READ_LIMIT = 200;

interface Read {
  triples: RawTriple[] | null;
  truncated: boolean;
  error?: string;
}

async function read(subject: string, limit: number, signal?: AbortSignal): Promise<Read> {
  try {
    const triples = await getTriples({ subject, limit, signal });
    return { triples, truncated: triples.length >= limit };
  } catch (err) {
    // An abort means the caller moved on; let it unwind instead of caching an
    // "unknown" that was never a real answer.
    if (err instanceof DOMException && err.name === "AbortError") throw err;
    return {
      triples: null,
      truncated: false,
      error: err instanceof Error ? err.message : String(err),
    };
  }
}

/**
 * Read the firing run entity once per distinct run and each control loop's own
 * entity, then derive one ControlEvidence per control, keyed by loop id.
 * A failed read degrades only the fields it feeds to "unknown".
 */
export async function loadControlEvidence(
  controls: Control[],
  signal?: AbortSignal,
): Promise<Record<string, ControlEvidence>> {
  const firingIds = [...new Set(controls.map((c) => c.firingEntityId))];
  const [runReads, loopReads] = await Promise.all([
    Promise.all(firingIds.map((id) => read(id, RUN_READ_LIMIT, signal))),
    Promise.all(controls.map((c) => read(c.loopEntityId, LOOP_READ_LIMIT, signal))),
  ]);
  const runByFiring: Record<string, Read> = {};
  firingIds.forEach((id, i) => (runByFiring[id] = runReads[i]));

  const out: Record<string, ControlEvidence> = {};
  controls.forEach((control, i) => {
    const run = runByFiring[control.firingEntityId];
    const loop = loopReads[i];
    const reads: ControlReads = {
      run: run.triples,
      runTruncated: run.truncated,
      runError: run.error,
      loop: loop.triples,
      loopTruncated: loop.truncated,
      loopError: loop.error,
    };
    out[control.loopId] = deriveControlEvidence(control, reads);
  });
  return out;
}
