<script lang="ts">
	/**
	 * PropertiesPanel - Context-aware right panel
	 *
	 * Modes:
	 * - Empty: Nothing selected, show placeholder
	 * - Type Preview: Hovering in Palette, show component type info
	 * - Edit: Component selected, show editable config form
	 */

	import type { FlowNode } from '$lib/types/flow';
	import type { ComponentType } from '$lib/types/component';
	import type { PropertySchema } from '$lib/types/schema';
	import type { PropertiesPanelMode } from '$lib/types/ui-state';
	import type { ConfigValue } from '$lib/types/config';
	import { getTypeColor } from '$lib/utils/category-colors';
	import { humanizeFieldName } from '$lib/utils/humanize';
	import SchemaField from './SchemaField.svelte';

	interface PropertiesPanelProps {
		/** Panel display mode */
		mode?: PropertiesPanelMode;
		/** Component type for preview mode */
		componentType?: ComponentType | null;
		/** Flow node for edit mode */
		node?: FlowNode | null;
		/** Component type definition for the node (for schema) */
		nodeComponentType?: ComponentType | null;
		/** Callback when config is saved */
		onSave?: (nodeId: string, name: string, config: Record<string, ConfigValue>) => void;
		/** Callback when node is deleted */
		onDelete?: (nodeId: string) => void;
	}

	let {
		mode = 'empty',
		componentType = null,
		node = null,
		nodeComponentType = null,
		onSave,
		onDelete
	}: PropertiesPanelProps = $props();

	// Edit form state
	let editedName = $state('');
	let editedConfig = $state<Record<string, ConfigValue>>({});
	let showDeleteConfirm = $state(false);
	let searchQuery = $state('');

	// Initialize form when node changes
	$effect(() => {
		if (node) {
			editedName = node.name;
			editedConfig = { ...(node.config as Record<string, ConfigValue>) };
		} else {
			editedName = '';
			editedConfig = {};
		}
		showDeleteConfirm = false;
		searchQuery = '';
	});

	// Categorize and filter fields - cast to PropertySchema (schema.ts) for type compatibility
	const allConfigFields = $derived.by(() => {
		if (!nodeComponentType?.schema) return [] as [string, PropertySchema][];
		return Object.entries(nodeComponentType.schema.properties) as [string, PropertySchema][];
	});

	const basicFields = $derived.by(() => {
		return allConfigFields
			.filter(([_, schema]) => schema.category === 'basic')
			.sort(([a], [b]) => a.localeCompare(b));
	});

	const advancedFields = $derived.by(() => {
		return allConfigFields
			.filter(([_, schema]) => schema.category !== 'basic')
			.sort(([a], [b]) => a.localeCompare(b));
	});

	const filteredFields = $derived.by(() => {
		if (!searchQuery.trim()) return [] as [string, PropertySchema][];
		const query = searchQuery.toLowerCase();
		return allConfigFields
			.filter(([fieldName, schema]) => {
				const nameMatch = fieldName.toLowerCase().includes(query);
				const descMatch = schema.description?.toLowerCase().includes(query) ?? false;
				return nameMatch || descMatch;
			})
			.sort(([a], [b]) => a.localeCompare(b));
	});

	const isSearching = $derived(searchQuery.trim().length > 0);
	const hasSearchResults = $derived(filteredFields.length > 0);

	// Dirty state - check if form has changes
	const isDirty = $derived.by(() => {
		if (!node) return false;
		if (editedName !== node.name) return true;

		const originalConfig = node.config;
		const editedKeys = Object.keys(editedConfig);
		const originalKeys = Object.keys(originalConfig);

		if (editedKeys.length !== originalKeys.length) return true;

		for (const key of editedKeys) {
			if (editedConfig[key] !== originalConfig[key]) return true;
		}

		return false;
	});

	// Validation
	const isFormValid = $derived.by(() => {
		if (!editedName.trim()) return false;

		if (nodeComponentType?.schema) {
			const required = nodeComponentType.schema.required || [];
			for (const field of required) {
				const value = editedConfig[field];
				if (value === undefined || value === null || value === '') return false;

				const schema = nodeComponentType.schema.properties[field];
				if (schema?.type === 'int' || schema?.type === 'float') {
					const numValue = Number(value);
					if (isNaN(numValue)) return false;
					if (schema.minimum !== undefined && numValue < schema.minimum) return false;
					if (schema.maximum !== undefined && numValue > schema.maximum) return false;
				}
			}
		}

		return true;
	});

	const canSave = $derived(isFormValid && isDirty);

	// Get validation error for a field
	function getFieldError(fieldName: string, schema: PropertySchema): string | undefined {
		const value = editedConfig[fieldName];
		if (value === undefined || value === null || value === '') return undefined;

		if (schema.type === 'int' || schema.type === 'float') {
			const numValue = Number(value);
			if (isNaN(numValue)) return 'Must be a valid number';
			if (schema.minimum !== undefined && numValue < schema.minimum) {
				return `Minimum: ${schema.minimum}`;
			}
			if (schema.maximum !== undefined && numValue > schema.maximum) {
				return `Maximum: ${schema.maximum}`;
			}
		}

		return undefined;
	}

	// Handlers
	function handleSave() {
		if (!canSave || !node) return;
		onSave?.(node.id, editedName, editedConfig);
	}

	function handleDelete() {
		showDeleteConfirm = true;
	}

	function handleConfirmDelete() {
		if (!node) return;
		onDelete?.(node.id);
		showDeleteConfirm = false;
	}

	function handleCancelDelete() {
		showDeleteConfirm = false;
	}

	function handleReset() {
		if (node) {
			editedName = node.name;
			editedConfig = { ...(node.config as Record<string, ConfigValue>) };
		}
	}

	// Auto-save on blur (if valid and dirty)
	function handleBlur() {
		if (canSave && node) {
			onSave?.(node.id, editedName, editedConfig);
		}
	}

	function handleClearSearch() {
		searchQuery = '';
	}

	function handleFieldChange(fieldName: string, value: ConfigValue) {
		editedConfig[fieldName] = value;
		handleBlur();
	}
</script>

<div class="properties-panel" data-testid="properties-panel">
	{#if mode === 'empty'}
		<!-- Empty State -->
		<div class="empty-state" data-testid="properties-empty">
			<div class="empty-icon">
				<svg width="48" height="48" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.5">
					<rect x="3" y="3" width="18" height="18" rx="2" />
					<line x1="9" y1="9" x2="15" y2="15" />
					<line x1="15" y1="9" x2="9" y2="15" />
				</svg>
			</div>
			<p class="empty-title">No Selection</p>
			<p class="empty-hint">Select a component from the canvas or list to view its properties</p>
		</div>

	{:else if mode === 'type-preview' && componentType}
		<!-- Type Preview -->
		{@const categoryColor = getTypeColor(componentType.type)}
		<div class="type-preview" data-testid="properties-type-preview">
			<header class="preview-header" style="border-left-color: {categoryColor};">
				<h3>{componentType.name}</h3>
				<span class="preview-category">{componentType.type}</span>
			</header>

			<div class="preview-content">
				<section class="preview-section">
					<h4>Description</h4>
					<p>{componentType.description || 'No description available'}</p>
				</section>

				{#if componentType.protocol}
					<section class="preview-section">
						<h4>Protocol</h4>
						<code class="protocol-badge">{componentType.protocol}</code>
					</section>
				{/if}

				{#if componentType.ports && componentType.ports.length > 0}
					<section class="preview-section">
						<h4>Ports</h4>
						<ul class="ports-list">
							{#each componentType.ports as port (port.id)}
								<li class="port-item">
									<span class="port-direction" class:input={port.direction === 'input'} class:output={port.direction === 'output'}>
										{port.direction}
									</span>
									<span class="port-name">{port.name}</span>
									{#if port.required}
										<span class="port-required">required</span>
									{/if}
								</li>
							{/each}
						</ul>
					</section>
				{/if}

				{#if componentType.schema}
					<section class="preview-section">
						<h4>Configuration</h4>
						<ul class="config-schema-list">
							{#each Object.entries(componentType.schema.properties) as [key, schema] (key)}
								{@const isRequired = componentType.schema.required?.includes(key)}
								<li class="config-item">
									<span class="config-key">{key}</span>
									<span class="config-type">{schema.type}</span>
									{#if isRequired}
										<span class="config-required">*</span>
									{/if}
								</li>
							{/each}
						</ul>
					</section>
				{/if}

				<p class="preview-hint">Click to add this component to your flow</p>
			</div>
		</div>

	{:else if mode === 'edit' && node}
		<!-- Edit Mode -->
		{@const categoryColor = getTypeColor(node.type)}
		<div class="edit-panel" data-testid="properties-edit">
			<header class="edit-header" style="border-left-color: {categoryColor};">
				<div class="header-top">
					<h3>{node.name}</h3>
					<div class="header-badges">
						{#if node.type}
							<span class="badge badge-{node.type}">
								{node.type}
							</span>
						{/if}
						{#if nodeComponentType?.domain}
							<span class="badge badge-domain">{nodeComponentType.domain}</span>
						{/if}
					</div>
				</div>
				<span class="edit-type">{nodeComponentType?.name || node.component}</span>
				{#if nodeComponentType?.description}
					<p class="component-description">{nodeComponentType.description}</p>
				{/if}
			</header>

			<form class="edit-form" onsubmit={(e) => e.preventDefault()}>
				<!-- Component Name -->
				<div class="form-group">
					<label for="prop-name">Name</label>
					<input
						id="prop-name"
						type="text"
						bind:value={editedName}
						onblur={handleBlur}
						required
						data-testid="prop-name-input"
					/>
				</div>

				<!-- Config Fields -->
				{#if nodeComponentType?.schema}
					<div class="config-section">
						<div class="search-header">
							<h4>Configuration</h4>
							<div class="search-controls">
								<input
									type="text"
									placeholder="Search fields..."
									bind:value={searchQuery}
									data-testid="field-search"
									class="field-search-input"
								/>
								<button
									type="button"
									onclick={handleClearSearch}
									disabled={!searchQuery.trim()}
									data-testid="clear-search"
									class="clear-search-button"
								>
									Clear
								</button>
							</div>
						</div>

						{#if isSearching}
							<!-- Search Results -->
							{#if hasSearchResults}
								{#each filteredFields as [fieldName, schema] (fieldName)}
									{@const isRequired = nodeComponentType.schema.required?.includes(fieldName)}
									{@const error = getFieldError(fieldName, schema)}
									<SchemaField
										name={fieldName}
										label={humanizeFieldName(fieldName)}
										{schema}
										bind:value={editedConfig[fieldName]}
										{error}
										{isRequired}
										onChange={(v) => handleFieldChange(fieldName, v)}
									/>
								{/each}
							{:else}
								<div data-testid="no-search-results" class="no-results">
									No fields match your search
								</div>
							{/if}
						{:else}
							<!-- Categorized Fields -->
							{#if basicFields.length > 0}
								<div data-testid="basic-fields" class="basic-fields-section">
									{#each basicFields as [fieldName, schema] (fieldName)}
										{@const isRequired = nodeComponentType.schema.required?.includes(fieldName)}
										{@const error = getFieldError(fieldName, schema)}
										<SchemaField
											name={fieldName}
											label={humanizeFieldName(fieldName)}
											{schema}
											bind:value={editedConfig[fieldName]}
											{error}
											{isRequired}
											onChange={(v) => handleFieldChange(fieldName, v)}
										/>
									{/each}
								</div>
							{/if}

							{#if advancedFields.length > 0}
								<details data-testid="advanced-fields" class="advanced-fields-section">
									<summary>Advanced Configuration</summary>
									{#each advancedFields as [fieldName, schema] (fieldName)}
										{@const isRequired = nodeComponentType.schema.required?.includes(fieldName)}
										{@const error = getFieldError(fieldName, schema)}
										<SchemaField
											name={fieldName}
											label={humanizeFieldName(fieldName)}
											{schema}
											bind:value={editedConfig[fieldName]}
											{error}
											{isRequired}
											onChange={(v) => handleFieldChange(fieldName, v)}
										/>
									{/each}
								</details>
							{/if}
						{/if}
					</div>
				{/if}
			</form>

			<!-- Actions -->
			<footer class="edit-footer">
				{#if isDirty}
					<div class="dirty-indicator">
						<span class="dirty-dot"></span>
						Unsaved changes
					</div>
				{/if}

				<div class="action-buttons">
					{#if isDirty}
						<button type="button" class="btn-secondary" onclick={handleReset}>
							Reset
						</button>
						<button type="button" class="btn-primary" onclick={handleSave} disabled={!canSave}>
							Save
						</button>
					{/if}
					<button type="button" class="btn-danger" onclick={handleDelete}>
						Delete
					</button>
				</div>
			</footer>

			<!-- Delete Confirmation -->
			{#if showDeleteConfirm}
				<div class="confirm-overlay" data-testid="delete-confirm">
					<div class="confirm-dialog">
						<h4>Delete Component?</h4>
						<p>Are you sure you want to delete "{node.name}"?</p>
						<p class="confirm-warning">This cannot be undone.</p>
						<div class="confirm-actions">
							<button class="btn-secondary" onclick={handleCancelDelete}>Cancel</button>
							<button class="btn-danger" onclick={handleConfirmDelete}>Delete</button>
						</div>
					</div>
				</div>
			{/if}
		</div>
	{/if}
</div>

<style>
	.properties-panel {
		display: flex;
		flex-direction: column;
		height: 100%;
		background: var(--properties-background, var(--ui-surface-secondary));
	}

	/* Empty State */
	.empty-state {
		display: flex;
		flex-direction: column;
		align-items: center;
		justify-content: center;
		height: 100%;
		padding: 2rem;
		text-align: center;
		color: var(--properties-empty-state-text, var(--ui-text-tertiary));
	}

	.empty-icon {
		margin-bottom: 1rem;
		opacity: 0.5;
	}

	.empty-title {
		margin: 0 0 0.5rem;
		font-weight: 600;
		font-size: 1rem;
		color: var(--ui-text-secondary);
	}

	.empty-hint {
		margin: 0;
		font-size: 0.875rem;
		line-height: 1.4;
	}

	/* Type Preview */
	.type-preview {
		display: flex;
		flex-direction: column;
		height: 100%;
	}

	.preview-header {
		padding: 1rem;
		background: var(--properties-header-bg, var(--ui-surface-tertiary));
		border-bottom: 1px solid var(--ui-border-subtle);
		border-left: 4px solid;
	}

	.preview-header h3 {
		margin: 0 0 0.25rem;
		font-size: 1rem;
	}

	.preview-category {
		font-size: 0.75rem;
		color: var(--ui-text-secondary);
		text-transform: uppercase;
	}

	.preview-content {
		flex: 1;
		overflow-y: auto;
		padding: 1rem;
	}

	.preview-section {
		margin-bottom: 1.25rem;
	}

	.preview-section h4 {
		margin: 0 0 0.5rem;
		font-size: 0.75rem;
		font-weight: 600;
		text-transform: uppercase;
		color: var(--ui-text-secondary);
	}

	.preview-section p {
		margin: 0;
		font-size: 0.875rem;
		color: var(--ui-text-primary);
		line-height: 1.4;
	}

	.protocol-badge {
		display: inline-block;
		padding: 0.25rem 0.5rem;
		background: var(--ui-surface-tertiary);
		border-radius: 4px;
		font-size: 0.75rem;
		font-family: monospace;
	}

	.ports-list,
	.config-schema-list {
		list-style: none;
		margin: 0;
		padding: 0;
	}

	.port-item,
	.config-item {
		display: flex;
		align-items: center;
		gap: 0.5rem;
		padding: 0.375rem 0;
		font-size: 0.8125rem;
		border-bottom: 1px solid var(--ui-border-subtle);
	}

	.port-direction {
		font-size: 0.6875rem;
		padding: 0.125rem 0.375rem;
		border-radius: 3px;
		text-transform: uppercase;
	}

	.port-direction.input {
		background: var(--domain-network-container);
		color: var(--domain-network);
	}

	.port-direction.output {
		background: var(--domain-storage-container);
		color: var(--domain-storage);
	}

	.port-name,
	.config-key {
		font-weight: 500;
	}

	.port-required,
	.config-required {
		font-size: 0.6875rem;
		color: var(--status-error);
	}

	.config-type {
		font-size: 0.6875rem;
		color: var(--ui-text-tertiary);
		font-family: monospace;
	}

	.preview-hint {
		margin-top: 1.5rem;
		padding: 0.75rem;
		background: var(--ui-surface-primary);
		border-radius: 4px;
		font-size: 0.8125rem;
		color: var(--ui-text-secondary);
		text-align: center;
	}

	/* Edit Panel */
	.edit-panel {
		display: flex;
		flex-direction: column;
		height: 100%;
		position: relative;
	}

	.edit-header {
		padding: 1rem;
		background: var(--properties-header-bg, var(--ui-surface-tertiary));
		border-bottom: 1px solid var(--ui-border-subtle);
		border-left: 4px solid;
	}

	.edit-header h3 {
		margin: 0 0 0.25rem;
		font-size: 1rem;
		word-break: break-word;
	}

	.edit-type {
		font-size: 0.75rem;
		color: var(--ui-text-secondary);
		font-family: monospace;
	}

	.header-top {
		display: flex;
		justify-content: space-between;
		align-items: flex-start;
		gap: 0.5rem;
	}

	.header-badges {
		display: flex;
		gap: 0.25rem;
		flex-shrink: 0;
	}

	.badge {
		font-size: 0.625rem;
		padding: 0.125rem 0.375rem;
		border-radius: 4px;
		text-transform: uppercase;
		font-weight: 600;
		white-space: nowrap;
	}

	.badge-input {
		background: var(--status-success-container);
		color: var(--status-success-on-container);
	}

	.badge-output {
		background: var(--status-warning-container);
		color: var(--status-warning-on-container);
	}

	.badge-processor {
		background: var(--ui-interactive-primary);
		color: white;
	}

	.badge-domain {
		background: var(--ui-surface-tertiary);
		color: var(--ui-text-secondary);
		border: 1px solid var(--ui-border-subtle);
	}

	.component-description {
		font-size: 0.75rem;
		color: var(--ui-text-secondary);
		margin: 0.5rem 0 0 0;
		line-height: 1.4;
	}

	.edit-form {
		flex: 1;
		overflow-y: auto;
		padding: 1rem;
	}

	.form-group {
		margin-bottom: 1rem;
	}

	.form-group label {
		display: block;
		margin-bottom: 0.375rem;
		font-size: 0.8125rem;
		font-weight: 500;
		color: var(--ui-text-primary);
	}

	.config-section {
		margin-top: 1.5rem;
		padding-top: 1rem;
		border-top: 1px solid var(--ui-border-subtle);
	}

	.config-section h4 {
		margin: 0 0 1rem;
		font-size: 0.8125rem;
		font-weight: 600;
		color: var(--ui-text-secondary);
		text-transform: uppercase;
	}

	.search-header {
		margin-bottom: 1rem;
	}

	.search-controls {
		display: flex;
		gap: 0.5rem;
		margin-top: 0.75rem;
	}

	.field-search-input {
		flex: 1;
		padding: 0.5rem 0.75rem;
		border: 1px solid var(--ui-border-subtle);
		border-radius: 4px;
		font-size: 0.875rem;
		background: var(--ui-surface-primary);
		color: var(--ui-text-primary);
	}

	.field-search-input:focus {
		outline: none;
		border-color: var(--ui-interactive-primary);
		box-shadow: 0 0 0 2px var(--ui-focus-ring);
	}

	.clear-search-button {
		padding: 0.5rem 0.75rem;
		border: 1px solid var(--ui-border-subtle);
		border-radius: 4px;
		font-size: 0.875rem;
		background: var(--ui-surface-primary);
		color: var(--ui-text-primary);
		cursor: pointer;
		transition: all 150ms ease;
	}

	.clear-search-button:hover:not(:disabled) {
		background: var(--ui-surface-secondary);
	}

	.clear-search-button:disabled {
		opacity: 0.5;
		cursor: not-allowed;
	}

	.basic-fields-section,
	.advanced-fields-section {
		margin-top: 1rem;
	}

	.advanced-fields-section {
		border: 1px solid var(--ui-border-subtle);
		border-radius: 4px;
		padding: 0.75rem;
	}

	.advanced-fields-section summary {
		cursor: pointer;
		font-weight: 600;
		padding: 0.5rem;
		margin: -0.75rem -0.75rem 0.75rem -0.75rem;
		user-select: none;
		border-radius: 4px;
	}

	.advanced-fields-section summary:hover {
		background: var(--ui-surface-secondary);
	}

	.advanced-fields-section[open] summary {
		margin-bottom: 1rem;
	}

	.no-results {
		padding: 1.5rem;
		text-align: center;
		color: var(--ui-text-tertiary);
		font-size: 0.875rem;
		border: 1px solid var(--ui-border-subtle);
		border-radius: 4px;
		background: var(--ui-surface-secondary);
	}

	/* Footer */
	.edit-footer {
		padding: 0.75rem 1rem;
		border-top: 1px solid var(--ui-border-subtle);
		background: var(--ui-surface-tertiary);
	}

	.dirty-indicator {
		display: flex;
		align-items: center;
		gap: 0.5rem;
		margin-bottom: 0.75rem;
		font-size: 0.75rem;
		color: var(--status-warning-on-container);
	}

	.dirty-dot {
		width: 8px;
		height: 8px;
		background: var(--status-warning);
		border-radius: 50%;
	}

	.action-buttons {
		display: flex;
		gap: 0.5rem;
		justify-content: flex-end;
	}

	.btn-primary,
	.btn-secondary,
	.btn-danger {
		padding: 0.5rem 1rem;
		border: none;
		border-radius: 4px;
		font-size: 0.8125rem;
		font-weight: 500;
		cursor: pointer;
		transition: all 150ms ease;
	}

	.btn-primary {
		background: var(--ui-interactive-primary);
		color: var(--ui-text-on-primary);
	}

	.btn-primary:hover:not(:disabled) {
		background: var(--ui-interactive-primary-hover);
	}

	.btn-primary:disabled {
		opacity: 0.5;
		cursor: not-allowed;
	}

	.btn-secondary {
		background: var(--ui-surface-primary);
		color: var(--ui-text-primary);
		border: 1px solid var(--ui-border-subtle);
	}

	.btn-secondary:hover {
		background: var(--ui-surface-secondary);
	}

	.btn-danger {
		background: var(--status-error);
		color: white;
	}

	.btn-danger:hover {
		background: var(--status-error-hover);
	}

	/* Delete Confirmation */
	.confirm-overlay {
		position: absolute;
		inset: 0;
		background: rgba(0, 0, 0, 0.5);
		display: flex;
		align-items: center;
		justify-content: center;
		padding: 1rem;
	}

	.confirm-dialog {
		background: var(--ui-surface-primary);
		padding: 1.5rem;
		border-radius: 8px;
		max-width: 300px;
		box-shadow: 0 4px 20px rgba(0, 0, 0, 0.2);
	}

	.confirm-dialog h4 {
		margin: 0 0 0.75rem;
		color: var(--status-error);
	}

	.confirm-dialog p {
		margin: 0 0 0.5rem;
		font-size: 0.875rem;
	}

	.confirm-warning {
		color: var(--ui-text-tertiary);
		font-style: italic;
	}

	.confirm-actions {
		display: flex;
		gap: 0.5rem;
		justify-content: flex-end;
		margin-top: 1rem;
	}
</style>
