<script lang="ts">
	/**
	 * DataView - Knowledge graph visualization view
	 *
	 * Three-column layout for exploring the semantic knowledge graph:
	 * - Left: GraphFilters (search, type/domain filters, confidence)
	 * - Center: GraphCanvas (D3 force-directed visualization)
	 * - Right: GraphDetailPanel (selected entity details)
	 *
	 * This view is shown when the user switches from Flow to Data view
	 * while a flow is running.
	 */

	import { graphStore } from '$lib/stores/graphStore.svelte';
	import type { GraphEntity, GraphRelationship, GraphFilters as GraphFiltersType } from '$lib/types/graph';
	import { graphApi, GraphApiError } from '$lib/services/graphApi';
	import { transformPathSearchResult } from '$lib/services/graphTransform';

	import GraphFiltersPanel from './runtime/GraphFilters.svelte';
	import GraphCanvas from './runtime/GraphCanvas.svelte';
	import GraphDetailPanel from './runtime/GraphDetailPanel.svelte';

	interface DataViewProps {
		flowId: string;
	}

	// flowId will be used in Phase 5 to fetch flow-specific entities via GraphQL
	let { flowId: _flowId }: DataViewProps = $props();

	// Reset graph state synchronously during component initialization so each
	// DataView mount starts clean. This also sets loading=true immediately so
	// the loading indicator is visible on the very first render.
	graphStore.clearEntities();
	graphStore.setLoading(true);
	graphStore.setError(null);

	// Derived directly from the runes-based store — no subscription needed
	const entities = $derived<GraphEntity[]>(graphStore.getFilteredEntities());
	const relationships = $derived<GraphRelationship[]>(graphStore.getFilteredRelationships());
	const selectedEntity = $derived<GraphEntity | null>(
		graphStore.selectedEntityId
			? graphStore.entities.get(graphStore.selectedEntityId) ?? null
			: null
	);
	const availableTypes = $derived<string[]>(graphStore.getEntityTypes());
	const availableDomains = $derived<string[]>(graphStore.getDomains());
	const filters = $derived<GraphFiltersType>(graphStore.filters);

	// Kick off the initial data load after mount
	$effect(() => {
		loadGraphData();
	});

	// Load graph data from GraphQL API
	async function loadGraphData() {
		try {
			// Query for all entities with depth 2, up to 50 nodes
			const result = await graphApi.pathSearch('*', 2, 50);

			// Transform backend result to frontend entities
			const entities = transformPathSearchResult(result);

			// Update store with entities
			graphStore.upsertEntities(entities);
			graphStore.setConnected(true);
		} catch (error) {
			let errorMessage = 'Unable to connect to graph service';

			if (error instanceof GraphApiError) {
				// Network error (statusCode 0)
				if (error.statusCode === 0) {
					errorMessage = 'Unable to connect to graph service';
				}
				// Timeout error (statusCode 504 OR message includes 'timeout')
				else if (error.statusCode === 504 || error.message.toLowerCase().includes('timeout')) {
					errorMessage = 'Query timed out';
				}
				// Other GraphApiError - use the error message directly
				else {
					errorMessage = error.message;
				}
			} else if (error instanceof Error) {
				// Regular Error (including fetch errors)
				errorMessage = 'Unable to connect to graph service';
			} else {
				// Other error types (string, null, etc.)
				errorMessage = 'Unable to connect to graph service';
			}

			graphStore.setError(errorMessage);
			graphStore.setConnected(false);
		} finally {
			graphStore.setLoading(false);
		}
	}


	// Event handlers
	function handleEntitySelect(entityId: string | null) {
		graphStore.selectEntity(entityId);
	}

	// Exported so tests can call it directly via the Svelte 5 component instance
	export async function handleEntityExpand(entityId: string) {
		// Check if entity is already expanded
		if (graphStore.isExpanded(entityId)) {
			return;
		}

		try {
			// Query for entity neighbors with depth 1, up to 20 nodes
			const result = await graphApi.pathSearch(entityId, 1, 20);

			// Transform and upsert entities
			const entities = transformPathSearchResult(result);
			graphStore.upsertEntities(entities);

			// Mark entity as expanded
			graphStore.markExpanded(entityId);
		} catch (error) {
			let errorMessage = 'Unable to connect to graph service';

			if (error instanceof GraphApiError) {
				// Show the error message directly for expansion errors
				errorMessage = error.message;
			} else if (error instanceof Error) {
				errorMessage = error.message;
			}

			graphStore.setError(errorMessage);
		}
	}

	function handleEntityHover(entityId: string | null) {
		graphStore.setHoveredEntity(entityId);
	}

	function handleFilterChange(newFilters: Partial<GraphFiltersType>) {
		graphStore.setFilters(newFilters);
	}

	function handleFilterReset() {
		graphStore.resetFilters();
	}

	function handleDetailClose() {
		graphStore.selectEntity(null);
	}

	function handleDetailEntityClick(entityId: string) {
		graphStore.selectEntity(entityId);
	}

	// Refresh data: clear entities then reload
	function handleRefresh() {
		graphStore.clearEntities();
		graphStore.setLoading(true);
		graphStore.setError(null);
		loadGraphData();
	}
</script>

<div class="data-view" data-testid="data-view">
	{#if graphStore.loading}
		<div class="loading-overlay">
			<div class="loading-spinner"></div>
			<span>Loading graph data...</span>
		</div>
	{/if}

	{#if graphStore.error}
		<div class="error-banner" role="alert">
			<span class="error-icon">!</span>
			<span class="error-message">{graphStore.error}</span>
			<button onclick={handleRefresh} class="retry-button">Retry</button>
		</div>
	{/if}

	<!-- Left Panel: Filters -->
	<aside class="data-view-left">
		<GraphFiltersPanel
			{filters}
			{availableTypes}
			{availableDomains}
			onFilterChange={handleFilterChange}
			onReset={handleFilterReset}
		/>
	</aside>

	<!-- Center Panel: Canvas -->
	<main class="data-view-center">
		<GraphCanvas
			{entities}
			{relationships}
			selectedEntityId={graphStore.selectedEntityId}
			hoveredEntityId={graphStore.hoveredEntityId}
			onEntitySelect={handleEntitySelect}
			onEntityExpand={handleEntityExpand}
			onEntityHover={handleEntityHover}
		/>

		<!-- Toolbar overlay -->
		<div class="toolbar">
			<button
				class="toolbar-button"
				onclick={handleRefresh}
				title="Refresh data"
				aria-label="Refresh"
				disabled={graphStore.loading}
			>
				<span class="toolbar-icon">↻</span>
			</button>
		</div>
	</main>

	<!-- Right Panel: Detail -->
	<aside class="data-view-right">
		<GraphDetailPanel
			entity={selectedEntity}
			onClose={handleDetailClose}
			onEntityClick={handleDetailEntityClick}
		/>
	</aside>
</div>

<style>
	.data-view {
		display: grid;
		grid-template-columns: 250px 1fr 320px;
		height: 100%;
		position: relative;
		background: var(--ui-surface-primary);
	}

	.data-view-left {
		border-right: 1px solid var(--ui-border-subtle);
		overflow: hidden;
	}

	.data-view-center {
		position: relative;
		overflow: hidden;
	}

	.data-view-right {
		border-left: 1px solid var(--ui-border-subtle);
		overflow: hidden;
	}

	/* Loading overlay */
	.loading-overlay {
		position: absolute;
		top: 0;
		left: 0;
		right: 0;
		bottom: 0;
		background: rgba(var(--ui-surface-primary-rgb), 0.8);
		display: flex;
		flex-direction: column;
		align-items: center;
		justify-content: center;
		gap: 12px;
		z-index: 100;
	}

	.loading-spinner {
		width: 32px;
		height: 32px;
		border: 3px solid var(--ui-border-subtle);
		border-top-color: var(--ui-interactive-primary);
		border-radius: 50%;
		animation: spin 1s linear infinite;
	}

	@keyframes spin {
		to {
			transform: rotate(360deg);
		}
	}

	/* Error banner */
	.error-banner {
		position: absolute;
		top: 12px;
		left: 50%;
		transform: translateX(-50%);
		display: flex;
		align-items: center;
		gap: 8px;
		padding: 8px 16px;
		background: var(--status-error-bg);
		border: 1px solid var(--status-error);
		border-radius: 6px;
		z-index: 90;
	}

	.error-icon {
		width: 20px;
		height: 20px;
		border-radius: 50%;
		background: var(--status-error);
		color: white;
		display: flex;
		align-items: center;
		justify-content: center;
		font-weight: bold;
		font-size: 12px;
	}

	.error-message {
		color: var(--status-error);
		font-size: 13px;
	}

	.retry-button {
		padding: 4px 10px;
		border: 1px solid var(--status-error);
		border-radius: 4px;
		background: transparent;
		color: var(--status-error);
		font-size: 12px;
		cursor: pointer;
		transition: all 0.2s;
	}

	.retry-button:hover {
		background: var(--status-error);
		color: white;
	}

	/* Toolbar */
	.toolbar {
		position: absolute;
		top: 12px;
		left: 12px;
		display: flex;
		gap: 4px;
		z-index: 10;
	}

	.toolbar-button {
		width: 32px;
		height: 32px;
		border: 1px solid var(--ui-border-subtle);
		border-radius: 6px;
		background: var(--ui-surface-primary);
		color: var(--ui-text-primary);
		font-size: 16px;
		cursor: pointer;
		display: flex;
		align-items: center;
		justify-content: center;
		transition: all 0.2s;
	}

	.toolbar-button:hover:not(:disabled) {
		background: var(--ui-surface-secondary);
		border-color: var(--ui-border-strong);
	}

	.toolbar-button:disabled {
		opacity: 0.5;
		cursor: not-allowed;
	}

	.toolbar-icon {
		font-size: 14px;
	}

	/* Responsive: collapse panels on smaller screens */
	@media (max-width: 1200px) {
		.data-view {
			grid-template-columns: 200px 1fr 280px;
		}
	}

	@media (max-width: 900px) {
		.data-view {
			grid-template-columns: 1fr;
		}

		.data-view-left,
		.data-view-right {
			display: none;
		}
	}
</style>
