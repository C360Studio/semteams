<script lang="ts">
  import type { EntityDetailAttachment } from "$lib/types/chat";

  interface Props {
    detail: EntityDetailAttachment;
    onViewEntity?: (entityId: string) => void;
  }

  let { detail, onViewEntity }: Props = $props();

  // entity is guaranteed by Phase 4 callers; guard for type safety
  let entity = $derived(detail.entity!);

  function handleEntityClick() {
    onViewEntity?.(entity.id);
  }
</script>

<div data-testid="entity-detail-card">
  {#if entity}
    <div class="entity-header">
      <button
        data-testid="entity-detail-entity-id"
        class="entity-id-btn"
        onclick={handleEntityClick}
      >
        {entity.label}
      </button>
      <span class="entity-type">{entity.type}</span>
      <span class="entity-domain">{entity.domain}</span>
    </div>

    {#if entity.properties.length > 0}
      <div class="entity-properties">
        <strong>Properties</strong>
        <dl>
          {#each entity.properties as prop (prop.predicate)}
            <div class="property-row">
              <dt class="property-key">{prop.predicate}</dt>
              <dd class="property-value">{String(prop.value)}</dd>
            </div>
          {/each}
        </dl>
      </div>
    {/if}

    {#if entity.relationships.length > 0}
      <div class="entity-relationships">
        <strong>Relationships ({entity.relationships.length})</strong>
        <ul>
          {#each entity.relationships as rel (rel.predicate + rel.targetId)}
            <li>
              <span class="rel-predicate">{rel.predicate}</span>
              <span class="rel-target">{rel.targetId}</span>
            </li>
          {/each}
        </ul>
      </div>
    {:else}
      <div class="no-relationships">0 relationships</div>
    {/if}
  {/if}
</div>

<style>
  .entity-header {
    display: flex;
    gap: 0.5rem;
    align-items: center;
    margin-bottom: 0.5rem;
  }

  .entity-id-btn {
    background: none;
    border: none;
    padding: 0;
    cursor: pointer;
    font-weight: 600;
    color: var(--pico-primary, #0d6efd);
    text-decoration: underline;
  }

  .entity-id-btn:hover {
    opacity: 0.8;
  }

  .entity-type,
  .entity-domain {
    font-size: 0.85em;
    color: var(--pico-muted-color, #6c757d);
  }

  .entity-properties {
    margin-bottom: 0.5rem;
  }

  dl {
    margin: 0;
    padding: 0;
  }

  .property-row {
    display: flex;
    gap: 0.5rem;
  }

  dt {
    font-weight: 500;
    min-width: 12ch;
  }

  dd {
    margin: 0;
    color: var(--pico-muted-color, #6c757d);
  }

  .entity-relationships ul {
    list-style: none;
    padding: 0;
    margin: 0;
  }

  .rel-predicate {
    font-weight: 500;
    margin-right: 0.25rem;
  }

  .rel-target {
    font-size: 0.85em;
    color: var(--pico-muted-color, #6c757d);
  }

  .no-relationships {
    font-size: 0.85em;
    color: var(--pico-muted-color, #6c757d);
  }
</style>
