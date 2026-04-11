<script lang="ts">
	import { SvelteMap } from "svelte/reactivity";
	import { onMount } from 'svelte';
	import type { ComponentType } from '$lib/types/component';
	import { getTypeColor } from '$lib/utils/category-colors';
	import { isConnectivityError } from '$lib/services/healthCheck';

	interface ComponentPaletteProps {
		onAddComponent?: (componentType: ComponentType) => void;
	}

	let { onAddComponent }: ComponentPaletteProps = $props();

	let components = $state<ComponentType[]>([]);
	let loading = $state(true);
	let error = $state<string | null>(null);

	// Group components by category
	const componentsByCategory = $derived.by(() => {
		const groups = new SvelteMap<string, ComponentType[]>();
		components.forEach((component) => {
			const category = component.category || 'other';
			if (!groups.has(category)) {
				groups.set(category, []);
			}
			groups.get(category)!.push(component);
		});
		return groups;
	});

	onMount(async () => {
		try {
			// Add timeout to prevent hanging forever
			const controller = new AbortController();
			const timeoutId = setTimeout(() => controller.abort(), 10000); // 10 second timeout

			const response = await fetch('/components/types', { signal: controller.signal });
			clearTimeout(timeoutId);

			if (!response.ok) {
				throw new Error(`Failed to fetch components: ${response.statusText}`);
			}
			const data = await response.json();
			// Backend returns flat array, not wrapped object
			components = Array.isArray(data) ? data : [];
		} catch (err) {
			if (isConnectivityError(err)) {
				error = 'Cannot connect to backend. Please ensure the backend service is running.';
			} else if (err instanceof Error) {
				if (err.name === 'AbortError') {
					error = 'Request timed out. The backend may be slow or unavailable.';
				} else {
					error = err.message;
				}
			} else {
				error = 'Unexpected error loading components';
			}
			console.error('ComponentPalette fetch error:', err);
		} finally {
			loading = false;
		}
	});

	function handleDragStart(event: DragEvent, componentType: ComponentType) {
		if (event.dataTransfer) {
			event.dataTransfer.setData('application/json', JSON.stringify(componentType));
			event.dataTransfer.effectAllowed = 'copy';
		}
	}

	function handleDoubleClick(componentType: ComponentType) {
		onAddComponent?.(componentType);
	}

	function handleKeyDown(event: KeyboardEvent, componentType: ComponentType) {
		if (event.key === 'Enter' || event.key === ' ') {
			event.preventDefault();
			onAddComponent?.(componentType);
		}
	}
</script>

<div class="component-palette">
	<header>
		<h3>Components</h3>
	</header>

	<div class="palette-content">
		{#if loading}
			<div class="loading">Loading components...</div>
		{:else if error}
			<div class="error">Error loading components: {error}</div>
		{:else if components.length === 0}
			<div class="empty">
				<p>Component palette coming soon</p>
				<p class="empty-hint">Component types will be loaded from the registry</p>
			</div>
		{:else}
			{#each [...componentsByCategory] as [category, categoryComponents] (category)}
				<div class="category">
					<div class="category-header">{category}</div>
					<div class="component-list">
						{#each categoryComponents as component (component.id)}
							{@const categoryColor = getTypeColor(component.type)}
							<div
								class="component-card"
								data-component-id={component.id}
								data-component-name={component.name}
								draggable="true"
								ondragstart={(e) => handleDragStart(e, component)}
								ondblclick={() => handleDoubleClick(component)}
								onkeydown={(e) => handleKeyDown(e, component)}
								role="button"
								tabindex="0"
								aria-label={`${component.name}: ${component.description}. Drag to canvas, double-click, or press Enter to add.`}
								style="border-left: var(--palette-domain-stripe-width) solid {categoryColor};"
							>
								<div class="component-name">{component.name}</div>
								<div class="component-description">{component.description}</div>
								<div class="component-meta">
									<span class="component-protocol">{component.protocol}</span>
								</div>
							</div>
						{/each}
					</div>
				</div>
			{/each}
		{/if}
	</div>
</div>

<style>
	.component-palette {
		display: flex;
		flex-direction: column;
		height: 100%;
		background: var(--palette-background);
		border: 1px solid var(--ui-border-subtle);
		border-radius: 4px;
		overflow: hidden;
	}

	header {
		padding: 1rem;
		border-bottom: 1px solid var(--ui-border-subtle);
		background: var(--palette-header-background);
	}

	header h3 {
		margin: 0;
		font-size: 1.125rem;
		font-weight: 600;
	}

	.palette-content {
		flex: 1;
		overflow-y: auto;
		padding: 0.5rem;
	}

	.loading,
	.error,
	.empty {
		padding: 2rem;
		text-align: center;
		color: var(--ui-text-secondary);
	}

	.error {
		color: var(--status-error);
	}

	.empty p {
		margin: 0 0 0.5rem 0;
	}

	.empty-hint {
		font-size: 0.875rem;
		color: var(--ui-text-tertiary);
	}

	.category {
		margin-bottom: 1.5rem;
	}

	.category-header {
		padding: 0.5rem;
		font-size: 0.875rem;
		font-weight: 600;
		text-transform: uppercase;
		color: var(--ui-text-secondary);
		background: var(--ui-surface-tertiary);
		border-radius: 4px;
		margin-bottom: 0.5rem;
	}

	.component-list {
		display: flex;
		flex-direction: column;
		gap: 0.5rem;
	}

	.component-card {
		padding: 0.75rem;
		background: var(--palette-card-background);
		border: 1px solid var(--palette-card-border);
		border-radius: 4px;
		cursor: grab;
		transition: all 0.2s;
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

	.component-card:active {
		cursor: grabbing;
	}

	.component-name {
		font-weight: 600;
		font-size: 0.875rem;
		margin-bottom: 0.25rem;
	}

	.component-description {
		font-size: 0.75rem;
		color: var(--ui-text-secondary);
		margin-bottom: 0.5rem;
	}

	.component-meta {
		display: flex;
		justify-content: space-between;
		align-items: center;
	}

	.component-protocol {
		font-size: 0.75rem;
		color: var(--ui-text-tertiary);
		font-family: monospace;
	}
</style>
