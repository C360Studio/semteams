<script lang="ts">
	import type { PropertySchema } from '$lib/types/schema';
	import type { PortDefinition } from '$lib/types/component';

	/**
	 * PortConfigEditor - Schema-driven port configuration editor.
	 *
	 * Specialized component for editing port configurations (inputs/outputs arrays)
	 * with field-level editability control based on schema.portFields metadata.
	 *
	 * Only fields marked as editable in the schema are rendered as inputs.
	 * Readonly fields are displayed as labels with their values.
	 */
	interface PortConfigEditorProps {
		/** Field name (e.g., "ports") */
		name: string;
		/** PropertySchema with portFields metadata */
		schema: PropertySchema;
		/** Current port configuration */
		value?: { inputs?: PortDefinition[]; outputs?: PortDefinition[] };
		/** Validation error message */
		error?: string;
		/** Whether this field is required */
		isRequired?: boolean;
		/** Callback when ports change */
		onChange?: (value: { inputs?: PortDefinition[]; outputs?: PortDefinition[] }) => void;
	}

	let {
		name,
		schema,
		value = $bindable(),
		error,
		isRequired = false,
		onChange
	}: PortConfigEditorProps = $props();

	// Ensure value has proper structure
	let portConfig = $derived({
		inputs: value?.inputs || [],
		outputs: value?.outputs || []
	});

	// Get editable field names from schema
	let editableFields = $derived(
		Object.entries(schema.portFields || {})
			.filter(([_, info]) => info.editable)
			.map(([fieldName, _]) => fieldName)
	);

	// Create a new empty port
	function createEmptyPort(direction: 'input' | 'output'): PortDefinition {
		return {
			id: `${direction}_${Date.now()}`,
			name: `new_${direction}`,
			direction,
			required: false,
			description: '',
			config: {
				type: 'nats',
				nats: {
					subject: '',
					interface: { type: 'message.Storable' }
				}
			}
		};
	}

	// Add input port
	function addInputPort() {
		const newPort = createEmptyPort('input');
		const updated = {
			inputs: [...portConfig.inputs, newPort],
			outputs: portConfig.outputs
		};
		value = updated;
		onChange?.(updated);
	}

	// Add output port
	function addOutputPort() {
		const newPort = createEmptyPort('output');
		const updated = {
			inputs: portConfig.inputs,
			outputs: [...portConfig.outputs, newPort]
		};
		value = updated;
		onChange?.(updated);
	}

	// Remove input port by index
	function removeInputPort(index: number) {
		const updated = {
			inputs: portConfig.inputs.filter((_, i) => i !== index),
			outputs: portConfig.outputs
		};
		value = updated;
		onChange?.(updated);
	}

	// Remove output port by index
	function removeOutputPort(index: number) {
		const updated = {
			inputs: portConfig.inputs,
			outputs: portConfig.outputs.filter((_, i) => i !== index)
		};
		value = updated;
		onChange?.(updated);
	}

	// Update a port field value
	function updatePortField(
		direction: 'input' | 'output',
		index: number,
		fieldPath: string,
		newValue: string
	) {
		const ports = direction === 'input' ? [...portConfig.inputs] : [...portConfig.outputs];
		const port = { ...ports[index] };

		// Handle nested config fields (e.g., "config.nats.subject")
		if (fieldPath.startsWith('config.')) {
			const pathParts = fieldPath.split('.');
			if (pathParts.length === 3 && pathParts[1] === 'nats' && port.config.nats) {
				// e.g., "config.nats.subject"
				const configField = pathParts[2];
				port.config = {
					...port.config,
					nats: {
						...port.config.nats,
						[configField]: newValue
					}
				};
			} else if (
				pathParts.length === 3 &&
				pathParts[1] === 'jetstream' &&
				port.config.jetstream
			) {
				// e.g., "config.jetstream.stream_name"
				const configField = pathParts[2];
				port.config = {
					...port.config,
					jetstream: {
						...port.config.jetstream,
						[configField]: newValue
					}
				};
			}
		} else {
			// Top-level field (e.g., "name", "description")
			(port as Record<string, unknown>)[fieldPath] = newValue;
		}

		ports[index] = port;

		const updated =
			direction === 'input'
				? { inputs: ports, outputs: portConfig.outputs }
				: { inputs: portConfig.inputs, outputs: ports };

		value = updated;
		onChange?.(updated);
	}

	// Get display value for a port field
	function getPortFieldValue(port: PortDefinition, fieldName: string): string {
		// Map common field names to their actual locations in the PortDefinition
		switch (fieldName) {
			case 'name':
				return port.name;
			case 'type':
				return port.config.type;
			case 'required':
				return port.required ? 'Yes' : 'No';
			case 'description':
				return port.description;
			case 'interface':
				return port.config.nats?.interface?.type || port.config.jetstream?.interface?.type || '';
			case 'subject':
				return port.config.nats?.subject || '';
			case 'timeout':
				return port.config.natsRequest?.timeout || '';
			case 'stream_name':
				return port.config.jetstream?.streamName || '';
			default:
				return '';
		}
	}

	// Get the field path for updates (handles nested config fields)
	function getFieldPath(fieldName: string): string {
		switch (fieldName) {
			case 'subject':
				return 'config.nats.subject';
			case 'timeout':
				return 'config.natsRequest.timeout';
			case 'stream_name':
				return 'config.jetstream.stream_name';
			default:
				return fieldName;
		}
	}

	// Check if a field is editable based on schema
	function isFieldEditable(fieldName: string): boolean {
		return editableFields.includes(fieldName);
	}

	// Get all field names from portFields schema
	let allFieldNames = $derived(Object.keys(schema.portFields || {}));
</script>

<div class="port-config-editor" data-testid="port-config-editor">
	<label for={name}>
		{name}
		{#if isRequired}
			<span class="required">*</span>
		{/if}
	</label>

	{#if schema.description}
		<span class="description">{schema.description}</span>
	{/if}

	<!-- Input Ports Section -->
	<section class="port-section">
		<h4>Input Ports</h4>
		{#if portConfig.inputs.length === 0}
			<p class="empty-state">No input ports configured</p>
		{:else}
			<div class="port-list">
				{#each portConfig.inputs as port, index (port.id)}
					<div class="port-item">
						<div class="port-fields">
							{#each allFieldNames as fieldName (fieldName)}
								<div class="port-field">
									{#if isFieldEditable(fieldName)}
										<label class="field-label" for="{name}-input-{index}-{fieldName}">{fieldName}</label>
										<input
											type="text"
											id="{name}-input-{index}-{fieldName}"
											value={getPortFieldValue(port, fieldName)}
											oninput={(e) =>
												updatePortField('input', index, getFieldPath(fieldName), e.currentTarget.value)}
											class="editable-field"
										/>
									{:else}
										<span class="field-label">{fieldName}</span>
										<span class="readonly-field">{getPortFieldValue(port, fieldName)}</span>
									{/if}
								</div>
							{/each}
						</div>
						<button type="button" onclick={() => removeInputPort(index)} class="remove-button">
							Remove
						</button>
					</div>
				{/each}
			</div>
		{/if}
		<button type="button" onclick={addInputPort} class="add-button">Add Input Port</button>
	</section>

	<!-- Output Ports Section -->
	<section class="port-section">
		<h4>Output Ports</h4>
		{#if portConfig.outputs.length === 0}
			<p class="empty-state">No output ports configured</p>
		{:else}
			<div class="port-list">
				{#each portConfig.outputs as port, index (port.id)}
					<div class="port-item">
						<div class="port-fields">
							{#each allFieldNames as fieldName (fieldName)}
								<div class="port-field">
									{#if isFieldEditable(fieldName)}
										<label class="field-label" for="{name}-output-{index}-{fieldName}">{fieldName}</label>
										<input
											type="text"
											id="{name}-output-{index}-{fieldName}"
											value={getPortFieldValue(port, fieldName)}
											oninput={(e) =>
												updatePortField('output', index, getFieldPath(fieldName), e.currentTarget.value)}
											class="editable-field"
										/>
									{:else}
										<span class="field-label">{fieldName}</span>
										<span class="readonly-field">{getPortFieldValue(port, fieldName)}</span>
									{/if}
								</div>
							{/each}
						</div>
						<button type="button" onclick={() => removeOutputPort(index)} class="remove-button">
							Remove
						</button>
					</div>
				{/each}
			</div>
		{/if}
		<button type="button" onclick={addOutputPort} class="add-button">Add Output Port</button>
	</section>

	{#if error}
		<span class="error" role="alert">{error}</span>
	{/if}
</div>

<style>
	.port-config-editor {
		margin-bottom: 1rem;
	}

	label {
		display: block;
		margin-bottom: 0.25rem;
		font-weight: 500;
	}

	.required {
		color: var(--status-error);
		margin-left: 0.25rem;
	}

	.description {
		display: block;
		font-size: 0.875rem;
		color: var(--ui-text-secondary);
		margin-bottom: 1rem;
	}

	.port-section {
		margin-top: 1.5rem;
		padding: 1rem;
		background-color: var(--ui-surface-elevated-1);
		border: 1px solid var(--ui-border-subtle);
		border-radius: 0.25rem;
	}

	h4 {
		margin-top: 0;
		margin-bottom: 1rem;
		font-size: 1rem;
		font-weight: 600;
		color: var(--ui-interactive-primary);
	}

	.empty-state {
		font-size: 0.875rem;
		color: var(--ui-text-secondary);
		font-style: italic;
		margin: 0.5rem 0;
	}

	.port-list {
		display: flex;
		flex-direction: column;
		gap: 1rem;
	}

	.port-item {
		padding: 0.75rem;
		background-color: var(--ui-surface-tertiary);
		border: 1px solid var(--ui-border-subtle);
		border-radius: 0.25rem;
	}

	.port-fields {
		display: grid;
		grid-template-columns: repeat(auto-fit, minmax(200px, 1fr));
		gap: 0.75rem;
		margin-bottom: 0.75rem;
	}

	.port-field {
		display: flex;
		flex-direction: column;
		gap: 0.25rem;
	}

	.field-label {
		font-size: 0.75rem;
		font-weight: 500;
		text-transform: capitalize;
		color: var(--ui-text-secondary);
		margin: 0;
	}

	.editable-field {
		padding: 0.25rem 0.5rem;
		font-size: 0.875rem;
		border: 1px solid var(--ui-border-subtle);
		border-radius: 0.25rem;
		background-color: var(--ui-surface-primary);
	}

	.readonly-field {
		font-size: 0.875rem;
		color: var(--ui-text-primary);
		padding: 0.25rem 0;
		word-break: break-word;
	}

	.add-button {
		margin-top: 0.75rem;
		margin-bottom: 0;
		font-size: 0.875rem;
	}

	.remove-button {
		margin-bottom: 0;
		font-size: 0.875rem;
		background-color: var(--status-error);
		border-color: var(--status-error);
	}

	.remove-button:hover {
		background-color: var(--status-error-hover);
		border-color: var(--status-error-hover);
	}

	.error {
		display: block;
		font-size: 0.875rem;
		color: var(--status-error);
		margin-top: 0.5rem;
	}

	@media (max-width: 768px) {
		.port-fields {
			grid-template-columns: 1fr;
		}
	}
</style>
