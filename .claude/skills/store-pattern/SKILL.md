---
name: store-pattern
description: Choose the right state management approach for a use case. Decide between component-local state, derived state, runes-based stores, SvelteKit load, or URL state.
argument-hint: [description of the state being managed]
---

# State Management Pattern Selection

## What state are you managing?

$ARGUMENTS

## Quick Decision

```
Where is this state used?

  One component only           --> $state (component-local)
  Computed from other state    --> $derived
  Shared across components     --> Store (factory + runes)
  Loaded from backend on nav   --> SvelteKit load function
  Should survive refresh       --> URL state ($page.url)
  Multiple of the above        --> Combine patterns (see below)
```

## The 5 Patterns

### 1. Component-Local `$state`

**Use for:** UI state owned by a single component (open/closed, selected tab, input value).

```svelte
<script lang="ts">
  let isOpen = $state(false);
  let selectedTab = $state<string>("overview");
  let searchQuery = $state("");
</script>
```

**Not for:** Data that other components need, or data from the backend.

### 2. `$derived` / `$derived.by`

**Use for:** Anything computed from other reactive values. Always prefer over `$state` + `$effect`.

```svelte
<script lang="ts">
  let items = $state<Item[]>([]);
  let searchQuery = $state("");

  // Simple expression
  let count = $derived(items.length);

  // Multi-line computation
  let filtered = $derived.by(() => {
    if (!searchQuery) return items;
    const q = searchQuery.toLowerCase();
    return items.filter(i => i.name.toLowerCase().includes(q));
  });
</script>
```

**Anti-pattern:** Using `$effect` to set derived state:

```svelte
// WRONG — never do this
let filtered = $state([]);
$effect(() => {
  filtered = items.filter(i => i.active);  // Use $derived instead
});
```

### 3. Store (Factory + Runes)

**Use for:** State shared across multiple components. This is the established pattern in this codebase.

```typescript
// src/lib/stores/myStore.svelte.ts
import { SvelteMap } from "svelte/reactivity";

function createMyStore() {
  let items = $state<Item[]>([]);
  let selectedId = $state<string | null>(null);
  let itemMap = new SvelteMap<string, Item>();

  let selected = $derived(items.find((i) => i.id === selectedId) ?? null);

  return {
    get items() {
      return items;
    },
    get selected() {
      return selected;
    },
    get itemMap() {
      return itemMap;
    },

    setItems(newItems: Item[]) {
      items = newItems;
    },
    select(id: string) {
      selectedId = id;
    },

    addItem(item: Item) {
      items = [...items, item];
      itemMap.set(item.id, item);
    },
  };
}

export const myStore = createMyStore();
```

**Key rules:**

- File must be `.svelte.ts` (runes require this)
- Use `SvelteMap`/`SvelteSet` from `svelte/reactivity` for reactive collections
- Expose state via getters (read-only from outside)
- Expose mutations via methods (controlled writes)

### 4. SvelteKit Load Function

**Use for:** Data fetched from the backend that initializes a page.

```typescript
// src/routes/flows/+page.ts
import type { PageLoad } from "./$types";

export const load: PageLoad = async ({ fetch }) => {
  const response = await fetch("/flowbuilder/flows");
  const flows = await response.json();
  return { flows };
};
```

```svelte
<!-- src/routes/flows/+page.svelte -->
<script lang="ts">
  let { data } = $props();
  // data.flows is available, typed from load function
</script>
```

**Key rules:**

- Use `fetch` from the load function (not global `fetch`) — SvelteKit handles SSR transforms
- For server-only data, use `+page.server.ts`
- `src/hooks.server.ts` transforms fetch URLs for SSR

### 5. URL State

**Use for:** State that should survive page refresh or be shareable via URL.

```svelte
<script lang="ts">
  import { page } from '$app/stores';
  import { goto } from '$app/navigation';

  let activeTab = $derived($page.url.searchParams.get('tab') ?? 'overview');

  function setTab(tab: string) {
    const url = new URL($page.url);
    url.searchParams.set('tab', tab);
    goto(url.toString(), { replaceState: true });
  }
</script>
```

## Common Combinations

| Scenario                     | Pattern                                      |
| ---------------------------- | -------------------------------------------- |
| Flow builder graph state     | Store (shared across panel, canvas, toolbar) |
| Modal open/closed            | Component-local `$state`                     |
| Filtered list in a component | `$derived` from store + local search query   |
| Component types from backend | SvelteKit load → store on first use          |
| Active flow tab              | URL state (survives refresh)                 |
| Runtime health data          | Store with periodic fetch via `$effect`      |
| Computed node positions      | `$derived.by` from graph store data          |

## Migration from Svelte 4

| Svelte 4                   | Svelte 5                                     |
| -------------------------- | -------------------------------------------- |
| `writable(value)`          | `$state(value)` in factory function          |
| `derived(store, fn)`       | `$derived(fn)` or `$derived.by(() => {...})` |
| `$store` auto-subscription | Use getter: `store.value`                    |
| `store.subscribe(fn)`      | `$effect(() => { /* read store.value */ })`  |
| `store.set(value)`         | `store.setValue(value)` method               |

## Decision Checklist

- [ ] State location chosen (local, derived, store, load, URL)
- [ ] No `$effect` used for derived state
- [ ] Store file uses `.svelte.ts` extension
- [ ] Reactive collections use `SvelteMap`/`SvelteSet`
- [ ] State exposed via getters (not raw `$state` variables)
- [ ] SSR considered (no `window`/`document` at module level)
