<script lang="ts">
	import type { PropertySchema } from '$lib/types/schema';

	/**
	 * StringField - Text input component for string schema fields.
	 * T080: Implements string field rendering with label, description, value, error.
	 */
	interface StringFieldProps {
		/** Field name (used for id, label association) */
		name: string;
		/** Display label (optional, defaults to name) */
		label?: string;
		/** PropertySchema definition for this field */
		schema: PropertySchema;
		/** Current field value */
		value?: string;
		/** Validation error message to display */
		error?: string;
		/** Whether this field is required */
		isRequired?: boolean;
		/** Callback when value changes */
		onChange?: (value: string) => void;
	}

	let { name, label, schema, value = $bindable(), error, isRequired = false, onChange }: StringFieldProps = $props();

	// Use label if provided, otherwise fall back to name
	const displayLabel = $derived(label ?? name);

	// Ensure value is never undefined for the input
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
		type="text"
		id={name}
		value={inputValue}
		oninput={(e) => {
			value = e.currentTarget.value;
			onChange?.(value);
		}}
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
