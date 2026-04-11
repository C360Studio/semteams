<script lang="ts">
	import type { ComponentInstance } from '$lib/types/flow';

	interface ComponentNodeProps {
		component: ComponentInstance;
		onclick?: (_componentId: string) => void;
		compact?: boolean;
		selected?: boolean;
	}

	let { component, onclick, compact = false, selected = false }: ComponentNodeProps = $props();

	function handleClick() {
		onclick?.(component.id);
	}

	function handleKeyDown(event: KeyboardEvent) {
		if (event.key === 'Enter' || event.key === ' ') {
			event.preventDefault();
			handleClick();
		}
	}

	const displayName = $derived(component.component || component.id);
</script>

<div
	class="component-node"
	data-node-id={component.id}
	class:compact
	class:selected
	onclick={handleClick}
	onkeydown={handleKeyDown}
	role="button"
	tabindex="0"
>
	<div class="node-header">
		<div class="node-name">{displayName}</div>
		<div class="health-indicator {component.health.status}">
			{component.health.status}
		</div>
	</div>

	{#if !compact}
		<div class="node-body">
			{#if component.health.errorMessage}
				<div class="error-message" title={component.health.errorMessage}>
					⚠️
				</div>
			{/if}
		</div>
	{/if}
</div>

<style>
	.component-node {
		background: white;
		border: 2px solid var(--node-border, #0066cc);
		border-radius: 8px;
		padding: 0.75rem;
		min-width: 150px;
		cursor: pointer;
		transition: all 0.2s;
	}

	.component-node:hover {
		border-color: var(--node-border-hover, #0052a3);
		box-shadow: 0 4px 8px rgba(0, 0, 0, 0.15);
	}

	.component-node.selected {
		border-color: var(--node-border-selected, #0052a3);
		box-shadow: 0 0 0 2px rgba(0, 102, 204, 0.2);
	}

	.component-node.compact {
		padding: 0.5rem;
		min-width: 100px;
	}

	.node-header {
		display: flex;
		justify-content: space-between;
		align-items: center;
		gap: 0.5rem;
	}

	.node-name {
		font-weight: 600;
		font-size: 0.875rem;
		flex: 1;
	}

	.health-indicator {
		font-size: 0.625rem;
		padding: 0.125rem 0.5rem;
		border-radius: 10px;
		text-transform: uppercase;
		font-weight: 500;
	}

	.health-indicator.healthy {
		background: var(--health-healthy-bg, #d4edda);
		color: var(--health-healthy-text, #155724);
	}

	.health-indicator.degraded {
		background: var(--health-degraded-bg, #fff3cd);
		color: var(--health-degraded-text, #856404);
	}

	.health-indicator.unhealthy {
		background: var(--health-unhealthy-bg, #f8d7da);
		color: var(--health-unhealthy-text, #721c24);
	}

	.health-indicator.not_running {
		background: var(--health-not-running-bg, #e9ecef);
		color: var(--health-not-running-text, #495057);
	}

	.node-body {
		margin-top: 0.5rem;
	}

	.error-message {
		color: var(--error-color, #dc3545);
		font-size: 1rem;
	}
</style>
