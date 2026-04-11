<script lang="ts">
	import type { PortFieldSchema } from '$lib/types/component';

	/**
	 * Port data structure from backend - flexible to accept any backend format
	 */
	type Port = Record<string, unknown> & {
		name?: string;
		subject?: string;
		type?: string;
		interface?: string;
		description?: string;
		stream_name?: string;
		timeout?: string;
		required?: boolean;
	};

	/**
	 * Ports value structure from backend
	 */
	interface PortsValue {
		inputs?: Port[];
		outputs?: Port[];
	}

	interface PortsEditorProps {
		/** Current ports value - accepts any object with inputs/outputs arrays */
		value: { inputs?: Record<string, unknown>[]; outputs?: Record<string, unknown>[] };
		/** Schema defining which fields are editable */
		portFields?: Record<string, PortFieldSchema>;
		/** Callback when value changes */
		onChange?: (value: PortsValue) => void;
	}

	let { value, portFields, onChange }: PortsEditorProps = $props();

	// Default editable fields if schema doesn't specify
	const defaultEditableFields = ['subject', 'stream_name', 'timeout'];

	/**
	 * Check if a field is editable based on schema or defaults
	 */
	function isFieldEditable(fieldName: string): boolean {
		if (portFields && portFields[fieldName]) {
			return portFields[fieldName].editable;
		}
		return defaultEditableFields.includes(fieldName);
	}

	/**
	 * Handle field change for a port
	 */
	function handleFieldChange(
		direction: 'inputs' | 'outputs',
		index: number,
		fieldName: string,
		newValue: string
	) {
		const newPortsValue = { ...value };
		const ports = direction === 'inputs' ? [...(value.inputs || [])] : [...(value.outputs || [])];

		if (ports[index]) {
			ports[index] = { ...ports[index], [fieldName]: newValue };
			newPortsValue[direction] = ports;
			onChange?.(newPortsValue);
		}
	}

	/**
	 * Get display value for a field
	 */
	function getFieldValue(port: Record<string, unknown>, fieldName: string): string {
		const val = port[fieldName];
		if (val === undefined || val === null) return '';
		if (typeof val === 'boolean') return val ? 'Yes' : 'No';
		return String(val);
	}

	/**
	 * Get editable fields for a port
	 */
	function getEditableFields(port: Record<string, unknown>): string[] {
		return Object.keys(port).filter((key) => isFieldEditable(key) && port[key] !== undefined);
	}

	/**
	 * Get read-only fields for a port (excluding editable ones)
	 */
	function getReadOnlyFields(port: Record<string, unknown>): string[] {
		const excludeFields = ['name', 'type']; // Shown in header
		return Object.keys(port).filter(
			(key) =>
				!isFieldEditable(key) &&
				!excludeFields.includes(key) &&
				port[key] !== undefined &&
				port[key] !== ''
		);
	}

	// Reactive port counts
	const inputCount = $derived(value?.inputs?.length ?? 0);
	const outputCount = $derived(value?.outputs?.length ?? 0);
</script>

<div class="ports-editor" data-testid="ports-editor">
	<!-- Inputs Section -->
	<section class="port-section">
		<h5 class="section-header">
			<span class="section-title">Inputs</span>
			<span class="port-count">{inputCount}</span>
		</h5>

		{#if inputCount === 0}
			<div class="empty-ports">No input ports defined</div>
		{:else}
			<div class="ports-list">
				{#each value.inputs ?? [] as port, i (port.name ?? i)}
					<div class="port-card" data-testid="input-port-{i}">
						<div class="port-header">
							<span class="port-name">{port.name ?? `input-${i}`}</span>
							<span class="port-type-badge">{port.type ?? 'unknown'}</span>
						</div>

						<!-- Editable fields -->
						{#each getEditableFields(port) as fieldName (fieldName)}
							<div class="port-field editable">
								<label for="input-{i}-{fieldName}">{fieldName}</label>
								<input
									id="input-{i}-{fieldName}"
									type="text"
									value={getFieldValue(port, fieldName)}
									oninput={(e) => handleFieldChange('inputs', i, fieldName, e.currentTarget.value)}
									data-testid="input-port-{i}-{fieldName}"
								/>
							</div>
						{/each}

						<!-- Read-only fields -->
						{#each getReadOnlyFields(port) as fieldName (fieldName)}
							<div class="port-field readonly">
								<span class="field-label">{fieldName}:</span>
								<span class="field-value">{getFieldValue(port, fieldName)}</span>
							</div>
						{/each}
					</div>
				{/each}
			</div>
		{/if}
	</section>

	<!-- Outputs Section -->
	<section class="port-section">
		<h5 class="section-header">
			<span class="section-title">Outputs</span>
			<span class="port-count">{outputCount}</span>
		</h5>

		{#if outputCount === 0}
			<div class="empty-ports">No output ports defined</div>
		{:else}
			<div class="ports-list">
				{#each value.outputs ?? [] as port, i (port.name ?? i)}
					<div class="port-card" data-testid="output-port-{i}">
						<div class="port-header">
							<span class="port-name">{port.name ?? `output-${i}`}</span>
							<span class="port-type-badge">{port.type ?? 'unknown'}</span>
						</div>

						<!-- Editable fields -->
						{#each getEditableFields(port) as fieldName (fieldName)}
							<div class="port-field editable">
								<label for="output-{i}-{fieldName}">{fieldName}</label>
								<input
									id="output-{i}-{fieldName}"
									type="text"
									value={getFieldValue(port, fieldName)}
									oninput={(e) => handleFieldChange('outputs', i, fieldName, e.currentTarget.value)}
									data-testid="output-port-{i}-{fieldName}"
								/>
							</div>
						{/each}

						<!-- Read-only fields -->
						{#each getReadOnlyFields(port) as fieldName (fieldName)}
							<div class="port-field readonly">
								<span class="field-label">{fieldName}:</span>
								<span class="field-value">{getFieldValue(port, fieldName)}</span>
							</div>
						{/each}
					</div>
				{/each}
			</div>
		{/if}
	</section>
</div>

<style>
	.ports-editor {
		display: flex;
		flex-direction: column;
		gap: 1rem;
	}

	.port-section {
		border: 1px solid var(--ui-border-subtle);
		border-radius: 6px;
		overflow: hidden;
	}

	.section-header {
		display: flex;
		justify-content: space-between;
		align-items: center;
		margin: 0;
		padding: 0.5rem 0.75rem;
		background: var(--ui-surface-tertiary);
		border-bottom: 1px solid var(--ui-border-subtle);
		font-size: 0.75rem;
		font-weight: 600;
		text-transform: uppercase;
		color: var(--ui-text-secondary);
	}

	.port-count {
		background: var(--ui-surface-secondary);
		padding: 0.125rem 0.5rem;
		border-radius: 10px;
		font-size: 0.6875rem;
	}

	.empty-ports {
		padding: 1rem;
		text-align: center;
		color: var(--ui-text-tertiary);
		font-size: 0.8125rem;
		font-style: italic;
	}

	.ports-list {
		display: flex;
		flex-direction: column;
		gap: 0.5rem;
		padding: 0.5rem;
	}

	.port-card {
		background: var(--ui-surface-primary);
		border: 1px solid var(--ui-border-subtle);
		border-radius: 4px;
		padding: 0.75rem;
	}

	.port-header {
		display: flex;
		justify-content: space-between;
		align-items: center;
		margin-bottom: 0.5rem;
		padding-bottom: 0.5rem;
		border-bottom: 1px solid var(--ui-border-subtle);
	}

	.port-name {
		font-weight: 600;
		font-size: 0.875rem;
		color: var(--ui-text-primary);
	}

	.port-type-badge {
		font-size: 0.6875rem;
		padding: 0.125rem 0.5rem;
		background: var(--ui-interactive-primary);
		color: var(--ui-text-on-primary);
		border-radius: 3px;
		text-transform: lowercase;
	}

	.port-field {
		margin-bottom: 0.5rem;
	}

	.port-field:last-child {
		margin-bottom: 0;
	}

	.port-field.editable label {
		display: block;
		font-size: 0.6875rem;
		font-weight: 500;
		color: var(--ui-text-secondary);
		margin-bottom: 0.25rem;
		text-transform: capitalize;
	}

	.port-field.editable input {
		width: 100%;
		padding: 0.375rem 0.5rem;
		border: 1px solid var(--ui-border-subtle);
		border-radius: 3px;
		font-size: 0.8125rem;
		font-family: 'SF Mono', Monaco, 'Cascadia Code', monospace;
		background: var(--ui-surface-secondary);
		color: var(--ui-text-primary);
	}

	.port-field.editable input:focus {
		outline: none;
		border-color: var(--ui-interactive-primary);
		box-shadow: 0 0 0 2px var(--ui-focus-ring);
	}

	.port-field.readonly {
		display: flex;
		gap: 0.5rem;
		font-size: 0.75rem;
		color: var(--ui-text-tertiary);
	}

	.field-label {
		text-transform: capitalize;
	}

	.field-value {
		color: var(--ui-text-secondary);
		font-family: 'SF Mono', Monaco, 'Cascadia Code', monospace;
	}
</style>
