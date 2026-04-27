<script lang="ts">
	import { taskStore } from '$lib/stores/taskStore.svelte';
	import KanbanBoard from '$lib/components/board/KanbanBoard.svelte';
	import TaskDetailPanel from '$lib/components/board/TaskDetailPanel.svelte';
</script>

<svelte:head>
	<title>Board - SemTeams</title>
</svelte:head>

<div class="board-page" data-testid="board-page">
	<h1 class="sr-only">Task Board</h1>
	<!-- Global status (connection + needs-you) lives in TopNav top-right.
	     Per-column counts are on the column-toggle chips below. -->
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
