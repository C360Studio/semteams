<script lang="ts">
	/**
	 * GraphFilters - Filter controls for knowledge graph visualization
	 *
	 * Provides controls for:
	 * - Entity search
	 * - Type/domain filtering
	 * - Confidence threshold
	 * - Time range selection
	 */

	import type { GraphFilters } from '$lib/types/graph';

	interface GraphFiltersProps {
		filters: GraphFilters;
		availableTypes: string[];
		availableDomains: string[];
		onFilterChange: (filters: Partial<GraphFilters>) => void;
		onReset: () => void;
	}

	let {
		filters,
		availableTypes = [],
		availableDomains = [],
		onFilterChange,
		onReset
	}: GraphFiltersProps = $props();

	// Local state for inputs
	let searchInput = $state(filters.search);

	// Debounced search
	let searchTimeout: ReturnType<typeof setTimeout> | null = null;

	function handleSearchInput(event: Event) {
		const value = (event.target as HTMLInputElement).value;
		searchInput = value;

		if (searchTimeout) clearTimeout(searchTimeout);
		searchTimeout = setTimeout(() => {
			onFilterChange({ search: value });
		}, 300);
	}

	function handleTypeToggle(type: string) {
		const newTypes = filters.types.includes(type)
			? filters.types.filter((t) => t !== type)
			: [...filters.types, type];
		onFilterChange({ types: newTypes });
	}

	function handleDomainToggle(domain: string) {
		const newDomains = filters.domains.includes(domain)
			? filters.domains.filter((d) => d !== domain)
			: [...filters.domains, domain];
		onFilterChange({ domains: newDomains });
	}

	function handleConfidenceChange(event: Event) {
		const value = parseFloat((event.target as HTMLInputElement).value);
		onFilterChange({ minConfidence: value });
	}

	// Computed: are any filters active?
	const hasActiveFilters = $derived(
		filters.search !== '' ||
			filters.types.length > 0 ||
			filters.domains.length > 0 ||
			filters.minConfidence > 0 ||
			filters.timeRange !== null
	);
</script>

<div class="graph-filters" data-testid="graph-filters">
	<!-- Search -->
	<div class="filter-section">
		<label for="entity-search" class="filter-label">Search</label>
		<input
			type="search"
			id="entity-search"
			class="filter-input"
			placeholder="Entity ID or name..."
			value={searchInput}
			oninput={handleSearchInput}
			data-testid="entity-search"
		/>
	</div>

	<!-- Confidence slider -->
	<div class="filter-section">
		<label for="confidence-slider" class="filter-label">
			Min Confidence: {(filters.minConfidence * 100).toFixed(0)}%
		</label>
		<input
			type="range"
			id="confidence-slider"
			class="filter-slider"
			min="0"
			max="1"
			step="0.1"
			value={filters.minConfidence}
			oninput={handleConfidenceChange}
			data-testid="confidence-slider"
		/>
	</div>

	<!-- Type filters -->
	{#if availableTypes.length > 0}
		<div class="filter-section">
			<span class="filter-label">Types</span>
			<div class="filter-chips">
				{#each availableTypes as type (type)}
					<button
						class="filter-chip"
						class:active={filters.types.includes(type)}
						onclick={() => handleTypeToggle(type)}
						data-testid="type-filter-{type}"
					>
						{type}
					</button>
				{/each}
			</div>
		</div>
	{/if}

	<!-- Domain filters -->
	{#if availableDomains.length > 0}
		<div class="filter-section">
			<span class="filter-label">Domains</span>
			<div class="filter-chips">
				{#each availableDomains as domain (domain)}
					<button
						class="filter-chip"
						class:active={filters.domains.includes(domain)}
						onclick={() => handleDomainToggle(domain)}
						data-testid="domain-filter-{domain}"
					>
						{domain}
					</button>
				{/each}
			</div>
		</div>
	{/if}

	<!-- Reset button -->
	{#if hasActiveFilters}
		<button class="reset-button" onclick={onReset} data-testid="reset-filters">
			Clear Filters
		</button>
	{/if}
</div>

<style>
	.graph-filters {
		display: flex;
		flex-direction: column;
		gap: 12px;
		padding: 12px;
		background: var(--ui-surface-secondary);
		border-right: 1px solid var(--ui-border-subtle);
		min-width: 200px;
		max-width: 250px;
		overflow-y: auto;
	}

	.filter-section {
		display: flex;
		flex-direction: column;
		gap: 6px;
	}

	.filter-label {
		font-size: 11px;
		font-weight: 600;
		color: var(--ui-text-secondary);
		text-transform: uppercase;
		letter-spacing: 0.5px;
	}

	.filter-input {
		padding: 6px 10px;
		border: 1px solid var(--ui-border-subtle);
		border-radius: 4px;
		background: var(--ui-surface-primary);
		color: var(--ui-text-primary);
		font-size: 13px;
	}

	.filter-input:focus {
		outline: none;
		border-color: var(--ui-interactive-primary);
		box-shadow: 0 0 0 2px rgba(var(--ui-interactive-primary-rgb), 0.1);
	}

	.filter-slider {
		width: 100%;
		height: 4px;
		border-radius: 2px;
		background: var(--ui-border-subtle);
		appearance: none;
		cursor: pointer;
	}

	.filter-slider::-webkit-slider-thumb {
		appearance: none;
		width: 14px;
		height: 14px;
		border-radius: 50%;
		background: var(--ui-interactive-primary);
		cursor: pointer;
	}

	.filter-chips {
		display: flex;
		flex-wrap: wrap;
		gap: 4px;
	}

	.filter-chip {
		padding: 4px 8px;
		border: 1px solid var(--ui-border-subtle);
		border-radius: 12px;
		background: var(--ui-surface-primary);
		color: var(--ui-text-secondary);
		font-size: 11px;
		cursor: pointer;
		transition: all 0.2s;
	}

	.filter-chip:hover {
		border-color: var(--ui-border-strong);
		color: var(--ui-text-primary);
	}

	.filter-chip.active {
		background: var(--ui-interactive-primary);
		border-color: var(--ui-interactive-primary);
		color: white;
	}

	.reset-button {
		padding: 6px 12px;
		border: 1px solid var(--ui-border-subtle);
		border-radius: 4px;
		background: var(--ui-surface-primary);
		color: var(--ui-text-secondary);
		font-size: 12px;
		cursor: pointer;
		transition: all 0.2s;
	}

	.reset-button:hover {
		background: var(--ui-surface-tertiary);
		color: var(--ui-text-primary);
	}
</style>
