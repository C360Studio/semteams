# Contributing to SemStreams UI

Thank you for your interest in contributing to SemStreams UI! This document provides guidelines for contributing to the project.

## Getting Started

1. **Fork the repository** and clone your fork
2. **Install dependencies**: `npm install`
3. **Set up your backend**: See [INTEGRATION_EXAMPLE.md](./INTEGRATION_EXAMPLE.md)
4. **Configure environment**: Copy `.env.example` to `.env` and set `OPENAPI_SPEC_PATH`
5. **Generate types**: `task generate-types`
6. **Start development**: `npm run dev`

## Development Workflow

### 1. Create a Branch

```bash
git checkout -b feature/your-feature-name
# or
git checkout -b fix/bug-description
```

### 2. Make Your Changes

Keep changes focused and related to a single issue or feature.

**Key Principles:**

- **Backend-agnostic**: The UI should work with ANY SemStreams-based backend
- **Runtime discovery**: Discover capabilities from the backend at runtime, not compile-time
- **No hardcoded backends**: Never hardcode references to specific backends (semstreams, semmem, etc.)
- **Type safety**: Use generated TypeScript types from OpenAPI specs
- **Accessibility**: Ensure WCAG AA compliance for all UI components

### 3. Write Tests

```bash
# Unit tests
npm run test

# E2E tests (requires backend)
BACKEND_CONTEXT=/path/to/backend task test:e2e

# With UI mode
npm run test:e2e:ui
```

### 4. Run Quality Checks

```bash
# Linting
task lint

# Type checking
task check

# All checks
task ci
```

### 5. Commit Your Changes

Use conventional commit format:

```
<type>(<scope>): <subject>

<body>

<footer>
```

**Types:**

- `feat`: New feature
- `fix`: Bug fix
- `docs`: Documentation changes
- `style`: Code style changes (formatting, etc.)
- `refactor`: Code refactoring
- `test`: Adding or updating tests
- `chore`: Maintenance tasks

**Examples:**

```
feat(palette): add drag-and-drop for component palette

Implements draggable components from the palette to the canvas
using @xyflow/svelte drag handlers.

Closes #42
```

```
fix(validation): handle missing schema gracefully

Previously crashed when component schema was undefined.
Now shows helpful error message instead.
```

### 6. Submit a Pull Request

1. Push your branch to your fork
2. Create a pull request against the `main` branch
3. Fill out the pull request template
4. Wait for review and address feedback

## Code Style Guidelines

### TypeScript/JavaScript

- Use TypeScript for all new code
- Prefer `const` over `let`, avoid `var`
- Use meaningful variable names
- Add JSDoc comments for public APIs
- Use type imports: `import type { Foo } from './types'`

### Svelte 5

Follow Svelte 5 best practices:

```svelte
<script>
  import { getDomainColor } from '$lib/utils/domain-colors';

  // Props with proper typing
  let { component, onUpdate = $bindable() } = $props();

  // Reactive state
  let isActive = $state(false);

  // Derived values
  let color = $derived(getDomainColor(component.domain));

  // Effects with cleanup
  $effect(() => {
    const cleanup = subscribe();
    return () => cleanup();
  });
</script>
```

### CSS and Styling

- Use CSS custom properties from the design system
- Never hardcode colors - use `var(--ui-*)`, `var(--status-*)`, or `var(--domain-*)`
- Ensure WCAG AA contrast compliance (4.5:1 for text)
- Mobile-first responsive design

**Example:**

```css
.error-message {
  color: var(--status-error);
  background: var(--status-error-container);
  border: 1px solid var(--ui-border-emphasis);
}
```

## Testing Guidelines

### Unit Tests (Vitest)

Test component behavior, not implementation:

```typescript
import { render, screen } from "@testing-library/svelte";
import { expect, test } from "vitest";
import ComponentCard from "./ComponentCard.svelte";

test("displays component name and description", () => {
  const component = {
    name: "UDP Source",
    description: "Receives UDP packets",
  };

  render(ComponentCard, { props: { component } });

  expect(screen.getByText("UDP Source")).toBeInTheDocument();
  expect(screen.getByText("Receives UDP packets")).toBeInTheDocument();
});
```

### E2E Tests (Playwright)

Test real user workflows:

```typescript
import { test, expect } from "@playwright/test";

test("create and save flow", async ({ page }) => {
  await page.goto("/");

  // Drag component from palette
  await page.dragAndDrop(
    '[data-testid="component-udp"]',
    '[data-testid="canvas"]',
  );

  // Save flow
  await page.click('[data-testid="save-flow"]');

  // Verify saved
  await expect(page.getByText("Flow saved successfully")).toBeVisible();
});
```

## Documentation

### Code Comments

Focus on "why", not "what":

```typescript
// ✅ Good: Explains reasoning
// Use event properties instead of createEventDispatcher for Svelte 5
let { onclick } = $props();

// ❌ Bad: States the obvious
// Set the color variable
const color = "red";
```

### Markdown Documentation

- Use single `#` for title
- No heading level skipping
- Code blocks with language specification
- Line length under 120 characters
- Examples with input/output

## Backend Integration

When testing with different backends:

```bash
# Generate types from your backend
OPENAPI_SPEC_PATH=/path/to/backend/specs/openapi.v3.yaml task generate-types

# Test E2E
BACKEND_CONTEXT=/path/to/backend task test:e2e
```

Ensure your changes work with:

- Different component types
- Various schema structures
- Different protocol domains

## Accessibility

All UI components must:

- Support keyboard navigation
- Have proper ARIA labels
- Maintain 4.5:1 contrast ratio
- Work with screen readers
- Not rely solely on color for information

Test with:

```bash
# Run accessibility checks
npm run test -- --grep accessibility
```

## Getting Help

- **Issues**: Open an issue for bugs or feature requests
- **Discussions**: Use GitHub Discussions for questions
- **Documentation**: Check [INTEGRATION_EXAMPLE.md](./INTEGRATION_EXAMPLE.md) and [E2E_SETUP.md](./E2E_SETUP.md)

## Review Process

Pull requests are reviewed for:

1. **Functionality**: Does it work as intended?
2. **Tests**: Are there adequate tests?
3. **Backend-agnostic**: Does it avoid hardcoding specific backends?
4. **Code quality**: Is the code clean and maintainable?
5. **Documentation**: Are changes documented?
6. **Accessibility**: Does it meet WCAG AA standards?

## License

By contributing, you agree that your contributions will be licensed under the same license as the project (see [LICENSE](./LICENSE)).

## Code of Conduct

Please be respectful and constructive in all interactions. We aim to foster an inclusive and welcoming community.

---

Thank you for contributing to SemStreams UI!
