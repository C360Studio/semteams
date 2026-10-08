// View-model helpers for the work lens (design D2/D5).
//
// Board cards and table rows show execution stage, needs-you and verification,
// but those are run facts and linked runs are read per item on request, never
// for the whole board. Until an item's runs are loaded the honest answer is
// `unknown`, with the reason; it is never `none`. `none` is legal only for a
// completed lookup that found no runs.

import type { LinkedRun, LookupStatus, Overlay, Program } from "$lib/types/work";

export type LinkedRunsState =
  | { status: "idle" }
  | { status: "loading" }
  | { status: "loaded"; lookup: LookupStatus; reason?: string; runs: LinkedRun[] }
  | { status: "error"; code: string; message: string };

export const RUNS_NOT_LOADED = "linked runs are read when an item is opened";
export const RUNS_LOADING = "linked runs are loading";

/**
 * Said once per lens (board, table) so the unknown badges are explained in
 * visible text without repeating the reason on every card or row.
 */
export const RUN_FACTS_LENS_NOTE =
  "Run facts are read when an item is opened; open an item and load its runs.";

export interface ItemOverlays {
  executionStage: Overlay<string>;
  needsYou: Overlay<boolean>;
  verification: Overlay<string>;
  linkedRuns: Overlay<number>;
}

function allUnknown(reason: string): ItemOverlays {
  const unknown = { state: "unknown" as const, reason };
  return {
    executionStage: unknown,
    needsYou: unknown,
    verification: unknown,
    linkedRuns: unknown,
  };
}

function reasonOf(overlays: Overlay<unknown>[], total: number): string {
  const reasons = overlays.map((o) => o.reason).filter((r): r is string => Boolean(r));
  const count = `${overlays.length} of ${total} linked runs unknown`;
  return reasons.length > 0 ? `${count}: ${reasons.join("; ")}` : count;
}

function combineText(overlays: Overlay<string>[]): Overlay<string> {
  if (overlays.length === 1) return overlays[0];
  const unknown = overlays.filter((o) => o.state === "unknown");
  if (unknown.length > 0) {
    return { state: "unknown", reason: reasonOf(unknown, overlays.length) };
  }
  const values = overlays
    .flatMap((o) => (o.state === "known" ? [o.value] : []))
    .filter((value, index, all) => all.indexOf(value) === index);
  return values.length > 0 ? { state: "known", value: values.join(", ") } : { state: "none" };
}

function combineNeedsYou(overlays: Overlay<boolean>[]): Overlay<boolean> {
  if (overlays.length === 1) return overlays[0];
  // One run waiting on the operator settles the summary whatever the others say.
  const waiting = overlays.find((o) => o.state === "known" && o.value === true);
  if (waiting) return waiting;
  const unknown = overlays.filter((o) => o.state === "unknown");
  if (unknown.length > 0) {
    return { state: "unknown", reason: reasonOf(unknown, overlays.length) };
  }
  return overlays.some((o) => o.state === "known")
    ? { state: "known", value: false }
    : { state: "none" };
}

/** One item's overlay summary from whatever linked-run state has been loaded for it. */
export function summarizeItemOverlays(state: LinkedRunsState | undefined): ItemOverlays {
  if (!state || state.status === "idle") return allUnknown(RUNS_NOT_LOADED);
  if (state.status === "loading") return allUnknown(RUNS_LOADING);
  if (state.status === "error") return allUnknown(`linked runs unavailable: ${state.message}`);

  if (state.runs.length === 0) {
    if (state.lookup === "complete") {
      const none = { state: "none" as const };
      return { executionStage: none, needsYou: none, verification: none, linkedRuns: none };
    }
    return allUnknown(state.reason ?? `linked-run lookup ${state.lookup}`);
  }
  const needsYou = combineNeedsYou(state.runs.map((run) => run.needs_you));
  if (state.lookup === "complete") {
    return {
      executionStage: combineText(state.runs.map((run) => run.execution_stage)),
      needsYou,
      verification: combineText(state.runs.map((run) => run.verification)),
      linkedRuns: { state: "known", value: state.runs.length },
    };
  }

  // Some runs came back but the lookup did not complete, so more may exist.
  // Everything a missing run could change is unknown: "no" needs-you, the run
  // count and the stage/verification join. Only a known `true` survives, because
  // one run waiting on the operator is true whatever else is missing.
  const count = state.runs.length;
  const unknown = {
    state: "unknown" as const,
    reason:
      `at least ${count} ${count === 1 ? "run" : "runs"}; lookup ${state.lookup}` +
      (state.reason ? `: ${state.reason}` : ""),
  };
  return {
    executionStage: unknown,
    needsYou: needsYou.state === "known" && needsYou.value ? needsYou : unknown,
    verification: unknown,
    linkedRuns: unknown,
  };
}

/**
 * `none` means "looked, and there is nothing" and is only believable from a
 * complete lookup. From anything else it is shown as unknown.
 */
export function guardNone<T>(overlay: Overlay<T>, lookup: LookupStatus): Overlay<T> {
  if (overlay.state !== "none" || lookup === "complete") return overlay;
  return { state: "unknown", reason: overlay.reason ?? `lookup ${lookup}` };
}

// ---------------------------------------------------------------------------
// Portfolio helpers
// ---------------------------------------------------------------------------

/** Every distinct configured repository as `owner/name`, in portfolio order. */
export function repositoryIds(programs: Program[]): string[] {
  return programs
    .flatMap((program) =>
      program.projects.flatMap((project) =>
        project.repositories.map((repository) => `${repository.owner}/${repository.name}`),
      ),
    )
    .filter((id, index, all) => all.indexOf(id) === index);
}

/** Stable test-id / DOM-id fragment: `In Progress` -> `in-progress`. */
export function slug(text: string): string {
  return text.toLowerCase().replace(/[^a-z0-9]+/g, "-").replace(/^-+|-+$/g, "");
}
