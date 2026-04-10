<script lang="ts">
	import type { RuntimeStateInfo } from '$lib/types/ui-state';

	interface StatusBarProps {
		runtimeState: RuntimeStateInfo;
		isFlowValid: boolean; // Keep for deploy button disable logic
		// REMOVED: validationResult prop (Feature 015 - T012)
		onDeploy?: () => void;
		onStart?: () => void;
		onStop?: () => void;
		onToggleRuntimePanel?: () => void;
		showRuntimePanel?: boolean;
	}

	let {
		runtimeState,
		isFlowValid,
		onDeploy,
		onStart,
		onStop,
		onToggleRuntimePanel,
		showRuntimePanel = false
	}: StatusBarProps = $props();

	// Determine which buttons to show based on state
	const showDeploy = $derived(runtimeState.state === 'not_deployed');
	const showStart = $derived(runtimeState.state === 'deployed_stopped');
	const showStop = $derived(runtimeState.state === 'running');
	const showDebugButton = $derived(runtimeState.state === 'running');
	const isDeployDisabled = $derived(!isFlowValid || runtimeState.state !== 'not_deployed');

	// Compute tooltip for deploy button
	const deployTooltip = $derived.by(() => {
		if (runtimeState.state !== 'not_deployed') {
			return 'Flow must be in not_deployed state';
		}
		if (!isFlowValid) {
			return 'Fix validation errors before deploying';
		}
		return 'Deploy this flow';
	});
</script>

<div class="status-bar" data-testid="status-bar">
	<div class="status-section">
		<span class="status-label">Runtime:</span>
		<span
			class="runtime-state {runtimeState.state}"
			data-state={runtimeState.state}
			aria-live="polite"
			aria-atomic="true"
		>
			{#if runtimeState.state === 'running'}
				<span class="state-icon" role="img" aria-label="Running">▶️</span>
			{:else if runtimeState.state === 'deployed_stopped'}
				<span class="state-icon" role="img" aria-label="Stopped">⏹️</span>
			{:else if runtimeState.state === 'error'}
				<span class="state-icon" role="img" aria-label="Error">🔴</span>
			{/if}
			{runtimeState.state}
		</span>
	</div>

	{#if runtimeState.state === 'error' && runtimeState.message}
		<div class="error-section" role="alert">
			<span class="error-icon">⚠</span>
			<span class="error-message">{runtimeState.message}</span>
		</div>
	{/if}

	{#if runtimeState.state === 'running'}
		<div class="status-message" role="status">
			Cannot edit running flow
		</div>
	{/if}

	<div class="button-section">
		{#if showDeploy}
			<button
				onclick={() => onDeploy?.()}
				class="deploy-button"
				disabled={isDeployDisabled}
				aria-label="Deploy flow"
				title={deployTooltip}
			>
				Deploy
			</button>
		{/if}

		{#if showStart}
			<button
				onclick={() => onStart?.()}
				class="start-button"
				disabled={runtimeState.state !== 'deployed_stopped'}
				aria-label="Start flow"
			>
				Start
			</button>
		{/if}

		{#if showStop}
			<button
				onclick={() => onStop?.()}
				class="stop-button"
				disabled={runtimeState.state !== 'running'}
				aria-label="Stop flow"
			>
				Stop
			</button>
		{/if}

		{#if showDebugButton}
			<button
				onclick={() => onToggleRuntimePanel?.()}
				class="debug-button"
				aria-label="Toggle runtime panel"
				title="Toggle runtime panel (Ctrl+`)"
				data-testid="debug-toggle-button"
			>
				{showRuntimePanel ? '▼' : '▲'} Debug
			</button>
		{/if}
	</div>
</div>

<style>
	.status-bar {
		display: flex;
		justify-content: space-between;
		align-items: center;
		padding: 0.75rem 1rem;
		background: var(--statusbar-background);
		border-top: var(--statusbar-border-top);
		gap: 1rem;
	}

	.status-section {
		display: flex;
		align-items: center;
		gap: 0.5rem;
	}

	.error-section {
		display: flex;
		align-items: center;
		gap: 0.5rem;
		padding: 0.5rem 0.75rem;
		background: var(--status-error-container);
		border-radius: 4px;
		flex: 1;
	}

	.error-icon {
		color: var(--status-error);
		font-weight: bold;
	}

	.error-message {
		color: var(--status-error);
		font-size: 0.875rem;
	}

	.status-message {
		display: flex;
		align-items: center;
		padding: 0.5rem 0.75rem;
		background: var(--status-info-container);
		color: var(--status-info-on-container);
		border-radius: 4px;
		font-size: 0.875rem;
		font-weight: 500;
		flex: 1;
	}

	.status-label {
		font-weight: 600;
		font-size: 0.875rem;
		color: var(--ui-text-secondary);
	}

	.save-status,
	.runtime-state {
		padding: 0.25rem 0.75rem;
		border-radius: 12px;
		font-size: 0.75rem;
		font-weight: 500;
		text-transform: uppercase;
		display: flex;
		align-items: center;
		gap: 0.375rem;
	}

	.state-icon {
		font-size: 1rem;
		line-height: 1;
	}

	.save-status.clean {
		background: var(--status-success-container);
		color: var(--status-success-on-container);
	}

	.save-status.dirty {
		background: var(--status-warning-container);
		color: var(--status-warning-on-container);
	}

	.save-status.saving {
		background: var(--status-info-container);
		color: var(--status-info-on-container);
	}

	.save-status.failed {
		background: var(--status-error-container);
		color: var(--status-error);
	}

	.runtime-state.not_deployed {
		background: var(--ui-surface-secondary);
		color: var(--ui-text-secondary);
	}

	.runtime-state.deployed_stopped {
		background: var(--status-warning-container);
		color: var(--status-warning-on-container);
	}

	.runtime-state.running {
		background: var(--status-success-container);
		color: var(--status-success-on-container);
	}

	.runtime-state.error {
		background: var(--status-error-container);
		color: var(--status-error);
	}

	.button-section {
		display: flex;
		gap: 0.5rem;
	}

	button {
		padding: 0.5rem 1rem;
		border: none;
		border-radius: 4px;
		font-size: 0.875rem;
		font-weight: 500;
		cursor: pointer;
		transition: all 0.2s;
		background: var(--ui-interactive-primary);
		color: white;
	}

	button:hover:not(:disabled) {
		background: var(--ui-interactive-primary-hover);
	}

	button:disabled {
		opacity: 0.5;
		cursor: not-allowed;
	}

	.deploy-button {
		background: var(--ui-interactive-primary);
	}

	.start-button {
		background: var(--statusbar-indicator-running);
	}

	.start-button:hover:not(:disabled) {
		background: var(--status-success-hover);
	}

	.stop-button {
		background: var(--statusbar-indicator-warning);
		color: var(--ui-text-on-warning);
	}

	.stop-button:hover:not(:disabled) {
		background: var(--status-warning-hover);
	}

	.undeploy-button {
		background: var(--ui-interactive-secondary);
	}

	.undeploy-button:hover:not(:disabled) {
		background: var(--ui-interactive-secondary-hover);
	}

	.debug-button {
		background: var(--ui-interactive-secondary);
		display: flex;
		align-items: center;
		gap: 0.25rem;
	}

	.debug-button:hover:not(:disabled) {
		background: var(--ui-interactive-secondary-hover);
	}
</style>
