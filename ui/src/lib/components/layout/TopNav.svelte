<script lang="ts">
  import { page } from "$app/stores";

  const tabs = [
    { href: "/", label: "Board" },
    { href: "/graph", label: "Graph" },
    { href: "/flows", label: "Flows" },
  ] as const;

  let currentPath = $derived($page.url.pathname);

  function isActive(href: string): boolean {
    if (href === "/") return currentPath === "/";
    return currentPath.startsWith(href);
  }
</script>

<header class="top-nav" data-testid="top-nav">
  <span class="app-name">semteams</span>
  <nav class="tab-bar" data-testid="tab-bar">
    <!-- eslint-disable svelte/no-navigation-without-resolve — static internal routes -->
    {#each tabs as tab (tab.href)}
      <a
        href={tab.href}
        class="tab"
        class:active={isActive(tab.href)}
        aria-current={isActive(tab.href) ? 'page' : undefined}
        data-testid="tab-{tab.label.toLowerCase()}"
      >
        {tab.label}
      </a>
    {/each}
  </nav>
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
  }

  .tab-bar {
    display: flex;
    gap: 0;
    height: 100%;
  }

  .tab {
    display: flex;
    align-items: center;
    padding: 0 0.875rem;
    height: 100%;
    font-size: 0.8125rem;
    font-weight: 500;
    color: var(--ui-text-secondary, #6b7280);
    text-decoration: none;
    border-bottom: 2px solid transparent;
    transition: color 0.15s, border-color 0.15s;
  }

  .tab:hover {
    color: var(--ui-text-primary, #111827);
  }

  .tab.active {
    color: var(--ui-interactive-primary, #3b82f6);
    border-bottom-color: var(--ui-interactive-primary, #3b82f6);
  }
</style>
