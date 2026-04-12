<script lang="ts">
	import { taskStore } from '$lib/stores/taskStore.svelte';
	import { agentStore } from '$lib/stores/agentStore.svelte';
	import KanbanBoard from '$lib/components/board/KanbanBoard.svelte';
	import TaskDetailPanel from '$lib/components/board/TaskDetailPanel.svelte';
</script>

<svelte:head>
	<title>Board - SemTeams</title>
</svelte:head>

<div class="board-page" data-testid="board-page">
	<h1 class="sr-only">Task Board</h1>
	<div class="board-status">
		<span
			class="connection-indicator"
			data-testid="connection-status"
			data-connected={agentStore.connected}
		>
			{agentStore.connected ? 'Connected' : 'Connecting...'}
		</span>
		<span class="task-count">{taskStore.tasks.length} tasks</span>
		{#if taskStore.needsAttentionCount > 0}
			<span class="attention-badge" data-testid="attention-badge">
				{taskStore.needsAttentionCount} needs you
			</span>
		{/if}
	</div>

	<div class="board-content">
		<KanbanBoard />
		{#if taskStore.selectedTask}
			<TaskDetailPanel
				task={taskStore.selectedTask}
				onClose={() => taskStore.deselectTask()}
			/>
		{/if}
	</div>
</div>

<style>
	.board-page {
		display: flex;
		flex-direction: column;
		width: 100%;
		height: 100%;
		overflow: hidden;
	}

	.board-status {
		display: flex;
		align-items: center;
		gap: 1rem;
		padding: 0.5rem 1rem;
		font-size: 0.8125rem;
		color: var(--ui-text-secondary, #6b7280);
		border-bottom: 1px solid var(--ui-border-subtle, #e5e7eb);
		flex-shrink: 0;
	}

	.connection-indicator {
		display: flex;
		align-items: center;
		gap: 0.375rem;
	}

	.connection-indicator::before {
		content: '';
		width: 6px;
		height: 6px;
		border-radius: 50%;
		background: var(--status-error, #ef4444);
	}

	.connection-indicator[data-connected='true']::before {
		background: var(--status-success, #22c55e);
	}

	.task-count {
		font-variant-numeric: tabular-nums;
	}

	.attention-badge {
		background: var(--col-needs-you, #f97316);
		color: white;
		padding: 0.125rem 0.5rem;
		border-radius: 9999px;
		font-size: 0.75rem;
		font-weight: 600;
	}

	.board-content {
		flex: 1;
		overflow: hidden;
		padding: 1rem;
		display: flex;
	}

	.sr-only {
		position: absolute;
		width: 1px;
		height: 1px;
		padding: 0;
		margin: -1px;
		overflow: hidden;
		clip: rect(0, 0, 0, 0);
		white-space: nowrap;
		border-width: 0;
	}
</style>
