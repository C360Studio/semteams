<script lang="ts">
	import type { SaveState } from '$lib/types/ui-state';
	import type { ValidationResult } from '$lib/types/port';

	interface Props {
		saveState: SaveState;
		onSave?: () => void;
		// NEW: Validation display props (Feature 015)
		validationResult?: ValidationResult | null;
		onValidationClick?: () => void;
	}

	let {
		saveState,
		onSave,
		validationResult,
		onValidationClick
	}: Props = $props();

	// Format timestamp for display
	function formatTime(date: Date | null): string {
		if (!date) return '';
		return new Intl.DateTimeFormat('en-US', {
			hour: 'numeric',
			minute: '2-digit',
			second: '2-digit'
		}).format(date);
	}

	// Get status text and icon
	const statusConfig = $derived.by(() => {
		switch (saveState.status) {
			case 'clean':
				return {
					text: 'Valid',
					icon: '✓',
					class: 'status-clean'
				};
			case 'draft':
				return {
					text: `Draft - ${saveState.error || 'has errors'}`,
					icon: '⚠',
					class: 'status-draft'
				};
			case 'dirty':
				return {
					text: 'Unsaved changes',
					icon: '●',
					class: 'status-dirty'
				};
			case 'saving':
				return {
					text: 'Saving...',
					icon: '⟳',
					class: 'status-saving'
				};
			case 'error':
				return {
					text: 'Save failed',
					icon: '⚠',
					class: 'status-error'
				};
		}
	});

	// NEW: Compute validation status display (Feature 015)
	// Trust backend validation result - no frontend override for blank canvas
	const validationStatusDisplay = $derived.by(() => {
		// No validation result yet
		if (!validationResult) {
			return {
				text: 'Validating...',
				icon: '⟳',
				class: 'validation-validating'
			};
		}

		// Has errors
		if (validationResult.errors.length > 0) {
			const count = validationResult.errors.length;
			return {
				text: `Draft - ${count} error${count > 1 ? 's' : ''}`,
				icon: '❌',
				class: 'validation-error'
			};
		}

		// Has warnings (but no errors)
		if (validationResult.warnings.length > 0) {
			const count = validationResult.warnings.length;
			return {
				text: `${count} warning${count > 1 ? 's' : ''}`,
				icon: '⚠️',
				class: 'validation-warning'
			};
		}

		// Valid (backend says no errors/warnings)
		return {
			text: 'Valid',
			icon: '✓',
			class: 'validation-valid'
		};
	});
</script>

<div id="save-status" class="save-status-indicator save-status" data-status={saveState.status}>
	<div class="status-content">
		<!-- NEW: Validation Status (Feature 015) - Clickable to open detail modal -->
		{#if validationResult !== undefined}
			<button
				type="button"
				class="validation-status {validationStatusDisplay.class}"
				data-testid="validation-status"
				onclick={onValidationClick}
				aria-label="View validation details"
			>
				<span class="status-icon">{validationStatusDisplay.icon}</span>
				<span class="status-text">{validationStatusDisplay.text}</span>
			</button>
		{/if}

		<!-- Save Status - Show alongside validation status (not mutually exclusive) -->
		{#if saveState.status === 'draft'}
		<span class="status-icon {statusConfig.class}" aria-label={statusConfig.text} role="img">
			{statusConfig.icon}
		</span>
		<span class="status-text">{statusConfig.text}</span>
	{:else if saveState.status === 'dirty'}
			<span class="status-icon {statusConfig.class}" aria-label={statusConfig.text} role="img">
				{statusConfig.icon}
			</span>
			<span class="status-text">{statusConfig.text}</span>
		{:else if saveState.status === 'saving'}
			<span class="status-icon {statusConfig.class}" aria-label={statusConfig.text} role="img">
				{statusConfig.icon}
			</span>
			<span class="status-text">{statusConfig.text}</span>
		{:else if saveState.status === 'error'}
			<span class="status-icon {statusConfig.class}" aria-label={statusConfig.text} role="img">
				{statusConfig.icon}
			</span>
			<span class="status-text">{statusConfig.text}</span>
		{/if}

		<!-- Last saved timestamp -->
		{#if saveState.lastSaved && (saveState.status === 'clean' || saveState.status === 'draft')}
			<span class="timestamp">saved at {formatTime(saveState.lastSaved)}</span>
		{/if}

		<!-- Save button -->
		{#if onSave}
			<button
				type="button"
				class="save-button"
				disabled={saveState.status === 'clean' || saveState.status === 'draft' || saveState.status === 'saving'}
				onclick={onSave}
				aria-label="Save flow"
			>
				Save
			</button>
		{/if}
	</div>

	<!-- Error message -->
	{#if saveState.status === 'error' && saveState.error}
		<div class="error-message" role="alert">
			{saveState.error}
		</div>
	{/if}
</div>

<style>
	.save-status-indicator {
		display: flex;
		flex-direction: column;
		gap: 0.5rem;
		padding: 0.5rem;
		background-color: var(--ui-surface-primary);
		border-radius: 4px;
	}

	.status-content {
		display: flex;
		align-items: center;
		gap: 0.5rem;
	}

	.status-icon {
		font-size: 1.2em;
		line-height: 1;
	}

	.status-clean {
		color: var(--status-success);
	}

	.status-draft {
		color: var(--status-warning);
	}

	.status-dirty {
		color: var(--status-warning);
		animation: pulse 2s ease-in-out infinite;
	}

	.status-saving {
		color: var(--ui-interactive-primary);
		animation: spin 1s linear infinite;
	}

	.status-error {
		color: var(--status-error);
	}

	.status-text {
		font-weight: 500;
	}

	.timestamp {
		font-size: 0.875em;
		color: var(--ui-text-secondary);
	}

	.save-button {
		margin-left: auto;
		padding: 0.25rem 0.75rem;
		font-size: 0.875em;
	}

	.save-button:disabled {
		opacity: 0.5;
		cursor: not-allowed;
	}

	.error-message {
		padding: 0.5rem;
		background-color: var(--status-error-container);
		color: var(--status-error);
		border-radius: 4px;
		font-size: 0.875em;
	}

	/* NEW: Validation status button (Feature 015) */
	.validation-status {
		display: flex;
		align-items: center;
		gap: 0.375rem;
		padding: 0.375rem 0.75rem;
		border: 1px solid;
		border-radius: 6px;
		background: white;
		font-size: 0.875rem;
		font-weight: 500;
		cursor: pointer;
		transition: all 0.2s;
	}

	.validation-status:hover {
		transform: translateY(-1px);
		box-shadow: 0 2px 4px rgba(0, 0, 0, 0.1);
	}

	.validation-status .status-icon {
		font-size: 1em;
	}

	.validation-valid {
		color: var(--status-success);
		border-color: var(--status-success);
		background: var(--status-success-container);
	}

	.validation-valid:hover {
		background: var(--status-success-hover);
	}

	.validation-error {
		color: var(--status-error);
		border-color: var(--status-error);
		background: var(--status-error-container);
	}

	.validation-error:hover {
		background: var(--status-error-hover);
	}

	.validation-warning {
		color: var(--status-warning);
		border-color: var(--status-warning);
		background: var(--status-warning-container);
	}

	.validation-warning:hover {
		background: var(--status-warning-hover);
	}

	.validation-validating {
		color: var(--ui-text-tertiary);
		border-color: var(--ui-border-strong);
		background: var(--ui-surface-tertiary);
	}

	.status-divider {
		color: var(--ui-text-secondary);
		font-weight: 300;
	}

	@keyframes pulse {
		0%,
		100% {
			opacity: 1;
		}
		50% {
			opacity: 0.5;
		}
	}

	@keyframes spin {
		from {
			transform: rotate(0deg);
		}
		to {
			transform: rotate(360deg);
		}
	}
</style>
