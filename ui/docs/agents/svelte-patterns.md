# Svelte Development Patterns

Reference document for all agents. Contains testing patterns, code standards, and review checklists for Svelte 5 / TypeScript / SvelteKit development.

## Technology Stack

| Tool                    | Purpose              | Command            |
| ----------------------- | -------------------- | ------------------ |
| Vitest                  | Unit/component tests | `npm run test`     |
| @testing-library/svelte | Component rendering  | (via Vitest)       |
| Playwright              | E2E tests            | `npm run test:e2e` |
| ESLint                  | Linting              | `npm run lint`     |
| Prettier                | Formatting           | `npm run format`   |
| svelte-check            | Type checking        | `npm run check`    |
| TypeScript              | Type safety          | (via svelte-check) |

## Component Test Patterns

### Basic Component Test

```typescript
import { render, screen } from "@testing-library/svelte";
import { expect, test, describe } from "vitest";
import MyComponent from "./MyComponent.svelte";

describe("MyComponent", () => {
  test("renders with default props", () => {
    render(MyComponent);
    expect(screen.getByRole("button")).toBeInTheDocument();
  });

  test("renders with custom props", () => {
    render(MyComponent, { props: { label: "Click me" } });
    expect(screen.getByText("Click me")).toBeInTheDocument();
  });
});
```

### Testing User Interactions

```typescript
import { render, screen } from "@testing-library/svelte";
import { expect, test } from "vitest";
import userEvent from "@testing-library/user-event";
import Counter from "./Counter.svelte";

test("increments count on click", async () => {
  const user = userEvent.setup();
  render(Counter);

  const button = screen.getByRole("button", { name: /increment/i });
  await user.click(button);

  expect(screen.getByText("Count: 1")).toBeInTheDocument();
});
```

### Testing Svelte 5 Runes

```typescript
import { render, screen, waitFor } from "@testing-library/svelte";
import { expect, test } from "vitest";
import ReactiveComponent from "./ReactiveComponent.svelte";

test("$derived updates when dependencies change", async () => {
  const { component } = render(ReactiveComponent, {
    props: { baseValue: 5 },
  });

  expect(screen.getByTestId("derived")).toHaveTextContent("10"); // 5 * 2

  // Update prop to trigger $derived recalculation
  await component.$set({ baseValue: 10 });

  await waitFor(() => {
    expect(screen.getByTestId("derived")).toHaveTextContent("20");
  });
});
```

### Testing Async Components

```typescript
import { render, screen, waitFor } from "@testing-library/svelte";
import { expect, test, vi } from "vitest";
import AsyncComponent from "./AsyncComponent.svelte";

test("shows loading state then data", async () => {
  const mockFetch = vi.fn().mockResolvedValue({ data: "test" });

  render(AsyncComponent, { props: { fetchFn: mockFetch } });

  // Initial loading state
  expect(screen.getByText("Loading...")).toBeInTheDocument();

  // Wait for data
  await waitFor(() => {
    expect(screen.getByText("test")).toBeInTheDocument();
  });
});
```

### Testing Error States

```typescript
import { render, screen, waitFor } from "@testing-library/svelte";
import { expect, test, vi } from "vitest";
import AsyncComponent from "./AsyncComponent.svelte";

test("displays error message on fetch failure", async () => {
  const mockFetch = vi.fn().mockRejectedValue(new Error("Network error"));

  render(AsyncComponent, { props: { fetchFn: mockFetch } });

  await waitFor(() => {
    expect(screen.getByRole("alert")).toHaveTextContent("Network error");
  });
});
```

### Table-Driven Tests

```typescript
import { render, screen } from "@testing-library/svelte";
import { expect, test, describe } from "vitest";
import StatusBadge from "./StatusBadge.svelte";

describe("StatusBadge", () => {
  const testCases = [
    {
      status: "success",
      expectedClass: "badge-success",
      expectedText: "Success",
    },
    { status: "error", expectedClass: "badge-error", expectedText: "Error" },
    {
      status: "pending",
      expectedClass: "badge-pending",
      expectedText: "Pending",
    },
  ];

  test.each(testCases)(
    "renders $status status correctly",
    ({ status, expectedClass, expectedText }) => {
      render(StatusBadge, { props: { status } });

      const badge = screen.getByTestId("status-badge");
      expect(badge).toHaveClass(expectedClass);
      expect(badge).toHaveTextContent(expectedText);
    },
  );
});
```

## E2E Test Patterns (Playwright)

### Basic Page Test

```typescript
import { test, expect } from "@playwright/test";

test("homepage loads correctly", async ({ page }) => {
  await page.goto("/");

  await expect(page).toHaveTitle(/My App/);
  await expect(page.locator("h1")).toBeVisible();
});
```

### Form Interaction

```typescript
import { test, expect } from "@playwright/test";

test("user can submit contact form", async ({ page }) => {
  await page.goto("/contact");

  await page.fill('[data-testid="name"]', "John Doe");
  await page.fill('[data-testid="email"]', "john@example.com");
  await page.fill('[data-testid="message"]', "Hello!");

  await page.click('[data-testid="submit"]');

  await expect(page.locator('[data-testid="success-message"]')).toBeVisible();
});
```

### Testing Navigation

```typescript
import { test, expect } from "@playwright/test";

test("navigation works correctly", async ({ page }) => {
  await page.goto("/");

  await page.click('a[href="/about"]');
  await expect(page).toHaveURL("/about");

  await page.click('a[href="/"]');
  await expect(page).toHaveURL("/");
});
```

### Waiting for API Responses

```typescript
import { test, expect } from "@playwright/test";

test("displays data after API call", async ({ page }) => {
  await page.goto("/dashboard");

  // Wait for specific API response
  await page.waitForResponse(
    (resp) => resp.url().includes("/api/data") && resp.status() === 200,
  );

  await expect(page.locator('[data-testid="data-table"]')).toBeVisible();
});
```

## Attack Test Patterns

### Undefined/Null Props

```typescript
import { render } from "@testing-library/svelte";
import { expect, test, describe } from "vitest";
import MyComponent from "./MyComponent.svelte";

describe("MyComponent Attack Tests", () => {
  test("handles undefined props gracefully", () => {
    // @ts-expect-error - intentional attack test
    expect(() =>
      render(MyComponent, { props: { data: undefined } }),
    ).not.toThrow();
  });

  test("handles null props gracefully", () => {
    // @ts-expect-error - intentional attack test
    expect(() => render(MyComponent, { props: { data: null } })).not.toThrow();
  });

  test("handles empty object props", () => {
    // @ts-expect-error - intentional attack test
    expect(() => render(MyComponent, { props: {} })).not.toThrow();
  });
});
```

### Effect Cleanup

```typescript
import { render } from "@testing-library/svelte";
import { expect, test, vi } from "vitest";
import EffectComponent from "./EffectComponent.svelte";

test("cleans up effects on unmount", () => {
  const cleanupSpy = vi.fn();
  const { unmount } = render(EffectComponent, {
    props: { onCleanup: cleanupSpy },
  });

  unmount();

  expect(cleanupSpy).toHaveBeenCalled();
});

test("cleans up intervals on unmount", async () => {
  vi.useFakeTimers();
  const callback = vi.fn();

  const { unmount } = render(EffectComponent, {
    props: { intervalCallback: callback },
  });

  // Advance timers to trigger interval
  vi.advanceTimersByTime(1000);
  expect(callback).toHaveBeenCalledTimes(1);

  // Unmount and verify interval stops
  unmount();
  vi.advanceTimersByTime(5000);
  expect(callback).toHaveBeenCalledTimes(1); // Should not increase

  vi.useRealTimers();
});
```

### Rapid State Changes

```typescript
import { render, screen, waitFor } from "@testing-library/svelte";
import { expect, test } from "vitest";
import userEvent from "@testing-library/user-event";
import RapidComponent from "./RapidComponent.svelte";

test("handles rapid state updates without race conditions", async () => {
  const user = userEvent.setup();
  render(RapidComponent);

  const button = screen.getByRole("button");

  // Rapid clicks
  await Promise.all([
    user.click(button),
    user.click(button),
    user.click(button),
    user.click(button),
    user.click(button),
  ]);

  await waitFor(() => {
    expect(screen.getByTestId("count")).toHaveTextContent("5");
  });
});
```

### Empty/Large Data

```typescript

import { render, screen } from "@testing-library/svelte";
import { expect, test, describe } from "vitest";
import DataList from "./DataList.svelte";

describe("DataList boundary conditions", () => {
  test("renders empty state for empty array", () => {
    render(DataList, { props: { items: [] } });
    expect(screen.getByText("No items")).toBeInTheDocument();
  });

  test("handles large dataset without crashing", () => {
    const largeData = Array.from({ length: 10000 }, (_, i) => ({
      id: i,
      name: `Item ${i}`,
    }));

    expect(() =>
      render(DataList, { props: { items: largeData } }),
    ).not.toThrow();
  });

  test("handles deeply nested objects", () => {
    const deeplyNested = {
      level1: {
        level2: {
          level3: {
            level4: {
              value: "deep",
            },
          },
        },
      },
    };

    expect(() =>
      render(DataList, {
        props: { data: deeplyNested },
      }),
    ).not.toThrow();
  });
});
```

### Accessibility Attacks

```typescript

import { render, screen } from "@testing-library/svelte";
import { expect, test, describe } from "vitest";
import { axe, toHaveNoViolations } from "jest-axe";
import InteractiveComponent from "./InteractiveComponent.svelte";

expect.extend(toHaveNoViolations);

describe("InteractiveComponent accessibility", () => {
  test("has no accessibility violations", async () => {
    const { container } = render(InteractiveComponent);
    const results = await axe(container);
    expect(results).toHaveNoViolations();
  });

  test("is keyboard navigable", async () => {
    render(InteractiveComponent);

    const button = screen.getByRole("button");
    button.focus();

    expect(document.activeElement).toBe(button);
  });

  test("has proper ARIA labels", () => {
    render(InteractiveComponent);

    expect(screen.getByRole("button")).toHaveAccessibleName();
    expect(screen.getByRole("dialog")).toHaveAttribute("aria-labelledby");
  });
});
```

## Svelte 5 Runes Reference

### $state - Reactive State

```svelte
<script lang="ts">
  let count = $state(0);
  let user = $state<User | null>(null);

  // Object state (deep reactivity)
  let settings = $state({
    theme: 'light',
    notifications: true
  });
</script>
```

### $derived - Computed Values

```svelte
<script lang="ts">
  let items = $state<Item[]>([]);

  // Simple derived
  let count = $derived(items.length);

  // Complex derived
  let total = $derived(items.reduce((sum, item) => sum + item.price, 0));

  // Derived with formatting
  let displayTotal = $derived(`$${total.toFixed(2)}`);
</script>
```

### $effect - Side Effects

```svelte
<script lang="ts">
  let searchQuery = $state('');

  // Effect with cleanup
  $effect(() => {
    const controller = new AbortController();

    fetch(`/api/search?q=${searchQuery}`, { signal: controller.signal })
      .then(r => r.json())
      .then(data => results = data);

    // Cleanup function
    return () => controller.abort();
  });

  // Effect for DOM manipulation
  $effect(() => {
    document.title = `Search: ${searchQuery}`;
  });
</script>
```

### $props - Component Props

```svelte
<script lang="ts">
  interface Props {
    name: string;
    count?: number;
    onUpdate?: (value: number) => void;
  }

  let { name, count = 0, onUpdate }: Props = $props();
</script>
```

### $bindable - Two-Way Binding

```svelte
<script lang="ts">
  interface Props {
    value: string;
  }

  let { value = $bindable() }: Props = $props();
</script>

<input bind:value />
```

## TypeScript Patterns

### Component Props Interface

```typescript
// src/lib/types/components.ts
export interface ButtonProps {
  variant?: "primary" | "secondary" | "danger";
  size?: "sm" | "md" | "lg";
  disabled?: boolean;
  loading?: boolean;
  onclick?: (event: MouseEvent) => void;
}

export interface ModalProps {
  open: boolean;
  title: string;
  onclose?: () => void;
  children?: import("svelte").Snippet;
}
```

### API Response Types

```typescript
// src/lib/types/api.ts
export interface ApiResponse<T> {
  data: T;
  error: null;
} | {
  data: null;
  error: ApiError;
}

export interface ApiError {
  message: string;
  code: string;
  status: number;
}

export interface PaginatedResponse<T> {
  items: T[];
  total: number;
  page: number;
  pageSize: number;
}
```

### Type-Safe Fetch Wrapper

```typescript
// src/lib/api/client.ts
export async function fetchApi<T>(
  endpoint: string,
  options?: RequestInit,
): Promise<ApiResponse<T>> {
  try {
    const response = await fetch(`/api${endpoint}`, {
      headers: { "Content-Type": "application/json" },
      ...options,
    });

    if (!response.ok) {
      const error = await response.json();
      return { data: null, error };
    }

    const data = await response.json();
    return { data, error: null };
  } catch (e) {
    return {
      data: null,
      error: {
        message: e instanceof Error ? e.message : "Unknown error",
        code: "NETWORK_ERROR",
        status: 0,
      },
    };
  }
}
```

## Code Review Checklist

### Component Review

- [ ] Props have TypeScript interface
- [ ] Default values for optional props
- [ ] Event handlers use `onclick` not `on:click` (Svelte 5)
- [ ] Effects have cleanup functions where needed
- [ ] No memory leaks (subscriptions, intervals cleaned up)
- [ ] Component is accessible (ARIA, keyboard nav)
- [ ] Loading and error states handled
- [ ] Empty states handled

### Reactivity Review

- [ ] `$state` used for mutable values
- [ ] `$derived` used for computed values (not `$state` with `$effect`)
- [ ] `$effect` only for true side effects (DOM, fetch, logging)
- [ ] No derived state stored in `$state`
- [ ] Effects don't create infinite loops

### TypeScript Review

- [ ] No `any` types (use `unknown` if needed)
- [ ] Props interface exported for testing
- [ ] API responses properly typed
- [ ] Null/undefined handled explicitly
- [ ] Type narrowing used appropriately

### Testing Review

- [ ] Happy path tested
- [ ] Error states tested
- [ ] Edge cases tested (empty, null, large data)
- [ ] User interactions tested
- [ ] Accessibility tested
- [ ] Tests are behavior-focused, not implementation-focused

### Security Review

- [ ] User input sanitized before rendering
- [ ] No `{@html}` with user content
- [ ] API calls use proper authentication
- [ ] Sensitive data not logged
- [ ] CSRF protection for mutations

### Performance Review

- [ ] Large lists use virtual scrolling or pagination
- [ ] Images have proper sizing/lazy loading
- [ ] No unnecessary re-renders
- [ ] Heavy computations in `$derived`, not inline
- [ ] Debounce/throttle on frequent events

## File Naming Conventions

| Type              | Pattern                   | Example                   |
| ----------------- | ------------------------- | ------------------------- |
| Component         | PascalCase.svelte         | `UserCard.svelte`         |
| Component test    | PascalCase.test.ts        | `UserCard.test.ts`        |
| Attack test       | PascalCase.attack.test.ts | `UserCard.attack.test.ts` |
| E2E test          | kebab-case.spec.ts        | `user-flow.spec.ts`       |
| TypeScript module | camelCase.ts              | `apiClient.ts`            |
| Type definitions  | camelCase.ts              | `types.ts`                |
| Store/state       | camelCase.svelte.ts       | `userStore.svelte.ts`     |

## Test File Conventions

### Unit/Component Tests

```typescript
// src/lib/components/ComponentName.test.ts
// Standard test file — any changes to existing tests must include
// a comment explaining why the change was needed.
```

### Attack Tests (Reviewer)

```typescript
// src/lib/components/ComponentName.attack.test.ts
// Attack tests by Reviewer Agent.
// These test adversarial inputs and edge cases.
```

### E2E Tests

```typescript
// e2e/feature-name.spec.ts
// Playwright E2E tests for user-facing flows.
```
