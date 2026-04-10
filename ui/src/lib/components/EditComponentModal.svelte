<script lang="ts">
	import type { FlowNode } from '$lib/types/flow';
	import type { ComponentType, PropertySchema } from '$lib/types/component';
	import PortsEditor from './PortsEditor.svelte';

	interface EditComponentModalProps {
		isOpen: boolean;
		node: FlowNode | null;
		componentType?: ComponentType;
		onSave?: (nodeId: string, name: string, config: Record<string, unknown>) => void;
		onDelete?: (nodeId: string) => void;
		onClose?: () => void;
	}

	let { isOpen, node, componentType, onSave, onDelete, onClose }: EditComponentModalProps =
		$props();

	// Form state
	let editedName = $state('');
	let editedConfig = $state<Record<string, unknown>>({});

	// Confirmation dialog state
	let showDeleteConfirm = $state(false);

	// Dialog element reference for focus management
	let dialogElement: HTMLDivElement | undefined = $state();

	// Initialize form when node changes
	$effect(() => {
		if (node) {
			editedName = node.name;
			editedConfig = { ...node.config };
		} else {
			editedName = '';
			editedConfig = {};
		}
		// Reset confirmation dialog when node changes
		showDeleteConfirm = false;
	});

	// Dirty state - check if form has changes
	const isDirty = $derived.by(() => {
		if (!node) return false;

		if (editedName !== node.name) return true;

		// Check if config has changed
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
		if (!editedName.trim()) {
			return false;
		}

		// Check required config fields
		if (componentType?.schema) {
			const required = componentType.schema.required || [];
			for (const field of required) {
				const value = editedConfig[field];
				if (value === undefined || value === null || value === '') {
					return false;
				}

				// Validate number ranges
				const schema = componentType.schema.properties[field];
				if (schema.type === 'number') {
					const numValue = Number(value);
					if (isNaN(numValue)) {
						return false;
					}
					if (schema.minimum !== undefined && numValue < schema.minimum) {
						return false;
					}
					if (schema.maximum !== undefined && numValue > schema.maximum) {
						return false;
					}
				}
			}
		}

		return true;
	});

	// Can save - form must be valid AND dirty
	const canSave = $derived(isFormValid && isDirty);

	// Get validation error for a field
	function getFieldValidationError(fieldName: string, schema: PropertySchema): string | null {
		const value = editedConfig[fieldName];

		// Only validate if field has a value
		if (value === undefined || value === null || value === '') {
			return null;
		}

		if (schema.type === 'number') {
			const numValue = Number(value);
			if (isNaN(numValue)) {
				return 'Must be a valid number';
			}
			if (schema.minimum !== undefined && numValue < schema.minimum) {
				return `Value must be at minimum ${schema.minimum}`;
			}
			if (schema.maximum !== undefined && numValue > schema.maximum) {
				return `Value must be at maximum ${schema.maximum}`;
			}
		}

		return null;
	}

	// Handle ESC key to close dialog
	function handleKeydown(event: KeyboardEvent) {
		if (event.key === 'Escape' && isOpen) {
			if (showDeleteConfirm) {
				// Close confirmation dialog
				showDeleteConfirm = false;
			} else {
				// Close main dialog
				handleClose();
			}
		}
	}

	// Handle click on overlay background to close dialog
	function handleBackgroundClick(event: MouseEvent) {
		if (event.target === event.currentTarget) {
			handleClose();
		}
	}

	// Handle save
	function handleSave() {
		if (!canSave || !node) {
			return;
		}

		onSave?.(node.id, editedName, editedConfig);
	}

	// Handle delete button click
	function handleDeleteClick() {
		showDeleteConfirm = true;
	}

	// Handle delete confirmation
	function handleDeleteConfirm() {
		if (!node) return;
		onDelete?.(node.id);
		showDeleteConfirm = false;
	}

	// Handle delete cancellation
	function handleDeleteCancel() {
		showDeleteConfirm = false;
	}

	// Handle close
	function handleClose() {
		onClose?.();
	}

	// Focus management - focus first input when dialog opens
	$effect(() => {
		if (isOpen && dialogElement && node) {
			const focusable = dialogElement.querySelector<HTMLInputElement>('input[name="name"]');
			focusable?.focus();
		}
	});
</script>

<svelte:window onkeydown={handleKeydown} />

{#if isOpen && node}
	<div
		bind:this={dialogElement}
		class="dialog-overlay"
		role="dialog"
		aria-modal="true"
		aria-labelledby="dialog-title"
		tabindex="-1"
		onclick={handleBackgroundClick}
		onkeydown={(e) => e.key === 'Escape' && handleClose()}
	>
		<!-- svelte-ignore a11y_no_static_element_interactions a11y_click_events_have_key_events -->
		<div class="dialog-content" onclick={(e) => e.stopPropagation()}>
			<header class="dialog-header">
				<h2 id="dialog-title">Edit: {node.name}</h2>
				<button class="close-button" onclick={handleClose} aria-label="Close dialog">×</button>
			</header>

			<div class="dialog-body">
				<form onsubmit={(e) => e.preventDefault()}>
					<!-- Component Name -->
					<div class="form-group">
						<label for="name">Component Name</label>
						<input
							id="name"
							name="name"
							type="text"
							bind:value={editedName}
							required
							aria-required="true"
							placeholder="Enter component name"
						/>
					</div>

					<!-- Component Type (Read-only) -->
					<div class="form-group">
						<span class="form-label">Component Type</span>
						<div class="component-type-readonly">Type: {node.component}</div>
					</div>

					<!-- Config Fields (dynamic based on component type) -->
					{#if componentType?.schema}
						<div class="config-section">
							<h3>Configuration</h3>
							{#each Object.entries(componentType.schema.properties) as [fieldName, schema] (fieldName)}
								{@const isRequired = componentType.schema.required?.includes(fieldName)}
								{@const validationError = getFieldValidationError(fieldName, schema)}

								<div class="form-group">
									<label for="config.{fieldName}">
										{fieldName}
										{#if isRequired}
											<span class="required">*</span>
										{/if}
									</label>
									{#if schema.description}
										<p class="field-description">{schema.description}</p>
									{/if}

									{#if schema.type === 'string'}
										<input
											id="config.{fieldName}"
											name="config.{fieldName}"
											type="text"
											bind:value={editedConfig[fieldName]}
											required={isRequired}
											aria-required={isRequired}
											aria-invalid={validationError !== null}
										/>
									{:else if schema.type === 'int' || schema.type === 'integer' || schema.type === 'number'}
										<input
											id="config.{fieldName}"
											name="config.{fieldName}"
											type="number"
											bind:value={editedConfig[fieldName]}
											required={isRequired}
											aria-required={isRequired}
											aria-invalid={validationError !== null}
											min={schema.minimum}
											max={schema.maximum}
										/>
									{:else if schema.type === 'bool' || schema.type === 'boolean'}
										<input
											id="config.{fieldName}"
											name="config.{fieldName}"
											type="checkbox"
											bind:checked={editedConfig[fieldName] as boolean}
										/>
									{:else if schema.type === 'enum' && schema.enum}
										<select
											id="config.{fieldName}"
											name="config.{fieldName}"
											bind:value={editedConfig[fieldName]}
											required={isRequired}
											aria-required={isRequired}
										>
											<option value="">Select...</option>
											{#each schema.enum as option (option)}
												<option value={option}>{option}</option>
											{/each}
										</select>
									{:else if schema.type === 'ports'}
										<!-- Ports editor -->
										<PortsEditor
											value={(editedConfig[fieldName] ?? { inputs: [], outputs: [] }) as { inputs?: Record<string, unknown>[]; outputs?: Record<string, unknown>[] }}
											portFields={schema.portFields}
											onChange={(v) => {
												editedConfig[fieldName] = v;
											}}
										/>
									{:else}
										<!-- object, array - JSON editor -->
										<textarea
											id="config.{fieldName}"
											name="config.{fieldName}"
											class="json-editor"
											value={JSON.stringify(editedConfig[fieldName] ?? {}, null, 2)}
											onchange={(e) => {
												try {
													editedConfig[fieldName] = JSON.parse(e.currentTarget.value);
												} catch {
													// Invalid JSON - keep current value
												}
											}}
										></textarea>
									{/if}

									{#if validationError}
										<div class="validation-error">{validationError}</div>
									{/if}
								</div>
							{/each}
						</div>
					{/if}
				</form>
			</div>

			<footer class="dialog-footer">
				<button class="danger-button" onclick={handleDeleteClick}>Delete</button>
				<div class="footer-right">
					<button class="secondary-button" onclick={handleClose}>Cancel</button>
					<button class="primary-button" onclick={handleSave} disabled={!canSave}>Save</button>
				</div>
			</footer>
		</div>
	</div>

	<!-- Delete Confirmation Dialog -->
	{#if showDeleteConfirm}
		<div
			class="confirm-dialog"
			role="alertdialog"
			aria-modal="true"
			tabindex="-1"
			onkeydown={(e) => e.key === 'Escape' && handleDeleteCancel()}
		>
			<!-- svelte-ignore a11y_no_static_element_interactions a11y_click_events_have_key_events -->
			<div class="confirm-content" onclick={(e) => e.stopPropagation()}>
				<h3>Confirm Deletion</h3>
				<p>Are you sure you want to delete "{node.name}"?</p>
				<p class="confirm-warning">This action cannot be undone.</p>
				<div class="confirm-actions">
					<button class="cancel-delete-button" onclick={handleDeleteCancel}>Cancel</button>
					<button class="confirm-delete-button" onclick={handleDeleteConfirm}>Delete</button>
				</div>
			</div>
		</div>
	{/if}
{/if}

<style>
	.dialog-overlay {
		position: fixed;
		top: 0;
		left: 0;
		right: 0;
		bottom: 0;
		background: rgba(0, 0, 0, 0.5);
		display: flex;
		align-items: center;
		justify-content: center;
		z-index: 1000;
	}

	.dialog-content {
		background: var(--ui-surface-primary);
		border-radius: 8px;
		max-width: 600px;
		width: 90%;
		max-height: 80vh;
		display: flex;
		flex-direction: column;
		box-shadow: 0 4px 20px rgba(0, 0, 0, 0.3);
	}

	.dialog-header {
		display: flex;
		justify-content: space-between;
		align-items: center;
		padding: 1.5rem;
		border-bottom: 1px solid var(--ui-border-subtle);
	}

	.dialog-header h2 {
		margin: 0;
		font-size: 1.5rem;
		color: var(--ui-text-primary);
	}

	.close-button {
		background: none;
		border: none;
		font-size: 2rem;
		cursor: pointer;
		color: var(--ui-text-secondary);
		padding: 0;
		width: 2rem;
		height: 2rem;
		line-height: 1;
	}

	.close-button:hover {
		color: var(--ui-text-primary);
	}

	.dialog-body {
		flex: 1;
		overflow-y: auto;
		padding: 1.5rem;
	}

	.form-group {
		margin-bottom: 1.5rem;
	}

	.form-group label,
	.form-label {
		display: block;
		margin-bottom: 0.5rem;
		font-weight: 600;
		color: var(--ui-text-primary);
	}

	.form-group input,
	.form-group select {
		width: 100%;
		padding: 0.5rem;
		border: 1px solid var(--ui-border-default);
		border-radius: 4px;
		font-size: 1rem;
		background: var(--ui-surface-primary);
		color: var(--ui-text-primary);
	}

	.form-group input[type='checkbox'] {
		width: auto;
	}

	.form-group input:focus,
	.form-group select:focus {
		outline: 2px solid var(--ui-focus-ring);
		outline-offset: 1px;
		border-color: var(--ui-interactive-primary);
	}

	.form-group input[aria-invalid='true'] {
		border-color: var(--status-error);
	}

	.json-editor {
		width: 100%;
		min-height: 100px;
		padding: 0.5rem;
		border: 1px solid var(--ui-border-default);
		border-radius: 4px;
		font-family: 'SF Mono', Monaco, 'Cascadia Code', monospace;
		font-size: 0.8rem;
		background: var(--ui-surface-primary);
		color: var(--ui-text-primary);
		resize: vertical;
		white-space: pre;
		tab-size: 2;
	}

	.json-editor:focus {
		outline: 2px solid var(--ui-focus-ring);
		outline-offset: 1px;
		border-color: var(--ui-interactive-primary);
	}

	.component-type-readonly {
		padding: 0.5rem;
		background: var(--ui-surface-secondary);
		border: 1px solid var(--ui-border-subtle);
		border-radius: 4px;
		color: var(--ui-text-secondary);
		font-family: monospace;
	}

	.field-description {
		font-size: 0.875rem;
		color: var(--ui-text-secondary);
		margin: 0.25rem 0 0.5rem 0;
	}

	.required {
		color: var(--status-error);
	}

	.validation-error {
		color: var(--status-error);
		font-size: 0.875rem;
		margin-top: 0.25rem;
	}

	.config-section {
		margin-top: 2rem;
		padding-top: 1.5rem;
		border-top: 1px solid var(--ui-border-subtle);
	}

	.config-section h3 {
		margin: 0 0 1rem 0;
		font-size: 1.125rem;
		color: var(--ui-text-primary);
	}

	.dialog-footer {
		padding: 1.5rem;
		border-top: 1px solid var(--ui-border-subtle);
		display: flex;
		justify-content: space-between;
		align-items: center;
	}

	.footer-right {
		display: flex;
		gap: 1rem;
	}

	.primary-button,
	.secondary-button,
	.danger-button {
		padding: 0.75rem 1.5rem;
		border: none;
		border-radius: 4px;
		cursor: pointer;
		font-size: 1rem;
		transition: all 0.2s;
	}

	.primary-button {
		background: var(--ui-interactive-primary);
		color: var(--ui-text-on-primary);
	}

	.primary-button:hover:not(:disabled) {
		background: var(--ui-interactive-primary-hover);
	}

	.primary-button:disabled {
		opacity: 0.5;
		cursor: not-allowed;
	}

	.secondary-button {
		background: var(--ui-surface-secondary);
		color: var(--ui-text-primary);
		border: 1px solid var(--ui-border-default);
	}

	.secondary-button:hover {
		background: var(--ui-surface-tertiary);
	}

	.danger-button {
		background: var(--status-error);
		color: white;
	}

	.danger-button:hover {
		background: #c53030;
	}

	/* Delete Confirmation Dialog */
	.confirm-dialog {
		position: fixed;
		top: 0;
		left: 0;
		right: 0;
		bottom: 0;
		background: rgba(0, 0, 0, 0.7);
		display: flex;
		align-items: center;
		justify-content: center;
		z-index: 1001;
	}

	.confirm-content {
		background: var(--ui-surface-primary);
		border-radius: 8px;
		padding: 2rem;
		max-width: 400px;
		width: 90%;
		box-shadow: 0 4px 20px rgba(0, 0, 0, 0.5);
	}

	.confirm-content h3 {
		margin: 0 0 1rem 0;
		color: var(--status-error);
	}

	.confirm-content p {
		margin: 0.5rem 0;
	}

	.confirm-warning {
		font-size: 0.875rem;
		color: var(--ui-text-secondary);
		font-style: italic;
	}

	.confirm-actions {
		display: flex;
		gap: 1rem;
		margin-top: 1.5rem;
		justify-content: flex-end;
	}

	.cancel-delete-button,
	.confirm-delete-button {
		padding: 0.75rem 1.5rem;
		border: none;
		border-radius: 4px;
		cursor: pointer;
		font-size: 1rem;
		transition: all 0.2s;
	}

	.cancel-delete-button {
		background: var(--ui-surface-secondary);
		color: var(--ui-text-primary);
		border: 1px solid var(--ui-border-default);
	}

	.cancel-delete-button:hover {
		background: var(--ui-surface-tertiary);
	}

	.confirm-delete-button {
		background: var(--status-error);
		color: white;
	}

	.confirm-delete-button:hover {
		background: #c53030;
	}
</style>
