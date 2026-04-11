<script lang="ts">
	/**
	 * AIPromptInput Component
	 *
	 * Prompt input component for describing flows in natural language.
	 * Allows users to enter a description of the flow they want to create,
	 * with keyboard shortcuts, character counting, and validation.
	 */

	interface Props {
		disabled?: boolean;
		loading?: boolean;
		placeholder?: string;
		maxLength?: number;
		onSubmit?: (prompt: string) => void;
		onCancel?: () => void;
	}

	let {
		disabled = false,
		loading = false,
		placeholder = 'Describe your flow in natural language...',
		maxLength = 2000,
		onSubmit,
		onCancel
	}: Props = $props();

	// Reactive state
	let promptText = $state('');
	let textareaElement = $state<HTMLTextAreaElement | null>(null);

	// Derived values
	let trimmedPrompt = $derived(promptText.trim());
	let charCount = $derived(promptText.length);
	let isValid = $derived(trimmedPrompt.length > 0 && charCount <= maxLength);
	let isSubmitDisabled = $derived(!isValid || disabled || loading);
	let nearLimit = $derived(charCount >= maxLength * 0.95);
	let atLimit = $derived(charCount >= maxLength);
	let exceedsLimit = $derived(charCount > maxLength);

	// Button text based on loading state
	let submitButtonText = $derived(loading ? 'Generating...' : 'Generate Flow');

	/**
	 * Handle textarea input
	 */
	function handleInput(event: Event) {
		const target = event.target as HTMLTextAreaElement;
		promptText = target.value;
	}

	/**
	 * Handle form submission
	 */
	function handleSubmit() {
		if (isSubmitDisabled) return;

		if (onSubmit) {
			onSubmit(trimmedPrompt);
		}
	}

	/**
	 * Handle cancel action
	 */
	function handleCancel() {
		promptText = '';
		if (onCancel) {
			onCancel();
		}
	}

	/**
	 * Handle keyboard shortcuts
	 */
	function handleKeyDown(event: KeyboardEvent) {
		// Ctrl+Enter or Cmd+Enter to submit
		if ((event.ctrlKey || event.metaKey) && event.key === 'Enter') {
			event.preventDefault();
			handleSubmit();
		}

		// Escape to cancel
		if (event.key === 'Escape') {
			event.preventDefault();
			handleCancel();
		}
	}
</script>

<div class="ai-prompt-input">
	<div class="input-container">
		<textarea
			bind:this={textareaElement}
			value={promptText}
			oninput={handleInput}
			onkeydown={handleKeyDown}
			{placeholder}
			disabled={disabled || loading}
			aria-label="Describe your flow in natural language"
			aria-disabled={disabled || loading}
			rows="5"
		></textarea>

		{#if exceedsLimit}
			<div class="validation-message error" role="alert">
				Prompt exceeds maximum length of {maxLength} characters
			</div>
		{/if}

		<div class="character-count" aria-live="polite">
			<span class:warning={nearLimit} class:error={atLimit && !exceedsLimit}>
				{charCount}{#if maxLength} / {maxLength}{/if}
			</span>
		</div>
	</div>

	<div class="actions">
		<button
			type="button"
			onclick={handleSubmit}
			disabled={isSubmitDisabled}
			aria-busy={loading}
			class="primary"
		>
			{#if loading}
				<span class="spinner" role="status" aria-label="Loading"></span>
			{/if}
			{submitButtonText}
		</button>

		<button type="button" onclick={handleCancel} disabled={disabled || loading}>
			Cancel
		</button>
	</div>
</div>

<style>
	.ai-prompt-input {
		display: flex;
		flex-direction: column;
		gap: 1rem;
		width: 100%;
	}

	.input-container {
		position: relative;
		width: 100%;
	}

	textarea {
		width: 100%;
		min-height: 120px;
		padding: 0.75rem;
		font-family: inherit;
		font-size: 1rem;
		line-height: 1.5;
		border: 1px solid var(--ui-border-subtle);
		border-radius: var(--radius-md);
		background-color: var(--ui-surface-primary);
		color: var(--ui-text-primary);
		resize: vertical;
		transition: border-color 0.2s ease;
	}

	textarea:focus {
		outline: none;
		border-color: var(--ui-interactive-primary);
		box-shadow: 0 0 0 2px var(--ui-focus-ring);
	}

	textarea:disabled {
		opacity: 0.6;
		cursor: not-allowed;
	}

	.character-count {
		position: absolute;
		bottom: 0.5rem;
		right: 0.75rem;
		font-size: 0.875rem;
		color: var(--ui-text-secondary);
		pointer-events: none;
	}

	.character-count .warning {
		color: var(--status-warning);
	}

	.character-count .error {
		color: var(--status-error);
		font-weight: 600;
	}

	.validation-message {
		margin-top: 0.5rem;
		padding: 0.5rem;
		font-size: 0.875rem;
		color: var(--status-error);
		background-color: var(--status-error-container);
		border-radius: var(--radius-md);
		border-left: 3px solid var(--status-error);
	}

	.actions {
		display: flex;
		gap: 0.75rem;
		justify-content: flex-end;
	}

	button {
		display: flex;
		align-items: center;
		gap: 0.5rem;
		padding: 0.5rem 1rem;
		font-size: 1rem;
		border-radius: var(--radius-md);
		cursor: pointer;
		transition: all 0.2s ease;
	}

	button:disabled {
		opacity: 0.6;
		cursor: not-allowed;
	}

	button.primary {
		background-color: var(--ui-interactive-primary);
		color: var(--ui-text-on-primary);
		border: none;
	}

	button.primary:hover:not(:disabled) {
		background-color: var(--ui-interactive-primary-hover);
	}

	.spinner {
		display: inline-block;
		width: 1rem;
		height: 1rem;
		border: 2px solid rgba(255, 255, 255, 0.3);
		border-top-color: white;
		border-radius: 50%;
		animation: spin 0.8s linear infinite;
	}

	@keyframes spin {
		to {
			transform: rotate(360deg);
		}
	}
</style>
