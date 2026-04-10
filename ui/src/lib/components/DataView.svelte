<script lang="ts">
	/**
	 * DataView - Knowledge graph visualization view
	 *
	 * Three-column layout for exploring the semantic knowledge graph:
	 * - Left: GraphFilters (search, type/domain filters, confidence)
	 * - Center: SigmaCanvas (WebGL Sigma.js visualization)
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
	import SigmaCanvas from './runtime/SigmaCanvas.svelte';
	import GraphDetailPanel from './runtime/GraphDetailPanel.svelte';
	import ChatPanel from './chat/ChatPanel.svelte';
	import { chatStore } from '$lib/stores/chatStore.svelte';
	import type { ContextChip } from '$lib/types/chat';
	import { getCommandsForPage } from '$lib/services/slashCommands';

	interface DataViewProps {
		flowId?: string;
	}

	let { flowId }: DataViewProps = $props();

	// Tab state for the right panel: "details" or "chat"
	// Default: "details" when an entity is selected, "chat" when none is selected.
	let activeTab = $state<'details' | 'chat'>(
		graphStore.selectedEntityId ? 'details' : 'chat'
	);

	// Auto-switch to Details tab when graphStore.selectedEntityId transitions from null to non-null.
	// This handles the "view entity from chat" flow without overriding explicit tab clicks.
	let prevSelectedEntityId = $state<string | null>(graphStore.selectedEntityId);
	$effect(() => {
		const current = graphStore.selectedEntityId;
		if (current !== prevSelectedEntityId) {
			prevSelectedEntityId = current;
			if (current !== null) {
				activeTab = 'details';
			}
		}
	});

	// On mount: reset transient state (loading, error, expansion tracking) but
	// do NOT call clearEntities() here — the initial data load (via $effect) will
	// upsert fresh data. This preserves pre-seeded store state until the first
	// async load completes.
	graphStore.clearExpanded();
	graphStore.setLoading(true);
	graphStore.setError(null);

	// Slash commands available for the data-view page
	const dataViewCommands = getCommandsForPage('data-view');

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

	// Kick off the initial data load after mount.
	$effect(() => {
		loadGraphData();
	});

	// Load graph data from GraphQL API
	async function loadGraphData() {
		try {
			// Load all entities via prefix scan, then enrich with pathSearch
			// for relationship discovery from connected entities.
			const backendEntities = await graphApi.getEntitiesByPrefix('', 200);

			// Guard: treat null/undefined response as an empty list.
			const safeBackendEntities = Array.isArray(backendEntities) ? backendEntities : [];

			// Transform to frontend entities using the same PathSearchResult shape
			const entities = transformPathSearchResult({
				entities: safeBackendEntities,
				edges: [],
			});

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

	// Tab switching
	function handleTabClick(tab: 'details' | 'chat') {
		activeTab = tab;
	}

	// Handle "+Chat" button in GraphDetailPanel: add chip and switch to chat tab
	function handleAddChipFromDetail(chip: ContextChip) {
		chatStore.addChip(chip);
		activeTab = 'chat';
	}

	// Called from chat when user wants to view an entity in the detail panel.
	// Exported so tests can call it directly via the Svelte 5 component instance.
	export function handleViewEntityFromChat(entityId: string) {
		graphStore.selectEntity(entityId);
		activeTab = 'details';
	}

	// Build DataViewContext for chat requests.
	// Used now to satisfy linting; will be wired to chatApi in a future phase.
	function buildDataViewContext() {
		return {
			page: 'data-view' as const,
			flowId: flowId ?? '',
			entityCount: graphStore.entities.size,
			selectedEntityId: graphStore.selectedEntityId,
			filters: graphStore.filters,
		};
	}

	// Chat submit handler — sends message with DataViewContext
	function handleChatSubmit(_content: string) {
		// Build context to confirm flowId is referenced (chatApi integration is future work)
		void buildDataViewContext();
	}

	// E2E test seam — expose entity selection on window so Playwright tests can
	// select graph entities deterministically without relying on WebGL canvas clicks.
	// Only registered in browser environments; cleaned up on component destroy.
	$effect(() => {
		if (typeof window === 'undefined') return;

		window.__e2eSelectEntity = (entityId: string) => {
			graphStore.selectEntity(entityId);
		};
		window.__e2eHoverEntity = (entityId: string | null) => {
			graphStore.setHoveredEntity(entityId);
		};
		window.__e2eSetFilters = (partial) => {
			graphStore.setFilters(partial);
		};
		window.__e2eExpandEntity = async (entityId: string) => {
			await handleEntityExpand(entityId);
		};

		return () => {
			delete window.__e2eSelectEntity;
			delete window.__e2eHoverEntity;
			delete window.__e2eSetFilters;
			delete window.__e2eExpandEntity;
		};
	});
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
		<SigmaCanvas
			{entities}
			{relationships}
			selectedEntityId={graphStore.selectedEntityId}
			hoveredEntityId={graphStore.hoveredEntityId}
			onEntitySelect={handleEntitySelect}
			onEntityExpand={handleEntityExpand}
			onEntityHover={handleEntityHover}
			onRefresh={handleRefresh}
			loading={graphStore.loading}
		/>
	</main>

	<!-- Right Panel: Tabs + Detail / Chat -->
	<aside class="data-view-right">
		<div class="right-panel-tabs" role="tablist">
			<button
				role="tab"
				data-testid="data-view-tab-details"
				aria-label="Details tab"
				aria-selected={activeTab === 'details'}
				data-active={activeTab === 'details' ? 'true' : undefined}
				class="tab-button"
				class:active={activeTab === 'details'}
				onclick={() => handleTabClick('details')}
			>Details</button>
			<button
				role="tab"
				data-testid="data-view-tab-chat"
				aria-label="Chat tab"
				aria-selected={activeTab === 'chat'}
				data-active={activeTab === 'chat' ? 'true' : undefined}
				class="tab-button"
				class:active={activeTab === 'chat'}
				onclick={() => handleTabClick('chat')}
			>Chat</button>
		</div>

		{#if activeTab === 'details'}
			<GraphDetailPanel
				entity={selectedEntity}
				onClose={handleDetailClose}
				onEntityClick={handleDetailEntityClick}
				onAddChip={handleAddChipFromDetail}
			/>
		{:else}
			<ChatPanel
				messages={chatStore.messages}
				isStreaming={chatStore.isStreaming}
				streamingContent={chatStore.streamingContent}
				error={chatStore.error}
				chips={chatStore.chips}
				onRemoveChip={chatStore.removeChip}
				onClearChips={chatStore.clearChips}
				commands={dataViewCommands}
				onSubmit={handleChatSubmit}
				onNewChat={() => chatStore.clearConversation()}
			/>
		{/if}
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
		display: flex;
		flex-direction: column;
	}

	.right-panel-tabs {
		display: flex;
		border-bottom: 1px solid var(--ui-border-subtle);
		flex-shrink: 0;
	}

	.tab-button {
		flex: 1;
		padding: 8px 12px;
		border: none;
		border-bottom: 2px solid transparent;
		background: transparent;
		color: var(--ui-text-secondary);
		font-size: 12px;
		font-weight: 500;
		cursor: pointer;
		transition: all 0.2s;
	}

	.tab-button:hover {
		color: var(--ui-text-primary);
		background: var(--ui-surface-tertiary);
	}

	.tab-button.active {
		color: var(--ui-interactive-primary, #4a9eff);
		border-bottom-color: var(--ui-interactive-primary, #4a9eff);
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
