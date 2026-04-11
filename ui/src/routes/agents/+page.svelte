<script lang="ts">
	import { agentStore } from '$lib/stores/agentStore.svelte';
	import { agentApi } from '$lib/services/agentApi';
	import { isActiveState } from '$lib/types/agent';
	import TrajectoryViewer from '$lib/components/agents/TrajectoryViewer.svelte';

	type FilterState = 'all' | 'active' | 'paused' | 'awaiting_approval' | 'complete' | 'failed';
	let filter = $state<FilterState>('all');
	let selectedLoopId = $state<string | null>(null);

	function toggleTrajectory(loopId: string) {
		selectedLoopId = selectedLoopId === loopId ? null : loopId;
	}

	let filteredLoops = $derived.by(() => {
		const all = agentStore.loopsList;
		switch (filter) {
			case 'all':
				return all;
			case 'active':
				return all.filter((l) => isActiveState(l.state));
			case 'paused':
				return all.filter((l) => l.state === 'paused');
			case 'awaiting_approval':
				return all.filter((l) => l.state === 'awaiting_approval');
			case 'complete':
				return all.filter((l) => l.state === 'complete');
			case 'failed':
				return all.filter((l) => l.state === 'failed');
		}
	});

	const filterTabs: { key: FilterState; label: string }[] = [
		{ key: 'all', label: 'All' },
		{ key: 'active', label: 'Active' },
		{ key: 'paused', label: 'Paused' },
		{ key: 'awaiting_approval', label: 'Awaiting Approval' },
		{ key: 'complete', label: 'Complete' },
		{ key: 'failed', label: 'Failed' }
	];

	async function handleSignal(
		loopId: string,
		signal: 'pause' | 'resume' | 'cancel' | 'approve' | 'reject'
	) {
		try {
			await agentApi.sendSignal(loopId, signal);
		} catch (e) {
			console.error(`Failed to send ${signal} signal:`, e);
		}
	}
</script>

<svelte:head>
	<title>Agents - SemStreams</title>
</svelte:head>

<main>
	<div class="page-header">
		<div class="header-row">
			<!-- eslint-disable-next-line svelte/no-navigation-without-resolve -->
			<a href="/" class="back-link">← Graph</a>
			<h1>Agents</h1>
			<span
				class="connection-status"
				data-testid="connection-status"
				data-connected={agentStore.connected}
			>
				{agentStore.connected ? 'Connected' : 'Disconnected'}
			</span>
		</div>
	</div>

	<div class="filter-tabs" data-testid="filter-tabs">
		{#each filterTabs as tab (tab.key)}
			<button
				class="filter-tab"
				class:active={filter === tab.key}
				data-testid="filter-{tab.key}"
				onclick={() => (filter = tab.key)}
			>
				{tab.label}
			</button>
		{/each}
	</div>

	{#if filteredLoops.length === 0}
		<div class="empty-state" data-testid="empty-state">
			<p>No agent loops</p>
		</div>
	{:else}
		<table class="loops-table" data-testid="loops-table">
			<thead>
				<tr>
					<th>ID</th>
					<th>State</th>
					<th>Role</th>
					<th>Progress</th>
					<th>User</th>
					<th>Actions</th>
				</tr>
			</thead>
			<tbody>
				{#each filteredLoops as loop (loop.loop_id)}
					<tr data-testid="loop-row">
						<td class="loop-id">{loop.loop_id.slice(0, 12)}</td>
						<td
							><span class="state-badge {loop.state}"
								>{loop.state.replace(/_/g, ' ')}</span
							></td
						>
						<td>{loop.role}</td>
						<td>{loop.iterations}/{loop.max_iterations}</td>
						<td>{loop.user_id}</td>
						<td class="actions">
							{#if isActiveState(loop.state)}
								<button onclick={() => handleSignal(loop.loop_id, 'pause')}>Pause</button>
								<button onclick={() => handleSignal(loop.loop_id, 'cancel')}>Cancel</button>
							{:else if loop.state === 'paused'}
								<button onclick={() => handleSignal(loop.loop_id, 'resume')}>Resume</button>
								<button onclick={() => handleSignal(loop.loop_id, 'cancel')}>Cancel</button>
							{:else if loop.state === 'awaiting_approval'}
								<button onclick={() => handleSignal(loop.loop_id, 'approve')}>Approve</button>
								<button onclick={() => handleSignal(loop.loop_id, 'reject')}>Reject</button>
							{/if}
							{#if loop.state === 'complete' || loop.state === 'failed'}
								<button
									data-testid="view-trajectory-{loop.loop_id}"
									onclick={() => toggleTrajectory(loop.loop_id)}
								>
									{selectedLoopId === loop.loop_id ? 'Hide' : 'View'}
								</button>
							{/if}
						</td>
					</tr>
					{#if selectedLoopId === loop.loop_id}
						<tr class="trajectory-row" data-testid="trajectory-row">
							<td colspan="6">
								<TrajectoryViewer loopId={loop.loop_id} />
							</td>
						</tr>
					{/if}
				{/each}
			</tbody>
		</table>
	{/if}
</main>

<style>
	main {
		max-width: 1200px;
		margin: 0 auto;
		padding: 2rem;
	}

	.page-header {
		margin-bottom: 2rem;
	}

	.header-row {
		display: flex;
		align-items: center;
		gap: 1rem;
	}

	.back-link {
		color: var(--ui-interactive-primary);
		text-decoration: none;
		font-weight: 500;
		font-size: 0.875rem;
		padding: 0.25rem 0.5rem;
		border-radius: 4px;
		transition: background-color 0.2s;
	}

	.back-link:hover {
		background-color: var(--ui-surface-secondary);
	}

	.page-header h1 {
		font-size: 2rem;
		margin: 0;
		color: var(--ui-text-primary);
	}

	.connection-status {
		margin-left: auto;
		font-size: 0.875rem;
		font-weight: 500;
		display: flex;
		align-items: center;
		gap: 0.5rem;
	}

	.connection-status::before {
		content: '';
		width: 8px;
		height: 8px;
		border-radius: 50%;
		background: var(--status-error);
	}

	.connection-status[data-connected='true']::before {
		background: var(--status-success);
	}

	.filter-tabs {
		display: flex;
		gap: 0.25rem;
		margin-bottom: 1.5rem;
		border-bottom: 1px solid var(--ui-border-subtle);
		padding-bottom: 0;
	}

	.filter-tab {
		padding: 0.5rem 1rem;
		border: none;
		background: none;
		color: var(--ui-text-secondary);
		font-size: 0.875rem;
		font-weight: 500;
		cursor: pointer;
		border-bottom: 2px solid transparent;
		transition: all 0.15s;
	}

	.filter-tab:hover {
		color: var(--ui-text-primary);
	}

	.filter-tab.active {
		color: var(--ui-interactive-primary);
		border-bottom-color: var(--ui-interactive-primary);
	}

	.empty-state {
		text-align: center;
		padding: 3rem 1rem;
		color: var(--ui-text-secondary);
	}

	.empty-state p {
		margin: 0;
		font-size: 1.125rem;
	}

	.loops-table {
		width: 100%;
		border-collapse: collapse;
		font-size: 0.875rem;
	}

	.loops-table th {
		text-align: left;
		padding: 0.75rem 1rem;
		font-weight: 600;
		color: var(--ui-text-secondary);
		border-bottom: 1px solid var(--ui-border-subtle);
		font-size: 0.75rem;
		text-transform: uppercase;
		letter-spacing: 0.05em;
	}

	.loops-table td {
		padding: 0.75rem 1rem;
		border-bottom: 1px solid var(--ui-border-subtle);
		color: var(--ui-text-primary);
	}

	.loops-table tbody tr:hover {
		background: var(--ui-surface-secondary);
	}

	.loop-id {
		font-family: monospace;
		font-size: 0.8125rem;
		color: var(--ui-text-secondary);
	}

	.state-badge {
		display: inline-block;
		padding: 0.125rem 0.5rem;
		border-radius: 10px;
		font-size: 0.75rem;
		font-weight: 500;
		text-transform: capitalize;
	}

	.state-badge.exploring,
	.state-badge.planning,
	.state-badge.architecting,
	.state-badge.executing,
	.state-badge.reviewing {
		background: var(--status-info-container, rgba(59, 130, 246, 0.15));
		color: var(--status-info, #3b82f6);
	}

	.state-badge.paused {
		background: var(--status-warning-container, rgba(234, 179, 8, 0.15));
		color: var(--status-warning, #eab308);
	}

	.state-badge.awaiting_approval {
		background: var(--status-warning-container, rgba(234, 179, 8, 0.15));
		color: var(--status-warning, #eab308);
	}

	.state-badge.complete {
		background: var(--status-success-container, rgba(34, 197, 94, 0.15));
		color: var(--status-success, #22c55e);
	}

	.state-badge.failed,
	.state-badge.cancelled {
		background: var(--status-error-container, rgba(239, 68, 68, 0.15));
		color: var(--status-error, #ef4444);
	}

	.actions {
		display: flex;
		gap: 0.5rem;
	}

	.actions button {
		padding: 0.25rem 0.75rem;
		border: 1px solid var(--ui-border-subtle);
		border-radius: 4px;
		background: var(--ui-surface-primary);
		color: var(--ui-text-primary);
		font-size: 0.75rem;
		cursor: pointer;
		transition: all 0.15s;
	}

	.actions button:hover {
		background: var(--ui-surface-secondary);
		border-color: var(--ui-interactive-primary);
	}

	.trajectory-row td {
		padding: 0;
		border-bottom: 1px solid var(--ui-border-subtle);
	}

	.trajectory-row td :global(.trajectory-viewer) {
		margin: 0.5rem 1rem 1rem;
	}
</style>
