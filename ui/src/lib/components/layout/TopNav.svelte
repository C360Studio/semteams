<script lang="ts">
  import { resolve } from "$app/paths";
</script>

<header class="top-nav" data-testid="top-nav">
  <a class="app-name" href={resolve("/")} data-testid="brand-home">semteams</a>
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
</style>
