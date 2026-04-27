// taskLabels — user-editable labels (titles + aliases) for tasks.
//
// Separate from taskRefs because the lifecycles are different:
//   taskRefs   — system-generated, monotonic, never recycled.
//   taskLabels — user-content, freely edited and cleared.
//
// API surface mirrors taskRefs: factory function returning getters,
// SvelteMap from svelte/reactivity for the per-loop tables, $state
// nowhere needed because everything lives in the maps. localStorage
// persistence with a typeof-guard for SSR. Title override is null when
// the user hasn't set one (caller falls back to the derived title).

import { SvelteMap } from "svelte/reactivity";

const STORAGE_KEY = "semteams:task-labels:v1";

interface Persisted {
  titles: Record<string, string>;
  aliases: Record<string, string[]>;
}

function loadPersisted(): Persisted {
  if (typeof localStorage === "undefined") return { titles: {}, aliases: {} };
  try {
    const raw = localStorage.getItem(STORAGE_KEY);
    if (!raw) return { titles: {}, aliases: {} };
    const parsed = JSON.parse(raw) as Persisted;
    return {
      titles: parsed?.titles ?? {},
      aliases: parsed?.aliases ?? {},
    };
  } catch {
    return { titles: {}, aliases: {} };
  }
}

function createTaskLabels() {
  const initial = loadPersisted();
  const titles = new SvelteMap<string, string>(Object.entries(initial.titles));
  const aliases = new SvelteMap<string, string[]>(
    Object.entries(initial.aliases),
  );

  function persist() {
    if (typeof localStorage === "undefined") return;
    try {
      localStorage.setItem(
        STORAGE_KEY,
        JSON.stringify({
          titles: Object.fromEntries(titles),
          aliases: Object.fromEntries(aliases),
        }),
      );
    } catch {
      // quota / private mode — drop silently.
    }
  }

  function normaliseAlias(alias: string): string {
    // Strip any leading @/# the user typed by reflex; an alias that's
    // just punctuation (`@@@` etc.) reduces to empty and is rejected.
    return alias.trim().replace(/^[#@]+/, "").toLowerCase();
  }

  return {
    /** User-set title override, or null if the user hasn't set one. */
    getTitle(loopId: string): string | null {
      return titles.get(loopId) ?? null;
    },

    /** Set a title override. Empty/whitespace-only clears the override. */
    setTitle(loopId: string, title: string) {
      const trimmed = title.trim();
      if (trimmed === "") titles.delete(loopId);
      else titles.set(loopId, trimmed);
      persist();
    },

    /** Drop the override; caller's fallback (prompt-derived) takes over. */
    clearTitle(loopId: string) {
      titles.delete(loopId);
      persist();
    },

    /** All user-defined aliases for this task. Order = insertion order. */
    getAliases(loopId: string): string[] {
      return aliases.get(loopId) ?? [];
    },

    /**
     * Add an alias. Returns false if the alias is empty, already
     * attached to this task, or already in use by a DIFFERENT task —
     * caller surfaces the third case as a conflict error.
     */
    addAlias(loopId: string, alias: string): boolean {
      const norm = normaliseAlias(alias);
      if (norm === "") return false;
      // Check uniqueness across all loops.
      for (const [otherLoopId, others] of aliases) {
        if (others.includes(norm)) {
          return otherLoopId === loopId; // already-attached → no-op success
        }
      }
      const current = aliases.get(loopId) ?? [];
      aliases.set(loopId, [...current, norm]);
      persist();
      return true;
    },

    /** Remove an alias. No-op if it doesn't exist. */
    removeAlias(loopId: string, alias: string) {
      const norm = normaliseAlias(alias);
      const current = aliases.get(loopId);
      if (!current) return;
      const filtered = current.filter((a) => a !== norm);
      if (filtered.length === 0) aliases.delete(loopId);
      else aliases.set(loopId, filtered);
      persist();
    },

    /** Reverse lookup — find a loop by alias. */
    findLoopByAlias(alias: string): string | null {
      const norm = normaliseAlias(alias);
      for (const [loopId, others] of aliases) {
        if (others.includes(norm)) return loopId;
      }
      return null;
    },

    /** Wipe everything. Tests only. */
    reset() {
      titles.clear();
      aliases.clear();
      persist();
    },
  };
}

export const taskLabels = createTaskLabels();
