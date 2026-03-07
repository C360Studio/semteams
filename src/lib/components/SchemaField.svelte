<script lang="ts">
	import type { PropertySchema } from '$lib/types/schema';
	import type { ConfigValue } from '$lib/types/config';
	import StringField from './StringField.svelte';
	import NumberField from './NumberField.svelte';
	import BooleanField from './BooleanField.svelte';
	import EnumField from './EnumField.svelte';
	import PortConfigEditor from './PortConfigEditor.svelte';

	/**
	 * SchemaField - Router component that renders the appropriate field type.
	 * T084: Routes to type-specific components based on PropertySchema.type.
	 */
	interface SchemaFieldProps {
		/** Field name */
		name: string;
		/** Display label (optional, defaults to name) */
		label?: string;
		/** PropertySchema definition */
		schema: PropertySchema;
		/** Current value (type varies by field type) */
		value?: ConfigValue;
		/** Validation error message */
		error?: string;
		/** Whether field is required */
		isRequired?: boolean;
		/** Callback when value changes */
		onChange?: (value: ConfigValue) => void;
	}

	import type { PortDefinition } from '$lib/types/component';

	let { name, label, schema, value = $bindable(undefined), error, isRequired = false, onChange }: SchemaFieldProps = $props();

	// Typed intermediaries — SchemaField narrows by schema.type in the template,
	// but Svelte can't propagate that narrowing into bind: expressions.
	// These getters/setters cast ConfigValue to the specific sub-field type.
	let stringValue = {
		get value(): string | undefined { return value as string | undefined; },
		set value(v: string | undefined) { value = v; }
	};
	let numberValue = {
		get value(): number | undefined { return value as number | undefined; },
		set value(v: number | undefined) { value = v; }
	};
	let boolValue = {
		get value(): boolean | undefined { return value as boolean | undefined; },
		set value(v: boolean | undefined) { value = v; }
	};
	let portValue = {
		get value(): { inputs?: PortDefinition[]; outputs?: PortDefinition[] } | undefined {
			return value as { inputs?: PortDefinition[]; outputs?: PortDefinition[] } | undefined;
		},
		set value(v: { inputs?: PortDefinition[]; outputs?: PortDefinition[] } | undefined) {
			value = v as ConfigValue;
		}
	};
</script>

{#if schema.type === 'string'}
	<StringField {name} {label} {schema} bind:value={stringValue.value} {error} {isRequired} onChange={(v) => onChange?.(v)} />
{:else if schema.type === 'int' || schema.type === 'float'}
	<NumberField {name} {label} {schema} bind:value={numberValue.value} {error} {isRequired} onChange={(v) => onChange?.(v as ConfigValue)} />
{:else if schema.type === 'bool'}
	<BooleanField {name} {label} {schema} bind:value={boolValue.value} {error} {isRequired} onChange={(v) => onChange?.(v)} />
{:else if schema.type === 'enum'}
	<EnumField {name} {label} {schema} bind:value={stringValue.value} {error} {isRequired} onChange={(v) => onChange?.(v)} />
{:else if schema.type === 'ports'}
	<PortConfigEditor {name} {schema} bind:value={portValue.value} {error} {isRequired} onChange={(v) => onChange?.(v as ConfigValue)} />
{:else if schema.type === 'object' || schema.type === 'array'}
	<div class="field complex-field-fallback">
		<label for={name}>
			{label ?? name}
			{#if isRequired}
				<span class="required">*</span>
			{/if}
		</label>
		<div class="fallback-message">
			<p>⚠️ This field requires complex configuration. Use JSON editor.</p>
			{#if schema.description}
				<p class="description">{schema.description}</p>
			{/if}
		</div>
		{#if error}
			<span class="error" role="alert">{error}</span>
		{/if}
	</div>
{:else}
	<div class="field unknown-field-type">
		<label for={name}>{label ?? name}</label>
		<div class="fallback-message">
			<p>⚠️ Unknown field type: {schema.type}</p>
		</div>
	</div>
{/if}

<style>
	.field {
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

	.fallback-message {
		padding: 1rem;
		background-color: var(--ui-surface-elevated-1);
		border: 1px solid var(--ui-border-subtle);
		border-radius: 0.25rem;
		margin-top: 0.5rem;
	}

	.fallback-message p {
		margin: 0;
		font-size: 0.875rem;
	}

	.fallback-message .description {
		margin-top: 0.5rem;
		color: var(--ui-text-secondary);
	}

	.error {
		display: block;
		font-size: 0.875rem;
		color: var(--status-error);
		margin-top: 0.25rem;
	}
</style>
