<script lang="ts">
	import { agentApi } from '$lib/services/agentApi';
	import type { TrajectoryEntry } from '$lib/types/agent';

	interface Props {
		loopId: string;
	}

	let { loopId }: Props = $props();

	let trajectory = $state<TrajectoryEntry | null>(null);
	let loading = $state(true);
	let error = $state<string | null>(null);

	$effect(() => {
		loadTrajectory(loopId);
	});

	async function loadTrajectory(id: string) {
		loading = true;
		error = null;
		try {
			trajectory = await agentApi.getTrajectory(id);
		} catch (e) {
			error = e instanceof Error ? e.message : 'Failed to load trajectory';
		} finally {
			loading = false;
		}
	}
</script>

<div data-testid="trajectory-viewer" class="trajectory-viewer">
	{#if loading}
		<div class="loading" data-testid="trajectory-loading">Loading trajectory...</div>
	{:else if error}
		<div class="error" data-testid="trajectory-error">{error}</div>
	{:else if trajectory}
		<div class="trajectory-header">
			<h3>{trajectory.role}</h3>
			<div class="trajectory-meta">
				<span class="outcome" data-testid="trajectory-outcome">{trajectory.outcome}</span>
				<span class="duration">{trajectory.duration_ms}ms</span>
				<span class="iterations">{trajectory.iterations} iterations</span>
			</div>
		</div>

		{#if trajectory.token_usage}
			<div class="token-usage" data-testid="token-usage">
				<span>Input: {trajectory.token_usage.input_tokens.toLocaleString()} tokens</span>
				<span>Output: {trajectory.token_usage.output_tokens.toLocaleString()} tokens</span>
			</div>
		{/if}

		{#if trajectory.tool_calls && trajectory.tool_calls.length > 0}
			<div class="tool-calls" data-testid="tool-calls">
				<h4>Tool Calls ({trajectory.tool_calls.length})</h4>
				{#each trajectory.tool_calls as call, i (i)}
					<div class="tool-call-entry" data-testid="tool-call-entry">
						<div class="tool-call-header">
							<span class="tool-name">{call.name}</span>
							{#if call.duration_ms != null}
								<span class="tool-duration">{call.duration_ms}ms</span>
							{/if}
						</div>
						<details>
							<summary>Arguments</summary>
							<pre>{JSON.stringify(call.args, null, 2)}</pre>
						</details>
						{#if call.result}
							<details>
								<summary>Result</summary>
								<pre>{call.result}</pre>
							</details>
						{/if}
						{#if call.error}
							<div class="tool-error" data-testid="tool-call-error">{call.error}</div>
						{/if}
					</div>
				{/each}
			</div>
		{/if}
	{/if}
</div>

<style>
	.trajectory-viewer {
		padding: 1rem;
		background: var(--ui-surface-secondary);
		border-radius: 8px;
		border: 1px solid var(--ui-border-subtle);
	}

	.loading {
		color: var(--ui-text-secondary);
		font-size: 0.875rem;
		padding: 1rem 0;
	}

	.error {
		color: var(--status-error, #ef4444);
		font-size: 0.875rem;
		padding: 0.5rem 0.75rem;
		background: var(--status-error-container, rgba(239, 68, 68, 0.15));
		border-radius: 4px;
	}

	.trajectory-header {
		display: flex;
		align-items: center;
		justify-content: space-between;
		gap: 1rem;
		margin-bottom: 1rem;
	}

	.trajectory-header h3 {
		margin: 0;
		font-size: 1rem;
		font-weight: 600;
		color: var(--ui-text-primary);
		text-transform: capitalize;
	}

	.trajectory-meta {
		display: flex;
		align-items: center;
		gap: 0.75rem;
		font-size: 0.8125rem;
		color: var(--ui-text-secondary);
	}

	.outcome {
		display: inline-block;
		padding: 0.125rem 0.5rem;
		border-radius: 10px;
		font-size: 0.75rem;
		font-weight: 500;
		text-transform: capitalize;
		background: var(--status-success-container, rgba(34, 197, 94, 0.15));
		color: var(--status-success, #22c55e);
	}

	.token-usage {
		display: flex;
		gap: 1.5rem;
		padding: 0.5rem 0.75rem;
		margin-bottom: 1rem;
		background: var(--ui-surface-tertiary);
		border-radius: 4px;
		font-size: 0.8125rem;
		color: var(--ui-text-secondary);
	}

	.tool-calls {
		margin-top: 0.5rem;
	}

	.tool-calls h4 {
		margin: 0 0 0.75rem 0;
		font-size: 0.875rem;
		font-weight: 600;
		color: var(--ui-text-primary);
	}

	.tool-call-entry {
		padding: 0.5rem 0.75rem;
		margin-bottom: 0.5rem;
		background: var(--ui-surface-tertiary);
		border-radius: 4px;
		border-left: 3px solid var(--ui-interactive-primary, #78a9ff);
	}

	.tool-call-header {
		display: flex;
		align-items: center;
		justify-content: space-between;
		margin-bottom: 0.25rem;
	}

	.tool-name {
		font-family: monospace;
		font-size: 0.8125rem;
		font-weight: 600;
		color: var(--ui-interactive-primary, #78a9ff);
	}

	.tool-duration {
		font-size: 0.75rem;
		color: var(--ui-text-tertiary);
	}

	details {
		margin-top: 0.25rem;
	}

	summary {
		font-size: 0.75rem;
		color: var(--ui-text-tertiary);
		cursor: pointer;
		user-select: none;
	}

	summary:hover {
		color: var(--ui-text-secondary);
	}

	pre {
		margin: 0.25rem 0 0 0;
		padding: 0.5rem;
		background: var(--ui-surface-primary);
		border-radius: 4px;
		font-size: 0.75rem;
		color: var(--ui-text-secondary);
		overflow-x: auto;
		white-space: pre-wrap;
		word-break: break-word;
	}

	.tool-error {
		margin-top: 0.25rem;
		padding: 0.25rem 0.5rem;
		font-size: 0.75rem;
		color: var(--status-error, #ef4444);
		background: var(--status-error-container, rgba(239, 68, 68, 0.15));
		border-radius: 4px;
	}
</style>
