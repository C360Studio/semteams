<script lang="ts">
	/**
	 * MetricsTab Component - Real-time metrics display
	 * Uses runtimeStore for WebSocket-driven data
	 *
	 * Features:
	 * - Display ALL metrics received from backend
	 * - Show raw values and computed rates for counters
	 * - Filter by component or metric name
	 * - Sortable columns
	 * - Data persists across tab switches (no polling needed)
	 */

	import { runtimeStore } from '$lib/stores/runtimeStore.svelte';
	import DataTable from '$lib/components/DataTable.svelte';

	interface MetricsTabProps {
		flowId: string;
		isActive: boolean;
	}

	// Metric row type from store
	type MetricRow = ReturnType<typeof runtimeStore.getMetricsArray>[number];

	// Column definition type
	interface Column {
		key: string;
		label: string;
		sortable?: boolean;
		align?: 'left' | 'right' | 'center';
		getValue?: (item: MetricRow) => unknown;
	}

	// Props passed from parent - may be used for future tab-specific logic
	let { flowId: _flowId, isActive: _isActive }: MetricsTabProps = $props();

	// Get metrics array from store helper - shows all metrics
	const metricsArray = $derived(runtimeStore.getMetricsArray());

	// Last updated timestamp
	const lastUpdated = $derived(
		runtimeStore.lastMetricsTimestamp ? new Date(runtimeStore.lastMetricsTimestamp) : null
	);

	// Column definitions for the metrics table
	const columns: Column[] = [
		{ key: 'component', label: 'Component', sortable: true },
		{ key: 'metricName', label: 'Metric', sortable: true },
		{
			key: 'value',
			label: 'Value',
			sortable: true,
			align: 'right',
			getValue: (item: MetricRow) => item.raw?.value ?? 0
		},
		{
			key: 'rate',
			label: 'Rate/sec',
			sortable: true,
			align: 'right',
			getValue: (item: MetricRow) => item.rate ?? -1
		}
	];

	/**
	 * Get unique key for each metric row
	 */
	function getRowKey(item: MetricRow): string {
		return `${item.component}:${item.metricName}`;
	}

	/**
	 * Format number with commas (1234 -> "1,234")
	 */
	function formatNumber(num: number): string {
		return num.toLocaleString('en-US', { maximumFractionDigits: 2 });
	}

	/**
	 * Format rate (per second), null means not yet calculated
	 */
	function formatRate(rate: number | null): string {
		if (rate === null) return '-';
		if (rate === 0) return '0';
		if (rate < 0.01) return '<0.01';
		return rate.toLocaleString('en-US', { maximumFractionDigits: 2 });
	}

	/**
	 * Format timestamp (Date -> "14:23:05")
	 */
	function formatTime(date: Date): string {
		return date.toLocaleTimeString('en-US', {
			hour: '2-digit',
			minute: '2-digit',
			second: '2-digit'
		});
	}

	/**
	 * Shorten metric name for display (remove common prefixes)
	 */
	function shortenMetricName(name: string): string {
		// Remove common prefixes like "semstreams_"
		return name.replace(/^semstreams_/, '');
	}
</script>

<div class="metrics-tab" data-testid="metrics-tab">
	<!-- Connection Status -->
	{#if runtimeStore.error}
		<div class="error-message" role="alert">
			<span class="error-icon">⚠</span>
			<span>{runtimeStore.error}</span>
		</div>
	{:else if !runtimeStore.connected}
		<div class="connecting-message">
			<span class="connecting-icon">⋯</span>
			<span>Connecting to runtime stream...</span>
		</div>
	{/if}

	<!-- Metrics Table -->
	<DataTable
		data={metricsArray}
		{columns}
		filterPlaceholder="Filter by component or metric..."
		filterFields={['component', 'metricName']}
		{getRowKey}
		ariaLabel="Component metrics"
		emptyMessage="No metrics available"
		showFilter={true}
		showCount={true}
		countLabel="metrics"
		testIdPrefix="metrics"
		noResultsMessage="No metrics match"
	>
		{#snippet headerInfo()}
			{#if lastUpdated}
				<span class="last-updated" data-testid="last-updated">
					Last: {formatTime(lastUpdated)}
				</span>
			{/if}
		{/snippet}

		{#snippet cell(column, item)}
			{#if column.key === 'component'}
				<span class="component-name">{item.component}</span>
			{:else if column.key === 'metricName'}
				<span class="metric-name" title={item.metricName}>
					{shortenMetricName(item.metricName)}
				</span>
			{:else if column.key === 'value'}
				<span class="metric-value">{formatNumber(item.raw?.value ?? 0)}</span>
			{:else if column.key === 'rate'}
				<span class="metric-rate">{formatRate(item.rate)}</span>
			{/if}
		{/snippet}
	</DataTable>
</div>

<style>
	.metrics-tab {
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

	/* Last updated (in DataTable header) */
	.last-updated {
		font-size: 0.875rem;
		color: var(--ui-text-tertiary);
	}

	/* Cell styling */
	.component-name {
		font-weight: 500;
		color: var(--ui-text-primary);
		white-space: nowrap;
	}

	.metric-name {
		font-family: 'Monaco', 'Menlo', 'Ubuntu Mono', 'Consolas', monospace;
		font-size: 0.8125rem;
		color: var(--ui-text-secondary);
		max-width: 250px;
		overflow: hidden;
		text-overflow: ellipsis;
		white-space: nowrap;
		display: block;
	}

	.metric-value,
	.metric-rate {
		font-family: 'Monaco', 'Menlo', 'Ubuntu Mono', 'Consolas', monospace;
		font-size: 0.875rem;
		color: var(--ui-text-secondary);
		white-space: nowrap;
	}
</style>
