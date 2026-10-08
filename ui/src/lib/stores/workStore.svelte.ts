// Work lens store (design D5). Loads the configured portfolio, then the items
// of every configured repository with bounded concurrency, and reads linked
// runs for ONE item at a time, only when asked.
//
// Linked runs are a per-item server-side graph walk; reading them for a whole
// board would turn the work lens into a poller of the runtime. So a repository
// board never carries run facts: until an item's runs are requested its
// overlays are `unknown` (see utils/workView.ts), never `none`.
//
// The page owns the instance, so state is as local as it can be and a test gets
// a fresh store. There are no timers: data moves on load() and refresh().
//
// Plain arrays instead of Set/Map throughout: this file is subject to
// svelte/prefer-svelte-reactivity and none of this needs a reactive collection.

import {
  getItem,
  getItems,
  getLinkedRuns,
  getPortfolio,
  parseItemRef,
  WorkApiRequestError,
} from "$lib/services/workApi";
import type { LookupStatus, PortfolioIssue, Program, WorkItem } from "$lib/types/work";
import { repositoryIds } from "$lib/utils/workView";
import type { LinkedRunsState } from "$lib/utils/workView";

/** Items are read for at most this many repositories at once. */
export const ITEM_CONCURRENCY = 4;

/** Server code for "no work source configured"; an unconfigured source, not a failure. */
export const SOURCE_UNCONFIGURED_CODE = "WORK_SOURCE_UNCONFIGURED";

export type SourceState =
  | { kind: "loading" }
  | { kind: "ready" }
  | { kind: "unconfigured" }
  | { kind: "error"; code: string; message: string; issues?: PortfolioIssue[] };

/** One configured repository's items. A failed read is `lookup: "failed"` with its reason. */
export interface RepositoryBoard {
  repository: string;
  loading: boolean;
  lookup: LookupStatus;
  reason?: string;
  columns: string[];
  items: WorkItem[];
}

/** What the overview panel should show for `?item=`. */
export type Selection =
  | { kind: "none" }
  | { kind: "loading"; ref: string }
  | { kind: "item"; ref: string; item: WorkItem }
  | { kind: "unavailable"; ref: string; message: string };

type DetailState =
  | { status: "idle" }
  | { status: "loading"; ref: string }
  | { status: "ready"; ref: string; item: WorkItem }
  | { status: "error"; ref: string; message: string };

function describe(err: unknown): string {
  return err instanceof Error ? err.message : String(err);
}

function codeOf(err: unknown): string | null {
  return err instanceof WorkApiRequestError ? err.code : null;
}

export function createWorkStore() {
  let sourceState = $state<SourceState>({ kind: "loading" });
  let programs = $state<Program[]>([]);
  let boards = $state<Record<string, RepositoryBoard>>({});
  let refreshing = $state(false);
  let selectedRef = $state<string | null>(null);
  let detail = $state<DetailState>({ status: "idle" });
  let linkedRuns = $state<Record<string, LinkedRunsState>>({});

  let loadAbort: AbortController | null = null;
  let detailAbort: AbortController | null = null;
  let runsAbort: { controller: AbortController; ref: string } | null = null;

  function listedItem(ref: string): WorkItem | null {
    for (const board of Object.values(boards)) {
      const found = board.items.find((item) => item.ref === ref);
      if (found) return found;
    }
    return null;
  }

  function settled(): boolean {
    return (
      sourceState.kind === "ready" &&
      loadAbort === null &&
      Object.values(boards).every((board) => !board.loading)
    );
  }

  /**
   * An item the repository lists do not carry (e.g. a deep link) is read from the
   * item endpoint once the lists have settled, so the overview can say precisely
   * what is wrong instead of staying blank.
   */
  async function ensureDetail(): Promise<void> {
    const ref = selectedRef;
    if (!ref || listedItem(ref) || !settled()) return;
    if (detail.status !== "idle" && detail.ref === ref) return;
    const parsed = parseItemRef(ref);
    if (!parsed) {
      detail = { status: "error", ref, message: `"${ref}" is not a work item reference (owner/repo#number)` };
      return;
    }
    detailAbort?.abort();
    const controller = new AbortController();
    detailAbort = controller;
    detail = { status: "loading", ref };
    try {
      const response = await getItem(parsed, controller.signal);
      if (controller.signal.aborted) return;
      detail = response.item
        ? { status: "ready", ref, item: response.item }
        : { status: "error", ref, message: response.reason ?? `item lookup ${response.lookup}` };
    } catch (err) {
      if (controller.signal.aborted) return;
      detail = { status: "error", ref, message: describe(err) };
    } finally {
      if (detailAbort === controller) detailAbort = null;
    }
  }

  async function loadRepository(repository: string, signal: AbortSignal): Promise<void> {
    try {
      const response = await getItems(repository, signal);
      if (signal.aborted) return;
      boards[repository] = {
        repository,
        loading: false,
        lookup: response.lookup,
        ...(response.reason && { reason: response.reason }),
        columns: response.columns,
        items: response.items,
      };
    } catch (err) {
      if (signal.aborted) return;
      if (codeOf(err) === SOURCE_UNCONFIGURED_CODE) {
        sourceState = { kind: "unconfigured" };
        return;
      }
      boards[repository] = {
        repository,
        loading: false,
        lookup: "failed",
        reason: describe(err),
        columns: [],
        items: [],
      };
    }
  }

  /** Runs `work` over `ids` with at most `limit` in flight. */
  async function pool(ids: string[], limit: number, work: (id: string) => Promise<void>): Promise<void> {
    let next = 0;
    const worker = async (): Promise<void> => {
      while (next < ids.length) {
        const id = ids[next];
        next += 1;
        await work(id);
      }
    };
    await Promise.all(Array.from({ length: Math.min(limit, ids.length) }, worker));
  }

  async function load(): Promise<void> {
    loadAbort?.abort();
    const controller = new AbortController();
    loadAbort = controller;
    const { signal } = controller;
    refreshing = true;
    if (sourceState.kind !== "ready") sourceState = { kind: "loading" };

    try {
      const portfolio = await getPortfolio(signal);
      if (signal.aborted) return;
      if (portfolio.source === "unconfigured") {
        programs = [];
        boards = {};
        sourceState = { kind: "unconfigured" };
        return;
      }
      programs = portfolio.programs;
      const ids = repositoryIds(portfolio.programs);
      // Keep what is already shown while a refresh runs; only new repositories start empty.
      const next: Record<string, RepositoryBoard> = {};
      for (const id of ids) {
        next[id] = boards[id] ?? {
          repository: id,
          loading: true,
          lookup: "complete",
          columns: [],
          items: [],
        };
      }
      boards = next;
      sourceState = { kind: "ready" };
      await pool(ids, ITEM_CONCURRENCY, (id) => loadRepository(id, signal));
    } catch (err) {
      if (signal.aborted) return;
      sourceState =
        err instanceof WorkApiRequestError && err.code === SOURCE_UNCONFIGURED_CODE
          ? { kind: "unconfigured" }
          : {
              kind: "error",
              code: codeOf(err) ?? "NETWORK_ERROR",
              message: describe(err),
              ...(err instanceof WorkApiRequestError &&
                err.body?.issues && { issues: err.body.issues }),
            };
    } finally {
      if (loadAbort === controller) {
        loadAbort = null;
        refreshing = false;
        void ensureDetail();
      }
    }
  }

  async function loadLinkedRuns(): Promise<void> {
    const ref = selectedRef;
    const parsed = ref ? parseItemRef(ref) : null;
    if (!ref || !parsed) return;
    runsAbort?.controller.abort();
    const controller = new AbortController();
    runsAbort = { controller, ref };
    linkedRuns[ref] = { status: "loading" };
    try {
      const response = await getLinkedRuns(parsed, controller.signal);
      if (controller.signal.aborted) return;
      linkedRuns[ref] = {
        status: "loaded",
        lookup: response.lookup,
        ...(response.reason && { reason: response.reason }),
        runs: response.runs,
      };
    } catch (err) {
      if (controller.signal.aborted) return;
      if (codeOf(err) === SOURCE_UNCONFIGURED_CODE) sourceState = { kind: "unconfigured" };
      linkedRuns[ref] = {
        status: "error",
        code: codeOf(err) ?? "NETWORK_ERROR",
        message: describe(err),
      };
    } finally {
      if (runsAbort?.controller === controller) runsAbort = null;
    }
  }

  /** Re-selecting cancels the previous item's in-flight run read and forgets its half-finished state. */
  function select(ref: string | null): void {
    if (ref === selectedRef) return;
    if (runsAbort && runsAbort.ref !== ref) {
      runsAbort.controller.abort();
      linkedRuns[runsAbort.ref] = { status: "idle" };
      runsAbort = null;
    }
    detailAbort?.abort();
    detailAbort = null;
    detail = { status: "idle" };
    selectedRef = ref;
    void ensureDetail();
  }

  async function refresh(): Promise<void> {
    const reload = selectedRef !== null && (linkedRuns[selectedRef]?.status ?? "idle") !== "idle";
    linkedRuns = {};
    detail = { status: "idle" };
    // Not awaited together: the run read for the open item must not wait on every repository.
    const runs = reload ? loadLinkedRuns() : Promise.resolve();
    await Promise.all([load(), runs]);
  }

  /** Cancels everything in flight; call when the page goes away. */
  function dispose(): void {
    loadAbort?.abort();
    loadAbort = null;
    detailAbort?.abort();
    detailAbort = null;
    runsAbort?.controller.abort();
    runsAbort = null;
    refreshing = false;
  }

  return {
    get sourceState(): SourceState {
      return sourceState;
    },
    get programs(): Program[] {
      return programs;
    },
    get boards(): Record<string, RepositoryBoard> {
      return boards;
    },
    get refreshing(): boolean {
      return refreshing;
    },
    get selectedRef(): string | null {
      return selectedRef;
    },
    get selection(): Selection {
      const ref = selectedRef;
      if (!ref) return { kind: "none" };
      const listed = listedItem(ref);
      if (listed) return { kind: "item", ref, item: listed };
      if (detail.status !== "idle" && detail.ref === ref) {
        if (detail.status === "ready") return { kind: "item", ref, item: detail.item };
        if (detail.status === "error") return { kind: "unavailable", ref, message: detail.message };
      }
      return { kind: "loading", ref };
    },
    /** Loaded linked-run state for one item; `idle` until requested. */
    linkedRunsFor(ref: string): LinkedRunsState {
      return linkedRuns[ref] ?? { status: "idle" };
    },
    load,
    refresh,
    select,
    loadLinkedRuns,
    dispose,
  };
}

export type WorkStore = ReturnType<typeof createWorkStore>;
