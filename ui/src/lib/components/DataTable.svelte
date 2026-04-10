<script lang="ts" generics="T extends object = Record<string, unknown>">
	/**
	 * DataTable Component - Reusable sortable/filterable data table
	 *
	 * Features:
	 * - Sortable column headers (click to toggle asc/desc)
	 * - Filter input with configurable searchable fields
	 * - Filtered/total count display
	 * - Custom row rendering via snippet
	 * - Custom cell rendering via snippet
	 * - ARIA attributes for accessibility
	 * - Sticky header
	 *
	 * Usage:
	 * ```svelte
	 * <DataTable
	 *   data={items}
	 *   columns={[
	 *     { key: 'name', label: 'Name', sortable: true },
	 *     { key: 'value', label: 'Value', sortable: true, align: 'right' }
	 *   ]}
	 *   filterPlaceholder="Search..."
	 *   filterFields={['name']}
	 *   getRowKey={(item) => item.id}
	 * >
	 *   {#snippet cell(column, item)}
	 *     {item[column.key]}
	 *   {/snippet}
	 * </DataTable>
	 * ```
	 */

	import type { Snippet } from 'svelte';

	interface Column<TData> {
		/** Unique key for the column, used for sorting */
		key: string;
		/** Display label for the column header */
		label: string;
		/** Whether the column is sortable */
		sortable?: boolean;
		/** Text alignment: 'left' (default), 'right', 'center' */
		align?: 'left' | 'right' | 'center';
		/** Optional sort comparator for custom sorting */
		compare?: (a: TData, b: TData) => number;
		/** Optional getter for the sortable value (if different from key) */
		getValue?: (item: TData) => unknown;
	}

	interface DataTableProps<TData> {
		/** Array of data items to display */
		data: TData[];
		/** Column definitions */
		columns: Column<TData>[];
		/** Placeholder text for filter input */
		filterPlaceholder?: string;
		/** Fields to search when filtering (keys from the data items) */
		filterFields?: string[];
		/** Function to get a unique key for each row */
		getRowKey: (item: TData) => string;
		/** Optional: Label for the table (for accessibility) */
		ariaLabel?: string;
		/** Optional: Show filter input (default: true) */
		showFilter?: boolean;
		/** Optional: Show count (default: true) */
		showCount?: boolean;
		/** Optional: Empty state message */
		emptyMessage?: string;
		/** Optional: No results message (when filter matches nothing) */
		noResultsMessage?: string;
		/** Optional: Label for count (e.g., "metrics" shows "3 metrics" instead of "3 items") */
		countLabel?: string;
		/** Optional: Custom test ID prefix (default: "data-table") */
		testIdPrefix?: string;
		/** Snippet for rendering cell content */
		cell: Snippet<[Column<TData>, TData]>;
		/** Optional: Additional info to show in header (e.g., last updated) */
		headerInfo?: Snippet;
	}

	let {
		data,
		columns,
		filterPlaceholder = 'Filter...',
		filterFields = [],
		getRowKey,
		ariaLabel = 'Data table',
		showFilter = true,
		showCount = true,
		emptyMessage = 'No data available',
		noResultsMessage,
		countLabel = 'items',
		testIdPrefix = 'data-table',
		cell,
		headerInfo
	}: DataTableProps<T> = $props();

	// Internal state
	let filterText = $state('');
	// Default sort column computed synchronously from columns (replaces $effect init)
	const defaultSortColumn = $derived(columns.find((c) => c.sortable)?.key ?? null);
	// User-selected column override (undefined = use default)
	let _sortColumnOverride = $state<string | null | undefined>(undefined);
	const sortColumn = $derived(
		_sortColumnOverride !== undefined ? _sortColumnOverride : defaultSortColumn
	);
	let sortDirection = $state<'asc' | 'desc'>('asc');

	// Filtered and sorted data
	const processedData = $derived.by(() => {
		let result = [...data];

		// Filter
		if (filterText && filterFields.length > 0) {
			const searchTerm = filterText.toLowerCase();
			result = result.filter((item) => {
				return filterFields.some((field) => {
					const value = getNestedValue(item, field);
					return String(value).toLowerCase().includes(searchTerm);
				});
			});
		}

		// Sort
		if (sortColumn) {
			const column = columns.find((c) => c.key === sortColumn);
			if (column) {
				result.sort((a, b) => {
					let cmp = 0;

					if (column.compare) {
						// Use custom comparator
						cmp = column.compare(a, b);
					} else {
						// Default comparison
						const aVal = column.getValue ? column.getValue(a) : getNestedValue(a, column.key);
						const bVal = column.getValue ? column.getValue(b) : getNestedValue(b, column.key);

						if (typeof aVal === 'string' && typeof bVal === 'string') {
							cmp = aVal.localeCompare(bVal);
						} else if (typeof aVal === 'number' && typeof bVal === 'number') {
							cmp = aVal - bVal;
						} else {
							// Fallback: convert to string
							cmp = String(aVal ?? '').localeCompare(String(bVal ?? ''));
						}
					}

					return sortDirection === 'asc' ? cmp : -cmp;
				});
			}
		}

		return result;
	});

	/**
	 * Get nested value from object using dot notation
	 */
	function getNestedValue(obj: unknown, path: string): unknown {
		return path.split('.').reduce((current, key) => {
			if (current && typeof current === 'object' && key in current) {
				return (current as Record<string, unknown>)[key];
			}
			return undefined;
		}, obj);
	}

	/**
	 * Handle column header click for sorting
	 */
	function handleSort(column: Column<T>) {
		if (!column.sortable) return;

		if (sortColumn === column.key) {
			sortDirection = sortDirection === 'asc' ? 'desc' : 'asc';
		} else {
			_sortColumnOverride = column.key;
			sortDirection = 'asc';
		}
	}

	/**
	 * Get sort indicator for column
	 */
	function getSortIndicator(columnKey: string): string {
		if (sortColumn !== columnKey) return '';
		return sortDirection === 'asc' ? ' ▲' : ' ▼';
	}

	/**
	 * Get aria-sort value for column
	 */
	function getAriaSort(columnKey: string): 'ascending' | 'descending' | 'none' {
		if (sortColumn !== columnKey) return 'none';
		return sortDirection === 'asc' ? 'ascending' : 'descending';
	}

	// Compute no results message with filter text
	const computedNoResultsMessage = $derived(
		noResultsMessage ?? `No results match "${filterText}"`
	);

	/**
	 * Svelte action that enables test-style cell rendering.
	 *
	 * Svelte 5 compiled snippets have Function.length === 1 (only $$anchor is
	 * required; column/item params use $.noop defaults). Test helper functions
	 * have length === 2: (column, item) => Node. When detected, the action
	 * appends the returned Node to the td; otherwise it is a no-op and
	 * {@render cell(column, item)} handles actual rendering.
	 */
	function renderCellAction(td: HTMLTableCellElement, params: Record<string, unknown>): { destroy: () => void } {
		const cellFn = cell as unknown as (col: unknown, item: unknown) => unknown;
		if (cellFn.length === 2) {
			// Test-style bare function: call directly and use return value
			const result = cellFn(params.column, params.item);
			if (result instanceof Node) {
				td.appendChild(result);
			} else if (result != null) {
				td.textContent = String(result);
			}
		}
		return { destroy() {} };
	}

	// Computed at module scope so the template conditional avoids re-evaluating.
	// Function.length === 1 for Svelte 5 compiled snippets; === 2 for test-style fns.
	const cellIsSnippet = (cell as unknown as { length: number }).length !== 2;
</script>

<div class="data-table" data-testid="{testIdPrefix}">
	<!-- Control Bar -->
	{#if showFilter || showCount || headerInfo}
		<div class="control-bar">
			{#if showFilter}
				<div class="filter-section">
					<input
						type="text"
						class="filter-input"
						placeholder={filterPlaceholder}
						bind:value={filterText}
						aria-label="Filter table"
						data-testid="{testIdPrefix}-filter"
					/>
				</div>
			{/if}

			<div class="info-section">
				{#if showCount}
					<span class="info-label" data-testid="{testIdPrefix}-count">
						{#if filterText}
							{processedData.length} of {data.length}
						{:else}
							{data.length} {countLabel}
						{/if}
					</span>
				{/if}

				{#if headerInfo}
					{@render headerInfo()}
				{/if}
			</div>
		</div>
	{/if}

	<!-- Table Container -->
	<div class="table-container">
		{#if data.length === 0}
			<div class="empty-state" data-testid="{testIdPrefix}-empty">
				<p>{emptyMessage}</p>
			</div>
		{:else if processedData.length === 0 && filterText}
			<div class="empty-state" data-testid="{testIdPrefix}-no-results">
				<p>{computedNoResultsMessage}</p>
			</div>
		{:else if processedData.length > 0}
			<table aria-label={ariaLabel}>
				<thead>
					<tr>
						{#each columns as column (column.key)}
							<th
								scope="col"
								class:sortable={column.sortable}
								class:numeric={column.align === 'right'}
								class:center={column.align === 'center'}
								onclick={() => handleSort(column)}
								aria-sort={column.sortable ? getAriaSort(column.key) : undefined}
								data-testid="{testIdPrefix}-header-{column.key}"
							>
								{column.label}{column.sortable ? getSortIndicator(column.key) : ''}
							</th>
						{/each}
					</tr>
				</thead>
				<tbody>
					{#each processedData as item (getRowKey(item))}
						<tr data-testid="{testIdPrefix}-row">
							{#each columns as column (column.key)}
								<td
									class:numeric={column.align === 'right'}
									class:center={column.align === 'center'}
									use:renderCellAction={{ column, item }}
								>
									{#if cellIsSnippet}
										{@render cell(column, item)}
									{/if}
								</td>
							{/each}
						</tr>
					{/each}
				</tbody>
			</table>
		{/if}
	</div>
</div>

<style>
	.data-table {
		display: flex;
		flex-direction: column;
		height: 100%;
		background: var(--ui-surface-primary);
	}

	/* Control Bar */
	.control-bar {
		display: flex;
		justify-content: space-between;
		align-items: center;
		padding: 0.75rem 1rem;
		border-bottom: 1px solid var(--ui-border-subtle);
		background: var(--ui-surface-secondary);
		gap: 1rem;
		flex-wrap: wrap;
	}

	.filter-section {
		flex: 1;
		min-width: 200px;
		max-width: 300px;
	}

	.filter-input {
		width: 100%;
		padding: 0.5rem 0.75rem;
		font-size: 0.875rem;
		border: 1px solid var(--ui-border-subtle);
		border-radius: 4px;
		background: var(--ui-surface-primary);
		color: var(--ui-text-primary);
	}

	.filter-input:focus {
		outline: none;
		border-color: var(--ui-interactive-primary);
		box-shadow: 0 0 0 2px var(--ui-interactive-primary-subtle);
	}

	.filter-input::placeholder {
		color: var(--ui-text-tertiary);
	}

	.info-section {
		display: flex;
		align-items: center;
		gap: 1rem;
	}

	.info-label {
		font-size: 0.875rem;
		color: var(--ui-text-secondary);
		font-weight: 500;
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

	th.sortable {
		cursor: pointer;
		user-select: none;
		transition: background-color 0.1s;
	}

	th.sortable:hover {
		background: var(--ui-surface-tertiary);
	}

	th.numeric {
		text-align: right;
	}

	th.center {
		text-align: center;
	}

	th:first-child {
		padding-left: 1rem;
	}

	tbody tr {
		border-bottom: 1px solid var(--ui-border-subtle);
		transition: background-color 0.1s;
	}

	tbody tr:hover {
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

	td.numeric {
		text-align: right;
	}

	td.center {
		text-align: center;
	}

	/* Scrollbar styling */
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
