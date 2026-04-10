<script lang="ts">
	/**
	 * JsonEditor - Fallback component for raw JSON editing.
	 * T094: Used when schema is missing or for complex types (object/array).
	 */
	import type { ConfigValue } from '$lib/types/config';

	interface JsonEditorProps {
		/** Current configuration object */
		config?: Record<string, ConfigValue>;
		/** Warning message to display */
		warning?: string;
		/** Parse error (bindable) - exposed to parent */
		parseError?: string | null;
	}

	let { config = $bindable({}), warning, parseError = $bindable(null) }: JsonEditorProps = $props();

	// User-edited text (undefined means "use derived from config")
	let userText = $state<string | undefined>(undefined);

	// Derive the JSON string from config when not being user-edited
	const derivedJsonString = $derived.by(() => {
		try {
			return JSON.stringify(config, null, 2);
		} catch {
			return '{}';
		}
	});

	// The actual textarea value: user edits take precedence, otherwise follow config
	const jsonString = $derived(userText !== undefined ? userText : derivedJsonString);

	// Handle JSON text changes
	function handleInput(event: Event) {
		const target = event.target as HTMLTextAreaElement;
		userText = target.value;

		try {
			const parsed = JSON.parse(userText) as Record<string, ConfigValue>;
			config = parsed;
			// Reset userText so textarea follows config again
			userText = undefined;
			parseError = null;
		} catch (e) {
			parseError = e instanceof Error ? e.message : 'Invalid JSON';
		}
	}
</script>

<div class="json-editor">
	{#if warning}
		<div class="warning" role="alert">
			⚠️ {warning}
		</div>
	{/if}

	<label for="json-config">Configuration (JSON)</label>
	<textarea
		id="json-config"
		value={jsonString}
		oninput={handleInput}
		rows="15"
		aria-invalid={parseError ? 'true' : undefined}
		aria-describedby={parseError ? 'json-error' : undefined}
	></textarea>

	{#if parseError}
		<span class="error" id="json-error" role="alert">{parseError}</span>
	{/if}

	<p class="help-text">Enter valid JSON configuration for this component.</p>
</div>

<style>
	.json-editor {
		display: flex;
		flex-direction: column;
		gap: 0.5rem;
	}

	.warning {
		padding: 0.75rem 1rem;
		background-color: var(--status-warning-container);
		border: 1px solid var(--status-warning);
		border-radius: 0.25rem;
		color: var(--status-warning-on-container);
		font-size: 0.875rem;
	}

	label {
		font-weight: 500;
		margin-bottom: 0.25rem;
	}

	textarea {
		font-family: 'Courier New', monospace;
		font-size: 0.875rem;
		width: 100%;
		padding: 0.75rem;
		border: 1px solid var(--ui-border-subtle);
		border-radius: 0.25rem;
		resize: vertical;
	}

	textarea[aria-invalid='true'] {
		border-color: var(--status-error);
	}

	.error {
		display: block;
		font-size: 0.875rem;
		color: var(--status-error);
	}

	.help-text {
		font-size: 0.875rem;
		color: var(--ui-text-secondary);
		margin: 0;
	}
</style>
