<script lang="ts">
	import type { ComponentInstance } from '$lib/types/flow';
	import type { SchemaResponse } from '$lib/types/schema';
	import type { ConfigValue } from '$lib/types/config';
	import SchemaForm from './SchemaForm.svelte';
	import JsonEditor from './JsonEditor.svelte';

	interface ConfigPanelProps {
		component: ComponentInstance | null;
		onSave?: (_nodeId: string, _config: Record<string, ConfigValue>) => void;
		onClose?: () => void;
	}

	let { component, onSave, onClose }: ConfigPanelProps = $props();

	// T092: Schema caching by component type
	let schemaCache = $state<Map<string, SchemaResponse>>(new Map());

	// Current component schema
	let componentSchema = $state<SchemaResponse | null>(null);

	// Schema loading state
	let loadingSchema = $state(false);
	let schemaError = $state<string | null>(null);

	// T095: Dirty state preservation (per component ID)
	let dirtyConfigs = $state<Map<string, Record<string, ConfigValue>>>(new Map());

	// Current config being edited
	let editingConfig = $state<Record<string, ConfigValue>>({});

	let error = $state<string | null>(null);

	// JSON parse error from JsonEditor
	let jsonParseError = $state<string | null>(null);

	// T097: Backend validation errors (reserved for future deployed component config)
	let backendValidationErrors = $state<Record<string, string>>({});

	// P2-013: Track current component ID to detect switches
	let currentComponentId = $state<string | null>(null);

	// P2-013: Load schema and reset session state when component changes.
	// Uses currentComponentId to detect switches without manual prev-ID bookkeeping.
	$effect(() => {
		if (component && component.id !== currentComponentId) {
			currentComponentId = component.id;
			loadSchemaForComponent(component.component);

			// T095: Restore dirty state if exists, otherwise use component config
			if (dirtyConfigs.has(component.id)) {
				editingConfig = { ...dirtyConfigs.get(component.id)! };
			} else {
				editingConfig = { ...(component.config as Record<string, ConfigValue>) };
			}

			// Isolate error state per component session
			error = null;
			schemaError = null;
			jsonParseError = null;
		} else if (!component) {
			currentComponentId = null;
			componentSchema = null;
			editingConfig = {};
			jsonParseError = null;
		}
	});

	// T095: Mirror editingConfig into dirtyConfigs whenever it changes.
	// This runs synchronously after each binding update from JsonEditor/SchemaForm.
	$effect(() => {
		if (component && currentComponentId && editingConfig) {
			dirtyConfigs.set(currentComponentId, editingConfig);
		}
	});

	// T092: Load schema with caching
	async function loadSchemaForComponent(componentType: string): Promise<void> {
		// Check cache first
		if (schemaCache.has(componentType)) {
			componentSchema = schemaCache.get(componentType)!;
			return;
		}

		// T099: Loading state
		loadingSchema = true;
		schemaError = null;

		try {
			// T096: Fetch schema with error handling
			const response = await fetch(`/components/types/${componentType}`);

			if (response.status === 404) {
				// Component has no schema - use JSON editor fallback
				componentSchema = null;
				schemaCache.set(componentType, null as unknown as SchemaResponse); // Cache the miss
				return;
			}

			if (!response.ok) {
				throw new Error(`Failed to load schema: ${response.statusText}`);
			}

			const schema = await response.json();
			componentSchema = schema;
			schemaCache.set(componentType, schema);
		} catch (err) {
			schemaError = err instanceof Error ? err.message : 'Failed to load schema';
			componentSchema = null;
		} finally {
			loadingSchema = false;
		}
	}

	// T097: Handle schema form save
	// NOTE: For draft flows, we just update the flow model locally.
	// Backend API calls are only for deployed/running components (future feature).
	function handleSchemaFormSave(config: Record<string, ConfigValue>) {
		if (!component) return;

		// Clear previous errors
		backendValidationErrors = {};
		error = null;

		// Success - clear dirty state and notify parent
		dirtyConfigs.delete(component.id);
		onSave?.(component.id, config);
		onClose?.();
	}

	// Handle JSON editor save
	function handleJsonSave() {
		if (!component) return;

		// Check if JsonEditor has a parse error
		if (jsonParseError) {
			error = jsonParseError;
			return;
		}

		// editingConfig is already parsed by JsonEditor via binding
		// Just validate it's a valid object and save
		if (editingConfig && typeof editingConfig === 'object') {
			dirtyConfigs.delete(component.id);
			onSave?.(component.id, editingConfig);
			error = null;
			onClose?.();
		} else {
			error = 'Invalid configuration';
		}
	}

	function handleCancel() {
		// Clear dirty state and reset to original config
		if (component) {
			dirtyConfigs.delete(component.id);
			editingConfig = { ...(component.config as Record<string, ConfigValue>) };
			error = null;
			jsonParseError = null;
		}
		onClose?.();
	}
</script>

{#if component}
	<div class="config-panel">
		<header>
			<h3>Configure: {component.component}</h3>
			<button class="close-button" onclick={handleCancel} aria-label="Close">✕</button>
		</header>

		<div class="panel-body">
			<div class="component-info">
				<div class="info-item">
					<span class="label">ID:</span>
					<span class="value">{component.id}</span>
				</div>
				<div class="info-item">
					<span class="label">Type:</span>
					<span class="value">{component.component}</span>
				</div>
				<div class="info-item">
					<span class="label">Name:</span>
					<span class="value">{component.name}</span>
				</div>
				{#if component.health}
					<div class="info-item">
						<span class="label">Health:</span>
						<span class="value {component.health.status}">{component.health.status}</span>
					</div>
				{/if}
			</div>

			<!-- T099: Loading state -->
			{#if loadingSchema}
				<div class="loading-message">
					<p>Loading schema...</p>
				</div>
			{:else if schemaError}
				<!-- T096: Schema fetch error -->
				<div class="error-message" role="alert">
					<p>Failed to load schema: {schemaError}</p>
					<p>Falling back to JSON editor.</p>
				</div>
				<JsonEditor bind:config={editingConfig} bind:parseError={jsonParseError} />
				<footer>
					<button onclick={handleCancel} class="cancel-button">Cancel</button>
					<button onclick={handleJsonSave} class="save-button">Apply</button>
				</footer>
			{:else if componentSchema}
				<!-- T093: Schema-driven form with backend validation (T097) -->
				{#if error}
					<div class="error-message" role="alert">
						<p>{error}</p>
					</div>
				{/if}
				<SchemaForm
					schema={componentSchema.schema}
					bind:config={editingConfig}
					externalErrors={backendValidationErrors}
					saving={false}
					onSave={handleSchemaFormSave}
					onCancel={handleCancel}
				/>
			{:else}
				<!-- T094, T041: Fallback to JSON editor when schema missing -->
				<p class="schema-info">Schema not available for this component type</p>
				<JsonEditor bind:config={editingConfig} bind:parseError={jsonParseError} />
				<footer>
					<button onclick={handleCancel} class="cancel-button">Cancel</button>
					<button onclick={handleJsonSave} class="save-button">Apply</button>
				</footer>
			{/if}
		</div>
	</div>
{/if}

<style>
	.config-panel {
		display: flex;
		flex-direction: column;
		height: 100%;
		background: var(--ui-surface-primary);
		border: 1px solid var(--ui-border-subtle);
		border-radius: 4px;
		overflow: hidden;
	}

	header {
		display: flex;
		justify-content: space-between;
		align-items: center;
		padding: 1rem;
		border-bottom: 1px solid var(--ui-border-subtle);
		background: var(--ui-surface-tertiary);
	}

	header h3 {
		margin: 0;
		font-size: 1.125rem;
		font-weight: 600;
	}

	.close-button {
		background: none;
		border: none;
		font-size: 1.5rem;
		cursor: pointer;
		padding: 0;
		width: 2rem;
		height: 2rem;
		display: flex;
		align-items: center;
		justify-content: center;
		border-radius: 4px;
	}

	.close-button:hover {
		background: var(--ui-surface-secondary);
	}

	.panel-body {
		flex: 1;
		padding: 1rem;
		overflow-y: auto;
	}

	.component-info {
		margin-bottom: 1.5rem;
		padding: 1rem;
		background: var(--ui-surface-secondary);
		border-radius: 4px;
	}

	.info-item {
		display: flex;
		gap: 0.5rem;
		margin-bottom: 0.5rem;
	}

	.info-item:last-child {
		margin-bottom: 0;
	}

	.label {
		font-weight: 600;
		min-width: 80px;
	}

	.value {
		flex: 1;
	}

	.value.healthy {
		color: var(--status-success);
	}

	.value.degraded {
		color: var(--status-warning);
	}

	.value.unhealthy {
		color: var(--status-error);
	}

	.value.not_running {
		color: var(--ui-text-disabled);
	}

	.error-message {
		color: var(--status-error);
		font-size: 0.875rem;
		padding: 0.5rem;
		background: var(--status-error-container);
		border-radius: 4px;
	}

	/* T099: Loading state styling */
	.loading-message {
		padding: 2rem;
		text-align: center;
		color: var(--ui-text-secondary);
		display: flex;
		flex-direction: column;
		align-items: center;
		gap: 1rem;
	}

	.loading-message p {
		margin: 0;
	}

	/* T099: Animated loading spinner */
	.loading-message::before {
		content: '⏳';
		font-size: 2rem;
		animation: spin 1s linear infinite;
	}

	@keyframes spin {
		from {
			transform: rotate(0deg);
		}
		to {
			transform: rotate(360deg);
		}
	}

	footer {
		display: flex;
		justify-content: flex-end;
		gap: 0.5rem;
		padding: 1rem;
		border-top: 1px solid var(--ui-border-subtle);
		background: var(--ui-surface-tertiary);
	}

	button {
		padding: 0.5rem 1rem;
		border: none;
		border-radius: 4px;
		font-size: 0.875rem;
		font-weight: 500;
		cursor: pointer;
		transition: all 0.2s;
	}

	.cancel-button {
		background: var(--ui-interactive-secondary);
		color: white;
	}

	.cancel-button:hover {
		background: var(--ui-interactive-secondary-hover);
	}

	.save-button {
		background: var(--ui-interactive-primary);
		color: white;
	}

	.save-button:hover {
		background: var(--ui-interactive-primary-hover);
	}

	.schema-info {
		font-size: 0.875rem;
		color: var(--ui-text-secondary);
		margin: 0 0 0.75rem;
	}
</style>
