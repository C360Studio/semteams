<script lang="ts">
	import type { Flow } from '$lib/types/flow';

	interface FlowListProps {
		flows: Flow[];
		onFlowClick?: (flowId: string) => void;
		onCreate?: () => void;
	}

	let { flows, onFlowClick, onCreate }: FlowListProps = $props();
</script>

<div class="flow-list">
	<header>
		<h2>Flows</h2>
		<button onclick={() => onCreate?.()} aria-label="Create New Flow">Create New Flow</button>
	</header>

	{#if flows.length === 0}
		<div class="empty-state">
			<p>No flows yet. Create your first flow to get started.</p>
		</div>
	{:else}
		<div class="flow-grid">
			{#each flows as flow (flow.id)}
				<button class="flow-card" onclick={() => onFlowClick?.(flow.id)}>
					<h3>{flow.name}</h3>
					{#if flow.description}
						<p class="description">{flow.description}</p>
					{/if}
					<span class="runtime-state {flow.runtime_state}">
						{flow.runtime_state}
					</span>
				</button>
			{/each}
		</div>
	{/if}
</div>

<style>
	.flow-list {
		padding: 1rem;
	}

	header {
		display: flex;
		justify-content: space-between;
		align-items: center;
		margin-bottom: 2rem;
	}

	header h2 {
		color: var(--ui-text-primary);
		margin: 0;
	}

	header button {
		background: var(--button-primary-background);
		color: var(--button-primary-text);
		border: none;
		padding: 0.5rem 1rem;
		border-radius: var(--radius-md);
		cursor: pointer;
		font-weight: 500;
		transition: background 0.2s;
	}

	header button:hover {
		background: var(--button-primary-background-hover);
	}

	.empty-state {
		text-align: center;
		padding: 3rem;
		color: var(--ui-text-tertiary);
	}

	.flow-grid {
		display: grid;
		grid-template-columns: repeat(auto-fill, minmax(280px, 1fr));
		gap: 1rem;
	}

	.flow-card {
		background: var(--ui-surface-secondary);
		border: 1px solid var(--ui-border-subtle);
		border-radius: 8px;
		padding: 1.5rem;
		text-align: left;
		cursor: pointer;
		transition: all 0.2s;
	}

	.flow-card:hover {
		border-color: var(--ui-interactive-primary);
		box-shadow: 0 2px 8px rgba(0, 0, 0, 0.3);
	}

	.flow-card h3 {
		margin: 0 0 0.5rem 0;
		font-size: 1.25rem;
		color: var(--ui-text-primary);
	}

	.description {
		color: var(--ui-text-secondary);
		margin: 0 0 1rem 0;
		font-size: 0.9rem;
	}

	.runtime-state {
		display: inline-block;
		padding: 0.25rem 0.75rem;
		border-radius: 12px;
		font-size: 0.75rem;
		font-weight: 500;
		text-transform: uppercase;
	}

	.runtime-state.not_deployed {
		background: var(--ui-surface-tertiary);
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
		color: var(--status-error-on-container);
	}
</style>
