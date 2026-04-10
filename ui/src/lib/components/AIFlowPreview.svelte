<script lang="ts">
	/**
	 * AIFlowPreview Component
	 *
	 * Modal dialog for previewing AI-generated flows before applying them.
	 * Shows nodes, connections, and validation results with apply/reject actions.
	 */

	import type { Flow, FlowConnection } from '$lib/types/flow';
	import type { ValidationResult } from '$lib/types/validation';

	interface Props {
		isOpen: boolean;
		flow: Partial<Flow> | null;
		validationResult?: ValidationResult | null;
		loading?: boolean;
		error?: string | null;
		onApply?: () => void;
		onReject?: () => void;
		onRetry?: () => void;
		onClose?: () => void;
	}

	let {
		isOpen = false,
		flow = null,
		validationResult = null,
		loading = false,
		error = null,
		onApply,
		onReject,
		onRetry,
		onClose
	}: Props = $props();

	// Derived values
	let hasErrors = $derived(
		validationResult?.errors && validationResult.errors.length > 0
	);
	let hasWarnings = $derived(
		validationResult?.warnings && validationResult.warnings.length > 0
	);
	let isValid = $derived(validationResult?.validation_status === 'valid');
	let canApply = $derived(!hasErrors && flow !== null && !loading);

	let nodeCount = $derived(flow?.nodes?.length ?? 0);
	let connectionCount = $derived(flow?.connections?.length ?? 0);

	/**
	 * Handle escape key to close modal
	 */
	function handleKeyDown(event: KeyboardEvent) {
		if (event.key === 'Escape' && !loading) {
			handleClose();
		}
	}

	/**
	 * Handle backdrop click to close
	 */
	function handleBackdropClick(event: MouseEvent) {
		if (event.target === event.currentTarget && !loading) {
			handleClose();
		}
	}

	/**
	 * Handle close action
	 */
	function handleClose() {
		if (onClose) {
			onClose();
		}
	}

	/**
	 * Handle apply action
	 */
	function handleApply() {
		if (canApply && onApply) {
			onApply();
		}
	}

	/**
	 * Handle reject action
	 */
	function handleReject() {
		if (onReject) {
			onReject();
		}
	}

	/**
	 * Handle retry action
	 */
	function handleRetry() {
		if (onRetry) {
			onRetry();
		}
	}

	/**
	 * Get connection display text
	 */
	function getConnectionText(connection: FlowConnection): string {
		const sourceName =
			flow?.nodes?.find((n) => n.id === connection.source_node_id)?.name || 'Unknown';
		const targetName =
			flow?.nodes?.find((n) => n.id === connection.target_node_id)?.name || 'Unknown';
		return `${sourceName} (${connection.source_port}) → ${targetName} (${connection.target_port})`;
	}

	// Keyboard listener is registered via <svelte:window onkeydown> below
</script>

<svelte:window onkeydown={(e) => { if (isOpen) handleKeyDown(e); }} />

{#if isOpen}
	<div class="modal-backdrop backdrop" onclick={handleBackdropClick} role="presentation">
		<dialog open aria-modal="true" aria-label="Generated Flow Preview" onclick={(e) => e.stopPropagation()}>
			<article>
				<header>
					<h2>Generated Flow Preview</h2>
					<button
						type="button"
						class="close"
						onclick={handleClose}
						disabled={loading}
						aria-label="Close"
					>
						✕
					</button>
				</header>

				<div class="modal-content">
					{#if loading}
						<div class="loading-state" role="status" aria-live="polite">
							<div class="spinner"></div>
							<p>Generating flow...</p>
						</div>
					{:else if error}
						<div class="error-state" role="alert">
							<div class="error-icon">⚠️</div>
							<h3>Error</h3>
							<p>{error}</p>
						</div>
					{:else if flow}
						<!-- Flow Description -->
						{#if flow.description}
							<section class="flow-description">
								<p>{flow.description}</p>
							</section>
						{/if}

						<!-- Components Section -->
						<section class="flow-section">
							<h3>Components ({nodeCount})</h3>
							{#if nodeCount === 0}
								<p class="empty-state">No components in this flow</p>
							{:else}
								<ul class="node-list">
									{#each flow.nodes as node, index (node.id || index)}
										<li>
											<div>
												<strong class="node-name">{node.name}</strong>
												<span class="node-type"> ({node.component})</span>
											</div>
											{#if Object.keys(node.config || {}).length > 0}
												<div class="node-config">
													{#each Object.entries(node.config) as [key, value] (key)}
														<div class="config-item">
															{key}: {JSON.stringify(value)}
														</div>
													{/each}
												</div>
											{/if}
										</li>
									{/each}
								</ul>
							{/if}
						</section>

						<!-- Connections Section -->
						<section class="flow-section">
							<h3>Connections ({connectionCount})</h3>
							{#if connectionCount === 0}
								<p class="empty-state">No connections in this flow</p>
							{:else}
								<ul class="connection-list">
									{#each flow.connections as connection, index (connection.id || index)}
										<li>
											{getConnectionText(connection)}
										</li>
									{/each}
								</ul>
							{/if}
						</section>

						<!-- Validation Section -->
						{#if validationResult}
							<section class="validation-section">
								<h3>Validation</h3>

								{#if isValid}
									<div class="validation-success">
										<span class="icon">✓</span>
										<span>Flow is valid</span>
									</div>
								{/if}

								{#if hasErrors}
									<div class="validation-errors">
										<h4>
											<span class="icon">✗</span>
											{validationResult.errors.length} Error{validationResult.errors
												.length !== 1
												? 's'
												: ''}
										</h4>
										<ul>
											{#each validationResult.errors as errorItem, index (index)}
												<li data-node-id={errorItem.component_name} data-severity="error">
													<strong>{errorItem.component_name}</strong>
													{#if errorItem.port_name}
														<span class="port-name">({errorItem.port_name})</span>
													{/if}
													: {errorItem.message}
												</li>
											{/each}
										</ul>
									</div>
								{/if}

								{#if hasWarnings}
									<div class="validation-warnings">
										<h4>
											<span class="icon">⚠</span>
											{validationResult.warnings.length} Warning{validationResult.warnings
												.length !== 1
												? 's'
												: ''}
										</h4>
										<ul>
											{#each validationResult.warnings as warning, index (index)}
												<li data-node-id={warning.component_name} data-severity="warning">
													<strong>{warning.component_name}</strong>
													{#if warning.port_name}
														<span class="port-name">({warning.port_name})</span>
													{/if}
													: {warning.message}
												</li>
											{/each}
										</ul>
									</div>
								{/if}
							</section>
						{/if}
					{:else}
						<div class="empty-state">
							<p>No flow to preview</p>
						</div>
					{/if}
				</div>

				<footer>
					{#if error}
						<button type="button" onclick={handleRetry}>Retry</button>
					{:else if !loading}
						<button type="button" onclick={handleApply} disabled={!canApply} class="primary">
							Apply to Canvas
						</button>
						<button type="button" onclick={handleReject}>Reject</button>
						<button type="button" onclick={handleRetry}>Retry</button>
					{/if}
				</footer>
			</article>
		</dialog>
	</div>
{/if}

<style>
	.modal-backdrop {
		position: fixed;
		inset: 0;
		background-color: rgba(0, 0, 0, 0.5);
		display: flex;
		align-items: center;
		justify-content: center;
		z-index: 1000;
		padding: 1rem;
	}

	dialog {
		border: none;
		border-radius: var(--radius-md);
		max-width: 800px;
		width: 100%;
		max-height: 90vh;
		margin: 0;
		padding: 0;
		box-shadow: 0 4px 20px rgba(0, 0, 0, 0.3);
	}

	article {
		display: flex;
		flex-direction: column;
		max-height: 90vh;
		margin: 0;
	}

	header {
		display: flex;
		justify-content: space-between;
		align-items: center;
		padding: 1.5rem;
		border-bottom: 1px solid var(--ui-border-subtle);
	}

	header h2 {
		margin: 0;
		font-size: 1.5rem;
	}

	.close {
		background: none;
		border: none;
		font-size: 1.5rem;
		cursor: pointer;
		color: var(--ui-text-secondary);
		padding: 0.25rem 0.5rem;
		line-height: 1;
	}

	.close:hover:not(:disabled) {
		color: var(--ui-text-primary);
	}

	.modal-content {
		flex: 1;
		overflow-y: auto;
		padding: 1.5rem;
	}

	.loading-state {
		display: flex;
		flex-direction: column;
		align-items: center;
		justify-content: center;
		padding: 3rem;
		gap: 1rem;
	}

	.spinner {
		width: 3rem;
		height: 3rem;
		border: 4px solid var(--ui-border-subtle);
		border-top-color: var(--ui-interactive-primary);
		border-radius: 50%;
		animation: spin 0.8s linear infinite;
	}

	@keyframes spin {
		to {
			transform: rotate(360deg);
		}
	}

	.error-state {
		display: flex;
		flex-direction: column;
		align-items: center;
		padding: 2rem;
		gap: 1rem;
	}

	.error-icon {
		font-size: 3rem;
	}

	.error-state h3 {
		margin: 0;
		color: var(--status-error);
	}

	.flow-description {
		margin-bottom: 1.5rem;
		padding: 1rem;
		background-color: var(--ui-surface-elevated-1);
		border-radius: var(--radius-md);
	}

	.flow-section {
		margin-bottom: 1.5rem;
	}

	.flow-section h3 {
		margin-top: 0;
		margin-bottom: 0.75rem;
		font-size: 1.25rem;
	}

	.node-list,
	.connection-list {
		list-style: none;
		padding: 0;
		margin: 0;
	}

	.node-list li,
	.connection-list li {
		padding: 0.75rem;
		margin-bottom: 0.5rem;
		background-color: var(--ui-surface-elevated-1);
		border-radius: var(--radius-md);
		border: 1px solid var(--ui-border-subtle);
	}

	.node-type {
		color: var(--ui-text-secondary);
		font-size: 0.875rem;
		margin-left: 0.5rem;
	}

	.node-config {
		margin-top: 0.5rem;
		padding-left: 1rem;
		font-size: 0.875rem;
		color: var(--ui-text-secondary);
	}

	.config-item {
		margin-bottom: 0.25rem;
	}

	.empty-state {
		color: var(--ui-text-secondary);
		font-style: italic;
		text-align: center;
		padding: 2rem;
	}

	.validation-section {
		margin-top: 1.5rem;
		padding-top: 1.5rem;
		border-top: 1px solid var(--ui-border-subtle);
	}

	.validation-section h3 {
		margin-top: 0;
		margin-bottom: 1rem;
	}

	.validation-success {
		display: flex;
		align-items: center;
		gap: 0.5rem;
		padding: 0.75rem;
		background-color: var(--status-success-container);
		color: var(--status-success);
		border-radius: var(--radius-md);
		margin-bottom: 1rem;
	}

	.validation-success .icon {
		font-size: 1.25rem;
	}

	.validation-errors,
	.validation-warnings {
		margin-bottom: 1rem;
	}

	.validation-errors h4,
	.validation-warnings h4 {
		display: flex;
		align-items: center;
		gap: 0.5rem;
		margin: 0 0 0.75rem 0;
		font-size: 1rem;
	}

	.validation-errors h4 {
		color: var(--status-error);
	}

	.validation-warnings h4 {
		color: var(--status-warning);
	}

	.validation-errors ul,
	.validation-warnings ul {
		list-style: none;
		padding: 0;
		margin: 0;
	}

	.validation-errors li {
		padding: 0.5rem;
		margin-bottom: 0.5rem;
		background-color: var(--status-error-container);
		color: var(--status-error);
		border-left: 3px solid var(--status-error);
		border-radius: var(--radius-md);
	}

	.validation-warnings li {
		padding: 0.5rem;
		margin-bottom: 0.5rem;
		background-color: var(--status-warning-container);
		color: var(--status-warning);
		border-left: 3px solid var(--status-warning);
		border-radius: var(--radius-md);
	}

	.port-name {
		font-size: 0.875rem;
		opacity: 0.8;
	}

	footer {
		display: flex;
		gap: 0.75rem;
		justify-content: flex-end;
		padding: 1.5rem;
		border-top: 1px solid var(--ui-border-subtle);
	}

	footer button {
		padding: 0.5rem 1rem;
	}

	footer button.primary {
		background-color: var(--ui-interactive-primary);
		color: var(--ui-text-on-primary);
		border: none;
	}

	footer button.primary:hover:not(:disabled) {
		background-color: var(--ui-interactive-primary-hover);
	}

	footer button:disabled {
		opacity: 0.6;
		cursor: not-allowed;
	}
</style>
