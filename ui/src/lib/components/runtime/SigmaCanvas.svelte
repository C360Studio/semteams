<script lang="ts">
  /**
   * SigmaCanvas - WebGL graph renderer using Sigma.js + graphology
   *
   * Drop-in replacement for GraphCanvas. Same props interface.
   * Uses WebGL for 10K+ node rendering performance.
   */

  import { onMount } from "svelte";
  import Graph from "graphology";
  import Sigma from "sigma";
  import type { GraphEntity, GraphRelationship } from "$lib/types/graph";
  import { syncStoreToGraph } from "$lib/utils/graphology-adapter";
  import { LayoutController } from "$lib/utils/sigma-layout";

  interface SigmaCanvasProps {
    entities: GraphEntity[];
    relationships: GraphRelationship[];
    selectedEntityId?: string | null;
    hoveredEntityId?: string | null;
    onEntitySelect?: (entityId: string | null) => void;
    onEntityExpand?: (entityId: string) => void;
    onEntityHover?: (entityId: string | null) => void;
    onRefresh?: () => void;
    loading?: boolean;
  }

  let {
    entities,
    relationships,
    selectedEntityId = null,
    hoveredEntityId = null,
    onEntitySelect,
    onEntityExpand,
    onEntityHover,
    onRefresh,
    loading = false,
  }: SigmaCanvasProps = $props();

  let containerElement: HTMLDivElement;
  let sigma: Sigma | null = null;
  let graph: Graph | null = null;
  let layout: LayoutController | null = null;

  // Track entity IDs to avoid full re-sync when array reference changes but content hasn't
  let lastEntityIds: string = "";
  let lastRelationshipIds: string = "";

  onMount(() => {
    graph = new Graph();
    layout = new LayoutController();

    sigma = new Sigma(graph, containerElement, {
      allowInvalidContainer: true,
      renderEdgeLabels: false,
      defaultEdgeType: "arrow",
      labelRenderedSizeThreshold: 8,
      labelColor: { color: "#f4f4f4" },
      nodeReducer: (node, data) => {
        const res = { ...data };

        if (selectedEntityId && node !== selectedEntityId) {
          const isNeighbor =
            graph!.hasEdge(selectedEntityId, node) ||
            graph!.hasEdge(node, selectedEntityId);
          if (!isNeighbor) {
            res.color = "#525252";
            res.label = "";
          }
        }

        if (node === hoveredEntityId) {
          res.highlighted = true;
        }

        return res;
      },
      edgeReducer: (edge, data) => {
        const res = { ...data };

        if (selectedEntityId) {
          const source = graph!.source(edge);
          const target = graph!.target(edge);
          const isConnected =
            source === selectedEntityId || target === selectedEntityId;
          if (!isConnected) {
            res.color = "#393939";
          }
        }

        return res;
      },
    });

    // Event handlers
    sigma.on("clickNode", ({ node }) => {
      onEntitySelect?.(node === selectedEntityId ? null : node);
    });

    sigma.on("doubleClickNode", ({ node, event }) => {
      event.original.preventDefault();
      onEntityExpand?.(node);
    });

    sigma.on("enterNode", ({ node }) => {
      onEntityHover?.(node);
    });

    sigma.on("leaveNode", () => {
      onEntityHover?.(null);
    });

    sigma.on("clickStage", () => {
      onEntitySelect?.(null);
    });

    // Initial sync is handled by the $effect below — no need to duplicate here.

    return () => {
      layout?.stop();
      sigma?.kill();
      sigma = null;
      graph = null;
      layout = null;
    };
  });

  // Sync when data actually changes (not just array reference from $derived)
  $effect(() => {
    if (!graph || !sigma || !layout) return;
    if (entities.length === 0 && graph.order === 0) return;

    // Cheap content fingerprint to avoid re-sync on identical data
    const entityIds = entities.map((e) => e.id).join(",");
    const relIds = relationships.map((r) => r.id).join(",");
    if (entityIds === lastEntityIds && relIds === lastRelationshipIds) return;
    lastEntityIds = entityIds;
    lastRelationshipIds = relIds;

    syncStoreToGraph(graph, entities, relationships);
    layout.start(graph);
    sigma.refresh();
  });

  // Refresh rendering when selection/hover changes
  $effect(() => {
    if (!sigma) return;
    // Touch reactive values to trigger on change
    void selectedEntityId;
    void hoveredEntityId;
    sigma.refresh();
  });

  function handleZoomIn() {
    if (!sigma) return;
    const camera = sigma.getCamera();
    camera.animatedZoom({ duration: 200 });
  }

  function handleZoomOut() {
    if (!sigma) return;
    const camera = sigma.getCamera();
    camera.animatedUnzoom({ duration: 200 });
  }

  function handleFitToContent() {
    if (!sigma) return;
    const camera = sigma.getCamera();
    camera.animatedReset({ duration: 300 });
  }
</script>

<div class="sigma-canvas-container" data-testid="sigma-canvas">
  <!-- Controls -->
  <div class="zoom-controls">
    <button onclick={handleZoomIn} aria-label="Zoom in" title="Zoom in"
      >+</button
    >
    <button onclick={handleZoomOut} aria-label="Zoom out" title="Zoom out"
      >-</button
    >
    <button
      onclick={handleFitToContent}
      aria-label="Fit to content"
      title="Fit to content"
    >
      <span class="fit-icon">&#x2A01;</span>
    </button>
    {#if onRefresh}
      <button
        onclick={onRefresh}
        aria-label="Refresh data"
        title="Refresh data"
        disabled={loading}
        class:refreshing={loading}
      >
        <span class="refresh-icon">&#x21bb;</span>
      </button>
    {/if}
  </div>

  <!-- Sigma container -->
  <div class="sigma-container" bind:this={containerElement}></div>

  <!-- Stats overlay -->
  <div class="graph-stats">
    <span>{entities.length} entities</span>
    <span>{relationships.length} relationships</span>
  </div>
</div>

<style>
  .sigma-canvas-container {
    position: relative;
    width: 100%;
    height: 100%;
    overflow: hidden;
    background: var(--ui-surface-primary);
  }

  .sigma-container {
    width: 100%;
    height: 100%;
  }

  /* Zoom Controls */
  .zoom-controls {
    position: absolute;
    top: 12px;
    right: 12px;
    display: flex;
    flex-direction: column;
    gap: 4px;
    z-index: 10;
  }

  .zoom-controls button {
    width: 32px;
    height: 32px;
    border: 1px solid var(--ui-border-subtle);
    border-radius: 6px;
    background: var(--ui-surface-primary);
    color: var(--ui-text-primary);
    font-size: 18px;
    font-weight: 500;
    cursor: pointer;
    display: flex;
    align-items: center;
    justify-content: center;
    transition: all 0.2s;
  }

  .zoom-controls button:hover {
    background: var(--ui-surface-secondary);
    border-color: var(--ui-border-strong);
  }

  .zoom-controls button:active {
    transform: scale(0.95);
  }

  .fit-icon {
    font-size: 14px;
  }

  .refresh-icon {
    font-size: 16px;
  }

  .zoom-controls button:disabled {
    opacity: 0.5;
    cursor: not-allowed;
  }

  .zoom-controls button.refreshing .refresh-icon {
    animation: spin 1s linear infinite;
  }

  @keyframes spin {
    to {
      transform: rotate(360deg);
    }
  }

  /* Stats overlay */
  .graph-stats {
    position: absolute;
    bottom: 12px;
    left: 12px;
    display: flex;
    gap: 12px;
    font-size: 11px;
    color: var(--ui-text-secondary);
    background: var(--ui-surface-primary);
    padding: 4px 8px;
    border-radius: 4px;
    border: 1px solid var(--ui-border-subtle);
  }
</style>
