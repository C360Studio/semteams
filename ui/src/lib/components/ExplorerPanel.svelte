<script lang="ts">
	/**
	 * ExplorerPanel - Left panel with tabbed interface
	 *
	 * Features:
	 * - Components tab: Current flow's components (search, list, selection)
	 * - Palette tab: Available component types to add (grouped by category)
	 * - Search input that filters the active tab
	 * - AI Assistant section at bottom
	 */

	import { SvelteMap } from 'svelte/reactivity';
	import { onMount } from 'svelte';
	import type { FlowNode } from '$lib/types/flow';
	import type { ComponentType } from '$lib/types/component';
	import type { ExplorerTab } from '$lib/types/ui-state';
	import ComponentCard from './ComponentCard.svelte';
	import { getTypeColor } from '$lib/utils/category-colors';
	import { isConnectivityError } from '$lib/services/healthCheck';

	interface ExplorerPanelProps {
		/** Current flow's nodes */
		nodes: FlowNode[];
		/** Currently selected node ID */
		selectedNodeId?: string | null;
		/** Active tab */
		activeTab?: ExplorerTab;
		/** Hovered component type (for Properties panel preview) */
		hoveredType?: ComponentType | null;
		/** Callback when node is selected */
		onSelectNode?: (nodeId: string) => void;
		/** Callback when component type is clicked (add to flow) */
		onAddComponent?: (type: ComponentType) => void;
		/** Callback when hovering over a component type */
		onHoverType?: (type: ComponentType | null) => void;
		/** Callback when tab changes */
		onTabChange?: (tab: ExplorerTab) => void;
		/** AI prompt input snippet (optional) */
		aiAssistant?: import('svelte').Snippet;
	}

	let {
		nodes,
		selectedNodeId = null,
		activeTab = 'components',
		hoveredType = null,
		onSelectNode,
		onAddComponent,
		onHoverType,
		onTabChange,
		aiAssistant
	}: ExplorerPanelProps = $props();

	// Search state
	let searchQuery = $state('');

	// Component types from backend (for Palette tab)
	let componentTypes = $state<ComponentType[]>([]);
	let typesLoading = $state(true);
	let typesError = $state<string | null>(null);

	// Load component types on mount
	onMount(async () => {
		try {
			const controller = new AbortController();
			const timeoutId = setTimeout(() => controller.abort(), 10000);

			const response = await fetch('/components/types', { signal: controller.signal });
			clearTimeout(timeoutId);

			if (!response.ok) {
				throw new Error(`Failed to fetch components: ${response.statusText}`);
			}
			const data = await response.json();
			componentTypes = Array.isArray(data) ? data : [];
		} catch (err) {
			if (isConnectivityError(err)) {
				typesError = 'Cannot connect to backend. Please ensure the backend service is running.';
			} else if (err instanceof Error) {
				if (err.name === 'AbortError') {
					typesError = 'Request timed out. The backend may be slow or unavailable.';
				} else {
					typesError = err.message;
				}
			} else {
				typesError = 'Unexpected error loading components';
			}
			console.error('ExplorerPanel fetch error:', err);
		} finally {
			typesLoading = false;
		}
	});

	// Filter nodes based on search (Components tab)
	const filteredNodes = $derived.by(() => {
		if (!searchQuery.trim()) return nodes;
		const query = searchQuery.toLowerCase().trim();
		return nodes.filter(
			(node) =>
				node.name.toLowerCase().includes(query) || node.component.toLowerCase().includes(query)
		);
	});

	// Filter and group component types by category (Palette tab)
	const filteredTypesByCategory = $derived.by(() => {
		const query = searchQuery.toLowerCase().trim();
		const filtered = query
			? componentTypes.filter(
					(ct) =>
						ct.name.toLowerCase().includes(query) ||
						ct.description?.toLowerCase().includes(query) ||
						ct.category?.toLowerCase().includes(query)
				)
			: componentTypes;

		const groups = new SvelteMap<string, ComponentType[]>();
		filtered.forEach((ct) => {
			const category = ct.category || 'other';
			if (!groups.has(category)) {
				groups.set(category, []);
			}
			groups.get(category)!.push(ct);
		});
		return groups;
	});

	// Tab handlers
	function handleTabClick(tab: ExplorerTab) {
		onTabChange?.(tab);
		searchQuery = ''; // Clear search when switching tabs
	}

	// Component type handlers (Palette tab)
	function handleTypeClick(type: ComponentType) {
		onAddComponent?.(type);
	}

	function handleTypeMouseEnter(type: ComponentType) {
		onHoverType?.(type);
	}

	function handleTypeMouseLeave() {
		onHoverType?.(null);
	}

	function handleTypeKeyDown(event: KeyboardEvent, type: ComponentType) {
		if (event.key === 'Enter' || event.key === ' ') {
			event.preventDefault();
			onAddComponent?.(type);
		}
	}

	// Drag support for palette items
	function handleDragStart(event: DragEvent, type: ComponentType) {
		if (event.dataTransfer) {
			event.dataTransfer.setData('application/json', JSON.stringify(type));
			event.dataTransfer.effectAllowed = 'copy';
		}
	}
</script>

<div class="explorer-panel" data-testid="explorer-panel">
	<!-- Tab Bar -->
	<div class="tab-bar" role="tablist" aria-label="Explorer tabs">
		<button
			role="tab"
			id="tab-components"
			aria-selected={activeTab === 'components'}
			aria-controls="panel-components"
			class="tab-button"
			class:active={activeTab === 'components'}
			onclick={() => handleTabClick('components')}
			data-testid="tab-components"
		>
			Components
			{#if nodes.length > 0}
				<span class="tab-badge">{nodes.length}</span>
			{/if}
		</button>
		<button
			role="tab"
			id="tab-palette"
			aria-selected={activeTab === 'palette'}
			aria-controls="panel-palette"
			class="tab-button"
			class:active={activeTab === 'palette'}
			onclick={() => handleTabClick('palette')}
			data-testid="tab-palette"
		>
			Palette
		</button>
	</div>

	<!-- Search Input -->
	<div class="search-section">
		<label for="explorer-search" class="visually-hidden">
			{activeTab === 'components' ? 'Search components' : 'Search component types'}
		</label>
		<input
			id="explorer-search"
			type="text"
			class="search-input"
			placeholder={activeTab === 'components' ? 'Search components...' : 'Search types...'}
			bind:value={searchQuery}
			data-testid="explorer-search"
		/>
	</div>

	<!-- Tab Content -->
	<div class="tab-content">
		{#if activeTab === 'components'}
			<!-- Components Tab -->
			<div
				id="panel-components"
				role="tabpanel"
				aria-labelledby="tab-components"
				class="components-panel"
				data-testid="panel-components"
			>
				{#if nodes.length === 0}
					<div class="empty-state">
						<p class="empty-title">No components yet</p>
						<p class="empty-hint">
							Switch to the <button
								class="link-button"
								onclick={() => handleTabClick('palette')}>Palette</button
							> tab to add components
						</p>
					</div>
				{:else if filteredNodes.length === 0}
					<div class="empty-state">
						<p class="empty-title">No matches</p>
						<p class="empty-hint">Try a different search term</p>
					</div>
				{:else}
					<ul class="component-list" role="list">
						{#each filteredNodes as node (node.id)}
							<li>
								<ComponentCard
									{node}
									selected={node.id === selectedNodeId}
									onSelect={() => onSelectNode?.(node.id)}
								/>
							</li>
						{/each}
					</ul>
				{/if}
			</div>
		{:else}
			<!-- Palette Tab -->
			<div
				id="panel-palette"
				role="tabpanel"
				aria-labelledby="tab-palette"
				class="palette-panel"
				data-testid="panel-palette"
			>
				{#if typesLoading}
					<div class="loading-state">Loading component types...</div>
				{:else if typesError}
					<div class="error-state">{typesError}</div>
				{:else if componentTypes.length === 0}
					<div class="empty-state">
						<p class="empty-title">No component types</p>
						<p class="empty-hint">Component types will be loaded from the registry</p>
					</div>
				{:else if filteredTypesByCategory.size === 0}
					<div class="empty-state">
						<p class="empty-title">No matches</p>
						<p class="empty-hint">Try a different search term</p>
					</div>
				{:else}
					{#each [...filteredTypesByCategory] as [category, types] (category)}
						<div class="category-group">
							<div class="category-header">{category}</div>
							<div class="type-list">
								{#each types as type (type.id)}
									{@const categoryColor = getTypeColor(type.type)}
									<div
										class="type-card"
										class:hovered={hoveredType?.id === type.id}
										role="button"
										tabindex="0"
										draggable="true"
										ondragstart={(e) => handleDragStart(e, type)}
										onclick={() => handleTypeClick(type)}
										onmouseenter={() => handleTypeMouseEnter(type)}
										onmouseleave={handleTypeMouseLeave}
										onkeydown={(e) => handleTypeKeyDown(e, type)}
										style="border-left-color: {categoryColor};"
										data-testid="type-card-{type.id}"
									>
										<div class="type-name">{type.name}</div>
										<div class="type-description">{type.description}</div>
										<div class="type-meta">
											<span class="type-protocol">{type.protocol}</span>
										</div>
									</div>
								{/each}
							</div>
						</div>
					{/each}
				{/if}
			</div>
		{/if}
	</div>

	<!-- AI Assistant Section (optional) -->
	{#if aiAssistant}
		<div class="ai-section">
			<div class="ai-divider"></div>
			{@render aiAssistant()}
		</div>
	{/if}
</div>

<style>
	.explorer-panel {
		display: flex;
		flex-direction: column;
		height: 100%;
		background: var(--explorer-background, var(--ui-surface-secondary));
	}

	/* Tab Bar */
	.tab-bar {
		display: flex;
		background: var(--explorer-tab-bar-bg, var(--ui-surface-tertiary));
		border-bottom: 1px solid var(--ui-border-subtle);
		padding: 0 0.5rem;
	}

	.tab-button {
		flex: 1;
		padding: 0.75rem 0.5rem;
		background: var(--explorer-tab-inactive-bg, transparent);
		border: none;
		border-bottom: 2px solid transparent;
		color: var(--explorer-tab-text, var(--ui-text-secondary));
		font-size: 0.875rem;
		font-weight: 500;
		cursor: pointer;
		transition: all 150ms ease;
		display: flex;
		align-items: center;
		justify-content: center;
		gap: 0.5rem;
	}

	.tab-button:hover {
		color: var(--ui-text-primary);
		background: var(--ui-surface-secondary);
	}

	.tab-button.active {
		background: var(--explorer-tab-active-bg, var(--ui-surface-secondary));
		color: var(--explorer-tab-active-text, var(--ui-text-primary));
		border-bottom-color: var(--ui-interactive-primary);
	}

	.tab-button:focus-visible {
		outline: 2px solid var(--ui-focus-ring);
		outline-offset: -2px;
	}

	.tab-badge {
		background: var(--ui-interactive-primary);
		color: var(--ui-text-on-primary);
		font-size: 0.75rem;
		padding: 0.125rem 0.375rem;
		border-radius: 10px;
		min-width: 1.25rem;
		text-align: center;
	}

	/* Search Section */
	.search-section {
		padding: 0.75rem;
		border-bottom: 1px solid var(--ui-border-subtle);
	}

	.search-input {
		width: 100%;
		padding: 0.5rem 0.75rem;
		border: 1px solid var(--ui-border-subtle);
		border-radius: 4px;
		font-size: 0.875rem;
		background: var(--ui-surface-primary);
		color: var(--ui-text-primary);
	}

	.search-input:focus {
		outline: none;
		border-color: var(--ui-interactive-primary);
		box-shadow: 0 0 0 2px var(--ui-focus-ring);
	}

	.search-input::placeholder {
		color: var(--ui-text-placeholder);
	}

	/* Tab Content */
	.tab-content {
		flex: 1;
		overflow-y: auto;
	}

	.components-panel,
	.palette-panel {
		padding: 0.5rem;
	}

	/* Empty/Loading/Error States */
	.empty-state,
	.loading-state,
	.error-state {
		padding: 2rem 1rem;
		text-align: center;
	}

	.empty-title {
		margin: 0 0 0.5rem;
		font-weight: 600;
		color: var(--ui-text-primary);
	}

	.empty-hint {
		margin: 0;
		font-size: 0.875rem;
		color: var(--ui-text-tertiary);
	}

	.link-button {
		background: none;
		border: none;
		color: var(--ui-interactive-primary);
		cursor: pointer;
		text-decoration: underline;
		font-size: inherit;
		padding: 0;
	}

	.link-button:hover {
		color: var(--ui-interactive-primary-hover);
	}

	.loading-state {
		color: var(--ui-text-secondary);
	}

	.error-state {
		color: var(--status-error);
	}

	/* Component List (Components tab) */
	.component-list {
		list-style: none;
		margin: 0;
		padding: 0;
		display: flex;
		flex-direction: column;
		gap: 0.5rem;
	}

	.component-list li {
		margin: 0;
	}

	/* Category Groups (Palette tab) */
	.category-group {
		margin-bottom: 1rem;
	}

	.category-header {
		padding: 0.5rem 0.75rem;
		font-size: 0.75rem;
		font-weight: 600;
		text-transform: uppercase;
		letter-spacing: 0.05em;
		color: var(--ui-text-secondary);
		background: var(--ui-surface-tertiary);
		border-radius: 4px;
		margin-bottom: 0.5rem;
	}

	.type-list {
		display: flex;
		flex-direction: column;
		gap: 0.375rem;
	}

	/* Type Card (Palette tab) */
	.type-card {
		padding: 0.625rem 0.75rem;
		background: var(--ui-surface-primary);
		border: 1px solid var(--ui-border-subtle);
		border-left-width: 3px;
		border-radius: 4px;
		cursor: pointer;
		transition: all 150ms ease;
	}

	.type-card:hover,
	.type-card.hovered {
		border-color: var(--ui-interactive-primary);
		box-shadow: 0 2px 4px rgba(0, 0, 0, 0.1);
	}

	.type-card:focus-visible {
		outline: 2px solid var(--ui-focus-ring);
		outline-offset: 1px;
	}

	.type-card:active {
		background: var(--ui-surface-secondary);
	}

	.type-name {
		font-weight: 600;
		font-size: 0.8125rem;
		margin-bottom: 0.25rem;
		color: var(--ui-text-primary);
	}

	.type-description {
		font-size: 0.75rem;
		color: var(--ui-text-secondary);
		margin-bottom: 0.375rem;
		line-height: 1.3;
	}

	.type-meta {
		display: flex;
		justify-content: space-between;
	}

	.type-protocol {
		font-size: 0.6875rem;
		color: var(--ui-text-tertiary);
		font-family: monospace;
	}

	/* AI Section */
	.ai-section {
		border-top: 1px solid var(--ui-border-subtle);
		flex-shrink: 0;
	}

	.ai-divider {
		height: 1px;
		background: var(--ui-border-subtle);
	}

	/* Utilities */
	.visually-hidden {
		position: absolute;
		width: 1px;
		height: 1px;
		padding: 0;
		margin: -1px;
		overflow: hidden;
		clip: rect(0, 0, 0, 0);
		white-space: nowrap;
		border: 0;
	}
</style>
