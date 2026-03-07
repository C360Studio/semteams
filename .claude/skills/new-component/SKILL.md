---
name: new-component
description: Step-by-step checklist for adding a new Svelte 5 component with proper patterns, tests, and TypeScript types. Use when creating new UI components.
argument-hint: [ComponentName]
---

# New Component Checklist

## What component are you adding?

$ARGUMENTS

## Step 1: Define the Props Interface

```typescript
// src/lib/components/ComponentName.svelte
<script lang="ts">
  interface Props {
    requiredProp: string;
    optionalProp?: number;
    onAction?: (value: string) => void;
  }

  let { requiredProp, optionalProp = 0, onAction }: Props = $props();
</script>
```

Key rules:

- Use `interface Props` (not `type`)
- Default values for optional props
- Event handlers as callback props (`onAction`, not `createEventDispatcher`)
- Use `$bindable()` only when parent needs two-way binding

## Step 2: Add Reactive State

```svelte
<script lang="ts">
  // Local UI state
  let isOpen = $state(false);

  // Computed values — ALWAYS use $derived, never $state + $effect
  let displayValue = $derived(formatValue(requiredProp));

  // Complex computation
  let filtered = $derived.by(() => {
    // multi-line logic here
    return items.filter(i => i.active);
  });
</script>
```

Decision guide:

- `$state` — mutable local UI state (open/closed, selected, input value)
- `$derived` — anything computed from props or other state
- `$effect` — side effects only (DOM manipulation, fetch, logging). Last resort.

## Step 3: Write the Template

```svelte
<!-- Use semantic HTML -->
<section class="component-name">
  <h3>{displayValue}</h3>

  <!-- Events: use onclick, not on:click -->
  <button onclick={() => isOpen = !isOpen}>
    Toggle
  </button>

  <!-- Conditional rendering -->
  {#if isOpen}
    <div class="content">
      {#each items as item (item.id)}
        <p>{item.name}</p>
      {/each}
    </div>
  {/if}
</section>
```

Key rules:

- Always use `(item.id)` key in `{#each}` blocks
- Use `onclick` not `on:click` (Svelte 5)
- Semantic HTML elements over generic `<div>`
- Add `data-testid` attributes for testing

## Step 4: Write Tests First

```typescript
// src/lib/components/ComponentName.test.ts
import { render, screen } from "@testing-library/svelte";
import { expect, test, describe } from "vitest";
import userEvent from "@testing-library/user-event";
import ComponentName from "./ComponentName.svelte";

describe("ComponentName", () => {
  test("renders with required props", () => {
    render(ComponentName, { props: { requiredProp: "test" } });
    expect(screen.getByText("test")).toBeInTheDocument();
  });

  test("handles user interaction", async () => {
    const user = userEvent.setup();
    const handler = vi.fn();
    render(ComponentName, {
      props: { requiredProp: "test", onAction: handler },
    });

    await user.click(screen.getByRole("button"));
    expect(handler).toHaveBeenCalledOnce();
  });

  test("handles empty state", () => {
    render(ComponentName, { props: { requiredProp: "" } });
    expect(screen.getByText("No data")).toBeInTheDocument();
  });
});
```

## Step 5: Style with pico.css

This project uses pico.css. Prefer pico's semantic styling over custom CSS:

```svelte
<!-- pico.css styles semantic elements automatically -->
<article>
  <header>Title</header>
  <p>Content</p>
  <footer>
    <button>Action</button>
    <button class="secondary">Cancel</button>
  </footer>
</article>
```

Add component-specific styles only when pico doesn't cover it:

```svelte
<style>
  .component-name {
    /* minimal custom styles */
  }
</style>
```

## Verification Checklist

- [ ] Props interface defined with TypeScript
- [ ] Default values for optional props
- [ ] `$derived` used for computed values (not `$state` + `$effect`)
- [ ] `$effect` only for true side effects (with cleanup if needed)
- [ ] Events use `onclick` pattern (Svelte 5)
- [ ] Template uses semantic HTML
- [ ] `{#each}` blocks have `(key)` expressions
- [ ] Tests cover: rendering, interaction, empty/error states
- [ ] No `any` types
- [ ] Accessibility: ARIA labels on non-semantic interactive elements

## File Naming

| Type        | Pattern                     | Example                   |
| ----------- | --------------------------- | ------------------------- |
| Component   | `PascalCase.svelte`         | `FlowNode.svelte`         |
| Test        | `PascalCase.test.ts`        | `FlowNode.test.ts`        |
| Attack test | `PascalCase.attack.test.ts` | `FlowNode.attack.test.ts` |
