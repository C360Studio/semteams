<script lang="ts">
  import { resolve } from "$app/paths";
  import { page } from "$app/state";
  import GlobalStatus from "./GlobalStatus.svelte";
</script>

<header class="top-nav" data-testid="top-nav">
  <a class="app-name" href={resolve("/")} data-testid="brand-home">semteams</a>
  <nav aria-label="Primary">
    <a
      class="nav-link"
      href={resolve("/work")}
      data-testid="nav-work"
      aria-current={page.route.id === "/work" ? "page" : undefined}
    >
      Work
    </a>
  </nav>
  <GlobalStatus />
</header>

<style>
  .top-nav {
    display: flex;
    align-items: center;
    gap: 1.5rem;
    padding: 0 1rem;
    height: 3rem;
    border-bottom: 1px solid var(--ui-border-subtle, #e5e7eb);
    background: var(--ui-surface-primary, #fff);
    flex-shrink: 0;
  }

  .app-name {
    font-size: 1rem;
    font-weight: 700;
    color: var(--ui-text-primary, #111827);
    letter-spacing: -0.01em;
    text-decoration: none;
  }

  .app-name:hover {
    color: var(--ui-interactive-primary, #3b82f6);
  }

  .nav-link {
    font-size: 0.875rem;
    font-weight: 600;
    color: var(--ui-text-secondary, #6b7280);
    text-decoration: none;
  }

  .nav-link:hover,
  .nav-link[aria-current="page"] {
    color: var(--ui-interactive-primary, #3b82f6);
  }

  .nav-link[aria-current="page"] {
    text-decoration: underline;
    text-underline-offset: 0.25em;
  }
</style>
