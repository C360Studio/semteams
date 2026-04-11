<script lang="ts">
	import { SvelteMap } from "svelte/reactivity";
	import type { ValidationResult, ValidationIssue } from '$lib/types/validation';

	/**
	 * ValidationErrorDialog Component
	 * Displays structured validation errors from backend FlowGraph validation
	 *
	 * Props:
	 * - isOpen: boolean - Controls dialog visibility
	 * - validationResult: ValidationResult | null - Structured validation errors from backend
	 * - onClose: () => void - Callback when dialog closed
	 */

	interface Props {
		isOpen: boolean;
		validationResult: ValidationResult | null;
		onClose: () => void;
	}

	let { isOpen, validationResult, onClose }: Props = $props();

	// Dialog element reference for focus management
	let dialogElement: HTMLDivElement | undefined = $state();

	// Handle ESC key to close dialog
	function handleKeydown(event: KeyboardEvent) {
		if (event.key === 'Escape' && isOpen) {
			onClose();
		}
	}

	// Handle click on overlay background to close dialog
	function handleBackgroundClick(event: MouseEvent) {
		if (event.target === event.currentTarget) {
			onClose();
		}
	}

	// Focus management - focus close button when dialog opens
	$effect(() => {
		if (isOpen && dialogElement) {
			const focusable = dialogElement.querySelector<HTMLButtonElement>('.primary-button');
			focusable?.focus();
		}
	});

	// Group issues by component for better readability
	function groupByComponent(issues: ValidationIssue[]): SvelteMap<string, ValidationIssue[]> {
		const grouped = new SvelteMap<string, ValidationIssue[]>();
		for (const issue of issues) {
			const key = issue.component_name;
			if (!grouped.has(key)) {
				grouped.set(key, []);
			}
			grouped.get(key)!.push(issue);
		}
		return grouped;
	}

	let errorsByComponent: SvelteMap<string, ValidationIssue[]> = $derived(
		validationResult ? groupByComponent(validationResult.errors) : new SvelteMap()
	);
	let warningsByComponent: SvelteMap<string, ValidationIssue[]> = $derived(
		validationResult ? groupByComponent(validationResult.warnings) : new SvelteMap()
	);
</script>

<svelte:window onkeydown={handleKeydown} />

{#if isOpen && validationResult}
	<div
		bind:this={dialogElement}
		class="dialog-overlay"
		role="dialog"
		aria-modal="true"
		aria-labelledby="dialog-title"
		tabindex="-1"
		onclick={handleBackgroundClick}
		onkeydown={handleKeydown}
	>
		<div class="dialog-content">
			<header class="dialog-header">
				<h2 id="dialog-title">Flow Validation Failed</h2>
				<button class="close-button" onclick={onClose} aria-label="Close dialog">×</button>
			</header>

			<div class="dialog-body">
				{#if validationResult.errors.length > 0}
					<section class="errors-section">
						<h3>❌ Errors ({validationResult.errors.length})</h3>
						<p class="section-description">These issues must be fixed before deployment:</p>

						{#each [...errorsByComponent.entries()] as [componentName, issues]: [string, ValidationIssue[]] (componentName)}
							<div class="issue-group">
								<h4>{componentName}</h4>
								<ul>
									{#each issues as issue, idx (`${issue.component_name}-${issue.port_name || 'main'}-${issue.type}-${idx}`)}
										<li class="error-item">
											<div class="issue-message">
												{#if issue.port_name}
													<strong>{issue.port_name}:</strong>
												{/if}
												{issue.message}
											</div>
											{#if issue.suggestions && issue.suggestions.length > 0}
												<div class="suggestions">
													<strong>Suggestions:</strong>
													<ul>
														{#each issue.suggestions as suggestion, suggIdx (`${issue.component_name}-${issue.type}-suggestion-${suggIdx}`)}
															<li>{suggestion}</li>
														{/each}
													</ul>
												</div>
											{/if}
										</li>
									{/each}
								</ul>
							</div>
						{/each}
					</section>
				{/if}

				{#if validationResult.warnings.length > 0}
					<section class="warnings-section">
						<h3>⚠️ Warnings ({validationResult.warnings.length})</h3>
						<p class="section-description">
							These issues won't block deployment but should be reviewed:
						</p>

						{#each [...warningsByComponent.entries()] as [componentName, issues]: [string, ValidationIssue[]] (componentName)}
							<div class="issue-group">
								<h4>{componentName}</h4>
								<ul>
									{#each issues as issue, idx (`${issue.component_name}-${issue.port_name || 'main'}-${issue.type}-${idx}`)}
										<li class="warning-item">
											<div class="issue-message">
												{#if issue.port_name}
													<strong>{issue.port_name}:</strong>
												{/if}
												{issue.message}
											</div>
											{#if issue.suggestions && issue.suggestions.length > 0}
												<div class="suggestions">
													<strong>Suggestions:</strong>
													<ul>
														{#each issue.suggestions as suggestion, suggIdx (`${issue.component_name}-${issue.type}-suggestion-${suggIdx}`)}
															<li>{suggestion}</li>
														{/each}
													</ul>
												</div>
											{/if}
										</li>
									{/each}
								</ul>
							</div>
						{/each}
					</section>
				{/if}
			</div>

			<footer class="dialog-footer">
				<button class="primary-button" onclick={onClose}>Close and Edit Flow</button>
			</footer>
		</div>
	</div>
{/if}

<style>
	.dialog-overlay {
		position: fixed;
		top: 0;
		left: 0;
		right: 0;
		bottom: 0;
		background: rgba(0, 0, 0, 0.5);
		display: flex;
		align-items: center;
		justify-content: center;
		z-index: 1000;
	}

	.dialog-content {
		background: var(--ui-surface-primary);
		border-radius: 8px;
		max-width: 600px;
		width: 90%;
		max-height: 80vh;
		display: flex;
		flex-direction: column;
		box-shadow: 0 4px 20px rgba(0, 0, 0, 0.3);
	}

	.dialog-header {
		display: flex;
		justify-content: space-between;
		align-items: center;
		padding: 1.5rem;
		border-bottom: 1px solid var(--ui-border-subtle);
	}

	.dialog-header h2 {
		margin: 0;
		font-size: 1.5rem;
		color: var(--ui-text-primary);
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

	.dialog-body {
		flex: 1;
		overflow-y: auto;
		padding: 1.5rem;
	}

	.errors-section,
	.warnings-section {
		margin-bottom: 1.5rem;
	}

	.errors-section h3 {
		color: var(--status-error);
		margin-top: 0;
	}

	.warnings-section h3 {
		color: var(--status-warning);
		margin-top: 0;
	}

	.section-description {
		color: var(--ui-text-secondary);
		font-size: 0.9rem;
		margin-bottom: 1rem;
	}

	.issue-group {
		margin-bottom: 1rem;
		padding: 1rem;
		background: var(--ui-surface-secondary);
		border-radius: 4px;
		border-left: 3px solid var(--ui-border-subtle);
	}

	.issue-group h4 {
		margin: 0 0 0.5rem 0;
		font-size: 1rem;
		color: var(--ui-text-primary);
	}

	.issue-group ul {
		list-style: none;
		padding: 0;
		margin: 0;
	}

	.error-item,
	.warning-item {
		margin-bottom: 0.75rem;
		padding-left: 1rem;
	}

	.error-item {
		border-left: 3px solid var(--status-error);
	}

	.warning-item {
		border-left: 3px solid var(--status-warning);
	}

	.issue-message {
		margin-bottom: 0.5rem;
	}

	.suggestions {
		font-size: 0.9rem;
		color: var(--ui-text-secondary);
		margin-top: 0.5rem;
	}

	.suggestions ul {
		list-style: disc;
		padding-left: 1.5rem;
		margin: 0.25rem 0 0 0;
	}

	.suggestions li {
		margin-bottom: 0.25rem;
	}

	.dialog-footer {
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
	}

	.primary-button:hover {
		background: var(--ui-interactive-primary-hover);
	}
</style>
