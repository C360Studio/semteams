<script lang="ts">
	import type { PropertySchema } from '$lib/types/schema';

	/**
	 * NumberField - Number input component for int/float schema fields.
	 * T081: Implements number field rendering with min/max validation attributes.
	 */
	interface NumberFieldProps {
		/** Field name (used for id, label association) */
		name: string;
		/** Display label (optional, defaults to name) */
		label?: string;
		/** PropertySchema definition for this field */
		schema: PropertySchema;
		/** Current field value */
		value?: number;
		/** Validation error message to display */
		error?: string;
		/** Whether this field is required */
		isRequired?: boolean;
		/** Callback when value changes (undefined when field cleared) */
		onChange?: (value: number | undefined) => void;
	}

	let { name, label, schema, value = $bindable(), error, isRequired = false, onChange }: NumberFieldProps = $props();

	// Use label if provided, otherwise fall back to name
	const displayLabel = $derived(label ?? name);

	// Determine step based on type
	let step = $derived(schema.type === 'float' ? 'any' : '1');

	// Ensure value is compatible with number input
	let inputValue = $derived(value ?? '');
</script>

<div class="field">
	<label for={name}>
		{displayLabel}
		{#if isRequired}
			<span class="required">*</span>
		{/if}
	</label>

	<input
		type="number"
		id={name}
		value={inputValue}
		oninput={(e) => {
			const val = e.currentTarget.value;
			value = val === '' ? undefined : schema.type === 'float' ? parseFloat(val) : parseInt(val, 10);
			onChange?.(value);
		}}
		min={schema.minimum}
		max={schema.maximum}
		{step}
		aria-required={isRequired ? 'true' : undefined}
		aria-invalid={error ? 'true' : undefined}
		aria-describedby={error ? `${name}-error` : schema.description ? `${name}-description` : undefined}
	/>

	{#if schema.description}
		<span class="description" id="{name}-description">{schema.description}</span>
	{/if}

	{#if error}
		<span class="error" id="{name}-error" role="alert">{error}</span>
	{/if}
</div>

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

	input {
		width: 100%;
	}

	.description {
		display: block;
		font-size: 0.875rem;
		color: var(--ui-text-secondary);
		margin-top: 0.25rem;
	}

	.error {
		display: block;
		font-size: 0.875rem;
		color: var(--status-error);
		margin-top: 0.25rem;
	}
</style>
