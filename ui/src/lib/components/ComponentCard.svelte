<script lang="ts">
	import type { FlowNode } from '$lib/types/flow';
	import { getTypeColor } from '$lib/utils/category-colors';

	interface ComponentCardProps {
		node: FlowNode;
		selected?: boolean;
		onSelect?: () => void;
	}

	let { node, selected = false, onSelect }: ComponentCardProps = $props();

	// Category color from backend
	const categoryColor = $derived(getTypeColor(node.type));

	// Handle card click for selection
	function handleCardClick() {
		onSelect?.();
	}

	// Handle keyboard events for accessibility
	function handleKeyDown(event: KeyboardEvent) {
		if (event.key === 'Enter' || event.key === ' ') {
			event.preventDefault();
			onSelect?.();
		}
	}

</script>

<div
	class="component-card"
	class:selected
	style="border-left: 4px solid {categoryColor}"
	role="button"
	tabindex="0"
	aria-label={node.name}
	aria-pressed={selected}
	onclick={handleCardClick}
	onkeydown={handleKeyDown}
>
	<div class="card-content">
		<div class="card-header">
			<h4 class="component-name">{node.name || node.id}</h4>
		</div>
		<div class="component-type">Type: {node.component}</div>
	</div>

</div>

<style>
	.component-card {
		display: flex;
		flex-direction: row;
		align-items: center;
		justify-content: space-between;
		padding: 0.75rem;
		background: var(--palette-card-background);
		border: 1px solid var(--palette-card-border);
		border-radius: 4px;
		cursor: pointer;
		transition: all 0.2s;
		text-align: left;
		width: 100%;
		font-family: inherit;
		color: inherit;
	}

	.component-card:hover {
		border-color: var(--palette-card-border-hover);
		box-shadow: 0 2px 4px var(--palette-card-shadow-hover);
	}

	.component-card:focus {
		outline: 2px solid var(--ui-focus-ring);
		outline-offset: 2px;
		border-color: var(--palette-card-border-hover);
	}

	.component-card.selected {
		background: var(--palette-card-background-selected);
		border-color: var(--palette-card-border-selected);
	}

	.card-content {
		flex: 1;
		min-width: 0; /* Allow text truncation */
	}

	.card-header {
		display: flex;
		align-items: center;
		margin-bottom: 0.25rem;
	}

	.component-name {
		margin: 0;
		font-size: 0.875rem;
		font-weight: 600;
		color: var(--ui-text-primary);
		overflow: hidden;
		text-overflow: ellipsis;
		white-space: nowrap;
	}

	.component-type {
		font-size: 0.75rem;
		color: var(--ui-text-secondary);
		overflow: hidden;
		text-overflow: ellipsis;
		white-space: nowrap;
	}

</style>
