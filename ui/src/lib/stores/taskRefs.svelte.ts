// taskRefs — short, GitHub-style "#42" handles for tasks.
//
// loop_id is "loop_e7e8c6bc" or "bac1e834-94c1-4d05-aa90-c027bab8cb4d";
// neither lets a human refer to the task in conversation. This store
// mints a monotonic counter ("#1, #2, #3, …") and maps each loop_id to
// its assigned number. The ref is the human handle; the loop_id stays
// canonical underneath.
//
// Persistence: localStorage today (browser-scoped, single-user). When
// we go multi-user / multi-tab-shared, the same store interface gets
// backed by a NATS KV counter in the product shell — TaskInfo callers
// don't change.
//
// Pattern note: factory function returning getters, SvelteMap from
// svelte/reactivity for the assignment table, $state for the counter.
// localStorage write happens synchronously after each mutation. No
// $effect inside the store — module-scope rune stores can't host one.
// Auto-assignment for newly-arrived loops lives in a layout $effect
// that calls ensure() per top-level loop.

import { SvelteMap } from "svelte/reactivity";

const STORAGE_KEY = "semteams:task-refs:v1";

interface Persisted {
  next: number;
  refs: Record<string, number>;
}

function loadPersisted(): Persisted {
  if (typeof localStorage === "undefined") return { next: 1, refs: {} };
  try {
    const raw = localStorage.getItem(STORAGE_KEY);
    if (!raw) return { next: 1, refs: {} };
    const parsed = JSON.parse(raw) as Persisted;
    if (
      typeof parsed?.next === "number" &&
      parsed.refs &&
      typeof parsed.refs === "object"
    ) {
      return parsed;
    }
    return { next: 1, refs: {} };
  } catch {
    return { next: 1, refs: {} };
  }
}

function createTaskRefs() {
  const initial = loadPersisted();
  const refsByLoop = new SvelteMap<string, number>(
    Object.entries(initial.refs),
  );
  let nextRef = $state<number>(initial.next);

  function persist() {
    if (typeof localStorage === "undefined") return;
    try {
      localStorage.setItem(
        STORAGE_KEY,
        JSON.stringify({
          next: nextRef,
          refs: Object.fromEntries(refsByLoop),
        }),
      );
    } catch {
      // quota / private mode — drop silently. Refs still work in-memory.
    }
  }

  return {
    /** Read a loop's ref, or null if not yet assigned. */
    get(loopId: string): number | null {
      return refsByLoop.get(loopId) ?? null;
    },

    /**
     * Idempotent — assign a ref if missing, return current. Never
     * recycles numbers; if a loop is forgotten and re-encountered, it
     * keeps its original ref because we key by loop_id.
     */
    ensure(loopId: string): number {
      const existing = refsByLoop.get(loopId);
      if (existing !== undefined) return existing;
      const ref = nextRef;
      nextRef = nextRef + 1;
      refsByLoop.set(loopId, ref);
      persist();
      return ref;
    },

    /** Reverse lookup — useful for resolving "@42" / "#42" mentions. */
    findLoopByRef(ref: number): string | null {
      for (const [loopId, n] of refsByLoop) {
        if (n === ref) return loopId;
      }
      return null;
    },

    /** Number that will be assigned to the next previously-unseen loop. */
    get nextRef(): number {
      return nextRef;
    },

    /** Total assignments. Mostly useful for tests + telemetry. */
    get size(): number {
      return refsByLoop.size;
    },

    /** Wipe everything. Currently only used by tests. */
    reset() {
      refsByLoop.clear();
      nextRef = 1;
      persist();
    },
  };
}

export const taskRefs = createTaskRefs();
