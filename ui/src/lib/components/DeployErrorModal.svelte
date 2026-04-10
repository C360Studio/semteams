<script lang="ts">
	import { SvelteMap } from "svelte/reactivity";
	import type { ValidationResult, ValidationIssue } from '$lib/types/port';

	/**
	 * DeployErrorModal Component
	 * Blocks deployment when validation errors exist
	 * Shows clear error messages and guides user to fix issues
	 *
	 * Props:
	 * - isOpen: boolean - Controls modal visibility
	 * - validationResult: ValidationResult | null - Validation errors blocking deployment
	 * - onClose: () => void - Callback when modal closed
	 */

	interface Props {
		isOpen: boolean;
		validationResult: ValidationResult | null;
		onClose: () => void;
	}

	let { isOpen, validationResult, onClose }: Props = $props();

	// Modal element reference for focus management
	let modalElement: HTMLDivElement | undefined = $state();

	// Handle ESC key to close modal
	function handleKeydown(event: KeyboardEvent) {
		if (event.key === 'Escape' && isOpen) {
			onClose();
		}
	}

	// Handle click on overlay background to close modal
	function handleBackgroundClick(event: MouseEvent) {
		if (event.target === event.currentTarget) {
			onClose();
		}
	}

	// Focus management - focus close button when modal opens
	$effect(() => {
		if (isOpen && modalElement) {
			const focusable = modalElement.querySelector<HTMLButtonElement>('.primary-button');
			focusable?.focus();
		}
	});

	// Group errors by component for better readability
	const errorsByComponent = $derived.by(() => {
		if (!validationResult) return new SvelteMap();

		const grouped = new SvelteMap<string, ValidationIssue[]>();
		for (const error of validationResult.errors) {
			const key = error.component_name;
			if (!grouped.has(key)) {
				grouped.set(key, []);
			}
			grouped.get(key)!.push(error);
		}
		return grouped;
	});

	// Convert to typed array for template iteration
	const groupedErrorsArray = $derived.by(() => {
		return Array.from(errorsByComponent.entries()) as Array<[string, ValidationIssue[]]>;
	});

	const errorCount = $derived(validationResult?.errors.length || 0);
</script>

<svelte:window onkeydown={handleKeydown} />

{#if isOpen && validationResult && errorCount > 0}
	<div
		bind:this={modalElement}
		class="modal-overlay"
		role="dialog"
		aria-modal="true"
		aria-labelledby="modal-title"
		tabindex="-1"
		onclick={handleBackgroundClick}
		onkeydown={handleKeydown}
	>
		<div class="modal-content">
			<header class="modal-header">
				<h2 id="modal-title">Cannot Deploy Flow</h2>
				<button class="close-button" onclick={onClose} aria-label="Close modal">×</button>
			</header>

			<div class="modal-body">
				<div class="error-summary">
					<span class="error-icon">❌</span>
					<p class="error-message">
						This flow has <strong>{errorCount} error{errorCount > 1 ? 's' : ''}</strong> that must
						be fixed before deployment.
					</p>
				</div>

				<section class="errors-section">
					<h3>Errors to Fix:</h3>

					{#each groupedErrorsArray as [componentId, errors] (componentId)}
						<div class="error-group">
							<h4>{componentId}</h4>
							<ul>
								{#each errors as error (error.port_name ? `${componentId}-${error.port_name}` : `${componentId}-${error.message}`)}
									<li class="error-item">
										<div class="error-detail">
											{#if error.port_name}
												<strong>{error.port_name}:</strong>
											{/if}
											{error.message}
										</div>
										{#if error.suggestions && error.suggestions.length > 0}
											{#each error.suggestions as suggestion (suggestion)}
												<div class="suggestion">
													<strong>💡 Suggestion:</strong>
													{suggestion}
												</div>
											{/each}
										{/if}
									</li>
								{/each}
							</ul>
						</div>
					{/each}
				</section>
			</div>

			<footer class="modal-footer">
				<button class="primary-button" onclick={onClose}>OK, I'll Fix These Errors</button>
			</footer>
		</div>
	</div>
{/if}

<style>
	.modal-overlay {
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

	.modal-content {
		background: var(--modal-background);
		border-radius: var(--modal-border-radius);
		max-width: 600px;
		width: 90%;
		max-height: 80vh;
		display: flex;
		flex-direction: column;
		box-shadow: var(--modal-shadow);
	}

	.modal-header {
		display: flex;
		justify-content: space-between;
		align-items: center;
		padding: 1.5rem;
		border-bottom: var(--modal-header-border-bottom);
		background: var(--status-error-container);
	}

	.modal-header h2 {
		margin: 0;
		font-size: 1.5rem;
		color: var(--status-error);
	}

	.close-button {
		background: none;
		border: none;
		font-size: 2rem;
		cursor: pointer;
		color: var(--ui-text-secondary);
		padding: 0;
		width: 2rem;
		height: 2rem;
		line-height: 1;
	}

	.close-button:hover {
		color: var(--ui-text-primary);
	}

	.modal-body {
		flex: 1;
		overflow-y: auto;
		padding: 1.5rem;
	}

	.error-summary {
		display: flex;
		align-items: start;
		gap: 1rem;
		padding: 1rem;
		background: var(--ui-surface-secondary);
		border-radius: 4px;
		border-left: 4px solid var(--status-error);
		margin-bottom: 1.5rem;
	}

	.error-icon {
		font-size: 1.5rem;
		flex-shrink: 0;
	}

	.error-message {
		margin: 0;
		font-size: 1rem;
		line-height: 1.5;
	}

	.errors-section h3 {
		color: var(--status-error);
		margin-top: 0;
		margin-bottom: 1rem;
		font-size: 1.1rem;
	}

	.error-group {
		margin-bottom: 1rem;
		padding: 1rem;
		background: var(--ui-surface-secondary);
		border-radius: 4px;
		border-left: 3px solid var(--status-error);
	}

	.error-group h4 {
		margin: 0 0 0.5rem 0;
		font-size: 1rem;
		color: var(--ui-text-primary);
		font-weight: 600;
	}

	.error-group ul {
		list-style: none;
		padding: 0;
		margin: 0;
	}

	.error-item {
		margin-bottom: 0.75rem;
		padding-left: 0.5rem;
	}

	.error-item:last-child {
		margin-bottom: 0;
	}

	.error-detail {
		margin-bottom: 0.5rem;
		line-height: 1.4;
	}

	.suggestion {
		font-size: 0.9rem;
		color: var(--ui-text-secondary);
		margin-top: 0.5rem;
		padding-left: 1rem;
		border-left: 2px solid var(--ui-border-subtle);
	}

	.modal-footer {
		padding: 1.5rem;
		border-top: 1px solid var(--ui-border-subtle);
		display: flex;
		justify-content: flex-end;
	}

	.primary-button {
		padding: 0.75rem 1.5rem;
		background: var(--ui-interactive-primary);
		color: var(--ui-text-on-primary);
		border: none;
		border-radius: 4px;
		cursor: pointer;
		font-size: 1rem;
		font-weight: 500;
	}

	.primary-button:hover {
		background: var(--ui-interactive-primary-hover);
	}
</style>
