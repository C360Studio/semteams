<script lang="ts">
	import type { ValidationResult } from '$lib/types/port';

	interface Props {
		isOpen: boolean;
		validationResult: ValidationResult | null;
		onClose: () => void;
	}

	let { isOpen, validationResult, onClose }: Props = $props();

	let dialogElement: HTMLElement | null = $state(null);
	let previouslyFocusedElement: HTMLElement | null = null;

	// Helper function to resolve user-friendly component name
	// Backend sends node IDs - we look up the user-friendly name from nodes array
	function resolveComponentName(componentName: string, portName?: string): string {
		// If we have nodes available, try to find a user-friendly name
		if (validationResult?.nodes) {
			// First, try to look up by component name (which is usually the node ID)
			if (componentName && componentName.trim() !== '') {
				const node = validationResult.nodes.find((n) => n.id === componentName);
				if (node) {
					return node.name || node.id;
				}
			}

			// If that didn't work and we have a port name, look up by port
			if (portName) {
				const node = validationResult.nodes.find(
					(n) =>
						n.input_ports?.some((p) => p.name === portName) ||
						n.output_ports?.some((p) => p.name === portName)
				);
				if (node) {
					return node.name || node.id;
				}
			}
		}

		// If we have a component name but couldn't look it up, use it as-is
		if (componentName && componentName.trim() !== '') {
			return componentName;
		}

		// Fallback: Flow-level issue
		return 'Flow';
	}

	// Handle ESC key press
	function handleKeyDown(event: KeyboardEvent) {
		if (event.key === 'Escape' && isOpen) {
			onClose();
			return;
		}

		// Focus trap: Handle Tab key
		if (event.key === 'Tab' && isOpen && dialogElement) {
			const focusableElements = dialogElement.querySelectorAll(
				'button, [href], input, select, textarea, [tabindex]:not([tabindex="-1"])'
			);
			const focusableArray = Array.from(focusableElements) as HTMLElement[];

			if (focusableArray.length === 0) return;

			const firstElement = focusableArray[0];
			const lastElement = focusableArray[focusableArray.length - 1];

			if (event.shiftKey) {
				// Shift+Tab: Move backwards
				if (document.activeElement === firstElement) {
					event.preventDefault();
					lastElement.focus();
				}
			} else {
				// Tab: Move forwards
				if (document.activeElement === lastElement) {
					event.preventDefault();
					firstElement.focus();
				}
			}
		}
	}

	// Handle backdrop click
	function handleBackdropClick(event: MouseEvent) {
		// Only close if clicking directly on backdrop (not bubbling from dialog)
		if (event.target === event.currentTarget) {
			onClose();
		}
	}

	// Handle backdrop keyboard events for accessibility
	function handleBackdropKeyDown(event: KeyboardEvent) {
		if (event.key === 'Enter' || event.key === ' ') {
			if (event.target === event.currentTarget) {
				event.preventDefault();
				onClose();
			}
		}
	}

	// Manage focus when modal opens/closes
	$effect(() => {
		if (isOpen) {
			// Save currently focused element
			previouslyFocusedElement = document.activeElement as HTMLElement;

			// Focus the dialog element after a short delay to allow DOM update
			setTimeout(() => {
				if (dialogElement) {
					const firstButton = dialogElement.querySelector('button') as HTMLElement;
					if (firstButton) {
						firstButton.focus();
					} else {
						dialogElement.focus();
					}
				}
			}, 10);

			return () => {
				// Return focus to previously focused element
				if (previouslyFocusedElement) {
					previouslyFocusedElement.focus();
				}
			};
		}
	});
</script>

<svelte:window onkeydown={handleKeyDown} />

{#if isOpen && validationResult}
	<div
		class="modal-backdrop"
		data-testid="modal-backdrop"
		onclick={handleBackdropClick}
		onkeydown={handleBackdropKeyDown}
		role="presentation"
	>
		<div
			bind:this={dialogElement}
			class="modal-dialog"
			role="dialog"
			aria-label="Validation Details"
			tabindex="0"
			onclick={(e) => e.stopPropagation()}
			onkeydown={(e) => e.stopPropagation()}
		>
			<div class="modal-header">
				<h2>Validation Issues</h2>
				<button
					type="button"
					class="close-button"
					onclick={onClose}
					aria-label="Close"
				>
					✕
				</button>
			</div>

			<div class="modal-body">
				{#if validationResult.errors.length > 0}
					<section class="issues-section errors-section">
						<h3 class="section-title">
							<span class="section-icon">❌</span>
							Errors ({validationResult.errors.length})
						</h3>
						<ul class="issues-list">
							{#each validationResult.errors as error, index (error.component_name + '|' + (error.port_name ?? '') + '|' + error.message + '|' + index)}
								<li class="issue-item error-item">
									<div class="issue-header">
										<strong class="component-name">
											{resolveComponentName(error.component_name, error.port_name)}
										</strong>
										{#if error.port_name}
											<span class="port-name">Port: {error.port_name}</span>
										{/if}
									</div>
									<div class="issue-message">{error.message}</div>
									{#if error.suggestions && error.suggestions.length > 0}
										<div class="issue-suggestion">
											<span class="suggestion-icon">💡</span>
											{error.suggestions[0]}
										</div>
									{/if}
								</li>
							{/each}
						</ul>
					</section>
				{/if}

				{#if validationResult.warnings.length > 0}
					<section class="issues-section warnings-section">
						<h3 class="section-title">
							<span class="section-icon">⚠️</span>
							Warnings ({validationResult.warnings.length})
						</h3>
						<ul class="issues-list">
							{#each validationResult.warnings as warning, index (warning.component_name + '|' + (warning.port_name ?? '') + '|' + warning.message + '|' + index)}
								<li class="issue-item warning-item">
									<div class="issue-header">
										<strong class="component-name">
											{resolveComponentName(warning.component_name, warning.port_name)}
										</strong>
										{#if warning.port_name}
											<span class="port-name">Port: {warning.port_name}</span>
										{/if}
									</div>
									<div class="issue-message">{warning.message}</div>
									{#if warning.suggestions && warning.suggestions.length > 0}
										<div class="issue-suggestion">
											<span class="suggestion-icon">💡</span>
											{warning.suggestions[0]}
										</div>
									{/if}
								</li>
							{/each}
						</ul>
					</section>
				{/if}

				{#if validationResult.errors.length === 0 && validationResult.warnings.length === 0}
					<div class="empty-state">
						<p>No validation issues found.</p>
					</div>
				{/if}
			</div>

			<div class="modal-footer">
				<button type="button" class="button-primary" onclick={onClose}>Close</button>
			</div>
		</div>
	</div>
{:else if isOpen && !validationResult}
	<div
		class="modal-backdrop"
		data-testid="modal-backdrop"
		onclick={handleBackdropClick}
		onkeydown={handleBackdropKeyDown}
		role="presentation"
	>
		<div
			bind:this={dialogElement}
			class="modal-dialog"
			role="dialog"
			aria-label="Validation Details"
			tabindex="0"
			onclick={(e) => e.stopPropagation()}
			onkeydown={(e) => e.stopPropagation()}
		>
			<div class="modal-header">
				<h2>Validation Details</h2>
				<button
					type="button"
					class="close-button"
					onclick={onClose}
					aria-label="Close"
				>
					✕
				</button>
			</div>
			<div class="modal-body">
				<div class="empty-state">
					<p>No validation results available.</p>
				</div>
			</div>
			<div class="modal-footer">
				<button type="button" class="button-primary" onclick={onClose}>Close</button>
			</div>
		</div>
	</div>
{/if}

<style>
	.modal-backdrop {
		position: fixed;
		top: 0;
		left: 0;
		right: 0;
		bottom: 0;
		background: var(--modal-backdrop);
		display: flex;
		align-items: center;
		justify-content: center;
		z-index: 1000;
	}

	.modal-dialog {
		background: var(--modal-background);
		border-radius: var(--modal-border-radius);
		box-shadow: var(--modal-shadow);
		max-width: 600px;
		width: 90%;
		max-height: 80vh;
		display: flex;
		flex-direction: column;
		overflow: hidden;
	}

	.modal-header {
		display: flex;
		justify-content: space-between;
		align-items: center;
		padding: 1.5rem;
		border-bottom: var(--modal-header-border-bottom);
	}

	.modal-header h2 {
		margin: 0;
		font-size: 1.25rem;
		font-weight: 600;
		color: var(--modal-header-text);
	}

	.close-button {
		background: none;
		border: none;
		font-size: 1.5rem;
		cursor: pointer;
		color: var(--ui-text-secondary);
		padding: 0;
		width: 2rem;
		height: 2rem;
		display: flex;
		align-items: center;
		justify-content: center;
		border-radius: 4px;
		transition: background-color 0.2s, color 0.2s;
	}

	.close-button:hover {
		background-color: var(--ui-surface-tertiary);
		color: var(--ui-text-primary);
	}

	.modal-body {
		padding: 1.5rem;
		overflow-y: auto;
		flex: 1;
	}

	.issues-section {
		margin-bottom: 1.5rem;
	}

	.issues-section:last-child {
		margin-bottom: 0;
	}

	.section-title {
		display: flex;
		align-items: center;
		gap: 0.5rem;
		font-size: 1rem;
		font-weight: 600;
		margin: 0 0 1rem 0;
		padding-bottom: 0.5rem;
		border-bottom: 2px solid;
	}

	.section-icon {
		font-size: 1.25rem;
	}

	.errors-section .section-title {
		color: var(--status-error);
		border-bottom-color: var(--status-error-container);
	}

	.warnings-section .section-title {
		color: var(--status-warning);
		border-bottom-color: var(--status-warning-container);
	}

	.issues-list {
		list-style: none;
		padding: 0;
		margin: 0;
		display: flex;
		flex-direction: column;
		gap: 0.75rem;
	}

	.issue-item {
		padding: 1rem;
		border-radius: 6px;
		border-left: 3px solid;
	}

	.error-item {
		background: var(--status-error-container);
		border-left-color: var(--status-error);
	}

	.warning-item {
		background: var(--status-warning-container);
		border-left-color: var(--status-warning);
	}

	.issue-header {
		display: flex;
		align-items: center;
		gap: 0.75rem;
		margin-bottom: 0.5rem;
		flex-wrap: wrap;
	}

	.component-name {
		font-size: 0.9375rem;
		font-weight: 600;
		color: var(--ui-text-primary);
	}

	.port-name {
		font-size: 0.8125rem;
		color: var(--ui-text-secondary);
		background: var(--ui-surface-tertiary);
		padding: 0.125rem 0.5rem;
		border-radius: 3px;
	}

	.issue-message {
		font-size: 0.9375rem;
		color: var(--ui-text-secondary);
		line-height: 1.5;
		margin-bottom: 0.5rem;
	}

	.issue-suggestion {
		display: flex;
		align-items: flex-start;
		gap: 0.5rem;
		font-size: 0.875rem;
		color: var(--ui-text-secondary);
		background: var(--ui-surface-primary);
		padding: 0.5rem 0.75rem;
		border-radius: 4px;
		line-height: 1.4;
	}

	.suggestion-icon {
		font-size: 1rem;
		flex-shrink: 0;
	}

	.empty-state {
		text-align: center;
		padding: 2rem;
		color: var(--ui-text-secondary);
	}

	.empty-state p {
		margin: 0;
	}

	.modal-footer {
		padding: 1rem 1.5rem;
		border-top: 1px solid var(--ui-border-subtle);
		display: flex;
		justify-content: flex-end;
	}

	.button-primary {
		padding: 0.5rem 1.5rem;
		background: var(--ui-interactive-primary);
		color: white;
		border: none;
		border-radius: 4px;
		font-size: 0.9375rem;
		font-weight: 500;
		cursor: pointer;
		transition: background-color 0.2s;
	}

	.button-primary:hover {
		background: var(--ui-interactive-primary-hover);
	}
</style>
