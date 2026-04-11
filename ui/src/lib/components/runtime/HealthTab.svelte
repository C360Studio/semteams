<script lang="ts">
	import { SvelteSet } from 'svelte/reactivity';
	/**
	 * HealthTab Component - Component health monitoring
	 * Uses runtimeStore for WebSocket-driven data
	 *
	 * Features:
	 * - Display component status (healthy, degraded, error) from runtimeStore
	 * - Connection health summary (X/Y components healthy)
	 * - Expandable details for components with messages
	 * - Status indicators using design system colors
	 * - Data persists across tab switches (no polling needed)
	 */

	import {
		runtimeStore,
		type ComponentHealth
	} from '$lib/stores/runtimeStore.svelte';

	interface HealthTabProps {
		flowId: string;
		isActive: boolean;
	}

	// Props passed from parent - may be used for future tab-specific logic
	let { flowId: _flowId, isActive: _isActive }: HealthTabProps = $props();

	// Local UI state
	let expandedComponents = new SvelteSet<string>();

	// Sorted components (alphabetically by name)
	const sortedComponents = $derived(
		runtimeStore.healthComponents.slice().sort((a, b) => a.name.localeCompare(b.name))
	);

	// Check if component is expanded
	function isExpanded(name: string): boolean {
		return expandedComponents.has(name);
	}

	/**
	 * Get status color from design system
	 */
	function getStatusColor(status: ComponentHealth['status']): string {
		const colors = {
			healthy: 'var(--status-success)',
			degraded: 'var(--status-warning)',
			error: 'var(--status-error)'
		};
		return colors[status];
	}

	/**
	 * Get status icon for visual display
	 */
	function getStatusIcon(status: ComponentHealth['status']): string {
		const icons = {
			healthy: '●',
			degraded: '⚠',
			error: '●'
		};
		return icons[status];
	}

	/**
	 * Get status label for accessibility
	 */
	function getStatusLabel(status: ComponentHealth['status']): string {
		const labels = {
			healthy: 'Healthy - Component is operating normally',
			degraded: 'Degraded - Component has warnings',
			error: 'Error - Component has critical issues'
		};
		return labels[status];
	}

	/**
	 * Get overall health status color
	 */
	function getOverallStatusColor(status: ComponentHealth['status']): string {
		const colors = {
			healthy: 'var(--status-success)',
			degraded: 'var(--status-warning)',
			error: 'var(--status-error)'
		};
		return colors[status];
	}

	/**
	 * Get overall health status icon
	 */
	function getOverallStatusIcon(status: ComponentHealth['status']): string {
		const icons = {
			healthy: '🟢',
			degraded: '🟡',
			error: '🔴'
		};
		return icons[status];
	}

	/**
	 * Toggle component details expansion
	 */
	function toggleDetails(componentName: string) {
		if (expandedComponents.has(componentName)) {
			expandedComponents.delete(componentName);
		} else {
			expandedComponents.add(componentName);
		}
	}
</script>

<div class="health-tab" data-testid="health-tab">
	<!-- Connection Status -->
	{#if runtimeStore.error}
		<div class="error-message" role="alert" data-testid="health-error">
			<span class="error-icon">⚠</span>
			<span>{runtimeStore.error}</span>
		</div>
	{:else if !runtimeStore.connected}
		<div class="connecting-message">
			<span class="connecting-icon">⋯</span>
			<span>Connecting to runtime stream...</span>
		</div>
	{/if}

	<!-- Health Summary -->
	{#if runtimeStore.healthOverall}
		<div class="health-summary" data-testid="health-summary">
			<span
				class="overall-status"
				style="color: {getOverallStatusColor(runtimeStore.healthOverall.status)}"
				aria-label="Overall system health: {runtimeStore.healthOverall.status}"
			>
				<span class="status-icon">{getOverallStatusIcon(runtimeStore.healthOverall.status)}</span>
				<span class="status-text">System Health:</span>
				<span class="health-count">
					{runtimeStore.healthOverall.counts.healthy}/{runtimeStore.healthOverall.counts.healthy +
						runtimeStore.healthOverall.counts.degraded +
						runtimeStore.healthOverall.counts.error} components healthy
				</span>
			</span>
		</div>
	{/if}

	<!-- Health Table -->
	<div class="table-container">
		{#if sortedComponents.length === 0 && !runtimeStore.error}
			<div class="empty-state">
				<p>No health data available</p>
			</div>
		{:else if sortedComponents.length > 0}
			<table aria-label="Component health status">
				<thead>
					<tr>
						<th scope="col" class="col-component">Component</th>
						<th scope="col" class="col-type">Type</th>
						<th scope="col" class="col-status">Status</th>
					</tr>
				</thead>
				<tbody>
					{#each sortedComponents as component (component.name)}
						<tr data-testid="health-row" class:has-details={component.message !== null}>
							<td class="component-name">
								{#if component.message}
									<button
										class="expand-button"
										onclick={() => toggleDetails(component.name)}
										aria-expanded={isExpanded(component.name)}
										aria-label={isExpanded(component.name)
											? `Collapse details for ${component.name}`
											: `Expand details for ${component.name}`}
										data-testid="expand-button"
									>
										{isExpanded(component.name) ? '▼' : '▶'}
									</button>
								{/if}
								<span class="name-text">{component.name}</span>
							</td>
							<td class="type-cell">
								<span class="type-label">{component.type}</span>
							</td>
							<td class="status-cell">
								<span
									class="status-indicator"
									style="color: {getStatusColor(component.status)}"
									aria-label={getStatusLabel(component.status)}
									data-testid="status-indicator"
								>
									{getStatusIcon(component.status)}
								</span>
								<span class="status-label">{component.status}</span>
							</td>
						</tr>

						<!-- Expandable Details Row -->
						{#if component.message && isExpanded(component.name)}
							<tr class="details-row" data-testid="details-row">
								<td colspan="3">
									<div class="details-content">
										<div class="detail-message" class:is-error={component.status === 'error'}>
											<span class="message-label"
												>{component.status === 'error' ? 'ERROR' : 'WARNING'}:</span
											>
											<span class="message-text">{component.message}</span>
										</div>
									</div>
								</td>
							</tr>
						{/if}
					{/each}
				</tbody>
			</table>
		{/if}
	</div>
</div>

<style>
	.health-tab {
		display: flex;
		flex-direction: column;
		height: 100%;
		background: var(--ui-surface-primary);
	}

	/* Error/Connecting Messages */
	.error-message,
	.connecting-message {
		display: flex;
		align-items: center;
		gap: 0.5rem;
		padding: 0.5rem 1rem;
		font-size: 0.875rem;
		border-bottom: 1px solid var(--ui-border-subtle);
	}

	.error-message {
		background: var(--status-error-container);
		color: var(--status-error-on-container);
	}

	.connecting-message {
		background: var(--status-info-container);
		color: var(--status-info-on-container);
	}

	.error-icon,
	.connecting-icon {
		font-size: 1rem;
	}

	/* Health Summary */
	.health-summary {
		display: flex;
		align-items: center;
		padding: 0.75rem 1rem;
		border-bottom: 1px solid var(--ui-border-subtle);
		background: var(--ui-surface-secondary);
		gap: 0.5rem;
	}

	.overall-status {
		display: flex;
		align-items: center;
		gap: 0.5rem;
		font-size: 0.875rem;
		font-weight: 600;
	}

	.status-icon {
		font-size: 1rem;
		line-height: 1;
	}

	.status-text {
		color: var(--ui-text-primary);
	}

	.health-count {
		font-weight: 500;
		color: var(--ui-text-secondary);
	}

	/* Table Container */
	.table-container {
		flex: 1;
		overflow-y: auto;
		overflow-x: auto;
		background: var(--ui-surface-primary);
	}

	.empty-state {
		display: flex;
		align-items: center;
		justify-content: center;
		height: 100%;
		min-height: 150px;
	}

	.empty-state p {
		margin: 0;
		color: var(--ui-text-secondary);
		font-size: 0.875rem;
	}

	/* Table Styles */
	table {
		width: 100%;
		border-collapse: collapse;
		font-size: 0.875rem;
	}

	thead {
		position: sticky;
		top: 0;
		background: var(--ui-surface-secondary);
		z-index: 1;
		border-bottom: 2px solid var(--ui-border-strong);
	}

	th {
		text-align: left;
		padding: 0.75rem 1rem;
		font-weight: 600;
		color: var(--ui-text-primary);
		white-space: nowrap;
	}

	th:first-child {
		padding-left: 1rem;
	}

	.col-component {
		width: 50%;
	}

	.col-type {
		width: 25%;
	}

	.col-status {
		width: 25%;
	}

	tbody tr {
		border-bottom: 1px solid var(--ui-border-subtle);
		transition: background-color 0.1s;
	}

	tbody tr:not(.details-row):hover {
		background: var(--ui-surface-secondary);
	}

	tbody tr:last-child {
		border-bottom: none;
	}

	td {
		padding: 0.75rem 1rem;
		color: var(--ui-text-primary);
	}

	td:first-child {
		padding-left: 1rem;
	}

	/* Component Name Column */
	.component-name {
		display: flex;
		align-items: center;
		gap: 0.5rem;
		font-weight: 500;
	}

	.expand-button {
		background: none;
		border: none;
		cursor: pointer;
		padding: 0.25rem 0.5rem;
		color: var(--ui-text-secondary);
		font-size: 0.75rem;
		line-height: 1;
		transition: all 0.2s;
		border-radius: 4px;
	}

	.expand-button:hover {
		background: var(--ui-surface-tertiary);
		color: var(--ui-text-primary);
	}

	.expand-button:focus-visible {
		outline: 2px solid var(--ui-focus-ring);
		outline-offset: 2px;
	}

	.name-text {
		color: var(--ui-text-primary);
		white-space: nowrap;
	}

	/* Type Column */
	.type-cell {
		color: var(--ui-text-secondary);
	}

	.type-label {
		text-transform: capitalize;
		font-weight: 500;
	}

	/* Status Column */
	.status-cell {
		display: flex;
		align-items: center;
		gap: 0.5rem;
	}

	.status-indicator {
		font-size: 1rem;
		line-height: 1;
		display: inline-block;
	}

	.status-label {
		color: var(--ui-text-secondary);
		text-transform: capitalize;
		font-weight: 500;
	}

	/* Details Row */
	.details-row {
		background: var(--ui-surface-tertiary);
		border-bottom: 1px solid var(--ui-border-subtle);
	}

	.details-row td {
		padding: 0.75rem 1rem;
	}

	.details-content {
		display: flex;
		flex-direction: column;
		gap: 0.5rem;
		padding: 0.25rem 0;
		padding-left: 2rem; /* Indent from expand button */
		font-size: 0.875rem;
	}

	.detail-message {
		display: flex;
		gap: 0.5rem;
		align-items: baseline;
	}

	.message-label {
		font-weight: 700;
		color: var(--status-warning);
		text-transform: uppercase;
		font-size: 0.75rem;
		letter-spacing: 0.5px;
	}

	.detail-message.is-error .message-label {
		color: var(--status-error);
	}

	.message-text {
		color: var(--ui-text-primary);
		font-weight: 500;
	}

	/* Scrollbar styling (optional, for better UX) */
	.table-container::-webkit-scrollbar {
		width: 8px;
		height: 8px;
	}

	.table-container::-webkit-scrollbar-track {
		background: var(--ui-surface-secondary);
	}

	.table-container::-webkit-scrollbar-thumb {
		background: var(--ui-border-strong);
		border-radius: 4px;
	}

	.table-container::-webkit-scrollbar-thumb:hover {
		background: var(--ui-interactive-secondary);
	}
</style>
