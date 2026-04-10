<script lang="ts">
	import type { PropertySchema } from '$lib/types/schema';

	/**
	 * BooleanField - Checkbox input component for boolean schema fields.
	 * T082: Implements boolean field rendering with checkbox.
	 */
	interface BooleanFieldProps {
		/** Field name (used for id, label association) */
		name: string;
		/** Display label (optional, defaults to name) */
		label?: string;
		/** PropertySchema definition for this field */
		schema: PropertySchema;
		/** Current field value */
		value?: boolean;
		/** Validation error message to display */
		error?: string;
		/** Whether this field is required */
		isRequired?: boolean;
		/** Callback when value changes */
		onChange?: (value: boolean) => void;
	}

	let { name, label, schema, value = $bindable(), error, isRequired = false, onChange }: BooleanFieldProps = $props();

	// Use label if provided, otherwise fall back to name
	const displayLabel = $derived(label ?? name);

	// Ensure value defaults to false if undefined
	let checked = $derived(value ?? false);
</script>

<div class="field checkbox-field">
	<label for={name}>
		<input
			type="checkbox"
			id={name}
			checked={checked}
			onchange={(e) => {
				value = e.currentTarget.checked;
				onChange?.(value);
			}}
			aria-required={isRequired ? 'true' : undefined}
			aria-invalid={error ? 'true' : undefined}
			aria-describedby={error ? `${name}-error` : schema.description ? `${name}-description` : undefined}
		/>
		{displayLabel}
		{#if isRequired}
			<span class="required">*</span>
		{/if}
	</label>

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

	.checkbox-field label {
		display: flex;
		align-items: center;
		font-weight: 500;
		cursor: pointer;
	}

	.checkbox-field input[type='checkbox'] {
		width: auto;
		margin-right: 0.5rem;
	}

	.required {
		color: var(--status-error);
		margin-left: 0.25rem;
	}

	.description {
		display: block;
		font-size: 0.875rem;
		color: var(--ui-text-secondary);
		margin-top: 0.25rem;
		margin-left: 1.75rem; /* Align with checkbox label text */
	}

	.error {
		display: block;
		font-size: 0.875rem;
		color: var(--status-error);
		margin-top: 0.25rem;
		margin-left: 1.75rem; /* Align with checkbox label text */
	}
</style>
