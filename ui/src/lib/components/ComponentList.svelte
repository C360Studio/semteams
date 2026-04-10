<script lang="ts">
	import type { FlowNode } from '$lib/types/flow';
	import ComponentCard from './ComponentCard.svelte';

	interface ComponentListProps {
		nodes: FlowNode[];
		selectedNodeId?: string | null;
		onSelectNode?: (nodeId: string) => void;
		onAddComponent?: () => void;
	}

	let {
		nodes,
		selectedNodeId = null,
		onSelectNode,
		onAddComponent
	}: ComponentListProps = $props();

	// Search/filter state
	let searchQuery = $state('');

	// Filter nodes based on search query
	const filteredNodes = $derived.by(() => {
		if (!searchQuery.trim()) {
			return nodes;
		}

		const query = searchQuery.toLowerCase().trim();
		return nodes.filter(
			(node) =>
				node.name.toLowerCase().includes(query) || node.component.toLowerCase().includes(query)
		);
	});

	// Handle Add button click
	function handleAddClick() {
		onAddComponent?.();
	}

	// Handle node selection
	function handleSelectNode(nodeId: string) {
		onSelectNode?.(nodeId);
	}
</script>

<div class="component-list">
	<header class="list-header">
		<h3>Components ({filteredNodes.length})</h3>
		<button
			class="add-button"
			onclick={handleAddClick}
			aria-label="Add component"
			title="Add new component"
		>
			+ Add
		</button>
	</header>

	<div class="search-section">
		<label for="component-search" class="visually-hidden">Search components</label>
		<input
			id="component-search"
			type="text"
			class="search-input"
			placeholder="Search components..."
			bind:value={searchQuery}
			aria-label="Search components"
		/>
	</div>

	<div class="list-content">
		{#if nodes.length === 0}
			<div class="empty-state">
				<p class="empty-message">No components yet</p>
				<p class="empty-hint">Add a component to get started</p>
			</div>
		{:else if filteredNodes.length === 0}
			<div class="empty-state">
				<p class="empty-message">No components found</p>
				<p class="empty-hint">Try a different search term</p>
			</div>
		{:else}
			<ul class="component-list-items" role="list" aria-live="polite">
				{#each filteredNodes as node (node.id)}
					<li role="listitem">
						<ComponentCard
							{node}
							selected={node.id === selectedNodeId}
							onSelect={() => handleSelectNode(node.id)}
						/>
					</li>
				{/each}
			</ul>
		{/if}
	</div>
</div>

<style>
	.component-list {
		display: flex;
		flex-direction: column;
		height: 100%;
		background: var(--palette-background);
		border: 1px solid var(--ui-border-subtle);
		border-radius: 4px;
		overflow: hidden;
	}

	.list-header {
		display: flex;
		justify-content: space-between;
		align-items: center;
		padding: 1rem;
		border-bottom: 1px solid var(--ui-border-subtle);
		background: var(--palette-header-background);
	}

	.list-header h3 {
		margin: 0;
		font-size: 1.125rem;
		font-weight: 600;
	}

	.add-button {
		padding: 0.5rem 1rem;
		background: var(--ui-interactive-primary);
		color: var(--ui-text-on-primary);
		border: none;
		border-radius: 4px;
		cursor: pointer;
		font-size: 0.875rem;
		font-weight: 600;
		transition: background 0.2s;
	}

	.add-button:hover {
		background: var(--ui-interactive-primary-hover);
	}

	.add-button:focus {
		outline: 2px solid var(--ui-focus-ring);
		outline-offset: 2px;
	}

	.search-section {
		padding: 0.75rem 1rem;
		border-bottom: 1px solid var(--ui-border-subtle);
	}

	.search-input {
		width: 100%;
		padding: 0.5rem;
		border: 1px solid var(--ui-border-default);
		border-radius: 4px;
		font-size: 0.875rem;
		background: var(--ui-surface-primary);
		color: var(--ui-text-primary);
	}

	.search-input:focus {
		outline: 2px solid var(--ui-focus-ring);
		outline-offset: 1px;
		border-color: var(--ui-interactive-primary);
	}

	.list-content {
		flex: 1;
		overflow-y: auto;
		padding: 0.5rem;
	}

	.empty-state {
		padding: 2rem 1rem;
		text-align: center;
		color: var(--ui-text-secondary);
	}

	.empty-message {
		margin: 0 0 0.5rem 0;
		font-weight: 600;
		color: var(--ui-text-primary);
	}

	.empty-hint {
		margin: 0;
		font-size: 0.875rem;
		color: var(--ui-text-tertiary);
	}

	.component-list-items {
		list-style: none;
		padding: 0;
		margin: 0;
		display: flex;
		flex-direction: column;
		gap: 0.5rem;
	}

	.component-list-items li {
		margin: 0;
		padding: 0;
	}

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
