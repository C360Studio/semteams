<script lang="ts">
	import { untrack } from 'svelte';
	import type { ConfigSchema } from '$lib/types/schema';
	import type { ConfigValue } from '$lib/types/config';
	import { validateField } from '$lib/validation/schema-validator';
	import SchemaField from './SchemaField.svelte';

	/**
	 * SchemaForm - Schema-driven form component.
	 * T085-T091: Implements complete schema-driven form with validation, defaults, callbacks.
	 */
	interface SchemaFormProps {
		/** ConfigSchema definition */
		schema: ConfigSchema;
		/** Current configuration values (bindable for two-way sync) */
		config?: Record<string, ConfigValue>;
		/** External validation errors from backend (T097) */
		externalErrors?: Record<string, string>;
		/** Saving state for UI feedback (T098) */
		saving?: boolean;
		/** Callback when form is saved */
		onSave?: (config: Record<string, ConfigValue>) => void;
		/** Callback when form is cancelled */
		onCancel?: () => void;
	}

	// T007: Make config bindable for two-way sync with parent
	let { schema, config = $bindable({}), externalErrors = {}, saving = false, onSave, onCancel }: SchemaFormProps = $props();

	// Validation errors keyed by field name (local validation only)
	let validationErrors = $state<Record<string, string>>({});

	// T097: Combine local and external errors for display
	let combinedErrors = $derived({
		...validationErrors,
		...externalErrors
	});

	// Debounce timer for real-time validation
	let debounceTimers = $state<Record<string, ReturnType<typeof setTimeout>>>({});

	// T086: Pre-fill defaults for fields not in config (on mount or schema change)
	// Use untrack to avoid re-running this effect when config values change
	$effect(() => {
		// Track schema changes only
		const properties = schema.properties;

		// Apply defaults without tracking config reads
		for (const [fieldName, propSchema] of Object.entries(properties)) {
			const currentValue = untrack(() => config[fieldName]);
			if (currentValue === undefined && propSchema.default !== undefined) {
				config[fieldName] = propSchema.default as ConfigValue;
			}
		}
	});

	// T085: Separate fields into basic and advanced categories
	// Basic fields: category === 'basic'
	// Advanced fields: category !== 'basic' (includes undefined, 'advanced', etc.)
	let basicFields = $derived(
		Object.entries(schema.properties)
			.filter(([_, prop]) => prop.category === 'basic')
			.sort(([a], [b]) => a.localeCompare(b)) // Alphabetical within category
	);

	let advancedFields = $derived(
		Object.entries(schema.properties)
			.filter(([_, prop]) => prop.category !== 'basic')
			.sort(([a], [b]) => a.localeCompare(b)) // Alphabetical within category
	);

	// T089: Debounced real-time validation (300ms)
	function handleFieldChange(fieldName: string, value: ConfigValue) {
		config[fieldName] = value;

		// Clear existing timer
		if (debounceTimers[fieldName]) {
			clearTimeout(debounceTimers[fieldName]);
		}

		// Set new timer for validation
		debounceTimers[fieldName] = setTimeout(() => {
			const propSchema = schema.properties[fieldName];
			const isRequired = schema.required.includes(fieldName);
			const error = validateField(fieldName, value, propSchema, isRequired);

			if (error) {
				validationErrors[fieldName] = error.message;
			} else {
				delete validationErrors[fieldName];
			}

			// Trigger reactivity
			validationErrors = { ...validationErrors };
		}, 300);
	}

	// T088, T091: Handle form submission with validation
	function handleSubmit(event: Event) {
		event.preventDefault();

		// Clear previous errors
		const newErrors: Record<string, string> = {};

		// Validate all fields
		for (const [fieldName, propSchema] of Object.entries(schema.properties)) {
			const isRequired = schema.required.includes(fieldName);
			const error = validateField(fieldName, config[fieldName], propSchema, isRequired);

			if (error) {
				newErrors[fieldName] = error.message;
			}
		}

		// Trigger reactivity by reassigning
		validationErrors = newErrors;

		// T091: Prevent submission if validation errors exist
		if (Object.keys(validationErrors).length > 0) {
			return;
		}

		// T087: Call onSave callback with config
		onSave?.(config);
	}

	// Handle cancel
	function handleCancel() {
		onCancel?.();
	}
</script>

<form class="schema-form" onsubmit={handleSubmit}>
	<!-- T085: Basic Configuration Section -->
	{#if basicFields.length > 0}
		<section class="basic-config">
			<h3>Basic Configuration</h3>
			{#each basicFields as [fieldName, propSchema] (fieldName)}
				<SchemaField
					name={fieldName}
					schema={propSchema}
					bind:value={config[fieldName]}
					error={combinedErrors[fieldName]}
					isRequired={schema.required.includes(fieldName)}
					onChange={(value) => handleFieldChange(fieldName, value)}
				/>
			{/each}
		</section>
	{/if}

	<!-- T085: Advanced Configuration Section (collapsible) -->
	{#if advancedFields.length > 0}
		<details class="advanced-config">
			<summary>Advanced Configuration</summary>
			{#each advancedFields as [fieldName, propSchema] (fieldName)}
				<SchemaField
					name={fieldName}
					schema={propSchema}
					bind:value={config[fieldName]}
					error={combinedErrors[fieldName]}
					isRequired={schema.required.includes(fieldName)}
					onChange={(value) => handleFieldChange(fieldName, value)}
				/>
			{/each}
		</details>
	{/if}

	<!-- Form Actions (T098: Disabled state during save) -->
	<div class="actions">
		<button
			type="submit"
			disabled={saving || Object.keys(combinedErrors).length > 0}
			aria-busy={saving ? 'true' : 'false'}
		>
			{saving ? 'Saving...' : 'Save'}
		</button>
		<button type="button" onclick={handleCancel} disabled={saving}>Cancel</button>
	</div>
</form>

<style>
	form {
		display: flex;
		flex-direction: column;
		gap: 1.5rem;
	}

	section,
	details {
		padding: 1rem;
		background-color: var(--ui-surface-elevated-1);
		border: 1px solid var(--ui-border-subtle);
		border-radius: 0.25rem;
	}

	h3 {
		margin-top: 0;
		margin-bottom: 1rem;
		font-size: 1.125rem;
		font-weight: 600;
	}

	details summary {
		cursor: pointer;
		font-weight: 600;
		margin-bottom: 1rem;
		user-select: none;
	}

	details[open] summary {
		margin-bottom: 1rem;
	}

	.actions {
		display: flex;
		gap: 0.5rem;
		justify-content: flex-end;
	}

	.actions button {
		margin-bottom: 0;
	}

	/* T099: Disabled and saving state styling */
	button:disabled {
		opacity: 0.6;
		cursor: not-allowed;
	}

	button[aria-busy='true'] {
		cursor: wait;
	}
</style>
