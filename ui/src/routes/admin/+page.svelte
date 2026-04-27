<script lang="ts">
  import { resolve } from "$app/paths";

  // Admin landing. Per docs/proposals/ui-redesign.md, the user surface
  // is delegate-and-watch; admin is for the rare config user (operator,
  // builder). Landing layout is a card grid — each section either
  // exists today or is a stub with a one-line note about what'll go
  // there.
  type AdminCard = {
    href: string;
    title: string;
    description: string;
    available: boolean;
  };

  const cards: AdminCard[] = [
    {
      href: "/admin/flows",
      title: "Flows",
      description:
        "Visual flow builder. Power-user surface — most flows are static configs in /configs.",
      available: true,
    },
    {
      href: "/admin/personas",
      title: "Personas",
      description:
        "Per-role prompt fragments and tool allowlists. Backed by NATS KV via the persona-CRUD tools.",
      available: false,
    },
    {
      href: "/admin/endpoints",
      title: "Model endpoints",
      description:
        "LLM endpoint registry + circuit-breaker state (beta.15). Tune, watch, and override health policy.",
      available: false,
    },
    {
      href: "/admin/tools",
      title: "Tools & governance",
      description:
        "Tool allowlist, approval-required list, category filters, governance rules.",
      available: false,
    },
  ];
</script>

<svelte:head>
  <title>Admin - SemTeams</title>
</svelte:head>

<div class="admin-page" data-testid="admin-page">
  <header class="admin-header">
    <h1 class="admin-title">Admin</h1>
    <p class="admin-subtitle">
      Configuration surfaces for personas, endpoints, tools, and flows.
      Most users never need this — agents handle most config decisions.
    </p>
  </header>

  <div class="card-grid">
    {#each cards as card (card.href)}
      {#if card.available}
        <!-- eslint-disable svelte/no-navigation-without-resolve -->
        <a
          href={resolve(card.href as "/admin/flows")}
          class="admin-card"
          data-testid="admin-card-{card.href.split('/').pop()}"
        >
          <h2 class="card-title">{card.title}</h2>
          <p class="card-description">{card.description}</p>
          <span class="card-arrow" aria-hidden="true">→</span>
        </a>
      {:else}
        <div
          class="admin-card placeholder"
          data-testid="admin-card-{card.href.split('/').pop()}"
          aria-disabled="true"
        >
          <h2 class="card-title">{card.title}</h2>
          <p class="card-description">{card.description}</p>
          <span class="card-tag">Coming soon</span>
        </div>
      {/if}
    {/each}
  </div>
</div>

<style>
  .admin-page {
    width: 100%;
    height: 100%;
    overflow-y: auto;
    padding: 2rem;
    box-sizing: border-box;
  }

  .admin-header {
    max-width: 56rem;
    margin: 0 auto 2rem;
  }

  .admin-title {
    margin: 0 0 0.5rem;
    font-size: 1.5rem;
    font-weight: 700;
    color: var(--ui-text-primary, #111827);
  }

  .admin-subtitle {
    margin: 0;
    font-size: 0.875rem;
    color: var(--ui-text-secondary, #6b7280);
    max-width: 48ch;
  }

  .card-grid {
    max-width: 56rem;
    margin: 0 auto;
    display: grid;
    grid-template-columns: repeat(auto-fill, minmax(18rem, 1fr));
    gap: 1rem;
  }

  .admin-card {
    display: flex;
    flex-direction: column;
    gap: 0.5rem;
    padding: 1rem 1.125rem;
    background: var(--ui-surface-secondary, #f7f7f7);
    border: 1px solid var(--ui-border-subtle, #e5e7eb);
    border-radius: 10px;
    text-decoration: none;
    color: inherit;
    position: relative;
    transition: border-color 0.15s, transform 0.15s;
  }

  a.admin-card:hover {
    border-color: var(--ui-interactive-primary, #3b82f6);
    transform: translateY(-1px);
  }

  a.admin-card:focus-visible {
    outline: 2px solid var(--ui-interactive-primary, #3b82f6);
    outline-offset: 2px;
  }

  .admin-card.placeholder {
    opacity: 0.7;
    cursor: not-allowed;
  }

  .card-title {
    margin: 0;
    font-size: 1rem;
    font-weight: 600;
    color: var(--ui-text-primary, #111827);
  }

  .card-description {
    margin: 0;
    font-size: 0.8125rem;
    color: var(--ui-text-secondary, #6b7280);
    line-height: 1.5;
  }

  .card-arrow {
    position: absolute;
    top: 1rem;
    right: 1.125rem;
    font-size: 1rem;
    color: var(--ui-text-tertiary, #9ca3af);
    transition: color 0.15s;
  }

  a.admin-card:hover .card-arrow {
    color: var(--ui-interactive-primary, #3b82f6);
  }

  .card-tag {
    align-self: flex-start;
    font-size: 0.6875rem;
    font-weight: 600;
    text-transform: uppercase;
    letter-spacing: 0.05em;
    color: var(--ui-text-tertiary, #9ca3af);
    background: var(--ui-surface-tertiary, #e5e7eb);
    padding: 0.125rem 0.5rem;
    border-radius: 9999px;
    margin-top: 0.25rem;
  }
</style>
